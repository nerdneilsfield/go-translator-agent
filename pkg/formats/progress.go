package formats

import (
	"sync"
	"time"
)

// TranslationProgressTracker 用于跟踪翻译进度
type TranslationProgressTracker struct {
	mu sync.Mutex
	// 总字数
	totalChars int
	// 已翻译字数
	translatedChars int
	// 开始时间
	startTime time.Time
	// 最后更新时间
	lastUpdateTime time.Time
	// 预计剩余时间（秒）
	estimatedTimeRemaining float64
	// 翻译速度（字/秒）
	translationSpeed float64
}

// NewProgressTracker 创建一个新的进度跟踪器
func NewProgressTracker(totalChars int) *TranslationProgressTracker {
	now := time.Now()
	return &TranslationProgressTracker{
		totalChars:     totalChars,
		startTime:      now,
		lastUpdateTime: now,
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

// GetProgress 获取当前进度信息
func (tp *TranslationProgressTracker) GetProgress() (totalChars int, translatedChars int, estimatedTimeRemaining float64) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.totalChars, tp.translatedChars, tp.estimatedTimeRemaining
}

// GetTranslationSpeed 获取当前翻译速度（字/秒）
func (tp *TranslationProgressTracker) GetTranslationSpeed() float64 {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.translationSpeed
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
}
