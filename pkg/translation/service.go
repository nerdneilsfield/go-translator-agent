package translation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// service 翻译服务实现
type service struct {
	config  *Config
	options serviceOptions
	chain   Chain
	mu      sync.RWMutex
}

// New 创建新的翻译服务
func New(config *Config, opts ...Option) (Service, error) {
	// 检查配置是否为nil
	if config == nil {
		return nil, WrapError(ErrInvalidConfig, ErrCodeConfig, "config is nil")
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, WrapError(err, ErrCodeConfig, fmt.Sprintf("invalid configuration: %v", err))
	}

	// 应用选项
	options := serviceOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	// 检查必要的依赖
	if options.llmClient == nil {
		return nil, ErrNoLLMClient
	}

	// 如果没有提供分块器，使用默认的
	if options.chunker == nil {
		options.chunker = NewSmartChunker(config.ChunkSize, config.ChunkOverlap)
	}

	// 创建服务实例
	s := &service{
		config:  config.Clone(),
		options: options,
	}

	// 构建翻译链
	if err := s.buildChain(); err != nil {
		return nil, err
	}

	return s, nil
}

// buildChain 构建翻译链
func (s *service) buildChain() error {
	s.chain = NewChain()

	// 为每个配置的步骤创建 Step
	for _, stepConfig := range s.config.Steps {
		// 复制配置以避免共享
		cfg := stepConfig

		// 设置默认变量
		if cfg.Variables == nil {
			cfg.Variables = make(map[string]string)
		}
		cfg.Variables["source_language"] = s.config.SourceLanguage
		cfg.Variables["target_language"] = s.config.TargetLanguage

		// 根据配置选择使用提供商还是 LLM
		var step Step
		if cfg.Provider != "" && s.options.providers != nil {
			// 使用指定的提供商
			if provider, ok := s.options.providers[cfg.Provider]; ok {
				step = NewProviderStep(&cfg, provider, s.options.cache)
			} else {
				// 如果找不到指定的提供商，回退到 LLM
				step = NewStep(&cfg, s.options.llmClient, s.options.cache)
			}
		} else {
			// 使用默认的 LLM
			step = NewStep(&cfg, s.options.llmClient, s.options.cache)
		}

		s.chain.AddStep(step)
	}

	return nil
}

// Translate 执行完整的翻译流程
func (s *service) Translate(ctx context.Context, req *Request) (*Response, error) {
	// 验证请求
	if req == nil || req.Text == "" {
		return nil, ErrEmptyText
	}

	// 执行前置钩子
	if s.options.beforeTranslate != nil {
		s.options.beforeTranslate(req)
	}

	// 开始计时
	startTime := time.Now()
	translationID := uuid.New().String()

	// 准备响应
	resp := &Response{
		SourceLanguage: s.config.SourceLanguage,
		TargetLanguage: s.config.TargetLanguage,
		Metadata:       make(map[string]interface{}),
	}

	// 覆盖语言设置（如果请求中指定）
	if req.SourceLanguage != "" {
		resp.SourceLanguage = req.SourceLanguage
	}
	if req.TargetLanguage != "" {
		resp.TargetLanguage = req.TargetLanguage
	}

	// 复制元数据
	if req.Metadata != nil {
		for k, v := range req.Metadata {
			resp.Metadata[k] = v
		}
	}

	// 文本分块
	chunks := s.options.chunker.Chunk(req.Text)
	chunkCount := len(chunks)

	// 初始化进度跟踪
	if s.options.progressTracker != nil {
		s.options.progressTracker.Start(chunkCount)
		defer s.options.progressTracker.Complete()
	}

	// 处理每个块
	var translatedChunks []string
	var totalTokensIn, totalTokensOut int
	var allSteps []StepResult

	for i, chunk := range chunks {
		// 更新进度
		if s.options.progressTracker != nil {
			s.options.progressTracker.Update(i, fmt.Sprintf("Processing chunk %d/%d", i+1, chunkCount))
		}
		if s.options.progressCallback != nil {
			s.options.progressCallback(&Progress{
				Total:     chunkCount,
				Completed: i,
				Current:   fmt.Sprintf("chunk_%d", i+1),
				Percent:   float64(i) / float64(chunkCount) * 100,
			})
		}

		// 执行翻译链
		chainResult, err := s.chain.Execute(ctx, chunk)
		if err != nil {
			if s.options.errorHandler != nil {
				s.options.errorHandler(err)
			}
			if s.options.progressTracker != nil {
				s.options.progressTracker.Error(err)
			}
			return nil, WrapError(err, ErrCodeChain, fmt.Sprintf("failed to process chunk %d", i+1))
		}

		// 收集结果
		translatedChunks = append(translatedChunks, chainResult.FinalOutput)

		// 收集步骤信息（只记录第一个块的步骤作为示例）
		if i == 0 {
			allSteps = chainResult.Steps
		}

		// 统计 token
		for _, step := range chainResult.Steps {
			totalTokensIn += step.TokensIn
			totalTokensOut += step.TokensOut
		}
	}

	// 合并翻译结果
	resp.Text = s.mergeChunks(translatedChunks)
	resp.Steps = allSteps

	// 创建指标
	metrics := &TranslationMetrics{
		ID:             translationID,
		StartTime:      startTime,
		EndTime:        time.Now(),
		Duration:       time.Since(startTime),
		SourceLanguage: resp.SourceLanguage,
		TargetLanguage: resp.TargetLanguage,
		InputLength:    len(req.Text),
		OutputLength:   len(resp.Text),
		TotalTokensIn:  totalTokensIn,
		TotalTokensOut: totalTokensOut,
		Success:        true,
		ChunkCount:     chunkCount,
	}

	// 记录指标
	if s.options.metricsCollector != nil {
		s.options.metricsCollector.RecordTranslation(metrics)
	}

	resp.Metrics = metrics

	// 执行后置钩子
	if s.options.afterTranslate != nil {
		s.options.afterTranslate(resp)
	}

	// 最终进度更新
	if s.options.progressCallback != nil {
		s.options.progressCallback(&Progress{
			Total:     chunkCount,
			Completed: chunkCount,
			Current:   "completed",
			Percent:   100,
		})
	}

	return resp, nil
}

// TranslateBatch 批量翻译
func (s *service) TranslateBatch(ctx context.Context, reqs []*Request) ([]*Response, error) {
	if len(reqs) == 0 {
		return []*Response{}, nil
	}

	// 使用工作池进行并发翻译
	type result struct {
		index int
		resp  *Response
		err   error
	}

	resultChan := make(chan result, len(reqs))
	var wg sync.WaitGroup

	// 限制并发数
	semaphore := make(chan struct{}, s.config.MaxConcurrency)

	for i, req := range reqs {
		wg.Add(1)
		go func(idx int, r *Request) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 执行翻译
			resp, err := s.Translate(ctx, r)
			resultChan <- result{
				index: idx,
				resp:  resp,
				err:   err,
			}
		}(i, req)
	}

	// 等待所有翻译完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	responses := make([]*Response, len(reqs))
	var firstError error

	for res := range resultChan {
		if res.err != nil && firstError == nil {
			firstError = res.err
		}
		responses[res.index] = res.resp
	}

	return responses, firstError
}

// GetConfig 获取当前配置
func (s *service) GetConfig() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.Clone()
}

// mergeChunks 合并翻译块
func (s *service) mergeChunks(chunks []string) string {
	if len(chunks) == 0 {
		return ""
	}
	if len(chunks) == 1 {
		return chunks[0]
	}

	// 简单地用换行符连接
	// 在实际使用中，可能需要更智能的合并策略
	var result string
	for i, chunk := range chunks {
		if i > 0 {
			// 检查是否需要添加分隔符
			if !endsWithPunctuation(chunks[i-1]) && !startsWithPunctuation(chunk) {
				result += " "
			}
		}
		result += chunk
	}

	return result
}

// endsWithPunctuation 检查字符串是否以标点符号结尾
func endsWithPunctuation(s string) bool {
	if s == "" {
		return false
	}
	lastRune := rune(s[len(s)-1])
	return isPunctuation(lastRune)
}

// startsWithPunctuation 检查字符串是否以标点符号开头
func startsWithPunctuation(s string) bool {
	if s == "" {
		return false
	}
	firstRune := rune(s[0])
	return isPunctuation(firstRune)
}

// isPunctuation 检查是否是标点符号
func isPunctuation(r rune) bool {
	punctuations := ".!?,;:。！？，；："
	for _, p := range punctuations {
		if r == p {
			return true
		}
	}
	return false
}
