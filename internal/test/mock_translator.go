package test

import (
	"archive/zip"
	"fmt"
	"io"
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

// MockTranslator 是一个模拟的翻译器实现
type MockTranslator struct {
	mock.Mock
	logger            logger.Logger
	progressTracker   *translator.TranslationProgressTracker
	config            *config.Config
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
func NewMockTranslator(cfg *config.Config, logger_ logger.Logger) *MockTranslator {
	// convert logger to ZapLogger
	zapLogger := (&logger.ZapLogger{}).GetZapLogger()
	return &MockTranslator{
		config:            cfg,
		logger:            logger_,
		predefinedResults: make(map[string]string),
		progressTracker:   translator.NewTranslationProgressTracker(0, zapLogger, cfg.TargetCurrency, cfg.UsdRmbRate),
	}
}

// SetPredefinedResult 设置预定义的翻译结果
func (m *MockTranslator) SetPredefinedResult(input, output string) {
	m.predefinedResults[input] = output
}

// GetLogger 返回日志记录器
func (m *MockTranslator) GetLogger() logger.Logger {
	return m.logger
}

// GetProgressTracker 返回进度跟踪器
func (m *MockTranslator) GetProgressTracker() *translator.TranslationProgressTracker {
	return m.progressTracker
}

// GetProgress 返回当前进度
func (m *MockTranslator) GetProgress() string {
	return "50%"
}

// GetProgressDetails 返回详细进度信息
func (m *MockTranslator) GetProgressDetails() (int, int, int, int, float64, translator.TokenUsage, translator.EstimatedCost) {
	return m.progressTracker.GetProgress()
}

// InitTranslator 初始化翻译器
func (m *MockTranslator) InitTranslator() {
	// 空实现
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

	// 处理包含节点标记的输入，保持标记并返回固定的翻译内容
	if strings.Contains(text, "@@NODE_") {
		parts := strings.Split(text, "\n\n")
		var results []string
		for _, p := range parts {
			marker := ""
			body := p
			if idx := strings.Index(p, "@@NODE_"); idx != -1 {
				lines := strings.SplitN(p, "\n", 2)
				marker = lines[0]
				if len(lines) > 1 {
					body = lines[1]
				} else {
					body = ""
				}
			}

			translated := "这是翻译后的文本"
			if result, ok := m.predefinedResults[body]; ok {
				translated = result
			} else {
				// 尝试按子串替换预定义结果
				for k, v := range m.predefinedResults {
					if strings.Contains(body, k) {
						translated = strings.ReplaceAll(body, k, v)
						break
					}
				}
			}

			if marker != "" {
				results = append(results, marker+"\n"+translated)
			} else {
				results = append(results, translated)
			}
		}
		return strings.Join(results, "\n\n"), nil
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
	// args := m.Called()
	// if args.Get(0) != nil {
	// 	return args.Get(0).(*config.Config)
	// }
	return m.config
}

// TranslateFile 模拟翻译文件
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
		// 尝试获取 zap.Logger 实例
		zapLogger, _ := zap.NewProduction()
		if loggerProvider, ok := m.logger.(interface{ GetZapLogger() *zap.Logger }); ok {
			zapLogger = loggerProvider.GetZapLogger()
		}
		translatedContent, err = formats.TranslateHTMLWithGoQuery(string(content), m, zapLogger)
		if err != nil {
			return fmt.Errorf("翻译HTML/XML文件失败: %w", err)
		}
	case ".epub":
		// 简单处理EPUB文件：复制所有文件，若为HTML/XHTML文件则附加翻译内容
		reader, err := zip.OpenReader(inputPath)
		if err != nil {
			return fmt.Errorf("打开EPUB失败: %w", err)
		}
		defer reader.Close()

		outFile, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("创建输出文件失败: %w", err)
		}
		defer outFile.Close()

		writer := zip.NewWriter(outFile)
		defer writer.Close()

		for _, f := range reader.File {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return err
			}
			header := f.FileHeader
			w, err := writer.CreateHeader(&header)
			if err != nil {
				return err
			}
			ext := strings.ToLower(filepath.Ext(f.Name))
			if ext == ".xhtml" || ext == ".html" || ext == ".htm" {
				str := string(data)
				if strings.Contains(str, "This is the last paragraph of the chapter.") {
					str = strings.ReplaceAll(str, "This is the last paragraph of the chapter.", "这是本书的最后一段")
				}
				str += "这是翻译后的文本"
				data = []byte(str)
			}
			if _, err := w.Write(data); err != nil {
				return err
			}
		}

		return nil
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

// CustomZapLogger is a mock logger that implements a subset of zap.Logger methods.
type CustomZapLogger struct{}

// Debug logs a debug message.
func (c *CustomZapLogger) Debug(msg string, _ ...interface{}) {
	fmt.Printf("DEBUG: %s\n", msg)
}

// Info logs an info message.
func (c *CustomZapLogger) Info(msg string, _ ...interface{}) {
	fmt.Printf("INFO: %s\n", msg)
}

// Warn logs a warning message.
func (c *CustomZapLogger) Warn(msg string, _ ...interface{}) {
	fmt.Printf("WARN: %s\n", msg)
}

// Error logs an error message.
func (c *CustomZapLogger) Error(msg string, _ ...interface{}) {
	fmt.Printf("ERROR: %s\n", msg)
}

// Fatal logs a fatal message.
func (c *CustomZapLogger) Fatal(msg string, _ ...interface{}) {
	fmt.Printf("FATAL: %s\n", msg)
}
