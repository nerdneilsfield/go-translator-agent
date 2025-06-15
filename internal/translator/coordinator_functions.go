package translator

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// createTranslationService 创建翻译服务
func createTranslationService(cfg *config.Config, progressPath string, logger *zap.Logger) (translation.Service, error) {
	// 检查是否配置了步骤集
	if cfg.ActiveStepSet == "" {
		return nil, fmt.Errorf("no active step set configured")
	}

	// 查找活动的步骤集
	var activeStepSet *config.StepSetConfigV2
	logger.Info("looking for step set",
		zap.String("active_step_set", cfg.ActiveStepSet),
		zap.Int("step_sets_v2_count", len(cfg.StepSetsV2)))
	
	for id, stepSet := range cfg.StepSetsV2 {
		logger.Debug("available step set", zap.String("id", id))
		if stepSet.ID == cfg.ActiveStepSet {
			activeStepSet = &stepSet
			break
		}
	}

	if activeStepSet == nil {
		// 尝试从旧格式转换
		logger.Info("step set not found in v2, checking old format")
		if oldStepSet, ok := cfg.StepSets[cfg.ActiveStepSet]; ok {
			logger.Info("found in old format, converting", zap.String("id", oldStepSet.ID))
			v2StepSet := oldStepSet.ToStepSetConfigV2()
			activeStepSet = &v2StepSet
		} else {
			return nil, fmt.Errorf("step set not found: %s", cfg.ActiveStepSet)
		}
	}

	// 创建翻译配置
	translationConfig := &translation.Config{
		SourceLanguage: cfg.SourceLang,
		TargetLanguage: cfg.TargetLang,
		ChunkSize:      cfg.ChunkSize,
		ChunkOverlap:   100, // 默认重叠
		MaxConcurrency: cfg.Concurrency,
		MaxRetries:     cfg.RetryAttempts,
		RetryDelay:     time.Second,
		Timeout:        time.Duration(cfg.RequestTimeout) * time.Second,
		EnableCache:    cfg.UseCache,
		CacheDir:       filepath.Join(progressPath, "cache"),
		Metadata: map[string]interface{}{
			"country":              cfg.Country,
			"fast_mode":            false,
			"fast_mode_threshold":  100,
		},
	}

	// 将步骤集转换为翻译步骤
	for _, step := range activeStepSet.Steps {
		stepConfig := translation.StepConfig{
			Name:        step.Name,
			Model:       step.ModelName,
			Provider:    step.Provider,
			Prompt:      step.Prompt,
			Temperature: float32(step.Temperature),
			MaxTokens:   step.MaxTokens,
			Variables:   step.Variables,
		}
		translationConfig.Steps = append(translationConfig.Steps, stepConfig)
	}

	// 初始化模型客户端
	// 将 zap.Logger 包装为 logger.Logger 接口
	loggerWrapper := &zapLoggerWrapper{logger: logger}
	
	logger.Info("initializing models", 
		zap.Int("model_configs_count", len(cfg.ModelConfigs)))
	
	// Debug: Print all model names
	for modelName := range cfg.ModelConfigs {
		logger.Debug("config has model", zap.String("name", modelName))
	}
	
	models, err := translator.InitModels(cfg, loggerWrapper)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize models: %w", err)
	}

	// 查找默认的 LLM 客户端
	var llmClient translator.LLMClient
	logger.Info("looking for LLM clients",
		zap.Int("models_count", len(models)),
		zap.Int("steps_count", len(activeStepSet.Steps)))
	
	for modelName := range models {
		logger.Info("available model", zap.String("name", modelName))
	}
	
	for _, step := range activeStepSet.Steps {
		logger.Info("checking step", 
			zap.String("model_name", step.ModelName),
			zap.String("provider", step.Provider))
		if client, ok := models[step.ModelName]; ok {
			llmClient = client
			logger.Info("found LLM client", zap.String("model", step.ModelName))
			break
		}
	}

	if llmClient == nil {
		return nil, fmt.Errorf("no LLM client available for the configured models")
	}

	// 创建翻译服务
	service, err := translation.New(
		translationConfig,
		translation.WithLLMClient(&translationLLMClientAdapter{llmClient}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create translation service: %w", err)
	}

	return service, nil
}

// translationLLMClientAdapter 适配器，将 translator.LLMClient 转换为 translation.LLMClient
type translationLLMClientAdapter struct {
	client translator.LLMClient
}

func (a *translationLLMClientAdapter) Complete(ctx context.Context, req *translation.CompletionRequest) (*translation.CompletionResponse, error) {
	// 调用底层客户端
	content, promptTokens, completionTokens, err := a.client.Complete(
		req.Prompt,
		req.MaxTokens,
		float64(req.Temperature),
	)
	if err != nil {
		return nil, err
	}

	return &translation.CompletionResponse{
		Text:      content,
		TokensIn:  promptTokens,
		TokensOut: completionTokens,
	}, nil
}

func (a *translationLLMClientAdapter) Chat(ctx context.Context, req *translation.ChatRequest) (*translation.ChatResponse, error) {
	// 将聊天请求转换为完成请求
	prompt := ""
	for _, msg := range req.Messages {
		prompt += fmt.Sprintf("%s: %s\n", msg.Role, msg.Content)
	}

	content, promptTokens, completionTokens, err := a.client.Complete(
		prompt,
		req.MaxTokens,
		float64(req.Temperature),
	)
	if err != nil {
		return nil, err
	}

	return &translation.ChatResponse{
		Message: translation.ChatMessage{
			Role:    "assistant",
			Content: content,
		},
		TokensIn:  promptTokens,
		TokensOut: completionTokens,
	}, nil
}

func (a *translationLLMClientAdapter) GetModel() string {
	return a.client.Name()
}

func (a *translationLLMClientAdapter) HealthCheck(ctx context.Context) error {
	// 简单的健康检查
	_, _, _, err := a.client.Complete("ping", 10, 0.0)
	return err
}

// zapLoggerWrapper 包装 zap.Logger 以实现 logger.Logger 接口
type zapLoggerWrapper struct {
	logger *zap.Logger
}

func (w *zapLoggerWrapper) Info(msg string, fields ...zapcore.Field) {
	w.logger.Info(msg, fields...)
}

func (w *zapLoggerWrapper) Debug(msg string, fields ...zapcore.Field) {
	w.logger.Debug(msg, fields...)
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