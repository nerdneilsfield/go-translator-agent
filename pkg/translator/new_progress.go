package translator

import (
	"sync"
	"time"

	customprogress "github.com/nerdneilsfield/go-translator-agent/pkg/progress"
	"go.uber.org/zap"
)

// NewProgressTracker 是新的进度跟踪器，用于替代旧的 TranslationProgressTracker
type NewProgressTracker struct {
	mu sync.Mutex

	// 进度适配器
	adapter *customprogress.Adapter

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

	// 使用的 token 数量
	usedInitialInputTokens      int
	usedInitialOutputTokens     int
	usedReflectionInputTokens   int
	usedReflectionOutputTokens  int
	usedImprovementInputTokens  int
	usedImprovementOutputTokens int

	// 模型价格
	modelPrice ModelPrice
}

// NewNewProgressTracker 创建一个新的进度跟踪器
func NewNewProgressTracker(totalChars int, logger *zap.Logger) *NewProgressTracker {
	now := time.Now()

	// 创建进度适配器
	adapter := customprogress.NewAdapter(logger)

	return &NewProgressTracker{
		adapter:                     adapter,
		totalChars:                  totalChars,
		startTime:                   now,
		lastUpdateTime:              now,
		usedInitialInputTokens:      0,
		usedInitialOutputTokens:     0,
		usedReflectionInputTokens:   0,
		usedReflectionOutputTokens:  0,
		usedImprovementInputTokens:  0,
		usedImprovementOutputTokens: 0,
		realTranslatedChars:         0,
	}
}

// Start 开始进度跟踪
func (tp *NewProgressTracker) Start() {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.adapter.Start()
}

// UpdateProgress 更新翻译进度
func (tp *NewProgressTracker) UpdateProgress(chars int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// 更新已翻译字数
	tp.translatedChars += chars

	// 更新时间
	tp.lastUpdateTime = time.Now()

	// 更新进度适配器
	tp.adapter.Update(int64(tp.translatedChars))
}

// UpdateModelPrice 更新模型价格
func (tp *NewProgressTracker) UpdateModelPrice(modelPrice ModelPrice) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.modelPrice = modelPrice
}

// GetModelPrice 获取模型价格
func (tp *NewProgressTracker) GetModelPrice() ModelPrice {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	return tp.modelPrice
}

// UpdateRealTranslatedChars 更新实际已翻译字数
func (tp *NewProgressTracker) UpdateRealTranslatedChars(chars int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.realTranslatedChars += chars
}

// UpdateInitialTokenUsage 更新初始 token 使用情况
func (tp *NewProgressTracker) UpdateInitialTokenUsage(inputTokens int, outputTokens int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.usedInitialInputTokens += inputTokens
	tp.usedInitialOutputTokens += outputTokens
}

// UpdateReflectionTokenUsage 更新反思 token 使用情况
func (tp *NewProgressTracker) UpdateReflectionTokenUsage(inputTokens int, outputTokens int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.usedReflectionInputTokens += inputTokens
	tp.usedReflectionOutputTokens += outputTokens
}

// UpdateImprovementTokenUsage 更新改进 token 使用情况
func (tp *NewProgressTracker) UpdateImprovementTokenUsage(inputTokens int, outputTokens int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.usedImprovementInputTokens += inputTokens
	tp.usedImprovementOutputTokens += outputTokens
}

// GetProgress 获取当前进度信息
func (tp *NewProgressTracker) GetProgress() (totalChars int, translatedChars int, realTotalChars int, estimatedTimeRemaining float64, tokenUsage TokenUsage, estimatedCost EstimatedCost) {
	tp.mu.Lock()

	// 复制所需数据
	modelPrice := tp.modelPrice
	usedInitialInput := tp.usedInitialInputTokens
	usedInitialOutput := tp.usedInitialOutputTokens
	usedReflectionInput := tp.usedReflectionInputTokens
	usedReflectionOutput := tp.usedReflectionOutputTokens
	usedImprovementInput := tp.usedImprovementInputTokens
	usedImprovementOutput := tp.usedImprovementOutputTokens
	total := tp.totalChars
	translated := tp.translatedChars
	realTotal := tp.realTotalChars
	startTime := tp.startTime

	// 获取 ETA
	eta := tp.adapter.GetTracker().GetETA()
	timeRemaining := eta.Seconds()

	tp.mu.Unlock()

	// 计算成本
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
			InitialInputTokens:      usedInitialInput,
			InitialOutputTokens:     usedInitialOutput,
			ReflectionInputTokens:   usedReflectionInput,
			ReflectionOutputTokens:  usedReflectionOutput,
			ImprovementInputTokens:  usedImprovementInput,
			ImprovementOutputTokens: usedImprovementOutput,
			InitialTokenSpeed:       0, // 不再使用
			ReflectionTokenSpeed:    0, // 不再使用
			ImprovementTokenSpeed:   0, // 不再使用
			ElapsedTime:             time.Since(startTime),
		}, EstimatedCost{
			InitialInputCost:      initialInputCost,
			InitialOutputCost:     initialOutputCost,
			InitialTotalCost:      initialTotalCost,
			InitialCostUnit:       modelPrice.InitialModelPriceUnit,
			ReflectionInputCost:   reflectionInputCost,
			ReflectionOutputCost:  reflectionOutputCost,
			ReflectionTotalCost:   reflectionTotalCost,
			ReflectionCostUnit:    modelPrice.ReflectionModelPriceUnit,
			ImprovementInputCost:  improvementInputCost,
			ImprovementOutputCost: improvementOutputCost,
			ImprovementTotalCost:  improvementTotalCost,
			ImprovementCostUnit:   modelPrice.ImprovementModelPriceUnit,
			TotalCost:             totalCost,
			TotalCostUnit:         totalCostUnit,
		}
}

// GetCompletionPercentage 获取完成百分比
func (tp *NewProgressTracker) GetCompletionPercentage() float64 {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	return tp.adapter.GetTracker().GetPercentage()
}

// SetTotalChars 设置总字数
func (tp *NewProgressTracker) SetTotalChars(totalChars int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.totalChars = totalChars
	tp.adapter.GetTracker().SetTotal(int64(totalChars))
}

// SetRealTotalChars 设置实际总字数
func (tp *NewProgressTracker) SetRealTotalChars(realTotalChars int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.realTotalChars = realTotalChars
}

// Reset 重置进度跟踪器
func (tp *NewProgressTracker) Reset() {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	now := time.Now()
	tp.translatedChars = 0
	tp.startTime = now
	tp.lastUpdateTime = now
	tp.usedInitialInputTokens = 0
	tp.usedInitialOutputTokens = 0
	tp.usedReflectionInputTokens = 0
	tp.usedReflectionOutputTokens = 0
	tp.usedImprovementInputTokens = 0
	tp.usedImprovementOutputTokens = 0
	tp.realTranslatedChars = 0

	// 创建新的进度适配器
	tp.adapter = customprogress.NewAdapter(nil)
}

// GetAdapter 获取进度适配器
func (tp *NewProgressTracker) GetAdapter() *customprogress.Adapter {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	return tp.adapter
}

// GetStartTime 获取开始时间
func (tp *NewProgressTracker) GetStartTime() time.Time {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	return tp.startTime
}

// Stop 停止进度跟踪
func (tp *NewProgressTracker) Stop() {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.adapter.Stop()
}

// Done 标记为已完成
func (tp *NewProgressTracker) Done(summary *customprogress.SummaryStats) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if tp.adapter != nil {
		tp.adapter.Done(summary)
	}
}
