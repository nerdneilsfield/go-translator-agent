package translator_tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/pkg/formats"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockTranslator 是一个模拟的翻译器实现，用于测试
type MockTranslator struct {
	mock.Mock
	logger            logger.Logger
	progressTracker   *translator.TranslationProgressTracker
	config            *config.Config
	predefinedResults map[string]string
}

// NewMockTranslator 创建一个新的模拟翻译器
func NewMockTranslator(cfg *config.Config, zapLogger *zap.Logger) *MockTranslator {
	log := NewCustomZapLogger(zapLogger)

	progressTracker := translator.NewTranslationProgressTracker(1000)

	return &MockTranslator{
		logger:            log,
		progressTracker:   progressTracker,
		config:            cfg,
		predefinedResults: make(map[string]string),
	}
}

// SetPredefinedResult 设置预定义的翻译结果
func (m *MockTranslator) SetPredefinedResult(input, output string) {
	m.predefinedResults[input] = output
}

// Translate 模拟翻译文本
func (m *MockTranslator) Translate(text string, retryFailedParts bool) (string, error) {
	args := m.Called(text, retryFailedParts)

	// 如果有预定义的结果，返回预定义的结果
	if result, ok := m.predefinedResults[text]; ok {
		return result, nil
	}

	// 如果是错误测试文本，返回错误
	if text == "Error test paragraph." {
		return "", fmt.Errorf("模拟翻译错误")
	}

	// 检查是否包含多个段落
	paragraphs := strings.Split(text, "\n\n")
	if len(paragraphs) > 1 {
		var translatedParagraphs []string
		for i, p := range paragraphs {
			if strings.Contains(p, "Paragraph") || strings.Contains(p, "段落") {
				translatedParagraphs = append(translatedParagraphs, fmt.Sprintf("段落%d", i+1))
			} else {
				translatedParagraphs = append(translatedParagraphs, fmt.Sprintf("这是翻译后的文本 %d", i+1))
			}
		}
		return strings.Join(translatedParagraphs, "\n\n"), nil
	}

	// 如果没有预定义的结果，返回模拟的结果
	if args.Get(0) != nil {
		return args.String(0), args.Error(1)
	}

	// 默认返回"这是翻译后的文本"
	return "这是翻译后的文本", nil
}

// GetLogger 返回日志记录器
func (m *MockTranslator) GetLogger() logger.Logger {
	return m.logger
}

// GetProgressTracker 返回进度跟踪器
func (m *MockTranslator) GetProgressTracker() *translator.TranslationProgressTracker {
	return m.progressTracker
}

// GetConfig 返回配置
func (m *MockTranslator) GetConfig() *config.Config {
	return m.config
}

// GetProgress 返回当前进度
func (m *MockTranslator) GetProgress() string {
	return "50%"
}

// GetProgressDetails 返回详细进度信息
func (m *MockTranslator) GetProgressDetails() (int, int, int, float64, translator.TokenUsage, translator.EstimatedCost) {
	return m.progressTracker.GetProgress()
}

// InitTranslator 初始化翻译器
func (m *MockTranslator) InitTranslator() {
	// 空实现
}

// Finish 结束翻译
func (m *MockTranslator) Finish() {
	// 空实现
}

// TranslateFile 是一个模拟的文件翻译方法
func (m *MockTranslator) TranslateFile(inputPath, outputPath string) error {
	// 检查输入文件是否存在
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("输入文件不存在: %s", inputPath)
	}

	// 读取输入文件
	content, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("读取文件失败 %s: %w", inputPath, err)
	}

	// 获取文件扩展名
	ext := strings.ToLower(filepath.Ext(inputPath))

	// 根据文件类型选择处理方式
	var translatedContent string

	switch ext {
	case ".txt", ".md", ".markdown":
		// 对于文本和Markdown文件，直接翻译内容
		if ext == ".md" || ext == ".markdown" {
			// 对于Markdown文件，需要保留代码块
			lines := strings.Split(string(content), "\n")
			var result []string
			inCodeBlock := false

			for _, line := range lines {
				if strings.HasPrefix(line, "```") {
					inCodeBlock = !inCodeBlock
					result = append(result, line) // 保留代码块标记
					continue
				}

				if inCodeBlock {
					result = append(result, line) // 保留代码块内容
				} else {
					// 翻译非代码块内容
					translated, err := m.Translate(line, false)
					if err != nil {
						return fmt.Errorf("翻译文件内容失败: %w", err)
					}
					result = append(result, translated)
				}
			}

			translatedContent = strings.Join(result, "\n")
		} else {
			// 对于普通文本文件，直接翻译
			translatedContent, err = m.Translate(string(content), false)
			if err != nil {
				return fmt.Errorf("翻译文件内容失败: %w", err)
			}
		}
	case ".html", ".htm", ".xml":
		// 对于HTML和XML文件，使用goquery库翻译
		translatedContent, err = formats.TranslateHTMLWithGoQuery(string(content), m, m.logger.(*CustomZapLogger).logger)
		if err != nil {
			return fmt.Errorf("翻译HTML/XML文件失败: %w", err)
		}
	case ".epub":
		// 对于EPUB文件，模拟翻译
		translatedContent = string(content)
		// 在实际测试中，我们只需要确保输出文件被创建
	default:
		return fmt.Errorf("不支持的文件类型: %s", ext)
	}

	// 创建输出目录（如果不存在）
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败 %s: %w", outputDir, err)
	}

	// 写入输出文件
	if err := os.WriteFile(outputPath, []byte(translatedContent), 0644); err != nil {
		return fmt.Errorf("写入文件失败 %s: %w", outputPath, err)
	}

	return nil
}
