package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockTranslator 是一个模拟的翻译器实现
type MockTranslator struct {
	mock.Mock
	cfg              *config.Config
	logger           logger.Logger
	predefinedResults map[string]string
}

// MockCache 是一个模拟的缓存实现
type MockCache struct {
	mock.Mock
}

// Get 实现缓存的Get方法
func (m *MockCache) Get(key string) (string, bool) {
	args := m.Called(key)
	return args.String(0), args.Bool(1)
}

// Set 实现缓存的Set方法
func (m *MockCache) Set(key, value string) error {
	args := m.Called(key, value)
	if args.Get(0) != nil {
		return args.Error(0)
	}
	return nil
}

// Clear 实现缓存的Clear方法
func (m *MockCache) Clear() error {
	args := m.Called()
	if args.Get(0) != nil {
		return args.Error(0)
	}
	return nil
}

// NewMockTranslator 创建一个新的模拟翻译器
func NewMockTranslator(cfg *config.Config, logger logger.Logger) *MockTranslator {
	return &MockTranslator{
		cfg:              cfg,
		logger:           logger,
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

// Finish 实现翻译器的Finish方法
func (m *MockTranslator) Finish() {
	m.Called()
}

// GetConfig 实现翻译器的GetConfig方法
func (m *MockTranslator) GetConfig() *config.Config {
	args := m.Called()
	if args.Get(0) != nil {
		return args.Get(0).(*config.Config)
	}
	return m.cfg
}

// TranslateFile 模拟翻译文件
func (m *MockTranslator) TranslateFile(inputPath, outputPath string) error {
	args := m.Called(inputPath, outputPath)
	if args.Get(0) != nil {
		return args.Error(0)
	}

	// 读取输入文件
	content, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	// 获取文件扩展名
	ext := strings.ToLower(filepath.Ext(inputPath))

	// 根据文件类型进行不同的处理
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
		// 对于HTML和XML文件，模拟翻译
		translatedContent, err = m.Translate(string(content), false)
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

	// 写入输出文件
	err = os.WriteFile(outputPath, []byte(translatedContent), 0644)
	if err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}

// CustomZapLogger 是一个自定义的Zap日志记录器
type CustomZapLogger struct {
	logger *zap.Logger
}

// Debug 实现Debug日志方法
func (c *CustomZapLogger) Debug(msg string, fields ...interface{}) {
	c.logger.Debug(msg)
}

// Info 实现Info日志方法
func (c *CustomZapLogger) Info(msg string, fields ...interface{}) {
	c.logger.Info(msg)
}

// Warn 实现Warn日志方法
func (c *CustomZapLogger) Warn(msg string, fields ...interface{}) {
	c.logger.Warn(msg)
}

// Error 实现Error日志方法
func (c *CustomZapLogger) Error(msg string, fields ...interface{}) {
	c.logger.Error(msg)
}

// Fatal 实现Fatal日志方法
func (c *CustomZapLogger) Fatal(msg string, fields ...interface{}) {
	c.logger.Fatal(msg)
}


