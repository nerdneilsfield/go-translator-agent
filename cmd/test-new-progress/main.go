package main

import (
	"fmt"
	"os"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
)

func main() {
	// 创建一个新的进度跟踪器
	tracker := translator.NewNewProgressTracker(1000)

	// 设置模型价格
	tracker.UpdateModelPrice(translator.ModelPrice{
		InitialModelInputPrice:      0.0001,
		InitialModelOutputPrice:     0.0002,
		InitialModelPriceUnit:       "$",
		ReflectionModelInputPrice:   0.0001,
		ReflectionModelOutputPrice:  0.0002,
		ReflectionModelPriceUnit:    "$",
		ImprovementModelInputPrice:  0.0001,
		ImprovementModelOutputPrice: 0.0002,
		ImprovementModelPriceUnit:   "$",
	})

	// 开始进度跟踪
	tracker.Start()

	// 模拟翻译过程
	for i := 0; i < 1000; i += 10 {
		// 更新进度
		tracker.UpdateProgress(10)

		// 模拟 token 使用
		if i % 100 == 0 {
			tracker.UpdateInitialTokenUsage(50, 100)
			tracker.UpdateReflectionTokenUsage(30, 60)
			tracker.UpdateImprovementTokenUsage(20, 40)
		}

		// 模拟翻译耗时
		time.Sleep(50 * time.Millisecond)
	}

	// 标记为完成
	tracker.Done()

	// 获取进度信息
	totalChars, translatedChars, realTotalChars, estimatedTimeRemaining, _, estimatedCost := tracker.GetProgress()

	// 输出最终统计信息
	fmt.Println("\n最终统计信息:")
	fmt.Printf("总字符数: %d\n", totalChars)
	fmt.Printf("已翻译字符数: %d\n", translatedChars)
	fmt.Printf("实际总字符数: %d\n", realTotalChars)
	fmt.Printf("预计剩余时间: %.2f秒\n", estimatedTimeRemaining)
	fmt.Printf("总耗时: %s\n", time.Since(tracker.GetStartTime()))
	fmt.Printf("总成本: $%.4f\n", estimatedCost.TotalCost)

	os.Exit(0)
}
