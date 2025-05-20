package translator_tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/formats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// 创建全流程测试配置
func createFullTestConfig() *config.Config {
	return &config.Config{
		DefaultModelName: "test-model",
		ModelConfigs: map[string]config.ModelConfig{
			"test-model": {
				Name:            "test-model",
				APIType:         "openai",
				BaseURL:         "http://localhost:8080",
				Key:             "sk-test",
				MaxInputTokens:  8000,
				MaxOutputTokens: 2000,
			},
		},
		RequestTimeout: 10, // 10秒超时
		MinSplitSize:   100,
		MaxSplitSize:   1000,
		Concurrency:    2,
	}
}

// 测试全流程翻译
func TestFullTranslationWorkflow(t *testing.T) {
	// 创建logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()

	// 创建模拟OpenAI服务器
	server := NewMockOpenAIServer(t)
	defer server.Stop()

	// 创建配置
	cfg := createFullTestConfig()
	// 设置模型配置
	cfg.ModelConfigs["test-model"] = config.ModelConfig{
		Name:            "test-model",
		APIType:         "openai",
		BaseURL:         server.URL,
		Key:             "sk-test",
		MaxInputTokens:  8000,
		MaxOutputTokens: 2000,
	}

	// 创建模拟翻译器
	mockTrans := NewMockTranslator(cfg, zapLogger)

	// 设置预定义翻译结果
	mockTrans.SetPredefinedResult("这是真的", "This is true")
	mockTrans.SetPredefinedResult("战福", "Zhan Fu")
	mockTrans.SetPredefinedResult("绿毛水怪", "Green-haired Water Monster")
	mockTrans.SetPredefinedResult("人妖", "Human Monster")
	mockTrans.On("Translate", mock.Anything, mock.Anything).Return("这是翻译后的文本", nil)

	// 测试文本文件翻译
	t.Run("Text File Translation", func(t *testing.T) {
		// 准备测试文件
		testFile := filepath.Join("..", "file", "test.txt")
		outputFile := filepath.Join(t.TempDir(), "test_translated.txt")

		// 执行翻译
		err := mockTrans.TranslateFile(testFile, outputFile)
		assert.NoError(t, err)

		// 验证输出文件存在
		_, err = os.Stat(outputFile)
		assert.NoError(t, err)

		// 读取翻译结果
		content, err := os.ReadFile(outputFile)
		assert.NoError(t, err)
		assert.Contains(t, string(content), "这是翻译后的文本")
	})

	// 测试Markdown文件翻译
	t.Run("Markdown File Translation", func(t *testing.T) {
		// 准备测试文件
		testFile := filepath.Join("..", "file", "test.md")
		outputFile := filepath.Join(t.TempDir(), "test_translated.md")

		// 执行翻译
		err := mockTrans.TranslateFile(testFile, outputFile)
		assert.NoError(t, err)

		// 验证输出文件存在
		_, err = os.Stat(outputFile)
		assert.NoError(t, err)

		// 读取翻译结果
		content, err := os.ReadFile(outputFile)
		assert.NoError(t, err)
		assert.Contains(t, string(content), "这是翻译后的文本")
	})

	// 测试EPUB文件翻译
	t.Run("EPUB File Translation", func(t *testing.T) {
		// 准备测试文件
		testFile := filepath.Join("..", "file", "test.epub")
		outputFile := filepath.Join(t.TempDir(), "test_translated.epub")

		// 执行翻译
		err := mockTrans.TranslateFile(testFile, outputFile)
		assert.NoError(t, err)

		// 验证输出文件存在
		_, err = os.Stat(outputFile)
		assert.NoError(t, err)
	})

	// 测试HTML文件翻译
	t.Run("HTML File Translation with goquery", func(t *testing.T) {
		// 创建测试HTML内容
		htmlContent := `<!DOCTYPE html>
<html>
<head>
    <title>测试页面</title>
</head>
<body>
    <h1>这是真的</h1>
    <p>战福是一个角色。</p>
    <div>
        <p>绿毛水怪是另一个角色。</p>
        <ul>
            <li>人妖</li>
            <li>这是第二项</li>
        </ul>
    </div>
</body>
</html>`

		// 创建临时测试文件
		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "test.html")
		outputFile := filepath.Join(tempDir, "test_translated.html")

		// 写入测试内容
		err := os.WriteFile(testFile, []byte(htmlContent), 0644)
		assert.NoError(t, err)

		// 使用goquery翻译HTML
		translated, err := formats.TranslateHTMLWithGoQuery(htmlContent, mockTrans, zapLogger)
		assert.NoError(t, err)

		// 写入翻译结果
		err = os.WriteFile(outputFile, []byte(translated), 0644)
		assert.NoError(t, err)

		// 验证翻译结果
		assert.Contains(t, translated, "This is true")
		assert.Contains(t, translated, "Zhan Fu")
		assert.Contains(t, translated, "Green-haired Water Monster")
		assert.Contains(t, translated, "Human Monster")

		// 验证HTML结构保持不变
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(translated))
		assert.NoError(t, err)
		assert.Equal(t, 1, doc.Find("title").Length())
		assert.Equal(t, 1, doc.Find("h1").Length())
		assert.Equal(t, 2, doc.Find("p").Length())
		assert.Equal(t, 2, doc.Find("li").Length())
	})
}
