package config

// StepConfigV2 新的步骤配置，支持提供商
type StepConfigV2 struct {
	Name        string            `mapstructure:"name" json:"name"`
	Provider    string            `mapstructure:"provider" json:"provider"`       // 提供商（如 openai, deepl, google）
	ModelName   string            `mapstructure:"model_name" json:"model_name"`   // 模型名称
	Temperature float64           `mapstructure:"temperature" json:"temperature"` // 温度参数
	MaxTokens   int               `mapstructure:"max_tokens" json:"max_tokens"`   // 最大令牌数
	Timeout     int               `mapstructure:"timeout" json:"timeout"`         // 超时时间（秒）
	Prompt      string            `mapstructure:"prompt" json:"prompt"`           // 提示词模板
	SystemRole  string            `mapstructure:"system_role" json:"system_role"` // 系统角色
	Variables   map[string]string `mapstructure:"variables" json:"variables"`     // 额外变量
}

// StepSetConfigV2 新的步骤集配置，支持灵活的步骤数量
type StepSetConfigV2 struct {
	ID                string          `mapstructure:"id" json:"id"`
	Name              string          `mapstructure:"name" json:"name"`
	Description       string          `mapstructure:"description" json:"description"`
	Steps             []StepConfigV2  `mapstructure:"steps" json:"steps"`                           // 灵活的步骤列表
	FastModeThreshold int             `mapstructure:"fast_mode_threshold" json:"fast_mode_threshold"` // 快速模式阈值
	
	// 兼容旧格式
	Legacy            bool            `mapstructure:"legacy" json:"legacy,omitempty"`                         // 是否是旧格式
	InitialTranslation *StepConfig    `mapstructure:"initial_translation" json:"initial_translation,omitempty"` // 兼容旧格式
	Reflection         *StepConfig    `mapstructure:"reflection" json:"reflection,omitempty"`                   // 兼容旧格式
	Improvement        *StepConfig    `mapstructure:"improvement" json:"improvement,omitempty"`                  // 兼容旧格式
}

// ToStepConfigV2 将旧格式的 StepConfig 转换为新格式
func (s StepConfig) ToStepConfigV2() StepConfigV2 {
	return StepConfigV2{
		Name:        s.Name,
		ModelName:   s.ModelName,
		Temperature: s.Temperature,
		// Provider 将在转换时根据模型名称推断
	}
}

// ToStepSetConfigV2 将旧格式的 StepSetConfig 转换为新格式
func (s StepSetConfig) ToStepSetConfigV2() StepSetConfigV2 {
	steps := []StepConfigV2{}
	
	// 转换三个固定步骤
	if s.InitialTranslation.ModelName != "" {
		step := s.InitialTranslation.ToStepConfigV2()
		step.Name = "initial_translation"
		steps = append(steps, step)
	}
	
	if s.Reflection.ModelName != "" {
		step := s.Reflection.ToStepConfigV2()
		step.Name = "reflection"
		steps = append(steps, step)
	}
	
	if s.Improvement.ModelName != "" {
		step := s.Improvement.ToStepConfigV2()
		step.Name = "improvement"
		steps = append(steps, step)
	}
	
	return StepSetConfigV2{
		ID:                s.ID,
		Name:              s.Name,
		Description:       s.Description,
		Steps:             steps,
		FastModeThreshold: s.FastModeThreshold,
		Legacy:            true,
	}
}

// GetDefaultStepSetsV2 返回默认的步骤集配置（新格式）
func GetDefaultStepSetsV2() map[string]StepSetConfigV2 {
	return map[string]StepSetConfigV2{
		"basic": {
			ID:          "basic",
			Name:        "基本翻译",
			Description: "使用单一模型的三步翻译过程",
			Steps: []StepConfigV2{
				{
					Name:        "initial_translation",
					Provider:    "openai",
					ModelName:   "gpt-3.5-turbo",
					Temperature: 0.5,
					MaxTokens:   4096,
					Prompt:      "Translate the following {{source}} text to {{target}}. Maintain the original meaning, tone, and style:\n\n{{text}}",
					SystemRole:  "You are a professional translator.",
				},
				{
					Name:        "reflection",
					Provider:    "openai",
					ModelName:   "gpt-3.5-turbo",
					Temperature: 0.3,
					MaxTokens:   2048,
					Prompt:      "Review this translation and identify any issues:\n\nOriginal: {{original_text}}\nTranslation: {{translation}}\n\nProvide specific feedback.",
					SystemRole:  "You are a translation quality reviewer.",
				},
				{
					Name:        "improvement",
					Provider:    "openai",
					ModelName:   "gpt-3.5-turbo",
					Temperature: 0.5,
					MaxTokens:   4096,
					Prompt:      "Improve this translation based on the feedback:\n\nOriginal: {{original_text}}\nTranslation: {{translation}}\nFeedback: {{feedback}}\n\nProvide an improved translation.",
					SystemRole:  "You are a professional translator focusing on quality improvement.",
				},
			},
			FastModeThreshold: 300,
		},
		"professional": {
			ID:          "professional",
			Name:        "专业翻译",
			Description: "使用专业翻译服务 + AI 优化",
			Steps: []StepConfigV2{
				{
					Name:        "initial_translation",
					Provider:    "deepl",
					ModelName:   "deepl",
					Temperature: 0,
					Prompt:      "{{text}}", // DeepL 不需要复杂提示词
				},
				{
					Name:        "reflection",
					Provider:    "openai",
					ModelName:   "gpt-4",
					Temperature: 0.3,
					MaxTokens:   2048,
					Prompt:      "Review this professional translation from {{source}} to {{target}}:\n\nOriginal: {{original_text}}\nTranslation: {{translation}}\n\nIdentify any cultural nuances, terminology issues, or areas for improvement.",
					SystemRole:  "You are an expert linguist specializing in {{source}} to {{target}} translation.",
				},
				{
					Name:        "improvement",
					Provider:    "openai",
					ModelName:   "gpt-4",
					Temperature: 0.3,
					MaxTokens:   4096,
					Prompt:      "Refine this translation based on the feedback, ensuring it sounds natural in {{target}}:\n\nOriginal: {{original_text}}\nTranslation: {{translation}}\nFeedback: {{feedback}}\n\nProvide a polished final translation.",
					SystemRole:  "You are a native {{target}} speaker and professional editor.",
				},
			},
			FastModeThreshold: 300,
		},
		"mixed": {
			ID:          "mixed",
			Name:        "混合模式",
			Description: "结合多个提供商的优势",
			Steps: []StepConfigV2{
				{
					Name:        "initial_translation_deepl",
					Provider:    "deepl",
					ModelName:   "deepl",
					Temperature: 0,
					Prompt:      "{{text}}",
				},
				{
					Name:        "initial_translation_google",
					Provider:    "google",
					ModelName:   "google-translate",
					Temperature: 0,
					Prompt:      "{{text}}",
				},
				{
					Name:        "comparison",
					Provider:    "openai",
					ModelName:   "gpt-4",
					Temperature: 0.2,
					MaxTokens:   3000,
					Prompt:      "Compare these two translations and create the best version:\n\nOriginal ({{source}}): {{original_text}}\n\nTranslation 1 (DeepL): {{initial_translation_deepl}}\nTranslation 2 (Google): {{initial_translation_google}}\n\nCombine the strengths of both to create an optimal {{target}} translation.",
					SystemRole:  "You are an expert translator and linguist.",
				},
				{
					Name:        "polish",
					Provider:    "openai",
					ModelName:   "gpt-4",
					Temperature: 0.3,
					MaxTokens:   4096,
					Prompt:      "Polish this final translation to ensure it reads naturally in {{target}}:\n\nTranslation: {{comparison}}\n\nMake any final adjustments for fluency and naturalness.",
					SystemRole:  "You are a native {{target}} speaker and professional editor.",
				},
			},
			FastModeThreshold: 500,
		},
		"fast": {
			ID:          "fast",
			Name:        "快速翻译",
			Description: "使用单一专业服务快速翻译",
			Steps: []StepConfigV2{
				{
					Name:        "translation",
					Provider:    "deeplx",  // 或 "libretranslate" 用于完全免费
					ModelName:   "deeplx",
					Temperature: 0,
					Prompt:      "{{text}}",
				},
			},
			FastModeThreshold: 10000,
		},
		"quality": {
			ID:          "quality",
			Name:        "高质量翻译",
			Description: "使用高级模型的多步翻译过程",
			Steps: []StepConfigV2{
				{
					Name:        "initial_translation",
					Provider:    "openai",
					ModelName:   "gpt-4",
					Temperature: 0.3,
					MaxTokens:   8192,
					Prompt:      "Translate the following {{source}} text to {{target}}. Pay careful attention to nuance, cultural context, and maintain the author's voice:\n\n{{text}}",
					SystemRole:  "You are a master translator with deep cultural knowledge of both {{source}} and {{target}} languages.",
				},
				{
					Name:        "reflection",
					Provider:    "anthropic",  // 未来支持
					ModelName:   "claude-3-opus",
					Temperature: 0.1,
					MaxTokens:   4096,
					Prompt:      "Critically analyze this translation for accuracy, cultural appropriateness, and stylistic fidelity:\n\nOriginal: {{original_text}}\nTranslation: {{translation}}\n\nProvide detailed feedback on any issues.",
					SystemRole:  "You are a translation critic and linguistic expert.",
				},
				{
					Name:        "improvement",
					Provider:    "openai",
					ModelName:   "gpt-4",
					Temperature: 0.3,
					MaxTokens:   8192,
					Prompt:      "Create the final, polished translation incorporating all feedback:\n\nOriginal: {{original_text}}\nDraft: {{translation}}\nFeedback: {{feedback}}\n\nEnsure the final version is publication-ready.",
					SystemRole:  "You are a senior translator and editor with decades of experience.",
				},
			},
			FastModeThreshold: 200,
		},
	}
}

// MergeStepSets 合并旧格式和新格式的步骤集
func MergeStepSets(oldSets map[string]StepSetConfig, newSets map[string]StepSetConfigV2) map[string]StepSetConfigV2 {
	result := make(map[string]StepSetConfigV2)
	
	// 先添加所有新格式的步骤集
	for k, v := range newSets {
		result[k] = v
	}
	
	// 转换并添加旧格式的步骤集（如果不存在同名的新格式）
	for k, v := range oldSets {
		if _, exists := result[k]; !exists {
			result[k] = v.ToStepSetConfigV2()
		}
	}
	
	return result
}