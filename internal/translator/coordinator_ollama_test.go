package translator

import (
	"context"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func createOllamaTestConfig() *config.Config {
	return &config.Config{
		SourceLang:       "en",
		TargetLang:       "zh",
		DefaultModelName: "ollama-llama2",
		ChunkSize:        1000,
		RetryAttempts:    3,
		Country:          "US",
		Concurrency:      2,
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
		StepSets: map[string]config.StepSetConfigV2{
			"ollama_local": {
				ID:          "ollama_local",
				Name:        "Ollama Local",
				Description: "Use local Ollama models",
				Steps: []config.StepConfigV2{
					{
						Name:        "initial_translation",
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
			"ollama_fast": {
				ID:          "ollama_fast",
				Name:        "Ollama Fast",
				Description: "Single step Ollama translation",
				Steps: []config.StepConfigV2{
					{
						Name:        "direct_translation",
						Provider:    "ollama",
						ModelName:   "ollama-llama2",
						Temperature: 0.3,
						MaxTokens:   4096,
					},
				},
				FastModeThreshold: 100,
			},
		},
		ActiveStepSet: "ollama_local",
		Metadata:      make(map[string]interface{}),
	}
}

func TestTranslationCoordinator_OllamaProvider(t *testing.T) {
	cfg := createOllamaTestConfig()
	logger := zap.NewNop()
	progressPath := t.TempDir()

	coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
	require.NoError(t, err)
	require.NotNil(t, coordinator)

	t.Run("Coordinator Creation with Ollama Config", func(t *testing.T) {
		assert.NotNil(t, coordinator.coordinatorConfig)
		assert.NotNil(t, coordinator.translationService)
		assert.NotNil(t, coordinator.translator)
		assert.NotNil(t, coordinator.logger)
	})

	t.Run("Ollama Text Translation Test", func(t *testing.T) {
		text := "Hello, world!"

		// 注意：这个测试可能会失败，因为需要实际的Ollama服务运行
		// 但我们可以验证基本的流程不会崩溃
		result, err := coordinator.TranslateText(context.Background(), text)

		if err != nil {
			// 预期错误：无法连接到Ollama服务
			t.Logf("Expected error (no Ollama service): %v", err)
			assert.Contains(t, err.Error(), "connection refused")
		} else {
			// 如果有Ollama服务运行，验证结果
			assert.NotEmpty(t, result)
			t.Logf("Translation result: %s", result)
		}
	})
}

func TestTranslationCoordinator_OllamaFastMode(t *testing.T) {
	cfg := createOllamaTestConfig()
	cfg.ActiveStepSet = "ollama_fast"
	
	logger := zap.NewNop()
	progressPath := t.TempDir()

	coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
	require.NoError(t, err)

	t.Run("Fast Mode Text Translation", func(t *testing.T) {
		text := "Short text"

		result, err := coordinator.TranslateText(context.Background(), text)

		if err != nil {
			t.Logf("Expected error (no Ollama service): %v", err)
			// 验证错误类型是连接错误而不是配置错误
			assert.Contains(t, err.Error(), "connection refused")
		} else {
			assert.NotEmpty(t, result)
		}
	})
}

func TestTranslationCoordinator_OllamaStepSetValidation(t *testing.T) {
	logger := zap.NewNop()
	progressPath := t.TempDir()

	t.Run("Valid Ollama Configuration", func(t *testing.T) {
		cfg := createOllamaTestConfig()
		coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
		
		require.NoError(t, err)
		assert.NotNil(t, coordinator)
	})

	t.Run("Missing Model Configuration", func(t *testing.T) {
		cfg := createOllamaTestConfig()
		// 删除模型配置
		delete(cfg.ModelConfigs, "ollama-llama2")
		
		_, err := NewTranslationCoordinator(cfg, logger, progressPath)
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in configuration")
	})

	t.Run("Invalid Step Set", func(t *testing.T) {
		cfg := createOllamaTestConfig()
		cfg.ActiveStepSet = "nonexistent_step_set"
		
		_, err := NewTranslationCoordinator(cfg, logger, progressPath)
		
		assert.Error(t, err)
	})
}

func TestTranslationCoordinator_OllamaConfigMapping(t *testing.T) {
	cfg := createOllamaTestConfig()
	logger := zap.NewNop()
	progressPath := t.TempDir()

	coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
	require.NoError(t, err)

	t.Run("Coordinator Config Mapping", func(t *testing.T) {
		// 验证coordinator配置正确映射
		assert.Equal(t, cfg.SourceLang, coordinator.coordinatorConfig.SourceLang)
		assert.Equal(t, cfg.TargetLang, coordinator.coordinatorConfig.TargetLang)
		assert.Equal(t, cfg.ChunkSize, coordinator.coordinatorConfig.ChunkSize)
	})
}

func TestTranslationCoordinator_OllamaMultipleModels(t *testing.T) {
	// 测试使用多个不同Ollama模型的配置
	cfg := createOllamaTestConfig()
	
	// 添加更多Ollama模型
	cfg.ModelConfigs["ollama-codellama"] = config.ModelConfig{
		Name:            "ollama-codellama",
		ModelID:         "codellama",
		APIType:         "ollama",
		BaseURL:         "http://localhost:11434",
		Key:             "",
		MaxOutputTokens: 4096,
		MaxInputTokens:  8192,
		Temperature:     0.1,
	}
	
	// 创建使用三个不同模型的步骤集
	cfg.StepSets["ollama_multi"] = config.StepSetConfigV2{
		ID:          "ollama_multi",
		Name:        "Ollama Multi Model",
		Description: "Use three different Ollama models",
		Steps: []config.StepConfigV2{
			{
				Name:        "initial_translation",
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
				ModelName:   "ollama-codellama",
				Temperature: 0.1,
				MaxTokens:   4096,
			},
		},
		FastModeThreshold: 500,
	}
	cfg.ActiveStepSet = "ollama_multi"
	
	logger := zap.NewNop()
	progressPath := t.TempDir()

	coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
	require.NoError(t, err)

	t.Run("Multiple Ollama Models Configuration", func(t *testing.T) {
		assert.NotNil(t, coordinator)
		
		// 验证translation service可以处理多个模型
		text := "Code function example: def hello_world(): print('Hello, World!')"
		
		result, err := coordinator.TranslateText(context.Background(), text)
		
		if err != nil {
			t.Logf("Expected error (no Ollama service): %v", err)
		} else {
			assert.NotEmpty(t, result)
			t.Logf("Multi-model translation result: %s", result)
		}
	})
}

func TestTranslationCoordinator_OllamaWithCustomEndpoint(t *testing.T) {
	cfg := createOllamaTestConfig()
	
	// 修改为自定义端点
	for name, modelCfg := range cfg.ModelConfigs {
		modelCfg.BaseURL = "http://custom-ollama:8080"
		cfg.ModelConfigs[name] = modelCfg
	}
	
	logger := zap.NewNop()
	progressPath := t.TempDir()

	coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
	require.NoError(t, err)

	t.Run("Custom Ollama Endpoint", func(t *testing.T) {
		text := "Test with custom endpoint"
		
		result, err := coordinator.TranslateText(context.Background(), text)
		
		if err != nil {
			// 应该是连接错误，因为自定义端点不存在
			t.Logf("Expected connection error: %v", err)
			assert.Contains(t, err.Error(), "connection refused")
		} else {
			assert.NotEmpty(t, result)
		}
	})
}

func TestTranslationCoordinator_OllamaEmptyText(t *testing.T) {
	cfg := createOllamaTestConfig()
	logger := zap.NewNop()
	progressPath := t.TempDir()

	coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
	require.NoError(t, err)

	t.Run("Empty Text Translation", func(t *testing.T) {
		result, err := coordinator.TranslateText(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("Whitespace Only Text", func(t *testing.T) {
		result, err := coordinator.TranslateText(context.Background(), "   \n\t  ")
		if err != nil {
			// 可能因为没有实际内容而返回错误
			t.Logf("Whitespace text error: %v", err)
		} else {
			// 应该返回空字符串或处理过的空白字符
			t.Logf("Whitespace text result: '%s'", result)
		}
	})
}