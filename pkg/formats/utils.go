package formats

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
	"golang.org/x/net/html"
)

type Chunk struct {
	Text            string
	NeedToTranslate bool
}

// NodeToTranslate 表示要翻译的节点
type NodeToTranslate struct {
	ID      int    // 唯一标识符
	Content string // 原始内容
}

// 如果有多余的 \n\n\n 换行符替换成 \n\n 递归进行
func RemoveRedundantNewlines(text string) string {
	for {
		if strings.Contains(text, "\n\n\n") {
			text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
		} else {
			break
		}
	}
	return text
}

func parallelTranslateChunks(chunks []Chunk, p *MarkdownProcessor, concurrency int) ([]string, error) {
	// concurrency 表示最大并发量
	results := make([]string, len(chunks))

	var wg sync.WaitGroup
	var translateErr error
	var mu sync.Mutex // 用来保护 translateErr

	// 建一个带缓冲的信号量通道，容量为 concurrency
	sem := make(chan struct{}, concurrency)

	for i, chunk := range chunks {
		wg.Add(1)

		go func(idx int, textChunk Chunk) {
			defer wg.Done()

			// 向信号量通道发送一个空结构体，若通道满则会阻塞
			sem <- struct{}{}

			// 翻译
			if textChunk.NeedToTranslate {
				p.logger.Debug("需要翻译", zap.Int("idx", idx))
				translatedChunk, err := p.TranslateText(textChunk.Text)
				if err != nil {
					mu.Lock()
					translateErr = err
					mu.Unlock()
					p.logger.Warn("翻译出错", zap.Error(err), zap.Int("idx", idx))
					results[idx] = textChunk.Text // 如果翻译出错，则返回原始内容
				} else {
					p.logger.Debug("翻译成功", zap.Int("idx", idx))
					results[idx] = translatedChunk
				}
			} else {
				p.logger.Debug("不需要翻译", zap.Int("idx", idx))
				results[idx] = textChunk.Text
			}

			// 释放一个信号量
			<-sem
		}(i, chunk)
	}

	wg.Wait()

	if translateErr != nil {
		return nil, translateErr
	}
	return results, nil
}

func IsFileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// TranslateHTMLDOM 翻译 HTML 字符串中的文本节点，保持原有的 DOM 结构
func TranslateHTMLDOM(htmlStr string, t translator.Translator, logger *zap.Logger) (string, error) {
	// 检查是否包含XML声明，如果有，需要特殊处理
	hasXMLDeclaration := strings.Contains(htmlStr, "<?xml")
	var xmlDeclaration string
	if hasXMLDeclaration {
		// 提取XML声明
		xmlDeclRegex := regexp.MustCompile(`<\?xml[^>]*\?>`)
		if match := xmlDeclRegex.FindString(htmlStr); match != "" {
			xmlDeclaration = match
			logger.Debug("找到XML声明", zap.String("declaration", xmlDeclaration))
		}
	}

	// 检查是否包含DOCTYPE声明
	hasDOCTYPE := strings.Contains(htmlStr, "<!DOCTYPE")
	var doctypeDeclaration string
	if hasDOCTYPE {
		// 提取DOCTYPE声明
		doctypeRegex := regexp.MustCompile(`<!DOCTYPE[^>]*>`)
		if match := doctypeRegex.FindString(htmlStr); match != "" {
			doctypeDeclaration = match
			logger.Debug("找到DOCTYPE声明", zap.String("doctype", doctypeDeclaration))
		}
	}

	// 解析HTML文档
	root, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return "", err
	}

	cfg := t.GetConfig()
	chunkSize := 6000
	if cfg != nil {
		if modelCfg, ok := cfg.ModelConfigs[cfg.DefaultModelName]; ok {
			if modelCfg.MaxInputTokens > 0 {
				// 减小缓冲区大小，确保不会超出模型限制
				chunkSize = modelCfg.MaxInputTokens - 2000
				if chunkSize <= 0 {
					chunkSize = modelCfg.MaxInputTokens / 2
				}
			}
		}
	}

	// 收集所有需要翻译的文本节点
	var textNodes []*html.Node
	collectTextNodes(root, &textNodes)

	// 记录总节点数
	totalNodes := len(textNodes)
	logger.Info("收集到的文本节点数", zap.Int("节点数", totalNodes))

	// 如果没有文本节点，直接返回原始HTML
	if totalNodes == 0 {
		return htmlStr, nil
	}

	// 将文本节点分组，确保每组不超过模型的输入限制
	groups := groupTextNodes(textNodes, chunkSize)
	logger.Info("文本节点分组完成", zap.Int("组数", len(groups)))

	// 逐组翻译文本节点
	for groupIndex, group := range groups {
		// 构建当前组的文本
		var builder strings.Builder
		for i, n := range group {
			if i > 0 {
				builder.WriteString("\n\n")
			}
			builder.WriteString(strings.TrimSpace(n.Data))
		}

		groupText := builder.String()
		if groupText == "" {
			logger.Warn("跳过空组", zap.Int("组索引", groupIndex))
			continue
		}

		logger.Debug("翻译文本组",
			zap.Int("组索引", groupIndex),
			zap.Int("组大小", len(group)),
			zap.Int("文本长度", len(groupText)))

		// 翻译当前组的文本
		translated, err := t.Translate(groupText, true)
		if err != nil {
			logger.Warn("翻译HTML节点组失败",
				zap.Error(err),
				zap.Int("组索引", groupIndex),
				zap.Int("组大小", len(group)))
			continue
		}

		// 确保翻译结果不为空
		if translated == "" {
			logger.Warn("翻译结果为空", zap.Int("组索引", groupIndex))
			continue
		}

		// 将翻译结果分配回各个节点
		parts := strings.Split(translated, "\n\n")

		// 记录分割后的部分数量，用于调试
		logger.Debug("翻译结果分割",
			zap.Int("组索引", groupIndex),
			zap.Int("原始节点数", len(group)),
			zap.Int("分割部分数", len(parts)))

		// 分配翻译结果到各个节点
		for i, n := range group {
			// 保留原始的前导和尾随空白
			leading := leadingWhitespace(n.Data)
			trailing := trailingWhitespace(n.Data)

			// 默认使用原始文本（如果翻译失败）
			translatedText := strings.TrimSpace(n.Data)

			// 如果有对应的翻译结果，则使用翻译结果
			if i < len(parts) {
				partText := strings.TrimSpace(parts[i])
				if partText != "" {
					translatedText = partText
				}
			} else {
				// 如果索引超出范围，记录警告
				logger.Warn("翻译结果部分不足",
					zap.Int("组索引", groupIndex),
					zap.Int("节点索引", i),
					zap.Int("可用部分数", len(parts)))
			}

			// 更新节点文本，保留原始空白
			n.Data = leading + translatedText + trailing
		}
	}

	// 将修改后的DOM渲染回HTML
	var buf bytes.Buffer
	if err := html.Render(&buf, root); err != nil {
		return "", fmt.Errorf("渲染HTML失败: %w", err)
	}

	// 直接使用渲染后的HTML，而不是通过goquery再次解析
	// 这样可以避免goquery对HTML结构的修改
	htmlResult := buf.String()

	// 恢复XML声明和DOCTYPE（如果有）
	if hasXMLDeclaration && xmlDeclaration != "" {
		// 确保XML声明在文档最开始
		if !strings.HasPrefix(htmlResult, xmlDeclaration) {
			htmlResult = xmlDeclaration + "\n" + htmlResult
		}
	}

	if hasDOCTYPE && doctypeDeclaration != "" {
		// 如果已经有XML声明，则在XML声明之后添加DOCTYPE
		if hasXMLDeclaration && strings.HasPrefix(htmlResult, xmlDeclaration) {
			htmlResult = strings.Replace(htmlResult, xmlDeclaration, xmlDeclaration+"\n"+doctypeDeclaration, 1)
		} else if !strings.Contains(htmlResult, doctypeDeclaration) {
			// 否则在文档开头添加DOCTYPE
			htmlResult = doctypeDeclaration + "\n" + htmlResult
		}
	}

	// 检查并修复可能的编码问题
	htmlResult = fixEncodingIssues(htmlResult)

	return htmlResult, nil
}

// fixEncodingIssues 修复HTML/XML文档中的编码问题
func fixEncodingIssues(html string) string {
	// 修复常见的编码问题
	result := html

	// 修复Unicode替换字符 \ufffd
	result = strings.ReplaceAll(result, "\ufffd", "")

	// 修复错误翻译的XML标签
	result = regexp.MustCompile(`<!--\?xml`).ReplaceAllString(result, "<?xml")

	// 修复其他可能的XML标签错误
	result = regexp.MustCompile(`<\s*!--\s*DOCTYPE`).ReplaceAllString(result, "<!DOCTYPE")

	// 修复可能被错误翻译的闭合标签
	result = regexp.MustCompile(`--\s*>`).ReplaceAllString(result, ">")

	// 移除翻译过程中可能引入的提示词标记
	tagsToRemove := []struct {
		start string
		end   string
	}{
		{start: "<SOURCE_TEXT>", end: "</SOURCE_TEXT>"},
		{start: "<TRANSLATION>", end: "</TRANSLATION>"},
		{start: "<TEXT TRANSLATED>", end: "</TEXT TRANSLATED>"},
		{start: "<翻译后的文本>", end: "</翻译后的文本>"},
		{start: "<翻译>", end: "</翻译>"},
	}

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

	// 修复可能的格式问题
	result = fixFormatIssues(result)

	return result
}

// fixFormatIssues 修复翻译结果中的格式问题
func fixFormatIssues(text string) string {
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

	return result
}

// translateHTMLNode 递归遍历HTML节点并翻译文本内容
// func translateHTMLNode(n *html.Node, t translator.Translator, logger *zap.Logger) { // UNUSED FUNCTION
// 	if n.Type == html.TextNode {
// 		originalText := strings.TrimSpace(n.Data)
// 		if originalText != "" {
// 			// ... (translation logic)
// 		}
// 	}
//
// 	for c := n.FirstChild; c != nil; c = c.NextSibling {
// 		translateHTMLNode(c, t, logger)
// 	}
// }

// leadingWhitespace 返回字符串前导的空白字符
func leadingWhitespace(s string) string {
	trimmed := strings.TrimLeftFunc(s, unicode.IsSpace)
	return s[:len(s)-len(trimmed)]
}

func trailingWhitespace(s string) string {
	trimmed := strings.TrimRightFunc(s, unicode.IsSpace)
	return s[len(trimmed):]
}

func collectTextNodes(n *html.Node, nodes *[]*html.Node) {
	// 检查是否是XML声明或DOCTYPE
	if n.Type == html.DoctypeNode || (n.Type == html.CommentNode && strings.Contains(n.Data, "?xml")) {
		return // 不处理XML声明和DOCTYPE
	}

	if n.Type == html.ElementNode {
		// 不处理脚本、样式、预格式化文本和代码块
		switch n.Data {
		case "script", "style", "pre", "code":
			return
		}

		// 检查是否有特殊属性，如xml:lang等，这些元素通常不应被翻译
		for _, attr := range n.Attr {
			if strings.HasPrefix(attr.Key, "xml:") || attr.Key == "xmlns" {
				// 这是一个XML命名空间相关的元素，不应翻译其内容
				return
			}
		}
	}

	if n.Type == html.TextNode {
		// 即使是空白文本也收集，以保持文档结构完整性
		// 但过滤掉只包含空白的节点，避免无意义的翻译
		trimmed := strings.TrimSpace(n.Data)
		if trimmed != "" {
			// 检查是否包含XML标签或声明，如果包含则不翻译
			if !strings.Contains(trimmed, "<?xml") && !strings.Contains(trimmed, "<!DOCTYPE") {
				*nodes = append(*nodes, n)
			}
		}
	}

	// 递归处理所有子节点
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectTextNodes(c, nodes)
	}
}

func groupTextNodes(nodes []*html.Node, limit int) [][]*html.Node {
	var groups [][]*html.Node
	var current []*html.Node
	currentLen := 0

	// 确保至少有一个节点被处理
	if len(nodes) == 0 {
		return groups
	}

	for _, n := range nodes {
		text := strings.TrimSpace(n.Data)
		textLen := len(text)

		// 如果单个节点超过限制，单独处理它
		if textLen > limit {
			// 如果当前组不为空，先保存
			if len(current) > 0 {
				groups = append(groups, current)
				current = nil
				currentLen = 0
			}
			// 单独将大节点作为一组
			groups = append(groups, []*html.Node{n})
			continue
		}

		// 如果添加当前节点会超出限制，先保存当前组
		if currentLen+textLen > limit && len(current) > 0 {
			groups = append(groups, current)
			current = nil
			currentLen = 0
		}

		// 添加当前节点到新组
		current = append(current, n)
		currentLen += textLen
	}

	// 保存最后一个组
	if len(current) > 0 {
		groups = append(groups, current)
	}

	return groups
}
