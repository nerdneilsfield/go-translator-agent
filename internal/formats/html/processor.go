package html

import (
	"fmt"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
)

// ProcessingMode HTML 处理模式
type ProcessingMode string

const (
	// ModeMarkdown 通过 Markdown 转换模式
	ModeMarkdown ProcessingMode = "markdown"
	// ModeNative 原生 HTML 处理模式
	ModeNative ProcessingMode = "native"
	// ModeAuto 自动选择模式
	ModeAuto ProcessingMode = "auto"
)

// Factory 创建 HTML 处理器的工厂函数
func Factory(opts document.ProcessorOptions) (document.Processor, error) {
	// 从选项中获取处理模式
	mode := ModeAuto
	if opts.Metadata != nil {
		if m, ok := opts.Metadata["html_mode"].(string); ok {
			mode = ProcessingMode(m)
		}
	}

	// 根据模式创建处理器
	switch mode {
	case ModeMarkdown:
		return NewMarkdownProcessor(opts)
	case ModeNative:
		return NewNativeProcessor(opts)
	case ModeAuto:
		// 自动模式：根据内容复杂度选择
		// 默认使用 Markdown 模式，因为它对大多数情况更友好
		return NewMarkdownProcessor(opts)
	default:
		return nil, fmt.Errorf("unknown HTML processing mode: %s", mode)
	}
}

// MarkdownFactory 专门创建 Markdown 模式的处理器
func MarkdownFactory(opts document.ProcessorOptions) (document.Processor, error) {
	return NewMarkdownProcessor(opts)
}

// init 注册 HTML 处理器
func init() {
	// 注册默认工厂
	document.Register(document.FormatHTML, Factory)

	// 也可以注册特定模式的工厂（如果需要）
	// 例如：document.Register("html-markdown", MarkdownFactory)
	// 例如：document.Register("html-native", NativeFactory)
}

// ProcessorWithMode 创建指定模式的 HTML 处理器
func ProcessorWithMode(mode ProcessingMode, opts document.ProcessorOptions) (document.Processor, error) {
	// 设置模式到选项中
	if opts.Metadata == nil {
		opts.Metadata = make(map[string]interface{})
	}
	opts.Metadata["html_mode"] = string(mode)

	return Factory(opts)
}
