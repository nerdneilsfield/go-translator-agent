package formats

import (
	"fmt"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// EPUBProcessor 是EPUB电子书的处理器
type EPUBProcessor struct {
	BaseProcessor
}

// NewEPUBProcessor 创建一个新的EPUB处理器
func NewEPUBProcessor(t translator.Translator, predefinedTranslations *config.PredefinedTranslation) (*EPUBProcessor, error) {
	return &EPUBProcessor{
		BaseProcessor: BaseProcessor{
			Translator:             t,
			Name:                   "EPUB",
			predefinedTranslations: predefinedTranslations,
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

// FormatFile 格式化EPUB文件
func (p *EPUBProcessor) FormatFile(inputPath, outputPath string) error {
	return fmt.Errorf("EPUB格式暂不支持格式化功能")
}

// EPUBFormattingProcessor 是 EPUB 格式化处理器
type EPUBFormattingProcessor struct {
	logger *zap.Logger
}

// NewEPUBFormattingProcessor 创建一个新的 EPUB 格式化处理器
func NewEPUBFormattingProcessor() (*EPUBFormattingProcessor, error) {
	zapLogger, _ := zap.NewProduction()
	return &EPUBFormattingProcessor{
		logger: zapLogger,
	}, nil
}

// FormatFile 格式化 EPUB 文件
func (p *EPUBFormattingProcessor) FormatFile(inputPath, outputPath string) error {
	return fmt.Errorf("EPUB格式暂不支持格式化功能")
}
