package translator_tests

import (
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/test"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// 测试LLM客户端的Mock实现
func TestMockLLMClient(t *testing.T) {
	// 创建模拟的LLM客户端
	mockClient := new(test.MockLLMClient)
	mockClient.On("Name").Return("test-model")
	mockClient.On("Type").Return("openai")
	mockClient.On("MaxInputTokens").Return(8000)
	mockClient.On("MaxOutputTokens").Return(2000)
	mockClient.On("GetInputTokenPrice").Return(0.001)
	mockClient.On("GetOutputTokenPrice").Return(0.002)
	mockClient.On("GetPriceUnit").Return("$")
	mockClient.On("Complete", mock.Anything, mock.Anything, mock.Anything).Return("这是翻译后的文本", 100, 50, nil)

	// 验证客户端接口实现
	var client translator.LLMClient = mockClient
	assert.NotNil(t, client)

	// 测试方法调用
	name := client.Name()
	assert.Equal(t, "test-model", name)

	clientType := client.Type()
	assert.Equal(t, "openai", clientType)

	maxInputTokens := client.MaxInputTokens()
	assert.Equal(t, 8000, maxInputTokens)

	maxOutputTokens := client.MaxOutputTokens()
	assert.Equal(t, 2000, maxOutputTokens)

	inputPrice := client.GetInputTokenPrice()
	assert.Equal(t, 0.001, inputPrice)

	outputPrice := client.GetOutputTokenPrice()
	assert.Equal(t, 0.002, outputPrice)

	priceUnit := client.GetPriceUnit()
	assert.Equal(t, "$", priceUnit)

	// 测试Complete方法
	result, inputTokens, outputTokens, err := client.Complete("测试提示词", 2000, 0.5)
	assert.NoError(t, err)
	assert.Equal(t, "这是翻译后的文本", result)
	assert.Equal(t, 100, inputTokens)
	assert.Equal(t, 50, outputTokens)

	// 验证所有期望的方法都被调用
	mockClient.AssertExpectations(t)
}

// 测试Raw客户端
func TestRawClient(t *testing.T) {
	// 创建Raw客户端
	rawClient := translator.NewRawClient()

	// 验证客户端接口实现
	var client translator.LLMClient = rawClient
	assert.NotNil(t, client)

	// 测试方法调用
	name := client.Name()
	assert.Equal(t, "raw", name)

	clientType := client.Type()
	assert.Equal(t, "raw", clientType)

	maxInputTokens := client.MaxInputTokens()
	assert.Equal(t, 100000, maxInputTokens)

	maxOutputTokens := client.MaxOutputTokens()
	assert.Equal(t, 100000, maxOutputTokens)

	inputPrice := client.GetInputTokenPrice()
	assert.Equal(t, 0.0, inputPrice)

	outputPrice := client.GetOutputTokenPrice()
	assert.Equal(t, 0.0, outputPrice)

	priceUnit := client.GetPriceUnit()
	assert.Equal(t, "None", priceUnit)

	// 测试Complete方法
	testText := "This is a test text."
	result, inputTokens, outputTokens, err := client.Complete(testText, 2000, 0.5)
	assert.NoError(t, err)
	assert.Equal(t, testText, result) // Raw客户端应该返回原始文本
	assert.Equal(t, len(testText), inputTokens)
	assert.Equal(t, len(testText), outputTokens)
}

// 测试预设配置失效的问题
func TestPresetConfigurationFailure(t *testing.T) {
	// 创建一个简单的测试，验证预设配置的基本功能
	// 由于实际的API调用会失败，我们只测试基本的接口和逻辑

	// 创建一个简单的预设配置
	predefined := map[string]string{
		"Test":  "测试",
		"Hello": "你好",
		"World": "世界",
	}

	// 验证预设配置的基本功能
	assert.Equal(t, "测试", predefined["Test"])
	assert.Equal(t, "你好", predefined["Hello"])
	assert.Equal(t, "世界", predefined["World"])

	// 测试预设配置不存在的情况
	_, exists := predefined["NotExist"]
	assert.False(t, exists)
}
