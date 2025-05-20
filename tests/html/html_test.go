package html_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/pkg/formats"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// 创建一个模拟翻译器，用于测试
type MockTranslator struct {
	logger logger.Logger
	cfg    *config.Config
	pt     *translator.TranslationProgressTracker
}

func NewMockTranslator(zapLogger *zap.Logger) *MockTranslator {
	// 创建一个包装的logger
	wrappedLogger := &MockLogger{zapLogger: zapLogger}

	return &MockTranslator{
		logger: wrappedLogger,
		cfg: &config.Config{
			DefaultModelName: "mock",
			ModelConfigs: map[string]config.ModelConfig{
				"mock": {
					MaxInputTokens: 8000,
				},
			},
		},
		pt: &translator.TranslationProgressTracker{},
	}
}

// MockLogger 实现logger.Logger接口
type MockLogger struct {
	zapLogger *zap.Logger
}

func (m *MockLogger) Debug(msg string, fields ...zap.Field) {
	m.zapLogger.Debug(msg, fields...)
}

func (m *MockLogger) Info(msg string, fields ...zap.Field) {
	m.zapLogger.Info(msg, fields...)
}

func (m *MockLogger) Warn(msg string, fields ...zap.Field) {
	m.zapLogger.Warn(msg, fields...)
}

func (m *MockLogger) Error(msg string, fields ...zap.Field) {
	m.zapLogger.Error(msg, fields...)
}

func (m *MockLogger) Fatal(msg string, fields ...zap.Field) {
	m.zapLogger.Fatal(msg, fields...)
}

func (m *MockLogger) With(fields ...zap.Field) logger.Logger {
	return &MockLogger{zapLogger: m.zapLogger.With(fields...)}
}

func (m *MockLogger) GetZapLogger() *zap.Logger {
	return m.zapLogger
}

func (m *MockTranslator) Translate(text string, batch bool) (string, error) {
	// 简单的模拟翻译：在每个文本前添加"[翻译]"
	return "[翻译] " + text, nil
}

func (m *MockTranslator) GetConfig() *config.Config {
	return m.cfg
}

func (m *MockTranslator) GetLogger() logger.Logger {
	return m.logger
}

func (m *MockTranslator) GetProgressTracker() *translator.TranslationProgressTracker {
	return m.pt
}

func (m *MockTranslator) GetProgress() string {
	return "模拟进度"
}

func (m *MockTranslator) InitTranslator() {
	// 空实现
}

func (m *MockTranslator) Finish() {
	// 空实现
}

// 辅助函数
func containsPattern(text, pattern string) bool {
	re := regexp.MustCompile(pattern)
	return re.MatchString(text)
}

func containsXMLDeclaration(text string) bool {
	return containsPattern(text, `<\?xml`)
}

func containsDOCTYPE(text string) bool {
	// 不区分大小写地检查DOCTYPE声明
	return containsPattern(text, `(?i)<!DOCTYPE`) || containsPattern(text, `(?i)<!doctype`)
}

func containsScriptContent(text string) bool {
	// 检查是否包含JavaScript代码的特征
	return containsPattern(text, `function`) ||
		containsPattern(text, `console\.log`) ||
		containsPattern(text, `alert\(`) ||
		containsPattern(text, `// This is a JavaScript`)
}

func containsStyleContent(text string) bool {
	// 检查是否包含CSS样式的特征
	return containsPattern(text, `body \{`) ||
		containsPattern(text, `font-family:`) ||
		containsPattern(text, `margin:`) ||
		containsPattern(text, `padding:`)
}

func TestHTMLTranslation(t *testing.T) {
	// 创建logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 创建模拟翻译器
	mockTranslator := NewMockTranslator(logger)

	// 测试用例
	testCases := []struct {
		name     string
		filename string
	}{
		{
			name:     "Advanced HTML Test",
			filename: "test4.html",
		},
		{
			name:     "Complex Nested Structure Test",
			filename: "test5.html",
		},
		{
			name:     "XML Test",
			filename: "test_xml.xml",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 读取测试文件
			inputPath := filepath.Join(".", tc.filename)
			content, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatalf("读取文件失败 %s: %v", inputPath, err)
			}

			// 使用改进的HTML处理器翻译
			translated, err := formats.TranslateHTMLWithGoQuery(string(content), mockTranslator, logger)
			if err != nil {
				t.Fatalf("翻译失败: %v", err)
			}

			// 保存翻译结果
			outputPath := filepath.Join(".", tc.filename+"_translated")
			if err := os.WriteFile(outputPath, []byte(translated), 0644); err != nil {
				t.Fatalf("写入文件失败 %s: %v", outputPath, err)
			}

			// 验证翻译结果
			// 1. 检查文件是否存在
			if _, err := os.Stat(outputPath); os.IsNotExist(err) {
				t.Fatalf("翻译后的文件不存在: %s", outputPath)
			}

			// 2. 检查翻译后的文件是否包含"[翻译]"标记
			translatedContent, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("读取翻译后的文件失败 %s: %v", outputPath, err)
			}

			if len(translatedContent) == 0 {
				t.Fatalf("翻译后的文件为空: %s", outputPath)
			}

			translatedStr := string(translatedContent)
			if !strings.Contains(translatedStr, "[翻译]") {
				t.Errorf("翻译后的文件不包含翻译标记: %s", outputPath)
			}

			// 3. 检查是否保留了DOCTYPE和XML声明
			if tc.filename == "test_xml.xml" {
				if !containsXMLDeclaration(translatedStr) {
					t.Errorf("翻译后的XML文件丢失了XML声明")
				}
			} else {
				if !containsDOCTYPE(translatedStr) {
					t.Errorf("翻译后的HTML文件丢失了DOCTYPE声明")
				}
			}

			// 4. 检查是否保留了脚本和样式
			contentStr := string(content)
			if containsScriptContent(contentStr) && !containsScriptContent(translatedStr) {
				t.Errorf("翻译后的文件丢失了脚本内容")
			}

			if containsStyleContent(contentStr) && !containsStyleContent(translatedStr) {
				t.Errorf("翻译后的文件丢失了样式内容")
			}

			t.Logf("成功翻译文件: %s -> %s", inputPath, outputPath)
		})
	}
}
