package markdown

import (
	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/nerdneilsfield/go-translator-agent/internal/formats/base"
)

// Processor Markdown 处理器
type Processor struct {
	*base.Processor
}

// NewProcessor 创建 Markdown 处理器
func NewProcessor(opts document.ProcessorOptions) (*Processor, error) {
	parser := NewParser()
	renderer := NewRenderer()
	chunker := base.NewSmartChunker()

	baseProcessor := base.NewProcessor(parser, renderer, chunker, opts)

	return &Processor{
		Processor: baseProcessor,
	}, nil
}

// GetFormat 返回处理器支持的格式
func (p *Processor) GetFormat() document.Format {
	return document.FormatMarkdown
}

// Factory 创建 Markdown 处理器的工厂函数
func Factory(opts document.ProcessorOptions) (document.Processor, error) {
	return NewProcessor(opts)
}

// init 注册 Markdown 处理器
func init() {
	document.Register(document.FormatMarkdown, Factory)
}
