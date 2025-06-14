package translation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// MockLLMClient 模拟的LLM客户端
type MockLLMClient struct {
	responses map[string]string
	calls     []translation.ChatRequest
	err       error
}

func NewMockLLMClient() *MockLLMClient {
	return &MockLLMClient{
		responses: make(map[string]string),
		calls:     make([]translation.ChatRequest, 0),
	}
}

func (m *MockLLMClient) Complete(ctx context.Context, req *translation.CompletionRequest) (*translation.CompletionResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &translation.CompletionResponse{
		Text:      "completed text",
		Model:     req.Model,
		TokensIn:  100,
		TokensOut: 150,
	}, nil
}

func (m *MockLLMClient) Chat(ctx context.Context, req *translation.ChatRequest) (*translation.ChatResponse, error) {
	m.calls = append(m.calls, *req)

	if m.err != nil {
		return nil, m.err
	}

	// 根据步骤返回不同的响应
	var responseText string
	if len(req.Messages) > 1 {
		userMessage := req.Messages[len(req.Messages)-1].Content

		// 简单的模拟：根据提示词内容返回不同结果
		if contains(userMessage, "Translate") {
			responseText = "这是翻译后的文本。"
		} else if contains(userMessage, "Review") {
			responseText = "翻译基本准确，但有几个地方可以改进：1. 语气可以更自然 2. 某些词汇选择可以更地道"
		} else if contains(userMessage, "improve") {
			responseText = "这是改进后的翻译文本。"
		} else {
			responseText = "默认响应"
		}
	}

	return &translation.ChatResponse{
		Message: translation.ChatMessage{
			Role:    "assistant",
			Content: responseText,
		},
		Model:     req.Model,
		TokensIn:  100,
		TokensOut: 150,
	}, nil
}

func (m *MockLLMClient) GetModel() string {
	return "mock-model"
}

func (m *MockLLMClient) HealthCheck(ctx context.Context) error {
	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) >= len(substr) && s[len(s)-len(substr):] == substr ||
		findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// MockCache 模拟的缓存
type MockCache struct {
	data map[string]string
}

func NewMockCache() *MockCache {
	return &MockCache{
		data: make(map[string]string),
	}
}

func (m *MockCache) Get(key string) (string, bool) {
	val, ok := m.data[key]
	return val, ok
}

func (m *MockCache) Set(key string, value string) error {
	m.data[key] = value
	return nil
}

func (m *MockCache) Delete(key string) error {
	delete(m.data, key)
	return nil
}

func (m *MockCache) Clear() error {
	m.data = make(map[string]string)
	return nil
}

func (m *MockCache) Stats() translation.CacheStats {
	return translation.CacheStats{}
}

// TestNewService 测试创建服务
func TestNewService(t *testing.T) {
	tests := []struct {
		name    string
		config  *translation.Config
		opts    []translation.Option
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing config",
			config:  nil,
			wantErr: true,
		},
		{
			name:    "invalid config - no steps",
			config:  &translation.Config{},
			wantErr: true,
			errMsg:  "source language is required",
		},
		{
			name: "missing LLM client",
			config: &translation.Config{
				SourceLanguage: "English",
				TargetLanguage: "Chinese",
				ChunkSize:      1000,
				MaxConcurrency: 3,
				Steps: []translation.StepConfig{
					{Name: "translate", Model: "gpt-4", Prompt: "translate"},
				},
			},
			opts:    []translation.Option{},
			wantErr: true,
			errMsg:  "LLM client not configured",
		},
		{
			name:   "valid config with LLM client",
			config: translation.DefaultConfig(),
			opts: []translation.Option{
				translation.WithLLMClient(NewMockLLMClient()),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := translation.New(tt.config, tt.opts...)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				if tt.errMsg != "" && err != nil && !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error message to contain %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if svc == nil {
					t.Errorf("expected service but got nil")
				}
			}
		})
	}
}

// TestTranslate 测试翻译功能
func TestTranslate(t *testing.T) {
	ctx := context.Background()

	// 创建服务
	config := translation.DefaultConfig()
	config.ChunkSize = 50 // 小块大小以便测试分块

	llmClient := NewMockLLMClient()
	cache := NewMockCache()

	svc, err := translation.New(config,
		translation.WithLLMClient(llmClient),
		translation.WithCache(cache),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	tests := []struct {
		name    string
		req     *translation.Request
		wantErr bool
	}{
		{
			name: "simple translation",
			req: &translation.Request{
				Text: "Hello, world!",
			},
			wantErr: false,
		},
		{
			name: "empty text",
			req: &translation.Request{
				Text: "",
			},
			wantErr: true,
		},
		{
			name:    "nil request",
			req:     nil,
			wantErr: true,
		},
		{
			name: "with metadata",
			req: &translation.Request{
				Text: "Test translation",
				Metadata: map[string]interface{}{
					"source": "test",
					"id":     "123",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := svc.Translate(ctx, tt.req)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil {
					t.Errorf("expected response but got nil")
				} else {
					// 验证响应
					if resp.Text == "" {
						t.Errorf("expected translated text but got empty")
					}
					if resp.SourceLanguage != config.SourceLanguage {
						t.Errorf("expected source language %s, got %s", config.SourceLanguage, resp.SourceLanguage)
					}
					if resp.TargetLanguage != config.TargetLanguage {
						t.Errorf("expected target language %s, got %s", config.TargetLanguage, resp.TargetLanguage)
					}
					if len(resp.Steps) == 0 {
						t.Errorf("expected steps in response but got none")
					}
					if resp.Metrics == nil {
						t.Errorf("expected metrics but got nil")
					}
				}
			}
		})
	}
}

// TestTranslateBatch 测试批量翻译
func TestTranslateBatch(t *testing.T) {
	ctx := context.Background()

	// 创建服务
	config := translation.DefaultConfig()
	config.MaxConcurrency = 2

	svc, err := translation.New(config,
		translation.WithLLMClient(NewMockLLMClient()),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// 准备批量请求
	reqs := []*translation.Request{
		{Text: "Hello"},
		{Text: "World"},
		{Text: "Test"},
	}

	// 执行批量翻译
	responses, err := svc.TranslateBatch(ctx, reqs)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// 验证结果
	if len(responses) != len(reqs) {
		t.Errorf("expected %d responses, got %d", len(reqs), len(responses))
	}

	for i, resp := range responses {
		if resp == nil {
			t.Errorf("response %d is nil", i)
		} else if resp.Text == "" {
			t.Errorf("response %d has empty text", i)
		}
	}
}

// TestChainExecution 测试翻译链执行
func TestChainExecution(t *testing.T) {
	ctx := context.Background()

	// 创建翻译链
	chain := translation.NewChain()

	// 添加步骤
	llmClient := NewMockLLMClient()

	steps := []translation.StepConfig{
		{
			Name:   "translate",
			Model:  "gpt-4",
			Prompt: "Translate the following text",
			Variables: map[string]string{
				"source_language": "English",
				"target_language": "Chinese",
			},
		},
		{
			Name:   "reflect",
			Model:  "gpt-4",
			Prompt: "Review this translation",
		},
		{
			Name:   "improve",
			Model:  "gpt-4",
			Prompt: "Improve the translation",
		},
	}

	for _, cfg := range steps {
		step := translation.NewStep(&cfg, llmClient, nil)
		chain.AddStep(step)
	}

	// 执行链
	result, err := chain.Execute(ctx, "Hello, world!")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// 验证结果
	if result == nil {
		t.Fatal("expected result but got nil")
	}
	if result.FinalOutput == "" {
		t.Errorf("expected final output but got empty")
	}
	if len(result.Steps) != 3 {
		t.Errorf("expected 3 steps, got %d", len(result.Steps))
	}
	if !result.Success {
		t.Errorf("expected success but got failure")
	}
}

// TestChunker 测试文本分块器
func TestChunker(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		chunkSize int
		overlap   int
		minChunks int
		maxChunks int
	}{
		{
			name:      "small text",
			text:      "Hello, world!",
			chunkSize: 100,
			overlap:   10,
			minChunks: 1,
			maxChunks: 1,
		},
		{
			name:      "multiple paragraphs",
			text:      "First paragraph.\n\nSecond paragraph.\n\nThird paragraph.",
			chunkSize: 30,
			overlap:   5,
			minChunks: 2,
			maxChunks: 3,
		},
		{
			name:      "long text",
			text:      generateLongText(500),
			chunkSize: 100,
			overlap:   20,
			minChunks: 5,
			maxChunks: 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunker := translation.NewDefaultChunker(tt.chunkSize, tt.overlap)
			chunks := chunker.Chunk(tt.text)

			if len(chunks) < tt.minChunks || len(chunks) > tt.maxChunks {
				t.Errorf("expected %d-%d chunks, got %d", tt.minChunks, tt.maxChunks, len(chunks))
			}

			// 验证块大小
			for i, chunk := range chunks {
				if len(chunk) > tt.chunkSize*4 { // 允许一定的弹性（UTF-8字符）
					t.Errorf("chunk %d exceeds size limit: %d > %d", i, len(chunk), tt.chunkSize*4)
				}
			}
		})
	}
}

// generateLongText 生成长文本用于测试
func generateLongText(words int) string {
	text := ""
	for i := 0; i < words; i++ {
		if i > 0 && i%10 == 0 {
			text += ". "
		}
		if i > 0 && i%50 == 0 {
			text += "\n\n"
		}
		text += "word "
	}
	return text
}

// TestProgressTracking 测试进度跟踪
func TestProgressTracking(t *testing.T) {
	ctx := context.Background()

	progressUpdates := make([]translation.Progress, 0)

	// 创建服务with进度回调
	config := translation.DefaultConfig()
	svc, err := translation.New(config,
		translation.WithLLMClient(NewMockLLMClient()),
		translation.WithProgressCallback(func(p *translation.Progress) {
			progressUpdates = append(progressUpdates, *p)
		}),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// 执行翻译
	_, err = svc.Translate(ctx, &translation.Request{
		Text: "Test text for progress tracking",
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// 验证进度更新
	if len(progressUpdates) == 0 {
		t.Errorf("expected progress updates but got none")
	}

	// 验证最后的进度是100%
	lastProgress := progressUpdates[len(progressUpdates)-1]
	if lastProgress.Percent != 100 {
		t.Errorf("expected final progress to be 100%%, got %.2f%%", lastProgress.Percent)
	}
}

// TestErrorHandling 测试错误处理
func TestErrorHandling(t *testing.T) {
	ctx := context.Background()

	// 创建会失败的LLM客户端
	llmClient := NewMockLLMClient()
	llmClient.err = errors.New("LLM service unavailable")

	config := translation.DefaultConfig()
	config.MaxRetries = 2

	errorHandled := false
	svc, err := translation.New(config,
		translation.WithLLMClient(llmClient),
		translation.WithErrorHandler(func(err error) {
			errorHandled = true
		}),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// 执行翻译（应该失败）
	_, err = svc.Translate(ctx, &translation.Request{
		Text: "Test text",
	})

	if err == nil {
		t.Errorf("expected error but got none")
	}

	if !errorHandled {
		t.Errorf("expected error handler to be called")
	}

	// 验证错误类型
	var transErr *translation.TranslationError
	if errors.As(err, &transErr) {
		if transErr.Code != translation.ErrCodeChain && transErr.Code != translation.ErrCodeStep {
			t.Errorf("expected error code %s or %s, got %s", translation.ErrCodeChain, translation.ErrCodeStep, transErr.Code)
		}
	} else {
		t.Errorf("expected TranslationError type")
	}
}
