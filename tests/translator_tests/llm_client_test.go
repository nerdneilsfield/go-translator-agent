package translator_tests

import (
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// 测试LLM客户端的错误处理
func TestLLMClientErrorHandling(t *testing.T) {
	// 由于需要实际的API调用，我们跳过这个测试
	t.Skip("跳过LLM客户端错误处理测试，因为需要实际的API调用")
}

// 测试LLM客户端的超时处理
func TestLLMClientTimeout(t *testing.T) {
	// 由于需要实际的API调用，我们跳过这个测试
	t.Skip("跳过LLM客户端超时测试，因为需要实际的API调用")
}

// 测试不同API类型的LLM客户端
func TestDifferentAPITypes(t *testing.T) {
	// 创建logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()

	// 测试用例
	testCases := []struct {
		name          string
		apiType       string
		modelName     string
		expectedError bool
	}{
		{
			name:          "OpenAI API",
			apiType:       "openai",
			modelName:     "gpt-3.5-turbo",
			expectedError: false,
		},
		{
			name:          "OpenAI Reasoning API",
			apiType:       "openai-reasoning",
			modelName:     "gpt-4",
			expectedError: false,
		},
		{
			name:          "Raw API",
			apiType:       "raw",
			modelName:     "raw",
			expectedError: false,
		},
		{
			name:          "Anthropic API",
			apiType:       "anthropic",
			modelName:     "claude-2",
			expectedError: false, // 不应该报错，因为会使用OpenAI客户端代替
		},
		{
			name:          "Mistral API",
			apiType:       "mistral",
			modelName:     "mistral-large",
			expectedError: false, // 不应该报错，因为会使用OpenAI客户端代替
		},
		{
			name:          "不支持的API类型",
			apiType:       "unsupported",
			modelName:     "unknown",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建配置
			cfg := &config.Config{
				SourceLang:       "English",
				TargetLang:       "Chinese",
				DefaultModelName: tc.modelName,
				ModelConfigs: map[string]config.ModelConfig{
					tc.modelName: {
						Name:            tc.modelName,
						APIType:         tc.apiType,
						MaxInputTokens:  8000,
						MaxOutputTokens: 2000,
						Key:             "test-key",
					},
				},
				StepSets: map[string]config.StepSetConfig{
					"test": {
						ID:          "test",
						Name:        "测试",
						Description: "测试步骤集",
						InitialTranslation: config.StepConfig{
							Name:        "初始翻译",
							ModelName:   tc.modelName,
							Temperature: 0.5,
						},
						Reflection: config.StepConfig{
							Name:        "反思",
							ModelName:   "none",
							Temperature: 0.3,
						},
						Improvement: config.StepConfig{
							Name:        "改进",
							ModelName:   "none",
							Temperature: 0.5,
						},
						FastModeThreshold: 300,
					},
				},
				ActiveStepSet: "test",
			}

			// 创建翻译器
			_, err := translator.New(cfg)

			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// 测试LLM客户端的响应处理
func TestLLMClientResponseProcessing(t *testing.T) {
	// 由于需要实际的API调用，我们跳过这个测试
	t.Skip("跳过LLM客户端响应处理测试，因为需要实际的API调用")
}
