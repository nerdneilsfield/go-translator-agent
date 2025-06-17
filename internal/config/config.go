package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// ModelConfig 保存模型配置
type ModelConfig struct {
	Name             string   `mapstructure:"name"`
	ModelID          string   `mapstructure:"model_id"`
	APIType          string   `mapstructure:"api_type"`
	BaseURL          string   `mapstructure:"base_url"`
	Key              string   `mapstructure:"key"`
	MaxOutputTokens  int      `mapstructure:"max_output_tokens"`
	MaxInputTokens   int      `mapstructure:"max_input_tokens"`
	Temperature      float64  `mapstructure:"temperature"`
	InputTokenPrice  float64  `mapstructure:"input_token_price"`  // 1M Token 的价格
	OutputTokenPrice float64  `mapstructure:"output_token_price"` // 1M Token 的价格
	PriceUnit        string   `mapstructure:"price_unit"`         // 价格单位
	IsReasoning      bool     `mapstructure:"is_reasoning"`       // 是否是推理模型
	ReasoningTags    []string `mapstructure:"reasoning_tags"`     // 推理过程标记（如 ["<think>", "</think>"]）
	IsLLM            bool     `mapstructure:"is_llm"`             // 是否是LLM模型（支持复杂推理和对话）
}

// Deprecated: Use StepConfigV2 instead
type StepConfig struct {
	Name        string  `mapstructure:"name"`
	ModelName   string  `mapstructure:"model_name"`
	Temperature float64 `mapstructure:"temperature"`
}

// Deprecated: Use StepSetConfigV2 instead
type StepSetConfig struct {
	ID                 string     `mapstructure:"id"`
	Name               string     `mapstructure:"name"`
	Description        string     `mapstructure:"description"`
	InitialTranslation StepConfig `mapstructure:"initial_translation"`
	Reflection         StepConfig `mapstructure:"reflection"`
	Improvement        StepConfig `mapstructure:"improvement"`
	FastModeThreshold  int        `mapstructure:"fast_mode_threshold"`
}

// Config 保存翻译器的所有配置
type Config struct {
	SourceLang              string                     `mapstructure:"source_lang"`
	TargetLang              string                     `mapstructure:"target_lang"`
	Country                 string                     `mapstructure:"country"`
	DefaultModelName        string                     `mapstructure:"default_model_name"`
	ModelConfigs            map[string]ModelConfig     `mapstructure:"models"`
	StepSets                map[string]StepSetConfigV2 `mapstructure:"step_sets"` // 步骤集配置
	ActiveStepSet           string                     `mapstructure:"active_step_set"`
	MaxTokensPerChunk       int                        `mapstructure:"max_tokens_per_chunk"`
	CacheDir                string                     `mapstructure:"cache_dir"`
	UseCache                bool                       `mapstructure:"use_cache"`
	Debug                   bool                       `mapstructure:"debug"`
	Verbose                 bool                       `mapstructure:"verbose"`                   // 详细模式，显示翻译片段
	
	// 详细日志配置
	LogLevel                string                     `mapstructure:"log_level"`                 // 基础日志级别
	EnableDetailedLog       bool                       `mapstructure:"enable_detailed_log"`       // 是否启用详细日志
	ConsoleLogLevel         string                     `mapstructure:"console_log_level"`         // 控制台日志级别
	NormalLogFile           string                     `mapstructure:"normal_log_file"`           // 普通日志文件路径
	DetailedLogFile         string                     `mapstructure:"detailed_log_file"`         // 详细日志文件路径
	RequestTimeout          int                        `mapstructure:"request_timeout"`           // 请求超时时间（秒）
	Concurrency             int                        `mapstructure:"concurrency"`               // 并行翻译请求数
	HtmlConcurrency         int                        `mapstructure:"html_concurrency"`          // 并行HTML翻译请求数(每个 html 内部翻译请求数)
	EpubConcurrency         int                        `mapstructure:"epub_concurrency"`          // 并行EPUB翻译请求数（同时翻译几个内部的 html)
	MinSplitSize            int                        `mapstructure:"min_split_size"`            // 最小分割大小（字符数）
	MaxSplitSize            int                        `mapstructure:"max_split_size"`            // 最大分割大小（字符数）
	FilterReasoning         bool                       `mapstructure:"filter_reasoning"`          // 是否过滤推理过程
	AutoSaveInterval        int                        `mapstructure:"auto_save_interval"`        // 自动保存间隔（秒）
	TranslationTimeout      int                        `mapstructure:"translation_timeout"`       // 翻译超时时间（秒）
	PreserveMath            bool                       `mapstructure:"preserve_math"`             // 保留数学公式
	TranslateFigureCaptions bool                       `mapstructure:"translate_figure_captions"` // 翻译图表标题
	RetryFailedParts        bool                       `mapstructure:"retry_failed_parts"`        // 重试失败的部分
	MaxRetries              int                        `mapstructure:"max_retries"`               // 最大重试次数
	PostProcessMarkdown     bool                       `mapstructure:"post_process_markdown"`     // 翻译后对 Markdown 进行后处理
	FixMathFormulas         bool                       `mapstructure:"fix_math_formulas"`         // 修复数学公式
	FixTableFormat          bool                       `mapstructure:"fix_table_format"`          // 修复表格格式
	FixMixedContent         bool                       `mapstructure:"fix_mixed_content"`         // 修复混合内容（中英文混合）
	FixPicture              bool                       `mapstructure:"fix_picture"`               // 修复图片
	TargetCurrency          string                     `mapstructure:"target_currency"`           // 目标货币单位 (例如 USD, RMB), 空字符串表示不转换
	UsdRmbRate              float64                    `mapstructure:"usd_rmb_rate"`              // USD 到 RMB 的汇率, 用于成本估算时的货币转换
	KeepIntermediateFiles   bool                       `mapstructure:"keep_intermediate_files"`   // 是否保留中间文件（如EPUB解压的临时文件夹）
	SaveDebugInfo           bool                       `mapstructure:"save_debug_info"`           // 是否保存调试信息到 JSON 文件
	ChunkSize               int                        `mapstructure:"chunk_size"`                // 分块大小
	RetryAttempts           int                        `mapstructure:"retry_attempts"`            // 重试次数
	Metadata                map[string]interface{}     `mapstructure:"metadata"`                  // 元数据

	// 格式修复配置
	EnableFormatFix      bool `mapstructure:"enable_format_fix"`      // 启用格式修复
	FormatFixInteractive bool `mapstructure:"format_fix_interactive"` // 交互式格式修复
	PreTranslationFix    bool `mapstructure:"pre_translation_fix"`    // 翻译前格式修复
	PostTranslationFix   bool `mapstructure:"post_translation_fix"`   // 翻译后格式修复
	UseExternalTools     bool `mapstructure:"use_external_tools"`     // 使用外部工具（如markdownlint、prettier）
	FormatFixMarkdown    bool `mapstructure:"format_fix_markdown"`    // 启用Markdown格式修复
	FormatFixText        bool `mapstructure:"format_fix_text"`        // 启用Text格式修复
	FormatFixHTML        bool `mapstructure:"format_fix_html"`        // 启用HTML格式修复
	FormatFixEPUB        bool `mapstructure:"format_fix_epub"`        // 启用EPUB格式修复

	// 翻译后处理配置
	EnablePostProcessing      bool   `mapstructure:"enable_post_processing"`      // 启用翻译后处理
	GlossaryPath              string `mapstructure:"glossary_path"`               // 词汇表文件路径
	ContentProtection         bool   `mapstructure:"content_protection"`          // 内容保护
	TerminologyConsistency    bool   `mapstructure:"terminology_consistency"`     // 术语一致性检查
	MixedLanguageSpacing      bool   `mapstructure:"mixed_language_spacing"`      // 中英文混排空格优化
	MachineTranslationCleanup bool   `mapstructure:"machine_translation_cleanup"` // 机器翻译痕迹清理

	// HTML/EPUB 处理配置
	HTMLProcessingMode string `mapstructure:"html_processing_mode"` // HTML处理模式: "markdown" 或 "native"，默认 "markdown"
}

// LoadConfig 从文件加载配置
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	// 设置默认值
	setDefaults(v)

	// 如果配置路径已指定，则直接使用
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// 查找家目录中的配置文件
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		// 添加可能的配置文件路径
		v.AddConfigPath(home)
		v.AddConfigPath(".")
		v.SetConfigName(".translator")
		v.SetConfigType("yaml")
	}

	// 读取环境变量
	v.AutomaticEnv()
	v.SetEnvPrefix("TRANSLATOR")

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		// 如果找不到配置文件，则使用默认值
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, err
	}
	
	// Fix for models with dots in names: manually unmarshal models section
	modelsRaw := v.GetStringMap("models")
	if len(modelsRaw) > 0 {
		config.ModelConfigs = make(map[string]ModelConfig)
		for modelName := range modelsRaw {
			var modelCfg ModelConfig
			subKey := fmt.Sprintf("models.%s", modelName)
			if err := v.UnmarshalKey(subKey, &modelCfg); err == nil {
				config.ModelConfigs[modelName] = modelCfg
			}
		}
	}

	// 设置缓存目录（如果未设置）
	if config.CacheDir == "" {
		config.CacheDir = getDefaultCacheDir()
	}

	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	return &config, nil
}

// SaveConfig 将配置保存到文件
func SaveConfig(config *Config, configPath string) error {
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		configPath = filepath.Join(home, ".translator.yaml")
	}

	v := viper.New()
	v.SetConfigFile(configPath)

	// 添加所有配置项
	if err := v.MergeConfigMap(structToMap(config)); err != nil {
		return err
	}

	// 创建父目录（如果不存在）
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	return v.WriteConfig()
}

// NewDefaultConfig 创建一个新的默认配置
func NewDefaultConfig() *Config {
	// 获取系统缓存目录
	cacheDir := getDefaultCacheDir()

	return &Config{
		SourceLang:              "English",
		TargetLang:              "Chinese",
		Country:                 "China",
		DefaultModelName:        "gpt-3.5-turbo",
		ModelConfigs:            DefaultModelConfigs(),
		StepSets:                GetDefaultStepSetsV2(),
		ActiveStepSet:           "basic",
		MaxTokensPerChunk:       2000,
		CacheDir:                cacheDir,
		UseCache:                true,
		Debug:                   false,
		
		// 详细日志默认配置
		LogLevel:                "info",              // 默认INFO级别
		EnableDetailedLog:       false,               // 默认禁用详细日志
		ConsoleLogLevel:         "info",              // 控制台默认INFO级别
		NormalLogFile:           "",                  // 默认不输出到文件
		DetailedLogFile:         "logs/detailed.log", // 默认详细日志文件路径
		
		RequestTimeout:          300,  // 默认5分钟超时
		Concurrency:             4,    // 默认4个并行请求
		MinSplitSize:            100,  // 默认最小分割大小100字符
		MaxSplitSize:            1000, // 默认最大分割大小1000字符
		FilterReasoning:         true, // 默认过滤推理过程
		AutoSaveInterval:        300,  // 默认自动保存间隔300秒
		TranslationTimeout:      300,  // 默认翻译超时时间300秒
		PreserveMath:            false,
		TranslateFigureCaptions: false,
		RetryFailedParts:        false,
		MaxRetries:              3,
		PostProcessMarkdown:     true,
		FixMathFormulas:         false,
		FixTableFormat:          false,
		FixMixedContent:         false,
		FixPicture:              false,
		TargetCurrency:          "",    // 默认不指定目标货币，按原始单位显示
		UsdRmbRate:              7.4,   // 默认 USD 到 RMB 汇率
		KeepIntermediateFiles:   false, // 默认不保留中间文件
		SaveDebugInfo:           false,
		ChunkSize:               2000, // 默认分块大小2000字符
		RetryAttempts:           3,    // 默认重试3次
		Metadata:                make(map[string]interface{}),

		// HTML/EPUB 处理配置
		HTMLProcessingMode:      "markdown", // 默认使用markdown模式
	}
}

// getDefaultCacheDir 获取默认缓存目录
func getDefaultCacheDir() string {
	// 优先使用系统缓存目录
	cacheDir, err := os.UserCacheDir()
	if err == nil {
		return filepath.Join(cacheDir, "translator")
	}

	// 如果无法获取系统缓存目录，使用用户主目录
	homeDir, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(homeDir, ".translator", "cache")
	}

	// 最后的兜底方案
	return "./translator-cache"
}

// DefaultModelConfigs 返回默认模型配置
func DefaultModelConfigs() map[string]ModelConfig {
	return map[string]ModelConfig{
		"gpt-3.5-turbo": {
			Name:             "gpt-3.5-turbo",
			ModelID:          "gpt-3.5-turbo",
			APIType:          "openai",
			BaseURL:          "",
			Key:              "",
			MaxOutputTokens:  4096,
			MaxInputTokens:   4096,
			Temperature:      0.7,
			InputTokenPrice:  0.5,
			OutputTokenPrice: 1.5,
			PriceUnit:        "USD",
			IsReasoning:      false,
		},
		"gpt-4o": {
			Name:             "gpt-4",
			ModelID:          "gpt-4",
			APIType:          "openai",
			BaseURL:          "",
			Key:              "",
			MaxOutputTokens:  8192,
			MaxInputTokens:   8192,
			Temperature:      0.7,
			InputTokenPrice:  2.5,
			OutputTokenPrice: 10,
			PriceUnit:        "USD",
			IsReasoning:      false,
		},
		"o1-preview": {
			Name:             "o1-preview",
			ModelID:          "o1-preview",
			APIType:          "openai",
			BaseURL:          "",
			Key:              "",
			MaxOutputTokens:  32768,
			MaxInputTokens:   128000,
			Temperature:      1,
			InputTokenPrice:  15,
			OutputTokenPrice: 60,
			PriceUnit:        "USD",
			IsReasoning:      true,
			// OpenAI o1 的内部推理已经被隐藏，API 不会返回
		},
		"o1-mini": {
			Name:             "o1-mini",
			ModelID:          "o1-mini",
			APIType:          "openai",
			BaseURL:          "",
			Key:              "",
			MaxOutputTokens:  65536,
			MaxInputTokens:   128000,
			Temperature:      1,
			InputTokenPrice:  3,
			OutputTokenPrice: 12,
			PriceUnit:        "USD",
			IsReasoning:      true,
		},
		"deepseek-r1": {
			Name:             "deepseek-r1",
			ModelID:          "deepseek-r1",
			APIType:          "openai",
			BaseURL:          "https://api.deepseek.com/v1",
			Key:              "",
			MaxOutputTokens:  8192,
			MaxInputTokens:   32768,
			Temperature:      0.7,
			InputTokenPrice:  0.14,
			OutputTokenPrice: 2.19,
			PriceUnit:        "USD",
			IsReasoning:      true,
			ReasoningTags:    []string{"<think>", "</think>"},
		},
		"qwq-32b": {
			Name:            "qwq-32b",
			ModelID:         "qwq-32b-preview",
			APIType:         "openai",
			BaseURL:         "",
			Key:             "",
			MaxOutputTokens: 32768,
			MaxInputTokens:  32768,
			Temperature:     0.7,
			IsReasoning:     true,
			ReasoningTags:   []string{"<think>", "</think>"},
		},
		"qwen-plus": {
			Name:            "qwen-plus",
			ModelID:         "qwen-plus",
			APIType:         "openai",
			BaseURL:         "https://dashscope.aliyuncs.com/compatible-mode/v1",
			Key:             "",
			MaxOutputTokens: 8192,
			MaxInputTokens:  8192,
			Temperature:     0.7,
			IsReasoning:     false,
		},
		"claude-3-opus": {
			Name:            "claude-3-opus",
			ModelID:         "claude-3-opus-20240229",
			APIType:         "anthropic",
			BaseURL:         "",
			Key:             "",
			MaxOutputTokens: 4096,
			MaxInputTokens:  4096,
			Temperature:     0.7,
			IsReasoning:     false,
		},
		"claude-3-sonnet": {
			Name:            "claude-3-sonnet",
			ModelID:         "claude-3-sonnet-20240229",
			APIType:         "anthropic",
			BaseURL:         "",
			Key:             "",
			MaxOutputTokens: 4096,
			MaxInputTokens:  4096,
			Temperature:     0.7,
			IsReasoning:     false,
		},
		"mistral-large": {
			Name:            "mistral-large",
			ModelID:         "mistral-large-latest",
			APIType:         "mistral",
			BaseURL:         "",
			Key:             "",
			MaxOutputTokens: 8192,
			MaxInputTokens:  8192,
			Temperature:     0.7,
			IsReasoning:     false,
		},
	}
}

// Deprecated: Use GetDefaultStepSetsV2 instead
func DefaultStepSets() map[string]StepSetConfig {
	return map[string]StepSetConfig{
		"basic": {
			ID:          "basic",
			Name:        "基本翻译",
			Description: "基本的三步翻译过程",
			InitialTranslation: StepConfig{
				Name:        "初始翻译",
				ModelName:   "gpt-3.5-turbo",
				Temperature: 0.5,
			},
			Reflection: StepConfig{
				Name:        "反思",
				ModelName:   "gpt-3.5-turbo",
				Temperature: 0.3,
			},
			Improvement: StepConfig{
				Name:        "改进",
				ModelName:   "gpt-3.5-turbo",
				Temperature: 0.5,
			},
			FastModeThreshold: 300,
		},
		"quality": {
			ID:          "quality",
			Name:        "高质量翻译",
			Description: "使用高级模型的三步翻译过程",
			InitialTranslation: StepConfig{
				Name:        "初始翻译",
				ModelName:   "gpt-4-turbo",
				Temperature: 0.5,
			},
			Reflection: StepConfig{
				Name:        "反思",
				ModelName:   "claude-3-opus",
				Temperature: 0.3,
			},
			Improvement: StepConfig{
				Name:        "改进",
				ModelName:   "gpt-4-turbo",
				Temperature: 0.5,
			},
			FastModeThreshold: 300,
		},
		"simple": {
			ID:          "simple",
			Name:        "简单翻译",
			Description: "仅执行初始翻译，不进行反思和改进",
			InitialTranslation: StepConfig{
				Name:        "初始翻译",
				ModelName:   "gpt-3.5-turbo",
				Temperature: 0.7,
			},
			Reflection: StepConfig{
				Name:        "反思",
				ModelName:   "none",
				Temperature: 0.0,
			},
			Improvement: StepConfig{
				Name:        "改进",
				ModelName:   "none",
				Temperature: 0.0,
			},
			FastModeThreshold: 300,
		},
	}
}

// setDefaults 设置默认值
func setDefaults(v *viper.Viper) {
	v.SetDefault("source_lang", "English")
	v.SetDefault("target_lang", "Chinese")
	v.SetDefault("country", "China")
	v.SetDefault("default_model_name", "gpt-3.5-turbo")
	v.SetDefault("active_step_set", "basic")
	v.SetDefault("max_tokens_per_chunk", 2000)
	v.SetDefault("use_cache", true)
	v.SetDefault("debug", false)
	v.SetDefault("request_timeout", 300)
	v.SetDefault("concurrency", 4)
	v.SetDefault("min_split_size", 100)
	v.SetDefault("max_split_size", 1000)
	v.SetDefault("filter_reasoning", true)
	v.SetDefault("auto_save_interval", 300)
	v.SetDefault("translation_timeout", 300)
	v.SetDefault("preserve_math", false)
	v.SetDefault("translate_figure_captions", false)
	v.SetDefault("retry_failed_parts", false)
	v.SetDefault("max_retries", 3)
	v.SetDefault("post_process_markdown", false)
	v.SetDefault("fix_math_formulas", false)
	v.SetDefault("fix_table_format", false)
	v.SetDefault("fix_mixed_content", false)
	v.SetDefault("fix_picture", false)
	v.SetDefault("target_currency", "")
	v.SetDefault("usd_rmb_rate", 7.4)
	v.SetDefault("keep_intermediate_files", false)
	v.SetDefault("save_debug_info", false)
	v.SetDefault("chunk_size", 2000)
	v.SetDefault("retry_attempts", 3)

	// 格式修复默认配置
	v.SetDefault("enable_format_fix", true)       // 默认启用格式修复
	v.SetDefault("format_fix_interactive", false) // 默认使用静默修复
	v.SetDefault("pre_translation_fix", true)     // 默认启用翻译前修复
	v.SetDefault("post_translation_fix", true)    // 默认启用翻译后修复
	v.SetDefault("use_external_tools", true)      // 默认尝试使用外部工具
	v.SetDefault("format_fix_markdown", true)     // 默认启用Markdown修复
	v.SetDefault("format_fix_text", true)         // 默认启用Text修复
	v.SetDefault("format_fix_html", false)        // HTML修复暂未实现
	v.SetDefault("format_fix_epub", false)        // EPUB修复暂未实现

	// HTML/EPUB 处理配置
	v.SetDefault("html_processing_mode", "markdown") // 默认使用markdown模式处理HTML

	// 设置默认的步骤集
	v.SetDefault("step_sets", GetDefaultStepSetsV2())
}

// structToMap 将结构体转换为map
func structToMap(config *Config) map[string]interface{} {
	return map[string]interface{}{
		"source_lang":               config.SourceLang,
		"target_lang":               config.TargetLang,
		"country":                   config.Country,
		"default_model_name":        config.DefaultModelName,
		"models":                    config.ModelConfigs,
		"step_sets":                 config.StepSets,
		"active_step_set":           config.ActiveStepSet,
		"max_tokens_per_chunk":      config.MaxTokensPerChunk,
		"cache_dir":                 config.CacheDir,
		"use_cache":                 config.UseCache,
		"debug":                     config.Debug,
		"request_timeout":           config.RequestTimeout,
		"concurrency":               config.Concurrency,
		"min_split_size":            config.MinSplitSize,
		"max_split_size":            config.MaxSplitSize,
		"filter_reasoning":          config.FilterReasoning,
		"auto_save_interval":        config.AutoSaveInterval,
		"translation_timeout":       config.TranslationTimeout,
		"preserve_math":             config.PreserveMath,
		"translate_figure_captions": config.TranslateFigureCaptions,
		"retry_failed_parts":        config.RetryFailedParts,
		"max_retries":               config.MaxRetries,
		"post_process_markdown":     config.PostProcessMarkdown,
		"fix_math_formulas":         config.FixMathFormulas,
		"fix_table_format":          config.FixTableFormat,
		"fix_mixed_content":         config.FixMixedContent,
		"fix_picture":               config.FixPicture,
		"target_currency":           config.TargetCurrency,
		"usd_rmb_rate":              config.UsdRmbRate,
		"keep_intermediate_files":   config.KeepIntermediateFiles,
		"save_debug_info":           config.SaveDebugInfo,
		"chunk_size":                config.ChunkSize,
		"retry_attempts":            config.RetryAttempts,
		"metadata":                  config.Metadata,

		// 格式修复配置
		"enable_format_fix":      config.EnableFormatFix,
		"format_fix_interactive": config.FormatFixInteractive,
		"pre_translation_fix":    config.PreTranslationFix,
		"post_translation_fix":   config.PostTranslationFix,
		"use_external_tools":     config.UseExternalTools,
		"format_fix_markdown":    config.FormatFixMarkdown,
		"format_fix_text":        config.FormatFixText,
		"format_fix_html":        config.FormatFixHTML,
		"format_fix_epub":        config.FormatFixEPUB,

		// HTML/EPUB 处理配置
		"html_processing_mode":   config.HTMLProcessingMode,
	}
}
