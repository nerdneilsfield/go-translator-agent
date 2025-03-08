package formats

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
)

// Processor 定义文件格式处理器的接口
type Processor interface {
	// TranslateFile 翻译文件内容并写入输出文件
	TranslateFile(inputPath, outputPath string) error

	// TranslateText 翻译文本内容并保留格式
	TranslateText(text string) (string, error)

	// GetName 返回处理器的名称
	GetName() string
}

// processorRegistry 存储所有注册的格式处理器
var processorRegistry = make(map[string]func(translator.Translator) (Processor, error))

// RegisterProcessor 注册一个格式处理器
func RegisterProcessor(name string, factory func(translator.Translator) (Processor, error)) {
	processorRegistry[name] = factory
}

// NewProcessor 创建指定格式的处理器
func NewProcessor(t translator.Translator, format string) (Processor, error) {
	factory, ok := processorRegistry[format]
	if !ok {
		return nil, fmt.Errorf("不支持的格式: %s", format)
	}

	return factory(t)
}

// ProcessorFromFilePath 根据文件扩展名选择合适的处理器
func ProcessorFromFilePath(t translator.Translator, filePath string) (Processor, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		return nil, fmt.Errorf("无法从文件路径确定格式: %s", filePath)
	}

	// 移除开头的点
	ext = ext[1:]

	// 处理特殊格式
	switch ext {
	case "md", "markdown":
		return NewProcessor(t, "markdown")
	case "txt":
		return NewProcessor(t, "text")
	case "epub":
		return NewProcessor(t, "epub")
	case "tex":
		return NewProcessor(t, "latex")
	default:
		return nil, fmt.Errorf("不支持的文件扩展名: %s", ext)
	}
}

// RegisteredFormats 返回支持的文件格式列表
func RegisteredFormats() []string {
	formats := make([]string, 0, len(processorRegistry))
	for format := range processorRegistry {
		formats = append(formats, format)
	}
	return formats
}

// BaseProcessor 提供所有处理器共享的基本功能
type BaseProcessor struct {
	Translator translator.Translator
	Name       string
}

// GetName 返回处理器的名称
func (p *BaseProcessor) GetName() string {
	return p.Name
}

// 初始化所有内置处理器
func init() {
	// 注册文本处理器
	RegisterProcessor("text", func(t translator.Translator) (Processor, error) {
		return NewTextProcessor(t)
	})

	// 注册Markdown处理器
	RegisterProcessor("markdown", func(t translator.Translator) (Processor, error) {
		return NewMarkdownProcessor(t)
	})

	// 注册EPUB处理器
	RegisterProcessor("epub", func(t translator.Translator) (Processor, error) {
		return NewEPUBProcessor(t)
	})

	// 注册LaTeX处理器
	RegisterProcessor("latex", func(t translator.Translator) (Processor, error) {
		return NewLaTeXProcessor(t)
	})
}
