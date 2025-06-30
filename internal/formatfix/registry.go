package formatfix

import (
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// FixerRegistry 格式修复器注册中心
type FixerRegistry struct {
	fixers      map[string]FormatFixer
	mu          sync.RWMutex
	logger      *zap.Logger
	toolManager ExternalTool
	toolChecker ToolChecker
	interactor  UserInteractor
}

// NewFixerRegistry 创建格式修复器注册中心
func NewFixerRegistry(logger *zap.Logger) *FixerRegistry {
	return &FixerRegistry{
		fixers: make(map[string]FormatFixer),
		logger: logger,
	}
}

// SetToolManager 设置工具管理器
func (fr *FixerRegistry) SetToolManager(toolManager ExternalTool) {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.toolManager = toolManager
}

// SetToolChecker 设置工具检查器
func (fr *FixerRegistry) SetToolChecker(toolChecker ToolChecker) {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.toolChecker = toolChecker
}

// SetInteractor 设置用户交互器
func (fr *FixerRegistry) SetInteractor(interactor UserInteractor) {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.interactor = interactor
}

// RegisterFixer 注册格式修复器
func (fr *FixerRegistry) RegisterFixer(fixer FormatFixer) error {
	if fixer == nil {
		return fmt.Errorf("修复器不能为空")
	}

	fr.mu.Lock()
	defer fr.mu.Unlock()

	name := fixer.GetName()
	if _, exists := fr.fixers[name]; exists {
		return fmt.Errorf("修复器 '%s' 已存在", name)
	}

	fr.fixers[name] = fixer
	fr.logger.Info("注册格式修复器",
		zap.String("name", name),
		zap.Strings("formats", fixer.GetSupportedFormats()),
	)

	return nil
}

// UnregisterFixer 注销格式修复器
func (fr *FixerRegistry) UnregisterFixer(name string) error {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	if _, exists := fr.fixers[name]; !exists {
		return fmt.Errorf("修复器 '%s' 不存在", name)
	}

	delete(fr.fixers, name)
	fr.logger.Info("注销格式修复器", zap.String("name", name))

	return nil
}

// GetFixer 根据名称获取修复器
func (fr *FixerRegistry) GetFixer(name string) (FormatFixer, error) {
	fr.mu.RLock()
	defer fr.mu.RUnlock()

	fixer, exists := fr.fixers[name]
	if !exists {
		return nil, fmt.Errorf("修复器 '%s' 不存在", name)
	}

	return fixer, nil
}

// GetFixerForFormat 根据文件格式获取修复器
func (fr *FixerRegistry) GetFixerForFormat(format string) (FormatFixer, error) {
	fr.mu.RLock()
	defer fr.mu.RUnlock()

	normalizedFormat := strings.ToLower(strings.TrimPrefix(format, "."))

	for _, fixer := range fr.fixers {
		for _, supportedFormat := range fixer.GetSupportedFormats() {
			if strings.ToLower(supportedFormat) == normalizedFormat {
				return fixer, nil
			}
		}
	}

	return nil, fmt.Errorf("没有找到支持格式 '%s' 的修复器", format)
}

// ListFixers 列出所有注册的修复器
func (fr *FixerRegistry) ListFixers() []string {
	fr.mu.RLock()
	defer fr.mu.RUnlock()

	var names []string
	for name := range fr.fixers {
		names = append(names, name)
	}

	return names
}

// GetSupportedFormats 获取所有支持的格式
func (fr *FixerRegistry) GetSupportedFormats() []string {
	fr.mu.RLock()
	defer fr.mu.RUnlock()

	formatSet := make(map[string]bool)
	for _, fixer := range fr.fixers {
		for _, format := range fixer.GetSupportedFormats() {
			formatSet[strings.ToLower(format)] = true
		}
	}

	var formats []string
	for format := range formatSet {
		formats = append(formats, format)
	}

	return formats
}

// IsFormatSupported 检查是否支持指定格式
func (fr *FixerRegistry) IsFormatSupported(format string) bool {
	_, err := fr.GetFixerForFormat(format)
	return err == nil
}

// GetFixerInfo 获取修复器信息
func (fr *FixerRegistry) GetFixerInfo() map[string][]string {
	fr.mu.RLock()
	defer fr.mu.RUnlock()

	info := make(map[string][]string)
	for name, fixer := range fr.fixers {
		info[name] = fixer.GetSupportedFormats()
	}

	return info
}

// CheckAllIssues 检查所有格式的问题
func (fr *FixerRegistry) CheckAllIssues(content []byte, format string) ([]*FixIssue, error) {
	fixer, err := fr.GetFixerForFormat(format)
	if err != nil {
		return nil, err
	}

	return fixer.CheckIssues(content)
}

// AutoFixAll 自动修复所有格式的问题
func (fr *FixerRegistry) AutoFixAll(content []byte, format string) ([]byte, []*FixIssue, error) {
	fixer, err := fr.GetFixerForFormat(format)
	if err != nil {
		return content, nil, err
	}

	return fixer.AutoFix(content)
}

// GetStats 获取统计信息
func (fr *FixerRegistry) GetStats() map[string]interface{} {
	fr.mu.RLock()
	defer fr.mu.RUnlock()

	stats := map[string]interface{}{
		"total_fixers":      len(fr.fixers),
		"registered_fixers": fr.ListFixers(),
		"supported_formats": fr.GetSupportedFormats(),
		"fixer_info":        fr.GetFixerInfo(),
	}

	return stats
}

// Validate 验证注册中心状态
func (fr *FixerRegistry) Validate() error {
	fr.mu.RLock()
	defer fr.mu.RUnlock()

	if len(fr.fixers) == 0 {
		return fmt.Errorf("没有注册任何格式修复器")
	}

	// 检查每个修复器的有效性
	for name, fixer := range fr.fixers {
		if fixer == nil {
			return fmt.Errorf("修复器 '%s' 为空", name)
		}

		if len(fixer.GetSupportedFormats()) == 0 {
			return fmt.Errorf("修复器 '%s' 没有支持任何格式", name)
		}
	}

	return nil
}

// Clear 清空所有注册的修复器
func (fr *FixerRegistry) Clear() {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	fr.fixers = make(map[string]FormatFixer)
	fr.logger.Info("清空所有格式修复器")
}
