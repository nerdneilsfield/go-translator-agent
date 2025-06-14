package formatter

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Kunde21/markdownfmt/v3"
	"github.com/Kunde21/markdownfmt/v3/markdown"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// MarkdownFormatter Markdown 格式化器
type MarkdownFormatter struct {
	name            string
	priority        int
	preserveManager *translation.PreserveManager
}

// NewMarkdownFormatter 创建 Markdown 格式化器
func NewMarkdownFormatter() *MarkdownFormatter {
	return &MarkdownFormatter{
		name:            "markdown-formatter",
		priority:        50,
		preserveManager: translation.NewPreserveManager(translation.DefaultPreserveConfig),
	}
}

// FormatString 格式化 Markdown 内容（测试使用的接口）
func (f *MarkdownFormatter) FormatString(ctx context.Context, content string, format string, opts *FormatOptions) (string, error) {
	// 调用内部格式化方法
	formatOpts := FormatOptions{}
	if opts != nil {
		formatOpts = *opts
	}
	result, err := f.FormatBytes([]byte(content), formatOpts)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// FormatBytes 格式化字节内容（实际的格式化实现）
func (f *MarkdownFormatter) FormatBytes(content []byte, opts FormatOptions) ([]byte, error) {
	text := string(content)

	// 保护特定内容块
	protected, markers := f.protectBlocks(text, opts.PreserveBlocks)

	// 配置 markdownfmt 选项
	mdOpts := []markdown.Option{
		markdown.WithCodeFormatters(markdown.GoCodeFormatter),
	}

	// 处理格式化
	formatted, err := markdownfmt.Process("", []byte(protected), mdOpts...)
	if err != nil {
		return nil, &FormatError{
			Formatter: f.name,
			Reason:    "markdown formatting failed",
			Err:       err,
		}
	}

	// 恢复保护的内容
	result := f.restoreBlocks(string(formatted), markers)

	// 额外的清理和格式化
	result = f.additionalFormatting(result, opts)

	return []byte(result), nil
}

// CanFormat 检查是否支持格式
func (f *MarkdownFormatter) CanFormat(format string) bool {
	return format == "markdown" || format == "md"
}

// Priority 返回优先级
func (f *MarkdownFormatter) Priority() int {
	return f.priority
}

// Name 返回格式化器名称
func (f *MarkdownFormatter) Name() string {
	return f.name
}

// Format 实现 Formatter 接口
func (f *MarkdownFormatter) Format(content []byte, opts FormatOptions) ([]byte, error) {
	return f.FormatBytes(content, opts)
}

// protectBlocks 保护特定内容块
func (f *MarkdownFormatter) protectBlocks(text string, blocks []PreserveBlock) (string, map[string]string) {
	markers := make(map[string]string)
	protected := text

	for _, block := range blocks {
		switch block.Type {
		case "code":
			// 保护代码块
			protected = f.protectPattern(protected, "```[\\s\\S]*?```", "CODE", markers)
			// 保护内联代码
			protected = f.protectPattern(protected, "`[^`]+`", "INLINE_CODE", markers)

		case "latex":
			// 保护 LaTeX 块
			protected = f.protectPattern(protected, "\\$\\$[\\s\\S]*?\\$\\$", "LATEX_BLOCK", markers)
			// 保护内联 LaTeX
			protected = f.protectPattern(protected, "\\$[^$]+\\$", "LATEX_INLINE", markers)

		case "link":
			// 保护链接
			protected = f.protectPattern(protected, "\\[([^\\]]+)\\]\\(([^)]+)\\)", "LINK", markers)

		case "custom":
			// 自定义模式
			if block.Pattern != "" {
				protected = f.protectPattern(protected, block.Pattern, block.Type, markers)
			}
		}
	}

	return protected, markers
}

// protectPattern 保护匹配的模式
func (f *MarkdownFormatter) protectPattern(text, pattern, prefix string, markers map[string]string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return text
	}

	counter := 0
	return re.ReplaceAllStringFunc(text, func(match string) string {
		counter++
		marker := fmt.Sprintf("@@%s_%d@@", prefix, counter)
		markers[marker] = match
		return marker
	})
}

// restoreBlocks 恢复保护的内容
func (f *MarkdownFormatter) restoreBlocks(text string, markers map[string]string) string {
	result := text
	for marker, original := range markers {
		result = strings.ReplaceAll(result, marker, original)
	}
	return result
}

// additionalFormatting 额外的格式化处理
func (f *MarkdownFormatter) additionalFormatting(text string, opts FormatOptions) string {
	// 确保标题前后有空行
	text = f.formatHeadings(text)

	// 格式化列表
	text = f.formatLists(text)

	// 格式化表格
	text = f.formatTables(text)

	// 清理多余的空行
	if !opts.PreserveWhitespace {
		text = f.cleanExtraEmptyLines(text)
	}

	// 确保文件以换行符结束
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}

	return text
}

// formatHeadings 格式化标题
func (f *MarkdownFormatter) formatHeadings(text string) string {
	lines := strings.Split(text, "\n")
	var result []string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 检查是否是标题
		if strings.HasPrefix(trimmed, "#") {
			// 确保标题前有空行（除非是文件开头）
			if i > 0 && len(result) > 0 && result[len(result)-1] != "" {
				result = append(result, "")
			}

			// 添加标题
			result = append(result, line)

			// 确保标题后有空行（除非是文件结尾）
			if i < len(lines)-1 {
				if i+1 < len(lines) && lines[i+1] != "" {
					result = append(result, "")
				}
			}
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// formatLists 格式化列表
func (f *MarkdownFormatter) formatLists(text string) string {
	// 统一列表标记
	lines := strings.Split(text, "\n")
	var result []string
	inList := false

	for _, line := range lines {
		// 检查是否是无序列表项
		if matched, _ := regexp.MatchString(`^\s*[-*+]\s+`, line); matched {
			// 统一使用 - 作为无序列表标记
			line = regexp.MustCompile(`^(\s*)[-*+](\s+)`).ReplaceAllString(line, "$1-$2")
			inList = true
		} else if matched, _ := regexp.MatchString(`^\s*\d+\.\s+`, line); matched {
			// 有序列表
			inList = true
		} else if inList && strings.TrimSpace(line) == "" {
			// 列表结束
			inList = false
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// formatTables 格式化表格
func (f *MarkdownFormatter) formatTables(text string) string {
	// 简单的表格对齐（这里只是占位，实际实现可能需要更复杂的逻辑）
	// markdownfmt 应该已经处理了大部分表格格式化
	return text
}

// cleanExtraEmptyLines 清理多余的空行
func (f *MarkdownFormatter) cleanExtraEmptyLines(text string) string {
	// 将连续的多个空行替换为两个空行
	re := regexp.MustCompile(`\n{3,}`)
	text = re.ReplaceAllString(text, "\n\n")

	// 移除文件开头的空行
	text = strings.TrimLeft(text, "\n")

	// 确保文件结尾只有一个换行符
	text = strings.TrimRight(text, "\n") + "\n"

	return text
}

// FormatWithOptions 使用特定选项格式化
func (f *MarkdownFormatter) FormatWithOptions(content []byte, preserveCodeBlocks, preserveLinks bool) ([]byte, error) {
	opts := DefaultFormatOptions()

	if !preserveCodeBlocks {
		// 移除代码块保护
		var filtered []PreserveBlock
		for _, block := range opts.PreserveBlocks {
			if block.Type != "code" {
				filtered = append(filtered, block)
			}
		}
		opts.PreserveBlocks = filtered
	}

	if preserveLinks {
		// 添加链接保护
		opts.PreserveBlocks = append(opts.PreserveBlocks, PreserveBlock{
			Type:    "link",
			Pattern: `\[([^\]]+)\]\(([^)]+)\)`,
		})
	}

	return f.FormatBytes(content, opts)
}

// GetMetadata 返回格式化器元数据
func (f *MarkdownFormatter) GetMetadata() FormatterMetadata {
	return FormatterMetadata{
		Name:        "markdown",
		Type:        "internal",
		Description: "Markdown formatter using markdownfmt library",
		Formats:     []string{"markdown", "md"},
		Priority:    50,
	}
}
