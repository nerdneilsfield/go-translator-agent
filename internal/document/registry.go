package document

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// Registry 格式处理器注册表
type Registry struct {
	mu         sync.RWMutex
	processors map[Format]ProcessorFactory
	extensions map[string]Format
}

// globalRegistry 全局注册表实例
var globalRegistry = &Registry{
	processors: make(map[Format]ProcessorFactory),
	extensions: make(map[string]Format),
}

// Register 注册处理器
func Register(format Format, factory ProcessorFactory) error {
	return globalRegistry.Register(format, factory)
}

// RegisterExtension 注册文件扩展名
func RegisterExtension(ext string, format Format) {
	globalRegistry.RegisterExtension(ext, format)
}

// GetProcessor 获取处理器
func GetProcessor(format Format, opts ProcessorOptions) (Processor, error) {
	return globalRegistry.GetProcessor(format, opts)
}

// GetProcessorByExtension 根据文件扩展名获取处理器
func GetProcessorByExtension(filename string, opts ProcessorOptions) (Processor, error) {
	return globalRegistry.GetProcessorByExtension(filename, opts)
}

// GetRegisteredFormats 获取所有已注册的格式
func GetRegisteredFormats() []Format {
	return globalRegistry.GetRegisteredFormats()
}

// Register 注册处理器到注册表
func (r *Registry) Register(format Format, factory ProcessorFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.processors[format]; exists {
		return fmt.Errorf("format %s already registered", format)
	}

	r.processors[format] = factory
	return nil
}

// RegisterExtension 注册文件扩展名映射
func (r *Registry) RegisterExtension(ext string, format Format) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 标准化扩展名（去除点号，转小写）
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	r.extensions[ext] = format
}

// GetProcessor 获取指定格式的处理器
func (r *Registry) GetProcessor(format Format, opts ProcessorOptions) (Processor, error) {
	r.mu.RLock()
	factory, exists := r.processors[format]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no processor registered for format: %s", format)
	}

	return factory(opts)
}

// GetProcessorByExtension 根据文件扩展名获取处理器
func (r *Registry) GetProcessorByExtension(filename string, opts ProcessorOptions) (Processor, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))

	r.mu.RLock()
	format, exists := r.extensions[ext]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no processor registered for extension: %s", ext)
	}

	return r.GetProcessor(format, opts)
}

// GetRegisteredFormats 获取所有已注册的格式
func (r *Registry) GetRegisteredFormats() []Format {
	r.mu.RLock()
	defer r.mu.RUnlock()

	formats := make([]Format, 0, len(r.processors))
	for format := range r.processors {
		formats = append(formats, format)
	}
	return formats
}

// GetFormatByExtension 根据扩展名获取格式
func (r *Registry) GetFormatByExtension(ext string) (Format, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	format, exists := r.extensions[ext]
	return format, exists
}

// getLoggerFromOptions 从ProcessorOptions中获取logger，如果没有则返回默认logger
func getLoggerFromOptions(opts ProcessorOptions) *zap.Logger {
	if opts.Metadata != nil {
		if logger, ok := opts.Metadata["logger"].(*zap.Logger); ok && logger != nil {
			return logger
		}
	}
	// 返回一个默认的nop logger
	return zap.NewNop()
}

// getHTMLProcessingModeFromOptions 从ProcessorOptions中获取HTML处理模式
func getHTMLProcessingModeFromOptions(opts ProcessorOptions) HTMLProcessingMode {
	if opts.Metadata != nil {
		if mode, ok := opts.Metadata["html_processing_mode"].(string); ok {
			switch mode {
			case "native":
				return HTMLModeNative
			case "markdown":
				return HTMLModeMarkdown
			default:
				return HTMLModeMarkdown // 默认使用markdown模式
			}
		}
	}
	return HTMLModeMarkdown // 默认使用markdown模式
}

// init 初始化默认扩展名映射和处理器注册
func init() {
	// 注册处理器工厂
	Register(FormatMarkdown, func(opts ProcessorOptions) (Processor, error) {
		logger := getLoggerFromOptions(opts)
		return NewMarkdownProcessor(opts, logger)
	})

	Register(FormatText, func(opts ProcessorOptions) (Processor, error) {
		logger := getLoggerFromOptions(opts)
		return NewTextProcessor(opts, logger)
	})

	Register(FormatHTML, func(opts ProcessorOptions) (Processor, error) {
		logger := getLoggerFromOptions(opts)
		mode := getHTMLProcessingModeFromOptions(opts)
		return NewHTMLProcessor(opts, logger, mode)
	})

	Register(FormatEPUB, func(opts ProcessorOptions) (Processor, error) {
		logger := getLoggerFromOptions(opts)
		mode := getHTMLProcessingModeFromOptions(opts)
		return NewEPUBProcessor(opts, logger, mode)
	})

	Register(FormatTextBundle, func(opts ProcessorOptions) (Processor, error) {
		logger := getLoggerFromOptions(opts)
		return NewTextBundleProcessor(opts, logger)
	})

	Register(FormatTextPack, func(opts ProcessorOptions) (Processor, error) {
		logger := getLoggerFromOptions(opts)
		return NewTextPackProcessor(opts, logger)
	})

	Register(FormatDOCX, func(opts ProcessorOptions) (Processor, error) {
		logger := getLoggerFromOptions(opts)
		return NewDocxProcessor(opts, logger)
	})

	// Markdown
	RegisterExtension(".md", FormatMarkdown)
	RegisterExtension(".markdown", FormatMarkdown)
	RegisterExtension(".mdown", FormatMarkdown)
	RegisterExtension(".mkd", FormatMarkdown)

	// Text
	RegisterExtension(".txt", FormatText)
	RegisterExtension(".text", FormatText)

	// HTML
	RegisterExtension(".html", FormatHTML)
	RegisterExtension(".htm", FormatHTML)
	RegisterExtension(".xhtml", FormatHTML)
	RegisterExtension(".xml", FormatHTML)

	// EPUB
	RegisterExtension(".epub", FormatEPUB)

	// LaTeX
	RegisterExtension(".tex", FormatLaTeX)
	RegisterExtension(".latex", FormatLaTeX)

	// PDF
	RegisterExtension(".pdf", FormatPDF)

	// DOCX
	RegisterExtension(".docx", FormatDOCX)
	RegisterExtension(".doc", FormatDOCX)

	// TextBundle
	RegisterExtension(".textbundle", FormatTextBundle)

	// TextPack
	RegisterExtension(".textpack", FormatTextPack)
}
