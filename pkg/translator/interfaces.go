package translator

import (
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
)

// Translator 提供从源语言到目标语言的翻译方法
type Translator interface {
	// Translate 将文本从源语言翻译到目标语言
	Translate(text string, retryFailedParts bool) (string, error)

	// GetLogger 返回与翻译器关联的日志记录器
	GetLogger() logger.Logger

	// GetProgressTracker 返回翻译进度跟踪器
	GetProgressTracker() *TranslationProgressTracker

	// GetConfig 返回翻译器配置
	GetConfig() *config.Config

	// GetProgress 返回当前翻译进度
	GetProgress() string

	// InitTranslator 初始化翻译器
	InitTranslator()

	// Finish 结束翻译
	Finish()
}

// LLMClient 语言模型客户端接口
type LLMClient interface {
	// Complete 从提示词生成文本
	Complete(prompt string, maxTokens int, temperature float64) (string, int, int, error)

	// Name 返回模型名称
	Name() string

	// Type 返回模型类型
	Type() string

	// MaxInputTokens 返回模型支持的最大输入令牌数
	MaxInputTokens() int

	// MaxOutputTokens 返回模型支持的最大输出令牌数
	MaxOutputTokens() int

	// GetInputTokenPrice 返回输入令牌价格
	GetInputTokenPrice() float64

	// GetOutputTokenPrice 返回输出令牌价格
	GetOutputTokenPrice() float64

	// GetPriceUnit 返回价格单位
	GetPriceUnit() string
}

// Cache 缓存接口
type Cache interface {
	// Get 从缓存中获取值
	Get(key string) (string, bool)

	// Set 将值存储到缓存中
	Set(key string, value string) error

	// Clear 清除缓存
	Clear() error
}
