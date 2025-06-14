package translator

import "errors"

// Common errors
var (
	// ErrInvalidConfig 配置无效错误
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrInvalidAPIKey API密钥无效错误
	ErrInvalidAPIKey = errors.New("invalid API key")

	// ErrTimeout 超时错误
	ErrTimeout = errors.New("operation timeout")

	// ErrRateLimited 速率限制错误
	ErrRateLimited = errors.New("rate limit exceeded")

	// ErrChunkTooLarge 文本块过大错误
	ErrChunkTooLarge = errors.New("text chunk too large")

	// ErrContextCancelled 上下文取消错误
	ErrContextCancelled = errors.New("context cancelled")
)
