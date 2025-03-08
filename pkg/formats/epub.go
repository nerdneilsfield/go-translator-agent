package formats

import (
	"fmt"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
)

// EPUBProcessor 是EPUB电子书的处理器
type EPUBProcessor struct {
	BaseProcessor
}

// NewEPUBProcessor 创建一个新的EPUB处理器
func NewEPUBProcessor(t translator.Translator) (*EPUBProcessor, error) {
	return &EPUBProcessor{
		BaseProcessor: BaseProcessor{
			Translator: t,
			Name:       "EPUB",
		},
	}, nil
}

// TranslateFile 翻译EPUB文件
func (p *EPUBProcessor) TranslateFile(inputPath, outputPath string) error {
	return fmt.Errorf("EPUB格式暂不支持翻译")
}

// TranslateText 翻译EPUB内容
func (p *EPUBProcessor) TranslateText(text string) (string, error) {
	return "", fmt.Errorf("EPUB格式暂不支持翻译")
}
