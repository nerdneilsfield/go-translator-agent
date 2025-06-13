package text

import (
	"github.com/nerdneilsfield/go-translator-agent/internal/formats/base"
	"github.com/nerdneilsfield/go-translator-agent/pkg/document"
)

// Processor 纯文本处理器
type Processor struct {
	*base.Processor
}

// NewProcessor 创建纯文本处理器
func NewProcessor(opts document.ProcessorOptions) (*Processor, error) {
	parser := NewParser()
	renderer := NewRenderer()
	chunker := base.NewSimpleChunker() // 纯文本使用简单分块器
	
	baseProcessor := base.NewProcessor(parser, renderer, chunker, opts)
	
	return &Processor{
		Processor: baseProcessor,
	}, nil
}

// GetFormat 返回处理器支持的格式
func (p *Processor) GetFormat() document.Format {
	return document.FormatText
}

// Factory 创建纯文本处理器的工厂函数
func Factory(opts document.ProcessorOptions) (document.Processor, error) {
	return NewProcessor(opts)
}

// init 注册纯文本处理器
func init() {
	document.Register(document.FormatText, Factory)
}