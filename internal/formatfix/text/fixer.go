package text

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/nerdneilsfield/go-translator-agent/internal/formatfix"
	"go.uber.org/zap"
)

// TextFixer 文本格式修复器
type TextFixer struct {
	toolManager formatfix.ExternalTool
	toolChecker formatfix.ToolChecker
	logger      *zap.Logger
}

// NewTextFixer 创建文本修复器
func NewTextFixer(toolManager formatfix.ExternalTool, toolChecker formatfix.ToolChecker, logger *zap.Logger) *TextFixer {
	return &TextFixer{
		toolManager: toolManager,
		toolChecker: toolChecker,
		logger:      logger,
	}
}

// GetName 返回修复器名称
func (tf *TextFixer) GetName() string {
	return "Text Fixer"
}

// GetSupportedFormats 返回支持的文件格式
func (tf *TextFixer) GetSupportedFormats() []string {
	return []string{"text", "txt"}
}

// CheckIssues 检查格式问题，但不修复
func (tf *TextFixer) CheckIssues(content []byte) ([]*formatfix.FixIssue, error) {
	var issues []*formatfix.FixIssue

	lines := strings.Split(string(content), "\n")

	for i, line := range lines {
		lineNum := i + 1

		// 检查各种文本问题
		issues = append(issues, tf.checkTrailingWhitespace(line, lineNum)...)
		issues = append(issues, tf.checkTabUsage(line, lineNum)...)
		issues = append(issues, tf.checkMixedLineEndings(line, lineNum)...)
		issues = append(issues, tf.checkLongLines(line, lineNum)...)
		issues = append(issues, tf.checkEncodingIssues(line, lineNum)...)
	}

	// 检查全文问题
	issues = append(issues, tf.checkConsecutiveBlankLines(lines)...)
	issues = append(issues, tf.checkFileEnding(lines)...)
	issues = append(issues, tf.checkOCRErrors(lines)...)

	return issues, nil
}

// PreTranslationFix 翻译前修复
func (tf *TextFixer) PreTranslationFix(ctx context.Context, content []byte, interactor formatfix.UserInteractor) ([]byte, []*formatfix.FixIssue, error) {
	return tf.fixWithInteraction(ctx, content, interactor, true)
}

// PostTranslationFix 翻译后修复
func (tf *TextFixer) PostTranslationFix(ctx context.Context, content []byte, interactor formatfix.UserInteractor) ([]byte, []*formatfix.FixIssue, error) {
	return tf.fixWithInteraction(ctx, content, interactor, false)
}

// AutoFix 自动修复所有可修复的问题
func (tf *TextFixer) AutoFix(content []byte) ([]byte, []*formatfix.FixIssue, error) {
	issues, err := tf.CheckIssues(content)
	if err != nil {
		return content, nil, err
	}

	var fixedIssues []*formatfix.FixIssue
	result := string(content)

	// 按优先级排序并修复
	for _, issue := range issues {
		if issue.CanAutoFix {
			result = strings.ReplaceAll(result, issue.OriginalText, issue.FixedText)
			fixedIssues = append(fixedIssues, issue)
		}
	}

	return []byte(result), fixedIssues, nil
}

// fixWithInteraction 交互式修复
func (tf *TextFixer) fixWithInteraction(ctx context.Context, content []byte, interactor formatfix.UserInteractor, isPreTranslation bool) ([]byte, []*formatfix.FixIssue, error) {
	issues, err := tf.CheckIssues(content)
	if err != nil {
		return content, nil, err
	}

	if len(issues) == 0 {
		return content, nil, nil
	}

	var fixedIssues []*formatfix.FixIssue
	var skippedIssues []*formatfix.FixIssue
	result := string(content)

	for i, issue := range issues {
		interactor.ShowProgress(i+1, len(issues), issue.Type)

		action := interactor.ConfirmFix(issue)

		switch action {
		case formatfix.FixActionApply:
			if issue.CanAutoFix {
				result = strings.ReplaceAll(result, issue.OriginalText, issue.FixedText)
				fixedIssues = append(fixedIssues, issue)
			}
		case formatfix.FixActionSkip:
			skippedIssues = append(skippedIssues, issue)
		case formatfix.FixActionAbort:
			interactor.ShowSummary(len(fixedIssues), len(skippedIssues), issues)
			return []byte(result), fixedIssues, nil
		}
	}

	interactor.ShowSummary(len(fixedIssues), len(skippedIssues), issues)

	return []byte(result), fixedIssues, nil
}

// checkTrailingWhitespace 检查行尾空白字符
func (tf *TextFixer) checkTrailingWhitespace(line string, lineNum int) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	if len(line) > 0 && unicode.IsSpace(rune(line[len(line)-1])) {
		trimmed := strings.TrimRightFunc(line, unicode.IsSpace)
		issues = append(issues, &formatfix.FixIssue{
			Type:         "TRAILING_WHITESPACE",
			Severity:     formatfix.SeverityWarning,
			Line:         lineNum,
			Column:       len(trimmed) + 1,
			Message:      "行尾存在空白字符",
			Suggestion:   "删除行尾空白字符",
			CanAutoFix:   true,
			OriginalText: line,
			FixedText:    trimmed,
		})
	}

	return issues
}

// checkTabUsage 检查 Tab 使用
func (tf *TextFixer) checkTabUsage(line string, lineNum int) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	if strings.Contains(line, "\t") {
		tabCount := strings.Count(line, "\t")
		fixed := strings.ReplaceAll(line, "\t", "    ") // 替换为 4 个空格

		issues = append(issues, &formatfix.FixIssue{
			Type:         "TAB_CHARACTER",
			Severity:     formatfix.SeverityInfo,
			Line:         lineNum,
			Column:       strings.Index(line, "\t") + 1,
			Message:      fmt.Sprintf("发现 %d 个 Tab 字符", tabCount),
			Suggestion:   "将 Tab 替换为空格",
			CanAutoFix:   true,
			OriginalText: line,
			FixedText:    fixed,
		})
	}

	return issues
}

// checkMixedLineEndings 检查混合行结束符
func (tf *TextFixer) checkMixedLineEndings(line string, lineNum int) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	if strings.Contains(line, "\r") {
		fixed := strings.ReplaceAll(line, "\r", "")
		issues = append(issues, &formatfix.FixIssue{
			Type:         "MIXED_LINE_ENDINGS",
			Severity:     formatfix.SeverityWarning,
			Line:         lineNum,
			Column:       1,
			Message:      "发现 Windows 风格的行结束符 (\\r\\n)",
			Suggestion:   "统一使用 Unix 风格的行结束符 (\\n)",
			CanAutoFix:   true,
			OriginalText: line,
			FixedText:    fixed,
		})
	}

	return issues
}

// checkLongLines 检查过长的行
func (tf *TextFixer) checkLongLines(line string, lineNum int) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	const maxLineLength = 100
	if len(line) > maxLineLength {
		issues = append(issues, &formatfix.FixIssue{
			Type:         "LONG_LINE",
			Severity:     formatfix.SeverityInfo,
			Line:         lineNum,
			Column:       maxLineLength + 1,
			Message:      fmt.Sprintf("行长度 %d 超过建议的 %d 字符", len(line), maxLineLength),
			Suggestion:   "考虑在适当位置换行",
			CanAutoFix:   false, // 需要用户判断如何换行
			OriginalText: line,
			FixedText:    "",
		})
	}

	return issues
}

// checkEncodingIssues 检查编码问题
func (tf *TextFixer) checkEncodingIssues(line string, lineNum int) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	// 检查可能的编码问题字符
	problematicChars := []string{
		"\ufffd", // 替换字符（编码错误）
		"\u00a0", // 不间断空格
		"\u2028", // 行分隔符
		"\u2029", // 段分隔符
	}

	for _, char := range problematicChars {
		if strings.Contains(line, char) {
			var suggestion string
			var fixed string

			switch char {
			case "\ufffd":
				suggestion = "修复编码错误"
				fixed = strings.ReplaceAll(line, char, "?")
			case "\u00a0":
				suggestion = "将不间断空格替换为普通空格"
				fixed = strings.ReplaceAll(line, char, " ")
			case "\u2028", "\u2029":
				suggestion = "移除特殊行分隔符"
				fixed = strings.ReplaceAll(line, char, "")
			}

			issues = append(issues, &formatfix.FixIssue{
				Type:         "ENCODING_ISSUE",
				Severity:     formatfix.SeverityWarning,
				Line:         lineNum,
				Column:       strings.Index(line, char) + 1,
				Message:      "发现可能的编码问题字符",
				Suggestion:   suggestion,
				CanAutoFix:   true,
				OriginalText: line,
				FixedText:    fixed,
			})
		}
	}

	return issues
}

// checkConsecutiveBlankLines 检查连续空行
func (tf *TextFixer) checkConsecutiveBlankLines(lines []string) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	consecutiveBlankLines := 0
	startLine := 0

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			if consecutiveBlankLines == 0 {
				startLine = i + 1
			}
			consecutiveBlankLines++
		} else {
			if consecutiveBlankLines > 2 { // 允许最多 2 个连续空行
				issues = append(issues, &formatfix.FixIssue{
					Type:         "CONSECUTIVE_BLANK_LINES",
					Severity:     formatfix.SeverityInfo,
					Line:         startLine,
					Column:       1,
					Message:      fmt.Sprintf("发现 %d 个连续空行", consecutiveBlankLines),
					Suggestion:   "减少连续空行数量",
					CanAutoFix:   true,
					OriginalText: strings.Repeat("\n", consecutiveBlankLines),
					FixedText:    "\n\n", // 最多保留 2 个空行
				})
			}
			consecutiveBlankLines = 0
		}
	}

	return issues
}

// checkFileEnding 检查文件结尾
func (tf *TextFixer) checkFileEnding(lines []string) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	if len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		if lastLine != "" {
			issues = append(issues, &formatfix.FixIssue{
				Type:         "NO_FINAL_NEWLINE",
				Severity:     formatfix.SeverityInfo,
				Line:         len(lines),
				Column:       len(lastLine) + 1,
				Message:      "文件末尾缺少换行符",
				Suggestion:   "在文件末尾添加换行符",
				CanAutoFix:   true,
				OriginalText: lastLine,
				FixedText:    lastLine + "\n",
			})
		}
	}

	return issues
}

// checkOCRErrors 检查 OCR 常见错误
func (tf *TextFixer) checkOCRErrors(lines []string) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	// OCR 常见字符识别错误
	ocrErrors := map[string]string{
		"rn": "m", // rn 被识别为 m
		"vv": "w", // vv 被识别为 w
		"1":  "l", // 数字1 被识别为字母 l
		"0":  "O", // 数字0 被识别为字母 O
		"5":  "S", // 数字5 被识别为字母 S
		"8":  "B", // 数字8 被识别为字母 B
	}

	for i, line := range lines {
		for wrong, correct := range ocrErrors {
			// 简单的模式匹配，实际应该更智能
			if tf.containsLikelyOCRError(line, wrong, correct) {
				issues = append(issues, &formatfix.FixIssue{
					Type:         "OCR_ERROR",
					Severity:     formatfix.SeverityInfo,
					Line:         i + 1,
					Column:       strings.Index(line, wrong) + 1,
					Message:      fmt.Sprintf("可能的 OCR 识别错误: '%s' 应该是 '%s'", wrong, correct),
					Suggestion:   fmt.Sprintf("检查是否应该将 '%s' 替换为 '%s'", wrong, correct),
					CanAutoFix:   false, // 需要用户确认
					OriginalText: line,
					FixedText:    "",
				})
			}
		}
	}

	return issues
}

// containsLikelyOCRError 检查是否包含可能的 OCR 错误
func (tf *TextFixer) containsLikelyOCRError(line, wrong, correct string) bool {
	// 简化实现：检查上下文来判断是否是 OCR 错误
	// 实际实现应该更复杂，考虑单词边界、语言模型等

	// 例如，如果 "rn" 出现在单词中间，更可能是 OCR 错误
	rnPattern := regexp.MustCompile(`\w+rn\w+`)
	if wrong == "rn" && rnPattern.MatchString(line) {
		return true
	}

	// 其他错误类型的检查...

	return false
}
