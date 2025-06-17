package translation

import (
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/ollama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestProviderManager_CreateOllamaProvider(t *testing.T) {
	cfg := &Config{
		ModelConfigs: map[string]config.ModelConfig{
			"ollama-llama2": {
				Name:            "ollama-llama2",
				ModelID:         "llama2",
				APIType:         "ollama",
				BaseURL:         "http://localhost:11434",
				Key:             "",
				MaxOutputTokens: 4096,
				MaxInputTokens:  8192,
				Temperature:     0.3,
			},
		},
		ActiveStepSet: "ollama_test",
		StepSets: map[string]config.StepSetConfigV2{
			"ollama_test": {
				ID:          "ollama_test",
				Name:        "Ollama Test",
				Description: "Test Ollama provider",
				Steps: []config.StepConfigV2{
					{
						Name:        "initial",
						Provider:    "ollama",
						ModelName:   "ollama-llama2",
						Temperature: 0.3,
						MaxTokens:   4096,
					},
				},
				FastModeThreshold: 500,
			},
		},
	}
	
	logger := zap.NewNop()
	pm := NewProviderManager(cfg, logger)
	
	modelConfig := cfg.ModelConfigs["ollama-llama2"]
	provider, err := pm.createOllamaProvider(modelConfig)
	
	require.NoError(t, err)
	assert.NotNil(t, provider)
	assert.Equal(t, "ollama", provider.GetName())
	assert.True(t, provider.SupportsSteps())
}

func TestProviderManager_CreateOllamaProviderWithCustomConfig(t *testing.T) {
	cfg := &Config{
		ModelConfigs: map[string]config.ModelConfig{
			"ollama-mistral": {
				Name:            "ollama-mistral",
				ModelID:         "mistral:7b",
				APIType:         "ollama",
				BaseURL:         "http://custom-ollama:8080",
				Key:             "",
				MaxOutputTokens: 2048,
				MaxInputTokens:  4096,
				Temperature:     0.7,
			},
		},
	}
	
	logger := zap.NewNop()
	pm := NewProviderManager(cfg, logger)
	
	modelConfig := cfg.ModelConfigs["ollama-mistral"]
	provider, err := pm.createOllamaProvider(modelConfig)
	
	require.NoError(t, err)
	assert.NotNil(t, provider)
	
	// 检查provider配置
	ollamaProvider, ok := provider.(*ollama.Provider)
	require.True(t, ok)
	
	// 通过反射或其他方式验证配置，这里我们主要验证provider创建成功
	assert.Equal(t, "ollama", ollamaProvider.GetName())
}

func TestProviderManager_CreateOllamaProviderDefaultURL(t *testing.T) {
	cfg := &Config{
		ModelConfigs: map[string]config.ModelConfig{
			"ollama-test": {
				Name:            "ollama-test",
				ModelID:         "llama2",
				APIType:         "ollama",
				BaseURL:         "", // 空URL，应该使用默认值
				Key:             "",
				MaxOutputTokens: 4096,
				MaxInputTokens:  8192,
				Temperature:     0.3,
			},
		},
	}
	
	logger := zap.NewNop()
	pm := NewProviderManager(cfg, logger)
	
	modelConfig := cfg.ModelConfigs["ollama-test"]
	provider, err := pm.createOllamaProvider(modelConfig)
	
	require.NoError(t, err)
	assert.NotNil(t, provider)
	assert.Equal(t, "ollama", provider.GetName())
}

func TestProviderManager_OllamaCapabilities(t *testing.T) {
	cfg := &Config{}
	logger := zap.NewNop()
	pm := NewProviderManager(cfg, logger)
	
	capabilities := pm.getProviderCapabilities("ollama")
	
	assert.True(t, capabilities.SupportsPrompts)
	assert.False(t, capabilities.SupportsSystemRole) // Ollama通常不支持系统角色
	assert.True(t, capabilities.SupportsTemperature)
	assert.True(t, capabilities.SupportsMultiStep)
	assert.False(t, capabilities.RequiresAPIKey)
	assert.Equal(t, "llama2", capabilities.DefaultModel)
}

func TestProviderManager_CreateProvidersWithOllama(t *testing.T) {
	cfg := &Config{
		ModelConfigs: map[string]config.ModelConfig{
			"ollama-llama2": {
				Name:            "ollama-llama2",
				ModelID:         "llama2",
				APIType:         "ollama",
				BaseURL:         "http://localhost:11434",
				Key:             "",
				MaxOutputTokens: 4096,
				MaxInputTokens:  8192,
				Temperature:     0.3,
			},
			"ollama-mistral": {
				Name:            "ollama-mistral",
				ModelID:         "mistral",
				APIType:         "ollama",
				BaseURL:         "http://localhost:11434",
				Key:             "",
				MaxOutputTokens: 4096,
				MaxInputTokens:  8192,
				Temperature:     0.2,
			},
		},
		ActiveStepSet: "ollama_multi",
		StepSets: map[string]config.StepSetConfigV2{
			"ollama_multi": {
				ID:          "ollama_multi",
				Name:        "Ollama Multi-Model",
				Description: "Use multiple Ollama models",
				Steps: []config.StepConfigV2{
					{
						Name:        "initial",
						Provider:    "ollama",
						ModelName:   "ollama-llama2",
						Temperature: 0.3,
						MaxTokens:   4096,
					},
					{
						Name:        "reflection",
						Provider:    "ollama",
						ModelName:   "ollama-mistral",
						Temperature: 0.2,
						MaxTokens:   3000,
					},
					{
						Name:        "improvement",
						Provider:    "ollama",
						ModelName:   "ollama-llama2",
						Temperature: 0.3,
						MaxTokens:   4096,
					},
				},
				FastModeThreshold: 500,
			},
		},
	}
	
	logger := zap.NewNop()
	pm := NewProviderManager(cfg, logger)
	
	providers, err := pm.CreateProviders()
	
	require.NoError(t, err)
	assert.Len(t, providers, 2) // 应该创建两个不同的ollama provider实例
	
	// 验证提供商
	assert.Contains(t, providers, "ollama")
	ollamaProvider := providers["ollama"]
	assert.NotNil(t, ollamaProvider)
	assert.Equal(t, "ollama", ollamaProvider.GetName())
}

func TestProviderManager_OllamaInStepSetValidation(t *testing.T) {
	cfg := &Config{
		ModelConfigs: map[string]config.ModelConfig{
			"ollama-llama2": {
				Name:            "ollama-llama2",
				ModelID:         "llama2",
				APIType:         "ollama",
				BaseURL:         "http://localhost:11434",
				Key:             "",
				MaxOutputTokens: 4096,
				MaxInputTokens:  8192,
				Temperature:     0.3,
			},
		},
		ActiveStepSet: "ollama_single",
		StepSets: map[string]config.StepSetConfigV2{
			"ollama_single": {
				ID:          "ollama_single",
				Name:        "Ollama Single Step",
				Description: "Single step with Ollama",
				Steps: []config.StepConfigV2{
					{
						Name:        "translation",
						Provider:    "ollama",
						ModelName:   "ollama-llama2",
						Temperature: 0.3,
						MaxTokens:   4096,
					},
				},
				FastModeThreshold: 1000,
			},
		},
	}
	
	logger := zap.NewNop()
	pm := NewProviderManager(cfg, logger)
	
	providers, err := pm.CreateProviders()
	
	require.NoError(t, err)
	assert.Len(t, providers, 1)
	assert.Contains(t, providers, "ollama")
}

func TestProviderManager_OllamaMissingModelConfig(t *testing.T) {
	cfg := &Config{
		ModelConfigs:  map[string]config.ModelConfig{}, // 空的模型配置
		ActiveStepSet: "ollama_missing",
		StepSets: map[string]config.StepSetConfigV2{
			"ollama_missing": {
				ID:          "ollama_missing",
				Name:        "Ollama Missing Config",
				Description: "Test missing model config",
				Steps: []config.StepConfigV2{
					{
						Name:        "translation",
						Provider:    "ollama",
						ModelName:   "nonexistent-model",
						Temperature: 0.3,
						MaxTokens:   4096,
					},
				},
				FastModeThreshold: 500,
			},
		},
	}
	
	logger := zap.NewNop()
	pm := NewProviderManager(cfg, logger)
	
	_, err := pm.CreateProviders()
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in configuration")
}

func TestProviderManager_CreateProviderSwitchCase(t *testing.T) {
	cfg := &Config{
		ModelConfigs: map[string]config.ModelConfig{
			"test-model": {
				Name:            "test-model",
				ModelID:         "llama2",
				APIType:         "ollama",
				BaseURL:         "http://localhost:11434",
				Key:             "",
				MaxOutputTokens: 4096,
				MaxInputTokens:  8192,
				Temperature:     0.3,
			},
		},
	}
	
	logger := zap.NewNop()
	pm := NewProviderManager(cfg, logger)
	
	modelConfig := cfg.ModelConfigs["test-model"]
	
	// 测试 "ollama" case
	provider, err := pm.createProvider("ollama", modelConfig)
	require.NoError(t, err)
	assert.Equal(t, "ollama", provider.GetName())
	
	// 测试不支持的provider类型
	_, err = pm.createProvider("unsupported", modelConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported provider type")
}