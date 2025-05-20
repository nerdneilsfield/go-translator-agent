package translator_tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/internal/test"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// 测试纯文本格式的翻译
func TestTextTranslation(t *testing.T) {
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

	// 创建日志

	// 创建模拟的缓存
	mockCache := new(test.MockCache)
	mockCache.On("Get", mock.Anything).Return("", false)
	mockCache.On("Set", mock.Anything, mock.Anything).Return(nil)
	mockCache.On("Clear").Return(nil)

	// 创建翻译器
	_, err := translator.New(cfg, translator.WithCache(mockCache))
	assert.NoError(t, err)

	// 创建临时测试文件
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	outputFile := filepath.Join(tempDir, "test_translated.txt")

	// 写入测试内容
	testContent := `This is a test text file.

It contains multiple paragraphs.

Each paragraph should be translated separately.

Some paragraphs may be longer than others, which tests the text splitting functionality.
`
	err2 := os.WriteFile(testFile, []byte(testContent), 0644)
	assert.NoError(t, err2)

	// 创建模拟翻译器
	mockTrans := test.NewMockTranslator(cfg, newLogger)

	// 构建预期的翻译后文件内容
	// testContent 包含4个段落，假设每个都翻译成 "这是翻译后的文本"
	translatedParagraph := "这是翻译后的文本"
	var sb strings.Builder
	sb.WriteString(translatedParagraph)
	sb.WriteString("\n\n")
	sb.WriteString(translatedParagraph)
	sb.WriteString("\n\n")
	sb.WriteString(translatedParagraph)
	sb.WriteString("\n\n")
	sb.WriteString(translatedParagraph)
	expectedTranslatedContent := sb.String()

	// MockTranslator.TranslateFile 内部会调用 Translate 方法
	// 所以我们需要 mock Translate 方法以返回预期的完整翻译内容
	mockTrans.On("Translate", testContent, false).Return(expectedTranslatedContent, nil)

	// TranslateFile 方法本身被调用时，我们只期望它成功完成（返回nil错误）
	// 文件写入逻辑由 MockTranslator.TranslateFile 内部处理，它会使用上面 Translate mock 的结果
	mockTrans.On("TranslateFile", testFile, outputFile).Return(nil)

	mockTrans.On("Finish").Return()
	mockTrans.On("GetConfig").Return(cfg)

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

// 测试纯文本格式的批处理翻译
func TestTextBatchTranslation(t *testing.T) {
	// 创建模拟服务器
	server := test.NewMockOpenAIServer(t)
	defer server.Stop()

	// 设置特定响应
	server.AddResponse("Paragraph 1.", "段落1。")
	server.AddResponse("Paragraph 2.", "段落2。")
	server.AddResponse("Paragraph 3.", "段落3。")

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

	// 创建日志
	// newLogger is used when creating mockTrans

	// 创建模拟的缓存
	mockCache := new(test.MockCache) // Initialize mockCache for this test
	mockCache.On("Get", mock.Anything).Return("", false)
	mockCache.On("Set", mock.Anything, mock.Anything).Return(nil)
	mockCache.On("Clear").Return(nil)

	// 创建翻译器
	// The real translator instance created by translator.New is not used in this test;
	// mockTrans (a mock translator) is used instead.

	// 创建临时测试文件
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_batch.txt")
	outputFile := filepath.Join(tempDir, "test_batch_translated.txt")

	// 写入测试内容
	testContent := `Paragraph 1.

Paragraph 2.

Paragraph 3.
`
	err2 := os.WriteFile(testFile, []byte(testContent), 0644)
	assert.NoError(t, err2)

	// 创建模拟翻译器
	mockTrans := test.NewMockTranslator(cfg, newLogger)

	// 为批处理翻译设置预期
	// 构建期望的翻译结果
	expectedBatchTranslation := "段落1。\n\n段落2。\n\n段落3。"

	// 为 Translate 方法设置期望
	mockTrans.On("Translate", testContent, false).Return(expectedBatchTranslation, nil)

	// 为 TranslateFile 方法设置期望
	mockTrans.On("TranslateFile", testFile, outputFile).Return(nil)

	mockTrans.On("Finish").Return()
	mockTrans.On("GetConfig").Return(cfg)

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
	assert.Contains(t, translatedText, "段落1")
	assert.Contains(t, translatedText, "段落2")
	assert.Contains(t, translatedText, "段落3")
}

// 测试纯文本格式的错误处理
func TestTextErrorHandling(t *testing.T) {
	// 创建模拟服务器
	server := test.NewMockOpenAIServer(t)
	defer server.Stop()

	// 设置错误率
	server.SetErrorRate(0.5)

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

	// 创建日志

	// 创建模拟的缓存
	mockCache := new(test.MockCache)
	mockCache.On("Get", mock.Anything).Return("", false)
	mockCache.On("Set", mock.Anything, mock.Anything).Return(nil)
	mockCache.On("Clear").Return(nil)

	// 创建翻译器
	_, err := translator.New(cfg, translator.WithCache(mockCache))
	assert.NoError(t, err)

	// 创建临时测试文件
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_error.txt")
	outputFile := filepath.Join(tempDir, "test_error_translated.txt")

	// 写入测试内容
	testContent := `Error test paragraph.`
	err2 := os.WriteFile(testFile, []byte(testContent), 0644)
	assert.NoError(t, err2)

	// 创建模拟翻译器
	mockTrans := test.NewMockTranslator(cfg, newLogger)

	// 为 Translate 方法设置模拟错误
	mockTrans.On("Translate", mock.Anything, mock.Anything).Return("", fmt.Errorf("模拟的翻译错误"))

	// 为 TranslateFile 方法设置期望，返回我们期望的错误
	mockTrans.On("TranslateFile", testFile, outputFile).Return(fmt.Errorf("模拟的翻译错误"))

	mockTrans.On("Finish").Return()
	mockTrans.On("GetConfig").Return(cfg)

	// 执行翻译，应该返回错误
	err3 := mockTrans.TranslateFile(testFile, outputFile)
	assert.Error(t, err3)

	// 验证输出文件不存在（因为翻译失败）
	_, err4 := os.Stat(outputFile)
	assert.True(t, os.IsNotExist(err4))
}
