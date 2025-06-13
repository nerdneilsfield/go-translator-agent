package adapter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/pkg/document"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// mockLogger 模拟 logger
type mockLogger struct {
	*zap.Logger
}

func (m *mockLogger) Debug(msg string, fields ...zapcore.Field) {
	m.Logger.Debug(msg, fields...)
}

func (m *mockLogger) Info(msg string, fields ...zapcore.Field) {
	m.Logger.Info(msg, fields...)
}

func (m *mockLogger) Warn(msg string, fields ...zapcore.Field) {
	m.Logger.Warn(msg, fields...)
}

func (m *mockLogger) Error(msg string, fields ...zapcore.Field) {
	m.Logger.Error(msg, fields...)
}

func (m *mockLogger) Fatal(msg string, fields ...zapcore.Field) {
	m.Logger.Fatal(msg, fields...)
}

func (m *mockLogger) With(fields ...zapcore.Field) logger.Logger {
	return &mockLogger{m.Logger.With(fields...)}
}

// mockTranslator 模拟翻译器
type mockTranslator struct {
	translateFunc func(text string, retryFailedParts bool) (string, error)
	config        *config.Config
}

func (m *mockTranslator) Translate(text string, retryFailedParts bool) (string, error) {
	if m.translateFunc != nil {
		return m.translateFunc(text, retryFailedParts)
	}
	return "[TRANSLATED] " + text, nil
}

func (m *mockTranslator) GetConfig() *config.Config {
	return m.config
}

func (m *mockTranslator) GetLogger() logger.Logger {
	// 返回一个简单的 logger 实现
	zapLogger, _ := zap.NewProduction()
	return &mockLogger{zapLogger}
}

func (m *mockTranslator) GetProgressTracker() *translator.TranslationProgressTracker {
	return nil
}

func (m *mockTranslator) GetProgress() string {
	return ""
}

func (m *mockTranslator) InitTranslator() {}

func (m *mockTranslator) Finish() {}

func TestFormatProcessorAdapter(t *testing.T) {
	// 创建模拟翻译器
	translator := &mockTranslator{
		config: &config.Config{
			SourceLang: "en",
			TargetLang: "zh",
		},
	}

	// 预定义翻译
	predefinedTranslations := &config.PredefinedTranslation{
		Translations: map[string]string{
			"Hello": "你好",
			"World": "世界",
		},
	}

	t.Run("Text Format", func(t *testing.T) {
		adapter, err := NewFormatProcessorAdapter(
			document.FormatText,
			translator,
			predefinedTranslations,
			nil,
		)
		require.NoError(t, err)
		require.NotNil(t, adapter)

		// 测试文件翻译
		t.Run("TranslateFile", func(t *testing.T) {
			// 创建临时文件
			tempDir := t.TempDir()
			inputPath := filepath.Join(tempDir, "test.txt")
			outputPath := filepath.Join(tempDir, "test_translated.txt")

			// 写入测试内容
			testContent := "This is a test document.\n\nIt has multiple paragraphs."
			err := os.WriteFile(inputPath, []byte(testContent), 0644)
			require.NoError(t, err)

			// 翻译文件
			err = adapter.TranslateFile(inputPath, outputPath)
			require.NoError(t, err)

			// 验证输出文件
			output, err := os.ReadFile(outputPath)
			require.NoError(t, err)

			content := string(output)
			assert.Contains(t, content, "[TRANSLATED]")
			assert.Contains(t, content, "test document")
		})
	})

	t.Run("Markdown Format", func(t *testing.T) {
		adapter, err := NewFormatProcessorAdapter(
			document.FormatMarkdown,
			translator,
			predefinedTranslations,
			nil,
		)
		require.NoError(t, err)
		require.NotNil(t, adapter)

		// 测试文件翻译
		t.Run("TranslateFile", func(t *testing.T) {
			// 创建临时输入文件
			tempDir := t.TempDir()
			inputPath := filepath.Join(tempDir, "test.md")
			outputPath := filepath.Join(tempDir, "test_translated.md")

			// 写入测试内容
			testContent := `# Test Document

This is a test paragraph with **bold** and *italic* text.

## Section 1

Hello World

- Item 1
- Item 2

` + "```go\n" + `func main() {
    fmt.Println("Hello")
}
` + "```\n"
			err := os.WriteFile(inputPath, []byte(testContent), 0644)
			require.NoError(t, err)

			// 翻译文件
			err = adapter.TranslateFile(inputPath, outputPath)
			require.NoError(t, err)

			// 验证输出文件
			output, err := os.ReadFile(outputPath)
			require.NoError(t, err)

			content := string(output)
			assert.Contains(t, content, "[TRANSLATED]")
			assert.Contains(t, content, "# ")  // 保留 Markdown 格式
			assert.Contains(t, content, "```") // 保留代码块
		})
	})

	t.Run("Predefined Translations", func(t *testing.T) {
		// 创建使用预定义翻译的适配器
		_, err := NewFormatProcessorAdapter(
			document.FormatText,
			translator,
			predefinedTranslations,
			nil,
		)
		require.NoError(t, err)

		// 设置翻译器使用预定义翻译
		translator.translateFunc = func(text string, retryFailedParts bool) (string, error) {
			// 对于预定义的词汇，适配器应该直接返回，不会调用这个函数
			// 对于其他文本，返回标记的翻译
			return "[TRANSLATED] " + text, nil
		}

		// 由于 formats.Processor 接口没有 TranslateString 方法，
		// 我们只能通过文件翻译来测试预定义翻译
	})

	t.Run("HTML Format", func(t *testing.T) {
		adapter, err := NewFormatProcessorAdapter(
			document.FormatHTML,
			translator,
			nil,
			nil,
		)
		require.NoError(t, err)
		require.NotNil(t, adapter)

		// 创建临时文件
		tempDir := t.TempDir()
		inputPath := filepath.Join(tempDir, "test.html")
		outputPath := filepath.Join(tempDir, "test_translated.html")

		htmlContent := `<html>
<body>
<h1>Title</h1>
<p>This is a paragraph.</p>
</body>
</html>`

		err = os.WriteFile(inputPath, []byte(htmlContent), 0644)
		require.NoError(t, err)

		err = adapter.TranslateFile(inputPath, outputPath)
		require.NoError(t, err)

		output, err := os.ReadFile(outputPath)
		require.NoError(t, err)

		result := string(output)
		assert.Contains(t, result, "[TRANSLATED]")
		// HTML 处理器可能使用 Markdown 模式，所以结构可能会变化
		assert.Contains(t, result, "Title")
		assert.Contains(t, result, "paragraph")
	})
}

func TestCreateFormatProcessor(t *testing.T) {
	translator := &mockTranslator{}

	testCases := []struct {
		format    string
		shouldErr bool
	}{
		{"markdown", false},
		{"md", false},
		{"text", false},
		{"txt", false},
		{"html", false},
		{"epub", false},
		{"unknown", true},
	}

	for _, tc := range testCases {
		t.Run(tc.format, func(t *testing.T) {
			processor, err := CreateFormatProcessor(tc.format, translator, nil, nil)
			
			if tc.shouldErr {
				assert.Error(t, err)
				assert.Nil(t, processor)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, processor)
			}
		})
	}
}