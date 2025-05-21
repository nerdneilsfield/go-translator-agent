package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

// Tracker is responsible for tracking the progress of a task.
// It can be used to display a progress bar or provide progress updates.
type Tracker struct {
	mu sync.Mutex

	// 基本进度信息
	totalUnits      int64         // 总单位数（如字符数、字节数等）
	completedUnits  int64         // 已完成的单位数
	startTime       time.Time     // 开始时间
	lastUpdateTime  time.Time     // 最后更新时间
	speedSamples    []float64     // 速度样本，用于计算平均速度
	maxSpeedSamples int           // 最大速度样本数
	unit            string        // 单位名称（如"字符"、"字节"等）
	unitSymbol      string        // 单位符号（如"chars"、"B"等）
	writer          io.Writer     // 输出写入器
	refreshInterval time.Duration // 刷新间隔
	isActive        bool          // 是否处于活动状态
	isDone          bool          // 是否已完成

	// 成本相关
	costPerUnit     float64 // 每单位成本
	costCurrency    string  // 成本货币符号（如"$"、"¥"等）
	accumulatedCost float64 // 累计成本

	// 自定义格式化
	percentFormat string // 百分比格式（如"%.1f%%"）

	// 渲染相关
	barWidth      int    // 进度条宽度
	completedChar string // 已完成部分的字符
	remainingChar string // 未完成部分的字符
	leftBracket   string // 左括号
	rightBracket  string // 右括号

	// 颜色设置
	percentColor text.Colors // 百分比颜色
	barColor     text.Colors // 进度条颜色
	statsColor   text.Colors // 统计信息颜色
	timeColor    text.Colors // 时间信息颜色
	unitColor    text.Colors // 单位信息颜色
	costColor    text.Colors // 成本信息颜色
	messageColor text.Colors // 消息颜色

	// 显示选项
	showPercent bool // 是否显示百分比
	showBar     bool // 是否显示进度条
	showStats   bool // 是否显示统计信息
	showTime    bool // 是否显示时间信息
	showETA     bool // 是否显示预计剩余时间
	showCost    bool // 是否显示成本信息
	showSpeed   bool // 是否显示速度信息

	// 消息
	message string // 进度条消息
}

// NewTracker creates a new progress tracker.
func NewTracker(totalUnits int64, options ...Option) *Tracker {
	now := time.Now()

	// 创建默认进度跟踪器
	pt := &Tracker{
		totalUnits:      totalUnits,
		completedUnits:  0,
		startTime:       now,
		lastUpdateTime:  now,
		speedSamples:    make([]float64, 0, 10),
		maxSpeedSamples: 10,
		unit:            "字符",
		unitSymbol:      "chars",
		writer:          os.Stderr,
		refreshInterval: time.Second,
		isActive:        false,
		isDone:          false,
		costPerUnit:     0,
		costCurrency:    "$",
		accumulatedCost: 0,
		percentFormat:   "%.1f%%",
		barWidth:        50,
		completedChar:   "█",
		remainingChar:   "░",
		leftBracket:     "[",
		rightBracket:    "]",
		percentColor:    text.Colors{text.FgHiWhite},
		barColor:        text.Colors{text.FgCyan},
		statsColor:      text.Colors{text.FgHiBlack},
		timeColor:       text.Colors{text.FgGreen},
		unitColor:       text.Colors{text.FgYellow},
		costColor:       text.Colors{text.FgMagenta},
		messageColor:    text.Colors{text.FgWhite},
		showPercent:     true,
		showBar:         true,
		showStats:       true,
		showTime:        true,
		showETA:         true,
		showCost:        false,
		showSpeed:       true,
		message:         "进度",
	}

	// 应用选项
	for _, option := range options {
		option(pt)
	}

	return pt
}

// Option 定义进度跟踪器的选项
type Option func(*Tracker)

// WithUnit 设置单位名称和符号
func WithUnit(name, symbol string) Option {
	return func(pt *Tracker) {
		pt.unit = name
		pt.unitSymbol = symbol
	}
}

// WithWriter 设置输出写入器
func WithWriter(writer io.Writer) Option {
	return func(pt *Tracker) {
		pt.writer = writer
	}
}

// WithRefreshInterval 设置刷新间隔
func WithRefreshInterval(interval time.Duration) Option {
	return func(pt *Tracker) {
		pt.refreshInterval = interval
	}
}

// WithCost 设置成本相关信息
func WithCost(costPerUnit float64, currency string) Option {
	return func(pt *Tracker) {
		pt.costPerUnit = costPerUnit
		pt.costCurrency = currency
		pt.showCost = true
	}
}

// WithBarStyle 设置进度条样式
func WithBarStyle(width int, completedChar, remainingChar, leftBracket, rightBracket string) Option {
	return func(pt *Tracker) {
		pt.barWidth = width
		pt.completedChar = completedChar
		pt.remainingChar = remainingChar
		pt.leftBracket = leftBracket
		pt.rightBracket = rightBracket
	}
}

// WithMessage 设置进度条消息
func WithMessage(message string) Option {
	return func(pt *Tracker) {
		pt.message = message
	}
}

// Start 开始进度跟踪
func (pt *Tracker) Start() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.isActive = true
	pt.startTime = time.Now()
	pt.lastUpdateTime = pt.startTime

	// 初始渲染
	pt.render()

	// 启动定时刷新
	go pt.refreshLoop()
}

// refreshLoop 定时刷新进度条
func (pt *Tracker) refreshLoop() {
	ticker := time.NewTicker(pt.refreshInterval)
	defer ticker.Stop()

	for range ticker.C {
		pt.mu.Lock()
		if !pt.isActive {
			pt.mu.Unlock()
			return
		}

		// 只有在活动状态下才渲染进度条
		if pt.isActive {
			// 检查是否需要更新进度条
			// 如果最后一次更新时间距离现在超过了刷新间隔的一半，则更新进度条
			if time.Since(pt.lastUpdateTime) > pt.refreshInterval/2 {
				pt.render()
			}
		}

		pt.mu.Unlock()
	}
}

// Update 更新已完成的单位数
func (pt *Tracker) Update(completedUnits int64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if !pt.isActive || pt.isDone {
		return
	}

	// 计算增量
	delta := completedUnits - pt.completedUnits
	pt.completedUnits = completedUnits

	// 更新成本
	if pt.showCost {
		pt.accumulatedCost += float64(delta) * pt.costPerUnit
	}

	// 计算速度
	now := time.Now()
	elapsed := now.Sub(pt.lastUpdateTime).Seconds()
	if elapsed > 0 && delta > 0 {
		speed := float64(delta) / elapsed

		// 添加到速度样本
		if len(pt.speedSamples) >= pt.maxSpeedSamples {
			// 移除最旧的样本
			pt.speedSamples = pt.speedSamples[1:]
		}
		pt.speedSamples = append(pt.speedSamples, speed)
	}

	pt.lastUpdateTime = now

	// 检查是否完成
	if pt.completedUnits >= pt.totalUnits {
		pt.isDone = true
	}

	// 立即渲染进度条，提供更及时的反馈
	pt.render()
}

// Increment 增加已完成的单位数
func (pt *Tracker) Increment(delta int64) {
	pt.Update(pt.completedUnits + delta)
}

// SetTotal 设置总单位数
func (pt *Tracker) SetTotal(totalUnits int64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.totalUnits = totalUnits

	// 检查是否完成
	if pt.completedUnits >= pt.totalUnits {
		pt.isDone = true
	}
}

// SetMessage 设置进度条消息
func (pt *Tracker) SetMessage(message string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.message = message
}

// Stop 停止进度跟踪
func (pt *Tracker) Stop() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.isActive = false
	pt.render() // 最后一次渲染

	// 添加一个换行，确保后续输出在新行开始
	fmt.Fprintln(pt.writer)
}

// Done 标记为已完成
func (pt *Tracker) Done(summary *SummaryStats) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.isDone = true
	if pt.totalUnits > 0 && pt.completedUnits < pt.totalUnits {
		pt.completedUnits = pt.totalUnits
	}
	pt.render() // 最后一次渲染进度条本身

	// fmt.Fprintln(pt.writer) // 在 renderSummaryTable 之前或之后添加换行，根据需要调整

	// Unlock before calling renderSummaryTable, as renderSummaryTable might also use locks
	// or we simply don't need to hold the lock during table rendering.
	// However, pt.writer is accessed, so lock should be held or writer passed.
	// For simplicity, let's keep the lock for now and ensure renderSummaryTable doesn't re-lock.
	// Decision: renderSummaryTable will directly use pt.writer, so lock needs to be held.

	if summary != nil {
		// Add a newline before the summary table to separate it from the progress bar's last line
		fmt.Fprintln(pt.writer)
		pt.renderSummaryTable(summary)
	} else {
		// If no summary, still ensure a final newline after the progress bar.
		fmt.Fprintln(pt.writer)
	}
}

// GetPercentage 获取完成百分比
func (pt *Tracker) GetPercentage() float64 {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.totalUnits <= 0 {
		return 0
	}

	return float64(pt.completedUnits) / float64(pt.totalUnits) * 100
}

// GetSpeed 获取当前速度（单位/秒）
func (pt *Tracker) GetSpeed() float64 {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if len(pt.speedSamples) == 0 {
		return 0
	}

	// 计算平均速度
	var sum float64
	for _, speed := range pt.speedSamples {
		sum += speed
	}

	return sum / float64(len(pt.speedSamples))
}

// GetETA 获取预计剩余时间
func (pt *Tracker) GetETA() time.Duration {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	// 如果已完成或总单位数无效，返回0
	if pt.isDone || pt.totalUnits <= 0 || pt.completedUnits >= pt.totalUnits {
		return 0
	}

	// 获取当前速度
	speed := pt.getSpeedLocked()
	if speed <= 0 {
		// 如果速度为0，尝试使用总体平均速度
		elapsedTotal := time.Since(pt.startTime).Seconds()
		if elapsedTotal > 0 && pt.completedUnits > 0 {
			speed = float64(pt.completedUnits) / elapsedTotal
		}

		// 如果仍然为0，返回一个基于已用时间的估计
		if speed <= 0 {
			if pt.completedUnits > 0 {
				// 如果已经完成了一部分，根据已完成的比例估计
				elapsed := time.Since(pt.startTime)
				completedRatio := float64(pt.completedUnits) / float64(pt.totalUnits)
				if completedRatio > 0 {
					// 根据已完成的比例估计总时间，然后减去已用时间
					totalEstimated := time.Duration(float64(elapsed) / completedRatio)
					return totalEstimated - elapsed
				}
			}
			return 0
		}
	}

	// 计算剩余单位数
	remaining := pt.totalUnits - pt.completedUnits

	// 计算预计剩余时间（秒）
	etaSeconds := float64(remaining) / speed

	// 将秒转换为时间间隔
	return time.Duration(etaSeconds * float64(time.Second))
}

// getSpeedLocked 获取当前速度（内部使用，已加锁）
func (pt *Tracker) getSpeedLocked() float64 {
	if len(pt.speedSamples) == 0 {
		// 如果没有速度样本，尝试使用总体平均速度
		elapsedTotal := time.Since(pt.startTime).Seconds()
		if elapsedTotal > 0 && pt.completedUnits > 0 {
			return float64(pt.completedUnits) / elapsedTotal
		}
		return 0
	}

	// 计算平均速度，使用最近的样本加权更高
	var sum float64
	var weights float64

	for i, speed := range pt.speedSamples {
		// 越新的样本权重越高
		weight := float64(i + 1)
		sum += speed * weight
		weights += weight
	}

	if weights > 0 {
		return sum / weights
	}

	return 0
}

// GetElapsedTime 获取已经过的时间
func (pt *Tracker) GetElapsedTime() time.Duration {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	return time.Since(pt.startTime)
}

// GetCost 获取累计成本
func (pt *Tracker) GetCost() float64 {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	return pt.accumulatedCost
}

// IsDone 检查是否已完成
func (pt *Tracker) IsDone() bool {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	return pt.isDone
}

// IsActive 检查是否处于活动状态
func (pt *Tracker) IsActive() bool {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	return pt.isActive
}

// render 渲染进度条
func (pt *Tracker) render() {
	if pt.writer == nil {
		return
	}

	// 计算百分比
	var percent float64
	if pt.totalUnits > 0 {
		percent = float64(pt.completedUnits) / float64(pt.totalUnits) * 100
	}

	// 计算速度
	speed := pt.getSpeedLocked()

	// 计算ETA
	var eta time.Duration
	if !pt.isDone && speed > 0 {
		remaining := pt.totalUnits - pt.completedUnits
		etaSeconds := float64(remaining) / speed
		eta = time.Duration(etaSeconds * float64(time.Second))
	}

	// 计算已用时间
	elapsed := time.Since(pt.startTime)

	// 构建进度条
	var builder strings.Builder

	// 清除到行尾，然后回车
	builder.WriteString("\x1b[K") // ANSI escape code to clear from cursor to end of line
	builder.WriteString("\r")     // Carriage return

	// 添加消息
	if pt.message != "" {
		builder.WriteString(pt.messageColor.Sprint(pt.message))
		builder.WriteString(": ")
	}

	// 添加百分比
	if pt.showPercent {
		percentStr := fmt.Sprintf(pt.percentFormat, percent)
		builder.WriteString(pt.percentColor.Sprint(percentStr))
		builder.WriteString(" ")
	}

	// 添加进度条
	if pt.showBar {
		builder.WriteString(pt.leftBracket)

		// 计算已完成和未完成的字符数
		var completedWidth int
		if pt.totalUnits > 0 {
			completedWidth = int(float64(pt.barWidth) * float64(pt.completedUnits) / float64(pt.totalUnits))
			if completedWidth > pt.barWidth {
				completedWidth = pt.barWidth
			}
		}

		// 添加已完成部分
		if completedWidth > 0 {
			builder.WriteString(pt.barColor.Sprint(strings.Repeat(pt.completedChar, completedWidth)))
		}

		// 添加未完成部分
		remainingWidth := pt.barWidth - completedWidth
		if remainingWidth > 0 {
			builder.WriteString(strings.Repeat(pt.remainingChar, remainingWidth))
		}

		builder.WriteString(pt.rightBracket)
		builder.WriteString(" ")
	}

	// 添加统计信息
	if pt.showStats {
		// 添加已完成/总单位数
		statsStr := fmt.Sprintf("%d/%d %s", pt.completedUnits, pt.totalUnits, pt.unitSymbol)
		builder.WriteString(pt.unitColor.Sprint(statsStr))
		builder.WriteString(" ")
	}

	// 添加时间信息
	if pt.showTime {
		timeStr := fmt.Sprintf("用时: %s", formatDuration(elapsed))
		builder.WriteString(pt.timeColor.Sprint(timeStr))
		builder.WriteString(" ")
	}

	// 添加速度信息
	if pt.showSpeed && speed > 0 {
		speedStr := fmt.Sprintf("%.1f %s/s", speed, pt.unitSymbol)
		builder.WriteString(pt.statsColor.Sprint(speedStr))
		builder.WriteString(" ")
	}

	// 添加ETA信息
	if pt.showETA && !pt.isDone && eta > 0 {
		etaStr := fmt.Sprintf("ETA: %s", formatDuration(eta))
		builder.WriteString(pt.timeColor.Sprint(etaStr))
		builder.WriteString(" ")
	}

	// 添加成本信息
	if pt.showCost {
		costStr := fmt.Sprintf("%s%.4f", pt.costCurrency, pt.accumulatedCost)
		builder.WriteString(pt.costColor.Sprint(costStr))
	}

	// 计算需要清除的字符数
	// 确保输出足够长，覆盖之前的输出
	outputStr := builder.String()

	// 输出进度条，并确保清除之前的输出
	fmt.Fprint(pt.writer, outputStr)
}

// formatDuration 格式化时间间隔
func formatDuration(d time.Duration) string {
	// 对于小于1分钟的时间，显示秒
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}

	// 对于小于1小时的时间，显示分钟和秒
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", m, s)
	}

	// 对于大于1小时的时间，显示小时、分钟和秒
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dh%dm%ds", h, m, s)
}

// WithColors 设置颜色
func WithColors(percent, bar, stats, timeColor, unit, cost, message text.Colors) Option {
	return func(pt *Tracker) {
		pt.percentColor = percent
		pt.barColor = bar
		pt.statsColor = stats
		pt.timeColor = timeColor
		pt.unitColor = unit
		pt.costColor = cost
		pt.messageColor = message
	}
}

// WithVisibility 设置显示选项
func WithVisibility(showPercent, showBar, showStats, showTime, showETA, showCost, showSpeed bool) Option {
	return func(pt *Tracker) {
		pt.showPercent = showPercent
		pt.showBar = showBar
		pt.showStats = showStats
		pt.showTime = showTime
		pt.showETA = showETA
		pt.showCost = showCost
		pt.showSpeed = showSpeed
	}
}

// StepStats 包含单个步骤的统计信息
type StepStats struct {
	StepName     string // 例如 "Initial Translation", "Reflection", "Improvement"
	InputTokens  int
	OutputTokens int
	TokenSpeed   float64 // tokens/sec
	Cost         float64
	CostUnit     string
	HasData      bool // 标记此步骤是否有数据，用于决定是否在表格中显示
}

// SummaryStats 包含用于生成最终总结表格的统计信息
type SummaryStats struct {
	InputTextLength int // 原始文本总长度
	TextTranslated  int // 实际翻译的字符/单位数
	TotalTime       time.Duration
	Steps           []StepStats // 各个步骤的统计信息
	TotalCost       float64
	TotalCostUnit   string
}

// renderSummaryTable 渲染最终的总结表格
func (pt *Tracker) renderSummaryTable(stats *SummaryStats) {
	if pt.writer == nil || stats == nil {
		return
	}

	tw := table.NewWriter()
	tw.SetOutputMirror(pt.writer) // 直接写入 pt.writer

	tw.AppendRow(table.Row{"项", "值"})
	tw.AppendSeparator()
	tw.AppendRow(table.Row{"原始文本长度 (单位)", fmt.Sprintf("%d %s", stats.InputTextLength, pt.unitSymbol)}) // 使用 pt 的单位
	tw.AppendRow(table.Row{"已翻译 (单位)", fmt.Sprintf("%d %s", stats.TextTranslated, pt.unitSymbol)})     // 使用 pt 的单位
	tw.AppendRow(table.Row{"总耗时", formatDuration(stats.TotalTime)})                                    // 使用现有的 formatDuration

	for _, step := range stats.Steps {
		if step.HasData {
			tw.AppendSeparator()
			tw.AppendRow(table.Row{fmt.Sprintf("%s - 输入 Tokens", step.StepName), step.InputTokens})
			tw.AppendRow(table.Row{fmt.Sprintf("%s - 输出 Tokens", step.StepName), step.OutputTokens})
			if step.TokenSpeed > 0 {
				tw.AppendRow(table.Row{fmt.Sprintf("%s - Token 速度", step.StepName), fmt.Sprintf("%.2f t/s", step.TokenSpeed)})
			}
			if step.Cost > 0 || step.InputTokens > 0 || step.OutputTokens > 0 { // Show cost if there are tokens or explicit cost
				tw.AppendRow(table.Row{fmt.Sprintf("%s - 成本", step.StepName), fmt.Sprintf("%s%.4f", step.CostUnit, step.Cost)})
			}
		}
	}

	if len(stats.Steps) > 0 { // Only show total cost if there were steps with cost info
		tw.AppendSeparator()
		tw.AppendRow(table.Row{"总成本", fmt.Sprintf("%s%.4f", stats.TotalCostUnit, stats.TotalCost)})
	}

	tw.SetStyle(table.StyleLight) // 或者其他样式 table.StyleRounded, table.StyleBold
	// fmt.Fprintln(pt.writer) // 移除在 tw.Render() 之前的额外换行，Done() 中已添加
	tw.Render()
	fmt.Fprintln(pt.writer) // Add a newline after the table
}
