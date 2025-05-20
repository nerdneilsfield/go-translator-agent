package main

import (
	"fmt"
	"os"
	"time"

	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/nerdneilsfield/go-translator-agent/pkg/progress"
)

func main() {
	// 创建一个进度跟踪器
	tracker := progress.NewTracker(
		1000, // 总单位数
		progress.WithUnit("字符", "chars"),
		progress.WithMessage("翻译进度"),
		progress.WithBarStyle(50, "█", "░", "[", "]"),
		progress.WithCost(0.00002, "$"),
		progress.WithColors(
			text.Colors{text.FgHiWhite, text.Bold}, // 百分比颜色
			text.Colors{text.FgCyan},               // 进度条颜色
			text.Colors{text.FgHiBlack},            // 统计信息颜色
			text.Colors{text.FgGreen},              // 时间信息颜色
			text.Colors{text.FgYellow},             // 单位信息颜色
			text.Colors{text.FgMagenta},            // 成本信息颜色
			text.Colors{text.FgWhite},              // 消息颜色
		),
		progress.WithVisibility(
			true, // 显示百分比
			true, // 显示进度条
			true, // 显示统计信息
			true, // 显示时间信息
			true, // 显示ETA
			true, // 显示成本信息
			true, // 显示速度信息
		),
		progress.WithRefreshInterval(100*time.Millisecond),
	)

	// 开始进度跟踪
	tracker.Start()

	// 模拟翻译过程
	for i := int64(0); i < 1000; i += 10 {
		// 更新进度
		tracker.Update(i)

		// 模拟翻译耗时
		time.Sleep(10 * time.Millisecond)
	}

	// 标记为完成
	tracker.Done()

	// 输出最终统计信息
	fmt.Println("\n最终统计信息:")
	fmt.Printf("总字符数: %d\n", 1000)
	fmt.Printf("翻译速度: %.2f 字符/秒\n", tracker.GetSpeed())
	fmt.Printf("总耗时: %s\n", tracker.GetElapsedTime())
	fmt.Printf("总成本: $%.4f\n", tracker.GetCost())

	os.Exit(0)
}
