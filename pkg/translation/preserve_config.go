package translation

// ExtendedConfig 扩展配置，用于存储额外的翻译选项
type ExtendedConfig struct {
	*Config
	// 国家/地区
	Country string
	// 是否启用快速模式
	FastMode bool
	// 快速模式阈值
	FastModeThreshold int
	// 保护块配置
	PreserveConfig PreserveConfig
	// 额外的指令
	ExtraInstructions []string
}

// NewExtendedConfig 创建扩展配置
func NewExtendedConfig(baseConfig *Config) *ExtendedConfig {
	return &ExtendedConfig{
		Config:            baseConfig,
		FastModeThreshold: 100,
		PreserveConfig:    DefaultPreserveConfig,
		ExtraInstructions: make([]string, 0),
	}
}

// WithCountry 设置国家/地区
func (ec *ExtendedConfig) WithCountry(country string) *ExtendedConfig {
	ec.Country = country
	return ec
}

// WithFastMode 设置快速模式
func (ec *ExtendedConfig) WithFastMode(enabled bool, threshold int) *ExtendedConfig {
	ec.FastMode = enabled
	ec.FastModeThreshold = threshold
	return ec
}

// WithPreserveConfig 设置保护块配置
func (ec *ExtendedConfig) WithPreserveConfig(config PreserveConfig) *ExtendedConfig {
	ec.PreserveConfig = config
	return ec
}

// AddExtraInstruction 添加额外指令
func (ec *ExtendedConfig) AddExtraInstruction(instruction string) *ExtendedConfig {
	ec.ExtraInstructions = append(ec.ExtraInstructions, instruction)
	return ec
}
