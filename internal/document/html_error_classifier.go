package document

import (
	"strings"
	"time"

	"go.uber.org/zap"
)

// HTMLErrorClassifier HTML错误分类器
type HTMLErrorClassifier struct {
	logger *zap.Logger
}

// NewHTMLErrorClassifier 创建HTML错误分类器
func NewHTMLErrorClassifier(logger *zap.Logger) *HTMLErrorClassifier {
	return &HTMLErrorClassifier{
		logger: logger,
	}
}

// ClassifyError 分类错误
func (c *HTMLErrorClassifier) ClassifyError(err error) RetryError {
	if err == nil {
		return RetryError{
			Timestamp:   time.Now(),
			ErrorType:   "unknown",
			ErrorCode:   "UNKNOWN",
			Message:     "no error provided",
			Severity:    "low",
			Recoverable: false,
		}
	}

	errMsg := strings.ToLower(err.Error())
	
	// 网络相关错误
	if c.isNetworkError(errMsg) {
		return RetryError{
			Timestamp:   time.Now(),
			ErrorType:   "network_error",
			ErrorCode:   c.getNetworkErrorCode(errMsg),
			Message:     err.Error(),
			Severity:    "high",
			Recoverable: true,
		}
	}

	// 超时错误
	if c.isTimeoutError(errMsg) {
		return RetryError{
			Timestamp:   time.Now(),
			ErrorType:   "timeout",
			ErrorCode:   "TIMEOUT",
			Message:     err.Error(),
			Severity:    "medium",
			Recoverable: true,
		}
	}

	// 限流错误
	if c.isRateLimitError(errMsg) {
		return RetryError{
			Timestamp:   time.Now(),
			ErrorType:   "rate_limit",
			ErrorCode:   "RATE_LIMIT",
			Message:     err.Error(),
			Severity:    "medium",
			Recoverable: true,
		}
	}

	// 认证错误
	if c.isAuthError(errMsg) {
		return RetryError{
			Timestamp:   time.Now(),
			ErrorType:   "auth_error",
			ErrorCode:   "AUTH_FAILED",
			Message:     err.Error(),
			Severity:    "high",
			Recoverable: false,
		}
	}

	// 配额错误
	if c.isQuotaError(errMsg) {
		return RetryError{
			Timestamp:   time.Now(),
			ErrorType:   "quota_error",
			ErrorCode:   "QUOTA_EXCEEDED",
			Message:     err.Error(),
			Severity:    "high",
			Recoverable: false,
		}
	}

	// 解析错误
	if c.isParsingError(errMsg) {
		return RetryError{
			Timestamp:   time.Now(),
			ErrorType:   "parsing_error",
			ErrorCode:   "PARSE_FAILED",
			Message:     err.Error(),
			Severity:    "medium",
			Recoverable: true,
		}
	}

	// 服务器错误
	if c.isServerError(errMsg) {
		return RetryError{
			Timestamp:   time.Now(),
			ErrorType:   "server_error",
			ErrorCode:   c.getServerErrorCode(errMsg),
			Message:     err.Error(),
			Severity:    "high",
			Recoverable: true,
		}
	}

	// 默认分类
	return RetryError{
		Timestamp:   time.Now(),
		ErrorType:   "generic_error",
		ErrorCode:   "GENERIC",
		Message:     err.Error(),
		Severity:    "medium",
		Recoverable: true,
	}
}

// isNetworkError 检查是否为网络错误
func (c *HTMLErrorClassifier) isNetworkError(errMsg string) bool {
	networkKeywords := []string{
		"connection refused",
		"connection reset",
		"connection timeout",
		"network unreachable",
		"no route to host",
		"name resolution failed",
		"dns lookup failed",
		"connection aborted",
		"socket",
		"tcp",
		"udp",
	}

	for _, keyword := range networkKeywords {
		if strings.Contains(errMsg, keyword) {
			return true
		}
	}

	return false
}

// getNetworkErrorCode 获取网络错误代码
func (c *HTMLErrorClassifier) getNetworkErrorCode(errMsg string) string {
	if strings.Contains(errMsg, "connection refused") {
		return "CONNECTION_REFUSED"
	}
	if strings.Contains(errMsg, "connection reset") {
		return "CONNECTION_RESET"
	}
	if strings.Contains(errMsg, "timeout") {
		return "CONNECTION_TIMEOUT"
	}
	if strings.Contains(errMsg, "unreachable") {
		return "NETWORK_UNREACHABLE"
	}
	if strings.Contains(errMsg, "dns") || strings.Contains(errMsg, "name resolution") {
		return "DNS_FAILED"
	}
	return "NETWORK_ERROR"
}

// isTimeoutError 检查是否为超时错误
func (c *HTMLErrorClassifier) isTimeoutError(errMsg string) bool {
	timeoutKeywords := []string{
		"timeout",
		"deadline exceeded",
		"request timeout",
		"operation timeout",
		"read timeout",
		"write timeout",
	}

	for _, keyword := range timeoutKeywords {
		if strings.Contains(errMsg, keyword) {
			return true
		}
	}

	return false
}

// isRateLimitError 检查是否为限流错误
func (c *HTMLErrorClassifier) isRateLimitError(errMsg string) bool {
	rateLimitKeywords := []string{
		"rate limit",
		"too many requests",
		"quota exceeded",
		"throttle",
		"rate exceeded",
		"429",
		"503",
	}

	for _, keyword := range rateLimitKeywords {
		if strings.Contains(errMsg, keyword) {
			return true
		}
	}

	return false
}

// isAuthError 检查是否为认证错误
func (c *HTMLErrorClassifier) isAuthError(errMsg string) bool {
	authKeywords := []string{
		"unauthorized",
		"authentication failed",
		"invalid token",
		"access denied",
		"forbidden",
		"401",
		"403",
		"api key",
		"invalid credentials",
	}

	for _, keyword := range authKeywords {
		if strings.Contains(errMsg, keyword) {
			return true
		}
	}

	return false
}

// isQuotaError 检查是否为配额错误
func (c *HTMLErrorClassifier) isQuotaError(errMsg string) bool {
	quotaKeywords := []string{
		"quota exceeded",
		"billing",
		"usage limit",
		"credit",
		"payment required",
		"402",
		"insufficient funds",
	}

	for _, keyword := range quotaKeywords {
		if strings.Contains(errMsg, keyword) {
			return true
		}
	}

	return false
}

// isParsingError 检查是否为解析错误
func (c *HTMLErrorClassifier) isParsingError(errMsg string) bool {
	parseKeywords := []string{
		"parse error",
		"parsing failed",
		"invalid json",
		"invalid xml",
		"invalid html",
		"malformed",
		"syntax error",
		"unexpected token",
		"invalid format",
	}

	for _, keyword := range parseKeywords {
		if strings.Contains(errMsg, keyword) {
			return true
		}
	}

	return false
}

// isServerError 检查是否为服务器错误
func (c *HTMLErrorClassifier) isServerError(errMsg string) bool {
	serverKeywords := []string{
		"internal server error",
		"bad gateway",
		"service unavailable",
		"gateway timeout",
		"500",
		"502",
		"503",
		"504",
		"server error",
	}

	for _, keyword := range serverKeywords {
		if strings.Contains(errMsg, keyword) {
			return true
		}
	}

	return false
}

// getServerErrorCode 获取服务器错误代码
func (c *HTMLErrorClassifier) getServerErrorCode(errMsg string) string {
	if strings.Contains(errMsg, "500") || strings.Contains(errMsg, "internal server error") {
		return "INTERNAL_SERVER_ERROR"
	}
	if strings.Contains(errMsg, "502") || strings.Contains(errMsg, "bad gateway") {
		return "BAD_GATEWAY"
	}
	if strings.Contains(errMsg, "503") || strings.Contains(errMsg, "service unavailable") {
		return "SERVICE_UNAVAILABLE"
	}
	if strings.Contains(errMsg, "504") || strings.Contains(errMsg, "gateway timeout") {
		return "GATEWAY_TIMEOUT"
	}
	return "SERVER_ERROR"
}