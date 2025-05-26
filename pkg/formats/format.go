package formats

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

var log *zap.Logger

func init() {
	var err error
	log, err = zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
}

// Processor 定义文件格式处理器的接口
type Processor interface {
	// TranslateFile 翻译文件内容并写入输出文件
	TranslateFile(inputPath, outputPath string) error

	//// TranslateText 翻译文本内容并保留格式
	//TranslateText(text string) (string, error)

	// GetName 返回处理器的名称
	GetName() string
}

// ReplacementInfo 存储占位符和原始内容的映射关系
type ReplacementInfo struct {
	Placeholder string `json:"placeholder"` // 占位符
	Original    string `json:"original"`    // 原始内容
}

type ReplacementInfoList struct {
	Replacements []ReplacementInfo `json:"replacements"`
}

var (
	// 匹配整个占位符（含开始、原始内容、结束），允许中间任何字符（?s 表示 . 能匹配换行）
	mdRePlaceholder         = regexp.MustCompile(`@@PRESERVE_\d+@@`)
	mdRestrictedPlaceholder = regexp.MustCompile(`^@@PRESERVE_\d+@@$`)
	mdRePlaceholderWildcard = regexp.MustCompile(`@@([^@]*?)_(\d+)@@`)
)

// processorRegistry 存储所有注册的格式处理器
var processorRegistry = make(map[string]func(translator.Translator, *config.PredefinedTranslation, *progress.Writer) (Processor, error))

// RegisterProcessor 注册一个格式处理器
func RegisterProcessor(name string, factory func(translator.Translator, *config.PredefinedTranslation, *progress.Writer) (Processor, error)) {
	processorRegistry[name] = factory
}

// NewProcessor 创建指定格式的处理器
func NewProcessor(t translator.Translator, format string, predefinedTranslations *config.PredefinedTranslation, progressBar *progress.Writer) (Processor, error) {
	factory, ok := processorRegistry[format]
	if !ok {
		return nil, fmt.Errorf("不支持的格式: %s", format)
	}

	return factory(t, predefinedTranslations, progressBar)
}

// ProcessorFromFilePath 根据文件扩展名选择合适的处理器
func ProcessorFromFilePath(t translator.Translator, filePath string, predefinedTranslations *config.PredefinedTranslation, progressBar *progress.Writer) (Processor, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		return nil, fmt.Errorf("无法从文件路径确定格式: %s", filePath)
	}

	// 移除开头的点
	ext = ext[1:]

	// 处理特殊格式
	switch ext {
	case "md", "markdown":
		return NewProcessor(t, "markdown", predefinedTranslations, progressBar)
	case "txt", "text":
		return NewProcessor(t, "text", predefinedTranslations, progressBar)
	case "epub":
		return NewProcessor(t, "epub", predefinedTranslations, progressBar)
	case "tex":
		return NewProcessor(t, "latex", predefinedTranslations, progressBar)
	case "html", "htm", "xhtml", "xml":
		return NewProcessor(t, "html", predefinedTranslations, progressBar)
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

// Snippet 返回文本的摘要，用于日志记录
func Snippet(s string) string {
	const seg = 20
	if len(s) <= seg*3 {
		return s
	}
	start := s[:seg]
	midIdx := len(s) / 2
	midStart := midIdx - seg/2
	if midStart < seg {
		midStart = seg
	}
	mid := s[midStart : midStart+seg]
	end := s[len(s)-seg:]
	return start + " ... " + mid + " ... " + end
}

// BaseProcessor 提供所有处理器共享的基本功能
type BaseProcessor struct {
	Translator             translator.Translator
	Name                   string
	predefinedTranslations *config.PredefinedTranslation
	progressBar            *progress.Writer
	logger                 *zap.Logger
}

// GetName 返回处理器的名称
func (p *BaseProcessor) GetName() string {
	return p.Name
}

// 初始化所有内置处理器
func init() {
	// 注册文本处理器
	RegisterProcessor("text", func(t translator.Translator, predefinedTranslations *config.PredefinedTranslation, progressBar *progress.Writer) (Processor, error) {
		return NewTextProcessor(t, predefinedTranslations, progressBar)
	})

	// 注册Markdown处理器
	RegisterProcessor("markdown", func(t translator.Translator, predefinedTranslations *config.PredefinedTranslation, progressBar *progress.Writer) (Processor, error) {
		return NewMarkdownProcessor(t, predefinedTranslations, progressBar)
	})

	// 注册EPUB处理器
	RegisterProcessor("epub", func(t translator.Translator, predefinedTranslations *config.PredefinedTranslation, progressBar *progress.Writer) (Processor, error) {
		return NewEPUBProcessor(t, predefinedTranslations, progressBar)
	})

	// 注册LaTeX处理器
	RegisterProcessor("latex", func(t translator.Translator, predefinedTranslations *config.PredefinedTranslation, progressBar *progress.Writer) (Processor, error) {
		return NewLaTeXProcessor(t, predefinedTranslations, progressBar)
	})

	// 注册HTML处理器
	RegisterProcessor("html", func(t translator.Translator, predefinedTranslations *config.PredefinedTranslation, progressBar *progress.Writer) (Processor, error) {
		return NewHTMLProcessor(t, predefinedTranslations, progressBar)
	})
}
