package adapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// ThreeStepTranslatorAdapter 适配器，将新的三步翻译器适配到旧接口
type ThreeStepTranslatorAdapter struct {
	translator      *translation.ThreeStepTranslator
	config          *config.Config
	logger          *logger.Logger
	progressTracker *translator.TranslationProgressTracker
	ctx             context.Context
	cancel          context.CancelFunc
}

// NewThreeStepTranslatorAdapter 创建三步翻译器适配器
func NewThreeStepTranslatorAdapter(cfg *config.Config) (*ThreeStepTranslatorAdapter, error) {
	// 创建 logger
	logger := logger.GetLogger()
	
	// 创建翻译配置
	translationConfig := &translation.Config{
		SourceLanguage: cfg.SourceLang,
		TargetLanguage: cfg.TargetLang,
		ChunkSize:      cfg.ChunkSize,
		ChunkOverlap:   cfg.OverlapSize,
		MaxConcurrency: cfg.MaxConcurrency,
		MaxRetries:     cfg.MaxRetries,
		RetryDelay:     cfg.RetryDelay,
		Timeout:        cfg.Timeout,
		EnableCache:    cfg.CacheEnabled,
		CacheDir:       cfg.CacheDir,
		Metadata: map[string]interface{}{
			"country": cfg.Country,
			"fast_mode": false, // 可以从配置中读取
			"fast_mode_threshold": 100,
		},
	}
	
	// 创建提供者映射
	providers := make(map[string]translation.Provider)
	
	// 根据步骤集配置创建提供者
	stepSet, exists := cfg.StepSets[cfg.ActiveStepSet]
	if !exists {
		return nil, fmt.Errorf("step set not found: %s", cfg.ActiveStepSet)
	}
	
	// 收集需要的模型
	modelNames := []string{
		stepSet.InitialTranslation.ModelName,
		stepSet.Reflection.ModelName,
		stepSet.Improvement.ModelName,
	}
	
	// 为每个唯一的模型创建提供者
	for _, modelName := range modelNames {
		if modelName == "" || modelName == "none" {
			continue
		}
		
		// 检查是否已创建
		if _, exists := providers[modelName]; exists {
			continue
		}
		
		// 从配置中获取模型配置
		modelConfig, ok := cfg.ModelConfigs[modelName]
		if !ok {
			return nil, fmt.Errorf("model config not found: %s", modelName)
		}
		
		// 创建 LLM 提供者
		provider := &LLMProvider{
			name:        modelName,
			modelConfig: modelConfig,
			logger:      logger,
		}
		
		providers[modelName] = provider
	}
	
	// 创建步骤集配置
	stepSetConfig := &translation.StepSetConfig{
		Name:        cfg.ActiveStepSet,
		Description: fmt.Sprintf("Step set: %s", cfg.ActiveStepSet),
		Initial: translation.StepConfig{
			Name:        "initial_translation",
			Provider:    stepSet.InitialTranslation.ModelName,
			Model:       stepSet.InitialTranslation.ModelName,
			Temperature: float32(stepSet.InitialTranslation.Temperature),
			MaxTokens:   4096, // 可以从配置中读取
		},
		Reflection: translation.StepConfig{
			Name:        "reflection",
			Provider:    stepSet.Reflection.ModelName,
			Model:       stepSet.Reflection.ModelName,
			Temperature: float32(stepSet.Reflection.Temperature),
			MaxTokens:   2048,
		},
		Improvement: translation.StepConfig{
			Name:        "improvement",
			Provider:    stepSet.Improvement.ModelName,
			Model:       stepSet.Improvement.ModelName,
			Temperature: float32(stepSet.Improvement.Temperature),
			MaxTokens:   4096,
		},
	}
	
	// 添加推理模型信息到元数据
	reasoningModels := make(map[string]bool)
	for _, modelName := range modelNames {
		if modelConfig, ok := cfg.ModelConfigs[modelName]; ok && modelConfig.IsReasoning {
			reasoningModels[modelName+":"+modelName] = true
		}
	}
	if len(reasoningModels) > 0 {
		translationConfig.Metadata["reasoning_models"] = reasoningModels
	}
	
	// 创建三步翻译器
	translator := translation.NewThreeStepTranslator(translationConfig, providers, stepSetConfig)
	
	// 如果启用缓存，设置缓存
	if cfg.CacheEnabled {
		cache := &FileCache{
			dir: cfg.CacheDir,
		}
		translator.SetCache(cache)
	}
	
	// 创建进度跟踪器
	progressTracker := translator.NewTranslationProgressTracker(0, nil, "", 0)
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &ThreeStepTranslatorAdapter{
		translator:      translator,
		config:          cfg,
		logger:          logger,
		progressTracker: progressTracker,
		ctx:             ctx,
		cancel:          cancel,
	}, nil
}

// Translate 实现 Translator 接口
func (t *ThreeStepTranslatorAdapter) Translate(text string, retryFailedParts bool) (string, error) {
	// 处理空文本
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	
	// 执行翻译
	result, err := t.translator.TranslateText(t.ctx, text)
	if err != nil {
		t.logger.Error("Translation failed", zap.Error(err))
		return "", err
	}
	
	// 移除推理标记（如果需要）
	result = t.removeReasoningTags(result)
	
	return result, nil
}

// removeReasoningTags 移除推理标记
func (t *ThreeStepTranslatorAdapter) removeReasoningTags(content string) string {
	// 获取活动步骤集
	stepSet, exists := t.config.StepSets[t.config.ActiveStepSet]
	if !exists {
		return content
	}
	
	var allTags []string
	tagsMap := make(map[string]bool)
	
	// 收集所有推理标记
	modelsToCheck := []string{
		stepSet.InitialTranslation.ModelName,
		stepSet.Reflection.ModelName,
		stepSet.Improvement.ModelName,
	}
	
	for _, modelName := range modelsToCheck {
		if modelName != "" && modelName != "none" {
			if modelConfig, ok := t.config.ModelConfigs[modelName]; ok {
				if modelConfig.IsReasoning && len(modelConfig.ReasoningTags) > 0 {
					for _, tag := range modelConfig.ReasoningTags {
						if !tagsMap[tag] {
							allTags = append(allTags, tag)
							tagsMap[tag] = true
						}
					}
				}
			}
		}
	}
	
	// 使用收集到的标记移除推理过程
	return translation.RemoveReasoningProcess(content, allTags)
}

// GetConfig 获取配置
func (t *ThreeStepTranslatorAdapter) GetConfig() *config.Config {
	return t.config
}

// GetProgress 获取进度
func (t *ThreeStepTranslatorAdapter) GetProgress() translator.Progress {
	return t.progressTracker.GetProgress()
}

// Stop 停止翻译
func (t *ThreeStepTranslatorAdapter) Stop() {
	t.cancel()
}

// SetDebugMode 设置调试模式
func (t *ThreeStepTranslatorAdapter) SetDebugMode(debug bool) {
	// 调试模式实现
}

// AddDebugRecord 添加调试记录
func (t *ThreeStepTranslatorAdapter) AddDebugRecord(original, translated string) {
	// 调试记录实现
}

// GetDebugRecords 获取调试记录
func (t *ThreeStepTranslatorAdapter) GetDebugRecords() []translator.DebugRecord {
	return nil
}

// LLMProvider 实现 translation.Provider 接口
type LLMProvider struct {
	name        string
	modelConfig config.ModelConfig
	logger      *logger.Logger
}

// Name 返回提供者名称
func (p *LLMProvider) Name() string {
	return p.name
}

// Translate 执行翻译
func (p *LLMProvider) Translate(ctx context.Context, req *translation.Request) (*translation.Response, error) {
	// 创建 LLM 客户端
	llmClient, err := createLLMClient(p.modelConfig)
	if err != nil {
		return nil, err
	}
	
	// 创建完成请求
	completionReq := &translation.CompletionRequest{
		Prompt:      req.Text,
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	
	// 调用 LLM
	completionResp, err := llmClient.Complete(ctx, completionReq)
	if err != nil {
		return nil, err
	}
	
	// 构建响应
	resp := &translation.Response{
		Text:           completionResp.Text,
		SourceLanguage: req.SourceLanguage,
		TargetLanguage: req.TargetLanguage,
		Usage: translation.Usage{
			InputTokens:  completionResp.TokensIn,
			OutputTokens: completionResp.TokensOut,
		},
		Metadata: req.Metadata,
	}
	
	return resp, nil
}

// createLLMClient 创建 LLM 客户端
func createLLMClient(modelConfig config.ModelConfig) (translation.LLMClient, error) {
	// 这里需要根据模型配置创建具体的 LLM 客户端
	// 暂时返回一个占位实现
	return &MockLLMClient{
		model: modelConfig.Name,
	}, nil
}

// MockLLMClient 模拟 LLM 客户端（临时实现）
type MockLLMClient struct {
	model string
}

func (m *MockLLMClient) Complete(ctx context.Context, req *translation.CompletionRequest) (*translation.CompletionResponse, error) {
	// 模拟响应
	return &translation.CompletionResponse{
		Text:      "Translated: " + req.Prompt,
		Model:     m.model,
		TokensIn:  len(req.Prompt),
		TokensOut: len(req.Prompt) + 12,
	}, nil
}

func (m *MockLLMClient) Chat(ctx context.Context, req *translation.ChatRequest) (*translation.ChatResponse, error) {
	return nil, fmt.Errorf("chat not implemented")
}

func (m *MockLLMClient) GetModel() string {
	return m.model
}

func (m *MockLLMClient) HealthCheck(ctx context.Context) error {
	return nil
}

// FileCache 简单的文件缓存实现
type FileCache struct {
	dir string
}

func (c *FileCache) Get(key string) (string, bool) {
	// 简单实现，实际应该从文件读取
	return "", false
}

func (c *FileCache) Set(key string, value string) error {
	// 简单实现，实际应该写入文件
	return nil
}

func (c *FileCache) Delete(key string) error {
	return nil
}

func (c *FileCache) Clear() error {
	return nil
}

func (c *FileCache) Stats() translation.CacheStats {
	return translation.CacheStats{}
}