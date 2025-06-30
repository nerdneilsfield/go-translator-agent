package markdown

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/nerdneilsfield/go-translator-agent/internal/formatfix"
	"go.uber.org/zap"
)

// MarkdownFixer Markdown 格式修复器
type MarkdownFixer struct {
	toolManager formatfix.ExternalTool
	toolChecker formatfix.ToolChecker
	logger      *zap.Logger
}

// NewMarkdownFixer 创建 Markdown 修复器
func NewMarkdownFixer(toolManager formatfix.ExternalTool, toolChecker formatfix.ToolChecker, logger *zap.Logger) *MarkdownFixer {
	return &MarkdownFixer{
		toolManager: toolManager,
		toolChecker: toolChecker,
		logger:      logger,
	}
}

// GetName 返回修复器名称
func (mf *MarkdownFixer) GetName() string {
	return "Markdown Fixer"
}

// GetSupportedFormats 返回支持的文件格式
func (mf *MarkdownFixer) GetSupportedFormats() []string {
	return []string{"markdown", "md"}
}

// CheckIssues 检查格式问题，但不修复
func (mf *MarkdownFixer) CheckIssues(content []byte) ([]*formatfix.FixIssue, error) {
	var issues []*formatfix.FixIssue

	lines := strings.Split(string(content), "\n")

	for i, line := range lines {
		lineNum := i + 1

		// 检查各种问题
		issues = append(issues, mf.checkHeadingSpacing(line, lineNum)...)
		issues = append(issues, mf.checkListFormatting(line, lineNum)...)
		issues = append(issues, mf.checkTabUsage(line, lineNum)...)
		issues = append(issues, mf.checkLinkSyntax(line, lineNum)...)
		issues = append(issues, mf.checkUnescapedCharacters(line, lineNum)...)
		issues = append(issues, mf.checkCodeBlockLanguage(line, lineNum)...)
	}

	// 检查全文问题
	issues = append(issues, mf.checkConsecutiveBlankLines(lines)...)
	issues = append(issues, mf.checkTableFormatting(lines)...)
	issues = append(issues, mf.checkStructureProblems(lines)...)

	return issues, nil
}

// PreTranslationFix 翻译前修复
func (mf *MarkdownFixer) PreTranslationFix(ctx context.Context, content []byte, interactor formatfix.UserInteractor) ([]byte, []*formatfix.FixIssue, error) {
	return mf.fixWithInteraction(ctx, content, interactor, true)
}

// PostTranslationFix 翻译后修复
func (mf *MarkdownFixer) PostTranslationFix(ctx context.Context, content []byte, interactor formatfix.UserInteractor) ([]byte, []*formatfix.FixIssue, error) {
	return mf.fixWithInteraction(ctx, content, interactor, false)
}

// AutoFix 自动修复所有可修复的问题
func (mf *MarkdownFixer) AutoFix(content []byte) ([]byte, []*formatfix.FixIssue, error) {
	// 首先尝试使用外部工具
	if result, issues, err := mf.tryExternalTools(content); err == nil && len(issues) > 0 {
		return result, issues, nil
	}

	// 回退到内部实现
	issues, err := mf.CheckIssues(content)
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

// tryExternalTools 尝试使用外部工具进行修复
func (mf *MarkdownFixer) tryExternalTools(content []byte) ([]byte, []*formatfix.FixIssue, error) {
	var result []byte = content
	var allIssues []*formatfix.FixIssue

	// 尝试使用 markdownlint
	if mf.toolChecker.IsToolAvailable("markdownlint") {
		if lintResult, issues, err := mf.runMarkdownlint(content); err == nil {
			result = lintResult
			allIssues = append(allIssues, issues...)
		}
	}

	// 尝试使用 prettier
	if mf.toolChecker.IsToolAvailable("prettier") {
		if prettyResult, issues, err := mf.runPrettier(result); err == nil {
			result = prettyResult
			allIssues = append(allIssues, issues...)
		}
	}

	if len(allIssues) == 0 {
		return content, nil, fmt.Errorf("no external tools available or successful")
	}

	return result, allIssues, nil
}

// runMarkdownlint 运行 markdownlint 工具
func (mf *MarkdownFixer) runMarkdownlint(content []byte) ([]byte, []*formatfix.FixIssue, error) {
	mf.logger.Debug("运行 markdownlint")

	// 创建临时文件
	tmpFile, err := mf.createTempFile(content, ".md")
	if err != nil {
		return content, nil, err
	}
	defer mf.cleanupTempFile(tmpFile)

	// 运行 markdownlint --fix
	stdout, _, err := mf.toolManager.Execute("markdownlint", []string{"--fix", tmpFile}, nil)
	result := string(stdout)
	if err != nil {
		mf.logger.Warn("markdownlint 执行失败", zap.Error(err))
		return content, nil, err
	}

	// 读取修复后的内容
	fixedContent, err := mf.readTempFile(tmpFile)
	if err != nil {
		return content, nil, err
	}

	// 创建修复问题记录
	issue := &formatfix.FixIssue{
		Type:         "MARKDOWNLINT",
		Severity:     formatfix.SeverityInfo,
		Line:         0,
		Column:       0,
		Message:      "使用 markdownlint 进行格式修复",
		Suggestion:   "应用 markdownlint 的建议修复",
		CanAutoFix:   true,
		OriginalText: string(content),
		FixedText:    string(fixedContent),
	}

	if result != "" {
		mf.logger.Info("markdownlint 输出", zap.String("output", result))
	}

	return fixedContent, []*formatfix.FixIssue{issue}, nil
}

// runPrettier 运行 prettier 工具
func (mf *MarkdownFixer) runPrettier(content []byte) ([]byte, []*formatfix.FixIssue, error) {
	mf.logger.Debug("运行 prettier")

	// 创建临时文件
	tmpFile, err := mf.createTempFile(content, ".md")
	if err != nil {
		return content, nil, err
	}
	defer mf.cleanupTempFile(tmpFile)

	// 运行 prettier --write
	stdout, _, err := mf.toolManager.Execute("prettier", []string{"--write", "--parser", "markdown", tmpFile}, nil)
	result := string(stdout)
	if err != nil {
		mf.logger.Warn("prettier 执行失败", zap.Error(err))
		return content, nil, err
	}

	// 读取格式化后的内容
	formattedContent, err := mf.readTempFile(tmpFile)
	if err != nil {
		return content, nil, err
	}

	// 创建修复问题记录
	issue := &formatfix.FixIssue{
		Type:         "PRETTIER",
		Severity:     formatfix.SeverityInfo,
		Line:         0,
		Column:       0,
		Message:      "使用 prettier 进行格式化",
		Suggestion:   "应用 prettier 的格式化规则",
		CanAutoFix:   true,
		OriginalText: string(content),
		FixedText:    string(formattedContent),
	}

	if result != "" {
		mf.logger.Info("prettier 输出", zap.String("output", result))
	}

	return formattedContent, []*formatfix.FixIssue{issue}, nil
}

// fixWithInteraction 交互式修复
func (mf *MarkdownFixer) fixWithInteraction(ctx context.Context, content []byte, interactor formatfix.UserInteractor, isPreTranslation bool) ([]byte, []*formatfix.FixIssue, error) {
	issues, err := mf.CheckIssues(content)
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

// checkHeadingSpacing 检查标题格式（# 后面需要空格）
func (mf *MarkdownFixer) checkHeadingSpacing(line string, lineNum int) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	// 匹配 # 开头但后面没有空格的标题
	headingRegex := regexp.MustCompile(`^(#{1,6})([^#\s].*)$`)
	if match := headingRegex.FindStringSubmatch(line); match != nil {
		issues = append(issues, &formatfix.FixIssue{
			Type:         "MD018",
			Severity:     formatfix.SeverityWarning,
			Line:         lineNum,
			Column:       len(match[1]) + 1,
			Message:      "标题井号后缺少空格",
			Suggestion:   "在 # 后面添加空格",
			CanAutoFix:   true,
			OriginalText: line,
			FixedText:    match[1] + " " + match[2],
		})
	}

	return issues
}

// checkListFormatting 检查列表格式
func (mf *MarkdownFixer) checkListFormatting(line string, lineNum int) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	// 检查列表项后面没有空格的情况
	listRegex := regexp.MustCompile(`^(\s*)([-*+])([^\s].*)$`)
	if match := listRegex.FindStringSubmatch(line); match != nil {
		issues = append(issues, &formatfix.FixIssue{
			Type:         "MD030",
			Severity:     formatfix.SeverityWarning,
			Line:         lineNum,
			Column:       len(match[1]) + len(match[2]) + 1,
			Message:      "列表项标记后缺少空格",
			Suggestion:   "在列表标记后添加空格",
			CanAutoFix:   true,
			OriginalText: line,
			FixedText:    match[1] + match[2] + " " + match[3],
		})
	}

	// 检查有序列表格式
	orderedListRegex := regexp.MustCompile(`^(\s*)(\d+\.)([^\s].*)$`)
	if match := orderedListRegex.FindStringSubmatch(line); match != nil {
		issues = append(issues, &formatfix.FixIssue{
			Type:         "MD030",
			Severity:     formatfix.SeverityWarning,
			Line:         lineNum,
			Column:       len(match[1]) + len(match[2]) + 1,
			Message:      "有序列表项后缺少空格",
			Suggestion:   "在数字后添加空格",
			CanAutoFix:   true,
			OriginalText: line,
			FixedText:    match[1] + match[2] + " " + match[3],
		})
	}

	return issues
}

// checkTabUsage 检查 Tab 使用
func (mf *MarkdownFixer) checkTabUsage(line string, lineNum int) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	if strings.Contains(line, "\t") {
		// 计算 Tab 位置
		tabCount := strings.Count(line, "\t")
		fixed := strings.ReplaceAll(line, "\t", "    ") // 替换为 4 个空格

		issues = append(issues, &formatfix.FixIssue{
			Type:         "MD010",
			Severity:     formatfix.SeverityWarning,
			Line:         lineNum,
			Column:       strings.Index(line, "\t") + 1,
			Message:      fmt.Sprintf("发现 %d 个 Tab 字符，建议使用空格", tabCount),
			Suggestion:   "将 Tab 替换为空格（通常是 4 个空格）",
			CanAutoFix:   true,
			OriginalText: line,
			FixedText:    fixed,
		})
	}

	return issues
}

// checkLinkSyntax 检查链接语法
func (mf *MarkdownFixer) checkLinkSyntax(line string, lineNum int) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	// 检查错误的链接语法：(文字)[链接] 应该是 [文字](链接)
	wrongLinkRegex := regexp.MustCompile(`\(([^)]+)\)\[([^\]]+)\]`)
	matches := wrongLinkRegex.FindAllStringSubmatch(line, -1)

	for _, match := range matches {
		issues = append(issues, &formatfix.FixIssue{
			Type:         "MD011",
			Severity:     formatfix.SeverityError,
			Line:         lineNum,
			Column:       strings.Index(line, match[0]) + 1,
			Message:      "链接语法错误",
			Suggestion:   "正确的链接语法是 [文字](链接)",
			CanAutoFix:   true,
			OriginalText: match[0],
			FixedText:    fmt.Sprintf("[%s](%s)", match[1], match[2]),
		})
	}

	return issues
}

// checkUnescapedCharacters 检查未转义的特殊字符
func (mf *MarkdownFixer) checkUnescapedCharacters(line string, lineNum int) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	// 需要转义的字符（在普通文本中出现时）
	specialChars := []string{"*", "_", "[", "]", "(", ")", "#", "`", "\\"}

	// 简单检查：如果行中包含这些字符但不在代码块或链接中
	// 这里实现一个简化版本，实际应该更复杂
	for _, char := range specialChars {
		if strings.Contains(line, char) && !mf.isInCodeBlock(line) && !mf.isInLink(line) {
			// 这里只是示例，实际实现需要更精确的检测
			if char == "*" && mf.hasUnpairedAsterisks(line) {
				issues = append(issues, &formatfix.FixIssue{
					Type:         "MD037",
					Severity:     formatfix.SeverityInfo,
					Line:         lineNum,
					Column:       strings.Index(line, char) + 1,
					Message:      "可能需要转义的特殊字符",
					Suggestion:   fmt.Sprintf("如果 '%s' 不用于格式化，请使用反斜杠转义", char),
					CanAutoFix:   false, // 需要用户判断
					OriginalText: line,
					FixedText:    "", // 不自动修复
				})
			}
		}
	}

	return issues
}

// checkCodeBlockLanguage 检查代码块语言标识
func (mf *MarkdownFixer) checkCodeBlockLanguage(line string, lineNum int) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	// 检查空的代码块标识
	if strings.TrimSpace(line) == "```" {
		issues = append(issues, &formatfix.FixIssue{
			Type:         "MD040",
			Severity:     formatfix.SeverityInfo,
			Line:         lineNum,
			Column:       1,
			Message:      "代码块缺少语言标识",
			Suggestion:   "添加语言标识，如 ```python, ```javascript 等",
			CanAutoFix:   false, // 需要用户指定语言
			OriginalText: line,
			FixedText:    "",
		})
	}

	return issues
}

// checkConsecutiveBlankLines 检查连续空行
func (mf *MarkdownFixer) checkConsecutiveBlankLines(lines []string) []*formatfix.FixIssue {
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
			if consecutiveBlankLines > 1 {
				issues = append(issues, &formatfix.FixIssue{
					Type:         "MD012",
					Severity:     formatfix.SeverityWarning,
					Line:         startLine,
					Column:       1,
					Message:      fmt.Sprintf("发现 %d 个连续空行", consecutiveBlankLines),
					Suggestion:   "最多保留一个空行",
					CanAutoFix:   true,
					OriginalText: strings.Repeat("\n", consecutiveBlankLines),
					FixedText:    "\n",
				})
			}
			consecutiveBlankLines = 0
		}
	}

	return issues
}

// checkTableFormatting 检查表格格式
func (mf *MarkdownFixer) checkTableFormatting(lines []string) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	for i, line := range lines {
		// 简单的表格行检测（包含 |）
		if strings.Contains(line, "|") && strings.Count(line, "|") >= 2 {
			lineNum := i + 1

			// 检查表格分隔符行
			if strings.Contains(line, "---") || strings.Contains(line, ":--") || strings.Contains(line, "--:") {
				// 检查分隔符格式
				if !mf.isValidTableSeparator(line) {
					issues = append(issues, &formatfix.FixIssue{
						Type:         "MD055",
						Severity:     formatfix.SeverityWarning,
						Line:         lineNum,
						Column:       1,
						Message:      "表格分隔符格式不正确",
						Suggestion:   "确保分隔符至少有三个连字符",
						CanAutoFix:   false,
						OriginalText: line,
						FixedText:    "",
					})
				}
			}
		}
	}

	return issues
}

// checkStructureProblems 检查结构问题（OCR 常见问题）
func (mf *MarkdownFixer) checkStructureProblems(lines []string) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	// 检查标题层级跳跃
	var lastHeadingLevel int
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			level := mf.getHeadingLevel(line)
			if level > 0 {
				if lastHeadingLevel > 0 && level > lastHeadingLevel+1 {
					issues = append(issues, &formatfix.FixIssue{
						Type:         "MD001",
						Severity:     formatfix.SeverityWarning,
						Line:         i + 1,
						Column:       1,
						Message:      fmt.Sprintf("标题层级跳跃：从 %d 级跳到 %d 级", lastHeadingLevel, level),
						Suggestion:   "避免跳过标题层级",
						CanAutoFix:   false,
						OriginalText: line,
						FixedText:    "",
					})
				}
				lastHeadingLevel = level
			}
		}
	}

	// 检查可能的阅读顺序问题（OCR 特有）
	issues = append(issues, mf.checkReadingOrderIssues(lines)...)

	return issues
}

// 辅助方法

// isInCodeBlock 检查是否在代码块中
func (mf *MarkdownFixer) isInCodeBlock(line string) bool {
	// 简化实现：检查是否包含反引号
	return strings.Contains(line, "`")
}

// isInLink 检查是否在链接中
func (mf *MarkdownFixer) isInLink(line string) bool {
	// 简化实现：检查是否包含链接语法
	return strings.Contains(line, "[") && strings.Contains(line, "]")
}

// hasUnpairedAsterisks 检查是否有未配对的星号
func (mf *MarkdownFixer) hasUnpairedAsterisks(line string) bool {
	count := strings.Count(line, "*")
	return count%2 != 0
}

// isValidTableSeparator 检查表格分隔符是否有效
func (mf *MarkdownFixer) isValidTableSeparator(line string) bool {
	// 简化检查：至少包含 --- 或类似格式
	return regexp.MustCompile(`^[|:\s-]+$`).MatchString(strings.TrimSpace(line))
}

// getHeadingLevel 获取标题级别
func (mf *MarkdownFixer) getHeadingLevel(line string) int {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return 0
	}

	count := 0
	for _, char := range trimmed {
		if char == '#' {
			count++
		} else {
			break
		}
	}

	return count
}

// checkReadingOrderIssues 检查阅读顺序问题（OCR 特有）
func (mf *MarkdownFixer) checkReadingOrderIssues(lines []string) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	// 检查可能的段落分割问题
	for i := 0; i < len(lines)-1; i++ {
		currentLine := strings.TrimSpace(lines[i])
		nextLine := strings.TrimSpace(lines[i+1])

		// 如果当前行以句号结尾，下一行以大写字母开头，但它们被不当地分割了
		if len(currentLine) > 0 && len(nextLine) > 0 {
			if strings.HasSuffix(currentLine, ".") &&
				unicode.IsUpper(rune(nextLine[0])) &&
				!strings.HasPrefix(nextLine, "#") &&
				!mf.looksLikeListItem(nextLine) {

				// 这可能是 OCR 错误分割的段落
				issues = append(issues, &formatfix.FixIssue{
					Type:         "OCR_SPLIT",
					Severity:     formatfix.SeverityInfo,
					Line:         i + 1,
					Column:       1,
					Message:      "可能的段落错误分割",
					Suggestion:   "检查是否应该合并这些行",
					CanAutoFix:   false,
					OriginalText: currentLine + "\n" + nextLine,
					FixedText:    "",
				})
			}
		}
	}

	// 检查 OCR 常见的内容错误放置
	issues = append(issues, mf.checkContentMisplacement(lines)...)

	// 检查可能的表格或公式识别错误
	issues = append(issues, mf.checkTableFormulaErrors(lines)...)

	return issues
}

// checkContentMisplacement 检查内容错误放置（OCR 常见问题）
func (mf *MarkdownFixer) checkContentMisplacement(lines []string) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 检查可能错误识别的图片标题或说明
		if mf.looksLikeMisplacedCaption(trimmed) {
			issues = append(issues, &formatfix.FixIssue{
				Type:         "OCR_CAPTION",
				Severity:     formatfix.SeverityInfo,
				Line:         i + 1,
				Column:       1,
				Message:      "可能的图片标题或说明错误放置",
				Suggestion:   "检查是否应该将此文本作为图片标题或说明",
				CanAutoFix:   false,
				OriginalText: line,
				FixedText:    "",
			})
		}

		// 检查可能的页眉页脚
		if mf.looksLikeHeaderFooter(trimmed) {
			issues = append(issues, &formatfix.FixIssue{
				Type:         "OCR_HEADER_FOOTER",
				Severity:     formatfix.SeverityInfo,
				Line:         i + 1,
				Column:       1,
				Message:      "可能的页眉或页脚内容",
				Suggestion:   "考虑删除或移动此内容",
				CanAutoFix:   false,
				OriginalText: line,
				FixedText:    "",
			})
		}
	}

	return issues
}

// checkTableFormulaErrors 检查表格和公式识别错误
func (mf *MarkdownFixer) checkTableFormulaErrors(lines []string) []*formatfix.FixIssue {
	var issues []*formatfix.FixIssue

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 检查可能的公式识别错误
		if mf.looksLikeMalformedFormula(trimmed) {
			issues = append(issues, &formatfix.FixIssue{
				Type:         "OCR_FORMULA",
				Severity:     formatfix.SeverityWarning,
				Line:         i + 1,
				Column:       1,
				Message:      "可能的数学公式识别错误",
				Suggestion:   "检查数学公式的格式，考虑使用 LaTeX 语法",
				CanAutoFix:   false,
				OriginalText: line,
				FixedText:    "",
			})
		}

		// 检查表格识别错误
		if mf.looksLikeMalformedTable(trimmed) {
			issues = append(issues, &formatfix.FixIssue{
				Type:         "OCR_TABLE",
				Severity:     formatfix.SeverityWarning,
				Line:         i + 1,
				Column:       1,
				Message:      "可能的表格识别错误",
				Suggestion:   "检查表格格式，确保使用正确的 Markdown 表格语法",
				CanAutoFix:   false,
				OriginalText: line,
				FixedText:    "",
			})
		}
	}

	return issues
}

// looksLikeMisplacedCaption 检查是否看起来像错误放置的标题
func (mf *MarkdownFixer) looksLikeMisplacedCaption(line string) bool {
	// 检查常见的图片标题模式
	captionPatterns := []string{
		`^(图|Figure|Fig\.?)\s*\d+`,
		`^(表|Table)\s*\d+`,
		`^(算法|Algorithm)\s*\d+`,
		`^(公式|Formula|Equation)\s*\d+`,
	}

	for _, pattern := range captionPatterns {
		if matched, _ := regexp.MatchString(pattern, line); matched {
			return true
		}
	}

	return false
}

// looksLikeHeaderFooter 检查是否看起来像页眉页脚
func (mf *MarkdownFixer) looksLikeHeaderFooter(line string) bool {
	// 检查常见的页眉页脚模式
	headerFooterPatterns := []string{
		`^\d+$`,                    // 纯数字（页码）
		`^第\s*\d+\s*页`,             // 中文页码
		`^Page\s*\d+`,              // 英文页码
		`^\d+\s*/\s*\d+$`,          // 页码格式 "1/10"
		`^©\s*\d{4}`,               // 版权信息
		`^www\.|\.com|\.org|\.edu`, // 网址
	}

	for _, pattern := range headerFooterPatterns {
		if matched, _ := regexp.MatchString(pattern, line); matched {
			return true
		}
	}

	return false
}

// looksLikeMalformedFormula 检查是否看起来像格式错误的公式
func (mf *MarkdownFixer) looksLikeMalformedFormula(line string) bool {
	// 检查可能的公式识别错误模式
	formulaPatterns := []string{
		`[a-zA-Z]\s*[=+\-*/]\s*[a-zA-Z0-9].*[=+\-*/]`, // 含有多个运算符的表达式
		`\b[xyz]\d*\s*[=+\-]\s*\d+`,                   // 简单方程
		`∑|∏|∫|√|≤|≥|≠|α|β|γ|θ|λ|μ|π|σ|∞`,             // 数学符号
	}

	for _, pattern := range formulaPatterns {
		if matched, _ := regexp.MatchString(pattern, line); matched {
			return true
		}
	}

	return false
}

// looksLikeMalformedTable 检查是否看起来像格式错误的表格
func (mf *MarkdownFixer) looksLikeMalformedTable(line string) bool {
	// 检查可能的表格错误模式
	if !strings.Contains(line, "|") {
		return false
	}

	// 检查表格行但格式不正确
	cells := strings.Split(line, "|")
	if len(cells) >= 3 { // 至少要有两个单元格内容
		// 检查是否有太多空单元格
		emptyCells := 0
		for _, cell := range cells {
			if strings.TrimSpace(cell) == "" {
				emptyCells++
			}
		}

		// 如果超过一半的单元格是空的，可能是识别错误
		return emptyCells > len(cells)/2
	}

	return false
}

// looksLikeListItem 检查是否看起来像列表项
func (mf *MarkdownFixer) looksLikeListItem(line string) bool {
	trimmed := strings.TrimSpace(line)
	return regexp.MustCompile(`^(\d+\.|[-*+])\s`).MatchString(trimmed)
}

// 临时文件处理方法

// createTempFile 创建临时文件
func (mf *MarkdownFixer) createTempFile(content []byte, suffix string) (string, error) {
	tmpFile, err := ioutil.TempFile("", "markdown-fixer-*"+suffix)
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(content); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}

	return tmpFile.Name(), nil
}

// readTempFile 读取临时文件内容
func (mf *MarkdownFixer) readTempFile(filename string) ([]byte, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取临时文件失败: %w", err)
	}
	return content, nil
}

// cleanupTempFile 清理临时文件
func (mf *MarkdownFixer) cleanupTempFile(filename string) {
	if err := os.Remove(filename); err != nil {
		mf.logger.Warn("清理临时文件失败", zap.String("file", filename), zap.Error(err))
	}
}
