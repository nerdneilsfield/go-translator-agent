package translation

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TraceLevel 定义 TRACE 日志级别，与logger包保持一致
const TraceLevel zapcore.Level = -2

// traceLog 安全地记录TRACE级别日志
func (c *chain) traceLog(msg string, fields ...zap.Field) {
	if c.logger != nil {
		if ce := c.logger.Check(TraceLevel, msg); ce != nil {
			ce.Write(fields...)
		}
	}
}

// chain 翻译链实现
type chain struct {
	steps   []Step
	options chainOptions
	logger  *zap.Logger // 新增：日志记录器
	mu      sync.RWMutex
	// 执行时的状态
	executionState struct {
		originalText string
		results      []StepResult
	}
}

// NewChain 创建新的翻译链
func NewChain(opts ...ChainOption) Chain {
	options := chainOptions{
		maxRetries: 3,
	}
	for _, opt := range opts {
		opt(&options)
	}

	return &chain{
		steps:   make([]Step, 0),
		options: options,
		logger:  options.logger,
	}
}

// Execute 执行翻译链
func (c *chain) Execute(ctx context.Context, input string) (*ChainResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.steps) == 0 {
		return nil, ErrNoSteps
	}

	// TRACE: 记录翻译链开始执行
	c.traceLog("translation_chain_start",
		zap.String("original_text", input),
		zap.Int("num_steps", len(c.steps)),
		zap.Int("input_length", len(input)))

	result := &ChainResult{
		Steps:       make([]StepResult, 0, len(c.steps)),
		Success:     true,
		FinalOutput: input,
	}

	startTime := time.Now()
	currentInput := input

	// 保存执行状态
	c.executionState.originalText = input
	c.executionState.results = make([]StepResult, 0, len(c.steps))

	// 执行每个步骤
	for i, step := range c.steps {
		stepResult, err := c.executeStep(ctx, step, currentInput, i)
		result.Steps = append(result.Steps, *stepResult)
		c.executionState.results = append(c.executionState.results, *stepResult)

		if err != nil {
			result.Success = false
			result.Error = err

			if !c.options.continueOnError {
				break
			}
		} else {
			currentInput = stepResult.Output
			result.FinalOutput = stepResult.Output
		}
	}

	result.TotalDuration = time.Since(startTime)

	// TRACE: 记录翻译链执行完成
	c.traceLog("translation_chain_complete",
		zap.String("original_text", input),
		zap.String("final_output", result.FinalOutput),
		zap.Duration("total_duration", result.TotalDuration),
		zap.Bool("success", result.Success),
		zap.Int("steps_executed", len(result.Steps)),
		zap.Int("output_length", len(result.FinalOutput)))

	return result, result.Error
}

// AddStep 添加翻译步骤
func (c *chain) AddStep(step Step) Chain {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.steps = append(c.steps, step)
	return c
}

// GetSteps 获取所有步骤
func (c *chain) GetSteps() []Step {
	c.mu.RLock()
	defer c.mu.RUnlock()

	steps := make([]Step, len(c.steps))
	copy(steps, c.steps)
	return steps
}

// executeStep 执行单个步骤
func (c *chain) executeStep(ctx context.Context, step Step, input string, index int) (*StepResult, error) {
	stepConfig := step.GetConfig()

	result := &StepResult{
		Name:  step.GetName(),
		Model: stepConfig.Model,
	}

	startTime := time.Now()
	defer func() {
		result.Duration = time.Since(startTime)
	}()

	// 准备步骤输入
	stepInput := StepInput{
		Text:           input,
		SourceLanguage: stepConfig.Variables["source_language"],
		TargetLanguage: stepConfig.Variables["target_language"],
	}

	// 设置上下文
	stepInput.Context = make(map[string]string)

	// 添加基本的占位符
	stepInput.Context["text"] = input
	stepInput.Context["source_language"] = stepConfig.Variables["source_language"]
	stepInput.Context["target_language"] = stepConfig.Variables["target_language"]

	// 从 context 中提取元数据标记
	if ctx.Value("_is_batch") != nil {
		stepInput.Context["_is_batch"] = fmt.Sprintf("%v", ctx.Value("_is_batch"))
	}
	if ctx.Value("_preserve_enabled") != nil {
		stepInput.Context["_preserve_enabled"] = fmt.Sprintf("%v", ctx.Value("_preserve_enabled"))
	}

	// 根据步骤类型添加特定的上下文
	if index == 0 {
		// 初始翻译：原文就是输入
		stepInput.Context["original_text"] = input
	} else if index == 1 && len(c.executionState.results) > 0 {
		// 反思步骤：需要原文和初始翻译
		stepInput.Context["original_text"] = c.executionState.originalText
		stepInput.Context["translation"] = c.executionState.results[0].Output
		stepInput.Context["initial_translation"] = c.executionState.results[0].Output
	} else if index == 2 && len(c.executionState.results) > 1 {
		// 改进步骤：需要原文、初始翻译和反思
		stepInput.Context["original_text"] = c.executionState.originalText
		stepInput.Context["translation"] = c.executionState.results[0].Output
		stepInput.Context["initial_translation"] = c.executionState.results[0].Output
		stepInput.Context["reflection"] = c.executionState.results[1].Output
		stepInput.Context["feedback"] = c.executionState.results[1].Output
		stepInput.Context["ai_review"] = c.executionState.results[1].Output
	}

	// 如果配置中有额外的变量，也添加进去
	for k, v := range stepConfig.Variables {
		if k != "source_language" && k != "target_language" {
			stepInput.Context[k] = v
		}
	}

	// 执行步骤，带重试
	var lastErr error
	for attempt := 0; attempt <= c.options.maxRetries; attempt++ {
		if attempt > 0 {
			// 重试延迟
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}

		// TRACE: 记录步骤执行前的详细信息
		c.traceLog("translation_step_start",
			zap.String("step_name", step.GetName()),
			zap.Int("step_index", index),
			zap.Int("attempt", attempt),
			zap.String("input_text", input),
			zap.String("model", stepConfig.Model),
			zap.Any("step_context", stepInput.Context))

		output, err := step.Execute(ctx, stepInput)
		if err == nil {
			result.Output = output.Text
			result.TokensIn = output.TokensIn
			result.TokensOut = output.TokensOut

			// TRACE: 记录步骤执行成功的详细信息
			c.traceLog("translation_step_success",
				zap.String("step_name", step.GetName()),
				zap.Int("step_index", index),
				zap.String("input_text", input),
				zap.String("output_text", output.Text),
				zap.Int("tokens_in", output.TokensIn),
				zap.Int("tokens_out", output.TokensOut),
				zap.Duration("step_duration", time.Since(startTime)))

			return result, nil
		}

		lastErr = err

		// 检查是否可重试
		if !isRetryableError(err) {
			break
		}
	}

	result.Error = lastErr.Error()

	// TRACE: 记录步骤执行失败的详细信息
	c.traceLog("translation_step_failed",
		zap.String("step_name", step.GetName()),
		zap.Int("step_index", index),
		zap.String("input_text", input),
		zap.String("error", lastErr.Error()),
		zap.Int("total_attempts", c.options.maxRetries+1),
		zap.Duration("step_duration", time.Since(startTime)))

	// 创建更详细的错误信息
	stepName := step.GetName()
	errorMsg := fmt.Sprintf("step '%s' failed after %d attempts", stepName, c.options.maxRetries+1)

	// 如果是 TranslationError，保留更多上下文
	if transErr, ok := lastErr.(*TranslationError); ok {
		errorMsg = fmt.Sprintf("step '%s' failed: %s", stepName, transErr.Message)
		// 创建新的错误，保留原有的错误码和可重试状态
		return result, &TranslationError{
			Code:    transErr.Code,
			Message: errorMsg,
			Cause:   transErr.Cause,
			Step:    stepName,
			Retry:   transErr.Retry,
		}
	}

	return result, WrapError(lastErr, ErrCodeStep, errorMsg)
}

// step 翻译步骤实现
type step struct {
	config    *StepConfig
	llmClient LLMClient
	provider  TranslationProvider
	cache     Cache
}

// NewStep 创建新的翻译步骤
func NewStep(config *StepConfig, llmClient LLMClient, cache Cache) Step {
	return &step{
		config:    config,
		llmClient: llmClient,
		cache:     cache,
	}
}

// NewProviderStep 使用翻译提供商创建步骤
func NewProviderStep(config *StepConfig, provider TranslationProvider, cache Cache) Step {
	return &step{
		config:   config,
		provider: provider,
		cache:    cache,
	}
}

// Execute 执行步骤
func (s *step) Execute(ctx context.Context, input StepInput) (*StepOutput, error) {
	// 如果有 provider，优先使用 provider（适用于专业翻译服务）
	if s.provider != nil {
		return s.executeWithProvider(ctx, input)
	}

	// 否则使用 LLM
	if s.llmClient == nil {
		return nil, ErrNoLLMClient
	}

	return s.executeWithLLM(ctx, input)
}

// executeWithProvider 使用翻译提供商执行
func (s *step) executeWithProvider(ctx context.Context, input StepInput) (*StepOutput, error) {
	// 检查缓存
	cacheKey := s.getCacheKeyForProvider(input)
	if s.cache != nil {
		if cached, found := s.cache.Get(cacheKey); found {
			return &StepOutput{
				Text:  cached,
				Model: s.provider.GetName(),
			}, nil
		}
	}

	// 准备请求
	metadata := make(map[string]interface{})
	for k, v := range s.config.Variables {
		metadata[k] = v
	}
	req := &ProviderRequest{
		Text:           input.Text,
		SourceLanguage: input.SourceLanguage,
		TargetLanguage: input.TargetLanguage,
		Metadata:       metadata,
	}

	// 设置超时
	if s.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.config.Timeout)
		defer cancel()
	}

	// 调用提供商
	resp, err := s.provider.Translate(ctx, req)
	if err != nil {
		// 创建详细的错误信息
		providerName := s.provider.GetName()
		errorMsg := fmt.Sprintf("provider '%s' translation failed for step '%s'", providerName, s.config.Name)
		return nil, WrapError(err, ErrCodeLLM, errorMsg)
	}

	// 从 metadata 中获取 model 信息
	model := ""
	if resp.Metadata != nil {
		if m, ok := resp.Metadata["model"]; ok {
			if modelStr, ok := m.(string); ok {
				model = modelStr
			}
		}
	}

	// 移除推理标记（总是尝试，如果没有推理标记则无副作用）
	cleanedText := RemoveReasoningMarkers(resp.Text)

	output := &StepOutput{
		Text:      cleanedText,
		Model:     model,
		TokensIn:  resp.TokensIn,
		TokensOut: resp.TokensOut,
	}

	// 缓存结果（使用清理后的文本）
	if s.cache != nil {
		_ = s.cache.Set(cacheKey, cleanedText)
	}

	return output, nil
}

// executeWithLLM 使用 LLM 执行
func (s *step) executeWithLLM(ctx context.Context, input StepInput) (*StepOutput, error) {
	// 准备提示词
	prompt := s.preparePrompt(input)

	// 检查缓存
	if s.cache != nil {
		cacheKey := s.getCacheKey(prompt)
		if cached, found := s.cache.Get(cacheKey); found {
			return &StepOutput{
				Text:  cached,
				Model: s.config.Model,
			}, nil
		}
	}

	// 准备聊天请求
	messages := []ChatMessage{
		{
			Role:    "system",
			Content: s.getSystemRole(),
		},
		{
			Role:    "user",
			Content: prompt,
		},
	}

	req := &ChatRequest{
		Messages:    messages,
		Model:       s.config.Model,
		Temperature: s.config.Temperature,
		MaxTokens:   s.config.MaxTokens,
	}

	// 设置超时
	if s.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.config.Timeout)
		defer cancel()
	}

	// 调用LLM
	resp, err := s.llmClient.Chat(ctx, req)
	if err != nil {
		// 创建详细的错误信息
		errorMsg := fmt.Sprintf("LLM call failed for step '%s' with model '%s'", s.config.Name, s.config.Model)
		return nil, WrapError(err, ErrCodeLLM, errorMsg)
	}

	// 移除推理标记（总是尝试，如果没有推理标记则无副作用）
	cleanedText := RemoveReasoningMarkers(resp.Message.Content)

	output := &StepOutput{
		Text:      cleanedText,
		Model:     resp.Model,
		TokensIn:  resp.TokensIn,
		TokensOut: resp.TokensOut,
	}

	// 缓存结果（使用清理后的文本）
	if s.cache != nil {
		cacheKey := s.getCacheKey(prompt)
		_ = s.cache.Set(cacheKey, cleanedText)
	}

	return output, nil
}

// GetName 获取步骤名称
func (s *step) GetName() string {
	return s.config.Name
}

// GetConfig 获取步骤配置
func (s *step) GetConfig() *StepConfig {
	return s.config
}

// getAdditionalNotes 获取用户自定义说明
func (s *step) getAdditionalNotes() string {
	return s.config.AdditionalNotes
}

// getSystemRole 根据步骤名称自动生成系统角色
func (s *step) getSystemRole() string {
	stepName := strings.ToLower(s.config.Name)

	if strings.Contains(stepName, "reflection") || strings.Contains(stepName, "review") {
		return "You are a translation quality reviewer and linguistic expert."
	} else if strings.Contains(stepName, "improvement") || strings.Contains(stepName, "polish") {
		return "You are a professional translator focusing on quality improvement."
	} else {
		// 默认翻译角色
		return "You are a professional translator."
	}
}

// preparePrompt 准备提示词
func (s *step) preparePrompt(input StepInput) string {
	// 根据步骤名称自动选择内置模板
	var templateType TemplateType
	stepName := strings.ToLower(s.config.Name)

	if strings.Contains(stepName, "reflection") || strings.Contains(stepName, "review") {
		templateType = TemplateTypeReflection
	} else if strings.Contains(stepName, "improvement") || strings.Contains(stepName, "polish") {
		templateType = TemplateTypeImprovement
	} else {
		// 默认使用标准翻译模板
		templateType = TemplateTypeStandard
	}

	promptTemplate := GetBuiltinTemplate(templateType)

	// 创建模板数据
	data := map[string]interface{}{
		"text":            input.Text,
		"source_language": input.SourceLanguage,
		"target_language": input.TargetLanguage,
		"source":          input.SourceLanguage,
		"target":          input.TargetLanguage,
		"country":         "China", // 默认值，可以从配置中获取
	}

	// 添加上下文变量
	for k, v := range input.Context {
		data[k] = v
	}

	// 添加步骤配置的 additional_notes
	if additionalNotes := s.getAdditionalNotes(); additionalNotes != "" {
		data["additional_notes"] = additionalNotes
	}

	// 使用 text/template 处理模板
	tmpl, err := template.New("prompt").Parse(promptTemplate)
	if err != nil {
		// 如果模板解析失败，回退到简单字符串替换（向后兼容）
		return s.fallbackStringReplace(promptTemplate, data)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		// 如果模板执行失败，回退到简单字符串替换（向后兼容）
		return s.fallbackStringReplace(promptTemplate, data)
	}

	return buf.String()
}

// fallbackStringReplace 回退到简单字符串替换（向后兼容）
func (s *step) fallbackStringReplace(promptTemplate string, data map[string]interface{}) string {
	prompt := promptTemplate

	// 基本变量替换
	if text, ok := data["text"].(string); ok {
		prompt = strings.ReplaceAll(prompt, "{{text}}", text)
	}
	if sourceLang, ok := data["source_language"].(string); ok {
		prompt = strings.ReplaceAll(prompt, "{{source_language}}", sourceLang)
		prompt = strings.ReplaceAll(prompt, "{{source}}", sourceLang)
	}
	if targetLang, ok := data["target_language"].(string); ok {
		prompt = strings.ReplaceAll(prompt, "{{target_language}}", targetLang)
		prompt = strings.ReplaceAll(prompt, "{{target}}", targetLang)
	}

	// 其他变量替换
	for k, v := range data {
		if str, ok := v.(string); ok {
			prompt = strings.ReplaceAll(prompt, fmt.Sprintf("{{%s}}", k), str)
		}
	}

	return prompt
}

// getCacheKey 生成缓存键
func (s *step) getCacheKey(prompt string) string {
	// 简单的缓存键生成，实际使用中可能需要更复杂的逻辑
	return fmt.Sprintf("translation:%s:%s:%x", s.config.Name, s.config.Model, hash(prompt))
}

// getCacheKeyForProvider 为提供商生成缓存键
func (s *step) getCacheKeyForProvider(input StepInput) string {
	key := fmt.Sprintf("provider:%s:%s:%s:%s:%x",
		s.provider.GetName(),
		s.config.Name,
		input.SourceLanguage,
		input.TargetLanguage,
		hash(input.Text),
	)
	return key
}

// hash 简单的哈希函数
func hash(s string) uint32 {
	h := uint32(0)
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return h
}
