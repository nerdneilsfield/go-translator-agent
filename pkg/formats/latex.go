package formats

import (
	"fmt"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// LaTeXProcessor 是LaTeX文档的处理器
type LaTeXProcessor struct {
	BaseProcessor
}

// NewLaTeXProcessor 创建一个新的LaTeX处理器
func NewLaTeXProcessor(t translator.Translator, predefinedTranslations *config.PredefinedTranslation) (*LaTeXProcessor, error) {
	return &LaTeXProcessor{
		BaseProcessor: BaseProcessor{
			Translator:             t,
			Name:                   "LaTeX",
			predefinedTranslations: predefinedTranslations,
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

// FormatFile 格式化LaTeX文件
func (p *LaTeXProcessor) FormatFile(inputPath, outputPath string) error {
	return fmt.Errorf("LaTeX格式暂不支持格式化功能")
}

// LaTeXFormattingProcessor 是 LaTeX 格式化处理器
type LaTeXFormattingProcessor struct {
	logger *zap.Logger
}

// NewLaTeXFormattingProcessor 创建一个新的 LaTeX 格式化处理器
func NewLaTeXFormattingProcessor() (*LaTeXFormattingProcessor, error) {
	zapLogger, _ := zap.NewProduction()
	return &LaTeXFormattingProcessor{
		logger: zapLogger,
	}, nil
}

// FormatFile 格式化 LaTeX 文件
func (p *LaTeXFormattingProcessor) FormatFile(inputPath, outputPath string) error {
	return fmt.Errorf("LaTeX格式暂不支持格式化功能")
}
