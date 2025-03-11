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
