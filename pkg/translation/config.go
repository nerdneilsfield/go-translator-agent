package translation

import (
	"errors"
	"time"
)

// Config 翻译器独立配置，不依赖外部配置框架
type Config struct {
	// 语言配置
	SourceLanguage string `json:"source_language"`
	TargetLanguage string `json:"target_language"`

	// 分块配置
	ChunkSize      int `json:"chunk_size"`
	ChunkOverlap   int `json:"chunk_overlap"`
	MaxConcurrency int `json:"max_concurrency"`

	// 重试配置
	MaxRetries int           `json:"max_retries"`
	RetryDelay time.Duration `json:"retry_delay"`

	// 超时配置
	Timeout time.Duration `json:"timeout"`

	// 翻译步骤
	Steps []StepConfig `json:"steps"`

	// 缓存配置
	EnableCache bool   `json:"enable_cache"`
	CacheDir    string `json:"cache_dir"`

	// 元数据
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// StepConfig 翻译步骤配置
type StepConfig struct {
	Name        string            `json:"name"`        // 步骤名称
	Provider    string            `json:"provider"`    // 提供商名称（可选，如 "deepl", "openai"）
	Model       string            `json:"model"`       // 使用的模型
	Temperature float32           `json:"temperature"` // 温度参数
	MaxTokens   int               `json:"max_tokens"`  // 最大token数
	Timeout     time.Duration     `json:"timeout"`     // 超时时间
	Prompt      string            `json:"prompt"`      // 提示词模板
	Variables   map[string]string `json:"variables"`   // 提示词变量
	SystemRole  string            `json:"system_role"` // 系统角色
	IsLLM       bool              `json:"is_llm"`      // 是否是LLM模型（支持复杂推理和对话）
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
		ChunkSize:      1000,
		ChunkOverlap:   100,
		MaxConcurrency: 3,
		MaxRetries:     3,
		RetryDelay:     time.Second,
		Timeout:        5 * time.Minute,
		EnableCache:    true,
		CacheDir:       ".translation_cache",
		Steps: []StepConfig{
			{
				Name:        "initial_translation",
				Model:       "gpt-4",
				Temperature: 0.3,
				MaxTokens:   4096,
				Timeout:     2 * time.Minute,
				Prompt:      defaultTranslationPrompt,
				SystemRole:  "You are a professional translator.",
			},
			{
				Name:        "reflection",
				Model:       "gpt-4",
				Temperature: 0.1,
				MaxTokens:   2048,
				Timeout:     1 * time.Minute,
				Prompt:      defaultReflectionPrompt,
				SystemRole:  "You are a translation quality reviewer.",
			},
			{
				Name:        "improvement",
				Model:       "gpt-4",
				Temperature: 0.3,
				MaxTokens:   4096,
				Timeout:     2 * time.Minute,
				Prompt:      defaultImprovementPrompt,
				SystemRole:  "You are a professional translator focusing on quality improvement.",
			},
		},
	}
}

// Validate 验证配置的合法性
func (c *Config) Validate() error {
	if c.SourceLanguage == "" {
		return errors.New("source language is required")
	}
	if c.TargetLanguage == "" {
		return errors.New("target language is required")
	}
	if c.ChunkSize <= 0 {
		return errors.New("chunk size must be positive")
	}
	if c.MaxConcurrency <= 0 {
		return errors.New("max concurrency must be positive")
	}
	if len(c.Steps) == 0 {
		return errors.New("at least one translation step is required")
	}

	// 验证每个步骤
	var hasRawStep bool
	
	for i, step := range c.Steps {
		if step.Name == "" {
			return errors.New("step name is required")
		}
		
		// 检查是否使用了 raw 步骤（raw 和 none 都视为 raw）
		if step.Model == "raw" || step.Model == "none" {
			if !hasRawStep {
				hasRawStep = true
			}
		} else {
			// 验证 raw 规则：一旦使用了 raw，后续步骤必须是 raw 或 none
			if hasRawStep {
				return errors.New("once a step uses 'raw' or 'none' model, all subsequent steps must use 'raw' or 'none' models")
			}
			
			// 验证第二步和第三步必须使用 LLM 模型（除非是特殊选项）
			if i > 0 && !step.IsLLM {
				return errors.New("reflection and improvement steps (position 2+) must use LLM models (is_llm: true) or special options (raw/none)")
			}
		}
		
		// Provider-based steps might not need model and prompt
		if step.Provider == "" {
			// Only require model and prompt for LLM-based steps
			if step.Model == "" {
				return errors.New("step model is required when no provider is specified")
			}
			if step.Prompt == "" {
				return errors.New("step prompt is required when no provider is specified")
			}
		}
		if step.Temperature < 0 || step.Temperature > 2 {
			return errors.New("temperature must be between 0 and 2")
		}
		if i > 0 && step.Name == c.Steps[i-1].Name {
			return errors.New("step names must be unique")
		}
	}

	return nil
}

// Clone 创建配置的深拷贝
func (c *Config) Clone() *Config {
	clone := *c
	clone.Steps = make([]StepConfig, len(c.Steps))
	for i, step := range c.Steps {
		clone.Steps[i] = step
		if step.Variables != nil {
			clone.Steps[i].Variables = make(map[string]string)
			for k, v := range step.Variables {
				clone.Steps[i].Variables[k] = v
			}
		}
	}
	return &clone
}

// 默认提示词模板
const (
	defaultTranslationPrompt = `Translate the following {{source_language}} text to {{target_language}}. 
Maintain the original meaning, tone, and style as much as possible.

Text to translate:
{{text}}`

	defaultReflectionPrompt = `Review the following translation from {{source_language}} to {{target_language}}.
Identify any issues with accuracy, fluency, cultural appropriateness, or style.

Original text:
{{original_text}}

Translation:
{{translation}}

Please provide specific feedback on what could be improved.`

	defaultImprovementPrompt = `Based on the feedback provided, improve the following translation from {{source_language}} to {{target_language}}.

Original text:
{{original_text}}

Current translation:
{{translation}}

Feedback:
{{feedback}}

Please provide an improved translation that addresses the feedback.`
)

// StepSetConfig 步骤集配置
type StepSetConfig struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Initial     StepConfig `json:"initial"`
	Reflection  StepConfig `json:"reflection"`
	Improvement StepConfig `json:"improvement"`
}
