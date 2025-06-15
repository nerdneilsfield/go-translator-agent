package translator

import (
	"fmt"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/factory"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// createTranslationService 创建翻译服务
func createTranslationService(cfg *config.Config, progressPath string, logger *zap.Logger) (translation.Service, error) {
	logger.Info("开始创建翻译服务")
	// 检查是否配置了步骤集
	if cfg.ActiveStepSet == "" {
		return nil, fmt.Errorf("no active step set configured")
	}

	// 查找活动的步骤集
	stepSetName := cfg.ActiveStepSet
	
	// 检查步骤集是否存在
	stepSet, exists := cfg.StepSets[stepSetName]
	if !exists {
		// 在错误情况下提供调试信息
		availableStepSets := make([]string, 0, len(cfg.StepSets))
		for name := range cfg.StepSets {
			availableStepSets = append(availableStepSets, name)
		}
		logger.Error("步骤集未找到",
			zap.String("requested", stepSetName),
			zap.Strings("available", availableStepSets),
			zap.Int("total", len(cfg.StepSets)))
		return nil, fmt.Errorf("step set '%s' not found. Available step sets: %v", stepSetName, availableStepSets)
	}

	if len(stepSet.Steps) == 0 {
		return nil, fmt.Errorf("step set '%s' has no steps configured", stepSetName)
	}

	// 创建提供商映射
	providerMap := make(map[string]translation.TranslationProvider)
	providerFactory := factory.New()
	
	// 为每个步骤创建对应的提供商
	for _, step := range stepSet.Steps {
		
		// 检查特殊步骤选项（raw 或 none）
		if step.ModelName == "raw" || step.ModelName == "none" {
			logger.Info("使用特殊步骤选项",
				zap.String("step", step.Name),
				zap.String("option", step.ModelName))
			
			// 为 raw/none 步骤创建 raw 提供商（使用虚拟模型配置）
			virtualModelConfig := config.ModelConfig{
				Name:     "raw",
				ModelID:  "raw",
				APIType:  "raw",
				IsLLM:    false,
			}
			
			provider, err := providerFactory.CreateProvider("raw", virtualModelConfig)
			if err != nil {
				logger.Error("创建 Raw 提供商失败",
					zap.String("step", step.Name),
					zap.Error(err))
				return nil, fmt.Errorf("failed to create raw provider for step %s: %w", step.Name, err)
			}
			
			providerMap[step.Provider] = provider
			
			logger.Info("创建 Raw 提供商成功",
				zap.String("step", step.Name),
				zap.String("provider", step.Provider))
			continue
		}
		
		// 检查模型配置是否存在
		modelConfig, exists := cfg.ModelConfigs[step.ModelName]
		if !exists {
			// 调试信息：显示所有可用的模型配置
			availableModels := make([]string, 0, len(cfg.ModelConfigs))
			for modelName := range cfg.ModelConfigs {
				availableModels = append(availableModels, modelName)
			}
			logger.Error("模型配置未找到",
				zap.String("requested", step.ModelName),
				zap.Strings("available", availableModels),
				zap.Int("total", len(cfg.ModelConfigs)))
			return nil, fmt.Errorf("model '%s' not found in configuration. Available models: %v", step.ModelName, availableModels)
		}

		// 检查提供商特性
		capabilities := factory.GetProviderCapabilities(step.Provider)
		
		// 使用工厂创建提供商
		provider, err := providerFactory.CreateProvider(step.Provider, modelConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider for step %s: %w", step.Name, err)
		}

		providerMap[step.Provider] = provider
		
		logger.Info("创建提供商成功",
			zap.String("step", step.Name),
			zap.String("provider", step.Provider),
			zap.String("model", step.ModelName),
			zap.Bool("supports_prompts", capabilities.SupportsPrompts),
			zap.Bool("requires_api_key", capabilities.RequiresAPIKey))
	}

	// 转换步骤配置
	translationSteps := make([]translation.StepConfig, len(stepSet.Steps))
	for i, step := range stepSet.Steps {
		// 获取模型的 IsLLM 信息
		var isLLM bool
		if step.ModelName == "raw" || step.ModelName == "none" {
			isLLM = false // 特殊选项不是 LLM
		} else if modelConfig, exists := cfg.ModelConfigs[step.ModelName]; exists {
			isLLM = modelConfig.IsLLM
		}
		
		translationSteps[i] = translation.StepConfig{
			Name:            step.Name,
			Provider:        step.Provider,
			Model:           step.ModelName,
			Temperature:     float32(step.Temperature),
			MaxTokens:       step.MaxTokens,
			AdditionalNotes: step.AdditionalNotes,
			Variables:       make(map[string]string), // 初始化空的 variables
			IsLLM:           isLLM,
		}
	}

	// 调试：输出提供商映射
	providerNames := make([]string, 0, len(providerMap))
	for name := range providerMap {
		providerNames = append(providerNames, name)
	}
	
	// 创建翻译服务
	logger.Info("创建翻译服务",
		zap.String("step_set", stepSetName),
		zap.Int("steps_count", len(translationSteps)),
		zap.Strings("providers", providerNames))
	
	translationConfig := &translation.Config{
		SourceLanguage: cfg.SourceLang,
		TargetLanguage: cfg.TargetLang,
		ChunkSize:      cfg.ChunkSize,
		ChunkOverlap:   100, // 默认重叠大小
		MaxConcurrency: cfg.Concurrency,
		EnableCache:    cfg.UseCache,
		CacheDir:       cfg.CacheDir,
		Steps:          translationSteps,
	}

	return translation.New(translationConfig, translation.WithProviders(providerMap))
}


// zapLoggerWrapper 包装 Zap logger 以符合翻译服务的接口
type zapLoggerWrapper struct {
	logger *zap.Logger
}

func (w *zapLoggerWrapper) Debug(msg string, fields ...zapcore.Field) {
	w.logger.Debug(msg, fields...)
}

func (w *zapLoggerWrapper) Info(msg string, fields ...zapcore.Field) {
	w.logger.Info(msg, fields...)
}

func (w *zapLoggerWrapper) Warn(msg string, fields ...zapcore.Field) {
	w.logger.Warn(msg, fields...)
}

func (w *zapLoggerWrapper) Error(msg string, fields ...zapcore.Field) {
	w.logger.Error(msg, fields...)
}

func (w *zapLoggerWrapper) Fatal(msg string, fields ...zapcore.Field) {
	w.logger.Fatal(msg, fields...)
}

func (w *zapLoggerWrapper) With(fields ...zapcore.Field) logger.Logger {
	return &zapLoggerWrapper{logger: w.logger.With(fields...)}
}

