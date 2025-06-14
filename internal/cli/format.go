package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/formatfix"
	"github.com/nerdneilsfield/go-translator-agent/internal/formatfix/loader"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	// format 命令相关标志
	formatInteractive bool
	formatAutoFix     bool
	formatListOnly    bool
	formatOutputFile  string
)

// NewFormatCommand 创建格式修复命令
func NewFormatCommand() *cobra.Command {
	formatCmd := &cobra.Command{
		Use:   "format [flags] <file1> [file2] ...",
		Short: "检查和修复文件格式问题",
		Long: `格式修复工具可以检查和修复各种格式的文件问题，包括：

- Markdown 格式问题（标题、列表、链接等）
- 文本格式问题（编码、行尾、空白字符等）
- OCR 转换常见错误
- 使用外部工具（markdownlint、prettier）进行专业修复

支持的格式：
  - Markdown (.md, .markdown)
  - 纯文本 (.txt)
  - HTML (.html, .htm) [计划中]
  - EPUB (.epub) [计划中]

用法示例：
  translator format document.md                    # 检查格式问题
  translator format --auto-fix document.md         # 自动修复问题
  translator format --interactive document.md      # 交互式修复
  translator format --list                         # 列出可用的修复器
  translator format -o fixed.md document.md        # 输出到指定文件`,
		Args: func(cmd *cobra.Command, args []string) error {
			if formatListOnly {
				return nil // 列表模式不需要文件参数
			}
			if len(args) < 1 {
				return fmt.Errorf("requires at least 1 file argument")
			}
			return nil
		},
		Run: runFormatCommand,
	}

	// 添加标志
	formatCmd.Flags().BoolVarP(&formatInteractive, "interactive", "i", false, "启用交互式修复模式")
	formatCmd.Flags().BoolVarP(&formatAutoFix, "auto-fix", "a", false, "自动修复所有可修复的问题")
	formatCmd.Flags().BoolVarP(&formatListOnly, "list", "l", false, "列出可用的格式修复器和外部工具")
	formatCmd.Flags().StringVarP(&formatOutputFile, "output", "o", "", "输出文件路径（仅支持单文件输入时）")
	formatCmd.Flags().BoolVar(&debugMode, "debug", false, "启用调试模式")

	return formatCmd
}

// runFormatCommand 运行格式修复命令
func runFormatCommand(cmd *cobra.Command, args []string) {
	// 初始化日志
	log := logger.NewLogger(debugMode)
	defer func() {
		_ = log.Sync()
	}()

	// 处理列表模式
	if formatListOnly {
		handleFormatList(log)
		return
	}

	// 加载配置
	_, err := config.LoadConfig(cfgFile)
	if err != nil {
		log.Debug("using default config", zap.Error(err))
	}

	// 创建格式修复器注册中心
	var registry *formatfix.FixerRegistry
	if formatInteractive {
		registry, err = loader.CreateRegistry(log)
	} else {
		registry, err = loader.CreateSilentRegistry(log)
	}

	if err != nil {
		log.Error("failed to create format fix registry", zap.Error(err))
		fmt.Println("错误：无法创建格式修复器注册中心")
		os.Exit(1)
	}

	// 处理文件
	if len(args) == 1 && formatOutputFile != "" {
		// 单文件处理，输出到指定文件
		handleSingleFileWithOutput(args[0], formatOutputFile, registry, log)
	} else {
		// 多文件处理或就地修复
		for _, filePath := range args {
			handleSingleFile(filePath, registry, log)
		}
	}
}

// handleFormatList 处理列表命令
func handleFormatList(log *zap.Logger) {
	registry, err := loader.CreateRegistry(log)
	if err != nil {
		log.Error("failed to create format fix registry", zap.Error(err))
		fmt.Println("错误：无法创建格式修复器注册中心")
		os.Exit(1)
	}

	fmt.Println("🔧 可用的格式修复器")
	fmt.Println(strings.Repeat("=", 50))

	stats := registry.GetStats()
	if fixerInfo, ok := stats["fixer_info"].(map[string][]string); ok {
		for name, formats := range fixerInfo {
			fmt.Printf("📄 %s\n", name)
			fmt.Printf("   支持格式: %s\n\n", strings.Join(formats, ", "))
		}
	}

	fmt.Printf("📋 支持的格式总览: %s\n\n", strings.Join(registry.GetSupportedFormats(), ", "))

	// 检查外部工具可用性
	fmt.Println("🛠️  外部工具状态")
	fmt.Println(strings.Repeat("=", 50))

	toolManager := formatfix.NewDefaultToolManager(log)
	tools := []struct {
		name        string
		description string
	}{
		{"markdownlint", "Markdown 代码检查工具"},
		{"prettier", "代码格式化工具"},
		{"htmlhint", "HTML 代码检查工具"},
	}

	for _, tool := range tools {
		status := "❌ 不可用"
		suggestion := ""

		if toolManager.IsToolAvailable(tool.name) {
			if version, err := toolManager.GetToolVersion(tool.name); err == nil {
				status = fmt.Sprintf("✅ 可用 (%s)", strings.TrimSpace(version))
			} else {
				status = "✅ 可用"
			}
		} else {
			suggestion = toolManager.SuggestInstallation(tool.name)
		}

		fmt.Printf("%-15s %s\n", tool.name+":", status)
		fmt.Printf("%-15s %s\n", "", tool.description)
		if suggestion != "" {
			fmt.Printf("%-15s 安装: %s\n", "", suggestion)
		}
		fmt.Println()
	}
}

// handleSingleFile 处理单个文件
func handleSingleFile(filePath string, registry *formatfix.FixerRegistry, log *zap.Logger) {
	fmt.Printf("\n🔍 检查文件: %s\n", filePath)
	fmt.Println(strings.Repeat("-", 60))

	// 读取文件
	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("❌ 错误：无法读取文件: %v\n", err)
		return
	}

	// 检测文件格式
	format := detectFileFormat(filePath)
	fmt.Printf("📝 检测到格式: %s\n", format)

	// 检查是否支持此格式
	if !registry.IsFormatSupported(format) {
		fmt.Printf("⚠️  警告：不支持的格式 %s\n", format)
		return
	}

	// 获取修复器
	fixer, err := registry.GetFixerForFormat(format)
	if err != nil {
		fmt.Printf("❌ 错误：无法获取格式修复器: %v\n", err)
		return
	}

	if formatAutoFix {
		// 自动修复模式
		fixedContent, issues, err := fixer.AutoFix(content)
		if err != nil {
			fmt.Printf("❌ 错误：自动修复失败: %v\n", err)
			return
		}

		if len(issues) == 0 {
			fmt.Println("✅ 未发现需要修复的问题")
		} else {
			fmt.Printf("🔧 自动修复了 %d 个问题:\n", len(issues))
			for _, issue := range issues {
				fmt.Printf("  ✓ 行%d: [%s] %s\n", issue.Line, issue.Type, issue.Message)
			}

			// 写回文件
			if err := os.WriteFile(filePath, fixedContent, 0o644); err != nil {
				fmt.Printf("❌ 错误：无法写入文件: %v\n", err)
				return
			}
			fmt.Printf("💾 已保存修复后的文件: %s\n", filePath)
		}
	} else {
		// 检查模式
		issues, err := fixer.CheckIssues(content)
		if err != nil {
			fmt.Printf("❌ 错误：检查格式问题失败: %v\n", err)
			return
		}

		if len(issues) == 0 {
			fmt.Println("✅ 未发现格式问题")
		} else {
			fmt.Printf("📋 发现 %d 个格式问题:\n", len(issues))
			showIssuesByseverity(issues)
			fmt.Printf("\n💡 提示：使用 --auto-fix 自动修复，或 --interactive 交互式修复\n")
		}
	}
}

// handleSingleFileWithOutput 处理单文件并输出到指定文件
func handleSingleFileWithOutput(inputPath, outputPath string, registry *formatfix.FixerRegistry, log *zap.Logger) {
	ctx := context.Background()

	fmt.Printf("🔍 处理文件: %s -> %s\n", inputPath, outputPath)
	fmt.Println(strings.Repeat("-", 60))

	// 读取输入文件
	content, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Printf("❌ 错误：无法读取文件: %v\n", err)
		return
	}

	// 检测文件格式
	format := detectFileFormat(inputPath)
	fmt.Printf("📝 检测到格式: %s\n", format)

	// 检查是否支持此格式
	if !registry.IsFormatSupported(format) {
		fmt.Printf("⚠️  警告：不支持的格式 %s，将直接复制文件\n", format)
		if err := os.WriteFile(outputPath, content, 0o644); err != nil {
			fmt.Printf("❌ 错误：无法写入文件: %v\n", err)
		} else {
			fmt.Printf("💾 已复制到: %s\n", outputPath)
		}
		return
	}

	// 获取修复器
	fixer, err := registry.GetFixerForFormat(format)
	if err != nil {
		fmt.Printf("❌ 错误：无法获取格式修复器: %v\n", err)
		return
	}

	// 执行修复
	var fixedContent []byte
	var issues []*formatfix.FixIssue

	if formatInteractive {
		// 交互式修复
		interactor := formatfix.NewConsoleInteractor(true, true)
		fixedContent, issues, err = fixer.PreTranslationFix(ctx, content, interactor)
	} else {
		// 自动修复
		fixedContent, issues, err = fixer.AutoFix(content)
	}

	if err != nil {
		fmt.Printf("❌ 错误：格式修复失败: %v\n", err)
		return
	}

	// 确保输出目录存在
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fmt.Printf("❌ 错误：无法创建输出目录: %v\n", err)
		return
	}

	// 写入输出文件
	if err := os.WriteFile(outputPath, fixedContent, 0o644); err != nil {
		fmt.Printf("❌ 错误：无法写入输出文件: %v\n", err)
		return
	}

	// 显示结果
	if len(issues) == 0 {
		fmt.Println("✅ 未发现需要修复的问题")
	} else {
		fmt.Printf("🔧 修复了 %d 个问题:\n", len(issues))
		for _, issue := range issues {
			fmt.Printf("  ✓ 行%d: [%s] %s\n", issue.Line, issue.Type, issue.Message)
		}
	}
	fmt.Printf("💾 已保存到: %s\n", outputPath)
}

// showIssuesByseverity 按严重性显示问题
func showIssuesByseverity(issues []*formatfix.FixIssue) {
	// 按严重性分组
	severityGroups := make(map[formatfix.Severity][]*formatfix.FixIssue)
	for _, issue := range issues {
		severityGroups[issue.Severity] = append(severityGroups[issue.Severity], issue)
	}

	// 按严重性显示
	severities := []formatfix.Severity{
		formatfix.SeverityCritical,
		formatfix.SeverityError,
		formatfix.SeverityWarning,
		formatfix.SeverityInfo,
	}

	for _, severity := range severities {
		if issues := severityGroups[severity]; len(issues) > 0 {
			fmt.Printf("\n%s (%d个):\n", getSeverityIcon(severity), len(issues))
			for _, issue := range issues {
				fmt.Printf("  行%d列%d: [%s] %s\n", issue.Line, issue.Column, issue.Type, issue.Message)
				if issue.Suggestion != "" && issue.Suggestion != issue.Message {
					fmt.Printf("    💡 建议: %s\n", issue.Suggestion)
				}
			}
		}
	}
}

// GetFormatCommand 返回格式化命令（保持向后兼容）
func GetFormatCommand() *cobra.Command {
	return NewFormatCommand()
}
