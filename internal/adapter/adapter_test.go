package adapter

import (
	"context"
	"testing"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockTranslationService 模拟翻译服务
type MockTranslationService struct {
	TranslateFunc      func(ctx context.Context, req *translation.Request) (*translation.Response, error)
	TranslateBatchFunc func(ctx context.Context, reqs []*translation.Request) ([]*translation.Response, error)
	GetConfigFunc      func() *translation.Config
}

func (m *MockTranslationService) Translate(ctx context.Context, req *translation.Request) (*translation.Response, error) {
	if m.TranslateFunc != nil {
		return m.TranslateFunc(ctx, req)
	}
	return &translation.Response{
		Text: "Translated: " + req.Text,
		Metrics: &translation.TranslationMetrics{
			Duration:      time.Second,
			InputLength:   len(req.Text),
			OutputLength:  len("Translated: " + req.Text),
			TotalTokensIn: 10,
			TotalTokensOut: 15,
		},
	}, nil
}

func (m *MockTranslationService) TranslateBatch(ctx context.Context, reqs []*translation.Request) ([]*translation.Response, error) {
	if m.TranslateBatchFunc != nil {
		return m.TranslateBatchFunc(ctx, reqs)
	}
	responses := make([]*translation.Response, len(reqs))
	for i, req := range reqs {
		resp, err := m.Translate(ctx, req)
		if err != nil {
			return nil, err
		}
		responses[i] = resp
	}
	return responses, nil
}

func (m *MockTranslationService) GetConfig() *translation.Config {
	if m.GetConfigFunc != nil {
		return m.GetConfigFunc()
	}
	return &translation.Config{
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
	}
}

func TestTranslatorAdapter_Translate(t *testing.T) {
	// 创建模拟服务
	mockService := &MockTranslationService{}
	
	// 创建测试配置
	cfg := &config.Config{
		SourceLang: "English",
		TargetLang: "Chinese",
		DefaultModelName: "gpt-3.5-turbo",
	}
	
	// 创建适配器（使用反射设置私有字段进行测试）
	adapter := &TranslatorAdapter{
		service:         mockService,
		config:          cfg,
		logger:          nil,
		progressTracker: nil, // 暂时不测试进度跟踪
	}
	adapter.ctx, adapter.cancel = context.WithCancel(context.Background())
	
	// 测试翻译
	t.Run("successful translation", func(t *testing.T) {
		result, err := adapter.Translate("Hello, world!", false)
		require.NoError(t, err)
		assert.Equal(t, "Translated: Hello, world!", result)
	})
	
	// 测试空文本
	t.Run("empty text", func(t *testing.T) {
		result, err := adapter.Translate("", false)
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})
	
	// 测试空白文本
	t.Run("whitespace text", func(t *testing.T) {
		result, err := adapter.Translate("   \n\t   ", false)
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})
}

func TestConvertConfig(t *testing.T) {
	tests := []struct {
		name    string
		input   *config.Config
		wantErr bool
		check   func(t *testing.T, result *translation.Config)
	}{
		{
			name: "basic config conversion",
			input: &config.Config{
				SourceLang:         "English",
				TargetLang:         "Chinese",
				MaxTokensPerChunk:  1000,
				Concurrency:        3,
				MaxRetries:         2,
				TranslationTimeout: 60,
				DefaultModelName:   "gpt-3.5-turbo",
			},
			wantErr: false,
			check: func(t *testing.T, result *translation.Config) {
				assert.Equal(t, "English", result.SourceLanguage)
				assert.Equal(t, "Chinese", result.TargetLanguage)
				assert.Equal(t, 1000, result.ChunkSize)
				assert.Equal(t, 100, result.ChunkOverlap) // 默认值
				assert.Equal(t, 3, result.MaxConcurrency)
				assert.Equal(t, 2, result.MaxRetries)
				assert.Equal(t, 5*time.Second, result.RetryDelay) // 默认值
				assert.Equal(t, 60*time.Second, result.Timeout)
				assert.Len(t, result.Steps, 3) // 默认三步
			},
		},
		{
			name: "step set config",
			input: &config.Config{
				SourceLang:         "English",
				TargetLang:         "Chinese",
				ActiveStepSet:      "custom",
				TranslationTimeout: 60,
				DefaultModelName:   "gpt-3.5-turbo",
				StepSets: map[string]config.StepSetConfig{
					"custom": {
						ID:   "custom",
						Name: "custom",
						InitialTranslation: config.StepConfig{
							Name:        "translate",
							ModelName:   "gpt-4",
							Temperature: 0.5,
						},
						Reflection: config.StepConfig{
							Name:        "reflect",
							ModelName:   "gpt-4",
							Temperature: 0.3,
						},
						Improvement: config.StepConfig{
							Name:        "improve",
							ModelName:   "deepl",
							Temperature: 0.5,
						},
					},
				},
				ModelConfigs: map[string]config.ModelConfig{
					"gpt-4": {
						Name:    "gpt-4",
						APIType: "openai",
					},
					"deepl": {
						Name:    "deepl",
						APIType: "deepl",
					},
				},
			},
			wantErr: false,
			check: func(t *testing.T, result *translation.Config) {
				assert.Len(t, result.Steps, 3)
				assert.Equal(t, "initial_translation", result.Steps[0].Name)
				assert.Equal(t, "openai", result.Steps[0].Provider)
				assert.Equal(t, "gpt-4", result.Steps[0].Model)
				assert.Equal(t, float32(0.5), result.Steps[0].Temperature)
				assert.Equal(t, "reflection", result.Steps[1].Name)
				assert.Equal(t, "openai", result.Steps[1].Provider)
				assert.Equal(t, "improvement", result.Steps[2].Name)
				assert.Equal(t, "deepl", result.Steps[2].Provider)
			},
		},
		{
			name:    "nil config",
			input:   nil,
			wantErr: true,
		},
		{
			name: "missing step set",
			input: &config.Config{
				ActiveStepSet: "nonexistent",
				StepSets:      map[string]config.StepSetConfig{},
			},
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertConfig(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				tt.check(t, result)
			}
		})
	}
}

func TestInferProviderFromModel(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"gpt-3.5-turbo", "openai"},
		{"gpt-4", "openai"},
		{"text-davinci-003", "openai"},
		{"claude-3-opus", "anthropic"},
		{"claude-instant", "anthropic"},
		{"gemini-pro", "google"},
		{"bard", "google"},
		{"deepl-pro", "deepl"},
		{"llama-2-70b", "ollama"},
		{"mistral-7b", "ollama"},
		{"unknown-model", "openai"}, // 默认
	}
	
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := inferProviderFromModel(tt.model)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertLanguageCode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"English", "en"},
		{"chinese", "zh"},
		{"JAPANESE", "ja"},
		{"en", "en"},
		{"zh-CN", "zh-CN"},
		{"unknown", "unknown"},
		{"  english  ", "en"},
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ConvertLanguageCode(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"timeout error", translation.ErrTimeout, true},
		{"rate limit error", translation.ErrRateLimited, true},
		{"invalid config", translation.ErrInvalidConfig, false},
		{"network error", translation.NewTranslationError("NETWORK", "network unavailable", nil), true},
		{"temporary error", translation.NewTranslationError("TEMP", "temporary failure", nil), true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}