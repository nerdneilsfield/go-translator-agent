package translator

import (
	"sync"

	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"go.uber.org/zap"
)

// RawTranslator 是一个简单的翻译器，它只返回原始文本
type RawTranslator struct {
	config            *config.Config
	logger            *zap.Logger
	progressBar       *progress.Writer
	translatedTracker *progress.Tracker

	progressTracker *TranslationProgressTracker

	lock *sync.RWMutex
}

// NewRawTranslator 创建一个新的原始文本翻译器
func NewRawTranslator(cfg *config.Config, logger *zap.Logger, progressBar *progress.Writer) *RawTranslator {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	return &RawTranslator{
		config:          cfg,
		logger:          logger,
		progressBar:     progressBar,
		lock:            &sync.RWMutex{},
		progressTracker: NewTranslationProgressTracker(0, logger, cfg.TargetCurrency, cfg.UsdRmbRate),
	}
}

func (t *RawTranslator) InitTranslator() {
	t.lock.Lock()
	defer t.lock.Unlock()

	totalChars, _, _, _, _, _, _ := t.progressTracker.GetProgress()

	t.translatedTracker = &progress.Tracker{
		Message: "翻译字数",
		Total:   int64(totalChars),
		Units:   progress.UnitsBytes,
	}
	(*t.progressBar).AppendTracker(t.translatedTracker)
}

// Translate 实现 Translator 接口，直接返回输入的文本
func (t *RawTranslator) Translate(text string, _ bool) (string, error) {
	return text, nil
}

// GetConfig 返回配置
func (t *RawTranslator) GetConfig() *config.Config {
	return t.config
}

// GetLogger 返回日志器
func (t *RawTranslator) GetLogger() interface{} {
	return t.logger
}

// Close 实现 Translator 接口的关闭方法
func (t *RawTranslator) Close() error {
	return nil
}

// RawClient 是一个简单的客户端，直接返回输入文本
type RawClient struct {
	modelName       string
	modelType       string
	maxInputTokens  int
	maxOutputTokens int
}

// NewRawClient 创建一个新的原始文本客户端
func NewRawClient() *RawClient {
	return &RawClient{
		modelName:       "raw",
		modelType:       "raw",
		maxInputTokens:  100000, // 设置一个较大的值
		maxOutputTokens: 100000, // 设置一个较大的值
	}
}

// Complete 直接返回输入的提示词
func (c *RawClient) Complete(prompt string, _ int, _ float64) (string, int, int, error) {
	// 直接返回输入文本，令牌数设为文本长度
	tokenCount := len(prompt)
	// For RawClient, input and output tokens can be considered the same as prompt length or 0.
	return prompt, tokenCount, tokenCount, nil
}

// Name 返回模型名称
func (c *RawClient) Name() string {
	return c.modelName
}

// Type 返回模型类型
func (c *RawClient) Type() string {
	return c.modelType
}

// MaxInputTokens 返回模型支持的最大输入令牌数
func (c *RawClient) MaxInputTokens() int {
	return c.maxInputTokens
}

// MaxOutputTokens 返回模型支持的最大输出令牌数
func (c *RawClient) MaxOutputTokens() int {
	return c.maxOutputTokens
}

// GetInputTokenPrice 返回输入令牌价格
func (c *RawClient) GetInputTokenPrice() float64 {
	return 0
}

// GetOutputTokenPrice 返回输出令牌价格
func (c *RawClient) GetOutputTokenPrice() float64 {
	return 0
}

// GetPriceUnit 返回价格单位
func (c *RawClient) GetPriceUnit() string {
	return "None"
}
