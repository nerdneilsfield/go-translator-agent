package translation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// chain 翻译链实现
type chain struct {
	steps   []Step
	options chainOptions
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
	}
}

// Execute 执行翻译链
func (c *chain) Execute(ctx context.Context, input string) (*ChainResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.steps) == 0 {
		return nil, ErrNoSteps
	}

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

		output, err := step.Execute(ctx, stepInput)
		if err == nil {
			result.Output = output.Text
			result.TokensIn = output.TokensIn
			result.TokensOut = output.TokensOut
			return result, nil
		}

		lastErr = err

		// 检查是否可重试
		if !isRetryableError(err) {
			break
		}
	}

	result.Error = lastErr.Error()
	return result, WrapError(lastErr, ErrCodeStep, fmt.Sprintf("step '%s' failed", step.GetName()))
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
	req := &ProviderRequest{
		Text:           input.Text,
		SourceLanguage: input.SourceLanguage,
		TargetLanguage: input.TargetLanguage,
		Options:        s.config.Variables,
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
		return nil, err
	}

	output := &StepOutput{
		Text:      resp.Text,
		Model:     resp.Model,
		TokensIn:  resp.TokensIn,
		TokensOut: resp.TokensOut,
	}

	// 缓存结果
	if s.cache != nil {
		_ = s.cache.Set(cacheKey, output.Text)
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
			Content: s.config.SystemRole,
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
		return nil, err
	}

	output := &StepOutput{
		Text:      resp.Message.Content,
		Model:     resp.Model,
		TokensIn:  resp.TokensIn,
		TokensOut: resp.TokensOut,
	}

	// 缓存结果
	if s.cache != nil {
		cacheKey := s.getCacheKey(prompt)
		_ = s.cache.Set(cacheKey, output.Text)
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

// preparePrompt 准备提示词
func (s *step) preparePrompt(input StepInput) string {
	prompt := s.config.Prompt

	// 替换基本变量
	replacements := map[string]string{
		"{{text}}":            input.Text,
		"{{source_language}}": input.SourceLanguage,
		"{{target_language}}": input.TargetLanguage,
		"{{source}}":          input.SourceLanguage,
		"{{target}}":          input.TargetLanguage,
	}

	// 添加上下文变量
	for k, v := range input.Context {
		replacements[fmt.Sprintf("{{%s}}", k)] = v
	}

	// 添加配置的变量
	for k, v := range s.config.Variables {
		replacements[fmt.Sprintf("{{%s}}", k)] = v
	}

	// 执行替换
	for k, v := range replacements {
		prompt = strings.ReplaceAll(prompt, k, v)
	}

	// 检查上下文中是否有额外的保护说明标记
	// 将保护说明插入到 "Please translate" 之前
	if input.Context != nil {
		var preserveInstructions []string
		
		// 如果有内容保护配置，添加保护块说明
		if preserveEnabled, ok := input.Context["_preserve_enabled"]; ok && preserveEnabled == "true" {
			preserveInstructions = append(preserveInstructions, GetPreservePrompt(DefaultPreserveConfig))
		}
		
		// 如果是批量翻译，添加节点标记保护说明
		if isBatch, ok := input.Context["_is_batch"]; ok && isBatch == "true" {
			preserveInstructions = append(preserveInstructions, GetNodeMarkerPrompt(DefaultNodeMarkerConfig))
		}
		
		// 插入保护说明到合适的位置
		if len(preserveInstructions) > 0 {
			// 查找 "Please translate" 或类似的标记
			markers := []string{
				"Please translate the following text:",
				"Please translate:",
				"Translate the following text:",
				"Translate:",
				"请翻译以下文本：",
				"请翻译：",
			}
			
			inserted := false
			for _, marker := range markers {
				if strings.Contains(prompt, marker) {
					instructions := strings.Join(preserveInstructions, "\n\n")
					prompt = strings.Replace(prompt, marker, instructions + "\n\n" + marker, 1)
					inserted = true
					break
				}
			}
			
			// 如果没找到标记，添加到末尾
			if !inserted {
				for _, instruction := range preserveInstructions {
					prompt = prompt + "\n\n" + instruction
				}
			}
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
