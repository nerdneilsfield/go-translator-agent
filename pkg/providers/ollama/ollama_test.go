package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/pkg/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	
	assert.Equal(t, "llama2", config.Model)
	assert.Equal(t, float32(0.3), config.Temperature)
	assert.Equal(t, 4096, config.MaxTokens)
	assert.False(t, config.Stream)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, time.Second, config.RetryDelay)
}

func TestNew(t *testing.T) {
	config := DefaultConfig()
	provider := New(config)
	
	assert.NotNil(t, provider)
	assert.Equal(t, "http://localhost:11434", provider.config.APIEndpoint)
	assert.NotNil(t, provider.httpClient)
}

func TestNewWithCustomEndpoint(t *testing.T) {
	config := DefaultConfig()
	config.APIEndpoint = "http://custom-ollama:8080"
	
	provider := New(config)
	
	assert.Equal(t, "http://custom-ollama:8080", provider.config.APIEndpoint)
}

func TestConfigure(t *testing.T) {
	provider := New(DefaultConfig())
	
	newConfig := Config{
		BaseConfig: providers.BaseConfig{
			APIEndpoint: "http://new-endpoint:11434",
			Timeout:     60 * time.Second,
		},
		Model:       "mistral",
		Temperature: 0.5,
		MaxTokens:   2048,
	}
	
	err := provider.Configure(newConfig)
	require.NoError(t, err)
	
	assert.Equal(t, "http://new-endpoint:11434", provider.config.APIEndpoint)
	assert.Equal(t, "mistral", provider.config.Model)
	assert.Equal(t, float32(0.5), provider.config.Temperature)
	assert.Equal(t, 2048, provider.config.MaxTokens)
}

func TestConfigureInvalidType(t *testing.T) {
	provider := New(DefaultConfig())
	
	err := provider.Configure("invalid-config")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config type")
}

func TestGetName(t *testing.T) {
	provider := New(DefaultConfig())
	assert.Equal(t, "ollama", provider.GetName())
}

func TestSupportsSteps(t *testing.T) {
	provider := New(DefaultConfig())
	assert.True(t, provider.SupportsSteps())
}

func TestGetCapabilities(t *testing.T) {
	provider := New(DefaultConfig())
	capabilities := provider.GetCapabilities()
	
	assert.True(t, len(capabilities.SupportedLanguages) > 0)
	assert.Contains(t, capabilities.SupportedLanguages, providers.Language{Code: "en", Name: "English"})
	assert.Contains(t, capabilities.SupportedLanguages, providers.Language{Code: "zh", Name: "Chinese"})
	assert.Equal(t, 8000, capabilities.MaxTextLength)
	assert.False(t, capabilities.SupportsBatch)
	assert.True(t, capabilities.SupportsFormatting)
	assert.False(t, capabilities.RequiresAPIKey)
	assert.NotNil(t, capabilities.RateLimit)
	assert.Equal(t, 60, capabilities.RateLimit.RequestsPerMinute)
}

func TestTranslate(t *testing.T) {
	// 创建模拟服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/generate", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		
		// 验证请求体
		var req GenerateRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		
		assert.Equal(t, "llama2", req.Model)
		assert.Contains(t, req.Prompt, "Hello, world!")
		assert.Contains(t, req.Prompt, "English")
		assert.Contains(t, req.Prompt, "Chinese")
		assert.False(t, req.Stream)
		
		// 模拟响应
		response := GenerateResponse{
			Model:              "llama2",
			CreatedAt:          time.Now(),
			Response:           "你好，世界！",
			Done:               true,
			TotalDuration:      1000000000, // 1秒
			LoadDuration:       100000000,  // 0.1秒
			PromptEvalCount:    10,
			PromptEvalDuration: 200000000, // 0.2秒
			EvalCount:          5,
			EvalDuration:       300000000, // 0.3秒
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	
	// 配置provider使用模拟服务器
	config := DefaultConfig()
	config.APIEndpoint = server.URL
	provider := New(config)
	
	// 执行翻译
	req := &providers.ProviderRequest{
		Text:           "Hello, world!",
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
	}
	
	resp, err := provider.Translate(context.Background(), req)
	require.NoError(t, err)
	
	assert.Equal(t, "你好，世界！", resp.Text)
	assert.Equal(t, 10, resp.TokensIn)
	assert.Equal(t, 5, resp.TokensOut)
	assert.NotNil(t, resp.Metadata)
	assert.Equal(t, "llama2", resp.Metadata["model"])
	assert.NotNil(t, resp.Metadata["created_at"])
	assert.Equal(t, int64(1000000000), resp.Metadata["total_duration"])
}

func TestTranslateWithMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req GenerateRequest
		json.NewDecoder(r.Body).Decode(&req)
		
		// 验证自定义指令被添加到提示中
		assert.Contains(t, req.Prompt, "Custom instruction:")
		assert.Contains(t, req.Prompt, "Be very careful with technical terms")
		
		response := GenerateResponse{
			Model:    "llama2",
			Response: "翻译结果",
			Done:     true,
		}
		
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	
	config := DefaultConfig()
	config.APIEndpoint = server.URL
	provider := New(config)
	
	req := &providers.ProviderRequest{
		Text:           "Test text",
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
		Metadata: map[string]interface{}{
			"instruction": "Custom instruction: Be very careful with technical terms",
		},
	}
	
	_, err := provider.Translate(context.Background(), req)
	require.NoError(t, err)
}

func TestTranslateWithMaxTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req GenerateRequest
		json.NewDecoder(r.Body).Decode(&req)
		
		// 验证max tokens设置
		assert.Equal(t, 2048, int(req.Options["num_predict"].(float64)))
		assert.Equal(t, 0.7, req.Options["temperature"].(float64))
		
		response := GenerateResponse{
			Model:    "llama2",
			Response: "翻译结果",
			Done:     true,
		}
		
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	
	config := DefaultConfig()
	config.APIEndpoint = server.URL
	config.MaxTokens = 2048
	config.Temperature = 0.7
	provider := New(config)
	
	req := &providers.ProviderRequest{
		Text:           "Test text",
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
	}
	
	_, err := provider.Translate(context.Background(), req)
	require.NoError(t, err)
}

func TestTranslateServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Internal server error"}`))
	}))
	defer server.Close()
	
	config := DefaultConfig()
	config.APIEndpoint = server.URL
	config.MaxRetries = 1 // 减少重试次数以加快测试
	provider := New(config)
	
	req := &providers.ProviderRequest{
		Text:           "Test text",
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
	}
	
	_, err := provider.Translate(context.Background(), req)
	assert.Error(t, err)
	assert.IsType(t, &APIError{}, err)
}

func TestTranslateNetworkError(t *testing.T) {
	config := DefaultConfig()
	config.APIEndpoint = "http://nonexistent-server:11434"
	config.MaxRetries = 1
	config.Timeout = 100 * time.Millisecond
	provider := New(config)
	
	req := &providers.ProviderRequest{
		Text:           "Test text",
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
	}
	
	_, err := provider.Translate(context.Background(), req)
	assert.Error(t, err)
}

func TestHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req GenerateRequest
		json.NewDecoder(r.Body).Decode(&req)
		
		// 验证健康检查请求
		assert.Equal(t, "llama2", req.Model)
		assert.Equal(t, "Hello", req.Prompt)
		assert.Equal(t, 5, int(req.Options["num_predict"].(float64)))
		
		response := GenerateResponse{
			Model:    "llama2",
			Response: "Hi",
			Done:     true,
		}
		
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	
	config := DefaultConfig()
	config.APIEndpoint = server.URL
	provider := New(config)
	
	err := provider.HealthCheck(context.Background())
	assert.NoError(t, err)
}

func TestHealthCheckFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error": "Service unavailable"}`))
	}))
	defer server.Close()
	
	config := DefaultConfig()
	config.APIEndpoint = server.URL
	config.MaxRetries = 0 // 不重试以加快测试
	provider := New(config)
	
	err := provider.HealthCheck(context.Background())
	assert.Error(t, err)
}

func TestAPIError(t *testing.T) {
	apiErr := &APIError{
		ErrorMsg: "Test error message",
	}
	
	assert.Equal(t, "Test error message", apiErr.Error())
}

func TestContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 延迟响应以测试取消
		time.Sleep(200 * time.Millisecond)
		
		response := GenerateResponse{
			Model:    "llama2",
			Response: "翻译结果",
			Done:     true,
		}
		
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	
	config := DefaultConfig()
	config.APIEndpoint = server.URL
	provider := New(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	
	req := &providers.ProviderRequest{
		Text:           "Test text",
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
	}
	
	_, err := provider.Translate(ctx, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestRetryLogic(t *testing.T) {
	retryCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		retryCount++
		t.Logf("Request attempt #%d", retryCount)
		
		if retryCount < 3 {
			// 前两次请求返回可重试的错误
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "Rate limited"}`))
			t.Logf("Returning error for attempt #%d", retryCount)
			return
		}
		
		// 第三次请求成功
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		response := GenerateResponse{
			Model:    "llama2",
			Response: "成功翻译",
			Done:     true,
		}
		
		t.Logf("Returning success for attempt #%d", retryCount)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	
	config := DefaultConfig()
	config.APIEndpoint = server.URL
	config.MaxRetries = 3 // 让它有足够的重试次数
	config.RetryDelay = 10 * time.Millisecond // 减少延迟以加快测试
	provider := New(config)
	
	req := &providers.ProviderRequest{
		Text:           "Test text",
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
	}
	
	resp, err := provider.Translate(context.Background(), req)
	if err != nil {
		t.Logf("Translation failed with error: %v", err)
	}
	require.NoError(t, err)
	assert.Equal(t, "成功翻译", resp.Text)
	assert.GreaterOrEqual(t, retryCount, 3) // 至少尝试了3次
}

func TestNonRetryableError(t *testing.T) {
	retryCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		retryCount++
		
		// 返回不可重试的错误（400 Bad Request）
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "Invalid request"}`))
	}))
	defer server.Close()
	
	config := DefaultConfig()
	config.APIEndpoint = server.URL
	config.MaxRetries = 3
	provider := New(config)
	
	req := &providers.ProviderRequest{
		Text:           "Test text",
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
	}
	
	_, err := provider.Translate(context.Background(), req)
	assert.Error(t, err)
	assert.Equal(t, 1, retryCount) // 应该只尝试一次，不重试
}