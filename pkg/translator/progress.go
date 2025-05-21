package translator

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
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
	// 速度样本
	recentSpeedSamples []float64
	// 最大速度样本数
	maxSpeedSamples int
	// 上次更新的字符数
	lastProgressUpdateChars int
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
	logger     *zap.Logger
	// 目标货币单位，例如 "USD", "RMB"
	TargetCurrency string
	// USD 到 RMB 的汇率
	UsdRmbRate float64
}

type TokenUsage struct {
	InitialInputTokens      int
	InitialOutputTokens     int
	ReflectionInputTokens   int
	ReflectionOutputTokens  int
	ImprovementInputTokens  int
	ImprovementOutputTokens int
	InitialTokenSpeed       float64
	ReflectionTokenSpeed    float64
	ImprovementTokenSpeed   float64
	ElapsedTime             time.Duration
}

type EstimatedCost struct {
	InitialInputCost       float64
	InitialOutputCost      float64
	InitialTotalCost       float64 // Potentially converted cost
	InitialCostUnit        string  // Potentially converted unit
	ReflectionInputCost    float64
	ReflectionOutputCost   float64
	ReflectionTotalCost    float64 // Potentially converted cost
	ReflectionCostUnit     string  // Potentially converted unit
	ImprovementInputCost   float64
	ImprovementOutputCost  float64
	ImprovementTotalCost   float64 // Potentially converted cost
	ImprovementCostUnit    string  // Potentially converted unit
	TotalCost              float64 // Potentially converted total cost
	TotalCostUnit          string  // Potentially converted total unit
	CostCalculationDetails string
}

// NewTranslationProgressTracker 创建一个新的进度跟踪器
func NewTranslationProgressTracker(totalCharsForProgressBar int, logger *zap.Logger, targetCurrency string, usdRmbRate float64) *TranslationProgressTracker {
	log := logger
	if log == nil {
		log = zap.NewNop()
	}
	log.Debug("Creating new TranslationProgressTracker",
		zap.Int("totalCharsForProgressBar", totalCharsForProgressBar),
		zap.String("targetCurrency", targetCurrency),
		zap.Float64("usdRmbRate", usdRmbRate))
	return &TranslationProgressTracker{
		totalChars:     totalCharsForProgressBar,
		realTotalChars: totalCharsForProgressBar,
		startTime:      time.Now(),
		logger:         log,
		modelPrice:     ModelPrice{},
		TargetCurrency: targetCurrency,
		UsdRmbRate:     usdRmbRate,
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
	tp.logger.Debug("Updating model price", zap.Any("current_modelPrice", tp.modelPrice), zap.Any("new_modelPrice", modelPrice))
	tp.logger.Debug("Model price updated", zap.Any("modelPrice", tp.modelPrice))
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
	tp.logger.Debug("Updating real translated chars (final output file)", zap.Int("current_realTranslatedChars", tp.realTranslatedChars), zap.Int("chars_to_set", chars))
	tp.logger.Debug("Real translated chars updated", zap.Int("realTranslatedChars", tp.realTranslatedChars))
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
	tp.logger.Debug("Updating initial token usage",
		zap.Int("current_initialInputTokens", tp.usedInitialInputTokens),
		zap.Int("current_initialOutputTokens", tp.usedInitialOutputTokens),
		zap.Int("inputTokens_to_add", inputTokens),
		zap.Int("outputTokens_to_add", outputTokens),
	)
	tp.logger.Debug("Initial token usage updated",
		zap.Int("initialInputTokens", tp.usedInitialInputTokens),
		zap.Int("initialOutputTokens", tp.usedInitialOutputTokens),
	)
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
	tp.logger.Debug("Updating reflection token usage",
		zap.Int("current_reflectionInputTokens", tp.usedReflectionInputTokens),
		zap.Int("current_reflectionOutputTokens", tp.usedReflectionOutputTokens),
		zap.Int("inputTokens_to_add", inputTokens),
		zap.Int("outputTokens_to_add", outputTokens),
	)
	tp.logger.Debug("Reflection token usage updated",
		zap.Int("reflectionInputTokens", tp.usedReflectionInputTokens),
		zap.Int("reflectionOutputTokens", tp.usedReflectionOutputTokens),
	)
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
	tp.logger.Debug("Updating improvement token usage",
		zap.Int("current_improvementInputTokens", tp.usedImprovementInputTokens),
		zap.Int("current_improvementOutputTokens", tp.usedImprovementOutputTokens),
		zap.Int("inputTokens_to_add", inputTokens),
		zap.Int("outputTokens_to_add", outputTokens),
	)
	tp.logger.Debug("Improvement token usage updated",
		zap.Int("improvementInputTokens", tp.usedImprovementInputTokens),
		zap.Int("improvementOutputTokens", tp.usedImprovementOutputTokens),
	)
}

// GetProgress 获取当前进度信息
func (tp *TranslationProgressTracker) GetProgress() (totalChars int, translatedChars int, realTotalChars int, realTranslatedChars int, estimatedTimeRemaining float64, tokenUsage TokenUsage, estimatedCost EstimatedCost) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	elapsedTime := time.Since(tp.startTime)

	tokenUsage = TokenUsage{
		InitialInputTokens:      tp.usedInitialInputTokens,
		InitialOutputTokens:     tp.usedInitialOutputTokens,
		ReflectionInputTokens:   tp.usedReflectionInputTokens,
		ReflectionOutputTokens:  tp.usedReflectionOutputTokens,
		ImprovementInputTokens:  tp.usedImprovementInputTokens,
		ImprovementOutputTokens: tp.usedImprovementOutputTokens,
		ElapsedTime:             elapsedTime,
	}

	// Calculate token speeds (tokens per second)
	elapsedSeconds := elapsedTime.Seconds()
	if elapsedSeconds > 0 {
		if tp.usedInitialOutputTokens > 0 || tp.usedInitialInputTokens > 0 { // Avoid division by zero if no tokens
			tokenUsage.InitialTokenSpeed = float64(tp.usedInitialInputTokens+tp.usedInitialOutputTokens) / elapsedSeconds
		}
		if tp.usedReflectionOutputTokens > 0 || tp.usedReflectionInputTokens > 0 {
			tokenUsage.ReflectionTokenSpeed = float64(tp.usedReflectionInputTokens+tp.usedReflectionOutputTokens) / elapsedSeconds
		}
		if tp.usedImprovementOutputTokens > 0 || tp.usedImprovementInputTokens > 0 {
			tokenUsage.ImprovementTokenSpeed = float64(tp.usedImprovementInputTokens+tp.usedImprovementOutputTokens) / elapsedSeconds
		}
	}

	modelPrice := tp.modelPrice
	var detailsBuilder strings.Builder
	targetCurrency := tp.TargetCurrency
	usdRmbRate := tp.UsdRmbRate

	// Helper function for cost conversion
	convert := func(cost float64, fromUnit string) (convertedCost float64, newUnit string, detail string) {
		if targetCurrency == "" || fromUnit == "" || fromUnit == targetCurrency || usdRmbRate <= 0 {
			return cost, fromUnit, "" // No conversion
		}
		if fromUnit == "USD" && targetCurrency == "RMB" {
			converted := cost * usdRmbRate
			return converted, "RMB", fmt.Sprintf(" (%.6f USD * %.4f = %.6f RMB)", cost, usdRmbRate, converted)
		}
		if fromUnit == "RMB" && targetCurrency == "USD" {
			converted := cost / usdRmbRate
			return converted, "USD", fmt.Sprintf(" (%.6f RMB / %.4f = %.6f USD)", cost, usdRmbRate, converted)
		}
		return cost, fromUnit, fmt.Sprintf(" (cannot convert %s to %s)", fromUnit, targetCurrency) // Cannot convert
	}

	// Calculate costs (prices are per 1,000,000 tokens)
	originalInitialInputCost := modelPrice.InitialModelInputPrice * float64(tp.usedInitialInputTokens) / 1000000.0
	originalInitialOutputCost := modelPrice.InitialModelOutputPrice * float64(tp.usedInitialOutputTokens) / 1000000.0
	originalInitialTotalCost := originalInitialInputCost + originalInitialOutputCost
	currentInitialCost, currentInitialUnit, convDetailInitial := convert(originalInitialTotalCost, modelPrice.InitialModelPriceUnit)

	originalReflectionInputCost := modelPrice.ReflectionModelInputPrice * float64(tp.usedReflectionInputTokens) / 1000000.0
	originalReflectionOutputCost := modelPrice.ReflectionModelOutputPrice * float64(tp.usedReflectionOutputTokens) / 1000000.0
	originalReflectionTotalCost := originalReflectionInputCost + originalReflectionOutputCost
	currentReflectionCost, currentReflectionUnit, convDetailReflection := convert(originalReflectionTotalCost, modelPrice.ReflectionModelPriceUnit)

	originalImprovementInputCost := modelPrice.ImprovementModelInputPrice * float64(tp.usedImprovementInputTokens) / 1000000.0
	originalImprovementOutputCost := modelPrice.ImprovementModelOutputPrice * float64(tp.usedImprovementOutputTokens) / 1000000.0
	originalImprovementTotalCost := originalImprovementInputCost + originalImprovementOutputCost
	currentImprovementCost, currentImprovementUnit, convDetailImprovement := convert(originalImprovementTotalCost, modelPrice.ImprovementModelPriceUnit)

	finalTotalCost := currentInitialCost + currentReflectionCost + currentImprovementCost
	finalTotalCostUnit := ""

	if currentInitialCost > 0 || (targetCurrency != "" && modelPrice.InitialModelPriceUnit != "" && modelPrice.InitialModelPriceUnit != targetCurrency) {
		detailsBuilder.WriteString(fmt.Sprintf("Initial: (In: %d * %.6f %s, Out: %d * %.6f %s) / 1M = %.6f %s%s. ",
			tp.usedInitialInputTokens, modelPrice.InitialModelInputPrice, modelPrice.InitialModelPriceUnit,
			tp.usedInitialOutputTokens, modelPrice.InitialModelOutputPrice, modelPrice.InitialModelPriceUnit,
			currentInitialCost, currentInitialUnit, convDetailInitial))
	}

	if currentReflectionCost > 0 || (targetCurrency != "" && modelPrice.ReflectionModelPriceUnit != "" && modelPrice.ReflectionModelPriceUnit != targetCurrency) {
		detailsBuilder.WriteString(fmt.Sprintf("Reflection: (In: %d * %.6f %s, Out: %d * %.6f %s) / 1M = %.6f %s%s. ",
			tp.usedReflectionInputTokens, modelPrice.ReflectionModelInputPrice, modelPrice.ReflectionModelPriceUnit,
			tp.usedReflectionOutputTokens, modelPrice.ReflectionModelOutputPrice, modelPrice.ReflectionModelPriceUnit,
			currentReflectionCost, currentReflectionUnit, convDetailReflection))
	}

	if currentImprovementCost > 0 || (targetCurrency != "" && modelPrice.ImprovementModelPriceUnit != "" && modelPrice.ImprovementModelPriceUnit != targetCurrency) {
		detailsBuilder.WriteString(fmt.Sprintf("Improvement: (In: %d * %.6f %s, Out: %d * %.6f %s) / 1M = %.6f %s%s. ",
			tp.usedImprovementInputTokens, modelPrice.ImprovementModelInputPrice, modelPrice.ImprovementModelPriceUnit,
			tp.usedImprovementOutputTokens, modelPrice.ImprovementModelOutputPrice, modelPrice.ImprovementModelPriceUnit,
			currentImprovementCost, currentImprovementUnit, convDetailImprovement))
	}

	if finalTotalCost > 0 {
		var activePhaseUnits []string
		if currentInitialCost > 0 && currentInitialUnit != "" {
			activePhaseUnits = append(activePhaseUnits, currentInitialUnit)
		}
		if currentReflectionCost > 0 && currentReflectionUnit != "" {
			activePhaseUnits = append(activePhaseUnits, currentReflectionUnit)
		}
		if currentImprovementCost > 0 && currentImprovementUnit != "" {
			activePhaseUnits = append(activePhaseUnits, currentImprovementUnit)
		}

		if targetCurrency != "" {
			if finalTotalCost == 0 {
				finalTotalCostUnit = targetCurrency // Or "" if preferred for zero cost in target currency
			} else {
				allMatchTarget := true
				for _, u := range activePhaseUnits {
					if u != targetCurrency {
						allMatchTarget = false
						break
					}
				}
				if allMatchTarget {
					finalTotalCostUnit = targetCurrency
				} else {
					finalTotalCostUnit = "无法统一到目标货币" // Some costs couldn't be converted or were not in target
				}
			}
		} else { // No target currency, use previous logic
			if finalTotalCost == 0 {
				finalTotalCostUnit = ""
			} else if len(activePhaseUnits) > 0 {
				firstUnit := activePhaseUnits[0]
				allSame := true
				for _, unit := range activePhaseUnits[1:] {
					if unit != firstUnit {
						allSame = false
						break
					}
				}
				if allSame {
					finalTotalCostUnit = firstUnit
				} else {
					finalTotalCostUnit = "模型价格单位不一致"
				}
			} else { // No active costs with units
				finalTotalCostUnit = ""
			}
		}
	} // else, if finalTotalCost is 0, finalTotalCostUnit remains ""

	estimatedCost = EstimatedCost{
		InitialInputCost:       originalInitialInputCost,  // Store original input for potential reference
		InitialOutputCost:      originalInitialOutputCost, // Store original output for potential reference
		InitialTotalCost:       currentInitialCost,        // This is the (potentially) converted cost
		InitialCostUnit:        currentInitialUnit,        // This is the (potentially) converted unit
		ReflectionInputCost:    originalReflectionInputCost,
		ReflectionOutputCost:   originalReflectionOutputCost,
		ReflectionTotalCost:    currentReflectionCost,
		ReflectionCostUnit:     currentReflectionUnit,
		ImprovementInputCost:   originalImprovementInputCost,
		ImprovementOutputCost:  originalImprovementOutputCost,
		ImprovementTotalCost:   currentImprovementCost,
		ImprovementCostUnit:    currentImprovementUnit,
		TotalCost:              finalTotalCost,
		TotalCostUnit:          finalTotalCostUnit,
		CostCalculationDetails: strings.TrimSpace(detailsBuilder.String()),
	}

	tp.logger.Debug("Getting progress",
		zap.Int("totalCharsForProgressBar", tp.totalChars),
		zap.Int("cumulativeTranslatedChars", tp.translatedChars),
		zap.Int("realTotalChars", tp.realTotalChars),
		zap.Int("realTranslatedChars", tp.realTranslatedChars),
		zap.Any("tokenUsage", tokenUsage),
		zap.Any("estimatedCost", estimatedCost),
	)

	return tp.totalChars, tp.translatedChars, tp.realTotalChars, tp.realTranslatedChars, tp.estimatedTimeRemaining, tokenUsage, estimatedCost
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
	tp.logger.Debug("Setting total chars for progress bar", zap.Int("current_totalChars", tp.totalChars), zap.Int("new_totalChars", totalChars))
	tp.logger.Debug("Total chars for progress bar set", zap.Int("totalChars", tp.totalChars))
}

func (tp *TranslationProgressTracker) SetRealTotalChars(realTotalChars int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.realTotalChars = realTotalChars
	tp.logger.Debug("Setting real total chars (original file)", zap.Int("current_realTotalChars", tp.realTotalChars), zap.Int("new_realTotalChars", realTotalChars))
	tp.logger.Debug("Real total chars set", zap.Int("realTotalChars", tp.realTotalChars))
}

// Reset 重置进度跟踪器
func (tp *TranslationProgressTracker) Reset() {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.logger.Debug("Resetting TranslationProgressTracker")
	tp.translatedChars = 0
	tp.startTime = time.Now()
	tp.lastUpdateTime = time.Now()
	tp.estimatedTimeRemaining = 0
	tp.translationSpeed = 0
	tp.recentSpeedSamples = make([]float64, 0, tp.maxSpeedSamples)
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
	tp.lastProgressUpdateChars = 0
	tp.modelPrice = ModelPrice{}
	tp.logger.Debug("TranslationProgressTracker has been reset")
}
