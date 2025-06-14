package translator_tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/internal/test"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// 测试Markdown格式的翻译
func TestMarkdownTranslation(t *testing.T) {
	// 创建模拟服务器
	server := test.NewMockOpenAIServer(t)
	defer server.Stop()

	// 设置默认响应
	server.SetDefaultResponse("这是翻译后的文本")

	// 创建logger
	zapLogger := logger.NewZapLogger(true)

	// 创建配置
	cfg := test.CreateTestConfig()
	// 设置模型配置
	cfg.ModelConfigs["test-model"] = config.ModelConfig{
		Name:            "test-model",
		APIType:         "openai",
		BaseURL:         server.URL,
		Key:             "sk-test",
		MaxInputTokens:  8000,
		MaxOutputTokens: 2000,
	}

	// 创建日志
	_ = zapLogger

	// 创建模拟的缓存
	mockCache := new(test.MockCache)
	mockCache.On("Get", mock.Anything).Return("", false)
	mockCache.On("Set", mock.Anything, mock.Anything).Return(nil)

	// 创建翻译器
	_, err := translator.New(cfg, translator.WithCache(mockCache))
	assert.NoError(t, err)

	// 创建临时测试文件
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.md")
	outputFile := filepath.Join(tempDir, "test_translated.md")

	// 写入测试内容
	testContent := `# Test Markdown

This is a test paragraph.

## Subsection

- Item 1
- Item 2

` + "```go" + `
func main() {
    fmt.Println("Hello, world!")
}
` + "```" + `

**Bold text** and *italic text*.

> This is a quote.

![Image](image.png)

[![Image](image.png)](https://example.com)


inline formula $\mathcal{s}$

$$
\mathcal{S}_{t}^{p} \int_{j}^{k}
$$

[Link](https://example.com)

| Column 1 | Column 2 |
|----------|----------|
| Cell 1   | Cell 2   |
| Cell 3   | Cell 4   |
`
	err2 := os.WriteFile(testFile, []byte(testContent), 0o644)
	assert.NoError(t, err2)

	// 创建模拟翻译器
	mockTrans := test.NewMockTranslator(cfg, zapLogger)
	mockTrans.On("Translate", mock.Anything, mock.Anything).Return("这是翻译后的文本", nil)

	// 执行翻译
	err3 := mockTrans.TranslateFile(testFile, outputFile)
	assert.NoError(t, err3)

	// 验证输出文件存在
	_, err4 := os.Stat(outputFile)
	assert.NoError(t, err4)

	// 读取翻译结果
	translatedContent, err5 := os.ReadFile(outputFile)
	assert.NoError(t, err5)
	translatedText := string(translatedContent)

	// 验证结果
	assert.Contains(t, translatedText, "这是翻译后的文本")

	// 验证代码块未被翻译
	assert.Contains(t, translatedText, "```go")
	assert.Contains(t, translatedText, "func main() {")
	assert.Contains(t, translatedText, "fmt.Println(\"Hello, world!\")")
	assert.Contains(t, translatedText, "}")
	assert.Contains(t, translatedText, "```")
}

// 测试Markdown格式的批处理翻译
func TestMarkdownBatchTranslation(t *testing.T) {
	// 创建模拟服务器
	server := test.NewMockOpenAIServer(t)
	defer server.Stop()

	// 设置特定响应
	server.AddResponse("This is paragraph 1.", "这是第1段。")
	server.AddResponse("This is paragraph 2.", "这是第2段。")
	server.AddResponse("This is paragraph 3.", "这是第3段。")

	// 创建logger
	zapLogger := logger.NewZapLogger(true)

	// 创建配置
	cfg := test.CreateTestConfig()
	// 设置模型配置
	cfg.ModelConfigs["test-model"] = config.ModelConfig{
		Name:            "test-model",
		APIType:         "openai",
		BaseURL:         server.URL,
		Key:             "sk-test",
		MaxInputTokens:  8000,
		MaxOutputTokens: 2000,
	}

	// 创建日志
	_ = zapLogger

	// 创建模拟的缓存
	mockCache := new(test.MockCache)
	mockCache.On("Get", mock.Anything).Return("", false)
	mockCache.On("Set", mock.Anything, mock.Anything).Return(nil)

	// 创建翻译器
	_, err := translator.New(cfg, translator.WithCache(mockCache))
	assert.NoError(t, err)

	// 创建临时测试文件
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_batch.md")
	outputFile := filepath.Join(tempDir, "test_batch_translated.md")

	// 写入测试内容
	testContent := `# Test Batch Translation

This is paragraph 1.

This is paragraph 2.

This is paragraph 3.
`
	err2 := os.WriteFile(testFile, []byte(testContent), 0o644)
	assert.NoError(t, err2)

	// 创建模拟翻译器
	mockTrans := test.NewMockTranslator(cfg, zapLogger)
	mockTrans.SetPredefinedResult("This is paragraph 1.", "这是第1段。")
	mockTrans.SetPredefinedResult("This is paragraph 2.", "这是第2段。")
	mockTrans.SetPredefinedResult("This is paragraph 3.", "这是第3段。")
	mockTrans.On("Translate", mock.Anything, mock.Anything).Return("这是翻译后的文本", nil)

	// 执行翻译
	err3 := mockTrans.TranslateFile(testFile, outputFile)
	assert.NoError(t, err3)

	// 验证输出文件存在
	_, err4 := os.Stat(outputFile)
	assert.NoError(t, err4)

	// 读取翻译结果
	translatedContent, err5 := os.ReadFile(outputFile)
	assert.NoError(t, err5)
	translatedText := string(translatedContent)

	// 验证结果
	assert.Contains(t, translatedText, "这是第1段")
	assert.Contains(t, translatedText, "这是第2段")
	assert.Contains(t, translatedText, "这是第3段")
}
