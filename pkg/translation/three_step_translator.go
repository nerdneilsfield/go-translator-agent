package translation

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// ThreeStepTranslator 三步翻译器实现
type ThreeStepTranslator struct {
	// 配置
	config *Config
	// 提供者管理器
	providers map[string]Provider
	// 缓存
	cache Cache
	// 提示词构建器
	promptBuilder *PromptBuilder
	// 保护块管理器
	preserveManager *PreserveManager
	// 步骤集
	stepSet *StepSetConfig
	// 互斥锁
	mu sync.RWMutex
}

// NewThreeStepTranslator 创建三步翻译器
func NewThreeStepTranslator(config *Config, providers map[string]Provider, stepSet *StepSetConfig) *ThreeStepTranslator {
	country := ""
	if config.Metadata != nil {
		if c, ok := config.Metadata["country"].(string); ok {
			country = c
		}
	}

	promptBuilder := NewPromptBuilder(config.SourceLanguage, config.TargetLanguage, country)

	// 如果配置中有保护块设置，使用它
	preserveConfig := DefaultPreserveConfig
	if config.Metadata != nil {
		if patterns, ok := config.Metadata["preserve_patterns"].([]string); ok && len(patterns) > 0 {
			// 可以根据配置调整保护块设置
			preserveConfig.Enabled = true
		}
	}

	return &ThreeStepTranslator{
		config:          config,
		providers:       providers,
		promptBuilder:   promptBuilder.WithPreserveConfig(preserveConfig),
		preserveManager: NewPreserveManager(preserveConfig),
		stepSet:         stepSet,
	}
}

// TranslateText 翻译文本
func (t *ThreeStepTranslator) TranslateText(ctx context.Context, text string) (string, error) {
	// 应用保护块
	protectedText, err := t.applyProtection(text)
	if err != nil {
		return "", fmt.Errorf("failed to apply protection: %w", err)
	}

	var translatedText string

	// 检查是否使用快速模式
	fastMode := false
	fastModeThreshold := 100

	// 从元数据中提取快速模式配置
	if t.config.Metadata != nil {
		if fm, ok := t.config.Metadata["fast_mode"].(bool); ok {
			fastMode = fm
		}
		if fmt, ok := t.config.Metadata["fast_mode_threshold"].(int); ok {
			fastModeThreshold = fmt
		}
	}

	if fastMode || len(text) < fastModeThreshold {
		translatedText, err = t.translateDirect(ctx, protectedText)
	} else {
		// 使用三步翻译流程
		translatedText, err = t.translateThreeStep(ctx, protectedText)
	}

	if err != nil {
		return "", err
	}

	// 还原保护块
	restoredText := t.preserveManager.Restore(translatedText)

	return restoredText, nil
}

// translateDirect 直接翻译（快速模式）
func (t *ThreeStepTranslator) translateDirect(ctx context.Context, text string) (string, error) {
	// 检查缓存
	cacheKey := GenerateCacheKey(CacheKeyComponents{
		Step:        "direct",
		Provider:    t.stepSet.Initial.Provider,
		Model:       t.stepSet.Initial.Model,
		SourceLang:  t.config.SourceLanguage,
		TargetLang:  t.config.TargetLanguage,
		Text:        text,
		Temperature: t.stepSet.Initial.Temperature,
		MaxTokens:   t.stepSet.Initial.MaxTokens,
	})
	if t.cache != nil {
		if cached, ok := t.cache.Get(cacheKey); ok {
			return cached, nil
		}
	}

	// 获取初始翻译的提供者
	provider, ok := t.providers[t.stepSet.Initial.Provider]
	if !ok {
		return "", fmt.Errorf("provider not found: %s", t.stepSet.Initial.Provider)
	}

	// 构建提示词
	prompt := t.promptBuilder.BuildDirectTranslationPrompt(text)

	// 执行翻译
	request := &Request{
		Text:        prompt,
		Model:       t.stepSet.Initial.Model,
		Temperature: t.stepSet.Initial.Temperature,
		MaxTokens:   t.stepSet.Initial.MaxTokens,
		Metadata: map[string]interface{}{
			"step": "direct",
		},
	}

	response, err := provider.Translate(ctx, request)
	if err != nil {
		return "", fmt.Errorf("translation failed: %w", err)
	}

	// 提取翻译结果
	translation := ExtractTranslationFromResponse(response.Text)

	// 如果是推理模型，移除推理标记
	if isReasoning := t.isReasoningModel(t.stepSet.Initial.Provider, t.stepSet.Initial.Model); isReasoning {
		translation = RemoveReasoningMarkers(translation)
	}

	// 缓存结果
	if t.cache != nil {
		_ = t.cache.Set(cacheKey, translation)
	}

	return translation, nil
}

// translateThreeStep 三步翻译流程
func (t *ThreeStepTranslator) translateThreeStep(ctx context.Context, text string) (string, error) {
	// 第一步：初始翻译
	initialTranslation, err := t.initialTranslation(ctx, text)
	if err != nil {
		return "", fmt.Errorf("initial translation failed: %w", err)
	}

	// 第二步：反思
	reflection, err := t.reflection(ctx, text, initialTranslation)
	if err != nil {
		// 反思失败时返回初始翻译
		return initialTranslation, nil
	}

	// 检查是否需要改进
	if strings.Contains(strings.ToLower(reflection), "no issues") ||
		strings.Contains(strings.ToLower(reflection), "perfect") {
		return initialTranslation, nil
	}

	// 第三步：改进
	improvedTranslation, err := t.improvement(ctx, text, initialTranslation, reflection)
	if err != nil {
		// 改进失败时返回初始翻译
		return initialTranslation, nil
	}

	return improvedTranslation, nil
}

// initialTranslation 执行初始翻译
func (t *ThreeStepTranslator) initialTranslation(ctx context.Context, text string) (string, error) {
	// 检查缓存
	cacheKey := GenerateCacheKey(CacheKeyComponents{
		Step:        "initial",
		Provider:    t.stepSet.Initial.Provider,
		Model:       t.stepSet.Initial.Model,
		SourceLang:  t.config.SourceLanguage,
		TargetLang:  t.config.TargetLanguage,
		Text:        text,
		Temperature: t.stepSet.Initial.Temperature,
		MaxTokens:   t.stepSet.Initial.MaxTokens,
	})
	if t.cache != nil {
		if cached, ok := t.cache.Get(cacheKey); ok {
			return cached, nil
		}
	}

	// 获取提供者
	provider, ok := t.providers[t.stepSet.Initial.Provider]
	if !ok {
		return "", fmt.Errorf("provider not found: %s", t.stepSet.Initial.Provider)
	}

	// 构建提示词
	prompt := t.promptBuilder.BuildInitialTranslationPrompt(text)

	// 执行翻译
	systemPrompt := "You are a professional translator. Follow the instructions carefully."
	fullPrompt := systemPrompt + "\n\n" + prompt

	request := &Request{
		Text:        fullPrompt,
		Model:       t.stepSet.Initial.Model,
		Temperature: t.stepSet.Initial.Temperature,
		MaxTokens:   t.stepSet.Initial.MaxTokens,
		Metadata: map[string]interface{}{
			"step": "initial",
		},
	}

	response, err := provider.Translate(ctx, request)
	if err != nil {
		return "", err
	}

	translation := ExtractTranslationFromResponse(response.Text)

	// 如果是推理模型，移除推理标记
	if isReasoning := t.isReasoningModel(t.stepSet.Initial.Provider, t.stepSet.Initial.Model); isReasoning {
		translation = RemoveReasoningMarkers(translation)
	}

	// 缓存结果
	if t.cache != nil {
		_ = t.cache.Set(cacheKey, translation)
	}

	return translation, nil
}

// reflection 执行反思步骤
func (t *ThreeStepTranslator) reflection(ctx context.Context, sourceText, translation string) (string, error) {
	// 检查缓存
	cacheKey := GenerateCacheKey(CacheKeyComponents{
		Step:        "reflection",
		Provider:    t.stepSet.Reflection.Provider,
		Model:       t.stepSet.Reflection.Model,
		SourceLang:  t.config.SourceLanguage,
		TargetLang:  t.config.TargetLanguage,
		Text:        sourceText,
		Context:     translation, // 初始翻译作为上下文
		Temperature: t.stepSet.Reflection.Temperature,
		MaxTokens:   t.stepSet.Reflection.MaxTokens,
	})
	if t.cache != nil {
		if cached, ok := t.cache.Get(cacheKey); ok {
			return cached, nil
		}
	}

	// 获取提供者
	provider, ok := t.providers[t.stepSet.Reflection.Provider]
	if !ok {
		return "", fmt.Errorf("provider not found: %s", t.stepSet.Reflection.Provider)
	}

	// 构建提示词
	prompt := t.promptBuilder.BuildReflectionPrompt(sourceText, translation)

	// 执行反思
	systemPrompt := "You are a professional translation reviewer. Analyze the translation carefully."
	fullPrompt := systemPrompt + "\n\n" + prompt

	request := &Request{
		Text:        fullPrompt,
		Model:       t.stepSet.Reflection.Model,
		Temperature: t.stepSet.Reflection.Temperature,
		MaxTokens:   t.stepSet.Reflection.MaxTokens,
		Metadata: map[string]interface{}{
			"step": "reflection",
		},
	}

	response, err := provider.Translate(ctx, request)
	if err != nil {
		return "", err
	}

	reflection := response.Text

	// 如果是推理模型，移除推理标记
	if isReasoning := t.isReasoningModel(t.stepSet.Reflection.Provider, t.stepSet.Reflection.Model); isReasoning {
		reflection = RemoveReasoningMarkers(reflection)
	}

	// 缓存结果
	if t.cache != nil {
		_ = t.cache.Set(cacheKey, reflection)
	}

	return reflection, nil
}

// improvement 执行改进步骤
func (t *ThreeStepTranslator) improvement(ctx context.Context, sourceText, translation, reflection string) (string, error) {
	// 检查缓存 - 使用完整的context包含初始翻译和反思结果
	contextData := fmt.Sprintf("translation:%s|reflection:%s", translation, reflection)
	cacheKey := GenerateCacheKey(CacheKeyComponents{
		Step:        "improvement",
		Provider:    t.stepSet.Improvement.Provider,
		Model:       t.stepSet.Improvement.Model,
		SourceLang:  t.config.SourceLanguage,
		TargetLang:  t.config.TargetLanguage,
		Text:        sourceText,
		Context:     contextData, // 包含初始翻译和反思结果
		Temperature: t.stepSet.Improvement.Temperature,
		MaxTokens:   t.stepSet.Improvement.MaxTokens,
	})
	if t.cache != nil {
		if cached, ok := t.cache.Get(cacheKey); ok {
			return cached, nil
		}
	}

	// 获取提供者
	provider, ok := t.providers[t.stepSet.Improvement.Provider]
	if !ok {
		return "", fmt.Errorf("provider not found: %s", t.stepSet.Improvement.Provider)
	}

	// 构建提示词
	prompt := t.promptBuilder.BuildImprovementPrompt(sourceText, translation, reflection)

	// 执行改进
	systemPrompt := "You are a professional translator. Improve the translation based on the feedback."
	fullPrompt := systemPrompt + "\n\n" + prompt

	request := &Request{
		Text:        fullPrompt,
		Model:       t.stepSet.Improvement.Model,
		Temperature: t.stepSet.Improvement.Temperature,
		MaxTokens:   t.stepSet.Improvement.MaxTokens,
		Metadata: map[string]interface{}{
			"step": "improvement",
		},
	}

	response, err := provider.Translate(ctx, request)
	if err != nil {
		return "", err
	}

	improvedTranslation := ExtractTranslationFromResponse(response.Text)

	// 如果是推理模型，移除推理标记
	if isReasoning := t.isReasoningModel(t.stepSet.Improvement.Provider, t.stepSet.Improvement.Model); isReasoning {
		improvedTranslation = RemoveReasoningMarkers(improvedTranslation)
	}

	// 缓存结果
	if t.cache != nil {
		_ = t.cache.Set(cacheKey, improvedTranslation)
	}

	return improvedTranslation, nil
}

// SetCache 设置缓存
func (t *ThreeStepTranslator) SetCache(cache Cache) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cache = cache
}

// UpdateStepSet 更新步骤集
func (t *ThreeStepTranslator) UpdateStepSet(stepSet *StepSetConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stepSet = stepSet
}

// GetPreserveManager 获取保护块管理器
func (t *ThreeStepTranslator) GetPreserveManager() *PreserveManager {
	return t.preserveManager
}

// GetPromptBuilder 获取提示词构建器
func (t *ThreeStepTranslator) GetPromptBuilder() *PromptBuilder {
	return t.promptBuilder
}

// applyProtection 应用保护块
func (t *ThreeStepTranslator) applyProtection(text string) (string, error) {
	if !t.preserveManager.config.Enabled {
		return text, nil
	}

	// 保护代码块
	protectedText := text

	// 保护 Markdown 代码块
	codeBlockRe := regexp.MustCompile("(?s)```[^\n]*\n(.*?)\n```")
	protectedText = codeBlockRe.ReplaceAllStringFunc(protectedText, func(match string) string {
		return t.preserveManager.Protect(match)
	})

	// 保护行内代码
	inlineCodeRe := regexp.MustCompile("`([^`]+)`")
	protectedText = inlineCodeRe.ReplaceAllStringFunc(protectedText, func(match string) string {
		return t.preserveManager.Protect(match)
	})

	// 保护 LaTeX 公式
	// 块级公式
	latexBlockRe := regexp.MustCompile("(?s)\\$\\$(.+?)\\$\\$|\\\\\\[(.+?)\\\\\\]")
	protectedText = latexBlockRe.ReplaceAllStringFunc(protectedText, func(match string) string {
		return t.preserveManager.Protect(match)
	})

	// 行内公式
	latexInlineRe := regexp.MustCompile("\\$([^$\n]+?)\\$|\\\\\\((.+?)\\\\\\)")
	protectedText = latexInlineRe.ReplaceAllStringFunc(protectedText, func(match string) string {
		return t.preserveManager.Protect(match)
	})

	// 保护 URL
	urlRe := regexp.MustCompile(`https?://[^\s<>"{}|\^` + "`" + `\[\]]+`)
	protectedText = urlRe.ReplaceAllStringFunc(protectedText, func(match string) string {
		return t.preserveManager.Protect(match)
	})

	// 保护文献引用
	citationRe := regexp.MustCompile(`\[[0-9]+([-,][0-9]+)*\]`)
	protectedText = citationRe.ReplaceAllStringFunc(protectedText, func(match string) string {
		return t.preserveManager.Protect(match)
	})

	return protectedText, nil
}

// isReasoningModel 判断是否为推理模型
func (t *ThreeStepTranslator) isReasoningModel(provider, model string) bool {
	// 从元数据中获取推理模型列表
	if t.config.Metadata != nil {
		if reasoningModels, ok := t.config.Metadata["reasoning_models"].(map[string]bool); ok {
			key := provider + ":" + model
			return reasoningModels[key]
		}
	}

	// 默认检查一些已知的推理模型
	reasoningModelPatterns := []string{
		"o1-preview",
		"o1-mini",
		"claude-3-opus",
		"deepseek-r1",
	}

	modelLower := strings.ToLower(model)
	for _, pattern := range reasoningModelPatterns {
		if strings.Contains(modelLower, pattern) {
			return true
		}
	}

	return false
}
