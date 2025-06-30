package formatfix

import (
	"fmt"

	"go.uber.org/zap"
)

// FactoryConfig 工厂配置
type FactoryConfig struct {
	EnableMarkdown bool
	EnableText     bool
	EnableHTML     bool
	EnableEPUB     bool
	Logger         *zap.Logger
	ToolManager    ExternalTool
	ToolChecker    ToolChecker
	Interactor     UserInteractor
}

// Factory 格式修复器工厂
type Factory struct {
	config   *FactoryConfig
	registry *FixerRegistry
}

// NewFactory 创建格式修复器工厂
func NewFactory(config *FactoryConfig) *Factory {
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}

	return &Factory{
		config:   config,
		registry: NewFixerRegistry(config.Logger),
	}
}

// CreateRegistry 创建并配置格式修复器注册中心
func (f *Factory) CreateRegistry() (*FixerRegistry, error) {
	// 设置工具管理器和检查器
	if f.config.ToolManager != nil {
		f.registry.SetToolManager(f.config.ToolManager)
	}

	if f.config.ToolChecker != nil {
		f.registry.SetToolChecker(f.config.ToolChecker)
	}

	if f.config.Interactor != nil {
		f.registry.SetInteractor(f.config.Interactor)
	}

	// 创建默认工具管理器（如果没有提供）
	if f.config.ToolManager == nil {
		f.config.ToolManager = NewDefaultToolManager(f.config.Logger)
		f.registry.SetToolManager(f.config.ToolManager)
	}

	if f.config.ToolChecker == nil {
		f.config.ToolChecker = f.config.ToolManager.(ToolChecker)
		f.registry.SetToolChecker(f.config.ToolChecker)
	}

	if f.config.Interactor == nil {
		f.config.Interactor = NewConsoleInteractor(true, true)
		f.registry.SetInteractor(f.config.Interactor)
	}

	// 注册修复器
	if err := f.registerFixers(); err != nil {
		return nil, fmt.Errorf("注册修复器失败: %w", err)
	}

	// 验证注册中心
	if err := f.registry.Validate(); err != nil {
		return nil, fmt.Errorf("验证注册中心失败: %w", err)
	}

	f.config.Logger.Info("格式修复器注册中心创建成功",
		zap.Strings("fixers", f.registry.ListFixers()),
		zap.Strings("formats", f.registry.GetSupportedFormats()),
	)

	return f.registry, nil
}

// registerFixers 注册所有启用的修复器
func (f *Factory) registerFixers() error {
	// 这个方法现在为空，修复器将通过外部调用注册
	// 避免循环导入问题

	f.config.Logger.Info("格式修复器注册方法已准备",
		zap.Bool("markdown_enabled", f.config.EnableMarkdown),
		zap.Bool("text_enabled", f.config.EnableText),
	)

	return nil
}

// CreateDefaultConfig 创建默认配置
func CreateDefaultConfig(logger *zap.Logger) *FactoryConfig {
	return &FactoryConfig{
		EnableMarkdown: true,
		EnableText:     true,
		EnableHTML:     false, // 暂未实现
		EnableEPUB:     false, // 暂未实现
		Logger:         logger,
		ToolManager:    nil, // 将使用默认实现
		ToolChecker:    nil, // 将使用默认实现
		Interactor:     nil, // 将使用默认实现
	}
}

// CreateWithDefaults 使用默认配置创建注册中心
func CreateWithDefaults(logger *zap.Logger) (*FixerRegistry, error) {
	config := CreateDefaultConfig(logger)
	factory := NewFactory(config)
	return factory.CreateRegistry()
}

// CreateInteractiveRegistry 创建交互式注册中心
func CreateInteractiveRegistry(logger *zap.Logger) (*FixerRegistry, error) {
	config := CreateDefaultConfig(logger)
	config.Interactor = NewConsoleInteractor(true, true)

	factory := NewFactory(config)
	return factory.CreateRegistry()
}

// CreateSilentRegistry 创建静默注册中心（自动修复）
func CreateSilentRegistry(logger *zap.Logger) (*FixerRegistry, error) {
	config := CreateDefaultConfig(logger)
	config.Interactor = NewSilentInteractor(true) // 自动应用所有修复

	factory := NewFactory(config)
	return factory.CreateRegistry()
}

// CreateTestRegistry 创建测试用注册中心
func CreateTestRegistry(logger *zap.Logger, responses []FixAction) (*FixerRegistry, error) {
	config := CreateDefaultConfig(logger)
	config.Interactor = NewTestInteractor(responses)

	factory := NewFactory(config)
	return factory.CreateRegistry()
}

// FormatFixerService 格式修复服务
type FormatFixerService struct {
	registry *FixerRegistry
	logger   *zap.Logger
}

// NewFormatFixerService 创建格式修复服务
func NewFormatFixerService(registry *FixerRegistry, logger *zap.Logger) *FormatFixerService {
	return &FormatFixerService{
		registry: registry,
		logger:   logger,
	}
}

// FixFormat 修复指定格式的内容
func (ffs *FormatFixerService) FixFormat(content []byte, format string, mode FixMode) ([]byte, []*FixIssue, error) {
	fixer, err := ffs.registry.GetFixerForFormat(format)
	if err != nil {
		return content, nil, err
	}

	switch mode {
	case FixModeCheck:
		issues, err := fixer.CheckIssues(content)
		return content, issues, err
	case FixModeAuto:
		return fixer.AutoFix(content)
	default:
		return content, nil, fmt.Errorf("不支持的修复模式: %v", mode)
	}
}

// FixMode 修复模式
type FixMode int

const (
	FixModeCheck FixMode = iota // 仅检查，不修复
	FixModeAuto                 // 自动修复
)

// GetSupportedFormats 获取支持的格式
func (ffs *FormatFixerService) GetSupportedFormats() []string {
	return ffs.registry.GetSupportedFormats()
}

// IsFormatSupported 检查是否支持指定格式
func (ffs *FormatFixerService) IsFormatSupported(format string) bool {
	return ffs.registry.IsFormatSupported(format)
}

// GetStats 获取服务统计信息
func (ffs *FormatFixerService) GetStats() map[string]interface{} {
	return ffs.registry.GetStats()
}
