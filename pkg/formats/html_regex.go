package formats

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// TranslateHTMLWithRegex 使用正则表达式翻译HTML文档，保留HTML结构
func TranslateHTMLWithRegex(htmlStr string, t translator.Translator, logger *zap.Logger) (string, error) {
	// 保存DOCTYPE和XML声明
	doctype := ""
	doctypeRegex := regexp.MustCompile(`<!DOCTYPE[^>]*>`)
	if match := doctypeRegex.FindString(htmlStr); match != "" {
		doctype = match
		// 从HTML中移除DOCTYPE，以便后续处理
		htmlStr = strings.Replace(htmlStr, match, "", 1)
	}

	xmlDecl := ""
	xmlDeclRegex := regexp.MustCompile(`<\?xml[^>]*\?>`)
	if match := xmlDeclRegex.FindString(htmlStr); match != "" {
		xmlDecl = match
		// 从HTML中移除XML声明，以便后续处理
		htmlStr = strings.Replace(htmlStr, match, "", 1)
	}

	// 定义需要保护的标签（不翻译其内容）
	protectedTags := []string{"script", "style", "code", "pre"}

	// 保存需要保护的内容
	protected := make(map[string]string)
	placeholderIndex := 0

	// 保护特定标签内容
	for _, tag := range protectedTags {
		tagRegex := regexp.MustCompile(`(?s)<` + tag + `[^>]*>(.*?)<\/` + tag + `>`)
		matches := tagRegex.FindAllStringSubmatch(htmlStr, -1)
		for _, match := range matches {
			if len(match) > 1 {
				placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
				protected[placeholder] = match[0]
				htmlStr = strings.Replace(htmlStr, match[0], placeholder, 1)
				placeholderIndex++
			}
		}
	}

	// 保护HTML注释
	commentRegex := regexp.MustCompile(`(?s)<!--.*?-->`)
	comments := commentRegex.FindAllString(htmlStr, -1)
	for i, comment := range comments {
		placeholder := fmt.Sprintf("@@COMMENT_%d@@", i)
		protected[placeholder] = comment
		htmlStr = strings.Replace(htmlStr, comment, placeholder, 1)
	}

	// 使用更精确的方法提取文本节点
	// 我们将HTML分割成标签和文本节点
	var parts []string
	var textParts []string
	var textPartIndices []int

	// 使用正则表达式分割HTML
	tagRegex := regexp.MustCompile(`<[^>]+>`)
	lastEnd := 0
	tagMatches := tagRegex.FindAllStringIndex(htmlStr, -1)

	for _, match := range tagMatches {
		start, end := match[0], match[1]

		// 如果标签前有文本，添加到parts
		if start > lastEnd {
			textNode := strings.TrimSpace(htmlStr[lastEnd:start])
			if textNode != "" {
				parts = append(parts, htmlStr[lastEnd:start])
				textParts = append(textParts, textNode)
				textPartIndices = append(textPartIndices, len(parts)-1)
			}
		}

		// 添加标签
		parts = append(parts, htmlStr[start:end])

		lastEnd = end
	}

	// 添加最后一个文本节点（如果有）
	if lastEnd < len(htmlStr) {
		textNode := strings.TrimSpace(htmlStr[lastEnd:])
		if textNode != "" {
			parts = append(parts, htmlStr[lastEnd:])
			textParts = append(textParts, textNode)
			textPartIndices = append(textPartIndices, len(parts)-1)
		}
	}

	// 记录收集到的文本节点数
	logger.Info("收集到的文本节点数", zap.Int("节点数", len(textParts)))

	// 如果没有文本节点，直接返回原始HTML
	if len(textParts) == 0 {
		return htmlStr, nil
	}

	// 将所有文本节点合并为一个字符串，用于翻译
	allText := strings.Join(textParts, "\n\n")

	// 翻译文本
	translatedText, err := t.Translate(allText, true)
	if err != nil {
		return "", fmt.Errorf("翻译HTML文本失败: %w", err)
	}

	// 将翻译结果分割回各个文本节点
	translatedParts := strings.Split(translatedText, "\n\n")

	// 确保翻译结果的部分数量与原始文本节点数量一致
	if len(translatedParts) < len(textParts) {
		logger.Warn("翻译结果部分数量少于原始节点数量",
			zap.Int("翻译部分数", len(translatedParts)),
			zap.Int("原始节点数", len(textParts)))

		// 如果部分数量不足，使用原始文本填充
		for i := len(translatedParts); i < len(textParts); i++ {
			translatedParts = append(translatedParts, textParts[i])
		}
	}

	// 将翻译结果应用到各个文本节点
	for i, idx := range textPartIndices {
		if i < len(translatedParts) {
			// 替换文本节点的内容，保留原始的空白
			originalText := parts[idx]
			leadingSpace := regexp.MustCompile(`^\s+`).FindString(originalText)
			trailingSpace := regexp.MustCompile(`\s+$`).FindString(originalText)

			translatedContent := strings.TrimSpace(translatedParts[i])
			if translatedContent != "" {
				parts[idx] = leadingSpace + translatedContent + trailingSpace
			}
		}
	}

	// 重新组合HTML
	result := strings.Join(parts, "")

	// 恢复保护的内容
	for placeholder, original := range protected {
		result = strings.Replace(result, placeholder, original, 1)
	}

	// 恢复DOCTYPE和XML声明
	if xmlDecl != "" {
		result = xmlDecl + "\n" + result
	}
	if doctype != "" {
		if xmlDecl != "" {
			result = strings.Replace(result, xmlDecl, xmlDecl+"\n"+doctype, 1)
		} else {
			result = doctype + "\n" + result
		}
	}

	// 修复可能的编码问题
	result = fixEncodingIssues(result)

	return result, nil
}
