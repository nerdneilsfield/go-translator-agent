package formats

import (
	"fmt"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
)

// LaTeXProcessor 是LaTeX文档的处理器
type LaTeXProcessor struct {
	BaseProcessor
}

// NewLaTeXProcessor 创建一个新的LaTeX处理器
func NewLaTeXProcessor(t translator.Translator) (*LaTeXProcessor, error) {
	return &LaTeXProcessor{
		BaseProcessor: BaseProcessor{
			Translator: t,
			Name:       "LaTeX",
		},
	}, nil
}

// TranslateFile 翻译LaTeX文件
func (p *LaTeXProcessor) TranslateFile(inputPath, outputPath string) error {
	return fmt.Errorf("LaTeX格式暂不支持翻译")
}

// TranslateText 翻译LaTeX内容
func (p *LaTeXProcessor) TranslateText(text string) (string, error) {
	return "", fmt.Errorf("LaTeX格式暂不支持翻译")
}
