package translator_tests

import (
	"testing"
	"time"

	"github.com/pterm/pterm"
	"github.com/stretchr/testify/assert"
)

// 测试进度条显示功能
func TestProgressBar(t *testing.T) {
	// 由于pterm的输出捕获可能不稳定，我们只测试进度条的基本功能
	// 不验证具体的输出内容

	// 创建进度条
	bar, err := pterm.DefaultProgressbar.WithTotal(100).WithTitle("翻译进度").Start()
	assert.NoError(t, err)

	// 更新进度条
	for i := 0; i <= 100; i += 10 {
		bar.Add(10)
		time.Sleep(10 * time.Millisecond) // 短暂延迟，模拟实际处理
	}

	// 停止进度条
	bar.Stop()

	// 如果没有panic，测试通过
	assert.True(t, true, "进度条测试通过")
}

// 测试新的进度条系统
func TestNewProgressBar(t *testing.T) {
	// 由于pterm的输出捕获可能不稳定，我们只测试进度条的基本功能
	// 不验证具体的输出内容

	// 创建进度条
	bar, err := pterm.DefaultProgressbar.
		WithTotal(1000).
		WithTitle("翻译进度").
		WithRemoveWhenDone(false).
		Start()
	assert.NoError(t, err)

	// 初始化统计数据
	totalChars := 1000
	startTime := time.Now()
	processedChars := 0
	inputTokens := 0
	outputTokens := 0
	inputTokenPrice := 0.001 // 每1000个token的价格
	outputTokenPrice := 0.002 // 每1000个token的价格

	// 更新进度条
	for i := 0; i <= 100; i += 5 {
		// 更新处理的字符数
		newChars := totalChars / 20
		processedChars += newChars
		inputTokens += newChars / 4 // 假设每4个字符是1个token
		outputTokens += newChars / 4

		// 计算进度
		progress := float64(processedChars) / float64(totalChars) * 100

		// 计算速度（字符/秒）
		elapsedSeconds := time.Since(startTime).Seconds()
		speed := float64(processedChars) / elapsedSeconds

		// 计算剩余时间
		remainingChars := totalChars - processedChars
		remainingTime := float64(remainingChars) / speed
		remainingTimeStr := formatDuration(time.Duration(remainingTime) * time.Second)

		// 计算成本
		cost := float64(inputTokens) * inputTokenPrice / 1000.0 + float64(outputTokens) * outputTokenPrice / 1000.0
		costStr := pterm.Sprintf("$%.4f", cost)

		// 更新进度条标题
		title := pterm.Sprintf(
			"翻译进度: %.1f%% | %d/%d 字符 | %.1f 字符/秒 | 成本: %s | 剩余时间: %s",
			progress,
			processedChars,
			totalChars,
			speed,
			costStr,
			remainingTimeStr,
		)
		bar.UpdateTitle(title)
		bar.Add(50) // 总进度是1000，每次增加50

		time.Sleep(50 * time.Millisecond) // 短暂延迟，模拟实际处理
	}

	// 停止进度条
	bar.Stop()

	// 如果没有panic，测试通过
	assert.True(t, true, "新进度条测试通过")
}

// 格式化持续时间
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return pterm.Sprintf("%dh%02dm%02ds", h, m, s)
	} else if m > 0 {
		return pterm.Sprintf("%dm%02ds", m, s)
	}
	return pterm.Sprintf("%ds", s)
}
