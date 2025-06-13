package epub

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/formats/base"
	"github.com/nerdneilsfield/go-translator-agent/internal/formats/html"
	"github.com/nerdneilsfield/go-translator-agent/pkg/document"
)

// Processor EPUB 格式处理器
type Processor struct {
	*base.Processor
	htmlProcessor document.Processor
}

// NewProcessor 创建 EPUB 处理器
func NewProcessor(opts document.ProcessorOptions) (*Processor, error) {
	// 使用 Markdown 模式的 HTML 处理器
	htmlProcessor, err := html.ProcessorWithMode(html.ModeMarkdown, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTML processor: %w", err)
	}

	parser := NewParser(htmlProcessor)
	renderer := NewRenderer()
	chunker := base.NewSmartChunker()

	baseProcessor := base.NewProcessor(parser, renderer, chunker, opts)

	return &Processor{
		Processor:     baseProcessor,
		htmlProcessor: htmlProcessor,
	}, nil
}

// GetFormat 返回处理器支持的格式
func (p *Processor) GetFormat() document.Format {
	return document.FormatEPUB
}

// Process 处理 EPUB 文档
func (p *Processor) Process(ctx context.Context, doc *document.Document, translator document.TranslateFunc) (*document.Document, error) {
	if doc == nil {
		return nil, fmt.Errorf("document is nil")
	}

	// 创建处理后的文档
	processedDoc := &document.Document{
		ID:        doc.ID,
		Format:    doc.Format,
		Metadata:  doc.Metadata,
		Blocks:    make([]document.Block, 0),
		Resources: make(map[string]document.Resource),
	}

	// 从元数据中获取 EPUB 文件信息
	epubFiles, ok := doc.Metadata.CustomFields["epub_files"].(map[string][]byte)
	if !ok {
		return nil, fmt.Errorf("no EPUB files found in document metadata")
	}

	// 创建处理后的文件映射
	processedFiles := make(map[string][]byte)
	processedDoc.Metadata.CustomFields["epub_files"] = processedFiles

	// 处理每个 HTML 文件
	for filePath, content := range epubFiles {
		// 只处理 HTML/XHTML 文件
		ext := strings.ToLower(path.Ext(filePath))
		if ext != ".html" && ext != ".xhtml" && ext != ".htm" {
			// 非 HTML 文件直接复制
			processedFiles[filePath] = content
			continue
		}

		// 解析 HTML 内容
		reader := bytes.NewReader(content)
		htmlDoc, err := p.htmlProcessor.Parse(ctx, reader)
		if err != nil {
			return nil, fmt.Errorf("failed to parse HTML file %s: %w", filePath, err)
		}

		// 处理 HTML 文档
		processedHTML, err := p.htmlProcessor.Process(ctx, htmlDoc, translator)
		if err != nil {
			return nil, fmt.Errorf("failed to process HTML file %s: %w", filePath, err)
		}

		// 渲染回 HTML
		var output bytes.Buffer
		if err := p.htmlProcessor.Render(ctx, processedHTML, &output); err != nil {
			return nil, fmt.Errorf("failed to render HTML file %s: %w", filePath, err)
		}

		// 保存处理后的 HTML
		processedFiles[filePath] = output.Bytes()

		// 将块添加到文档中（用于进度跟踪）
		processedDoc.Blocks = append(processedDoc.Blocks, processedHTML.Blocks...)
	}

	// 复制资源
	for k, v := range doc.Resources {
		processedDoc.Resources[k] = v
	}

	return processedDoc, nil
}

// Factory 创建 EPUB 处理器的工厂函数
func Factory(opts document.ProcessorOptions) (document.Processor, error) {
	return NewProcessor(opts)
}

// init 注册 EPUB 处理器
func init() {
	document.Register(document.FormatEPUB, Factory)
}