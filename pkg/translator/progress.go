package translator

import (
	"sync"
	"time"
)

// TranslationProgressTracker 用于跟踪翻译进度
type TranslationProgressTracker struct {
	mu sync.Mutex
	// 总字数
	totalChars int
	// 实际总字数
	realTotalChars int
	// 翻译字数
	translatedChars int
	// 已翻译字数
	realTranslatedChars int
	// 开始时间
	startTime time.Time
	// 最后更新时间
	lastUpdateTime time.Time
	// 预计剩余时间（秒）
	estimatedTimeRemaining float64
	// 翻译速度（字/秒）
	translationSpeed float64
	// 使用的 initial input token 数量
	usedInitialInputTokens int
	// 使用的 initial output token 数量
	usedInitialOutputTokens int
	// 使用的 reflection input token 数量
	usedReflectionInputTokens int
	// 使用的 reflection output token 数量
	usedReflectionOutputTokens int
	// 使用的 improvement input token 数量
	usedImprovementInputTokens int
	// 使用的 improvement output token 数量
	usedImprovementOutputTokens int
	// 生成的 initial token 速度（token/s）
	generatedInitialTokenSpeed float64
	// 生成的 reflection token 速度（token/s）
	generatedReflectionTokenSpeed float64
	// 生成的 improvement token 速度（token/s）
	generatedImprovementTokenSpeed float64
	// 模型价格
	modelPrice ModelPrice
}

type TokenUsage struct {
	initialInputTokens      int
	initialOutputTokens     int
	reflectionInputTokens   int
	reflectionOutputTokens  int
	improvementInputTokens  int
	improvementOutputTokens int
	initialTokenSpeed       float64
	reflectionTokenSpeed    float64
	improvementTokenSpeed   float64
	elapsedTime             time.Duration
}

type EstimatedCost struct {
	initialInputCost      float64
	initialOutputCost     float64
	initialTotalCost      float64
	initialCostUnit       string
	reflectionInputCost   float64
	reflectionOutputCost  float64
	reflectionTotalCost   float64
	reflectionCostUnit    string
	improvementInputCost  float64
	improvementOutputCost float64
	improvementTotalCost  float64
	improvementCostUnit   string
	totalCost             float64
	totalCostUnit         string
}

// NewProgressTracker 创建一个新的进度跟踪器
func NewProgressTracker(totalChars int) *TranslationProgressTracker {
	now := time.Now()
	return &TranslationProgressTracker{
		totalChars:                     totalChars,
		startTime:                      now,
		lastUpdateTime:                 now,
		usedInitialInputTokens:         0,
		usedInitialOutputTokens:        0,
		usedReflectionInputTokens:      0,
		usedReflectionOutputTokens:     0,
		usedImprovementInputTokens:     0,
		usedImprovementOutputTokens:    0,
		generatedInitialTokenSpeed:     0,
		generatedReflectionTokenSpeed:  0,
		generatedImprovementTokenSpeed: 0,
		estimatedTimeRemaining:         0,
		translationSpeed:               0,
		realTranslatedChars:            0,
	}
}

// UpdateProgress 更新翻译进度
func (tp *TranslationProgressTracker) UpdateProgress(chars int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// 更新已翻译字数
	tp.translatedChars += chars

	// 更新时间
	now := time.Now()
	elapsed := now.Sub(tp.lastUpdateTime).Seconds()

	// 计算翻译速度（使用移动平均）
	if elapsed > 0 {
		instantSpeed := float64(chars) / elapsed
		if tp.translationSpeed == 0 {
			tp.translationSpeed = instantSpeed
		} else {
			// 使用指数移动平均，alpha = 0.3
			tp.translationSpeed = 0.3*instantSpeed + 0.7*tp.translationSpeed
		}
	}

	// 计算预计剩余时间
	if tp.translationSpeed > 0 {
		remainingChars := tp.totalChars - tp.translatedChars
		tp.estimatedTimeRemaining = float64(remainingChars) / tp.translationSpeed
	}

	// 更新最后更新时间
	tp.lastUpdateTime = now
}

func (tp *TranslationProgressTracker) UpdateModelPrice(modelPrice ModelPrice) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.modelPrice = modelPrice
}

func (tp *TranslationProgressTracker) GetModelPrice() ModelPrice {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.modelPrice
}

func (tp *TranslationProgressTracker) UpdateRealTranslatedChars(chars int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.realTranslatedChars += chars
}

// UpdateTokenUsage 更新使用的 token 数量
func (tp *TranslationProgressTracker) UpdateInitialTokenUsage(inputTokens int, outputTokens int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.usedInitialInputTokens += inputTokens
	tp.usedInitialOutputTokens += outputTokens

	// 计算生成的 token 速度
	now := time.Now()
	elapsed := now.Sub(tp.lastUpdateTime).Seconds()
	if elapsed > 0 {
		instantSpeed := float64(outputTokens) / elapsed
		if tp.generatedInitialTokenSpeed == 0 {
			tp.generatedInitialTokenSpeed = instantSpeed
		} else {
			// 使用指数移动平均，alpha = 0.3
			tp.generatedInitialTokenSpeed = 0.3*instantSpeed + 0.7*tp.generatedInitialTokenSpeed
		}
	}
}

func (tp *TranslationProgressTracker) UpdateReflectionTokenUsage(inputTokens int, outputTokens int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.usedReflectionInputTokens += inputTokens
	tp.usedReflectionOutputTokens += outputTokens

	// 计算生成的 token 速度
	now := time.Now()
	elapsed := now.Sub(tp.lastUpdateTime).Seconds()
	if elapsed > 0 {
		instantSpeed := float64(outputTokens) / elapsed
		if tp.generatedReflectionTokenSpeed == 0 {
			tp.generatedReflectionTokenSpeed = instantSpeed
		} else {
			// 使用指数移动平均，alpha = 0.3
			tp.generatedReflectionTokenSpeed = 0.3*instantSpeed + 0.7*tp.generatedReflectionTokenSpeed
		}
	}
}

func (tp *TranslationProgressTracker) UpdateImprovementTokenUsage(inputTokens int, outputTokens int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.usedImprovementInputTokens += inputTokens
	tp.usedImprovementOutputTokens += outputTokens

	// 计算生成的 token 速度
	now := time.Now()
	elapsed := now.Sub(tp.lastUpdateTime).Seconds()
	if elapsed > 0 {
		instantSpeed := float64(outputTokens) / elapsed
		if tp.generatedImprovementTokenSpeed == 0 {
			tp.generatedImprovementTokenSpeed = instantSpeed
		} else {
			// 使用指数移动平均，alpha = 0.3
			tp.generatedImprovementTokenSpeed = 0.3*instantSpeed + 0.7*tp.generatedImprovementTokenSpeed
		}
	}
}

// GetProgress 获取当前进度信息
func (tp *TranslationProgressTracker) GetProgress() (totalChars int, translatedChars int, realTotalChars int, estimatedTimeRemaining float64, tokenUsage TokenUsage, estimatedCost EstimatedCost) {
	tp.mu.Lock()
	// 先复制所有需要的数据
	modelPrice := tp.modelPrice // 直接使用字段，不调用 GetModelPrice
	usedInitialInput := tp.usedInitialInputTokens
	usedInitialOutput := tp.usedInitialOutputTokens
	usedReflectionInput := tp.usedReflectionInputTokens
	usedReflectionOutput := tp.usedReflectionOutputTokens
	usedImprovementInput := tp.usedImprovementInputTokens
	usedImprovementOutput := tp.usedImprovementOutputTokens
	total := tp.totalChars
	translated := tp.translatedChars
	realTotal := tp.realTotalChars
	timeRemaining := tp.estimatedTimeRemaining
	startTime := tp.startTime
	speeds := TokenUsage{
		initialTokenSpeed:     tp.generatedInitialTokenSpeed,
		reflectionTokenSpeed:  tp.generatedReflectionTokenSpeed,
		improvementTokenSpeed: tp.generatedImprovementTokenSpeed,
	}
	tp.mu.Unlock()

	// 在锁外进行计算
	initialInputCost := modelPrice.InitialModelInputPrice * float64(usedInitialInput) / 1000000
	initialOutputCost := modelPrice.InitialModelOutputPrice * float64(usedInitialOutput) / 1000000
	initialTotalCost := initialInputCost + initialOutputCost

	reflectionInputCost := modelPrice.ReflectionModelInputPrice * float64(usedReflectionInput) / 1000000
	reflectionOutputCost := modelPrice.ReflectionModelOutputPrice * float64(usedReflectionOutput) / 1000000
	reflectionTotalCost := reflectionInputCost + reflectionOutputCost

	improvementInputCost := modelPrice.ImprovementModelInputPrice * float64(usedImprovementInput) / 1000000
	improvementOutputCost := modelPrice.ImprovementModelOutputPrice * float64(usedImprovementOutput) / 1000000
	improvementTotalCost := improvementInputCost + improvementOutputCost

	totalCost := initialTotalCost + reflectionTotalCost + improvementTotalCost
	totalCostUnit := modelPrice.InitialModelPriceUnit

	if modelPrice.InitialModelPriceUnit != modelPrice.ReflectionModelPriceUnit ||
		modelPrice.InitialModelPriceUnit != modelPrice.ImprovementModelPriceUnit {
		totalCost = 0
		totalCostUnit = "模型价格单位不一致"
	}

	return total, translated, realTotal, timeRemaining, TokenUsage{
			initialInputTokens:      usedInitialInput,
			initialOutputTokens:     usedInitialOutput,
			reflectionInputTokens:   usedReflectionInput,
			reflectionOutputTokens:  usedReflectionOutput,
			improvementInputTokens:  usedImprovementInput,
			improvementOutputTokens: usedImprovementOutput,
			initialTokenSpeed:       speeds.initialTokenSpeed,
			reflectionTokenSpeed:    speeds.reflectionTokenSpeed,
			improvementTokenSpeed:   speeds.improvementTokenSpeed,
			elapsedTime:             time.Since(startTime),
		}, EstimatedCost{
			initialInputCost:      initialInputCost,
			initialOutputCost:     initialOutputCost,
			initialTotalCost:      initialTotalCost,
			initialCostUnit:       modelPrice.InitialModelPriceUnit,
			reflectionInputCost:   reflectionInputCost,
			reflectionOutputCost:  reflectionOutputCost,
			reflectionTotalCost:   reflectionTotalCost,
			reflectionCostUnit:    modelPrice.ReflectionModelPriceUnit,
			improvementInputCost:  improvementInputCost,
			improvementOutputCost: improvementOutputCost,
			improvementTotalCost:  improvementTotalCost,
			improvementCostUnit:   modelPrice.ImprovementModelPriceUnit,
			totalCost:             totalCost,
			totalCostUnit:         totalCostUnit,
		}
}

// GetTranslationSpeed 获取当前翻译速度（字/秒）
func (tp *TranslationProgressTracker) GetTranslationSpeed() float64 {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.translationSpeed
}

// GetGeneratedTokenSpeed 获取当前生成的 token 速度（token/秒）
func (tp *TranslationProgressTracker) GetInitialGeneratedTokenSpeed() float64 {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.generatedInitialTokenSpeed
}

func (tp *TranslationProgressTracker) GetReflectionGeneratedTokenSpeed() float64 {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.generatedReflectionTokenSpeed
}

func (tp *TranslationProgressTracker) GetImprovementGeneratedTokenSpeed() float64 {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.generatedImprovementTokenSpeed
}

// GetCompletionPercentage 获取完成百分比
func (tp *TranslationProgressTracker) GetCompletionPercentage() float64 {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	if tp.totalChars == 0 {
		return 0
	}
	return float64(tp.translatedChars) / float64(tp.totalChars) * 100
}

// SetTotalChars 设置总字数
func (tp *TranslationProgressTracker) SetTotalChars(totalChars int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.totalChars = totalChars
}

func (tp *TranslationProgressTracker) SetRealTotalChars(realTotalChars int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.realTotalChars = realTotalChars
}

// Reset 重置进度跟踪器
func (tp *TranslationProgressTracker) Reset() {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	now := time.Now()
	tp.translatedChars = 0
	tp.startTime = now
	tp.lastUpdateTime = now
	tp.estimatedTimeRemaining = 0
	tp.translationSpeed = 0
	tp.generatedInitialTokenSpeed = 0
	tp.generatedReflectionTokenSpeed = 0
	tp.generatedImprovementTokenSpeed = 0
	tp.usedInitialInputTokens = 0
	tp.usedInitialOutputTokens = 0
	tp.usedReflectionInputTokens = 0
	tp.usedReflectionOutputTokens = 0
	tp.usedImprovementInputTokens = 0
	tp.usedImprovementOutputTokens = 0
	tp.realTranslatedChars = 0
}
