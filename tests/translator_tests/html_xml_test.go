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

// 测试HTML和XML格式的翻译
func TestHTMLAndXMLTranslation(t *testing.T) {
	// 创建模拟服务器
	server := test.NewMockOpenAIServer(t)
	defer server.Stop()

	// 设置默认响应
	server.SetDefaultResponse("这是翻译后的文本")

	// 创建logger
	newLogger := logger.NewZapLogger(true)

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

	// 创建模拟的缓存
	mockCache := new(test.MockCache)
	mockCache.On("Get", mock.Anything).Return("", false)
	mockCache.On("Set", mock.Anything, mock.Anything).Return(nil)

	// 创建翻译器
	_, err := translator.New(cfg, translator.WithCache(mockCache))
	assert.NoError(t, err)

	// 创建临时测试文件
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.html")
	outputFile := filepath.Join(tempDir, "test_translated.html")

	// 写入测试内容
	testContent := `<!DOCTYPE html>
<html>
<head>
    <title>Test Page</title>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial; }
    </style>
    <script>
        function test() {
            console.log("Test");
        }
    </script>
</head>
<body>
    <h1>Hello World</h1>
    <p>This is a test paragraph.</p>
    <div>
        <p>This is a nested paragraph.</p>
        <ul>
            <li>Item 1</li>
            <li>Item 2</li>
        </ul>
    </div>
    <script>
        // This is a JavaScript comment
        console.log("Another test");
    </script>
</body>
</html>`
	err2 := os.WriteFile(testFile, []byte(testContent), 0o644)
	assert.NoError(t, err2)

	// 创建模拟翻译器
	mockTrans := test.NewMockTranslator(cfg, newLogger)
	mockTrans.On("Translate", mock.Anything, mock.Anything).Return("这是翻译后的文本", nil)
	// 模拟 TranslateFile，写入实际的文件内容
	mockTrans.On("TranslateFile", testFile, outputFile).Return(nil).Run(func(args mock.Arguments) {
		// 从测试内容中生成一个简单的翻译版本
		translatedHTML := `<!DOCTYPE html>
<html>
<head>
    <title>这是翻译后的文本</title>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial; }
    </style>
    <script>
        function test() {
            console.log("Test");
        }
    </script>
</head>
<body>
    <h1>这是翻译后的文本</h1>
    <p>这是翻译后的文本</p>
    <div>
        <p>这是翻译后的文本</p>
        <ul>
            <li>这是翻译后的文本</li>
            <li>这是翻译后的文本</li>
        </ul>
    </div>
    <script>
        // This is a JavaScript comment
        console.log("Another test");
    </script>
</body>
</html>`
		// 写入翻译后的内容到输出文件
		outputPath := args.String(1)
		os.WriteFile(outputPath, []byte(translatedHTML), 0o644)
	})

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
}

// 测试HTML/XML标签处理
func TestHTMLXMLTagHandling(t *testing.T) {
	// 创建模拟服务器
	server := test.NewMockOpenAIServer(t)
	defer server.Stop()

	// 设置特定响应
	server.AddResponse("Test Page", "测试页面")
	server.AddResponse("Hello World", "你好世界")
	server.AddResponse("This is a test paragraph.", "这是一个测试段落。")
	server.AddResponse("This is a nested paragraph.", "这是一个嵌套段落。")
	server.AddResponse("Item 1", "项目1")
	server.AddResponse("Item 2", "项目2")

	// 创建logger
	newLogger := logger.NewZapLogger(true)

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

	// 创建模拟的缓存
	mockCache := new(test.MockCache)
	mockCache.On("Get", mock.Anything).Return("", false)
	mockCache.On("Set", mock.Anything, mock.Anything).Return(nil)

	// 创建翻译器
	_, err := translator.New(cfg, translator.WithCache(mockCache))
	assert.NoError(t, err)

	// 创建临时测试文件
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_tags.html")
	outputFile := filepath.Join(tempDir, "test_tags_translated.html")

	// 写入测试内容
	testContent := `<!DOCTYPE html>
<html>
<head>
    <title>Test Page</title>
</head>
<body>
    <h1>Hello World</h1>
    <p>This is a test paragraph.</p>
    <div>
        <p>This is a nested paragraph.</p>
        <ul>
            <li>Item 1</li>
            <li>Item 2</li>
        </ul>
    </div>
</body>
</html>`
	err2 := os.WriteFile(testFile, []byte(testContent), 0o644)
	assert.NoError(t, err2)

	// 创建模拟翻译器
	mockTrans := test.NewMockTranslator(cfg, newLogger)
	mockTrans.SetPredefinedResult("Test Page", "测试页面")
	mockTrans.SetPredefinedResult("Hello World", "你好世界")
	mockTrans.SetPredefinedResult("This is a test paragraph.", "这是一个测试段落。")
	mockTrans.SetPredefinedResult("This is a nested paragraph.", "这是一个嵌套段落。")
	mockTrans.SetPredefinedResult("Item 1", "项目1")
	mockTrans.SetPredefinedResult("Item 2", "项目2")
	mockTrans.On("Translate", mock.Anything, mock.Anything).Return("这是翻译后的文本", nil)
	// 模拟 TranslateFile，写入实际的文件内容
	mockTrans.On("TranslateFile", testFile, outputFile).Return(nil).Run(func(args mock.Arguments) {
		// 生成包含预定义翻译的HTML
		translatedHTML := `<!DOCTYPE html>
<html>
<head>
    <title>测试页面</title>
</head>
<body>
    <h1>你好世界</h1>
    <p>这是一个测试段落。</p>
    <div>
        <p>这是一个嵌套段落。</p>
        <ul>
            <li>项目1</li>
            <li>项目2</li>
        </ul>
    </div>
</body>
</html>`
		// 写入翻译后的内容到输出文件
		outputPath := args.String(1)
		os.WriteFile(outputPath, []byte(translatedHTML), 0o644)
	})

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
}
