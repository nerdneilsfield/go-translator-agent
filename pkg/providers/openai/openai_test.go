package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

func TestProvider_Translate(t *testing.T) {
	// 创建模拟服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		
		// 验证认证头
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			t.Errorf("unexpected auth header: %s", auth)
		}
		
		// 返回模拟响应
		resp := ChatResponse{
			ID:    "test-id",
			Model: "gpt-3.5-turbo",
		}
		resp.Choices = []struct {
			Index        int     `json:"index"`
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		}{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: "你好，世界！",
				},
				FinishReason: "stop",
			},
		}
		resp.Usage.PromptTokens = 10
		resp.Usage.CompletionTokens = 5
		resp.Usage.TotalTokens = 15
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()
	
	// 创建配置
	config := DefaultConfig()
	config.APIKey = "test-api-key"
	config.APIEndpoint = server.URL
	
	// 创建提供商
	provider := New(config)
	
	// 测试翻译
	req := &translation.ProviderRequest{
		Text:           "Hello, world!",
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
	}
	
	resp, err := provider.Translate(context.Background(), req)
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}
	
	// 验证响应
	if resp.Text != "你好，世界！" {
		t.Errorf("unexpected translation: %s", resp.Text)
	}
	if resp.Model != "gpt-3.5-turbo" {
		t.Errorf("unexpected model: %s", resp.Model)
	}
	if resp.TokensIn != 10 {
		t.Errorf("unexpected tokens in: %d", resp.TokensIn)
	}
	if resp.TokensOut != 5 {
		t.Errorf("unexpected tokens out: %d", resp.TokensOut)
	}
}

func TestProvider_GetCapabilities(t *testing.T) {
	provider := New(DefaultConfig())
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

func TestProvider_ErrorHandling(t *testing.T) {
	// 创建返回错误的模拟服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 返回API错误
		apiErr := APIError{
			ErrorInfo: struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			}{
				Message: "Invalid API key",
				Type:    "invalid_request_error",
				Code:    "invalid_api_key",
			},
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(apiErr)
	}))
	defer server.Close()
	
	// 创建配置
	config := DefaultConfig()
	config.APIKey = "invalid-key"
	config.APIEndpoint = server.URL
	config.MaxRetries = 0 // 不重试
	
	// 创建提供商
	provider := New(config)
	
	// 测试翻译
	req := &translation.ProviderRequest{
		Text:           "Hello",
		SourceLanguage: "en",
		TargetLanguage: "zh",
	}
	
	_, err := provider.Translate(context.Background(), req)
	if err == nil {
		t.Fatal("expected error but got none")
	}
	
	// 验证错误消息
	if err.Error() != "Invalid API key" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLLMClient_Chat(t *testing.T) {
	// 创建模拟服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 返回模拟响应
		resp := ChatResponse{
			ID:    "test-id",
			Model: "gpt-4",
		}
		resp.Choices = []struct {
			Index        int     `json:"index"`
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		}{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: "Test response",
				},
				FinishReason: "stop",
			},
		}
		resp.Usage.PromptTokens = 20
		resp.Usage.CompletionTokens = 10
		resp.Usage.TotalTokens = 30
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()
	
	// 创建配置
	config := DefaultConfig()
	config.APIKey = "test-api-key"
	config.APIEndpoint = server.URL
	
	// 创建LLMClient
	client := NewLLMClient(config)
	
	// 测试Chat
	req := &translation.ChatRequest{
		Messages: []translation.ChatMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
		Model: "gpt-4",
	}
	
	resp, err := client.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	
	// 验证响应
	if resp.Message.Content != "Test response" {
		t.Errorf("unexpected content: %s", resp.Message.Content)
	}
	if resp.Model != "gpt-4" {
		t.Errorf("unexpected model: %s", resp.Model)
	}
}

func TestLLMClient_Complete(t *testing.T) {
	// 创建模拟服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 返回模拟响应
		resp := ChatResponse{
			ID:    "test-id",
			Model: "gpt-3.5-turbo",
		}
		resp.Choices = []struct {
			Index        int     `json:"index"`
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		}{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: "Completed text",
				},
				FinishReason: "stop",
			},
		}
		resp.Usage.PromptTokens = 15
		resp.Usage.CompletionTokens = 8
		resp.Usage.TotalTokens = 23
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()
	
	// 创建配置
	config := DefaultConfig()
	config.APIKey = "test-api-key"
	config.APIEndpoint = server.URL
	
	// 创建LLMClient
	client := NewLLMClient(config)
	
	// 测试Complete
	req := &translation.CompletionRequest{
		Prompt: "Complete this",
		Model:  "gpt-3.5-turbo",
	}
	
	resp, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("completion failed: %v", err)
	}
	
	// 验证响应
	if resp.Text != "Completed text" {
		t.Errorf("unexpected text: %s", resp.Text)
	}
	if resp.Model != "gpt-3.5-turbo" {
		t.Errorf("unexpected model: %s", resp.Model)
	}
}