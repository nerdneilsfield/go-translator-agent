package html

import (
	"context"
	"fmt"
	"sync"

	"github.com/nerdneilsfield/go-translator-agent/internal/formats/base"
	"github.com/nerdneilsfield/go-translator-agent/pkg/document"
)

// NativeProcessor 原生 HTML 处理器
type NativeProcessor struct {
	*base.Processor
}

// NewNativeProcessor 创建原生 HTML 处理器
func NewNativeProcessor(opts document.ProcessorOptions) (*NativeProcessor, error) {
	parser := NewNativeParser()
	renderer := NewNativeRenderer()
	chunker := base.NewSmartChunker()
	
	baseProcessor := base.NewProcessor(parser, renderer, chunker, opts)
	
	return &NativeProcessor{
		Processor: baseProcessor,
	}, nil
}

// GetFormat 返回处理器支持的格式
func (p *NativeProcessor) GetFormat() document.Format {
	return document.FormatHTML
}

// Process 处理文档（重写以保留 HTMLBlock 类型）
func (p *NativeProcessor) Process(ctx context.Context, doc *document.Document, translator document.TranslateFunc) (*document.Document, error) {
	if doc == nil {
		return nil, fmt.Errorf("document is nil")
	}

	// 创建处理后的文档
	processedDoc := &document.Document{
		ID:        doc.ID,
		Format:    doc.Format,
		Metadata:  doc.Metadata,
		Blocks:    make([]document.Block, 0, len(doc.Blocks)),
		Resources: doc.Resources,
	}

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		errors []error
	)

	// 并发处理每个块
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
			translatedContent, err := translator(ctx, b.GetContent())
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to translate block %s: %w", b.GetType(), err))
				mu.Unlock()
				return
			}

			// 如果是 HTMLBlock，保留其类型和节点引用
			if htmlBlock, ok := b.(*HTMLBlock); ok {
				newBlock := &HTMLBlock{
					BaseBlock: document.BaseBlock{
						Type:         b.GetType(),
						Content:      translatedContent,
						Translatable: b.IsTranslatable(),
						Metadata:     b.GetMetadata(),
					},
					node: htmlBlock.node,
				}
				mu.Lock()
				processedDoc.Blocks = append(processedDoc.Blocks, newBlock)
				mu.Unlock()
			} else {
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
			}
		}(block)
	}

	wg.Wait()

	if len(errors) > 0 {
		return nil, fmt.Errorf("processing failed with %d errors: %v", len(errors), errors[0])
	}

	return processedDoc, nil
}

// Factory 创建原生 HTML 处理器的工厂函数
func NativeFactory(opts document.ProcessorOptions) (document.Processor, error) {
	return NewNativeProcessor(opts)
}