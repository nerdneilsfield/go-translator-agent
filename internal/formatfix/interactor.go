package formatfix

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
)

// ConsoleInteractor 控制台交互实现
type ConsoleInteractor struct {
	reader       *bufio.Reader
	autoActions  map[string]FixAction // 自动动作缓存，避免重复询问
	verbose      bool                 // 是否显示详细信息
	showProgress bool                 // 是否显示进度
}

// NewConsoleInteractor 创建控制台交互器
func NewConsoleInteractor(verbose, showProgress bool) *ConsoleInteractor {
	return &ConsoleInteractor{
		reader:       bufio.NewReader(os.Stdin),
		autoActions:  make(map[string]FixAction),
		verbose:      verbose,
		showProgress: showProgress,
	}
}

// ConfirmFix 询问用户是否应用修复
func (ci *ConsoleInteractor) ConfirmFix(issue *FixIssue) FixAction {
	// 检查是否有缓存的自动动作
	if action, exists := ci.autoActions[issue.Type]; exists {
		switch action {
		case FixActionApplyAll:
			return FixActionApply
		case FixActionSkipAll:
			return FixActionSkip
		}
	}

	// 显示问题信息
	ci.displayIssue(issue)

	// 获取用户输入
	for {
		fmt.Print(ci.getPrompt())

		input, err := ci.reader.ReadString('\n')
		if err != nil {
			fmt.Printf("读取输入错误: %v\n", err)
			continue
		}

		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "y", "yes", "a", "apply":
			return FixActionApply
		case "n", "no", "s", "skip":
			return FixActionSkip
		case "aa", "apply-all":
			ci.autoActions[issue.Type] = FixActionApplyAll
			return FixActionApply
		case "sa", "skip-all":
			ci.autoActions[issue.Type] = FixActionSkipAll
			return FixActionSkip
		case "q", "quit", "abort":
			return FixActionAbort
		case "h", "help", "?":
			ci.showHelp()
		case "d", "details":
			ci.showIssueDetails(issue)
		default:
			color.Red("无效输入，请重试。输入 'h' 查看帮助。")
		}
	}
}

// displayIssue 显示问题信息
func (ci *ConsoleInteractor) displayIssue(issue *FixIssue) {
	fmt.Println(strings.Repeat("─", 70))

	// 显示问题类型和严重程度
	severityColor := ci.getSeverityColor(issue.Severity)
	typeColor := color.New(color.FgCyan, color.Bold)

	severityColor.Printf("► %s", issue.Severity.String())
	fmt.Print(" | ")
	typeColor.Printf("%s", issue.Type)

	if issue.Line > 0 {
		fmt.Printf(" | 行 %d", issue.Line)
		if issue.Column > 0 {
			fmt.Printf(":%d", issue.Column)
		}
	}
	fmt.Println()

	// 显示问题描述
	fmt.Printf("问题: %s\n", issue.Message)

	if issue.Suggestion != "" {
		color.Yellow("建议: %s", issue.Suggestion)
	}

	// 显示文本对比（如果有的话）
	if issue.OriginalText != "" && issue.FixedText != "" {
		ci.showTextComparison(issue.OriginalText, issue.FixedText)
	}

	fmt.Println()
}

// showTextComparison 显示文本对比
func (ci *ConsoleInteractor) showTextComparison(original, fixed string) {
	fmt.Println()
	color.Red("- 原始: %s", ci.escapeString(original))
	color.Green("+ 修复: %s", ci.escapeString(fixed))
}

// escapeString 转义特殊字符用于显示
func (ci *ConsoleInteractor) escapeString(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\r", "\\r")

	// 如果字符串太长，截取并添加省略号
	if len(s) > 100 {
		s = s[:97] + "..."
	}

	return fmt.Sprintf("%q", s)
}

// getSeverityColor 根据严重程度获取颜色
func (ci *ConsoleInteractor) getSeverityColor(severity Severity) *color.Color {
	switch severity {
	case SeverityInfo:
		return color.New(color.FgBlue, color.Bold)
	case SeverityWarning:
		return color.New(color.FgYellow, color.Bold)
	case SeverityError:
		return color.New(color.FgRed, color.Bold)
	case SeverityCritical:
		return color.New(color.FgMagenta, color.Bold)
	default:
		return color.New(color.FgWhite, color.Bold)
	}
}

// getPrompt 获取用户输入提示
func (ci *ConsoleInteractor) getPrompt() string {
	prompt := color.New(color.FgGreen, color.Bold)
	return prompt.Sprint("应用修复? [y/n/aa/sa/q/h]: ")
}

// showHelp 显示帮助信息
func (ci *ConsoleInteractor) showHelp() {
	fmt.Println()
	color.Cyan("可用命令:")
	fmt.Println("  y, yes, a, apply  - 应用此修复")
	fmt.Println("  n, no, s, skip    - 跳过此修复")
	fmt.Println("  aa, apply-all     - 应用此类型的所有修复")
	fmt.Println("  sa, skip-all      - 跳过此类型的所有修复")
	fmt.Println("  q, quit, abort    - 中止修复过程")
	fmt.Println("  d, details        - 显示详细信息")
	fmt.Println("  h, help, ?        - 显示此帮助")
	fmt.Println()
}

// showIssueDetails 显示问题详细信息
func (ci *ConsoleInteractor) showIssueDetails(issue *FixIssue) {
	fmt.Println()
	color.Cyan("问题详细信息:")
	fmt.Printf("  类型: %s\n", issue.Type)
	fmt.Printf("  严重程度: %s\n", issue.Severity.String())
	fmt.Printf("  位置: 行 %d, 列 %d\n", issue.Line, issue.Column)
	fmt.Printf("  可自动修复: %t\n", issue.CanAutoFix)

	if issue.OriginalText != "" {
		fmt.Printf("  原始文本: %s\n", ci.escapeString(issue.OriginalText))
	}

	if issue.FixedText != "" {
		fmt.Printf("  修复后文本: %s\n", ci.escapeString(issue.FixedText))
	}

	fmt.Println()
}

// ShowSummary 显示修复摘要
func (ci *ConsoleInteractor) ShowSummary(applied, skipped int, issues []*FixIssue) {
	fmt.Println()
	fmt.Println(strings.Repeat("═", 70))

	title := color.New(color.FgGreen, color.Bold)
	title.Println("📊 修复摘要")

	fmt.Println(strings.Repeat("═", 70))

	// 统计信息
	total := len(issues)
	fmt.Printf("总问题数: %d\n", total)

	if applied > 0 {
		color.Green("✅ 已修复: %d", applied)
	}

	if skipped > 0 {
		color.Yellow("⏭️  已跳过: %d", skipped)
	}

	if total > applied+skipped {
		remaining := total - applied - skipped
		color.Red("❌ 未处理: %d", remaining)
	}

	// 按类型统计
	if ci.verbose && len(issues) > 0 {
		fmt.Println()
		color.Cyan("按问题类型统计:")

		typeStats := make(map[string]int)
		for _, issue := range issues {
			typeStats[issue.Type]++
		}

		for issueType, count := range typeStats {
			fmt.Printf("  %s: %d\n", issueType, count)
		}
	}

	fmt.Println(strings.Repeat("═", 70))
}

// ShowProgress 显示修复进度
func (ci *ConsoleInteractor) ShowProgress(current, total int, currentIssue string) {
	if !ci.showProgress {
		return
	}

	percentage := float64(current) / float64(total) * 100
	bar := ci.createProgressBar(percentage)

	fmt.Printf("\r正在处理: [%s] %.1f%% (%d/%d) - %s",
		bar, percentage, current, total, currentIssue)
}

// createProgressBar 创建进度条
func (ci *ConsoleInteractor) createProgressBar(percentage float64) string {
	const barWidth = 20
	filled := int(percentage / 100 * barWidth)

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	if percentage >= 100 {
		return color.GreenString(bar)
	} else if percentage >= 50 {
		return color.YellowString(bar)
	} else {
		return color.RedString(bar)
	}
}

// SilentInteractor 静默交互器（自动应用所有修复）
type SilentInteractor struct {
	autoApply bool
}

// NewSilentInteractor 创建静默交互器
func NewSilentInteractor(autoApply bool) *SilentInteractor {
	return &SilentInteractor{autoApply: autoApply}
}

// ConfirmFix 静默模式直接返回预设动作
func (si *SilentInteractor) ConfirmFix(issue *FixIssue) FixAction {
	if si.autoApply && issue.CanAutoFix {
		return FixActionApply
	}
	return FixActionSkip
}

// ShowSummary 静默显示摘要
func (si *SilentInteractor) ShowSummary(applied, skipped int, issues []*FixIssue) {
	total := len(issues)
	fmt.Printf("格式修复完成: 总计 %d 个问题，修复 %d 个，跳过 %d 个\n",
		total, applied, skipped)
}

// ShowProgress 静默模式不显示进度
func (si *SilentInteractor) ShowProgress(current, total int, currentIssue string) {
	// 静默模式不显示进度
}

// TestInteractor 测试用交互器（预定义响应）
type TestInteractor struct {
	responses []FixAction
	index     int
}

// NewTestInteractor 创建测试交互器
func NewTestInteractor(responses []FixAction) *TestInteractor {
	return &TestInteractor{
		responses: responses,
		index:     0,
	}
}

// ConfirmFix 返回预定义的响应
func (ti *TestInteractor) ConfirmFix(issue *FixIssue) FixAction {
	if ti.index >= len(ti.responses) {
		return FixActionSkip // 默认跳过
	}

	action := ti.responses[ti.index]
	ti.index++
	return action
}

// ShowSummary 测试模式不显示摘要
func (ti *TestInteractor) ShowSummary(applied, skipped int, issues []*FixIssue) {
	// 测试模式不显示摘要
}

// ShowProgress 测试模式不显示进度
func (ti *TestInteractor) ShowProgress(current, total int, currentIssue string) {
	// 测试模式不显示进度
}
