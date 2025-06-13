package adapter

import (
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStepSetV2Conversion 测试新步骤集格式的转换
func TestStepSetV2Conversion(t *testing.T) {
	tests := []struct {
		name     string
		stepSet  config.StepSetConfigV2
		expected int // 期望的步骤数量
	}{
		{
			name: "基础三步翻译",
			stepSet: config.StepSetConfigV2{
				ID:          "basic",
				Name:        "基本翻译",
				Description: "测试基本翻译",
				Steps: []config.StepConfigV2{
					{
						Name:        "initial_translation",
						Provider:    "openai",
						ModelName:   "gpt-3.5-turbo",
						Temperature: 0.5,
						MaxTokens:   4096,
					},
					{
						Name:        "reflection",
						Provider:    "openai",
						ModelName:   "gpt-3.5-turbo",
						Temperature: 0.3,
						MaxTokens:   2048,
					},
					{
						Name:        "improvement",
						Provider:    "openai",
						ModelName:   "gpt-3.5-turbo",
						Temperature: 0.5,
						MaxTokens:   4096,
					},
				},
			},
			expected: 3,
		},
		{
			name: "混合提供商",
			stepSet: config.StepSetConfigV2{
				ID:          "mixed",
				Name:        "混合模式",
				Description: "使用多个提供商",
				Steps: []config.StepConfigV2{
					{
						Name:        "deepl_translation",
						Provider:    "deepl",
						ModelName:   "deepl",
						Temperature: 0,
					},
					{
						Name:        "ai_review",
						Provider:    "openai",
						ModelName:   "gpt-4",
						Temperature: 0.2,
						MaxTokens:   3000,
					},
				},
			},
			expected: 2,
		},
		{
			name: "单步快速翻译",
			stepSet: config.StepSetConfigV2{
				ID:          "fast",
				Name:        "快速翻译",
				Description: "仅使用一步",
				Steps: []config.StepConfigV2{
					{
						Name:      "translation",
						Provider:  "deeplx",
						ModelName: "deeplx",
					},
				},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				SourceLang: "English",
				TargetLang: "Chinese",
				ModelConfigs: map[string]config.ModelConfig{
					"gpt-3.5-turbo": {
						Name:    "gpt-3.5-turbo",
						APIType: "openai",
					},
					"gpt-4": {
						Name:    "gpt-4",
						APIType: "openai",
					},
					"deepl": {
						Name:    "deepl",
						APIType: "deepl",
					},
					"deeplx": {
						Name:    "deeplx",
						APIType: "deeplx",
					},
				},
			}

			steps := ConvertStepSetV2(tt.stepSet, cfg)
			assert.Len(t, steps, tt.expected)

			// 验证每个步骤的基本属性
			for i, step := range steps {
				assert.NotEmpty(t, step.Name)
				assert.NotEmpty(t, step.Provider)
				assert.NotEmpty(t, step.Model)
				assert.NotNil(t, step.Variables)
				assert.Equal(t, cfg.SourceLang, step.Variables["source"])
				assert.Equal(t, cfg.TargetLang, step.Variables["target"])

				// 验证提供商设置正确
				assert.Equal(t, tt.stepSet.Steps[i].Provider, step.Provider)
			}
		})
	}
}

// TestLegacyStepSetConversion 测试旧格式转换为新格式
func TestLegacyStepSetConversion(t *testing.T) {
	oldStepSet := config.StepSetConfig{
		ID:          "legacy",
		Name:        "旧格式",
		Description: "测试旧格式转换",
		InitialTranslation: config.StepConfig{
			Name:        "初始翻译",
			ModelName:   "gpt-3.5-turbo",
			Temperature: 0.5,
		},
		Reflection: config.StepConfig{
			Name:        "反思",
			ModelName:   "gpt-3.5-turbo",
			Temperature: 0.3,
		},
		Improvement: config.StepConfig{
			Name:        "改进",
			ModelName:   "gpt-3.5-turbo",
			Temperature: 0.5,
		},
		FastModeThreshold: 300,
	}

	newStepSet := oldStepSet.ToStepSetConfigV2()

	assert.Equal(t, oldStepSet.ID, newStepSet.ID)
	assert.Equal(t, oldStepSet.Name, newStepSet.Name)
	assert.Equal(t, oldStepSet.Description, newStepSet.Description)
	assert.Equal(t, oldStepSet.FastModeThreshold, newStepSet.FastModeThreshold)
	assert.True(t, newStepSet.Legacy)
	assert.Len(t, newStepSet.Steps, 3)

	// 验证步骤内容
	assert.Equal(t, "initial_translation", newStepSet.Steps[0].Name)
	assert.Equal(t, oldStepSet.InitialTranslation.ModelName, newStepSet.Steps[0].ModelName)
	assert.Equal(t, oldStepSet.InitialTranslation.Temperature, newStepSet.Steps[0].Temperature)
}

// TestMergeStepSets 测试合并新旧格式步骤集
func TestMergeStepSets(t *testing.T) {
	oldSets := map[string]config.StepSetConfig{
		"old1": {
			ID:          "old1",
			Name:        "旧格式1",
			Description: "旧格式步骤集1",
		},
		"shared": {
			ID:          "shared",
			Name:        "共享名称（旧）",
			Description: "这个会被新格式覆盖",
		},
	}

	newSets := map[string]config.StepSetConfigV2{
		"new1": {
			ID:          "new1",
			Name:        "新格式1",
			Description: "新格式步骤集1",
		},
		"shared": {
			ID:          "shared",
			Name:        "共享名称（新）",
			Description: "这个会覆盖旧格式",
		},
	}

	merged := config.MergeStepSets(oldSets, newSets)

	// 验证合并结果
	assert.Len(t, merged, 3)
	assert.Contains(t, merged, "old1")
	assert.Contains(t, merged, "new1")
	assert.Contains(t, merged, "shared")

	// 验证新格式优先
	assert.Equal(t, "共享名称（新）", merged["shared"].Name)
	assert.Equal(t, "这个会覆盖旧格式", merged["shared"].Description)

	// 验证旧格式被转换
	assert.True(t, merged["old1"].Legacy)
}

// TestDefaultStepSetsV2 测试默认步骤集
func TestDefaultStepSetsV2(t *testing.T) {
	defaults := config.GetDefaultStepSetsV2()

	// 验证包含预期的默认步骤集
	expectedSets := []string{"basic", "professional", "mixed", "fast", "quality"}
	for _, expected := range expectedSets {
		assert.Contains(t, defaults, expected)
	}

	// 验证每个步骤集的基本结构
	for id, stepSet := range defaults {
		assert.Equal(t, id, stepSet.ID)
		assert.NotEmpty(t, stepSet.Name)
		assert.NotEmpty(t, stepSet.Description)
		assert.NotEmpty(t, stepSet.Steps)

		// 验证每个步骤都有必要的字段
		for _, step := range stepSet.Steps {
			assert.NotEmpty(t, step.Name)
			assert.NotEmpty(t, step.Provider)
			assert.NotEmpty(t, step.ModelName)
		}
	}

	// 特别验证混合模式
	mixed := defaults["mixed"]
	assert.Greater(t, len(mixed.Steps), 2, "混合模式应该有多个步骤")
	
	// 验证使用了不同的提供商
	providers := make(map[string]bool)
	for _, step := range mixed.Steps {
		providers[step.Provider] = true
	}
	assert.Greater(t, len(providers), 1, "混合模式应该使用多个提供商")
}

// TestConfigAdapterWithV2StepSets 测试配置适配器处理V2步骤集
func TestConfigAdapterWithV2StepSets(t *testing.T) {
	cfg := &config.Config{
		SourceLang:       "English",
		TargetLang:       "Chinese",
		DefaultModelName: "gpt-3.5-turbo",
		ActiveStepSet:    "mixed_test",
		StepSetsV2: map[string]config.StepSetConfigV2{
			"mixed_test": {
				ID:          "mixed_test",
				Name:        "混合测试",
				Description: "测试混合提供商",
				Steps: []config.StepConfigV2{
					{
						Name:        "deepl_step",
						Provider:    "deepl",
						ModelName:   "deepl",
						Temperature: 0,
						Prompt:      "{{text}}",
					},
					{
						Name:        "openai_step",
						Provider:    "openai",
						ModelName:   "gpt-4",
						Temperature: 0.3,
						MaxTokens:   4096,
						Prompt:      "Review: {{deepl_step}}",
					},
				},
			},
		},
		ModelConfigs: map[string]config.ModelConfig{
			"deepl": {
				Name:    "deepl",
				APIType: "deepl",
			},
			"gpt-4": {
				Name:    "gpt-4",
				APIType: "openai",
			},
		},
		TranslationTimeout: 300,
	}

	translationConfig, err := ConvertConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, translationConfig)

	// 验证步骤正确转换
	assert.Len(t, translationConfig.Steps, 2)
	
	// 验证第一步（DeepL）
	assert.Equal(t, "deepl_step", translationConfig.Steps[0].Name)
	assert.Equal(t, "deepl", translationConfig.Steps[0].Provider)
	assert.Equal(t, "deepl", translationConfig.Steps[0].Model)
	
	// 验证第二步（OpenAI）
	assert.Equal(t, "openai_step", translationConfig.Steps[1].Name)
	assert.Equal(t, "openai", translationConfig.Steps[1].Provider)
	assert.Equal(t, "gpt-4", translationConfig.Steps[1].Model)
}

// TestProviderInference 测试提供商推断
func TestProviderInference(t *testing.T) {
	tests := []struct {
		modelName        string
		expectedProvider string
	}{
		{"gpt-3.5-turbo", "openai"},
		{"gpt-4", "openai"},
		{"claude-3-opus", "anthropic"},
		{"deepl", "deepl"},
		{"gemini-pro", "google"},
		{"llama2", "ollama"},
		{"unknown-model", "openai"}, // 默认
	}

	for _, tt := range tests {
		t.Run(tt.modelName, func(t *testing.T) {
			provider := InferProviderFromModel(tt.modelName)
			assert.Equal(t, tt.expectedProvider, provider)
		})
	}
}