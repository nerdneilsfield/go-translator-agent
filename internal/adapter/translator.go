package adapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
)

// TranslatorAdapter 适配器，将旧的 Translator 接口适配到新的 translation.Service
type TranslatorAdapter struct {
	service         translation.Service
	config          *config.Config
	logger          logger.Logger
	progressTracker *translator.TranslationProgressTracker
	ctx             context.Context
	cancel          context.CancelFunc
}

// NewTranslatorAdapter 创建新的翻译器适配器
func NewTranslatorAdapter(cfg *config.Config, logger logger.Logger) (*TranslatorAdapter, error) {
	// 将旧配置转换为新配置
	translationConfig, err := ConvertConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to convert config: %w", err)
	}

	// 创建提供商
	providers, err := CreateProviders(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create providers: %w", err)
	}

	// 创建选项
	options := []translation.Option{}
	
	// 添加提供商
	if len(providers) > 0 {
		options = append(options, translation.WithProviders(providers))
	}

	// 如果只有一个 LLM 客户端，使用传统方式
	if llmClient := CreateLLMClient(cfg); llmClient != nil {
		options = append(options, translation.WithLLMClient(llmClient))
	}

	// 添加进度回调
	progressTracker := translator.NewTranslationProgressTracker(0, nil, "", 0)
	options = append(options, translation.WithProgressCallback(func(p *translation.Progress) {
		progressTracker.UpdateProgress(p.Completed)
		// 进度日志可选，不影响功能
	}))

	// 创建翻译服务
	service, err := translation.New(translationConfig, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create translation service: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TranslatorAdapter{
		service:         service,
		config:          cfg,
		logger:          logger,
		progressTracker: progressTracker,
		ctx:             ctx,
		cancel:          cancel,
	}, nil
}

// Translate 实现 Translator 接口
func (t *TranslatorAdapter) Translate(text string, retryFailedParts bool) (string, error) {
	// 处理空文本
	if strings.TrimSpace(text) == "" {
		return "", nil
	}

	// 创建翻译请求
	req := &translation.Request{
		Text:           text,
		SourceLanguage: t.config.SourceLang,
		TargetLanguage: t.config.TargetLang,
		Metadata: map[string]string{
			"retry_failed": fmt.Sprintf("%v", retryFailedParts),
		},
	}

	// 执行翻译
	resp, err := t.service.Translate(t.ctx, req)
	if err != nil {
		// 错误日志可选，不影响功能
		return "", err
	}

	// 检查并处理推理模型的输出
	translatedText := resp.Text
	if t.shouldRemoveReasoning() {
		translatedText = t.removeReasoningFromResult(translatedText)
	}

	// 记录统计信息（暂时禁用，需要正确的 zap fields）

	return translatedText, nil
}

// GetLogger 返回日志记录器
func (t *TranslatorAdapter) GetLogger() logger.Logger {
	return t.logger
}

// GetProgressTracker 返回进度跟踪器
func (t *TranslatorAdapter) GetProgressTracker() *translator.TranslationProgressTracker {
	return t.progressTracker
}

// GetConfig 返回配置
func (t *TranslatorAdapter) GetConfig() *config.Config {
	return t.config
}

// GetProgress 返回当前进度
func (t *TranslatorAdapter) GetProgress() string {
	if t.progressTracker == nil {
		return "0%"
	}
	
	progress, _, _, _, _, _, _ := t.progressTracker.GetProgress()
	completed := int(progress * 100)
	return fmt.Sprintf("%d%%", completed)
}

// InitTranslator 初始化翻译器
func (t *TranslatorAdapter) InitTranslator() {
	// 重置进度跟踪器
	if t.progressTracker != nil {
		t.progressTracker.Reset()
	}
	
	// 创建新的上下文
	t.ctx, t.cancel = context.WithCancel(context.Background())
}

// Finish 结束翻译
func (t *TranslatorAdapter) Finish() {
	// 取消上下文
	if t.cancel != nil {
		t.cancel()
	}
	
	// 标记进度为完成（进度跟踪器没有该方法）
}

// shouldRemoveReasoning 检查是否需要移除推理过程
func (t *TranslatorAdapter) shouldRemoveReasoning() bool {
	// 检查当前活动的步骤集中是否有任何推理模型
	if t.config.ActiveStepSet == "" {
		return false
	}

	// 检查新格式步骤集
	if stepSetV2, exists := t.config.StepSetsV2[t.config.ActiveStepSet]; exists {
		for _, step := range stepSetV2.Steps {
			if modelConfig, ok := t.config.ModelConfigs[step.ModelName]; ok {
				if modelConfig.IsReasoning {
					return true
				}
			}
		}
	}

	// 检查旧格式步骤集
	if stepSet, exists := t.config.StepSets[t.config.ActiveStepSet]; exists {
		modelsToCheck := []string{
			stepSet.InitialTranslation.ModelName,
			stepSet.Reflection.ModelName,
			stepSet.Improvement.ModelName,
		}
		for _, modelName := range modelsToCheck {
			if modelName != "" && modelName != "none" {
				if modelConfig, ok := t.config.ModelConfigs[modelName]; ok {
					if modelConfig.IsReasoning {
						return true
					}
				}
			}
		}
	}

	return false
}

// removeReasoningFromResult 从结果中移除推理过程
func (t *TranslatorAdapter) removeReasoningFromResult(content string) string {
	// 收集所有可能的推理标记
	var allTags []string
	tagsMap := make(map[string]bool)

	// 从步骤集中收集模型的推理标记
	if stepSetV2, exists := t.config.StepSetsV2[t.config.ActiveStepSet]; exists {
		for _, step := range stepSetV2.Steps {
			if modelConfig, ok := t.config.ModelConfigs[step.ModelName]; ok {
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

	if stepSet, exists := t.config.StepSets[t.config.ActiveStepSet]; exists {
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
	}

	// 使用收集到的标记移除推理过程
	return translation.RemoveReasoningProcess(content, allTags)
}

