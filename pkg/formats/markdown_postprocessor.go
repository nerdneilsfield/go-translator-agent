package formats

import (
	"regexp"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"go.uber.org/zap"
)

// MarkdownPostProcessor 用于处理翻译后的 Markdown 文件
type MarkdownPostProcessor struct {
	config *config.Config
	logger *zap.Logger
}

// NewMarkdownPostProcessor 创建一个新的 Markdown 后处理器
func NewMarkdownPostProcessor(config *config.Config, logger *zap.Logger) *MarkdownPostProcessor {
	return &MarkdownPostProcessor{
		config: config,
		logger: logger,
	}
}

// ProcessMarkdown 处理翻译后的 Markdown 文本
func (p *MarkdownPostProcessor) ProcessMarkdown(text string) string {
	if !p.config.PostProcessMarkdown {
		return text
	}

	// 移除 HTML 标签
	text = strings.ReplaceAll(text, "<html><body>", "\n")
	text = strings.ReplaceAll(text, "</body></html>", "\n")

	// 修复数学公式中的空格
	text = fixMathSpaces(text)

	// 修复重复的代码块标记
	text = fixDuplicateCodeBlocks(text)

	return text
}

// fixMathSpaces 移除数学公式中不必要的空格
func fixMathSpaces(text string) string {
	// 修复行内公式中的空格
	mathRegex := regexp.MustCompile(`\$\s*([^$]+?)\s*\$`)
	text = mathRegex.ReplaceAllString(text, `$${1}$`)

	// 修复块级公式中的空格
	blockMathRegex := regexp.MustCompile(`\$\$\s*([^$]+?)\s*\$\$`)
	text = blockMathRegex.ReplaceAllString(text, `$$${1}$$`)

	return text
}

// fixDuplicateCodeBlocks 修复重复的代码块标记
func fixDuplicateCodeBlocks(text string) string {
	// 修复连续的代码块开始标记
	text = regexp.MustCompile("```\n```\n\n").ReplaceAllString(text, "```\n")
	text = regexp.MustCompile("\n```\n```").ReplaceAllString(text, "\n```")

	return text
}
