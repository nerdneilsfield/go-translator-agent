package providers

import (
	"context"
	"time"
)

// BaseConfig 基础配置
type BaseConfig struct {
	// API配置
	APIKey      string `json:"api_key,omitempty"`
	APIEndpoint string `json:"api_endpoint,omitempty"`

	// 超时和重试
	Timeout    time.Duration `json:"timeout"`
	MaxRetries int           `json:"max_retries"`
	RetryDelay time.Duration `json:"retry_delay"`

	// 代理设置
	ProxyURL string `json:"proxy_url,omitempty"`

	// 自定义头部
	Headers map[string]string `json:"headers,omitempty"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() BaseConfig {
	return BaseConfig{
		Timeout:    5 * time.Minute, // 增加到5分钟，支持长时间的LLM请求
		MaxRetries: 3,
		RetryDelay: time.Second,
		Headers:    make(map[string]string),
	}
}

// TranslationProvider 提供商基础接口
type TranslationProvider interface {
	// Translate 执行翻译
	Translate(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error)

	// GetName 获取提供商名称
	GetName() string

	// SupportsSteps 是否支持多步骤翻译
	SupportsSteps() bool
}

// Provider 提供商接口（扩展 TranslationProvider）
type Provider interface {
	TranslationProvider

	// Configure 配置提供商
	Configure(config interface{}) error

	// GetCapabilities 获取提供商能力
	GetCapabilities() Capabilities

	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error
}

// Capabilities 提供商能力
type Capabilities struct {
	// 支持的语言
	SupportedLanguages []Language `json:"supported_languages"`

	// 最大文本长度
	MaxTextLength int `json:"max_text_length"`

	// 是否支持批量翻译
	SupportsBatch bool `json:"supports_batch"`

	// 是否支持格式保留
	SupportsFormatting bool `json:"supports_formatting"`

	// 是否需要API密钥
	RequiresAPIKey bool `json:"requires_api_key"`

	// 速率限制
	RateLimit *RateLimit `json:"rate_limit,omitempty"`
}

// Language 语言信息
type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// RateLimit 速率限制
type RateLimit struct {
	RequestsPerMinute int `json:"requests_per_minute"`
	CharactersPerDay  int `json:"characters_per_day"`
}

// Error 提供商错误
type Error struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

// IsRetryable 判断错误是否可重试
func (e *Error) IsRetryable() bool {
	switch e.Code {
	case "rate_limit", "timeout", "server_error":
		return true
	default:
		return false
	}
}

// NewError 创建提供商错误
func NewError(code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// NewErrorWithDetails 创建带详情的错误
func NewErrorWithDetails(code, message string, details map[string]interface{}) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Details: details,
	}
}

// ProviderRequest 提供商请求
type ProviderRequest struct {
	Text           string                 `json:"text"`
	SourceLanguage string                 `json:"source_language,omitempty"`
	TargetLanguage string                 `json:"target_language,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// ProviderResponse 提供商响应
type ProviderResponse struct {
	Text         string                 `json:"text"`
	SourceLang   string                 `json:"source_lang,omitempty"`
	TargetLang   string                 `json:"target_lang,omitempty"`
	TokensIn     int                    `json:"tokens_in,omitempty"`
	TokensOut    int                    `json:"tokens_out,omitempty"`
	Cost         float64                `json:"cost,omitempty"`
	CostCurrency string                 `json:"cost_currency,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}
