package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// LoadTranslatorConfig 智能加载配置（自动检测新旧格式）
func LoadTranslatorConfig(configPath string) (interface{}, error) {
	// 尝试加载新格式配置
	v2Config, err := LoadStepSetConfigV2WithPath(configPath)
	if err == nil && v2Config != nil && len(v2Config.Steps) > 0 {
		// 成功加载 V2 配置
		return v2Config, nil
	}

	// 回退到旧格式配置
	v1Config, err := LoadConfig(configPath)
	if err == nil && v1Config != nil {
		return v1Config, nil
	}

	return nil, fmt.Errorf("failed to load any valid configuration")
}

// LoadStepSetConfigV2WithPath 加载指定路径的 V2 配置
func LoadStepSetConfigV2WithPath(configPath string) (*StepSetConfigV2, error) {
	v := viper.New()

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// 默认配置路径
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		// 搜索配置文件
		v.AddConfigPath(home)
		v.AddConfigPath(".")
		v.SetConfigName(".translator")
		v.SetConfigType("yaml")
	}

	// 读取配置
	if err := v.ReadInConfig(); err != nil {
		// 如果没有配置文件，创建默认配置
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return createDefaultStepSetConfigV2(), nil
		}
		return nil, err
	}

	// 解析配置
	var config StepSetConfigV2
	if err := v.Unmarshal(&config); err != nil {
		return nil, err
	}

	// 验证配置
	if err := validateStepSetConfigV2(&config); err != nil {
		return nil, err
	}

	// 设置默认值
	setDefaultsStepSetConfigV2(&config)

	return &config, nil
}

// createDefaultStepSetConfigV2 创建默认的 V2 配置
func createDefaultStepSetConfigV2() *StepSetConfigV2 {
	return &StepSetConfigV2{
		ID:                "default",
		Name:              "Default Three-Step Translation",
		Description:       "Standard three-step translation process",
		FastModeThreshold: 100,
		Steps: []StepConfigV2{
			{
				Name:        "initial_translation",
				Provider:    "openai",
				ModelName:   "gpt-3.5-turbo",
				Temperature: 0.3,
				MaxTokens:   4096,
				Timeout:     30,
				Prompt:      "Translate the following text from {{source}} to {{target}}:\n\n{{text}}",
				SystemRole:  "You are a professional translator.",
			},
			{
				Name:        "reflection",
				Provider:    "openai",
				ModelName:   "gpt-3.5-turbo",
				Temperature: 0.1,
				MaxTokens:   2048,
				Timeout:     30,
				Prompt:      "Review the translation and identify any issues:\n\nOriginal: {{original}}\nTranslation: {{translation}}",
				SystemRole:  "You are a translation quality reviewer.",
			},
			{
				Name:        "improvement",
				Provider:    "openai",
				ModelName:   "gpt-3.5-turbo",
				Temperature: 0.3,
				MaxTokens:   4096,
				Timeout:     30,
				Prompt:      "Improve the translation based on feedback:\n\nOriginal: {{original}}\nTranslation: {{translation}}\nFeedback: {{feedback}}",
				SystemRole:  "You are a professional translator.",
			},
		},
	}
}

// validateStepSetConfigV2 验证 V2 配置
func validateStepSetConfigV2(config *StepSetConfigV2) error {
	if config.ID == "" {
		return fmt.Errorf("step set ID must be specified")
	}

	if config.Name == "" {
		return fmt.Errorf("step set name must be specified")
	}

	if len(config.Steps) == 0 {
		return fmt.Errorf("at least one step must be configured")
	}

	// 验证每个步骤
	for i, step := range config.Steps {
		if step.Name == "" {
			return fmt.Errorf("step %d: name must be specified", i)
		}
		if step.Provider == "" {
			return fmt.Errorf("step %d: provider must be specified", i)
		}
		if step.ModelName == "" {
			return fmt.Errorf("step %d: model name must be specified", i)
		}
	}

	return nil
}

// setDefaultsStepSetConfigV2 设置默认值
func setDefaultsStepSetConfigV2(config *StepSetConfigV2) {
	if config.FastModeThreshold <= 0 {
		config.FastModeThreshold = 100
	}

	// 为步骤设置默认值
	for i := range config.Steps {
		if config.Steps[i].Temperature == 0 {
			config.Steps[i].Temperature = 0.3
		}
		if config.Steps[i].MaxTokens == 0 {
			config.Steps[i].MaxTokens = 4096
		}
		if config.Steps[i].Timeout == 0 {
			config.Steps[i].Timeout = 30
		}
	}
}

// MigrateV1ToV2Config 将 V1 配置迁移到 V2
func MigrateV1ToV2Config(v1 *Config) *StepSetConfigV2 {
	// 从 V1 配置中获取默认步骤集
	defaultStepSetName := v1.ActiveStepSet
	if defaultStepSetName == "" {
		// 如果没有默认步骤集，使用第一个
		for name := range v1.StepSets {
			defaultStepSetName = name
			break
		}
	}

	var defaultStepSet StepSetConfigV2
	if stepSet, exists := v1.StepSets[defaultStepSetName]; exists {
		defaultStepSet = stepSet
	} else {
		// 创建默认步骤集
		defaultStepSet = StepSetConfigV2{
			ID:   "default",
			Name: "Default",
		}
	}

	return &defaultStepSet
}

// SaveStepSetConfigV2 保存 V2 配置
func SaveStepSetConfigV2(config *StepSetConfigV2, path string) error {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		path = filepath.Join(home, ".translator.yaml")
	}

	v := viper.New()
	v.SetConfigType("yaml")

	// 设置所有配置值
	v.Set("id", config.ID)
	v.Set("name", config.Name)
	v.Set("description", config.Description)
	v.Set("steps", config.Steps)
	v.Set("fast_mode_threshold", config.FastModeThreshold)
	v.Set("legacy", config.Legacy)

	// 写入文件
	return v.WriteConfigAs(path)
}
