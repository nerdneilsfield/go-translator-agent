package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/openai"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

func main() {
	// 获取API密钥
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable not set")
	}

	// 创建配置
	config := openai.DefaultConfigV2()
	config.APIKey = apiKey
	config.Model = "gpt-4" // 使用 GPT-4

	// 示例1：基本翻译
	fmt.Println("=== Example 1: Basic Translation ===")
	basicTranslation(config)

	// 示例2：流式翻译
	fmt.Println("\n=== Example 2: Streaming Translation ===")
	streamingTranslation(config)

	// 示例3：三步翻译流程
	fmt.Println("\n=== Example 3: Three-Step Translation ===")
	threeStepTranslation(config)

	// 示例4：自定义配置
	fmt.Println("\n=== Example 4: Custom Configuration ===")
	customConfiguration()
}

// basicTranslation 基本翻译示例
func basicTranslation(config openai.ConfigV2) {
	provider := openai.NewV2(config)

	req := &translation.ProviderRequest{
		Text:           "The quick brown fox jumps over the lazy dog.",
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
	}

	resp, err := provider.Translate(context.Background(), req)
	if err != nil {
		log.Printf("Translation failed: %v", err)
		return
	}

	fmt.Printf("Original: %s\n", req.Text)
	fmt.Printf("Translated: %s\n", resp.Text)
	fmt.Printf("Model: %s\n", resp.Model)
	fmt.Printf("Tokens: %d in, %d out\n", resp.TokensIn, resp.TokensOut)
}

// streamingTranslation 流式翻译示例
func streamingTranslation(config openai.ConfigV2) {
	provider := openai.NewV2(config)

	req := &translation.ProviderRequest{
		Text: `In the heart of Silicon Valley, a new revolution is brewing. 
		Artificial intelligence is no longer just a buzzword—it's becoming 
		an integral part of our daily lives, transforming how we work, 
		communicate, and solve problems.`,
		SourceLanguage: "English",
		TargetLanguage: "Spanish",
	}

	chunks, err := provider.StreamTranslate(context.Background(), req)
	if err != nil {
		log.Printf("Stream translation failed: %v", err)
		return
	}

	fmt.Println("Streaming translation:")
	fullText := ""
	for chunk := range chunks {
		if chunk.Error != nil {
			log.Printf("Stream error: %v", chunk.Error)
			break
		}
		fmt.Print(chunk.Text)
		fullText += chunk.Text
	}
	fmt.Println("\n\nComplete translation:", fullText)
}

// threeStepTranslation 三步翻译流程示例
func threeStepTranslation(config openai.ConfigV2) {
	// 创建LLM客户端
	llmClient := openai.NewLLMClientV2(config)

	// 创建翻译配置
	translationConfig := &translation.Config{
		SourceLanguage: "English",
		TargetLanguage: "Japanese",
		ChunkSize:      1000,
		MaxConcurrency: 1,
		Steps: []translation.StepConfig{
			{
				Name:        "initial_translation",
				Model:       "gpt-4",
				Temperature: 0.3,
				MaxTokens:   2048,
				Prompt: `Translate the following English text to Japanese. 
Maintain the original meaning, tone, and style as much as possible.

Text to translate:
{{text}}`,
				SystemRole: "You are a professional translator specializing in English to Japanese translation.",
			},
			{
				Name:        "reflection",
				Model:       "gpt-4",
				Temperature: 0.1,
				MaxTokens:   1024,
				Prompt: `Review the following translation from English to Japanese.
Identify any issues with accuracy, fluency, cultural appropriateness, or natural expression.

Original text:
{{original_text}}

Translation:
{{translation}}

Please provide specific feedback on what could be improved.`,
				SystemRole: "You are a Japanese language expert and translation quality reviewer.",
			},
			{
				Name:        "improvement",
				Model:       "gpt-4",
				Temperature: 0.3,
				MaxTokens:   2048,
				Prompt: `Based on the feedback provided, improve the following translation from English to Japanese.

Original text:
{{original_text}}

Current translation:
{{translation}}

Feedback:
{{feedback}}

Please provide an improved translation that addresses the feedback.`,
				SystemRole: "You are a professional translator focusing on creating natural, accurate Japanese translations.",
			},
		},
	}

	// 创建翻译服务
	translator, err := translation.New(translationConfig,
		translation.WithLLMClient(llmClient),
	)
	if err != nil {
		log.Fatalf("Failed to create translator: %v", err)
	}

	// 执行翻译
	text := "Life is like riding a bicycle. To keep your balance, you must keep moving."
	req := &translation.Request{
		Text: text,
	}

	fmt.Printf("Original text: %s\n\n", text)

	resp, err := translator.Translate(context.Background(), req)
	if err != nil {
		log.Printf("Translation failed: %v", err)
		return
	}

	// 显示每个步骤的结果
	for i, step := range resp.Steps {
		fmt.Printf("Step %d - %s:\n", i+1, step.Name)
		fmt.Printf("Output: %s\n", step.Output)
		fmt.Printf("Duration: %v\n\n", step.Duration)
	}

	fmt.Printf("Final Translation: %s\n", resp.Text)
}

// customConfiguration 自定义配置示例
func customConfiguration() {
	config := openai.ConfigV2{
		BaseConfig:  openai.DefaultConfigV2().BaseConfig,
		Model:       "gpt-3.5-turbo-16k", // 使用长上下文模型
		Temperature: 0.1,                 // 更低的温度，更确定性的输出
		MaxTokens:   8000,                // 更大的token限制
	}

	// 设置API密钥
	config.APIKey = os.Getenv("OPENAI_API_KEY")

	// 设置自定义端点（例如使用代理）
	if proxy := os.Getenv("OPENAI_PROXY_URL"); proxy != "" {
		config.APIEndpoint = proxy
	}

	// 设置组织ID（如果有）
	if orgID := os.Getenv("OPENAI_ORG_ID"); orgID != "" {
		config.OrgID = orgID
	}

	provider := openai.NewV2(config)

	// 带自定义指令的翻译
	req := &translation.ProviderRequest{
		Text:           "Quantum computing represents a fundamental shift in how we process information.",
		SourceLanguage: "English",
		TargetLanguage: "French",
		Options: map[string]string{
			"instruction": "Please use formal language suitable for academic publications.",
		},
	}

	resp, err := provider.Translate(context.Background(), req)
	if err != nil {
		log.Printf("Translation failed: %v", err)
		return
	}

	fmt.Printf("Original: %s\n", req.Text)
	fmt.Printf("Translated (formal): %s\n", resp.Text)
	fmt.Printf("Model used: %s\n", resp.Model)
}
