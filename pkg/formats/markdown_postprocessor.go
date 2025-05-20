package formats

import (
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"go.uber.org/zap"
)

// MarkdownPostProcessor 用于处理翻译后的 Markdown 文件
type MarkdownPostProcessor struct {
	config              *config.Config
	logger              *zap.Logger
	currentInputFile    string // 当前处理的文件路径
	currentReplacements []ReplacementInfo
}

// NewMarkdownPostProcessor 创建一个新的 Markdown 后处理器
func NewMarkdownPostProcessor(config *config.Config, logger *zap.Logger) *MarkdownPostProcessor {
	return &MarkdownPostProcessor{
		config: config,
		logger: logger,
	}
}

// SetInputFile 设置当前处理的文件路径
func (p *MarkdownPostProcessor) SetInputFile(filePath string) {
	p.currentInputFile = filePath
}

// ProcessMarkdown 处理翻译后的 Markdown 文本
func (p *MarkdownPostProcessor) ProcessMarkdown(text string, replacements []ReplacementInfo) string {
	p.currentReplacements = replacements

	if !p.config.PostProcessMarkdown {
		return text
	}

	p.logger.Debug("开始后处理Markdown文本")

	// 0. 清理翻译过程中可能引入的提示词标记
	text = p.cleanupPromptTags(text)

	// 1. 先处理引用
	text = p.fixQuotes(text)

	// 2. 处理表格
	text = p.fixTables(text)

	// 3. 处理数学公式
	text = p.fixMathSpaces(text)

	// 4. 处理图片和链接
	text = p.fixLinksAndImages(text)

	// 5. 处理标题
	text = p.fixHeadingSpaces(text)

	// 6. 处理未翻译的占位符
	text = p.fixUntranslatable(text)

	// 7. 修复格式问题
	text = p.fixFormatIssues(text)

	// 7. 使用 Prettier 格式化
	tempFile := p.currentInputFile
	if tempFile == "" {
		tempFile = os.TempDir() + "/markdown_temp_" + time.Now().Format("20060102150405") + ".md"
	} else {
		tempFile = tempFile + ".temp"
	}
	if err := os.WriteFile(tempFile, []byte(text), 0644); err != nil {
		p.logger.Error("创建临时文件失败", zap.Error(err))
		return text
	}
	defer os.Remove(tempFile)

	if err := formatWithPrettier(tempFile); err != nil {
		p.logger.Error("Prettier 格式化失败", zap.Error(err))
		return text
	}

	formatted, err := os.ReadFile(tempFile)
	if err != nil {
		p.logger.Error("读取格式化后的文件失败", zap.Error(err))
		return text
	}

	p.logger.Debug("Markdown后处理完成",
		zap.Int("原始长度", len(text)),
		zap.Int("处理后长度", len(formatted)),
	)

	return string(formatted)
}

// fixQuotes 修复引用格式
func (p *MarkdownPostProcessor) fixQuotes(text string) string {
	// 确保引用符号后有空格
	quoteRegex := regexp.MustCompile(`^(>+)([^\s>])`)
	text = quoteRegex.ReplaceAllString(text, "$1 $2")

	// 确保多级引用格式正确
	multiQuoteRegex := regexp.MustCompile(`^(>+)\s*(>+)\s*`)
	text = multiQuoteRegex.ReplaceAllString(text, "$1$2 ")

	return text
}

// fixTables 修复表格格式
func (p *MarkdownPostProcessor) fixTables(text string) string {
	// 修复表格对齐标记
	alignRegex := regexp.MustCompile(`\|[\s-]*:?-+:?[\s-]*\|`)
	text = alignRegex.ReplaceAllStringFunc(text, func(match string) string {
		// 保持原有的对齐方式
		left := strings.Contains(match, ":-")
		right := strings.Contains(match, "-:")
		if left && right {
			return "|:---:|"
		} else if left {
			return "|:---|"
		} else if right {
			return "|---:|"
		}
		return "|---|"
	})

	// 修复表格单元格内容
	cellRegex := regexp.MustCompile(`\|(.*?)\|`)
	text = cellRegex.ReplaceAllStringFunc(text, func(match string) string {
		// 移除单元格内容前后的多余空格，但保留一个空格
		return regexp.MustCompile(`\|\s*(.*?)\s*\|`).ReplaceAllString(match, "| $1 |")
	})

	return text
}

// fixMathSpaces 修复数学公式中不必要的空格
func (p *MarkdownPostProcessor) fixMathSpaces(text string) string {
	// 修复行内公式中的空格，同时处理 ${1}$ 的问题
	mathRegex := regexp.MustCompile(`\$\s*\{?\s*([^${}]+?)\s*\}?\s*\$`)
	text = mathRegex.ReplaceAllString(text, `$${1}$`)

	// 修复块级公式中的空格
	blockMathRegex := regexp.MustCompile(`\$\$\s*\{?\s*([^${}]+?)\s*\}?\s*\$\$`)
	text = blockMathRegex.ReplaceAllString(text, `$$${1}$$`)

	return text
}

// fixHeadingSpaces 修复标题前后的空格
func (p *MarkdownPostProcessor) fixHeadingSpaces(text string) string {
	// 确保标题前有空行（除非是文档开头）
	headingRegex := regexp.MustCompile(`([^\n])\n(#{1,6}\s)`)
	text = headingRegex.ReplaceAllString(text, "$1\n\n$2")

	// 确保标题后有空行
	afterHeadingRegex := regexp.MustCompile(`(#{1,6}\s.*?\n)([^#\n])`)
	text = afterHeadingRegex.ReplaceAllString(text, "$1\n$2")

	return text
}

// fixLinksAndImages 修复链接和图片格式
func (p *MarkdownPostProcessor) fixLinksAndImages(text string) string {
	// 修复链接中的空格
	linkRegex := regexp.MustCompile(`\[(.*?)\]\(\s*(.*?)\s*\)`)
	text = linkRegex.ReplaceAllString(text, "[$1]($2)")

	// 修复图片中的空格
	imageRegex := regexp.MustCompile(`!\[(.*?)\]\(\s*(.*?)\s*\)`)
	text = imageRegex.ReplaceAllString(text, "![$1]($2)")

	return text
}

// cleanupPromptTags 清理翻译过程中可能引入的提示词标记
func (p *MarkdownPostProcessor) cleanupPromptTags(text string) string {
	// 移除常见的提示词标记
	tagsToRemove := []struct {
		start string
		end   string
	}{
		{start: "<SOURCE_TEXT>", end: "</SOURCE_TEXT>"},
		{start: "<TRANSLATION>", end: "</TRANSLATION>"},
		{start: "<EXPERT_SUGGESTIONS>", end: "</EXPERT_SUGGESTIONS>"},
		{start: "<TEXT TO EDIT>", end: "</TEXT TO EDIT>"},
		{start: "<TEXT TO TRANSLATE>", end: "</TEXT TO TRANSLATE>"},
		{start: "<TRANSLATE_THIS>", end: "</TRANSLATE_THIS>"},
		{start: "<翻译>", end: "</翻译>"},
		{start: "<翻译后的文本>", end: "</翻译后的文本>"},
		{start: "<TEXT TRANSLATED>", end: "</TEXT TRANSLATED>"},
	}

	result := text

	// 移除成对的标记
	for _, tag := range tagsToRemove {
		// 先尝试移除完整的标记对
		for {
			startIdx := strings.Index(result, tag.start)
			if startIdx == -1 {
				break
			}

			endIdx := strings.Index(result, tag.end)
			if endIdx == -1 || endIdx < startIdx {
				break
			}

			// 保留标记之间的内容，移除标记本身
			content := result[startIdx+len(tag.start) : endIdx]
			result = result[:startIdx] + content + result[endIdx+len(tag.end):]
		}

		// 然后移除任何剩余的单独标记
		result = strings.ReplaceAll(result, tag.start, "")
		result = strings.ReplaceAll(result, tag.end, "")
	}

	// 使用正则表达式移除其他可能的提示词标记
	promptTagsRegex := []*regexp.Regexp{
		regexp.MustCompile(`</?[A-Z_]+>`),                   // 如 <TRANSLATION> 或 </TRANSLATION>
		regexp.MustCompile(`</?[a-z_]+>`),                   // 如 <translation> 或 </translation>
		regexp.MustCompile(`</?[\p{Han}]+>`),                // 中文标记，如 <翻译> 或 </翻译>
		regexp.MustCompile(`</?[\p{Han}][^>]{0,20}>`),       // 带属性的中文标记
		regexp.MustCompile(`\[INTERNAL INSTRUCTIONS:.*?\]`), // 内部指令
	}

	for _, regex := range promptTagsRegex {
		result = regex.ReplaceAllString(result, "")
	}

	return result
}

// fixFormatIssues 修复翻译结果中的格式问题
func (p *MarkdownPostProcessor) fixFormatIssues(text string) string {
	result := text

	// 修复错误的斜体标记（确保*前后有空格或在行首尾）
	italicRegex := regexp.MustCompile(`(\S)\*(\S)`)
	result = italicRegex.ReplaceAllString(result, "$1 * $2")

	// 修复错误的粗体标记
	boldRegex := regexp.MustCompile(`(\S)\*\*(\S)`)
	result = boldRegex.ReplaceAllString(result, "$1 ** $2")

	// 移除多余的空行
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}

	// 移除行首行尾多余的空格
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	result = strings.Join(lines, "\n")

	return result
}

// fixUntranslatable 处理未翻译的占位符
func (p *MarkdownPostProcessor) fixUntranslatable(text string) string {
	// 首先尝试匹配已有的占位符
	preservePattern := regexp.MustCompile(`@@PRESERVE_(\d+)@@(.*?)@@/PRESERVE_$1@@`)

	// 如果文本中包含占位符，直接返回
	if preservePattern.MatchString(text) {
		return text
	}

	// 否则尝试恢复原始内容
	for _, replacement := range p.currentReplacements {
		if strings.Contains(text, replacement.Placeholder) {
			text = strings.ReplaceAll(text, replacement.Placeholder, replacement.Original)
		}
	}

	return text
}
