package testutils

import (
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
)

// CreateTestConfig 创建通用测试配置
func CreateTestConfig(progressPath string) *config.Config {
	return &config.Config{
		// 基础配置
		SourceLang:        "English",
		TargetLang:        "Chinese",
		Concurrency:       1,
		ChunkSize:         1000,
		UseCache:          false,
		Debug:             false,
		
		// 超时配置
		RequestTimeout:    30,
		RetryAttempts:     3,
		TranslationTimeout: 1800,
		
		// 格式修复配置
		EnableFormatFix:      true,
		FormatFixInteractive: true,
		PreTranslationFix:    false,
		PostTranslationFix:   false,
		
		// 翻译后处理配置
		EnablePostProcessing:      true,
		GlossaryPath:             "configs/glossary_example.yaml",
		ContentProtection:        true,
		TerminologyConsistency:   true,
		MixedLanguageSpacing:     true,
		MachineTranslationCleanup: true,
		
		// 步骤集配置
		ActiveStepSet: "test-stepset",
		StepSets: map[string]config.StepSetConfig{
			"test-stepset": {
				ID:   "test-stepset",
				Name: "Test Step Set",
				InitialTranslation: config.StepConfig{
					Name:        "initial",
					ModelName:   "test-model",
					Temperature: 0.3,
				},
				Reflection: config.StepConfig{
					Name:        "reflect",
					ModelName:   "test-model",
					Temperature: 0.1,
				},
				Improvement: config.StepConfig{
					Name:        "improve",
					ModelName:   "test-model",
					Temperature: 0.3,
				},
			},
		},
		
		// 模型配置
		ModelConfigs: map[string]config.ModelConfig{
			"test-model": {
				Name:    "test-model",
				APIType: "openai",
				BaseURL: "http://localhost:8080",
				Key:     "test-key",
			},
		},
	}
}

// CreateParallelTestConfig 创建并行测试配置
func CreateParallelTestConfig(concurrency int, progressPath string) *config.Config {
	cfg := CreateTestConfig(progressPath)
	cfg.Concurrency = concurrency
	cfg.ChunkSize = 500 // 较小的分块确保会产生多个块
	return cfg
}

// CreateMockConfig 创建模拟配置
func CreateMockConfig(progressPath string) *config.Config {
	cfg := CreateTestConfig(progressPath)
	cfg.UseCache = false
	cfg.Debug = true
	return cfg
}

// CreatePerformanceTestConfig 创建性能测试配置
func CreatePerformanceTestConfig(progressPath string) *config.Config {
	cfg := CreateTestConfig(progressPath)
	cfg.Concurrency = 4
	cfg.ChunkSize = 200
	cfg.RequestTimeout = 60
	return cfg
}

// CreateIntegrationTestConfig 创建集成测试配置
func CreateIntegrationTestConfig(progressPath string) *config.Config {
	cfg := CreateTestConfig(progressPath)
	cfg.FormatFixInteractive = false // 集成测试中不需要交互
	cfg.Debug = false                // 减少日志输出
	return cfg
}

// CreatePostProcessingTestConfig 创建后处理测试配置
func CreatePostProcessingTestConfig(progressPath string) *config.Config {
	cfg := CreateTestConfig(progressPath)
	cfg.EnablePostProcessing = true
	cfg.GlossaryPath = "configs/glossary_example.yaml"
	cfg.ContentProtection = true
	cfg.TerminologyConsistency = true
	return cfg
}

// TestModelConfig 测试模型配置
var TestModelConfig = config.ModelConfig{
	Name:    "test-model",
	APIType: "openai",
	BaseURL: "http://localhost:8080",
	Key:     "test-key",
}

// TestStepSetConfig 测试步骤集配置
var TestStepSetConfig = config.StepSetConfig{
	ID:   "test-stepset",
	Name: "Test Step Set",
	InitialTranslation: config.StepConfig{
		Name:        "initial",
		ModelName:   "test-model",
		Temperature: 0.3,
	},
	Reflection: config.StepConfig{
		Name:        "reflect",
		ModelName:   "test-model",
		Temperature: 0.1,
	},
	Improvement: config.StepConfig{
		Name:        "improve",
		ModelName:   "test-model",
		Temperature: 0.3,
	},
}