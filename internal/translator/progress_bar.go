package translator

import (
	"fmt"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

// ProgressBar 翻译进度条
type ProgressBar struct {
	bar            *progressbar.ProgressBar
	totalChars     int64
	processedChars int64
	startTime      time.Time
	mu             sync.Mutex
}

// NewProgressBar 创建新的进度条
func NewProgressBar(totalChars int64, description string) *ProgressBar {
	bar := progressbar.NewOptions64(
		totalChars,
		progressbar.OptionSetDescription(description),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(50),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	return &ProgressBar{
		bar:        bar,
		totalChars: totalChars,
		startTime:  time.Now(),
	}
}

// Update 更新进度
func (pb *ProgressBar) Update(chars int64) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	pb.processedChars += chars
	pb.bar.Add64(chars)
}

// SetDescription 更新描述
func (pb *ProgressBar) SetDescription(desc string) {
	pb.bar.Describe(desc)
}

// Finish 完成进度条
func (pb *ProgressBar) Finish() {
	pb.bar.Finish()

	// 显示完成统计
	duration := time.Since(pb.startTime)
	speed := float64(pb.processedChars) / duration.Seconds()

	fmt.Printf("\n✓ 翻译完成: %d 字符, 耗时: %s, 速度: %.0f 字符/秒\n",
		pb.processedChars, duration.Round(time.Second), speed)
}

// SimpleProgressBar 简单的文本进度显示（备用）
type SimpleProgressBar struct {
	totalNodes     int
	completedNodes int
	lastUpdate     time.Time
	mu             sync.Mutex
}

// NewSimpleProgressBar 创建简单进度条
func NewSimpleProgressBar(totalNodes int) *SimpleProgressBar {
	return &SimpleProgressBar{
		totalNodes: totalNodes,
		lastUpdate: time.Now(),
	}
}

// UpdateNode 更新节点进度
func (spb *SimpleProgressBar) UpdateNode() {
	spb.mu.Lock()
	defer spb.mu.Unlock()

	spb.completedNodes++

	// 每秒最多更新一次显示
	if time.Since(spb.lastUpdate) < time.Second && spb.completedNodes < spb.totalNodes {
		return
	}

	percentage := float64(spb.completedNodes) * 100 / float64(spb.totalNodes)
	fmt.Printf("\r翻译进度: [%d/%d] %.1f%%", spb.completedNodes, spb.totalNodes, percentage)

	spb.lastUpdate = time.Now()
}

// Finish 完成
func (spb *SimpleProgressBar) Finish() {
	fmt.Printf("\r翻译进度: [%d/%d] 100.0%%\n", spb.totalNodes, spb.totalNodes)
}
