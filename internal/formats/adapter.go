package formats

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	// 导入具体实现以触发注册
	_ "github.com/nerdneilsfield/go-translator-agent/internal/formats/markdown"
	_ "github.com/nerdneilsfield/go-translator-agent/internal/formats/text"
	"github.com/nerdneilsfield/go-translator-agent/pkg/formats"
	oldFormats "github.com/nerdneilsfield/go-translator-agent/pkg/formats"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
)

// ProcessorAdapter 将新的 formats.Processor 接口适配到旧的系统
type ProcessorAdapter struct {
	processor   formats.Processor
	translator  translator.Translator
	progressBar *progress.Writer
	format      formats.Format
}

// NewProcessorAdapter 创建处理器适配器
func NewProcessorAdapter(t translator.Translator, format string, predefinedTranslations *config.PredefinedTranslation, progressBar *progress.Writer) (oldFormats.Processor, error) {
	// 将字符串格式转换为 Format 类型
	var formatType formats.Format
	switch strings.ToLower(format) {
	case "markdown", "md":
		formatType = formats.FormatMarkdown
	case "text", "txt":
		formatType = formats.FormatText
	case "html", "htm":
		formatType = formats.FormatHTML
	case "epub":
		formatType = formats.FormatEPUB
	case "latex", "tex":
		formatType = formats.FormatLaTeX
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}

	// 创建处理器选项
	opts := formats.ProcessorOptions{
		ChunkSize:    2000, // 默认分块大小
		ChunkOverlap: 100,  // 默认重叠大小
	}

	// 从配置中获取设置
	if configProvider, ok := t.(interface{ GetConfig() *config.Config }); ok {
		cfg := configProvider.GetConfig()
		if cfg != nil {
			if cfg.MaxTokensPerChunk > 0 {
				opts.ChunkSize = cfg.MaxTokensPerChunk
			}
		}
	}

	// 获取新的处理器
	processor, err := formats.GetProcessor(formatType, opts)
	if err != nil {
		return nil, err
	}

	return &ProcessorAdapter{
		processor:   processor,
		translator:  t,
		progressBar: progressBar,
		format:      formatType,
	}, nil
}

// TranslateFile 翻译文件（实现旧接口）
func (a *ProcessorAdapter) TranslateFile(inputPath, outputPath string) error {
	// 读取输入文件
	input, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer input.Close()

	// 解析文档
	ctx := context.Background()
	doc, err := a.processor.Parse(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to parse document: %w", err)
	}

	// 创建翻译函数
	translateFunc := func(ctx context.Context, text string) (string, error) {
		// 调用旧的翻译器接口
		result, err := a.translator.Translate(text, false)
		if err != nil {
			return "", err
		}
		return result, nil
	}

	// 处理文档（翻译）
	translatedDoc, err := a.processor.Process(ctx, doc, translateFunc)
	if err != nil {
		return fmt.Errorf("failed to process document: %w", err)
	}

	// 创建输出文件
	output, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer output.Close()

	// 渲染文档
	err = a.processor.Render(ctx, translatedDoc, output)
	if err != nil {
		return fmt.Errorf("failed to render document: %w", err)
	}

	return nil
}

// GetName 返回处理器名称（实现旧接口）
func (a *ProcessorAdapter) GetName() string {
	return string(a.format)
}

// TranslateText 翻译文本（用于兼容）
func (a *ProcessorAdapter) TranslateText(text string) (string, error) {
	ctx := context.Background()

	// 创建内存中的文档
	reader := strings.NewReader(text)
	doc, err := a.processor.Parse(ctx, reader)
	if err != nil {
		return "", err
	}

	// 翻译函数
	translateFunc := func(ctx context.Context, text string) (string, error) {
		result, err := a.translator.Translate(text, false)
		if err != nil {
			return "", err
		}
		return result, nil
	}

	// 处理文档
	translatedDoc, err := a.processor.Process(ctx, doc, translateFunc)
	if err != nil {
		return "", err
	}

	// 渲染到内存
	var output bytes.Buffer
	err = a.processor.Render(ctx, translatedDoc, &output)
	if err != nil {
		return "", err
	}

	return output.String(), nil
}
