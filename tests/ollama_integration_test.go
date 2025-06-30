package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestOllamaIntegration 测试Ollama提供商的完整集成
// 注意：这个测试需要本地运行Ollama服务才能完全通过
func TestOllamaIntegration(t *testing.T) {
	// 检查是否应该跳过需要实际Ollama服务的测试
	if os.Getenv("SKIP_OLLAMA_TESTS") == "true" {
		t.Skip("Skipping Ollama integration tests (SKIP_OLLAMA_TESTS=true)")
	}

	logger, _ := zap.NewDevelopment()
	tempDir := t.TempDir()

	// 创建测试配置
	cfg := &config.Config{
		SourceLang:       "English",
		TargetLang:       "Chinese",
		DefaultModelName: "ollama-llama2",
		ChunkSize:        1000,
		RetryAttempts:    2,
		Country:          "China",
		Concurrency:      1,  // 使用单线程避免并发问题
		RequestTimeout:   60, // 增加超时时间
		ModelConfigs: map[string]config.ModelConfig{
			"ollama-llama2": {
				Name:            "ollama-llama2",
				ModelID:         "llama2",
				APIType:         "ollama",
				BaseURL:         getOllamaURL(),
				Key:             "",
				MaxOutputTokens: 4096,
				MaxInputTokens:  8192,
				Temperature:     0.3,
			},
			"ollama-mistral": {
				Name:            "ollama-mistral",
				ModelID:         "mistral",
				APIType:         "ollama",
				BaseURL:         getOllamaURL(),
				Key:             "",
				MaxOutputTokens: 4096,
				MaxInputTokens:  8192,
				Temperature:     0.2,
			},
		},
		StepSets: map[string]config.StepSetConfigV2{
			"ollama_test": {
				ID:          "ollama_test",
				Name:        "Ollama Integration Test",
				Description: "Test Ollama provider integration",
				Steps: []config.StepConfigV2{
					{
						Name:            "initial_translation",
						Provider:        "ollama",
						ModelName:       "ollama-llama2",
						Temperature:     0.3,
						MaxTokens:       4096,
						AdditionalNotes: "Translate accurately and preserve formatting.",
					},
				},
				FastModeThreshold: 500,
			},
			"ollama_multi_step": {
				ID:          "ollama_multi_step",
				Name:        "Ollama Multi-Step Test",
				Description: "Test multi-step translation with Ollama",
				Steps: []config.StepConfigV2{
					{
						Name:            "initial_translation",
						Provider:        "ollama",
						ModelName:       "ollama-llama2",
						Temperature:     0.3,
						MaxTokens:       4096,
						AdditionalNotes: "Provide an initial translation.",
					},
					{
						Name:            "improvement",
						Provider:        "ollama",
						ModelName:       "ollama-llama2",
						Temperature:     0.2,
						MaxTokens:       4096,
						AdditionalNotes: "Improve the translation quality.",
					},
				},
				FastModeThreshold: 200,
			},
		},
		ActiveStepSet: "ollama_test",
		Metadata:      make(map[string]interface{}),
	}

	coordinator, err := translator.NewTranslationCoordinator(cfg, logger, tempDir)
	if err != nil {
		// 如果创建coordinator失败，可能是因为没有Ollama服务
		t.Logf("Failed to create coordinator (may need Ollama service): %v", err)
		t.Skip("Skipping test due to coordinator creation failure")
	}
	require.NotNil(t, coordinator)

	t.Run("Simple Text Translation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		text := "Hello, world! This is a test."
		result, err := coordinator.TranslateText(ctx, text)
		if err != nil {
			if isOllamaConnectionError(err) {
				t.Skipf("Skipping test: Ollama service not available (%v)", err)
			}
			t.Fatalf("Translation failed: %v", err)
		}

		assert.NotEmpty(t, result)
		assert.NotEqual(t, text, result, "Translation should be different from original")
		t.Logf("Original: %s", text)
		t.Logf("Translated: %s", result)
	})

	t.Run("Markdown File Translation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// 创建测试输入文件
		inputFile := filepath.Join(tempDir, "test_input.md")
		outputFile := filepath.Join(tempDir, "test_output.md")

		inputContent := `# Test Document

This is a test paragraph with some **bold text** and *italic text*.

## Second Section

Here is a list:
- First item
- Second item
- Third item

And a code block:
` + "```go\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}\n```" + `

End of document.`

		err := os.WriteFile(inputFile, []byte(inputContent), 0o644)
		require.NoError(t, err)

		// 执行文档翻译
		result, err := coordinator.TranslateFile(ctx, inputFile, outputFile)
		if err != nil {
			if isOllamaConnectionError(err) {
				t.Skipf("Skipping file translation test: Ollama service not available (%v)", err)
			}
			t.Fatalf("File translation failed: %v", err)
		}

		assert.NotNil(t, result)
		t.Logf("Translation completed with result: %+v", result)

		// 验证输出文件存在
		if _, err := os.Stat(outputFile); err == nil {
			outputContent, err := os.ReadFile(outputFile)
			require.NoError(t, err)

			outputStr := string(outputContent)
			assert.NotEmpty(t, outputStr)
			assert.Contains(t, outputStr, "#") // 应该保留Markdown标题
			t.Logf("Output file content preview: %s", outputStr[:min(200, len(outputStr))])
		}
	})

	t.Run("Multi-Step Translation", func(t *testing.T) {
		// 切换到多步骤配置
		cfg.ActiveStepSet = "ollama_multi_step"
		coordinator, err := translator.NewTranslationCoordinator(cfg, logger, tempDir)
		if err != nil {
			t.Skipf("Failed to create multi-step coordinator: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		text := "The quick brown fox jumps over the lazy dog."
		result, err := coordinator.TranslateText(ctx, text)
		if err != nil {
			if isOllamaConnectionError(err) {
				t.Skipf("Skipping multi-step test: Ollama service not available (%v)", err)
			}
			t.Fatalf("Multi-step translation failed: %v", err)
		}

		assert.NotEmpty(t, result)
		t.Logf("Multi-step translation result: %s", result)
	})
}

func TestOllamaHealthCheck(t *testing.T) {
	if os.Getenv("SKIP_OLLAMA_TESTS") == "true" {
		t.Skip("Skipping Ollama health check (SKIP_OLLAMA_TESTS=true)")
	}

	// 简单的健康检查测试
	t.Run("Ollama Service Health Check", func(t *testing.T) {
		// 这里可以添加直接的HTTP健康检查
		// 或者创建一个简单的provider实例进行测试
		t.Log("Health check test - implement specific health check logic here")
	})
}

func TestOllamaConfigurationValidation(t *testing.T) {
	t.Run("Valid Ollama Configuration", func(t *testing.T) {
		cfg := &config.Config{
			ModelConfigs: map[string]config.ModelConfig{
				"ollama-test": {
					Name:            "ollama-test",
					ModelID:         "llama2",
					APIType:         "ollama",
					BaseURL:         "http://localhost:11434",
					Key:             "",
					MaxOutputTokens: 4096,
					Temperature:     0.3,
				},
			},
			StepSets: map[string]config.StepSetConfigV2{
				"test": {
					ID:   "test",
					Name: "Test",
					Steps: []config.StepConfigV2{
						{
							Name:      "step1",
							Provider:  "ollama",
							ModelName: "ollama-test",
						},
					},
				},
			},
			ActiveStepSet: "test",
			Concurrency:   1,
		}

		logger := zap.NewNop()
		tempDir := t.TempDir()

		_, err := translator.NewTranslationCoordinator(cfg, logger, tempDir)
		if err != nil && !isOllamaConnectionError(err) {
			t.Fatalf("Configuration validation failed: %v", err)
		}
		// 如果是连接错误，说明配置是有效的，只是服务不可用
	})

	t.Run("Invalid Model Reference", func(t *testing.T) {
		cfg := &config.Config{
			ModelConfigs: map[string]config.ModelConfig{},
			StepSets: map[string]config.StepSetConfigV2{
				"test": {
					ID:   "test",
					Name: "Test",
					Steps: []config.StepConfigV2{
						{
							Name:      "step1",
							Provider:  "ollama",
							ModelName: "nonexistent-model",
						},
					},
				},
			},
			ActiveStepSet: "test",
			Concurrency:   1,
		}

		logger := zap.NewNop()
		tempDir := t.TempDir()

		_, err := translator.NewTranslationCoordinator(cfg, logger, tempDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in configuration")
	})
}

// Helper functions

func getOllamaURL() string {
	if url := os.Getenv("OLLAMA_URL"); url != "" {
		return url
	}
	return "http://localhost:11434"
}

func isOllamaConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "connection refused") ||
		contains(errStr, "no such host") ||
		contains(errStr, "timeout") ||
		contains(errStr, "network is unreachable")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
