package translation

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// 预定义错误
var (
	// ErrNoLLMClient LLM客户端未设置
	ErrNoLLMClient = errors.New("LLM client not configured")

	// ErrEmptyText 空文本错误
	ErrEmptyText = errors.New("empty text provided")

	// ErrInvalidConfig 无效配置
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrNoSteps 没有配置翻译步骤
	ErrNoSteps = errors.New("no translation steps configured")

	// ErrStepFailed 步骤执行失败
	ErrStepFailed = errors.New("translation step failed")

	// ErrChainFailed 翻译链执行失败
	ErrChainFailed = errors.New("translation chain failed")

	// ErrCacheFailed 缓存操作失败
	ErrCacheFailed = errors.New("cache operation failed")

	// ErrTimeout 超时错误
	ErrTimeout = errors.New("translation timeout")

	// ErrRateLimited 速率限制错误
	ErrRateLimited = errors.New("rate limited")

	// ErrContextCanceled 上下文取消错误
	ErrContextCanceled = errors.New("context canceled")
)

// TranslationError 翻译错误
type TranslationError struct {
	Code    string // 错误代码
	Message string // 错误消息
	Cause   error  // 原因
	Step    string // 发生错误的步骤
	Retry   bool   // 是否可重试
}

// Error 实现error接口
func (e *TranslationError) Error() string {
	if e.Step != "" {
		return fmt.Sprintf("[%s] %s at step '%s'", e.Code, e.Message, e.Step)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 返回原因错误
func (e *TranslationError) Unwrap() error {
	return e.Cause
}

// IsRetryable 是否可重试
func (e *TranslationError) IsRetryable() bool {
	return e.Retry
}

// NewTranslationError 创建翻译错误
func NewTranslationError(code, message string, cause error) *TranslationError {
	return &TranslationError{
		Code:    code,
		Message: message,
		Cause:   cause,
		Retry:   false,
	}
}

// NewRetryableError 创建可重试错误
func NewRetryableError(code, message string, cause error) *TranslationError {
	return &TranslationError{
		Code:    code,
		Message: message,
		Cause:   cause,
		Retry:   true,
	}
}

// 错误代码常量
const (
	ErrCodeConfig     = "CONFIG_ERROR"
	ErrCodeValidation = "VALIDATION_ERROR"
	ErrCodeLLM        = "LLM_ERROR"
	ErrCodeNetwork    = "NETWORK_ERROR"
	ErrCodeTimeout    = "TIMEOUT_ERROR"
	ErrCodeRateLimit  = "RATE_LIMIT_ERROR"
	ErrCodeCache      = "CACHE_ERROR"
	ErrCodeStep       = "STEP_ERROR"
	ErrCodeChain      = "CHAIN_ERROR"
	ErrCodeUnknown    = "UNKNOWN_ERROR"
)

// WrapError 包装错误
func WrapError(err error, code, message string) *TranslationError {
	if err == nil {
		return nil
	}

	// 如果已经是TranslationError，保留原有信息
	if te, ok := err.(*TranslationError); ok {
		te.Message = message + ": " + te.Message
		return te
	}

	// 判断是否可重试
	retry := isRetryableError(err)

	return &TranslationError{
		Code:    code,
		Message: message,
		Cause:   err,
		Retry:   retry,
	}
}

// isRetryableError 判断错误是否可重试
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 检查是否是预定义的可重试错误
	switch {
	case errors.Is(err, ErrTimeout),
		errors.Is(err, ErrRateLimited),
		errors.Is(err, context.DeadlineExceeded):
		return true
	}

	// 检查是否包含特定的错误信息
	errStr := err.Error()
	retryablePatterns := []string{
		"timeout",
		"deadline exceeded",
		"connection refused",
		"temporary failure",
		"rate limit",
		"429",
		"503",
		"504",
		"ContentLength",          // HTTP Content-Length 错误
		"Body length 0",          // HTTP Body 长度为0的错误
		"connection reset",       // 连接重置
		"broken pipe",            // 管道中断
		"no such host",           // DNS解析失败
		"network is unreachable", // 网络不可达
		"i/o timeout",            // I/O超时
	}

	for _, pattern := range retryablePatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// contains 检查字符串是否包含子串（不区分大小写）
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
