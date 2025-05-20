package test

import (
	"github.com/stretchr/testify/mock"
)

// MockLLMClient 是一个模拟的LLM客户端
type MockLLMClient struct {
	mock.Mock
}

// Complete 执行完成请求
func (m *MockLLMClient) Complete(prompt string, maxTokens int, temperature float64) (string, int, int, error) {
	args := m.Called(prompt, maxTokens, temperature)
	return args.String(0), args.Int(1), args.Int(2), args.Error(3)
}

// Name 返回模型名称
func (m *MockLLMClient) Name() string {
	args := m.Called()
	return args.String(0)
}

// Type 返回模型类型
func (m *MockLLMClient) Type() string {
	args := m.Called()
	return args.String(0)
}

// MaxInputTokens 返回最大输入令牌数
func (m *MockLLMClient) MaxInputTokens() int {
	args := m.Called()
	return args.Int(0)
}

// MaxOutputTokens 返回最大输出令牌数
func (m *MockLLMClient) MaxOutputTokens() int {
	args := m.Called()
	return args.Int(0)
}

// GetInputTokenPrice 返回输入令牌价格
func (m *MockLLMClient) GetInputTokenPrice() float64 {
	args := m.Called()
	return args.Get(0).(float64)
}

// GetOutputTokenPrice 返回输出令牌价格
func (m *MockLLMClient) GetOutputTokenPrice() float64 {
	args := m.Called()
	return args.Get(0).(float64)
}

// GetPriceUnit 返回价格单位
func (m *MockLLMClient) GetPriceUnit() string {
	args := m.Called()
	return args.String(0)
}
