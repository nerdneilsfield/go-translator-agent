package formatter

import (
	"fmt"
	"sort"
	"sync"

	"go.uber.org/zap"
)

// Format string constants
const (
	stringText     = "text"
	stringMarkdown = "markdown"
	stringHTML     = "html"
	stringLaTeX    = "latex"
	stringEPUB     = "epub"
	stringUnknown  = "unknown"
)

// Registry 格式化器注册表
type Registry struct {
	formatters       map[string][]Formatter
	formattersByName map[string]Formatter
	mu               sync.RWMutex
	logger           *zap.Logger
}

// globalRegistry 全局注册表实例
var globalRegistry = &Registry{
	formatters:       make(map[string][]Formatter),
	formattersByName: make(map[string]Formatter),
}

// NewFormatterRegistry 创建新的格式化器注册表
func NewFormatterRegistry() *Registry {
	return &Registry{
		formatters:       make(map[string][]Formatter),
		formattersByName: make(map[string]Formatter),
	}
}

// Register 注册格式化器
func Register(format string, formatter Formatter) {
	globalRegistry.RegisterFormat(format, formatter)
}

// Get 获取格式化器
func Get(format string) []Formatter {
	return globalRegistry.Get(format)
}

// GetBest 获取最佳格式化器
func GetBest(format string) Formatter {
	return globalRegistry.GetBest(format)
}

// NewRegistry 创建新的注册表
func NewRegistry(logger *zap.Logger) *Registry {
	return &Registry{
		formatters: make(map[string][]Formatter),
		logger:     logger,
	}
}

// RegisterFormat 注册格式化器到特定格式
func (r *Registry) RegisterFormat(format string, formatter Formatter) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := string(format)
	r.formatters[key] = append(r.formatters[key], formatter)

	// 按优先级排序
	sort.Slice(r.formatters[key], func(i, j int) bool {
		return r.formatters[key][i].Priority() > r.formatters[key][j].Priority()
	})

	if r.logger != nil {
		r.logger.Info("registered formatter",
			zap.String("format", key),
			zap.String("formatter", formatter.Name()),
			zap.Int("priority", formatter.Priority()))
	}
}

// Unregister 注销格式化器
func (r *Registry) Unregister(format string, formatterName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := string(format)
	formatters := r.formatters[key]

	// 过滤掉指定的格式化器
	var filtered []Formatter
	for _, f := range formatters {
		if f.Name() != formatterName {
			filtered = append(filtered, f)
		}
	}

	r.formatters[key] = filtered

	if r.logger != nil {
		r.logger.Info("unregistered formatter",
			zap.String("format", key),
			zap.String("formatter", formatterName))
	}
}

// Get 获取指定格式的所有格式化器
func (r *Registry) Get(format string) []Formatter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := string(format)
	formatters := r.formatters[key]

	// 返回副本
	result := make([]Formatter, len(formatters))
	copy(result, formatters)

	return result
}

// GetBest 获取最佳格式化器（优先级最高的）
func (r *Registry) GetBest(format string) Formatter {
	formatters := r.Get(format)
	if len(formatters) > 0 {
		return formatters[0]
	}
	return nil
}

// GetByFormatAndName 根据格式和名称获取格式化器
func (r *Registry) GetByFormatAndName(format string, name string) Formatter {
	formatters := r.Get(format)
	for _, f := range formatters {
		if f.Name() == name {
			return f
		}
	}
	return nil
}

// ListFormats 列出所有注册的格式化器
func (r *Registry) ListFormats() map[string][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]string)
	for format, formatters := range r.formatters {
		var names []string
		for _, f := range formatters {
			names = append(names, fmt.Sprintf("%s (priority: %d)", f.Name(), f.Priority()))
		}
		result[format] = names
	}

	return result
}

// Clear 清空注册表
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.formatters = make(map[string][]Formatter)
	r.formattersByName = make(map[string]Formatter)
}

// Register 注册格式化器（新接口）
func (r *Registry) Register(formatter Formatter) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := formatter.Name()
	if _, exists := r.formattersByName[name]; exists {
		return fmt.Errorf("formatter %s already registered", name)
	}

	r.formattersByName[name] = formatter

	// 注册到支持的格式
	meta := formatter.GetMetadata()
	for _, format := range meta.Formats {
		r.formatters[format] = append(r.formatters[format], formatter)
		// 按优先级排序
		sort.Slice(r.formatters[format], func(i, j int) bool {
			return r.formatters[format][i].Priority() > r.formatters[format][j].Priority()
		})
	}

	return nil
}

// GetByName 通过名称获取格式化器
func (r *Registry) GetByName(name string) (Formatter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	formatter, exists := r.formattersByName[name]
	if !exists {
		return nil, fmt.Errorf("formatter %s not found", name)
	}
	return formatter, nil
}

// GetByFormat 通过格式获取格式化器
func (r *Registry) GetByFormat(format string) (Formatter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	formatters, exists := r.formatters[format]
	if !exists || len(formatters) == 0 {
		return nil, fmt.Errorf("no formatter found for format %s", format)
	}
	return formatters[0], nil // 返回优先级最高的
}

// List 列出所有格式化器的元数据
func (r *Registry) List() []FormatterMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []FormatterMetadata
	for _, formatter := range r.formattersByName {
		result = append(result, formatter.GetMetadata())
	}
	return result
}

// AutoRegisterFormatters 自动注册内置格式化器
func AutoRegisterFormatters(logger *zap.Logger) {
	// 注册文本格式化器
	globalRegistry.Register(NewTextFormatter())

	// 注册 Markdown 格式化器
	globalRegistry.Register(NewMarkdownFormatter())

	// 注册 HTML 格式化器
	if IsExternalToolAvailable("prettier") {
		Register(stringHTML, NewExternalFormatter("prettier", []string{"--parser", "html"}))
	}

	// 检测并注册其他外部工具
	registerExternalFormatters(logger)
}

// registerExternalFormatters 注册外部格式化器
func registerExternalFormatters(logger *zap.Logger) {
	// Prettier 支持多种格式
	if IsExternalToolAvailable("prettier") {
		prettierFormats := map[string][]string{
			stringMarkdown: {"--parser", "markdown"},
			stringHTML:     {"--parser", "html"},
		}

		for format, args := range prettierFormats {
			formatter := NewExternalFormatter("prettier", args)
			formatter.priority = 100 // 外部工具优先级更高
			Register(format, formatter)
		}
	}

	// LaTeX 格式化器
	if IsExternalToolAvailable("latexindent") {
		formatter := NewExternalFormatter("latexindent", []string{"-w"})
		formatter.priority = 100
		Register(stringLaTeX, formatter)
	}

	if logger != nil {
		logger.Info("external formatters registered",
			zap.Bool("prettier", IsExternalToolAvailable("prettier")),
			zap.Bool("latexindent", IsExternalToolAvailable("latexindent")))
	}
}
