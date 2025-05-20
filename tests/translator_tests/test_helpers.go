package translator_tests

import (
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockLLMClient 是一个模拟的LLM客户端
type MockLLMClient struct {
	mock.Mock
}

func (m *MockLLMClient) Complete(prompt string, maxTokens int, temperature float64) (string, int, int, error) {
	args := m.Called(prompt, maxTokens, temperature)
	return args.String(0), args.Int(1), args.Int(2), args.Error(3)
}

func (m *MockLLMClient) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockLLMClient) Type() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockLLMClient) MaxInputTokens() int {
	args := m.Called()
	return args.Int(0)
}

func (m *MockLLMClient) MaxOutputTokens() int {
	args := m.Called()
	return args.Int(0)
}

func (m *MockLLMClient) GetInputTokenPrice() float64 {
	args := m.Called()
	return args.Get(0).(float64)
}

func (m *MockLLMClient) GetOutputTokenPrice() float64 {
	args := m.Called()
	return args.Get(0).(float64)
}

func (m *MockLLMClient) GetPriceUnit() string {
	args := m.Called()
	return args.String(0)
}

// MockCache 是一个模拟的缓存
type MockCache struct {
	mock.Mock
}

func (m *MockCache) Get(key string) (string, bool) {
	args := m.Called(key)
	return args.String(0), args.Bool(1)
}

func (m *MockCache) Set(key string, value string) error {
	args := m.Called(key, value)
	return args.Error(0)
}

func (m *MockCache) Clear() error {
	args := m.Called()
	return args.Error(0)
}

// 创建测试用的配置
func createTestConfig() *config.Config {
	return &config.Config{
		SourceLang:       "English",
		TargetLang:       "Chinese",
		Country:          "China",
		DefaultModelName: "test-model",
		UseCache:         true,
		Debug:            true,
		ActiveStepSet:    "test-step-set",
		StepSets: map[string]config.StepSetConfig{
			"test-step-set": {
				ID:          "test-step-set",
				Name:        "测试步骤集",
				Description: "用于测试的步骤集",
				InitialTranslation: config.StepConfig{
					Name:        "初始翻译",
					ModelName:   "test-model",
					Temperature: 0.5,
				},
				Reflection: config.StepConfig{
					Name:        "反思",
					ModelName:   "test-model",
					Temperature: 0.3,
				},
				Improvement: config.StepConfig{
					Name:        "改进",
					ModelName:   "test-model",
					Temperature: 0.5,
				},
				FastModeThreshold: 300,
			},
			"raw-step-set": {
				ID:          "raw-step-set",
				Name:        "原始步骤集",
				Description: "使用raw模型的步骤集",
				InitialTranslation: config.StepConfig{
					Name:        "初始翻译",
					ModelName:   "raw",
					Temperature: 0.5,
				},
				Reflection: config.StepConfig{
					Name:        "反思",
					ModelName:   "none",
					Temperature: 0.3,
				},
				Improvement: config.StepConfig{
					Name:        "改进",
					ModelName:   "none",
					Temperature: 0.5,
				},
				FastModeThreshold: 300,
			},
		},
		ModelConfigs: map[string]config.ModelConfig{
			"test-model": {
				Name:            "test-model",
				APIType:         "openai",
				MaxInputTokens:  8000,
				MaxOutputTokens: 2000,
			},
			"raw": {
				Name:            "raw",
				APIType:         "raw",
				MaxInputTokens:  8000,
				MaxOutputTokens: 2000,
			},
		},
	}
}

// 创建一个自定义的ZapLogger
type CustomZapLogger struct {
	logger *zap.Logger
}

func NewCustomZapLogger(zapLogger *zap.Logger) *CustomZapLogger {
	return &CustomZapLogger{
		logger: zapLogger,
	}
}

func (l *CustomZapLogger) Debug(msg string, fields ...zap.Field) {
	l.logger.Debug(msg, fields...)
}

func (l *CustomZapLogger) Info(msg string, fields ...zap.Field) {
	l.logger.Info(msg, fields...)
}

func (l *CustomZapLogger) Warn(msg string, fields ...zap.Field) {
	l.logger.Warn(msg, fields...)
}

func (l *CustomZapLogger) Error(msg string, fields ...zap.Field) {
	l.logger.Error(msg, fields...)
}

func (l *CustomZapLogger) Fatal(msg string, fields ...zap.Field) {
	l.logger.Fatal(msg, fields...)
}

func (l *CustomZapLogger) With(fields ...zap.Field) logger.Logger {
	return NewCustomZapLogger(l.logger.With(fields...))
}

func (l *CustomZapLogger) GetZapLogger() *zap.Logger {
	return l.logger
}

func (l *CustomZapLogger) GetConfig() *config.Config {
	return createTestConfig()
}
