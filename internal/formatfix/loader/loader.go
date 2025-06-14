package loader

import (
	"github.com/nerdneilsfield/go-translator-agent/internal/formatfix"
	"github.com/nerdneilsfield/go-translator-agent/internal/formatfix/markdown"
	"github.com/nerdneilsfield/go-translator-agent/internal/formatfix/text"
	"go.uber.org/zap"
)

// LoaderConfig 加载器配置
type LoaderConfig struct {
	EnableMarkdown bool
	EnableText     bool
	EnableHTML     bool
	EnableEPUB     bool
	Logger         *zap.Logger
	ToolManager    formatfix.ExternalTool
	ToolChecker    formatfix.ToolChecker
	Interactor     formatfix.UserInteractor
}

// RegisterFixers 注册格式修复器
func RegisterFixers(registry *formatfix.FixerRegistry, config *LoaderConfig) error {
	// 注册 Markdown 修复器
	if config.EnableMarkdown {
		markdownFixer := markdown.NewMarkdownFixer(
			config.ToolManager,
			config.ToolChecker,
			config.Logger,
		)
		if err := registry.RegisterFixer(markdownFixer); err != nil {
			return err
		}
	}

	// 注册 Text 修复器
	if config.EnableText {
		textFixer := text.NewTextFixer(
			config.ToolManager,
			config.ToolChecker,
			config.Logger,
		)
		if err := registry.RegisterFixer(textFixer); err != nil {
			return err
		}
	}

	return nil
}

// CreateRegistry 创建完整的格式修复器注册中心
func CreateRegistry(logger *zap.Logger) (*formatfix.FixerRegistry, error) {
	// 创建基础注册中心
	registry := formatfix.NewFixerRegistry(logger)

	// 设置默认组件
	toolManager := formatfix.NewDefaultToolManager(logger)
	registry.SetToolManager(toolManager)
	registry.SetToolChecker(toolManager)
	registry.SetInteractor(formatfix.NewConsoleInteractor(true, true))

	// 注册修复器
	config := &LoaderConfig{
		EnableMarkdown: true,
		EnableText:     true,
		EnableHTML:     false,
		EnableEPUB:     false,
		Logger:         logger,
		ToolManager:    toolManager,
		ToolChecker:    toolManager,
		Interactor:     formatfix.NewConsoleInteractor(true, true),
	}

	if err := RegisterFixers(registry, config); err != nil {
		return nil, err
	}

	return registry, nil
}

// CreateSilentRegistry 创建静默修复注册中心
func CreateSilentRegistry(logger *zap.Logger) (*formatfix.FixerRegistry, error) {
	registry := formatfix.NewFixerRegistry(logger)

	toolManager := formatfix.NewDefaultToolManager(logger)
	registry.SetToolManager(toolManager)
	registry.SetToolChecker(toolManager)
	registry.SetInteractor(formatfix.NewSilentInteractor(true))

	config := &LoaderConfig{
		EnableMarkdown: true,
		EnableText:     true,
		EnableHTML:     false,
		EnableEPUB:     false,
		Logger:         logger,
		ToolManager:    toolManager,
		ToolChecker:    toolManager,
		Interactor:     formatfix.NewSilentInteractor(true),
	}

	if err := RegisterFixers(registry, config); err != nil {
		return nil, err
	}

	return registry, nil
}
