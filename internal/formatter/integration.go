package formatter

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"go.uber.org/zap"
)

// Block 文档块接口（简化版，用于格式化）
type Block interface {
	GetContent() string
	SetContent(content string)
	GetMetadata() map[string]interface{}
	IsTranslatable() bool
	GetType() string
}

// DocumentFormatter 文档格式化器
type DocumentFormatter struct {
	manager *Manager
}

// NewDocumentFormatter 创建文档格式化器
func NewDocumentFormatter(manager *Manager) *DocumentFormatter {
	return &DocumentFormatter{
		manager: manager,
	}
}

// FormatBlock 格式化文档块
func (f *DocumentFormatter) FormatBlock(ctx context.Context, block Block, opts *FormatOptions) error {
	// 获取格式信息
	format := ""
	if metadata := block.GetMetadata(); metadata != nil {
		if fmt, ok := metadata["format"].(string); ok {
			format = fmt
		}
	}

	// 格式化内容
	content := block.GetContent()
	formatted, err := f.manager.Format(ctx, content, format, opts)
	if err != nil {
		return err
	}

	// 更新内容
	block.SetContent(formatted)
	return nil
}

// ProcessorWithFormatter 带格式化功能的文档处理器包装器
type ProcessorWithFormatter struct {
	processor document.Processor
	manager   *Manager
	options   FormatIntegrationOptions
	logger    *zap.Logger
}

// FormatIntegrationOptions 格式化集成选项
type FormatIntegrationOptions struct {
	// 格式化时机
	FormatBeforeParse  bool // 解析前格式化
	FormatAfterProcess bool // 处理后格式化
	FormatBeforeRender bool // 渲染前格式化
	FormatAfterRender  bool // 渲染后格式化

	// 格式化选项
	FormatOptions FormatOptions

	// 错误处理
	ContinueOnFormatError bool // 格式化失败时是否继续

	// 性能选项
	SkipIfNoChanges bool // 如果内容未改变则跳过格式化
}

// NewProcessorWithFormatter 创建带格式化功能的处理器
func NewProcessorWithFormatter(processor document.Processor, manager *Manager, options FormatIntegrationOptions, logger *zap.Logger) *ProcessorWithFormatter {
	return &ProcessorWithFormatter{
		processor: processor,
		manager:   manager,
		options:   options,
		logger:    logger,
	}
}

// Parse 解析输入（带格式化）
func (p *ProcessorWithFormatter) Parse(ctx context.Context, input io.Reader) (*document.Document, error) {
	var reader io.Reader = input

	// 解析前格式化
	if p.options.FormatBeforeParse {
		formatted, err := p.formatInput(input)
		if err != nil {
			if !p.options.ContinueOnFormatError {
				return nil, fmt.Errorf("pre-parse formatting failed: %w", err)
			}
			p.logger.Warn("pre-parse formatting failed, continuing", zap.Error(err))
		} else {
			reader = formatted
		}
	}

	// 调用原始解析器
	return p.processor.Parse(ctx, reader)
}

// Process 处理文档（带格式化）
func (p *ProcessorWithFormatter) Process(ctx context.Context, doc *document.Document, translator document.TranslateFunc) (*document.Document, error) {
	// 调用原始处理器
	processed, err := p.processor.Process(ctx, doc, translator)
	if err != nil {
		return nil, err
	}

	// 处理后格式化
	if p.options.FormatAfterProcess {
		formatted, err := p.formatDocument(processed)
		if err != nil {
			if !p.options.ContinueOnFormatError {
				return nil, fmt.Errorf("post-process formatting failed: %w", err)
			}
			p.logger.Warn("post-process formatting failed, continuing", zap.Error(err))
		} else {
			processed = formatted
		}
	}

	return processed, nil
}

// Render 渲染文档（带格式化）
func (p *ProcessorWithFormatter) Render(ctx context.Context, doc *document.Document, output io.Writer) error {
	// 渲染前格式化
	if p.options.FormatBeforeRender {
		formatted, err := p.formatDocument(doc)
		if err != nil {
			if !p.options.ContinueOnFormatError {
				return fmt.Errorf("pre-render formatting failed: %w", err)
			}
			p.logger.Warn("pre-render formatting failed, continuing", zap.Error(err))
		} else {
			doc = formatted
		}
	}

	// 如果需要渲染后格式化，先渲染到缓冲区
	if p.options.FormatAfterRender {
		var buf bytes.Buffer
		if err := p.processor.Render(ctx, doc, &buf); err != nil {
			return err
		}

		// 格式化渲染结果
		result, err := p.manager.FormatBytes(buf.Bytes(), string(doc.Format), p.options.FormatOptions)
		if err != nil {
			if !p.options.ContinueOnFormatError {
				return fmt.Errorf("post-render formatting failed: %w", err)
			}
			p.logger.Warn("post-render formatting failed, using unformatted", zap.Error(err))
			_, err = output.Write(buf.Bytes())
			return err
		}

		_, err = output.Write(result.Content)
		return err
	}

	// 直接渲染
	return p.processor.Render(ctx, doc, output)
}

// GetFormat 返回支持的格式
func (p *ProcessorWithFormatter) GetFormat() document.Format {
	return p.processor.GetFormat()
}

// formatInput 格式化输入流
func (p *ProcessorWithFormatter) formatInput(input io.Reader) (io.Reader, error) {
	// 读取内容
	content, err := io.ReadAll(input)
	if err != nil {
		return nil, err
	}

	// 检测格式
	format := p.processor.GetFormat()

	// 格式化
	result, err := p.manager.FormatBytes(content, string(format), p.options.FormatOptions)
	if err != nil {
		return nil, err
	}

	// 返回格式化后的内容
	return bytes.NewReader(result.Content), nil
}

// formatDocument 格式化文档
func (p *ProcessorWithFormatter) formatDocument(doc *document.Document) (*document.Document, error) {
	// 复制文档以避免修改原始文档
	formatted := &document.Document{
		ID:        doc.ID,
		Format:    doc.Format,
		Metadata:  doc.Metadata,
		Blocks:    make([]document.Block, len(doc.Blocks)),
		Resources: doc.Resources,
	}

	// 格式化每个块
	for i, block := range doc.Blocks {
		if block.IsTranslatable() {
			content := []byte(block.GetContent())

			// 根据块类型选择格式
			blockFormat := p.getBlockFormat(block.GetType())
			if blockFormat == document.FormatUnknown {
				blockFormat = doc.Format
			}

			// 格式化
			result, err := p.manager.FormatBytes(content, string(blockFormat), p.options.FormatOptions)
			if err != nil {
				// 格式化失败，保留原始内容
				p.logger.Warn("failed to format block",
					zap.Int("index", i),
					zap.String("type", string(block.GetType())),
					zap.Error(err))
				formatted.Blocks[i] = block
			} else if result.Changed || !p.options.SkipIfNoChanges {
				// 创建新块
				newBlock := &document.BaseBlock{
					Type:         block.GetType(),
					Content:      string(result.Content),
					Translatable: block.IsTranslatable(),
					Metadata:     block.GetMetadata(),
				}
				formatted.Blocks[i] = newBlock
			} else {
				formatted.Blocks[i] = block
			}
		} else {
			formatted.Blocks[i] = block
		}
	}

	return formatted, nil
}

// getBlockFormat 获取块对应的格式
func (p *ProcessorWithFormatter) getBlockFormat(blockType document.BlockType) document.Format {
	switch blockType {
	case document.BlockTypeCode:
		// 代码块可能需要特殊处理
		return document.FormatText
	case document.BlockTypeHTML:
		return document.FormatHTML
	default:
		return document.FormatUnknown
	}
}

// CreateFormatterMiddleware 创建格式化中间件
func CreateFormatterMiddleware(manager *Manager, options FormatIntegrationOptions, logger *zap.Logger) func(document.Processor) document.Processor {
	return func(processor document.Processor) document.Processor {
		return NewProcessorWithFormatter(processor, manager, options, logger)
	}
}

// DefaultFormatIntegrationOptions 默认集成选项
func DefaultFormatIntegrationOptions() FormatIntegrationOptions {
	return FormatIntegrationOptions{
		FormatBeforeParse:     true,  // 解析前格式化，确保输入规范
		FormatAfterProcess:    false, // 处理后不格式化，保持翻译结果
		FormatBeforeRender:    false, // 渲染前不格式化
		FormatAfterRender:     true,  // 渲染后格式化，确保输出美观
		FormatOptions:         DefaultFormatOptions(),
		ContinueOnFormatError: true, // 格式化失败继续处理
		SkipIfNoChanges:       true, // 跳过无变化的格式化
	}
}

// FormatCommand 格式化命令（用于 CLI）
type FormatCommand struct {
	manager *Manager
	logger  *zap.Logger
}

// NewFormatCommand 创建格式化命令
func NewFormatCommand(logger *zap.Logger) *FormatCommand {
	// 自动注册格式化器
	AutoRegisterFormatters(logger)

	return &FormatCommand{
		manager: NewManager(logger),
		logger:  logger,
	}
}

// FormatFile 格式化文件
func (c *FormatCommand) FormatFile(inputPath, outputPath string, options *FormatOptions) error {
	if outputPath == "" {
		outputPath = inputPath
	}

	result, err := c.manager.FormatFile(inputPath, outputPath, options)
	if err != nil {
		return err
	}

	c.logger.Info("file formatted",
		zap.String("input", inputPath),
		zap.String("output", outputPath),
		zap.Bool("changed", result.Changed),
		zap.String("formatter", result.FormatterUsed),
		zap.Duration("duration", result.Duration))

	return nil
}

// ListFormatters 列出可用的格式化器
func (c *FormatCommand) ListFormatters() {
	formatters := c.manager.ListAvailableFormatters()

	fmt.Println("Available formatters:")
	for format, names := range formatters {
		fmt.Printf("\n%s:\n", format)
		for _, name := range names {
			fmt.Printf("  - %s\n", name)
		}
	}
}
