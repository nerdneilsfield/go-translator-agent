package adapter

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
)

// ConvertError 将新的错误类型转换为旧的错误类型
func ConvertError(err error) error {
	if err == nil {
		return nil
	}

	// 检查是否是 translation 包的错误
	var translationErr *translation.TranslationError
	if errors.As(err, &translationErr) {
		return convertTranslationError(translationErr)
	}

	// 检查特定的错误类型
	switch {
	case errors.Is(err, translation.ErrInvalidConfig):
		return translator.ErrInvalidConfig
	case errors.Is(err, translation.ErrNoLLMClient):
		return fmt.Errorf("no LLM client configured")
	case errors.Is(err, translation.ErrEmptyText):
		return fmt.Errorf("empty text provided")
	case errors.Is(err, translation.ErrStepFailed):
		return fmt.Errorf("translation step failed")
	case errors.Is(err, translation.ErrTimeout):
		return translator.ErrTimeout
	case errors.Is(err, translation.ErrRateLimited):
		return translator.ErrRateLimited
	}

	// 检查错误消息中的关键词
	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, "timeout"):
		return translator.ErrTimeout
	case strings.Contains(errMsg, "rate limit"):
		return translator.ErrRateLimited
	case strings.Contains(errMsg, "invalid"):
		return translator.ErrInvalidConfig
	case strings.Contains(errMsg, "API key"):
		return translator.ErrInvalidAPIKey
	}

	// 返回原始错误
	return err
}

// convertTranslationError 转换 translation.TranslationError 到旧的错误类型
func convertTranslationError(err *translation.TranslationError) error {
	switch err.Code {
	case "INVALID_CONFIG":
		return translator.ErrInvalidConfig
	case "TIMEOUT":
		return translator.ErrTimeout
	case "RATE_LIMITED":
		return translator.ErrRateLimited
	case "INVALID_API_KEY":
		return translator.ErrInvalidAPIKey
	case "CHUNK_TOO_LARGE":
		return translator.ErrChunkTooLarge
	case "CONTEXT_CANCELLED":
		return translator.ErrContextCancelled
	default:
		// 创建格式化的错误
		if err.Step != "" {
			return fmt.Errorf("%s at step '%s': %s", err.Code, err.Step, err.Message)
		}
		return fmt.Errorf("%s: %s", err.Code, err.Message)
	}
}

// WrapError 包装错误并添加上下文
func WrapError(err error, context string) error {
	if err == nil {
		return nil
	}

	// 转换错误
	convertedErr := ConvertError(err)
	
	// 添加上下文
	return fmt.Errorf("%s: %w", context, convertedErr)
}

// IsRetryableError 判断错误是否可重试
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 检查已知的可重试错误
	retryableErrors := []error{
		translator.ErrTimeout,
		translator.ErrRateLimited,
		translator.ErrContextCancelled,
	}

	for _, retryableErr := range retryableErrors {
		if errors.Is(err, retryableErr) {
			return true
		}
	}

	// 检查错误消息
	errMsg := err.Error()
	retryableKeywords := []string{
		"timeout",
		"rate limit",
		"temporary",
		"unavailable",
		"connection",
		"network",
	}

	for _, keyword := range retryableKeywords {
		if strings.Contains(strings.ToLower(errMsg), keyword) {
			return true
		}
	}

	return false
}

// ErrorSeverity 错误严重程度
type ErrorSeverity int

const (
	// SeverityInfo 信息级别
	SeverityInfo ErrorSeverity = iota
	// SeverityWarning 警告级别
	SeverityWarning
	// SeverityError 错误级别
	SeverityError
	// SeverityCritical 严重级别
	SeverityCritical
)

// GetErrorSeverity 获取错误的严重程度
func GetErrorSeverity(err error) ErrorSeverity {
	if err == nil {
		return SeverityInfo
	}

	// 严重错误
	criticalErrors := []error{
		translator.ErrInvalidConfig,
		translator.ErrInvalidAPIKey,
	}
	for _, criticalErr := range criticalErrors {
		if errors.Is(err, criticalErr) {
			return SeverityCritical
		}
	}

	// 错误级别
	errorLevelErrors := []error{
		translator.ErrChunkTooLarge,
		translator.ErrContextCancelled,
	}
	for _, errorLevelErr := range errorLevelErrors {
		if errors.Is(err, errorLevelErr) {
			return SeverityError
		}
	}

	// 警告级别
	warningErrors := []error{
		translator.ErrTimeout,
		translator.ErrRateLimited,
	}
	for _, warningErr := range warningErrors {
		if errors.Is(err, warningErr) {
			return SeverityWarning
		}
	}

	// 默认为错误级别
	return SeverityError
}

// FormatErrorForUser 格式化错误信息供用户查看
func FormatErrorForUser(err error) string {
	if err == nil {
		return ""
	}

	// 转换错误
	convertedErr := ConvertError(err)

	// 根据错误类型提供友好的消息
	switch {
	case errors.Is(convertedErr, translator.ErrInvalidConfig):
		return "Configuration error: Please check your settings"
	case errors.Is(convertedErr, translator.ErrInvalidAPIKey):
		return "Invalid API key: Please verify your API credentials"
	case errors.Is(convertedErr, translator.ErrTimeout):
		return "Request timed out: The operation took too long to complete"
	case errors.Is(convertedErr, translator.ErrRateLimited):
		return "Rate limit exceeded: Please try again later"
	case errors.Is(convertedErr, translator.ErrChunkTooLarge):
		return "Text chunk too large: Consider using smaller chunks"
	case errors.Is(convertedErr, translator.ErrContextCancelled):
		return "Operation cancelled: The translation was interrupted"
	default:
		// 清理错误消息
		msg := convertedErr.Error()
		// 移除技术细节
		if idx := strings.Index(msg, ":"); idx > 0 && idx < 50 {
			msg = msg[idx+1:]
		}
		return strings.TrimSpace(msg)
	}
}