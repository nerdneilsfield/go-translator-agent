package retry

import (
	"context"
	"errors"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// RetryConfig 重试配置
type RetryConfig struct {
	// 最大重试次数（总重试次数）
	MaxRetries int `json:"max_retries"`

	// 网络错误专用重试次数（快速重试）
	NetworkMaxRetries int `json:"network_max_retries"`

	// 初始延迟时间
	InitialDelay time.Duration `json:"initial_delay"`

	// 最大延迟时间
	MaxDelay time.Duration `json:"max_delay"`

	// 退避因子（指数退避）
	BackoffFactor float64 `json:"backoff_factor"`

	// 网络错误的初始延迟（通常更短）
	NetworkInitialDelay time.Duration `json:"network_initial_delay"`

	// 网络错误的最大延迟
	NetworkMaxDelay time.Duration `json:"network_max_delay"`
}

// DefaultRetryConfig 返回默认重试配置
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:          3,
		NetworkMaxRetries:   5,
		InitialDelay:        1 * time.Second,
		MaxDelay:            30 * time.Second,
		BackoffFactor:       2.0,
		NetworkInitialDelay: 100 * time.Millisecond,
		NetworkMaxDelay:     5 * time.Second,
	}
}

// ErrorType 错误类型枚举
type ErrorType int

const (
	ErrorTypeNone          ErrorType = iota
	ErrorTypeNetwork                 // 网络瞬时错误
	ErrorTypeRetryableHTTP           // 可重试的HTTP错误
	ErrorTypeClientError             // 客户端错误（4xx）
	ErrorTypeServerError             // 服务端错误（5xx）
	ErrorTypePermanent               // 永久性错误
)

// NetworkRetrier 网络重试器
type NetworkRetrier struct {
	config RetryConfig
}

// NewNetworkRetrier 创建网络重试器
func NewNetworkRetrier(config RetryConfig) *NetworkRetrier {
	return &NetworkRetrier{
		config: config,
	}
}

// RetryableFunc 可重试的函数类型
type RetryableFunc func() (*http.Response, error)

// ExecuteWithRetry 执行带重试的函数
func (nr *NetworkRetrier) ExecuteWithRetry(ctx context.Context, fn RetryableFunc) (*http.Response, error) {
	var lastErr error
	var lastResp *http.Response

	// 网络错误的快速重试循环
	for networkRetry := 0; networkRetry <= nr.config.NetworkMaxRetries; networkRetry++ {
		// 总体重试循环
		for totalRetry := 0; totalRetry <= nr.config.MaxRetries; totalRetry++ {
			// 检查上下文是否被取消
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			// 执行函数
			resp, err := fn()

			// 成功的情况
			if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return resp, nil
			}

			// 分析错误类型
			errorType := nr.classifyError(err, resp)

			// 记录错误和响应
			lastErr = err
			if resp != nil {
				if lastResp != nil {
					lastResp.Body.Close()
				}
				lastResp = resp
			}

			// 根据错误类型决定是否重试
			shouldRetry, isNetworkError := nr.shouldRetry(errorType, totalRetry, networkRetry)
			if !shouldRetry {
				break
			}

			// 计算延迟时间
			delay := nr.calculateDelay(isNetworkError,
				totalRetry, networkRetry)

			// 等待后重试
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				// 继续重试
			}
		}

		// 如果不是网络错误，不进行网络级别的重试
		if lastErr == nil || !nr.isNetworkError(lastErr) {
			break
		}
	}

	// 返回最后的错误
	if lastErr != nil {
		return lastResp, lastErr
	}

	if lastResp != nil {
		return lastResp, nil
	}

	return nil, errors.New("no response received")
}

// classifyError 分类错误
func (nr *NetworkRetrier) classifyError(err error, resp *http.Response) ErrorType {
	// 网络错误
	if err != nil {
		if nr.isNetworkError(err) {
			return ErrorTypeNetwork
		}
		return ErrorTypePermanent
	}

	// HTTP状态码错误
	if resp != nil {
		switch {
		case resp.StatusCode >= 500:
			return ErrorTypeServerError
		case resp.StatusCode == 429: // Too Many Requests
			return ErrorTypeRetryableHTTP
		case resp.StatusCode >= 400:
			return ErrorTypeClientError
		}
	}

	return ErrorTypeNone
}

// isNetworkError 判断是否为网络错误
func (nr *NetworkRetrier) isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// 检查网络相关错误
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	// 检查URL错误
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return nr.isNetworkError(urlErr.Err)
	}

	// 检查系统调用错误
	var syscallErr *net.OpError
	if errors.As(err, &syscallErr) {
		return true
	}

	// 检查连接错误
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}

	// 检查错误消息模式
	errStr := strings.ToLower(err.Error())
	networkPatterns := []string{
		"connection refused",
		"connection reset",
		"connection timed out",
		"timeout",
		"temporary failure",
		"network is unreachable",
		"no such host",
		"broken pipe",
		"contentlength",
		"body length 0",
		"i/o timeout",
		"eof",
	}

	for _, pattern := range networkPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// shouldRetry 判断是否应该重试
func (nr *NetworkRetrier) shouldRetry(errorType ErrorType, totalRetry, networkRetry int) (bool, bool) {
	switch errorType {
	case ErrorTypeNetwork:
		// 网络错误：两层重试都检查
		return totalRetry < nr.config.MaxRetries &&
			networkRetry < nr.config.NetworkMaxRetries, true

	case ErrorTypeServerError, ErrorTypeRetryableHTTP:
		// 服务端错误和可重试HTTP错误：只检查总重试次数
		return totalRetry < nr.config.MaxRetries, false

	case ErrorTypeClientError, ErrorTypePermanent:
		// 客户端错误和永久性错误：不重试
		return false, false

	default:
		return false, false
	}
}

// calculateDelay 计算延迟时间
func (nr *NetworkRetrier) calculateDelay(isNetworkError bool, totalRetry, networkRetry int) time.Duration {
	var delay time.Duration
	var maxDelay time.Duration
	var retryCount int

	if isNetworkError {
		// 网络错误使用较短的延迟
		delay = nr.config.NetworkInitialDelay
		maxDelay = nr.config.NetworkMaxDelay
		retryCount = networkRetry
	} else {
		// 其他错误使用标准延迟
		delay = nr.config.InitialDelay
		maxDelay = nr.config.MaxDelay
		retryCount = totalRetry
	}

	// 指数退避
	if retryCount > 0 {
		backoffFactor := nr.config.BackoffFactor
		if backoffFactor <= 1.0 {
			backoffFactor = 2.0
		}

		multiplier := math.Pow(backoffFactor, float64(retryCount))
		delay = time.Duration(float64(delay) * multiplier)
	}

	// 限制最大延迟
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

// WrapHTTPClient 包装HTTP客户端，添加重试功能
func (nr *NetworkRetrier) WrapHTTPClient(client *http.Client) *RetryableHTTPClient {
	return &RetryableHTTPClient{
		client:  client,
		retrier: nr,
	}
}

// RetryableHTTPClient 可重试的HTTP客户端
type RetryableHTTPClient struct {
	client  *http.Client
	retrier *NetworkRetrier
}

// Do 执行HTTP请求（带重试）
func (rc *RetryableHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return rc.retrier.ExecuteWithRetry(req.Context(), func() (*http.Response, error) {
		// 克隆请求以避免Body被消费的问题
		clonedReq := req.Clone(req.Context())
		return rc.client.Do(clonedReq)
	})
}
