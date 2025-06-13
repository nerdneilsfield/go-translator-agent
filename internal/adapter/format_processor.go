package adapter

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/document"
	"github.com/nerdneilsfield/go-translator-agent/pkg/formats"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"

	// 导入格式处理器以触发注册
	_ "github.com/nerdneilsfield/go-translator-agent/internal/formats/epub"
	_ "github.com/nerdneilsfield/go-translator-agent/internal/formats/html"
	_ "github.com/nerdneilsfield/go-translator-agent/internal/formats/markdown"
	_ "github.com/nerdneilsfield/go-translator-agent/internal/formats/text"
)

// FormatProcessorAdapter 适配新的文档处理器到旧的格式处理器接口
type FormatProcessorAdapter struct {
	processor              document.Processor
	translator             translator.Translator
	format                 document.Format
	predefinedTranslations *config.PredefinedTranslation
	progressBar            *progress.Writer
	logger                 *zap.Logger
}

// NewFormatProcessorAdapter 创建格式处理器适配器
func NewFormatProcessorAdapter(
	format document.Format,
	translator translator.Translator,
	predefinedTranslations *config.PredefinedTranslation,
	progressBar *progress.Writer,
) (formats.Processor, error) {
	// 获取 logger
	zapLogger, _ := zap.NewProduction()
	if loggerProvider, ok := translator.GetLogger().(interface{ GetZapLogger() *zap.Logger }); ok {
		if zl := loggerProvider.GetZapLogger(); zl != nil {
			zapLogger = zl
		}
	}

	// 创建处理器选项
	opts := document.ProcessorOptions{
		ChunkSize:    1000,
		ChunkOverlap: 50,
		Metadata:     make(map[string]interface{}),
	}

	// 从翻译器配置中获取选项
	// Config 结构中没有 ChunkSize 和 ChunkOverlap，使用默认值

	// 获取对应格式的处理器
	processor, err := document.GetProcessor(format, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get processor for format %s: %w", format, err)
	}

	return &FormatProcessorAdapter{
		processor:              processor,
		translator:             translator,
		format:                 format,
		predefinedTranslations: predefinedTranslations,
		progressBar:            progressBar,
		logger:                 zapLogger,
	}, nil
}

// TranslateFile 翻译文件
func (a *FormatProcessorAdapter) TranslateFile(inputPath, outputPath string) error {
	ctx := context.Background()

	// 打开输入文件
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer inputFile.Close()

	// 解析文档
	a.logger.Info("Parsing document", zap.String("file", inputPath))
	doc, err := a.processor.Parse(ctx, inputFile)
	if err != nil {
		return fmt.Errorf("failed to parse document: %w", err)
	}

	// 统计总字符数
	totalChars := 0
	for _, block := range doc.Blocks {
		if block.IsTranslatable() {
			totalChars += len(block.GetContent())
		}
	}
	a.logger.Info("Document parsed", 
		zap.Int("blocks", len(doc.Blocks)),
		zap.Int("totalChars", totalChars),
	)

	// 创建翻译函数
	translateFunc := a.createTranslateFunc()

	// 设置进度跟踪
	if a.progressBar != nil {
		tracker := &progress.Tracker{
			Message: fmt.Sprintf("Translating %s", inputPath),
			Total:   int64(totalChars),
			Units:   progress.UnitsBytes,
		}
		// progressBar 是指向接口的指针，需要解引用
		(*a.progressBar).AppendTracker(tracker)
		
		// 创建包装的翻译函数来更新进度
		originalTranslateFunc := translateFunc
		processedChars := int64(0)
		var mu sync.Mutex
		
		translateFunc = func(ctx context.Context, text string) (string, error) {
			result, err := originalTranslateFunc(ctx, text)
			
			mu.Lock()
			processedChars += int64(len(text))
			tracker.SetValue(processedChars)
			mu.Unlock()
			
			return result, err
		}
	}

	// 处理文档
	a.logger.Info("Processing document")
	start := time.Now()
	processedDoc, err := a.processor.Process(ctx, doc, translateFunc)
	if err != nil {
		return fmt.Errorf("failed to process document: %w", err)
	}
	a.logger.Info("Document processed", zap.Duration("duration", time.Since(start)))

	// 创建输出文件
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	// 渲染文档
	a.logger.Info("Rendering document", zap.String("file", outputPath))
	if err := a.processor.Render(ctx, processedDoc, outputFile); err != nil {
		return fmt.Errorf("failed to render document: %w", err)
	}

	a.logger.Info("Translation completed",
		zap.String("input", inputPath),
		zap.String("output", outputPath),
		zap.Duration("totalDuration", time.Since(start)),
	)

	return nil
}

// TranslateString 翻译字符串
func (a *FormatProcessorAdapter) TranslateString(content string) (string, error) {
	ctx := context.Background()

	// 创建内存中的文档
	doc := &document.Document{
		ID:     "string-doc",
		Format: document.FormatText,
		Blocks: []document.Block{
			&document.BaseBlock{
				Type:         document.BlockTypeParagraph,
				Content:      content,
				Translatable: true,
			},
		},
	}

	// 创建翻译函数
	translateFunc := a.createTranslateFunc()

	// 处理文档
	processedDoc, err := a.processor.Process(ctx, doc, translateFunc)
	if err != nil {
		return "", fmt.Errorf("failed to process string: %w", err)
	}

	// 提取翻译后的内容
	if len(processedDoc.Blocks) > 0 {
		return processedDoc.Blocks[0].GetContent(), nil
	}

	return "", fmt.Errorf("no translated content found")
}

// TranslateReader 翻译 Reader 内容
func (a *FormatProcessorAdapter) TranslateReader(reader io.Reader, writer io.Writer) error {
	ctx := context.Background()

	// 解析文档
	doc, err := a.processor.Parse(ctx, reader)
	if err != nil {
		return fmt.Errorf("failed to parse from reader: %w", err)
	}

	// 创建翻译函数
	translateFunc := a.createTranslateFunc()

	// 处理文档
	processedDoc, err := a.processor.Process(ctx, doc, translateFunc)
	if err != nil {
		return fmt.Errorf("failed to process document: %w", err)
	}

	// 渲染文档
	if err := a.processor.Render(ctx, processedDoc, writer); err != nil {
		return fmt.Errorf("failed to render to writer: %w", err)
	}

	return nil
}

// GetName 获取处理器名称
func (a *FormatProcessorAdapter) GetName() string {
	return fmt.Sprintf("%s (Document Processor)", a.format)
}

// createTranslateFunc 创建翻译函数
func (a *FormatProcessorAdapter) createTranslateFunc() document.TranslateFunc {
	return func(ctx context.Context, text string) (string, error) {
		// 检查预定义翻译
		if a.predefinedTranslations != nil {
			for original, translated := range a.predefinedTranslations.Translations {
				if text == original {
					a.logger.Debug("Using predefined translation",
						zap.String("original", original),
						zap.String("translated", translated),
					)
					return translated, nil
				}
			}
		}

		// 使用翻译器翻译
		result, err := a.translator.Translate(text, false)
		if err != nil {
			return "", fmt.Errorf("translation failed: %w", err)
		}

		return result, nil
	}
}

// CreateFormatProcessor 创建指定格式的处理器
func CreateFormatProcessor(
	format string,
	translator translator.Translator,
	predefinedTranslations *config.PredefinedTranslation,
	progressBar *progress.Writer,
) (formats.Processor, error) {
	// 映射格式字符串到文档格式
	var docFormat document.Format
	switch format {
	case "markdown", "md":
		docFormat = document.FormatMarkdown
	case "text", "txt":
		docFormat = document.FormatText
	case "html":
		docFormat = document.FormatHTML
	case "epub":
		docFormat = document.FormatEPUB
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}

	return NewFormatProcessorAdapter(docFormat, translator, predefinedTranslations, progressBar)
}