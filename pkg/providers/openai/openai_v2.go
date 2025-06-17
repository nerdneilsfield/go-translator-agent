package openai

import (
	"context"
	"fmt"

	"github.com/nerdneilsfield/go-translator-agent/pkg/providers"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/retry"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// 辅助函数
func stringPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}

func int64Ptr(i int64) *int64 {
	return &i
}

// getModel 根据字符串获取模型常量
func getModel(model string) openai.ChatModel {
	switch model {
	case "gpt-4":
		return openai.ChatModelGPT4
	case "gpt-4-turbo", "gpt-4-turbo-preview":
		return openai.ChatModelGPT4Turbo
	case "gpt-4o":
		return openai.ChatModelGPT4o
	case "gpt-4o-mini":
		return openai.ChatModelGPT4oMini
	case "gpt-3.5-turbo":
		return openai.ChatModelGPT3_5Turbo
	default:
		// 对于新模型或自定义模型，使用字符串
		return openai.ChatModel(model)
	}
}

// ConfigV2 OpenAI配置（使用官方SDK）
type ConfigV2 struct {
	providers.BaseConfig
	Model       string  `json:"model"`
	Temperature float32 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
	OrgID       string  `json:"org_id,omitempty"` // 可选的组织ID
	RetryConfig retry.RetryConfig `json:"retry_config"`
}

// DefaultConfigV2 返回默认配置
func DefaultConfigV2() ConfigV2 {
	return ConfigV2{
		BaseConfig:  providers.DefaultConfig(),
		Model:       "gpt-3.5-turbo",
		Temperature: 0.3,
		MaxTokens:   4096,
		RetryConfig: retry.DefaultRetryConfig(),
	}
}

// ProviderV2 OpenAI提供商（使用官方SDK）
type ProviderV2 struct {
	config ConfigV2
	client openai.Client
}

// 确保 ProviderV2 实现 providers.TranslationProvider 接口
var _ providers.TranslationProvider = (*ProviderV2)(nil)

// NewV2 创建新的OpenAI提供商（使用官方SDK）
func NewV2(config ConfigV2) *ProviderV2 {
	// 构建客户端选项
	opts := []option.RequestOption{
		option.WithAPIKey(config.APIKey),
	}

	// 添加自定义端点（如果有）
	if config.APIEndpoint != "" {
		opts = append(opts, option.WithBaseURL(config.APIEndpoint))
	}

	// 添加组织ID（如果有）
	if config.OrgID != "" {
		opts = append(opts, option.WithOrganization(config.OrgID))
	}

	// 添加自定义头部
	for k, v := range config.Headers {
		opts = append(opts, option.WithHeader(k, v))
	}

	// 设置超时
	if config.Timeout > 0 {
		opts = append(opts, option.WithRequestTimeout(config.Timeout))
	}

	// 设置重试
	if config.MaxRetries > 0 {
		opts = append(opts, option.WithMaxRetries(config.MaxRetries))
	}

	// 创建客户端
	client := openai.NewClient(opts...)

	return &ProviderV2{
		config: config,
		client: client,
	}
}

// Configure 配置提供商
func (p *ProviderV2) Configure(config interface{}) error {
	cfg, ok := config.(ConfigV2)
	if !ok {
		return fmt.Errorf("invalid config type: expected ConfigV2")
	}
	p.config = cfg
	// 重新创建客户端
	*p = *NewV2(cfg)
	return nil
}

// Translate 执行翻译
func (p *ProviderV2) Translate(ctx context.Context, req *providers.ProviderRequest) (*providers.ProviderResponse, error) {
	// 构建消息
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("You are a professional translator. Translate accurately while preserving the original meaning and tone."),
		openai.UserMessage(fmt.Sprintf("Translate the following text from %s to %s:\n\n%s",
			req.SourceLanguage, req.TargetLanguage, req.Text)),
	}

	// 如果有额外的指令
	if req.Metadata != nil {
		if instruction, ok := req.Metadata["instruction"]; ok {
			if instructionStr, ok := instruction.(string); ok {
				messages[0] = openai.SystemMessage("You are a professional translator. Translate accurately while preserving the original meaning and tone.\n\n" + instructionStr)
			}
		}
	}

	// 创建聊天完成请求
	params := openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    getModel(p.config.Model),
	}

	// 设置可选参数
	if p.config.Temperature > 0 {
		params.Temperature = openai.Float(float64(p.config.Temperature))
	}
	if p.config.MaxTokens > 0 {
		params.MaxTokens = openai.Int(int64(p.config.MaxTokens))
	}

	// 执行请求
	completion, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai chat completion failed: %w", err)
	}

	// 检查响应
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned from OpenAI")
	}

	// 返回响应
	return &providers.ProviderResponse{
		Text:      completion.Choices[0].Message.Content,
		TokensIn:  int(completion.Usage.PromptTokens),
		TokensOut: int(completion.Usage.CompletionTokens),
		Metadata: map[string]interface{}{
			"model":         completion.Model,
			"finish_reason": string(completion.Choices[0].FinishReason),
			"id":            completion.ID,
		},
	}, nil
}

// GetName 获取提供商名称
func (p *ProviderV2) GetName() string {
	return "openai"
}

// SupportsSteps 支持多步骤翻译
func (p *ProviderV2) SupportsSteps() bool {
	return true
}

// GetCapabilities 获取提供商能力
func (p *ProviderV2) GetCapabilities() providers.Capabilities {
	return providers.Capabilities{
		SupportedLanguages: []providers.Language{
			{Code: "en", Name: "English"},
			{Code: "zh", Name: "Chinese"},
			{Code: "ja", Name: "Japanese"},
			{Code: "ko", Name: "Korean"},
			{Code: "es", Name: "Spanish"},
			{Code: "fr", Name: "French"},
			{Code: "de", Name: "German"},
			{Code: "ru", Name: "Russian"},
			{Code: "pt", Name: "Portuguese"},
			{Code: "it", Name: "Italian"},
			{Code: "ar", Name: "Arabic"},
			{Code: "hi", Name: "Hindi"},
			{Code: "th", Name: "Thai"},
			{Code: "vi", Name: "Vietnamese"},
			{Code: "nl", Name: "Dutch"},
			{Code: "pl", Name: "Polish"},
			{Code: "tr", Name: "Turkish"},
			{Code: "he", Name: "Hebrew"},
			{Code: "sv", Name: "Swedish"},
			{Code: "da", Name: "Danish"},
			{Code: "no", Name: "Norwegian"},
			{Code: "fi", Name: "Finnish"},
			// OpenAI支持更多语言
		},
		MaxTextLength:      8000, // 取决于模型
		SupportsBatch:      false,
		SupportsFormatting: true,
		RequiresAPIKey:     true,
		RateLimit: &providers.RateLimit{
			RequestsPerMinute: 60, // 取决于账户类型
		},
	}
}

// HealthCheck 健康检查
func (p *ProviderV2) HealthCheck(ctx context.Context) error {
	// 使用一个简单的完成请求进行健康检查
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Hello"),
		},
		Model:     getModel(p.config.Model),
		MaxTokens: openai.Int(10),
	}

	_, err := p.client.Chat.Completions.New(ctx, params)
	return err
}

// // LLMClientV2 实现 translation.LLMClient 接口（使用官方SDK）
// type LLMClientV2 struct {
// 	provider *ProviderV2
// }
// 
// // NewLLMClientV2 创建LLMClient（使用官方SDK）
// func NewLLMClientV2(config ConfigV2) *LLMClientV2 {
// 	return &LLMClientV2{
// 		provider: NewV2(config),
// 	}
// }
// 
// // Chat 实现 translation.LLMClient 接口
// func (c *LLMClientV2) Chat(ctx context.Context, req *translation.ChatRequest) (*translation.ChatResponse, error) {
// 	// 转换消息格式
// 	messages := make([]openai.ChatCompletionMessageParamUnion, len(req.Messages))
// 	for i, msg := range req.Messages {
// 		switch msg.Role {
// 		case "system":
// 			messages[i] = openai.SystemMessage(msg.Content)
// 		case "user":
// 			messages[i] = openai.UserMessage(msg.Content)
// 		case "assistant":
// 			messages[i] = openai.AssistantMessage(msg.Content)
// 		default:
// 			messages[i] = openai.UserMessage(msg.Content)
// 		}
// 	}
// 
// 	// 创建请求
// 	model := req.Model
// 	if model == "" {
// 		model = c.provider.config.Model
// 	}
// 
// 	params := openai.ChatCompletionNewParams{
// 		Messages: messages,
// 		Model:    getModel(model),
// 	}
// 
// 	// 设置可选参数
// 	if req.Temperature > 0 {
// 		params.Temperature = openai.Float(float64(req.Temperature))
// 	}
// 	if req.MaxTokens > 0 {
// 		params.MaxTokens = openai.Int(int64(req.MaxTokens))
// 	}
// 
// 	// 执行请求
// 	completion, err := c.provider.client.Chat.Completions.New(ctx, params)
// 	if err != nil {
// 		return nil, err
// 	}
// 
// 	if len(completion.Choices) == 0 {
// 		return nil, fmt.Errorf("no choices returned")
// 	}
// 
// 	// 转换响应
// 	return &translation.ChatResponse{
// 		Message: translation.ChatMessage{
// 			Role:    "assistant",
// 			Content: completion.Choices[0].Message.Content,
// 		},
// 		Model:     completion.Model,
// 		TokensIn:  int(completion.Usage.PromptTokens),
// 		TokensOut: int(completion.Usage.CompletionTokens),
// 	}, nil
// }
// 
// // Complete 实现 translation.LLMClient 接口
// func (c *LLMClientV2) Complete(ctx context.Context, req *translation.CompletionRequest) (*translation.CompletionResponse, error) {
// 	// 将completion请求转换为chat请求
// 	chatReq := &translation.ChatRequest{
// 		Messages: []translation.ChatMessage{
// 			{
// 				Role:    "user",
// 				Content: req.Prompt,
// 			},
// 		},
// 		Model:       req.Model,
// 		Temperature: req.Temperature,
// 		MaxTokens:   req.MaxTokens,
// 	}
// 
// 	resp, err := c.Chat(ctx, chatReq)
// 	if err != nil {
// 		return nil, err
// 	}
// 
// 	return &translation.CompletionResponse{
// 		Text:      resp.Message.Content,
// 		Model:     resp.Model,
// 		TokensIn:  resp.TokensIn,
// 		TokensOut: resp.TokensOut,
// 	}, nil
// }
// 
// // GetModel 获取模型
// func (c *LLMClientV2) GetModel() string {
// 	return c.provider.config.Model
// }
// 
// // HealthCheck 健康检查
// func (c *LLMClientV2) HealthCheck(ctx context.Context) error {
// 	return c.provider.HealthCheck(ctx)
// }

// 流式响应支持（可选功能）

// StreamTranslate 流式翻译（返回channel）
func (p *ProviderV2) StreamTranslate(ctx context.Context, req *providers.ProviderRequest) (<-chan StreamChunk, error) {
	// 创建结果channel
	chunks := make(chan StreamChunk)

	// 构建消息
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("You are a professional translator. Translate accurately while preserving the original meaning and tone."),
		openai.UserMessage(fmt.Sprintf("Translate the following text from %s to %s:\n\n%s",
			req.SourceLanguage, req.TargetLanguage, req.Text)),
	}

	// 创建流式请求
	params := openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    getModel(p.config.Model),
	}

	// 设置可选参数
	if p.config.Temperature > 0 {
		params.Temperature = openai.Float(float64(p.config.Temperature))
	}
	if p.config.MaxTokens > 0 {
		params.MaxTokens = openai.Int(int64(p.config.MaxTokens))
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, params)

	// 在goroutine中处理流
	go func() {
		defer close(chunks)

		for stream.Next() {
			chunk := stream.Current()
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				select {
				case chunks <- StreamChunk{
					Text:  chunk.Choices[0].Delta.Content,
					Model: chunk.Model,
				}:
				case <-ctx.Done():
					return
				}
			}
		}

		if err := stream.Err(); err != nil {
			chunks <- StreamChunk{Error: err}
		}
	}()

	return chunks, nil
}

// StreamChunk 流式响应块
type StreamChunk struct {
	Text  string
	Model string
	Error error
}
