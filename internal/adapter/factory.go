package adapter

import (
	"fmt"
	"os"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/deepl"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/deeplx"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/google"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/libretranslate"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/openai"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// CreateProviders 根据配置创建提供商
func CreateProviders(cfg *config.Config) (map[string]translation.TranslationProvider, error) {
	providers := make(map[string]translation.TranslationProvider)

	// 收集所有使用的提供商
	usedProviders := collectUsedProviders(cfg)

	// 创建每个使用的提供商
	for providerName := range usedProviders {
		provider, err := createProvider(providerName, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider %s: %w", providerName, err)
		}
		if provider != nil {
			providers[providerName] = provider
		}
	}

	return providers, nil
}

// CreateLLMClient 创建 LLM 客户端（用于向后兼容）
func CreateLLMClient(cfg *config.Config) translation.LLMClient {
	// 如果使用传统配置，创建默认的 OpenAI 客户端
	if cfg.DefaultModelName != "" {
		provider, err := createOpenAIProvider(cfg)
		if err == nil {
			// 尝试转换为 LLMClient
			if llmClient, ok := provider.(translation.LLMClient); ok {
				return llmClient
			}
		}
	}
	return nil
}

// collectUsedProviders 收集配置中使用的所有提供商
func collectUsedProviders(cfg *config.Config) map[string]bool {
	providers := make(map[string]bool)

	// 从步骤集中收集
	if cfg.ActiveStepSet != "" && len(cfg.StepSets) > 0 {
		if stepSet, exists := cfg.StepSets[cfg.ActiveStepSet]; exists {
			// 检查三个步骤的模型
			stepModels := []string{
				stepSet.InitialTranslation.ModelName,
				stepSet.Reflection.ModelName,
				stepSet.Improvement.ModelName,
			}
			
			for _, model := range stepModels {
				if model != "" && model != "none" {
					// 从模型配置获取提供商
					if modelConfig, ok := cfg.ModelConfigs[model]; ok && modelConfig.APIType != "" {
						switch modelConfig.APIType {
						case "openai", "openai-reasoning":
							providers["openai"] = true
						case "anthropic":
							providers["anthropic"] = true
						case "mistral":
							providers["mistral"] = true
						}
					} else {
						// 从模型名称推断
						provider := inferProviderFromModel(model)
						providers[provider] = true
					}
				}
			}
		}
	} else {
		// 从默认模型推断
		provider := inferProviderFromModel(cfg.DefaultModelName)
		providers[provider] = true
	}

	return providers
}

// createProvider 创建特定的提供商
func createProvider(name string, cfg *config.Config) (translation.TranslationProvider, error) {
	switch strings.ToLower(name) {
	case "openai":
		return createOpenAIProvider(cfg)
	case "deepl":
		return createDeepLProvider(cfg)
	case "deeplx":
		return createDeepLXProvider(cfg)
	case "google":
		return createGoogleProvider(cfg)
	case "libretranslate":
		return createLibreTranslateProvider(cfg)
	case "anthropic":
		// TODO: 实现 Anthropic 提供商
		return nil, fmt.Errorf("anthropic provider not yet implemented")
	case "ollama":
		// TODO: 实现 Ollama 提供商
		return nil, fmt.Errorf("ollama provider not yet implemented")
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}

// createOpenAIProvider 创建 OpenAI 提供商
func createOpenAIProvider(cfg *config.Config) (translation.TranslationProvider, error) {
	// 获取 API 密钥
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		// 检查是否有配置了 OpenAI 的模型
		for _, modelConfig := range cfg.ModelConfigs {
			if modelConfig.APIType == "openai" && modelConfig.Key != "" {
				apiKey = modelConfig.Key
				break
			}
		}
	}
	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key not found")
	}

	// 决定使用哪个版本
	useV2 := false
	if envV2 := os.Getenv("OPENAI_USE_V2"); envV2 == "true" {
		useV2 = true
	}

	// 查找 OpenAI 模型配置以获取 base URL
	var baseURL string
	for _, modelConfig := range cfg.ModelConfigs {
		if modelConfig.APIType == "openai" && modelConfig.BaseURL != "" {
			baseURL = modelConfig.BaseURL
			break
		}
	}

	if useV2 {
		// 使用官方 SDK 版本
		config := openai.DefaultConfigV2()
		config.APIKey = apiKey
		
		// 设置基础 URL
		if baseURL != "" {
			config.APIEndpoint = baseURL
		}
		
		// 设置模型
		if cfg.DefaultModelName != "" {
			config.Model = cfg.DefaultModelName
		}
		
		return openai.NewV2(config), nil
	} else {
		// 使用自定义实现版本
		config := openai.DefaultConfig()
		config.APIKey = apiKey
		
		if baseURL != "" {
			config.APIEndpoint = baseURL
		}
		
		if cfg.DefaultModelName != "" {
			config.Model = cfg.DefaultModelName
		}
		
		return openai.New(config), nil
	}
}

// createDeepLProvider 创建 DeepL 提供商
func createDeepLProvider(cfg *config.Config) (translation.TranslationProvider, error) {
	apiKey := os.Getenv("DEEPL_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("DeepL API key not found")
	}

	deeplConfig := deepl.DefaultConfig()
	deeplConfig.APIKey = apiKey
	
	// 检查是否使用免费 API
	if os.Getenv("DEEPL_FREE_API") == "true" {
		deeplConfig.UseFreeAPI = true
	}
	
	return deepl.New(deeplConfig), nil
}

// createDeepLXProvider 创建 DeepLX 提供商
func createDeepLXProvider(cfg *config.Config) (translation.TranslationProvider, error) {
	deeplxConfig := deeplx.DefaultConfig()
	
	// 设置端点
	if endpoint := os.Getenv("DEEPLX_ENDPOINT"); endpoint != "" {
		deeplxConfig.APIEndpoint = endpoint
	}
	
	// 设置访问令牌
	if token := os.Getenv("DEEPLX_TOKEN"); token != "" {
		deeplxConfig.AccessToken = token
	}
	
	return deeplx.New(deeplxConfig), nil
}

// createGoogleProvider 创建 Google 翻译提供商
func createGoogleProvider(cfg *config.Config) (translation.TranslationProvider, error) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("Google API key not found")
	}

	googleConfig := google.DefaultConfig()
	googleConfig.APIKey = apiKey
	
	return google.New(googleConfig), nil
}

// createLibreTranslateProvider 创建 LibreTranslate 提供商
func createLibreTranslateProvider(cfg *config.Config) (translation.TranslationProvider, error) {
	libreConfig := libretranslate.DefaultConfig()
	
	// 设置端点
	if endpoint := os.Getenv("LIBRETRANSLATE_ENDPOINT"); endpoint != "" {
		libreConfig.APIEndpoint = endpoint
	}
	
	// 设置 API 密钥
	if apiKey := os.Getenv("LIBRETRANSLATE_API_KEY"); apiKey != "" {
		libreConfig.APIKey = apiKey
		libreConfig.RequiresAPIKey = true
	}
	
	return libretranslate.New(libreConfig), nil
}

// ProviderFactory 提供商工厂
type ProviderFactory struct {
	config *config.Config
}

// NewProviderFactory 创建提供商工厂
func NewProviderFactory(cfg *config.Config) *ProviderFactory {
	return &ProviderFactory{
		config: cfg,
	}
}

// CreateProvider 创建指定的提供商
func (f *ProviderFactory) CreateProvider(name string) (translation.TranslationProvider, error) {
	return createProvider(name, f.config)
}

// CreateAllProviders 创建所有配置的提供商
func (f *ProviderFactory) CreateAllProviders() (map[string]translation.TranslationProvider, error) {
	return CreateProviders(f.config)
}

// GetAvailableProviders 获取可用的提供商列表
func (f *ProviderFactory) GetAvailableProviders() []string {
	return []string{
		"openai",
		"deepl",
		"deeplx",
		"google",
		"libretranslate",
		// "anthropic",  // TODO
		// "ollama",     // TODO
	}
}