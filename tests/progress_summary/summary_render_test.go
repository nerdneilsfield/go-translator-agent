package progress_summary

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/pkg/progress" // 确保路径正确
)

func TestProgressTracker_SummaryTableRendering(t *testing.T) {
	var buf bytes.Buffer

	// 1. 初始化 Tracker，并使用 bytes.Buffer 作为 writer
	tracker := progress.NewTracker(
		100, // totalUnits
		progress.WithWriter(&buf),
		progress.WithMessage("测试摘要表格"),
		progress.WithUnit("项", "items"),
		progress.WithRefreshInterval(100*time.Millisecond), // 加快测试时的刷新
	)

	// 2. 模拟进度
	tracker.Start()
	tracker.Update(25)
	time.Sleep(150 * time.Millisecond) // 等待几次渲染
	tracker.Update(75)
	time.Sleep(150 * time.Millisecond)

	// 3. 准备 SummaryStats 数据
	summaryStats := &progress.SummaryStats{
		InputTextLength: 1000,
		TextTranslated:  750,
		TotalTime:       25 * time.Second,
		Steps: []progress.StepStats{
			{
				StepName:     "翻译步骤1",
				InputTokens:  1200,
				OutputTokens: 1500,
				TokenSpeed:   50.5,
				Cost:         0.0025,
				CostUnit:     "$",
				HasData:      true,
			},
			{
				StepName:     "反思步骤",
				InputTokens:  300,
				OutputTokens: 400,
				TokenSpeed:   0, // 假设无速度信息
				Cost:         0.0005,
				CostUnit:     "$",
				HasData:      true,
			},
			{
				StepName: "没有数据的步骤", // 这个步骤不应出现在表格中，除非我们改变逻辑
				HasData:  false,
			},
		},
		TotalCost:     0.0030,
		TotalCostUnit: "$",
	}

	// 4. 调用 Done() 传入 summary
	tracker.Done(summaryStats)

	// 5. 检查输出
	output := buf.String()

	// 打印捕获的输出，方便调试
	t.Logf("捕获的进度条和表格输出:\\n%s", output)

	// 断言进度条活动期间的某些输出
	if !strings.Contains(output, "测试摘要表格:") { // 移除了对 \r 的检查
		t.Errorf("输出中未找到预期的进度条消息")
	}
	// 检查最终的百分比，考虑可能的颜色代码
	if !strings.Contains(output, "100.0%") { // 直接检查 "100.0%"
		t.Errorf("输出中未找到预期的最终进度百分比 '100.0%%'")
	}

	// 断言总结表格中的关键信息
	expectedTableContent := []string{
		"项", "值", // 表头
		"原始文本长度 (单位)", "1000 items",
		"已翻译 (单位)", "750 items",
		"总耗时", "25.0s", // 修正：匹配实际输出
		"翻译步骤1 - 输入 Tokens", "1200",
		"翻译步骤1 - 输出 Tokens", "1500",
		"翻译步骤1 - Token 速度", "50.50 t/s",
		"翻译步骤1 - 成本", "$0.0025",
		"反思步骤 - 输入 Tokens", "300",
		"反思步骤 - 成本", "$0.0005",
		"总成本", "$0.0030",
	}

	for _, expected := range expectedTableContent {
		if !strings.Contains(output, expected) {
			t.Errorf("输出中未找到预期的表格内容: %q", expected)
		}
	}

	if strings.Contains(output, "没有数据的步骤") {
		t.Errorf("表格中不应包含 HasData 为 false 的步骤")
	}

	// 检查是否有换行，确保表格和进度条分离
	// 进度条最后一行后有一个换行，表格渲染（包含多个换行），表格后有一个换行
	// 因此，总换行符数量应该明显多于3
	if strings.Count(output, "\n") < 3 { // 修正：使用 "\n" 来查找实际的换行符
		t.Errorf("输出中换行符数量不足，可能影响可读性。找到 %d 个换行符。输出: %s", strings.Count(output, "\n"), output)
	}
}
