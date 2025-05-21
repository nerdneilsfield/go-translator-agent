package formats

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
	"golang.org/x/net/html"
)

// NodeFormatInfo 保存节点的原始格式信息
type NodeFormatInfo struct {
	LeadingWhitespace  string
	TrailingWhitespace string
	OriginalHTML       string // 用于保存原始HTML结构
}

// replaceTextPreservingStructure 尝试替换文本内容，同时保留HTML结构
// 这个函数会尝试智能地将翻译后的文本分配到原始HTML结构中的文本节点
func replaceTextPreservingStructure(selection *goquery.Selection, translatedText string) {
	// 收集所有文本节点
	var textNodes []*goquery.Selection
	selection.Contents().Each(func(_ int, s *goquery.Selection) {
		if goquery.NodeName(s) == "#text" {
			if strings.TrimSpace(s.Text()) != "" {
				textNodes = append(textNodes, s)
			}
		} else {
			// 递归处理子元素
			replaceTextPreservingStructure(s, "")
		}
	})

	// 如果没有文本节点或没有提供翻译文本，直接返回
	if len(textNodes) == 0 || translatedText == "" {
		return
	}

	// 如果只有一个文本节点，直接替换
	if len(textNodes) == 1 {
		textNodes[0].ReplaceWithHtml(translatedText)
		return
	}

	// 尝试将翻译文本分割成与原始文本节点数量相同的部分
	parts := strings.Split(translatedText, " ")
	if len(parts) < len(textNodes) {
		// 如果分割后的部分少于文本节点数，直接将整个翻译文本放入第一个节点
		textNodes[0].ReplaceWithHtml(translatedText)
		// 清空其他节点
		for i := 1; i < len(textNodes); i++ {
			textNodes[i].ReplaceWithHtml("")
		}
		return
	}

	// 计算每个节点应该分配的部分数量
	partsPerNode := len(parts) / len(textNodes)
	extraParts := len(parts) % len(textNodes)

	// 分配翻译文本到各个节点
	startIdx := 0
	for i, node := range textNodes {
		endIdx := startIdx + partsPerNode
		if i < extraParts {
			endIdx++
		}
		if endIdx > len(parts) {
			endIdx = len(parts)
		}

		// 将分配的部分组合成文本
		nodeText := strings.Join(parts[startIdx:endIdx], " ")
		node.ReplaceWithHtml(nodeText)

		startIdx = endIdx
	}
}

// containsOnlyHTMLElements 检查一个节点是否只包含HTML元素，没有文本内容
func containsOnlyHTMLElements(s *goquery.Selection) bool {
	hasText := false
	s.Contents().Each(func(_ int, child *goquery.Selection) {
		if goquery.NodeName(child) == "#text" {
			text := strings.TrimSpace(child.Text())
			if text != "" {
				hasText = true
			}
		}
	})
	return !hasText
}

// TranslateHTMLWithGoQuery 使用goquery库翻译HTML文档，更好地保留HTML结构
func TranslateHTMLWithGoQuery(htmlStr string, t translator.Translator, logger *zap.Logger) (string, error) {
	// 保存XML声明和DOCTYPE，因为goquery可能会移除它们
	hasXMLDeclaration := strings.Contains(htmlStr, "<?xml")
	var xmlDeclaration string
	if hasXMLDeclaration {
		xmlDeclRegex := regexp.MustCompile(`<\\?xml[^>]*\\?>`)
		if match := xmlDeclRegex.FindString(htmlStr); match != "" {
			xmlDeclaration = match
			logger.Debug("找到XML声明", zap.String("declaration", xmlDeclaration))
			htmlStr = strings.Replace(htmlStr, xmlDeclaration, "", 1) //移除一次，避免影响解析
		}
	}

	hasDOCTYPE := strings.Contains(strings.ToLower(htmlStr), "<!doctype")
	var doctypeDeclaration string
	if hasDOCTYPE {
		doctypeRegex := regexp.MustCompile(`(?i)<!DOCTYPE[^>]*>`)
		if match := doctypeRegex.FindString(htmlStr); match != "" {
			doctypeDeclaration = match
			logger.Debug("找到DOCTYPE声明", zap.String("doctype", doctypeDeclaration))
			htmlStr = strings.Replace(htmlStr, doctypeDeclaration, "", 1) //移除一次
		}
	}

	// 使用goquery解析HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return "", fmt.Errorf("解析HTML失败: %w", err)
	}

	// 获取配置
	agentConfig := t.GetConfig()
	chunkSize := 6000
	concurrency := 1 // 默认为1，即不进行文件内并行
	if agentConfig != nil {
		if modelCfg, ok := agentConfig.ModelConfigs[agentConfig.DefaultModelName]; ok {
			if modelCfg.MaxInputTokens > 0 {
				chunkSize = modelCfg.MaxInputTokens - 2000
				if chunkSize <= 0 {
					chunkSize = modelCfg.MaxInputTokens / 2
				}
			}
		}
		// 使用 HtmlConcurrency 控制单个HTML文件内部的并发
		if agentConfig.HtmlConcurrency > 0 {
			concurrency = agentConfig.HtmlConcurrency
		} else {
			logger.Debug("HtmlConcurrency未配置或为0，单个HTML文件内节点翻译将串行执行。", zap.Int("resolved_concurrency", concurrency))
		}
	}
	logger.Debug("HTML文件内节点翻译并发设置", zap.Int("html_concurrency", concurrency), zap.Int("chunkSize", chunkSize))

	// 保护特定内容
	protected := make(map[string]string)
	placeholderIndex := 0

	// 为了保护，我们需要操作原始字符串，然后重新解析。或者在goquery文档上操作。
	// 这里选择先在原始字符串上操作，然后重新解析，因为正则表达式在字符串上更直接。
	tempHTMLForProtection, err := doc.Html() // Get HTML from current doc state
	if err != nil {
		return "", fmt.Errorf("获取临时HTML失败: %w", err)
	}

	protectRegexes := []*regexp.Regexp{
		regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`),
		regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`),
		regexp.MustCompile(`(?s)<pre[^>]*>.*?</pre>`),
		regexp.MustCompile(`(?s)<code[^>]*>.*?</code>`),
		regexp.MustCompile(`(?s)<!--.*?-->`),
	}

	for _, re := range protectRegexes {
		tempHTMLForProtection = re.ReplaceAllStringFunc(tempHTMLForProtection, func(match string) string {
			placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
			protected[placeholder] = match
			placeholderIndex++
			return placeholder
		})
	}

	doc, err = goquery.NewDocumentFromReader(strings.NewReader(tempHTMLForProtection))
	if err != nil {
		return "", fmt.Errorf("重新解析受保护的HTML失败: %w", err)
	}

	type TextNodeInfo struct {
		Selection     *goquery.Selection
		Text          string
		Path          string
		Format        NodeFormatInfo
		IsAttribute   bool
		AttributeName string
	}

	var textNodes []TextNodeInfo
	skipSelectors := []string{"script", "style", "code", "pre"}

	var processNode func(*goquery.Selection, string)
	processNode = func(s *goquery.Selection, currentPath string) {
		nodeName := goquery.NodeName(s)
		for _, skip := range skipSelectors {
			if nodeName == skip {
				return
			}
		}
		if IsSVGElement(s) {
			return
		} // Skip SVG elements

		// 处理属性
		attrs := s.Get(0).Attr
		for _, attr := range attrs {
			if shouldTranslateAttr(attr.Key) && attr.Val != "" {
				textNodes = append(textNodes, TextNodeInfo{
					Selection:     s,
					Text:          attr.Val,
					Path:          fmt.Sprintf("%s[@%s]", currentPath, attr.Key),
					IsAttribute:   true,
					AttributeName: attr.Key,
				})
			}
		}

		// 处理子节点
		s.Contents().Each(func(_ int, child *goquery.Selection) {
			if goquery.NodeName(child) == "#text" {
				text := child.Text()
				trimmedText := strings.TrimSpace(text)
				if trimmedText != "" {
					formatInfo := NodeFormatInfo{
						LeadingWhitespace:  text[:len(text)-len(strings.TrimLeft(text, " \t\n\r"))],
						TrailingWhitespace: text[len(strings.TrimRight(text, " \t\n\r")):],
					}
					textNodes = append(textNodes, TextNodeInfo{
						Selection: child,
						Text:      trimmedText,
						Path:      fmt.Sprintf("%s/#text", currentPath),
						Format:    formatInfo,
					})
				}
			} else if child.Is("*") { // 确保是元素节点
				processNode(child, fmt.Sprintf("%s/%s", currentPath, goquery.NodeName(child)))
			}
		})
	}

	doc.Find("body").Children().Each(func(i int, s *goquery.Selection) {
		processNode(s, "body/"+goquery.NodeName(s))
	})
	if len(textNodes) == 0 { // 尝试 head > title
		doc.Find("head > title").Each(func(i int, s *goquery.Selection) {
			processNode(s, "head/title")
		})
	}

	logger.Info("收集到的可翻译节点数量", zap.Int("count", len(textNodes)))
	if len(textNodes) == 0 {
		// 没有可翻译内容，恢复原始声明并返回
		outputHTML, _ := doc.Html()
		for placeholder, original := range protected {
			outputHTML = strings.ReplaceAll(outputHTML, placeholder, original)
		}
		// 使用 strings.Builder 构建最终输出，确保声明顺序正确
		var finalOutputBuilder strings.Builder
		if hasXMLDeclaration && xmlDeclaration != "" {
			finalOutputBuilder.WriteString(xmlDeclaration)
			finalOutputBuilder.WriteString("\n")
		}
		if hasDOCTYPE && doctypeDeclaration != "" {
			finalOutputBuilder.WriteString(doctypeDeclaration)
			finalOutputBuilder.WriteString("\n")
		}
		finalOutputBuilder.WriteString(outputHTML)
		return finalOutputBuilder.String(), nil
	}

	var groups [][]TextNodeInfo
	var currentGroup []TextNodeInfo
	currentLength := 0
	for _, node := range textNodes {
		nodeTextLength := len(node.Text)
		if currentLength+nodeTextLength > chunkSize && len(currentGroup) > 0 {
			groups = append(groups, currentGroup)
			currentGroup = []TextNodeInfo{}
			currentLength = 0
		}
		currentGroup = append(currentGroup, node)
		currentLength += nodeTextLength
	}
	if len(currentGroup) > 0 {
		groups = append(groups, currentGroup)
	}
	logger.Info("文本节点分组完成", zap.Int("组数", len(groups)))

	groupTranslationResults := make([]map[int]string, len(groups))
	groupTranslationErrors := make([]error, len(groups))
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	for groupIndex, groupData := range groups {
		wg.Add(1)
		go func(gIdx int, currentGroupData []TextNodeInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var textsToTranslate []string
			originalTextsForGroup := make(map[int]string)

			for i, node := range currentGroupData {
				nodeMarker := fmt.Sprintf("@@NODE_%d@@", i)
				originalTextsForGroup[i] = node.Text
				textsToTranslate = append(textsToTranslate, nodeMarker+"\\n"+node.Text)
			}
			groupText := strings.Join(textsToTranslate, "\\n\\n")

			if strings.TrimSpace(groupText) == "" && len(textsToTranslate) == 0 {
				logger.Debug("跳过翻译空组", zap.Int("groupIndex", gIdx))
				groupTranslationResults[gIdx] = make(map[int]string) // Empty map for this group
				return
			}

			logger.Debug("开始翻译HTML文本组", zap.Int("groupIndex", gIdx), zap.Int("textLength", len(groupText)))
			translatedText, err := t.Translate(groupText, true)
			if err != nil {
				groupTranslationErrors[gIdx] = err
				logger.Warn("翻译HTML节点组失败", zap.Error(err), zap.Int("groupIndex", gIdx))
				currentGroupTranslations := make(map[int]string)
				for i := range currentGroupData {
					currentGroupTranslations[i] = originalTextsForGroup[i]
				}
				groupTranslationResults[gIdx] = currentGroupTranslations
				return
			}

			translatedParts := strings.Split(translatedText, "\\n\\n")
			currentGroupTranslations := make(map[int]string)
			for _, part := range translatedParts {
				for i := range currentGroupData {
					nodeMarker := fmt.Sprintf("@@NODE_%d@@", i)
					if strings.Contains(part, nodeMarker) {
						translatedContent := strings.Replace(part, nodeMarker, "", 1)
						translatedContent = strings.TrimSpace(translatedContent)
						if translatedContent != "" {
							currentGroupTranslations[i] = translatedContent
						}
						break
					}
				}
			}
			for i := range currentGroupData {
				if _, ok := currentGroupTranslations[i]; !ok {
					currentGroupTranslations[i] = originalTextsForGroup[i]
				}
			}
			groupTranslationResults[gIdx] = currentGroupTranslations
		}(groupIndex, groupData)
	}
	wg.Wait()

	for groupIndex, groupData := range groups {
		if groupTranslationErrors[groupIndex] != nil {
			logger.Warn("跳过应用翻译组（由于错误）", zap.Int("groupIndex", groupIndex), zap.Error(groupTranslationErrors[groupIndex]))
		}
		translationsForThisGroup := groupTranslationResults[groupIndex]
		for nodeIdxInGroup, nodeInfo := range groupData {
			translatedContent, ok := translationsForThisGroup[nodeIdxInGroup]
			if !ok {
				logger.Debug("节点无翻译（应已回退到原文）", zap.Int("groupIndex", groupIndex), zap.Int("nodeIdx", nodeIdxInGroup), zap.String("path", nodeInfo.Path))
				continue
			}
			if nodeInfo.IsAttribute {
				nodeInfo.Selection.SetAttr(nodeInfo.AttributeName, translatedContent)
			} else if goquery.NodeName(nodeInfo.Selection) == "#text" {
				formattedContent := nodeInfo.Format.LeadingWhitespace + translatedContent + nodeInfo.Format.TrailingWhitespace
				nodeInfo.Selection.ReplaceWithHtml(formattedContent)
			} else if nodeInfo.Format.OriginalHTML != "" {
				tempDoc, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + nodeInfo.Format.OriginalHTML + "</div>"))
				if err == nil {
					replaceTextPreservingStructure(tempDoc.Find("div"), translatedContent)
					newHTML, err := tempDoc.Find("div").Html()
					if err == nil {
						nodeInfo.Selection.SetHtml(newHTML)
					} else {
						nodeInfo.Selection.SetText(translatedContent)
					}
				} else {
					nodeInfo.Selection.SetText(translatedContent)
				}
			} else {
				nodeInfo.Selection.SetText(translatedContent)
			}
		}
	}

	htmlResult, err := doc.Html()
	if err != nil {
		return "", fmt.Errorf("生成最终HTML失败: %w", err)
	}

	for placeholder, original := range protected {
		htmlResult = strings.ReplaceAll(htmlResult, placeholder, original)
	}

	// 使用 strings.Builder 构建最终输出，确保声明顺序正确
	var finalOutputBuilder strings.Builder
	if hasXMLDeclaration && xmlDeclaration != "" {
		finalOutputBuilder.WriteString(xmlDeclaration)
		finalOutputBuilder.WriteString("\n")
	}
	if hasDOCTYPE && doctypeDeclaration != "" {
		finalOutputBuilder.WriteString(doctypeDeclaration)
		finalOutputBuilder.WriteString("\n")
	}
	finalOutputBuilder.WriteString(htmlResult)

	return finalOutputBuilder.String(), nil
}

func shouldTranslateAttr(attrName string) bool {
	switch strings.ToLower(attrName) {
	case "title", "alt", "label", "aria-label", "placeholder", "summary":
		return true
	}
	return false
}

// IsSVGElement 检查节点是否为SVG元素 (SVG内容通常不翻译)
func IsSVGElement(s *goquery.Selection) bool {
	if goquery.NodeName(s) == "svg" {
		return true
	}
	// 还可以检查父节点是否为SVG，以处理SVG内部的元素
	if s.ParentsFiltered("svg").Length() > 0 {
		return true
	}
	return false
}

func GetTextNodesWithExclusions(selection *goquery.Selection, exclusions []string) []*html.Node {
	var textNodes []*html.Node
	selection.Contents().Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node.Type == html.TextNode {
			textNodes = append(textNodes, node)
		} else if node.Type == html.ElementNode {
			// 检查是否在排除列表中
			isExcluded := false
			for _, ex := range exclusions {
				if node.Data == ex {
					isExcluded = true
					break
				}
			}
			if !isExcluded {
				textNodes = append(textNodes, GetTextNodesWithExclusions(s, exclusions)...)
			}
		}
	})
	return textNodes
}

// getChildTextNodes 收集子节点的文本节点
// func getChildTextNodes(s *goquery.Selection, exclusions []string, includeStyleAndScript bool) []*html.Node { // UNUSED FUNCTION
// 	var nodes []*html.Node
// 	s.Contents().Each(func(k int, child *goquery.Selection) {
// 		node := child.Get(0)
// 		if node.Type == html.TextNode {
// 			// 过滤掉只包含空白字符的文本节点
// 			if strings.TrimSpace(node.Data) != "" {
// 				nodes = append(nodes, node)
// 			}
// 		} else if node.Type == html.ElementNode {
// 			// 恢复内层循环来定义 grandchild 和 k
// 			child.Contents().Each(func(k int, grandchild *goquery.Selection) {
// 				// 检查是否是需要排除的元素类型 (例如 <script>, <style>)
// 				isExcluded := false
// 				for _, ex := range exclusions {
// 					if grandchild.Is(ex) {
// 						isExcluded = true
// 						break
// 					}
// 				}
// 				if includeStyleAndScript && (grandchild.Is("style") || grandchild.Is("script")) {
// 					isExcluded = false // 如果明确要求包含，则不排除
// 				}
//
// 				if !isExcluded {
// 					if grandchild.Get(0).Type == html.TextNode {
// 						if strings.TrimSpace(grandchild.Text()) != "" {
// 							nodes = append(nodes, grandchild.Get(0))
// 						}
// 					} else {
// 						// 递归处理更深层级的节点
// 						// 注意：这里的 depth 应该来自外层函数的参数或迭代变量，此处假设为 k 或其他合适的值
// 						grandchildTextNodes := collectTextNodesRecursive(grandchild, exclusions, false, false, k+1)
// 						nodes = append(nodes, grandchildTextNodes...)
// 					}
// 				}
// 			})
// 		}
// 	})
// 	return nodes
// }

// collectTextNodesRecursive 递归收集文本节点，处理嵌套情况
// ... (collectTextNodesRecursive function)
