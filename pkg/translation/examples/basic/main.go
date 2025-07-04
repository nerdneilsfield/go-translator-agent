package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// ExampleLLMClient 示例LLM客户端实现
type ExampleLLMClient struct {
	apiKey  string
	baseURL string
	model   string
}

func NewExampleLLMClient(apiKey, baseURL, model string) *ExampleLLMClient {
	return &ExampleLLMClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
	}
}

func (c *ExampleLLMClient) Complete(ctx context.Context, req *translation.CompletionRequest) (*translation.CompletionResponse, error) {
	// 实际实现中，这里会调用真实的API
	// 这里只是示例
	return &translation.CompletionResponse{
		Text:      "Translated: " + req.Prompt,
		Model:     c.model,
		TokensIn:  100,
		TokensOut: 150,
	}, nil
}

func (c *ExampleLLMClient) Chat(ctx context.Context, req *translation.ChatRequest) (*translation.ChatResponse, error) {
	// 实际实现中，这里会调用真实的API
	// 这里只是示例
	var responseText string
	if len(req.Messages) > 0 {
		lastMessage := req.Messages[len(req.Messages)-1]
		responseText = fmt.Sprintf("Translated: %s", lastMessage.Content)
	}

	return &translation.ChatResponse{
		Message: translation.ChatMessage{
			Role:    "assistant",
			Content: responseText,
		},
		Model:     c.model,
		TokensIn:  100,
		TokensOut: 150,
	}, nil
}

func (c *ExampleLLMClient) GetModel() string {
	return c.model
}

func (c *ExampleLLMClient) HealthCheck(ctx context.Context) error {
	// 检查API连接
	return nil
}

// SimpleCache 简单的内存缓存实现
type SimpleCache struct {
	data map[string]string
}

func NewSimpleCache() *SimpleCache {
	return &SimpleCache{
		data: make(map[string]string),
	}
}

func (c *SimpleCache) Get(key string) (string, bool) {
	val, ok := c.data[key]
	return val, ok
}

func (c *SimpleCache) Set(key string, value string) error {
	c.data[key] = value
	return nil
}

func (c *SimpleCache) Delete(key string) error {
	delete(c.data, key)
	return nil
}

func (c *SimpleCache) Clear() error {
	c.data = make(map[string]string)
	return nil
}

func (c *SimpleCache) Stats() translation.CacheStats {
	return translation.CacheStats{
		Hits:       0, // Simple implementation doesn't track stats
		Misses:     0,
		Size:       len(c.data),
		Evictions:  0,
	}
}

func main() {
	// 1. 创建配置
	config := &translation.Config{
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
		ChunkSize:      1000,
		ChunkOverlap:   100,
		MaxConcurrency: 3,
		Steps: []translation.StepConfig{
			{
				Name:        "initial_translation",
				Model:       "gpt-4",
				Temperature: 0.3,
				MaxTokens:   4096,
					AdditionalNotes: "Maintain the original meaning, tone, and style as much as possible.",
			},
			{
				Name:        "reflection",
				Model:       "gpt-4",
				Temperature: 0.1,
				MaxTokens:   2048,
					AdditionalNotes: "Identify any issues with accuracy, fluency, cultural appropriateness, or style.",
			},
			{
				Name:        "improvement",
				Model:       "gpt-4",
				Temperature: 0.3,
				MaxTokens:   4096,
					AdditionalNotes: "Provide an improved translation that addresses the feedback.",
			},
		},
	}

	// 2. 创建LLM客户端
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Println("Warning: OPENAI_API_KEY not set, using mock client")
	}

	llmClient := NewExampleLLMClient(
		apiKey,
		"https://api.openai.com/v1",
		"gpt-4",
	)

	// 3. 创建缓存
	cache := NewSimpleCache()

	// 4. 创建翻译服务
	translator, err := translation.New(config,
		translation.WithLLMClient(llmClient),
		translation.WithCache(cache),
		translation.WithProgressCallback(func(p *translation.Progress) {
			fmt.Printf("Progress: %.2f%% - %s\n", p.Percent, p.Current)
		}),
		translation.WithErrorHandler(func(err error) {
			log.Printf("Translation error: %v\n", err)
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	// 5. 执行翻译
	ctx := context.Background()

	// 示例1：简单翻译
	fmt.Println("=== Example 1: Simple Translation ===")
	result, err := translator.TranslateText(ctx,
"Hello, world! This is a test of the translation service.")
	if err != nil {
		log.Printf("Translation failed: %v\n", err)
	} else {
		fmt.Printf("Original: %s\n", "Hello, world! This is a test of the translation service.")
		fmt.Printf("Translated: %s\n", result)
	}

	// 示例2：带元数据的翻译
	fmt.Println("\n=== Example 2: Translation with Metadata ===")
	result2, err := translator.TranslateText(ctx, "The quick brown fox jumps over the lazy dog.")
	if err != nil {
		log.Printf("Translation failed: %v\n", err)
	} else {
		fmt.Printf("Translated: %s\n", result2)
	}

	// 示例3：批量翻译
	fmt.Println("\n=== Example 3: Batch Translation ===")
	texts := []string{
		"Good morning!",
		"How are you?",
		"Thank you very much.",
	}

	for i, text := range texts {
		result, err := translator.TranslateText(ctx, text)
		if err != nil {
			log.Printf("Translation %d failed: %v\n", i+1, err)
		} else {
			fmt.Printf("%d. %s -> %s\n", i+1, text, result)
		}
	}

	// 示例4：长文本翻译（会自动分块）
	fmt.Println("\n=== Example 4: Long Text Translation ===")
	longText := `Artificial intelligence (AI) is intelligence demonstrated by machines, in contrast to the natural intelligence displayed by humans and animals. Leading AI textbooks define the field as the study of "intelligent agents": any device that perceives its environment and takes actions that maximize its chance of successfully achieving its goals.

The term "artificial intelligence" is often used to describe machines that mimic "cognitive" functions that humans associate with the human mind, such as "learning" and "problem solving". As machines become increasingly capable, tasks considered to require "intelligence" are often removed from the definition of AI, a phenomenon known as the AI effect.

The traditional goals of AI research include reasoning, knowledge representation, planning, learning, natural language processing, perception and the ability to move and manipulate objects. General intelligence is among the field's long-term goals.`

	result4, err := translator.TranslateText(ctx, longText)
	if err != nil {
		log.Printf("Long text translation failed: %v\n", err)
	} else {
		fmt.Printf("Original length: %d characters\n", len(longText))
		fmt.Printf("Translated length: %d characters\n", len(result4))
		if len(result4) > 100 {
			fmt.Printf("Translation preview: %s...\n", result4[:100])
		} else {
			fmt.Printf("Translation: %s\n", result4)
		}
	}

	// 示例5：自定义配置
	fmt.Println("\n=== Example 5: Custom Configuration ===")
	customConfig := &translation.Config{
		SourceLanguage: "Chinese",
		TargetLanguage: "English",
		ChunkSize:      500,
		MaxConcurrency: 5,
		Steps: []translation.StepConfig{
			{
				Name:        "translate",
				Model:       "gpt-3.5-turbo",
				Temperature: 0.5,
				AdditionalNotes: "Simple direct translation without reflection or improvement.",
			},
		},
	}

	simpleTranslator, err := translation.New(customConfig,
		translation.WithLLMClient(llmClient),
	)
	if err != nil {
		log.Fatal(err)
	}

	result5, err := simpleTranslator.TranslateText(ctx, "你好，世界！")
	if err != nil {
		log.Printf("Custom translation failed: %v\n", err)
	} else {
		fmt.Printf("Translated: %s\n", result5)
	}

	fmt.Println("\n=== Translation Examples Complete ===")
}
