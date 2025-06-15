package factory

import (
	"fmt"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/deepl"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/deeplx"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/google"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/libretranslate"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/openai"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/raw"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// ProviderFactory 提供商工厂
type ProviderFactory struct {
	registry *providers.Registry
}

// New 创建新的提供商工厂
func New() *ProviderFactory {
	return &ProviderFactory{
		registry: providers.NewRegistry(),
	}
}

// CreateProvider 根据配置创建提供商
func (f *ProviderFactory) CreateProvider(providerType string, modelConfig config.ModelConfig) (translation.TranslationProvider, error) {
	switch providerType {
	case "openai":
		return f.createOpenAIProvider(modelConfig)
	case "deepl":
		return f.createDeepLProvider(modelConfig)
	case "deeplx":
		return f.createDeepLXProvider(modelConfig)
	case "google":
		return f.createGoogleProvider(modelConfig)
	case "libretranslate":
		return f.createLibreTranslateProvider(modelConfig)
	case "raw", "none":
		return f.createRawProvider(modelConfig)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

// createOpenAIProvider 创建 OpenAI 提供商
func (f *ProviderFactory) createOpenAIProvider(modelConfig config.ModelConfig) (translation.TranslationProvider, error) {
	config := openai.ConfigV2{
		BaseConfig: providers.BaseConfig{
			APIKey:      modelConfig.Key,
			APIEndpoint: modelConfig.BaseURL,
			Timeout:     30 * time.Second,
			MaxRetries:  3,
			RetryDelay:  time.Second,
			Headers:     make(map[string]string),
		},
		Model:       modelConfig.ModelID,
		Temperature: float32(modelConfig.Temperature),
		MaxTokens:   modelConfig.MaxOutputTokens,
	}

	// 如果没有设置 BaseURL，使用默认值
	if config.APIEndpoint == "" {
		config.APIEndpoint = "https://api.openai.com/v1"
	}

	provider := openai.NewV2(config)
	return provider, nil
}

// createDeepLProvider 创建 DeepL 提供商
func (f *ProviderFactory) createDeepLProvider(modelConfig config.ModelConfig) (translation.TranslationProvider, error) {
	config := deepl.Config{
		BaseConfig: providers.BaseConfig{
			APIKey:      modelConfig.Key,
			APIEndpoint: modelConfig.BaseURL,
			Timeout:     30 * time.Second,
			MaxRetries:  3,
			RetryDelay:  time.Second,
			Headers:     make(map[string]string),
		},
		UseFreeAPI: false,
	}

	// 如果没有设置 BaseURL，使用默认值
	if config.APIEndpoint == "" {
		config.APIEndpoint = "https://api.deepl.com/v2"
	}

	provider := deepl.New(config)
	return provider, nil
}

// createDeepLXProvider 创建 DeepLX 提供商
func (f *ProviderFactory) createDeepLXProvider(modelConfig config.ModelConfig) (translation.TranslationProvider, error) {
	config := deeplx.Config{
		BaseConfig: providers.BaseConfig{
			APIKey:      modelConfig.Key,
			APIEndpoint: modelConfig.BaseURL,
			Timeout:     30 * time.Second,
			MaxRetries:  3,
			RetryDelay:  time.Second,
			Headers:     make(map[string]string),
		},
	}

	// 如果没有设置 BaseURL，使用默认值
	if config.APIEndpoint == "" {
		config.APIEndpoint = "http://localhost:1188/translate"
	}

	provider := deeplx.New(config)
	return provider, nil
}

// createGoogleProvider 创建 Google Translate 提供商
func (f *ProviderFactory) createGoogleProvider(modelConfig config.ModelConfig) (translation.TranslationProvider, error) {
	config := google.Config{
		BaseConfig: providers.BaseConfig{
			APIKey:      modelConfig.Key,
			APIEndpoint: modelConfig.BaseURL,
			Timeout:     30 * time.Second,
			MaxRetries:  3,
			RetryDelay:  time.Second,
			Headers:     make(map[string]string),
		},
	}

	// 如果没有设置 BaseURL，使用默认值
	if config.APIEndpoint == "" {
		config.APIEndpoint = "https://translation.googleapis.com/language/translate/v2"
	}

	provider := google.New(config)
	return provider, nil
}

// createLibreTranslateProvider 创建 LibreTranslate 提供商
func (f *ProviderFactory) createLibreTranslateProvider(modelConfig config.ModelConfig) (translation.TranslationProvider, error) {
	config := libretranslate.Config{
		BaseConfig: providers.BaseConfig{
			APIKey:      modelConfig.Key,
			APIEndpoint: modelConfig.BaseURL,
			Timeout:     30 * time.Second,
			MaxRetries:  3,
			RetryDelay:  time.Second,
			Headers:     make(map[string]string),
		},
	}

	// 如果没有设置 BaseURL，使用默认值
	if config.APIEndpoint == "" {
		config.APIEndpoint = "https://libretranslate.com"
	}

	provider := libretranslate.New(config)
	return provider, nil
}

// createRawProvider 创建 Raw 提供商（raw 和 none 都使用相同的实现）
func (f *ProviderFactory) createRawProvider(modelConfig config.ModelConfig) (translation.TranslationProvider, error) {
	config := raw.DefaultConfig()
	provider := raw.New(config)
	return provider, nil
}

// GetSupportedProviders 获取支持的提供商列表
func (f *ProviderFactory) GetSupportedProviders() []string {
	return []string{
		"openai",
		"deepl", 
		"deeplx",
		"google",
		"libretranslate",
		"raw",
		"none",
	}
}

// IsLLMProvider 判断是否是 LLM 提供商（需要复杂 prompts）
func IsLLMProvider(providerType string) bool {
	switch providerType {
	case "openai", "anthropic", "mistral", "gemini":
		return true
	case "deepl", "deeplx", "google", "libretranslate", "raw", "none":
		return false
	default:
		return false
	}
}

// IsDirectTranslationProvider 判断是否是直接翻译提供商（不需要复杂 prompts）
func IsDirectTranslationProvider(providerType string) bool {
	return !IsLLMProvider(providerType)
}

// GetProviderCapabilities 获取提供商特性
func GetProviderCapabilities(providerType string) ProviderCapabilities {
	switch providerType {
	case "openai":
		return ProviderCapabilities{
			SupportsPrompts:     true,
			SupportsSystemRole:  true,
			SupportsTemperature: true,
			SupportsMultiStep:   true,
			RequiresAPIKey:      true,
			DefaultModel:        "gpt-3.5-turbo",
		}
	case "deepl":
		return ProviderCapabilities{
			SupportsPrompts:     false,
			SupportsSystemRole:  false,
			SupportsTemperature: false,
			SupportsMultiStep:   false,
			RequiresAPIKey:      true,
			DefaultModel:        "deepl",
		}
	case "deeplx":
		return ProviderCapabilities{
			SupportsPrompts:     false,
			SupportsSystemRole:  false,
			SupportsTemperature: false,
			SupportsMultiStep:   false,
			RequiresAPIKey:      false,
			DefaultModel:        "deeplx",
		}
	case "google":
		return ProviderCapabilities{
			SupportsPrompts:     false,
			SupportsSystemRole:  false,
			SupportsTemperature: false,
			SupportsMultiStep:   false,
			RequiresAPIKey:      true,
			DefaultModel:        "google-translate",
		}
	case "libretranslate":
		return ProviderCapabilities{
			SupportsPrompts:     false,
			SupportsSystemRole:  false,
			SupportsTemperature: false,
			SupportsMultiStep:   false,
			RequiresAPIKey:      false,
			DefaultModel:        "libretranslate",
		}
	case "raw", "none":
		return ProviderCapabilities{
			SupportsPrompts:     false,
			SupportsSystemRole:  false,
			SupportsTemperature: false,
			SupportsMultiStep:   true,
			RequiresAPIKey:      false,
			DefaultModel:        "raw",
		}
	default:
		return ProviderCapabilities{}
	}
}

// ProviderCapabilities 提供商能力定义
type ProviderCapabilities struct {
	SupportsPrompts     bool   `json:"supports_prompts"`     // 是否支持自定义提示词
	SupportsSystemRole  bool   `json:"supports_system_role"` // 是否支持系统角色
	SupportsTemperature bool   `json:"supports_temperature"` // 是否支持温度参数
	SupportsMultiStep   bool   `json:"supports_multi_step"`  // 是否支持多步骤翻译
	RequiresAPIKey      bool   `json:"requires_api_key"`     // 是否需要 API 密钥
	DefaultModel        string `json:"default_model"`        // 默认模型
}

// 全局工厂实例
var DefaultFactory = New()

// CreateProvider 使用默认工厂创建提供商
func CreateProvider(providerType string, modelConfig config.ModelConfig) (translation.TranslationProvider, error) {
	return DefaultFactory.CreateProvider(providerType, modelConfig)
}