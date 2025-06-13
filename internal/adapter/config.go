package adapter

import (
	"fmt"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// ConvertConfig 将 Viper 配置转换为 translation.Config
func ConvertConfig(cfg *config.Config) (*translation.Config, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	// 创建基础配置
	translationConfig := &translation.Config{
		SourceLanguage: cfg.SourceLang,
		TargetLanguage: cfg.TargetLang,
		ChunkSize:      cfg.MaxTokensPerChunk,
		ChunkOverlap:   100, // 默认重叠
		MaxConcurrency: cfg.Concurrency,
		MaxRetries:     cfg.MaxRetries,
		RetryDelay:     5 * time.Second, // 默认重试延迟
		Timeout:        time.Duration(cfg.TranslationTimeout) * time.Second,
	}

	// 转换翻译步骤
	if cfg.ActiveStepSet != "" {
		// 如果指定了步骤集但没有找到，返回错误
		if len(cfg.StepSets) == 0 {
			return nil, fmt.Errorf("no step sets defined")
		}
		
		stepSet, exists := cfg.StepSets[cfg.ActiveStepSet]
		if !exists {
			return nil, fmt.Errorf("step set %s not found", cfg.ActiveStepSet)
		}

		translationConfig.Steps = convertStepSet(stepSet, cfg)
	} else {
		// 使用默认三步配置
		translationConfig.Steps = createDefaultSteps(cfg)
	}

	return translationConfig, nil
}

// convertStepSet 转换步骤集配置
func convertStepSet(stepSet config.StepSetConfig, cfg *config.Config) []translation.StepConfig {
	// 从 StepSetConfig 创建三个步骤
	steps := make([]translation.StepConfig, 0, 3)

	// 初始翻译步骤
	if stepSet.InitialTranslation.ModelName != "" && stepSet.InitialTranslation.ModelName != "none" {
		modelConfig := getModelConfig(stepSet.InitialTranslation.ModelName, cfg)
		provider := determineProviderFromModel(stepSet.InitialTranslation.ModelName, modelConfig)
		
		steps = append(steps, translation.StepConfig{
			Name:        "initial_translation",
			Provider:    provider,
			Model:       stepSet.InitialTranslation.ModelName,
			Temperature: float32(stepSet.InitialTranslation.Temperature),
			MaxTokens:   4096,
			Timeout:     time.Duration(cfg.TranslationTimeout) * time.Second,
			Prompt:      getInitialTranslationPrompt(cfg),
			SystemRole:  "You are a professional translator.",
			Variables: map[string]string{
				"source": cfg.SourceLang,
				"target": cfg.TargetLang,
			},
		})
	}

	// 反思步骤
	if stepSet.Reflection.ModelName != "" && stepSet.Reflection.ModelName != "none" {
		modelConfig := getModelConfig(stepSet.Reflection.ModelName, cfg)
		provider := determineProviderFromModel(stepSet.Reflection.ModelName, modelConfig)
		
		steps = append(steps, translation.StepConfig{
			Name:        "reflection",
			Provider:    provider,
			Model:       stepSet.Reflection.ModelName,
			Temperature: float32(stepSet.Reflection.Temperature),
			MaxTokens:   2048,
			Timeout:     time.Duration(cfg.TranslationTimeout) * time.Second,
			Prompt:      getReflectionPrompt(cfg),
			SystemRole:  "You are a translation quality reviewer.",
			Variables: map[string]string{
				"source": cfg.SourceLang,
				"target": cfg.TargetLang,
			},
		})
	}

	// 改进步骤
	if stepSet.Improvement.ModelName != "" && stepSet.Improvement.ModelName != "none" {
		modelConfig := getModelConfig(stepSet.Improvement.ModelName, cfg)
		provider := determineProviderFromModel(stepSet.Improvement.ModelName, modelConfig)
		
		steps = append(steps, translation.StepConfig{
			Name:        "improvement",
			Provider:    provider,
			Model:       stepSet.Improvement.ModelName,
			Temperature: float32(stepSet.Improvement.Temperature),
			MaxTokens:   4096,
			Timeout:     time.Duration(cfg.TranslationTimeout) * time.Second,
			Prompt:      getImprovementPrompt(cfg),
			SystemRole:  "You are a professional translator focusing on quality improvement.",
			Variables: map[string]string{
				"source": cfg.SourceLang,
				"target": cfg.TargetLang,
			},
		})
	}

	return steps
}

// createDefaultSteps 创建默认的三步翻译配置
func createDefaultSteps(cfg *config.Config) []translation.StepConfig {
	defaultModel := cfg.DefaultModelName
	if defaultModel == "" {
		defaultModel = "gpt-3.5-turbo"
	}

	modelConfig := getModelConfig(defaultModel, cfg)
	provider := determineProviderFromModel(defaultModel, modelConfig)

	basePromptVars := map[string]string{
		"source": cfg.SourceLang,
		"target": cfg.TargetLang,
	}

	return []translation.StepConfig{
		{
			Name:        "initial_translation",
			Provider:    provider,
			Model:       defaultModel,
			Temperature: 0.3,
			MaxTokens:   4096,
			Timeout:     time.Duration(cfg.TranslationTimeout) * time.Second,
			Prompt:      getInitialTranslationPrompt(cfg),
			SystemRole:  "You are a professional translator.",
			Variables:   basePromptVars,
		},
		{
			Name:        "reflection",
			Provider:    provider,
			Model:       defaultModel,
			Temperature: 0.1,
			MaxTokens:   2048,
			Timeout:     time.Duration(cfg.TranslationTimeout) * time.Second,
			Prompt:      getReflectionPrompt(cfg),
			SystemRole:  "You are a translation quality reviewer.",
			Variables:   basePromptVars,
		},
		{
			Name:        "improvement",
			Provider:    provider,
			Model:       defaultModel,
			Temperature: 0.3,
			MaxTokens:   4096,
			Timeout:     time.Duration(cfg.TranslationTimeout) * time.Second,
			Prompt:      getImprovementPrompt(cfg),
			SystemRole:  "You are a professional translator focusing on quality improvement.",
			Variables:   basePromptVars,
		},
	}
}


// determineProviderFromModel 从模型配置确定提供商
func determineProviderFromModel(model string, modelConfig *config.ModelConfig) string {
	// 从 API 类型推断提供商
	if modelConfig != nil && modelConfig.APIType != "" {
		switch modelConfig.APIType {
		case "openai", "openai-reasoning":
			return "openai"
		case "anthropic":
			return "anthropic"
		case "mistral":
			return "mistral"
		}
	}
	return inferProviderFromModel(model)
}

// inferProviderFromModel 从模型名称推断提供商
func inferProviderFromModel(model string) string {
	model = strings.ToLower(model)
	
	switch {
	case strings.Contains(model, "gpt") || strings.Contains(model, "davinci") || strings.Contains(model, "turbo"):
		return "openai"
	case strings.Contains(model, "claude"):
		return "anthropic"
	case strings.Contains(model, "gemini") || strings.Contains(model, "bard"):
		return "google"
	case strings.Contains(model, "deepl"):
		return "deepl"
	case strings.Contains(model, "llama") || strings.Contains(model, "mistral"):
		return "ollama"
	default:
		return "openai" // 默认使用 OpenAI
	}
}

// getModelConfig 获取模型配置
func getModelConfig(model string, cfg *config.Config) *config.ModelConfig {
	if cfg.ModelConfigs == nil {
		return nil
	}
	
	modelConfig, exists := cfg.ModelConfigs[model]
	if !exists {
		return nil
	}
	
	return &modelConfig
}

// getInitialTranslationPrompt 获取初始翻译提示词
func getInitialTranslationPrompt(cfg *config.Config) string {
	return fmt.Sprintf(`Translate the following %s text to %s. 
Maintain the original meaning, tone, and style as much as possible.

Text to translate:
{{text}}`, cfg.SourceLang, cfg.TargetLang)
}

// getReflectionPrompt 获取反思提示词
func getReflectionPrompt(cfg *config.Config) string {
	return fmt.Sprintf(`Review the following translation from %s to %s.
Identify any issues with accuracy, fluency, cultural appropriateness, or style.

Original text:
{{original_text}}

Translation:
{{translation}}

Please provide specific feedback on what could be improved.`, cfg.SourceLang, cfg.TargetLang)
}

// getImprovementPrompt 获取改进提示词
func getImprovementPrompt(cfg *config.Config) string {
	return fmt.Sprintf(`Based on the feedback provided, improve the following translation from %s to %s.

Original text:
{{original_text}}

Current translation:
{{translation}}

Feedback:
{{feedback}}

Please provide an improved translation that addresses the feedback.`, cfg.SourceLang, cfg.TargetLang)
}

// mergeVariables 合并变量
func mergeVariables(base, override map[string]string) map[string]string {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}
	
	result := make(map[string]string)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		result[k] = v
	}
	
	return result
}

// ConvertLanguageCode 转换语言代码格式
func ConvertLanguageCode(lang string) string {
	// 保留原始大小写的语言代码
	originalLang := strings.TrimSpace(lang)
	
	// 标准化语言名称（小写）用于查找
	langLower := strings.ToLower(originalLang)
	
	// 常见语言映射
	languageMap := map[string]string{
		"english":    "en",
		"chinese":    "zh",
		"japanese":   "ja",
		"korean":     "ko",
		"spanish":    "es",
		"french":     "fr",
		"german":     "de",
		"russian":    "ru",
		"italian":    "it",
		"portuguese": "pt",
		"dutch":      "nl",
		"polish":     "pl",
		"arabic":     "ar",
		"hindi":      "hi",
		"vietnamese": "vi",
		"thai":       "th",
		"turkish":    "tr",
		"swedish":    "sv",
		"danish":     "da",
		"norwegian":  "no",
		"finnish":    "fi",
	}
	
	// 检查是否需要转换
	if code, exists := languageMap[langLower]; exists {
		return code
	}
	
	// 已经是代码格式，保留原始大小写
	if len(originalLang) == 2 || (len(originalLang) == 5 && originalLang[2] == '-') { // ISO 639-1 或 带地区的格式如 zh-CN
		return originalLang
	}
	
	// 返回原值
	return originalLang
}