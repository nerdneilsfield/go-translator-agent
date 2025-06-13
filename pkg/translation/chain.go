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
	
	// 执行每个步骤
	for i, step := range c.steps {
		stepResult, err := c.executeStep(ctx, step, currentInput, i)
		result.Steps = append(result.Steps, *stepResult)
		
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
	
	// 如果是反思或改进步骤，添加之前的输出作为上下文
	if index > 0 && len(c.steps) > 1 {
		if index == 1 { // 反思步骤
			stepInput.Context = map[string]string{
				"original_text": c.steps[0].GetConfig().Variables["original_text"],
				"translation":   input,
			}
		} else if index == 2 { // 改进步骤
			stepInput.Context = map[string]string{
				"original_text": c.steps[0].GetConfig().Variables["original_text"],
				"translation":   c.steps[0].GetConfig().Variables["translation"],
				"feedback":      input,
			}
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
		if cached, found, err := s.cache.Get(ctx, cacheKey); err == nil && found {
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
		_ = s.cache.Set(ctx, cacheKey, output.Text)
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
		if cached, found, err := s.cache.Get(ctx, cacheKey); err == nil && found {
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
		_ = s.cache.Set(ctx, cacheKey, output.Text)
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