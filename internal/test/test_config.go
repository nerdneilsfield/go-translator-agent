package test

import (
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
)

// CreateTestConfig 创建用于测试的配置
func CreateTestConfig() *config.Config {
	return &config.Config{
		SourceLang:       "English",
		TargetLang:       "Chinese",
		DefaultModelName: "test-model",
		ActiveStepSet:    "test-step-set",
		ModelConfigs: map[string]config.ModelConfig{
			"test-model": {
				Name:            "test-model",
				APIType:         "openai",
				BaseURL:         "",
				Key:             "sk-test",
				MaxInputTokens:  8000,
				MaxOutputTokens: 2000,
			},
			"raw": {
				Name:    "raw",
				APIType: "raw",
			},
		},
		StepSets: map[string]config.StepSetConfig{
			"test-step-set": {
				ID:          "test-step-set",
				Name:        "测试步骤集",
				Description: "用于测试的步骤集",
				InitialTranslation: config.StepConfig{
					Name:        "初始翻译",
					ModelName:   "test-model",
					Temperature: 0.5,
				},
				Reflection: config.StepConfig{
					Name:        "反思",
					ModelName:   "raw",
					Temperature: 0.3,
				},
				Improvement: config.StepConfig{
					Name:        "改进",
					ModelName:   "raw",
					Temperature: 0.5,
				},
				FastModeThreshold: 300,
			},
		},
	}
}
