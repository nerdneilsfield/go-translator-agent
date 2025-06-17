package translation

import (
	"fmt"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/deepl"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/deeplx"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/google"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/libretranslate"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/ollama"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/openai"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/raw"
	"go.uber.org/zap"
)

// ProviderManager 负责provider创建和管理
type ProviderManager struct {
	config *Config
	logger *zap.Logger
}

// NewProviderManager 创建provider管理器
func NewProviderManager(cfg *Config, logger *zap.Logger) *ProviderManager {
	return &ProviderManager{
		config: cfg,
		logger: logger,
	}
}

// CreateProviders 根据配置创建所有需要的providers
func (pm *ProviderManager) CreateProviders() (map[string]TranslationProvider, error) {
	pm.logger.Info("开始创建翻译服务providers")
	
	// 检查是否配置了步骤集
	if pm.config.ActiveStepSet == "" {
		return nil, fmt.Errorf("no active step set configured")
	}

	// 查找活动的步骤集
	stepSetName := pm.config.ActiveStepSet
	
	// 检查步骤集是否存在
	stepSet, exists := pm.config.StepSets[stepSetName]
	if !exists {
		// 在错误情况下提供调试信息
		availableStepSets := make([]string, 0, len(pm.config.StepSets))
		for name := range pm.config.StepSets {
			availableStepSets = append(availableStepSets, name)
		}
		pm.logger.Error("步骤集未找到",
			zap.String("requested", stepSetName),
			zap.Strings("available", availableStepSets),
			zap.Int("total", len(pm.config.StepSets)))
		return nil, fmt.Errorf("step set '%s' not found. Available step sets: %v", stepSetName, availableStepSets)
	}

	if len(stepSet.Steps) == 0 {
		return nil, fmt.Errorf("step set '%s' has no steps configured", stepSetName)
	}

	// 创建提供商映射
	providerMap := make(map[string]TranslationProvider)
	
	// 为每个步骤创建对应的提供商
	for _, step := range stepSet.Steps {
		
		// 检查特殊步骤选项（raw 或 none）
		if step.ModelName == "raw" || step.ModelName == "none" {
			pm.logger.Info("使用特殊步骤选项",
				zap.String("step", step.Name),
				zap.String("option", step.ModelName))
			
			// 为 raw/none 步骤创建 raw 提供商
			provider := raw.New(raw.DefaultConfig())
			providerMap[step.Provider] = provider
			
			pm.logger.Info("创建 Raw 提供商成功",
				zap.String("step", step.Name),
				zap.String("provider", step.Provider))
			continue
		}
		
		// 检查模型配置是否存在
		modelConfig, exists := pm.config.ModelConfigs[step.ModelName]
		if !exists {
			// 调试信息：显示所有可用的模型配置
			availableModels := make([]string, 0, len(pm.config.ModelConfigs))
			for modelName := range pm.config.ModelConfigs {
				availableModels = append(availableModels, modelName)
			}
			pm.logger.Error("模型配置未找到",
				zap.String("requested", step.ModelName),
				zap.Strings("available", availableModels),
				zap.Int("total", len(pm.config.ModelConfigs)))
			return nil, fmt.Errorf("model '%s' not found in configuration. Available models: %v", step.ModelName, availableModels)
		}

		// 创建提供商
		provider, err := pm.createProvider(step.Provider, modelConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider for step %s: %w", step.Name, err)
		}
		
		// 检查提供商特性
		capabilities := pm.getProviderCapabilities(step.Provider)

		providerMap[step.Provider] = provider
		
		pm.logger.Info("创建提供商成功",
			zap.String("step", step.Name),
			zap.String("provider", step.Provider),
			zap.String("model", step.ModelName),
			zap.Bool("supports_prompts", capabilities.SupportsPrompts),
			zap.Bool("requires_api_key", capabilities.RequiresAPIKey))
	}

	// 调试：输出提供商映射
	providerNames := make([]string, 0, len(providerMap))
	for name := range providerMap {
		providerNames = append(providerNames, name)
	}
	
	pm.logger.Info("创建翻译服务providers完成",
		zap.String("step_set", stepSetName),
		zap.Int("steps_count", len(stepSet.Steps)),
		zap.Strings("providers", providerNames))
		
	return providerMap, nil
}

// createProvider 根据配置创建提供商
func (pm *ProviderManager) createProvider(providerType string, modelConfig config.ModelConfig) (TranslationProvider, error) {
	switch providerType {
	case "openai":
		return pm.createOpenAIProvider(modelConfig)
	case "deepl":
		return pm.createDeepLProvider(modelConfig)
	case "deeplx":
		return pm.createDeepLXProvider(modelConfig)
	case "google":
		return pm.createGoogleProvider(modelConfig)
	case "libretranslate":
		return pm.createLibreTranslateProvider(modelConfig)
	case "ollama":
		return pm.createOllamaProvider(modelConfig)
	case "raw", "none":
		return pm.createRawProvider(modelConfig)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

// createOpenAIProvider 创建 OpenAI 提供商
func (pm *ProviderManager) createOpenAIProvider(modelConfig config.ModelConfig) (TranslationProvider, error) {
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
func (pm *ProviderManager) createDeepLProvider(modelConfig config.ModelConfig) (TranslationProvider, error) {
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
func (pm *ProviderManager) createDeepLXProvider(modelConfig config.ModelConfig) (TranslationProvider, error) {
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
func (pm *ProviderManager) createGoogleProvider(modelConfig config.ModelConfig) (TranslationProvider, error) {
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
func (pm *ProviderManager) createLibreTranslateProvider(modelConfig config.ModelConfig) (TranslationProvider, error) {
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

// createOllamaProvider 创建 Ollama 提供商
func (pm *ProviderManager) createOllamaProvider(modelConfig config.ModelConfig) (TranslationProvider, error) {
	config := ollama.Config{
		BaseConfig: providers.BaseConfig{
			APIKey:      modelConfig.Key, // Ollama通常不需要API密钥，但保留配置
			APIEndpoint: modelConfig.BaseURL,
			Timeout:     60 * time.Second, // Ollama可能需要更长时间
			MaxRetries:  3,
			RetryDelay:  time.Second,
			Headers:     make(map[string]string),
		},
		Model:       modelConfig.ModelID,
		Temperature: float32(modelConfig.Temperature),
		MaxTokens:   modelConfig.MaxOutputTokens,
		Stream:      false, // 翻译时不使用流式输出
	}

	// 如果没有设置 BaseURL，使用默认值
	if config.APIEndpoint == "" {
		config.APIEndpoint = "http://localhost:11434"
	}

	provider := ollama.New(config)
	return provider, nil
}

// createRawProvider 创建 Raw 提供商（raw 和 none 都使用相同的实现）
func (pm *ProviderManager) createRawProvider(modelConfig config.ModelConfig) (TranslationProvider, error) {
	config := raw.DefaultConfig()
	provider := raw.New(config)
	return provider, nil
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

// getProviderCapabilities 获取提供商特性
func (pm *ProviderManager) getProviderCapabilities(providerType string) ProviderCapabilities {
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
	case "ollama":
		return ProviderCapabilities{
			SupportsPrompts:     true,
			SupportsSystemRole:  false, // Ollama一般不支持系统角色，取决于模型
			SupportsTemperature: true,
			SupportsMultiStep:   true,
			RequiresAPIKey:      false, // Ollama本地部署通常不需要API密钥
			DefaultModel:        "llama2",
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