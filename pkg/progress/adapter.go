package progress

import (
	"fmt"
	"os"
	"time"

	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/jedib0t/go-pretty/v6/text"
)

// ProgressAdapter 是一个适配器，将我们的进度条系统与现有的翻译器集成
type ProgressAdapter struct {
	tracker        *ProgressTracker
	translationBar *progress.Tracker
	costBar        *progress.Tracker
	writer         *progress.Writer
}

// NewProgressAdapter 创建一个新的进度条适配器
func NewProgressAdapter(totalUnits int64) *ProgressAdapter {
	// 创建进度条写入器
	pw := progress.NewWriter()

	// 配置进度条样式
	pw.SetStyle(progress.StyleDefault)
	pw.Style().Colors = progress.StyleColorsExample
	pw.Style().Options.PercentFormat = "%4.1f%%"

	// 设置更新频率为1秒
	pw.SetUpdateFrequency(time.Second)

	// 设置自动停止为false，确保进度条持续显示
	pw.SetAutoStop(false)

	// 设置追踪器配置
	pw.SetTrackerLength(50)  // 设置进度条长度
	pw.SetMessageLength(20)  // 设置消息宽度
	pw.SetNumTrackersExpected(2)  // 预期的追踪器数量

	// 可配置的可见性
	pw.Style().Visibility.ETA = true        // 显示预计剩余时间
	pw.Style().Visibility.Percentage = true // 显示百分比
	pw.Style().Visibility.Speed = true      // 显示速度
	pw.Style().Visibility.Value = true      // 显示当前值
	pw.Style().Visibility.TrackerOverall = true // 显示总体进度

	// 创建翻译字数跟踪器
	translationBar := &progress.Tracker{
		Message: "翻译字数",
		Total:   totalUnits,
		Units:   progress.UnitsBytes,
	}

	// 设置完成状态
	translationBar.UpdateMessage("翻译字数 (0.0%)")

	// 创建翻译成本跟踪器
	costBar := &progress.Tracker{
		Message: "翻译成本",
		Total:   1000, // 设置一个合理的最大值
		Units:   progress.UnitsCurrencyDollar,
	}

	// 设置完成状态
	costBar.UpdateMessage("翻译成本 (0.0%)")

	// 添加到进度条
	pw.AppendTracker(translationBar)
	pw.AppendTracker(costBar)

	// 启动渲染协程
	go pw.Render()

	// 创建我们自己的进度跟踪器
	tracker := NewProgressTracker(
		totalUnits,
		WithUnit("字符", "chars"),
		WithMessage("翻译进度"),
		WithBarStyle(50, "█", "░", "[", "]"),
		WithCost(0.00002, "$"),
		WithColors(
			text.Colors{text.FgHiWhite, text.Bold},  // 百分比颜色
			text.Colors{text.FgCyan},                // 进度条颜色
			text.Colors{text.FgHiBlack},             // 统计信息颜色
			text.Colors{text.FgGreen},               // 时间信息颜色
			text.Colors{text.FgYellow},              // 单位信息颜色
			text.Colors{text.FgMagenta},             // 成本信息颜色
			text.Colors{text.FgWhite},               // 消息颜色
		),
		WithVisibility(
			true,  // 显示百分比
			true,  // 显示进度条
			true,  // 显示统计信息
			true,  // 显示时间信息
			true,  // 显示ETA
			true,  // 显示成本信息
			true,  // 显示速度信息
		),
		WithWriter(os.Stdout),
		WithRefreshInterval(time.Second),
	)

	return &ProgressAdapter{
		tracker:        tracker,
		translationBar: translationBar,
		costBar:        costBar,
		writer:         &pw,
	}
}

// Start 开始进度跟踪
func (pa *ProgressAdapter) Start() {
	pa.tracker.Start()
}

// Update 更新进度
func (pa *ProgressAdapter) Update(completedUnits int64) {
	// 更新我们自己的进度跟踪器
	pa.tracker.Update(completedUnits)

	// 更新 go-pretty 进度条
	pa.translationBar.SetValue(completedUnits)

	// 确保总值已设置
	if pa.translationBar.Total <= 0 && pa.tracker.totalUnits > 0 {
		pa.translationBar.UpdateTotal(pa.tracker.totalUnits)
	}

	// 更新成本跟踪器
	cost := pa.tracker.GetCost()
	// 将成本乘以1000作为整数值，以便在进度条中显示
	costValue := int64(cost * 1000)
	// 确保成本跟踪器的总值合理
	if costValue > pa.costBar.Total && costValue > 0 {
		pa.costBar.UpdateTotal(costValue * 2) // 动态调整总值
	}
	pa.costBar.SetValue(costValue)

	// 更新消息
	percent := pa.tracker.GetPercentage()
	pa.translationBar.UpdateMessage(fmt.Sprintf("翻译字数 (%.1f%%)", percent))

	// 更新成本消息
	eta := pa.tracker.GetETA()
	var remainingTimeStr string
	if eta > 0 {
		if eta.Minutes() < 1 {
			remainingTimeStr = fmt.Sprintf("%.1f秒", eta.Seconds())
		} else {
			remainingTimeStr = fmt.Sprintf("%.1f分钟", eta.Minutes())
		}
	} else {
		remainingTimeStr = "计算中..."
	}
	pa.costBar.UpdateMessage(fmt.Sprintf("成本: $%.4f (剩余: %s)",
		cost, remainingTimeStr))
}

// Stop 停止进度跟踪
func (pa *ProgressAdapter) Stop() {
	pa.tracker.Stop()
	(*pa.writer).Stop()
}

// Done 标记为已完成
func (pa *ProgressAdapter) Done() {
	// 先更新进度为100%
	if pa.tracker.totalUnits > 0 {
		pa.tracker.Update(pa.tracker.totalUnits)
	}

	// 标记为已完成
	pa.tracker.Done()

	// 更新 go-pretty 进度条
	// 更新消息
	pa.translationBar.UpdateMessage("翻译字数 (100.0%)")
	pa.costBar.UpdateMessage(fmt.Sprintf("翻译成本: $%.4f (已完成)", pa.tracker.GetCost()))

	// 标记为已完成
	pa.translationBar.MarkAsDone()
	pa.costBar.MarkAsDone()

	// 停止渲染
	(*pa.writer).Stop()
}

// GetWriter 获取 go-pretty 进度条写入器
func (pa *ProgressAdapter) GetWriter() *progress.Writer {
	return pa.writer
}

// GetTracker 获取进度跟踪器
func (pa *ProgressAdapter) GetTracker() *ProgressTracker {
	return pa.tracker
}
