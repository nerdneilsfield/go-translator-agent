package translation

import (
	"regexp"
	"strings"
)

// RemoveReasoningProcessV2 移除推理模型输出中的思考过程
// 支持多种格式的推理标记（增强版本）
func RemoveReasoningProcessV2(content string, tags []string) string {
	if len(tags) == 0 {
		// 如果没有指定标签，尝试自动检测常见的推理标记
		return removeCommonReasoningTags(content)
	}

	// 如果指定了标签，使用指定的标签对
	if len(tags) >= 2 {
		startTag := regexp.QuoteMeta(tags[0])
		endTag := regexp.QuoteMeta(tags[1])

		// 构建正则表达式，支持跨行匹配
		pattern := startTag + `(?s:.*?)` + endTag
		re := regexp.MustCompile(pattern)

		// 移除所有匹配的推理过程
		result := re.ReplaceAllString(content, "")

		// 清理多余的空行
		return cleanupEmptyLines(result)
	}

	return content
}

// removeCommonReasoningTags 移除常见的推理标记
func removeCommonReasoningTags(content string) string {
	// 常见的推理标记模式
	patterns := []struct {
		start string
		end   string
	}{
		{"<think>", "</think>"},
		{"<thinking>", "</thinking>"},
		{"<thought>", "</thought>"},
		{"<reasoning>", "</reasoning>"},
		{"<reflection>", "</reflection>"},
		{"<internal>", "</internal>"},
		{"[THINKING]", "[/THINKING]"},
		{"[REASONING]", "[/REASONING]"},
		{"```thinking", "```"},
		{"```reasoning", "```"},
	}

	result := content
	for _, p := range patterns {
		startTag := regexp.QuoteMeta(p.start)
		endTag := regexp.QuoteMeta(p.end)

		// 构建正则表达式
		pattern := startTag + `(?s:.*?)` + endTag
		re := regexp.MustCompile(pattern)

		// 移除匹配的内容
		result = re.ReplaceAllString(result, "")
	}

	// 移除特殊的 Markdown 代码块格式的推理过程
	result = removeMarkdownReasoningBlocks(result)

	return cleanupEmptyLines(result)
}

// removeMarkdownReasoningBlocks 移除 Markdown 代码块格式的推理过程
func removeMarkdownReasoningBlocks(content string) string {
	// 匹配以 thinking、reasoning 等标记的代码块
	pattern := `(?m)^` + "```" + `(?:thinking|reasoning|thought|reflection|internal).*?\n(?s:.*?)^` + "```" + `.*?$`
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(content, "")
}

// cleanupEmptyLines 清理多余的空行
func cleanupEmptyLines(content string) string {
	// 移除开头和结尾的空白
	content = strings.TrimSpace(content)

	// 将多个连续空行替换为最多两个空行
	re := regexp.MustCompile(`\n{3,}`)
	content = re.ReplaceAllString(content, "\n\n")

	// 分割行，但不清理每行的空白（保留代码块的缩进）
	lines := strings.Split(content, "\n")

	// 只处理连续的空行，不改变行内容
	var result []string
	var lastWasEmpty bool
	inCodeBlock := false

	for _, line := range lines {
		// 检测代码块的开始和结束
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCodeBlock = !inCodeBlock
		}

		// 在代码块内，保留所有内容
		if inCodeBlock {
			result = append(result, line)
			lastWasEmpty = false
		} else {
			// 在代码块外，处理空行
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine == "" {
				if !lastWasEmpty {
					result = append(result, "")
					lastWasEmpty = true
				}
			} else {
				// 非空行，只移除末尾空白
				result = append(result, strings.TrimRight(line, " \t"))
				lastWasEmpty = false
			}
		}
	}

	return strings.Join(result, "\n")
}

// HasReasoningTags 检查内容是否包含推理标记
func HasReasoningTags(content string) bool {
	// 检查常见的推理标记
	commonTags := []string{
		"<think>", "</think>",
		"<thinking>", "</thinking>",
		"<thought>", "</thought>",
		"<reasoning>", "</reasoning>",
		"<reflection>", "</reflection>",
		"<internal>", "</internal>",
		"[THINKING]", "[/THINKING]",
		"[REASONING]", "[/REASONING]",
	}

	contentLower := strings.ToLower(content)
	for _, tag := range commonTags {
		if strings.Contains(contentLower, strings.ToLower(tag)) {
			return true
		}
	}

	// 检查 Markdown 代码块格式
	if regexp.MustCompile(`(?m)^` + "```" + `(?:thinking|reasoning|thought|reflection|internal)`).MatchString(content) {
		return true
	}

	return false
}

// ExtractReasoningProcess 提取推理过程（用于调试或分析）
func ExtractReasoningProcess(content string, tags []string) []string {
	var reasoningParts []string

	if len(tags) >= 2 {
		startTag := regexp.QuoteMeta(tags[0])
		endTag := regexp.QuoteMeta(tags[1])

		// 构建正则表达式
		pattern := startTag + `((?s:.*?))` + endTag
		re := regexp.MustCompile(pattern)

		// 查找所有匹配
		matches := re.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) > 1 {
				reasoningParts = append(reasoningParts, strings.TrimSpace(match[1]))
			}
		}
	} else {
		// 尝试提取常见格式的推理过程
		reasoningParts = extractCommonReasoningParts(content)
	}

	return reasoningParts
}

// extractCommonReasoningParts 提取常见格式的推理部分
func extractCommonReasoningParts(content string) []string {
	var parts []string

	patterns := []struct {
		start string
		end   string
	}{
		{"<think>", "</think>"},
		{"<thinking>", "</thinking>"},
		{"<thought>", "</thought>"},
		{"<reasoning>", "</reasoning>"},
	}

	for _, p := range patterns {
		startTag := regexp.QuoteMeta(p.start)
		endTag := regexp.QuoteMeta(p.end)

		pattern := startTag + `((?s:.*?))` + endTag
		re := regexp.MustCompile(pattern)

		matches := re.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) > 1 {
				parts = append(parts, strings.TrimSpace(match[1]))
			}
		}
	}

	return parts
}
