package translation

import (
	"context"
)

// llmClientAdapter 将 LLMClient 适配为 TranslationProvider
type llmClientAdapter struct {
	client LLMClient
	name   string
}

// NewLLMClientAdapter 创建 LLMClient 适配器
func NewLLMClientAdapter(client LLMClient) TranslationProvider {
	return &llmClientAdapter{
		client: client,
		name:   "llm-" + client.GetModel(),
	}
}

// Translate 实现 TranslationProvider 接口
func (a *llmClientAdapter) Translate(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	// 简单翻译，不使用三步流程
	chatReq := &ChatRequest{
		Messages: []ChatMessage{
			{
				Role:    "system",
				Content: "You are a professional translator.",
			},
			{
				Role:    "user",
				Content: "Translate the following text from " + req.SourceLanguage + " to " + req.TargetLanguage + ":\n\n" + req.Text,
			},
		},
		Model: a.client.GetModel(),
	}

	resp, err := a.client.Chat(ctx, chatReq)
	if err != nil {
		return nil, WrapError(err, ErrCodeLLM, "provider '"+a.client.GetModel()+"' translation failed")
	}

	return &ProviderResponse{
		Text:      resp.Message.Content,
		TokensIn:  resp.TokensIn,
		TokensOut: resp.TokensOut,
		Metadata: map[string]interface{}{
			"model": resp.Model,
		},
	}, nil
}

// GetName 获取提供商名称
func (a *llmClientAdapter) GetName() string {
	return a.name
}

// SupportsSteps 支持多步骤翻译
func (a *llmClientAdapter) SupportsSteps() bool {
	return true
}

// WithProvider 使用翻译提供商（未来的选项函数）
func WithProvider(provider TranslationProvider) Option {
	return func(o *serviceOptions) {
		// 如果提供商支持步骤，包装为 LLMClient
		// TODO: 实现providerLLMClient或直接使用provider
		if provider.SupportsSteps() {
			// o.llmClient = &providerLLMClient{provider: provider}
		}
		// 		// 未来可以直接使用 provider
		// 	}
		// }
		//
		// // providerLLMClient 将 TranslationProvider 包装为 LLMClient
		// type providerLLMClient struct {
		// 	provider TranslationProvider
		// }
		//
		// func (p *providerLLMClient) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
		// 	provReq := &ProviderRequest{
		// 		Text: req.Prompt,
		// 	}
		//
		// 	resp, err := p.provider.Translate(ctx, provReq)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		//
		// 	return &CompletionResponse{
		// 		Text:      resp.Text,
		// 		Model:     resp.Model,
		// 		TokensIn:  resp.TokensIn,
		// 		TokensOut: resp.TokensOut,
		// 	}, nil
		// }
		//
		// func (p *providerLLMClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
		// 	// 将最后一条消息作为翻译内容
		// 	if len(req.Messages) == 0 {
		// 		return nil, ErrEmptyText
		// 	}
		//
		// 	lastMessage := req.Messages[len(req.Messages)-1]
		// 	provReq := &ProviderRequest{
		// 		Text: lastMessage.Content,
		// 	}
		//
		// 	resp, err := p.provider.Translate(ctx, provReq)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		//
		// 	return &ChatResponse{
		// 		Message: ChatMessage{
		// 			Role:    "assistant",
		// 			Content: resp.Text,
		// 		},
		// 		Model:     resp.Model,
		// 		TokensIn:  resp.TokensIn,
		// 		TokensOut: resp.TokensOut,
		// 	}, nil
		// }
		//
		// func (p *providerLLMClient) GetModel() string {
		// 	return p.provider.GetName()
		// }
		//
		// func (p *providerLLMClient) HealthCheck(ctx context.Context) error {
		// 	// 简单测试翻译
		// 	_, err := p.provider.Translate(ctx, &ProviderRequest{
		// 		Text:           "test",
		// 		SourceLanguage: "en",
		// 		TargetLanguage: "zh",
		// 	})
		// 	return err
		// }
	}
}
