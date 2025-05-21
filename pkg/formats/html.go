package formats

import (
	"fmt"
	"os"
	"strings"

	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// HTMLProcessor 是HTML文件的处理器
type HTMLProcessor struct {
	BaseProcessor
	replacements []ReplacementInfo
}

// NewHTMLProcessor 创建一个新的HTML处理器
func NewHTMLProcessor(t translator.Translator, predefinedTranslations *config.PredefinedTranslation, progressBar *progress.Writer) (*HTMLProcessor, error) {
	// 获取logger，如果无法转换则创建新的
	zapLogger, _ := zap.NewProduction()
	if loggerProvider, ok := t.GetLogger().(interface{ GetZapLogger() *zap.Logger }); ok {
		if zl := loggerProvider.GetZapLogger(); zl != nil {
			zapLogger = zl
		}
	}
	return &HTMLProcessor{
		BaseProcessor: BaseProcessor{
			Translator:             t,
			Name:                   "HTML",
			predefinedTranslations: predefinedTranslations,
			progressBar:            progressBar,
			logger:                 zapLogger,
		},
		replacements: []ReplacementInfo{},
	}, nil
}

// TranslateFile 翻译HTML文件
func (p *HTMLProcessor) TranslateFile(inputPath, outputPath string) error {
	// 读取输入文件
	content, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("读取文件失败 %s: %w", inputPath, err)
	}

	p.logger.Info("开始翻译HTML文件",
		zap.String("输入文件", inputPath),
		zap.String("输出文件", outputPath),
		zap.Int("文件大小", len(content)),
	)

	// 保护文本中的预定义翻译
	protectedText, err := p.ProtectText(string(content))
	if err != nil {
		return fmt.Errorf("保护文本失败: %w", err)
	}

	// 翻译文本
	translated, err := p.TranslateText(protectedText)
	if err != nil {
		return fmt.Errorf("翻译HTML文件失败: %w", err)
	}

	// 还原被保护的文本
	restoredText, err := p.RestoreText(translated)
	if err != nil {
		return fmt.Errorf("还原文本失败: %w", err)
	}

	// 写入输出文件
	if err := os.WriteFile(outputPath, []byte(restoredText), 0644); err != nil {
		return fmt.Errorf("写入文件失败 %s: %w", outputPath, err)
	}

	// 格式化输出文件
	if err := FormatFile(outputPath, p.logger); err != nil {
		p.logger.Warn("格式化HTML文件失败", zap.Error(err))
	}

	p.logger.Debug("HTML文件翻译完成",
		zap.String("输出文件", outputPath),
		zap.Int("原始长度", len(content)),
		zap.Int("翻译长度", len(restoredText)),
	)

	return nil
}

// TranslateText 翻译HTML内容
func (p *HTMLProcessor) TranslateText(text string) (string, error) {
	// 使用goquery库翻译HTML
	translated, err := TranslateHTMLWithGoQuery(text, p.Translator, p.logger)
	if err != nil {
		p.logger.Warn("使用goquery翻译HTML失败", zap.Error(err))
		return "", err
	}
	return translated, nil
}

// ProtectText 保护文本中的预定义翻译
func (p *HTMLProcessor) ProtectText(text string) (string, error) {
	placeholderIndex := 0

	for key, value := range p.predefinedTranslations.Translations {
		placeholder := fmt.Sprintf("@@PRESERVE_%d@@", placeholderIndex)
		p.replacements = append(p.replacements, ReplacementInfo{
			Placeholder: placeholder,
			Original:    value,
		})
		placeholderIndex++
		text = strings.ReplaceAll(text, key, placeholder)
	}

	return text, nil
}

// RestoreText 还原被保护的文本
func (p *HTMLProcessor) RestoreText(text string) (string, error) {
	for _, replacement := range p.replacements {
		text = strings.ReplaceAll(text, replacement.Placeholder, replacement.Original)
	}
	return text, nil
}
