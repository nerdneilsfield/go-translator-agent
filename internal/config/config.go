package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// ModelConfig 保存模型配置
type ModelConfig struct {
	Name             string  `mapstructure:"name"`
	ModelID          string  `mapstructure:"model_id"`
	APIType          string  `mapstructure:"api_type"`
	BaseURL          string  `mapstructure:"base_url"`
	Key              string  `mapstructure:"key"`
	MaxOutputTokens  int     `mapstructure:"max_output_tokens"`
	MaxInputTokens   int     `mapstructure:"max_input_tokens"`
	Temperature      float64 `mapstructure:"temperature"`
	InputTokenPrice  float64 `mapstructure:"input_token_price"`  // 1M Token 的价格
	OutputTokenPrice float64 `mapstructure:"output_token_price"` // 1M Token 的价格
	PriceUnit        string  `mapstructure:"price_unit"`         // 价格单位
}

// StepConfig 保存步骤配置
type StepConfig struct {
	Name        string  `mapstructure:"name"`
	ModelName   string  `mapstructure:"model_name"`
	Temperature float64 `mapstructure:"temperature"`
}

// StepSetConfig 保存步骤集配置
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
	SourceLang              string                   `mapstructure:"source_lang"`
	TargetLang              string                   `mapstructure:"target_lang"`
	Country                 string                   `mapstructure:"country"`
	DefaultModelName        string                   `mapstructure:"default_model_name"`
	ModelConfigs            map[string]ModelConfig   `mapstructure:"models"`
	StepSets                map[string]StepSetConfig `mapstructure:"step_sets"`
	ActiveStepSet           string                   `mapstructure:"active_step_set"`
	MaxTokensPerChunk       int                      `mapstructure:"max_tokens_per_chunk"`
	CacheDir                string                   `mapstructure:"cache_dir"`
	UseCache                bool                     `mapstructure:"use_cache"`
	Debug                   bool                     `mapstructure:"debug"`
	RequestTimeout          int                      `mapstructure:"request_timeout"`           // 请求超时时间（秒）
	Concurrency             int                      `mapstructure:"concurrency"`               // 并行翻译请求数
	HtmlConcurrency         int                      `mapstructure:"html_concurrency"`          // 并行HTML翻译请求数(每个 html 内部翻译请求数)
	EpubConcurrency         int                      `mapstructure:"epub_concurrency"`          // 并行EPUB翻译请求数（同时翻译几个内部的 html)
	MinSplitSize            int                      `mapstructure:"min_split_size"`            // 最小分割大小（字符数）
	MaxSplitSize            int                      `mapstructure:"max_split_size"`            // 最大分割大小（字符数）
	FilterReasoning         bool                     `mapstructure:"filter_reasoning"`          // 是否过滤推理过程
	AutoSaveInterval        int                      `mapstructure:"auto_save_interval"`        // 自动保存间隔（秒）
	TranslationTimeout      int                      `mapstructure:"translation_timeout"`       // 翻译超时时间（秒）
	PreserveMath            bool                     `mapstructure:"preserve_math"`             // 保留数学公式
	TranslateFigureCaptions bool                     `mapstructure:"translate_figure_captions"` // 翻译图表标题
	RetryFailedParts        bool                     `mapstructure:"retry_failed_parts"`        // 重试失败的部分
	PostProcessMarkdown     bool                     `mapstructure:"post_process_markdown"`     // 翻译后对 Markdown 进行后处理
	FixMathFormulas         bool                     `mapstructure:"fix_math_formulas"`         // 修复数学公式
	FixTableFormat          bool                     `mapstructure:"fix_table_format"`          // 修复表格格式
	FixMixedContent         bool                     `mapstructure:"fix_mixed_content"`         // 修复混合内容（中英文混合）
	FixPicture              bool                     `mapstructure:"fix_picture"`               // 修复图片
	TargetCurrency          string                   `mapstructure:"target_currency"`           // 目标货币单位 (例如 USD, RMB), 空字符串表示不转换
	UsdRmbRate              float64                  `mapstructure:"usd_rmb_rate"`              // USD 到 RMB 的汇率, 用于成本估算时的货币转换
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

	// 设置缓存目录（如果未设置）
	if config.CacheDir == "" {
		tmpDir := os.TempDir()
		config.CacheDir = filepath.Join(tmpDir, "translator-cache")
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return v.WriteConfig()
}

// NewDefaultConfig 创建一个新的默认配置
func NewDefaultConfig() *Config {
	return &Config{
		SourceLang:              "English",
		TargetLang:              "Chinese",
		Country:                 "China",
		DefaultModelName:        "gpt-3.5-turbo",
		ModelConfigs:            DefaultModelConfigs(),
		StepSets:                DefaultStepSets(),
		ActiveStepSet:           "basic",
		MaxTokensPerChunk:       2000,
		CacheDir:                "",
		UseCache:                true,
		Debug:                   false,
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
		PostProcessMarkdown:     true,
		FixMathFormulas:         false,
		FixTableFormat:          false,
		FixMixedContent:         false,
		FixPicture:              false,
		TargetCurrency:          "",  // 默认不指定目标货币，按原始单位显示
		UsdRmbRate:              7.4, // 默认 USD 到 RMB 汇率
	}
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
		},
		"qwen-plus": {
			Name:            "qwen-plus",
			ModelID:         "qwen-plus",
			APIType:         "openai-reasoning",
			BaseURL:         "https://dashscope.aliyuncs.com/compatible-mode/v1",
			Key:             "",
			MaxOutputTokens: 8192,
			MaxInputTokens:  8192,
			Temperature:     0.7,
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
		},
	}
}

// DefaultStepSets 返回默认步骤集
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
	v.SetDefault("post_process_markdown", false)
	v.SetDefault("fix_math_formulas", false)
	v.SetDefault("fix_table_format", false)
	v.SetDefault("fix_mixed_content", false)
	v.SetDefault("fix_picture", false)
	v.SetDefault("target_currency", "")
	v.SetDefault("usd_rmb_rate", 7.4)
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
		"post_process_markdown":     config.PostProcessMarkdown,
		"fix_math_formulas":         config.FixMathFormulas,
		"fix_table_format":          config.FixTableFormat,
		"fix_mixed_content":         config.FixMixedContent,
		"fix_picture":               config.FixPicture,
		"target_currency":           config.TargetCurrency,
		"usd_rmb_rate":              config.UsdRmbRate,
	}
}
