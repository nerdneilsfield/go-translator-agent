package base

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/nerdneilsfield/go-translator-agent/pkg/document"
)

// Processor 基础处理器实现
type Processor struct {
	parser   document.Parser
	renderer document.Renderer
	chunker  document.Chunker
	opts     document.ProcessorOptions
}

// NewProcessor 创建基础处理器
func NewProcessor(parser document.Parser, renderer document.Renderer, chunker document.Chunker, opts document.ProcessorOptions) *Processor {
	return &Processor{
		parser:   parser,
		renderer: renderer,
		chunker:  chunker,
		opts:     opts,
	}
}

// Parse 解析输入流为文档结构
func (p *Processor) Parse(ctx context.Context, input io.Reader) (*document.Document, error) {
	if p.parser == nil {
		return nil, fmt.Errorf("parser not set")
	}
	return p.parser.Parse(ctx, input)
}

// Process 处理文档（分块、翻译、重组）
func (p *Processor) Process(ctx context.Context, doc *document.Document, translator document.TranslateFunc) (*document.Document, error) {
	if doc == nil {
		return nil, fmt.Errorf("document is nil")
	}

	// 创建新文档，保留原始元数据
	processedDoc := &document.Document{
		ID:        doc.ID,
		Format:    doc.Format,
		Metadata:  doc.Metadata,
		Blocks:    make([]document.Block, 0, len(doc.Blocks)),
		Resources: doc.Resources,
	}

	// 处理每个块
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	for _, block := range doc.Blocks {
		if !block.IsTranslatable() {
			// 不可翻译的块直接复制
			mu.Lock()
			processedDoc.Blocks = append(processedDoc.Blocks, block)
			mu.Unlock()
			continue
		}

		wg.Add(1)
		go func(b document.Block) {
			defer wg.Done()

			// 翻译块内容
			translatedContent, err := p.translateBlock(ctx, b, translator)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to translate block %s: %w", b.GetType(), err))
				mu.Unlock()
				return
			}

			// 创建新块
			newBlock := &document.BaseBlock{
				Type:         b.GetType(),
				Content:      translatedContent,
				Translatable: b.IsTranslatable(),
				Metadata:     b.GetMetadata(),
			}

			mu.Lock()
			processedDoc.Blocks = append(processedDoc.Blocks, newBlock)
			mu.Unlock()
		}(block)
	}

	wg.Wait()

	if len(errors) > 0 {
		return nil, fmt.Errorf("processing failed with %d errors: %v", len(errors), errors[0])
	}

	return processedDoc, nil
}

// Render 将文档渲染为输出格式
func (p *Processor) Render(ctx context.Context, doc *document.Document, output io.Writer) error {
	if p.renderer == nil {
		return fmt.Errorf("renderer not set")
	}
	return p.renderer.Render(ctx, doc, output)
}

// GetFormat 返回处理器支持的格式
func (p *Processor) GetFormat() document.Format {
	// 基础处理器返回未知格式，具体格式由子类实现
	return document.FormatUnknown
}

// translateBlock 翻译单个块
func (p *Processor) translateBlock(ctx context.Context, block document.Block, translator document.TranslateFunc) (string, error) {
	content := block.GetContent()
	
	// 如果没有分块器，直接翻译整个内容
	if p.chunker == nil {
		return translator(ctx, content)
	}

	// 分块翻译
	chunks := p.chunker.Chunk(content, document.ChunkOptions{
		MaxSize:      p.opts.ChunkSize,
		Overlap:      p.opts.ChunkOverlap,
		PreserveTags: p.opts.PreserveTags,
	})

	// 并发翻译各个分块
	translatedChunks := make([]document.TranslatedChunk, len(chunks))
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	for i, chunk := range chunks {
		wg.Add(1)
		go func(idx int, ch document.Chunk) {
			defer wg.Done()
			
			translated, err := translator(ctx, ch.Content)
			
			mu.Lock()
			translatedChunks[idx] = document.TranslatedChunk{
				Chunk:             ch,
				TranslatedContent: translated,
				Error:             err,
			}
			mu.Unlock()
		}(i, chunk)
	}
	
	wg.Wait()

	// 检查错误
	for _, tc := range translatedChunks {
		if tc.Error != nil {
			return "", tc.Error
		}
	}

	// 合并翻译结果
	return p.chunker.Merge(translatedChunks), nil
}