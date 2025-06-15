package translator

import (
	"context"
	"fmt"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
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
	stepSetName := cfg.ActiveStepSet
	
	// 检查步骤集是否存在
	stepSet, exists := cfg.StepSets[stepSetName]
	if !exists {
		return nil, fmt.Errorf("step set '%s' not found", stepSetName)
	}

	if len(stepSet.Steps) == 0 {
		return nil, fmt.Errorf("step set '%s' has no steps configured", stepSetName)
	}

	// 使用第一个步骤的模型配置
	firstStep := stepSet.Steps[0]
	modelConfig, exists := cfg.ModelConfigs[firstStep.ModelName]
	if !exists {
		return nil, fmt.Errorf("model '%s' not found in configuration", firstStep.ModelName)
	}
	
	// 创建 Zap 日志包装器
	loggerWrapper := &zapLoggerWrapper{logger: logger}

	// 创建简单的 LLM 客户端
	llmClient := &simpleLLMClient{
		config: &modelConfig,
		logger: loggerWrapper,
	}

	// 转换步骤配置
	translationSteps := make([]translation.StepConfig, len(stepSet.Steps))
	for i, step := range stepSet.Steps {
		translationSteps[i] = translation.StepConfig{
			Name:        step.Name,
			Provider:    step.Provider,
			Model:       step.ModelName, // 注意：translation包使用Model而不是ModelName
			Temperature: float32(step.Temperature),
			MaxTokens:   step.MaxTokens,
			Prompt:      step.Prompt,
			Variables:   step.Variables,
			SystemRole:  step.SystemRole,
		}
	}

	// 创建翻译服务
	logger.Info("creating translation service with step set", 
		zap.String("step_set", stepSetName),
		zap.Int("steps_count", len(translationSteps)))
	
	translationConfig := &translation.Config{
		SourceLanguage: cfg.SourceLang,
		TargetLanguage: cfg.TargetLang,
		ChunkSize:      cfg.ChunkSize,
		ChunkOverlap:   100, // 默认重叠大小
		MaxConcurrency: cfg.Concurrency,
		EnableCache:    cfg.UseCache,
		CacheDir:       cfg.CacheDir,
		Steps:          translationSteps, // 添加步骤配置
	}

	return translation.New(translationConfig, translation.WithLLMClient(llmClient))
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

// simpleLLMClient 简单的 LLM 客户端实现
type simpleLLMClient struct {
	config *config.ModelConfig
	logger logger.Logger
}

func (c *simpleLLMClient) Complete(ctx context.Context, req *translation.CompletionRequest) (*translation.CompletionResponse, error) {
	// 实现文本补全
	return &translation.CompletionResponse{
		Text: "Completion not implemented",
	}, fmt.Errorf("completion not implemented")
}

func (c *simpleLLMClient) Chat(ctx context.Context, req *translation.ChatRequest) (*translation.ChatResponse, error) {
	// 实现对话接口
	return &translation.ChatResponse{
		Message: translation.ChatMessage{
			Role:    "assistant",
			Content: "Chat not implemented",
		},
	}, fmt.Errorf("chat not implemented")
}

func (c *simpleLLMClient) GetModel() string {
	if c.config != nil {
		return c.config.ModelID
	}
	return "unknown"
}

func (c *simpleLLMClient) HealthCheck(ctx context.Context) error {
	// 简单的健康检查
	return nil
}