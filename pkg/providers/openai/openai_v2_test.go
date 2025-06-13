package openai

import (
	"context"
	"testing"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

func TestProviderV2_Translate(t *testing.T) {
	// 跳过需要真实API密钥的测试
	apiKey := "test-api-key"
	if apiKey == "test-api-key" {
		t.Skip("Skipping test that requires real OpenAI API key")
	}
	
	// 创建配置
	config := DefaultConfigV2()
	config.APIKey = apiKey
	
	// 创建提供商
	provider := NewV2(config)
	
	// 测试翻译
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	req := &translation.ProviderRequest{
		Text:           "Hello, world!",
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
	}
	
	resp, err := provider.Translate(ctx, req)
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}
	
	// 验证响应
	if resp.Text == "" {
		t.Error("empty translation")
	}
	if resp.Model == "" {
		t.Error("empty model")
	}
	
	t.Logf("Translation: %s", resp.Text)
	t.Logf("Model: %s", resp.Model)
	t.Logf("Tokens: %d in, %d out", resp.TokensIn, resp.TokensOut)
}

func TestProviderV2_GetCapabilities(t *testing.T) {
	provider := NewV2(DefaultConfigV2())
	caps := provider.GetCapabilities()
	
	// 验证能力
	if !caps.RequiresAPIKey {
		t.Error("should require API key")
	}
	if !caps.SupportsFormatting {
		t.Error("should support formatting")
	}
	if caps.SupportsBatch {
		t.Error("should not support batch")
	}
	if len(caps.SupportedLanguages) == 0 {
		t.Error("should have supported languages")
	}
}

func TestLLMClientV2_Chat(t *testing.T) {
	// 跳过需要真实API密钥的测试
	apiKey := "test-api-key"
	if apiKey == "test-api-key" {
		t.Skip("Skipping test that requires real OpenAI API key")
	}
	
	// 创建配置
	config := DefaultConfigV2()
	config.APIKey = apiKey
	
	// 创建LLMClient
	client := NewLLMClientV2(config)
	
	// 测试Chat
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	req := &translation.ChatRequest{
		Messages: []translation.ChatMessage{
			{
				Role:    "system",
				Content: "You are a helpful assistant.",
			},
			{
				Role:    "user",
				Content: "Say hello in Chinese",
			},
		},
		Model:       "gpt-3.5-turbo",
		Temperature: 0.7,
		MaxTokens:   100,
	}
	
	resp, err := client.Chat(ctx, req)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	
	// 验证响应
	if resp.Message.Content == "" {
		t.Error("empty response")
	}
	if resp.Model == "" {
		t.Error("empty model")
	}
	
	t.Logf("Response: %s", resp.Message.Content)
	t.Logf("Model: %s", resp.Model)
	t.Logf("Tokens: %d in, %d out", resp.TokensIn, resp.TokensOut)
}

func TestProviderV2_StreamTranslate(t *testing.T) {
	// 跳过需要真实API密钥的测试
	apiKey := "test-api-key"
	if apiKey == "test-api-key" {
		t.Skip("Skipping test that requires real OpenAI API key")
	}
	
	// 创建配置
	config := DefaultConfigV2()
	config.APIKey = apiKey
	
	// 创建提供商
	provider := NewV2(config)
	
	// 测试流式翻译
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	req := &translation.ProviderRequest{
		Text:           "Hello, world! This is a streaming test.",
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
	}
	
	chunks, err := provider.StreamTranslate(ctx, req)
	if err != nil {
		t.Fatalf("stream translation failed: %v", err)
	}
	
	// 收集所有块
	var fullText string
	for chunk := range chunks {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		fullText += chunk.Text
		t.Logf("Chunk: %s", chunk.Text)
	}
	
	if fullText == "" {
		t.Error("empty translation")
	}
	
	t.Logf("Full translation: %s", fullText)
}

// 基准测试
func BenchmarkProviderV2_Translate(b *testing.B) {
	// 需要真实API密钥
	apiKey := "test-api-key"
	if apiKey == "test-api-key" {
		b.Skip("Skipping benchmark that requires real OpenAI API key")
	}
	
	config := DefaultConfigV2()
	config.APIKey = apiKey
	provider := NewV2(config)
	
	req := &translation.ProviderRequest{
		Text:           "Hello, world!",
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
	}
	
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := provider.Translate(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}