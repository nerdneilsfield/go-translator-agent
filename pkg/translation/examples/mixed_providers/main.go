package main

import (
	"context"
	"fmt"
	"log"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// DeepLProvider 模拟 DeepL 提供商
type DeepLProvider struct {
	apiKey string
}

func (d *DeepLProvider) Translate(ctx context.Context, req *translation.ProviderRequest) (*translation.ProviderResponse, error) {
	// 这里应该调用真实的 DeepL API
	// 模拟 DeepL 的高质量直接翻译
	translatedText := fmt.Sprintf("[DeepL] %s -> %s: %s", 
		req.SourceLanguage, 
		req.TargetLanguage,
		"这是由 DeepL 翻译的高质量文本。",
	)
	
	return &translation.ProviderResponse{
		Text:  translatedText,
		Model: "deepl-pro",
	}, nil
}

func (d *DeepLProvider) GetName() string {
	return "deepl"
}

func (d *DeepLProvider) SupportsSteps() bool {
	return false // DeepL 不需要多步骤
}

// GoogleTranslateProvider 模拟 Google Translate 提供商
type GoogleTranslateProvider struct {
	apiKey string
}

func (g *GoogleTranslateProvider) Translate(ctx context.Context, req *translation.ProviderRequest) (*translation.ProviderResponse, error) {
	// 这里应该调用真实的 Google Translate API
	translatedText := fmt.Sprintf("[Google] %s -> %s: %s", 
		req.SourceLanguage, 
		req.TargetLanguage,
		"这是由 Google 翻译的文本。",
	)
	
	return &translation.ProviderResponse{
		Text:  translatedText,
		Model: "google-translate",
	}, nil
}

func (g *GoogleTranslateProvider) GetName() string {
	return "google"
}

func (g *GoogleTranslateProvider) SupportsSteps() bool {
	return false
}

// OpenAIProvider 模拟 OpenAI LLM 提供商
type OpenAIProvider struct {
	apiKey string
}

func (o *OpenAIProvider) Translate(ctx context.Context, req *translation.ProviderRequest) (*translation.ProviderResponse, error) {
	// 模拟 OpenAI 的响应，可以处理复杂的提示词
	var response string
	
	// 根据请求中的选项判断是哪个步骤
	if stepType, ok := req.Options["step_type"]; ok {
		switch stepType {
		case "reflection":
			response = "经过分析，翻译基本准确，但有以下改进建议：1. 某些专业术语可以更精确 2. 语气可以更自然"
		case "improvement":
			response = "这是基于反馈改进后的最终翻译版本，更加准确和自然。"
		default:
			response = "这是 OpenAI 的翻译结果。"
		}
	} else {
		response = "这是 OpenAI 的翻译结果。"
	}
	
	return &translation.ProviderResponse{
		Text:      response,
		Model:     "gpt-4",
		TokensIn:  100,
		TokensOut: 150,
	}, nil
}

func (o *OpenAIProvider) GetName() string {
	return "openai"
}

func (o *OpenAIProvider) SupportsSteps() bool {
	return true
}

func main() {
	// 创建不同的提供商
	deepl := &DeepLProvider{apiKey: "deepl-api-key"}
	google := &GoogleTranslateProvider{apiKey: "google-api-key"}
	openai := &OpenAIProvider{apiKey: "openai-api-key"}

	// 配置1：使用 DeepL 进行初始翻译，OpenAI 进行反思和改进
	fmt.Println("=== 示例1: DeepL + OpenAI 组合 ===")
	config1 := &translation.Config{
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
		ChunkSize:      1000,
		Steps: []translation.StepConfig{
			{
				Name:     "initial_translation",
				Provider: "deepl", // 使用 DeepL
				Prompt:   "not used for deepl",
			},
			{
				Name:     "reflection",
				Provider: "openai", // 使用 OpenAI
				Model:    "gpt-4",
				Prompt:   "Review this translation and provide feedback:\n\nOriginal: {{original_text}}\nTranslation: {{translation}}",
				Variables: map[string]string{
					"step_type": "reflection",
				},
			},
			{
				Name:     "improvement",
				Provider: "openai", // 使用 OpenAI
				Model:    "gpt-4",
				Prompt:   "Improve the translation based on feedback:\n\nOriginal: {{original_text}}\nCurrent: {{translation}}\nFeedback: {{feedback}}",
				Variables: map[string]string{
					"step_type": "improvement",
				},
			},
		},
	}

	translator1, err := translation.New(config1,
		translation.WithProviders(map[string]translation.TranslationProvider{
			"deepl":  deepl,
			"openai": openai,
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	result1, err := translator1.Translate(context.Background(), &translation.Request{
		Text: "Hello, world! This is a test of mixed translation providers.",
	})
	if err != nil {
		log.Printf("Translation error: %v", err)
	} else {
		fmt.Printf("最终翻译结果: %s\n", result1.Text)
		fmt.Println("\n各步骤结果:")
		for _, step := range result1.Steps {
			fmt.Printf("- %s: %s\n", step.Name, step.Output)
		}
	}

	// 配置2：使用 Google Translate 进行快速翻译（单步骤）
	fmt.Println("\n=== 示例2: Google Translate 快速翻译 ===")
	config2 := &translation.Config{
		SourceLanguage: "English",
		TargetLanguage: "Spanish",
		ChunkSize:      1000,
		Steps: []translation.StepConfig{
			{
				Name:     "translation",
				Provider: "google",
			},
		},
	}

	translator2, err := translation.New(config2,
		translation.WithSingleProvider("google", google),
	)
	if err != nil {
		log.Fatal(err)
	}

	result2, err := translator2.Translate(context.Background(), &translation.Request{
		Text: "Quick translation example.",
	})
	if err != nil {
		log.Printf("Translation error: %v", err)
	} else {
		fmt.Printf("翻译结果: %s\n", result2.Text)
	}

	// 配置3：仅使用 LLM 的传统三步翻译
	fmt.Println("\n=== 示例3: 纯 OpenAI 三步翻译 ===")
	
	// 为了兼容性，创建一个 LLMClient 适配器
	llmClient := &MockLLMClient{provider: openai}
	
	config3 := translation.DefaultConfig()
	translator3, err := translation.New(config3,
		translation.WithLLMClient(llmClient),
	)
	if err != nil {
		log.Fatal(err)
	}

	result3, err := translator3.Translate(context.Background(), &translation.Request{
		Text: "Traditional three-step translation.",
	})
	if err != nil {
		log.Printf("Translation error: %v", err)
	} else {
		fmt.Printf("翻译结果: %s\n", result3.Text)
	}
}

// MockLLMClient 将 Provider 适配为 LLMClient
type MockLLMClient struct {
	provider translation.TranslationProvider
}

func (m *MockLLMClient) Complete(ctx context.Context, req *translation.CompletionRequest) (*translation.CompletionResponse, error) {
	resp, err := m.provider.Translate(ctx, &translation.ProviderRequest{
		Text: req.Prompt,
	})
	if err != nil {
		return nil, err
	}
	return &translation.CompletionResponse{
		Text:      resp.Text,
		Model:     resp.Model,
		TokensIn:  resp.TokensIn,
		TokensOut: resp.TokensOut,
	}, nil
}

func (m *MockLLMClient) Chat(ctx context.Context, req *translation.ChatRequest) (*translation.ChatResponse, error) {
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("no messages")
	}
	
	lastMsg := req.Messages[len(req.Messages)-1]
	resp, err := m.provider.Translate(ctx, &translation.ProviderRequest{
		Text: lastMsg.Content,
	})
	if err != nil {
		return nil, err
	}
	
	return &translation.ChatResponse{
		Message: translation.ChatMessage{
			Role:    "assistant",
			Content: resp.Text,
		},
		Model:     resp.Model,
		TokensIn:  resp.TokensIn,
		TokensOut: resp.TokensOut,
	}, nil
}

func (m *MockLLMClient) GetModel() string {
	return m.provider.GetName()
}

func (m *MockLLMClient) HealthCheck(ctx context.Context) error {
	return nil
}