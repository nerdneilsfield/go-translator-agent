package formats

import (
	"fmt"

	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/dlclark/regexp2"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
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

	// 如果有多个文本节点，尝试智能分配翻译文本
	// 这里使用一个简单的启发式方法：按照原始文本长度的比例分配翻译文本
	totalOriginalLength := 0
	for _, node := range textNodes {
		totalOriginalLength += len(strings.TrimSpace(node.Text()))
	}

	// 如果原始文本总长度为0，无法按比例分配，直接返回
	if totalOriginalLength == 0 {
		return
	}

	// 按比例分配翻译文本
	translatedWords := strings.Split(translatedText, " ")
	if len(translatedWords) == 0 {
		return
	}

	// 计算每个节点应该分配的单词数
	wordsPerNode := make([]int, len(textNodes))
	for i, node := range textNodes {
		nodeTextLength := len(strings.TrimSpace(node.Text()))
		ratio := float64(nodeTextLength) / float64(totalOriginalLength)
		wordsPerNode[i] = int(ratio * float64(len(translatedWords)))
	}

	// 确保所有单词都被分配
	totalAllocated := 0
	for _, count := range wordsPerNode {
		totalAllocated += count
	}
	remaining := len(translatedWords) - totalAllocated
	if remaining > 0 {
		// 将剩余的单词分配给最长的文本节点
		maxLengthIndex := 0
		maxLength := 0
		for i, node := range textNodes {
			nodeTextLength := len(strings.TrimSpace(node.Text()))
			if nodeTextLength > maxLength {
				maxLength = nodeTextLength
				maxLengthIndex = i
			}
		}
		wordsPerNode[maxLengthIndex] += remaining
	}

	// 分配翻译文本到各个节点
	startIndex := 0
	for i, node := range textNodes {
		endIndex := startIndex + wordsPerNode[i]
		if endIndex > len(translatedWords) {
			endIndex = len(translatedWords)
		}
		if startIndex < endIndex {
			nodeTranslation := strings.Join(translatedWords[startIndex:endIndex], " ")
			node.ReplaceWithHtml(nodeTranslation)
			startIndex = endIndex
		}
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
		xmlDeclRegex := regexp.MustCompile(`<\?xml[^>]*\?>`)
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

	var processNode func(*goquery.Selection, string, bool)
	processNode = func(s *goquery.Selection, currentPath string, parentTranslates bool) {
		nodeName := goquery.NodeName(s)

		// Elements that are fundamentally not for content translation take precedence
		for _, skip := range skipSelectors {
			if nodeName == skip {
				return
			}
		}
		if IsSVGElement(s) {
			return
		}

		// Check if this node has a translate attribute
		currentNodeTranslates := parentTranslates
		if translateAttr, exists := s.Attr("translate"); exists {
			attrValLower := strings.ToLower(translateAttr)
			if attrValLower == "no" || attrValLower == "false" {
				currentNodeTranslates = false
			} else if attrValLower == "yes" || attrValLower == "true" {
				// Explicitly set to yes, can override a parent's "no"
				currentNodeTranslates = true
			}
		}

		// Process attributes only if the element itself is considered translatable
		if currentNodeTranslates {
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
		}

		// Process child nodes
		s.Contents().Each(func(_ int, child *goquery.Selection) {
			childNodeName := goquery.NodeName(child)
			if childNodeName == "#text" {
				// Text nodes are translated only if their direct parent element is translatable
				if currentNodeTranslates {
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
				}
			} else if child.Is("*") { // Element node
				// Translatability of child is determined by its own 'translate' attr
				// and this current node's translatability (passed as parentTranslates for the child)
				processNode(child, fmt.Sprintf("%s/%s", currentPath, childNodeName), currentNodeTranslates)
			}
		})
	}

	doc.Find("body").Children().Each(func(i int, s *goquery.Selection) {
		processNode(s, "body/"+goquery.NodeName(s), true)
	})
	if len(textNodes) == 0 { // 尝试 head > title
		doc.Find("head > title").Each(func(i int, s *goquery.Selection) {
			processNode(s, "head/title", true)
		})
	}

	logger.Debug("收集到的可翻译节点数量", zap.Int("count", len(textNodes)))
	if len(textNodes) == 0 {
		// 没有可翻译内容，恢复原始声明并返回
		outputHTML, _ := doc.Html()

		// 清理可能的XML/DOCTYPE声明（因为我们会在后面显式添加）
		outputHTML = regexp.MustCompile(`(?i)<\?xml[^>]*>\s*`).ReplaceAllString(outputHTML, "")
		outputHTML = regexp.MustCompile(`(?i)<!DOCTYPE[^>]*>\s*`).ReplaceAllString(outputHTML, "")

		// 恢复所有被保护的内容
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
			currentGroup = []TextNodeInfo{node}
			currentLength = nodeTextLength
		} else {
			currentGroup = append(currentGroup, node)
			currentLength += nodeTextLength
		}
	}
	if len(currentGroup) > 0 {
		groups = append(groups, currentGroup)
	}

	logger.Debug("分组后的节点数量", zap.Int("groups", len(groups)))

	// 并行翻译各个组
	var wg sync.WaitGroup
	groupTranslationResults := make([]map[int]string, len(groups))
	groupTranslationErrors := make([]error, len(groups))

	// 创建信号量控制并发
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
				startMarker := fmt.Sprintf("@@NODE_START_%d@@", i)
				endMarker := fmt.Sprintf("@@NODE_END_%d@@", i)
				originalTextsForGroup[i] = node.Text
				textsToTranslate = append(textsToTranslate, startMarker+"\n"+node.Text+"\n"+endMarker)
			}
			groupText := strings.Join(textsToTranslate, "\n\n")

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

			// 解析翻译结果，提取每个节点的翻译
			//translatedParts := strings.Split(translatedText, "\n\n")
			currentGroupTranslations := make(map[int]string)
			// 正则表达式：
			// (?s) 允许 . 匹配换行符
			// @@NODE_START_(\d+)@@ 匹配开始标记并捕获数字索引 (group 1)
			// \n 匹配开始标记后的换行符
			// (.*?) 懒惰匹配翻译内容，直到遇到下一个模式 (group 2)
			// \n 匹配结束标记前的换行符
			// @@NODE_END_\1@@ 使用反向引用 \1确保结束标记的数字与开始标记的数字一致
			re2, err := regexp2.Compile(`@@NODE_START_(\d+)@@\n(.*?)\n@@NODE_END_\1@@`, regexp2.Singleline)
			if err != nil {
				logger.Panic("regexp2编译失败", zap.Error(err)) // 或者返回错误
				return                                          // or panic
			}

			var m *regexp2.Match
			m, err = re2.FindStringMatch(translatedText) // 找到第一个匹配
			if err != nil {
				logger.Error("regexp2 查找匹配时出错", zap.Error(err))
				// 根据错误类型决定是否继续或返回
			}
			// matches 是一个 [][]string，每个元素是：
			// [ 完整匹配的字符串, 捕获的数字N, 捕获的翻译内容 ]

			for m != nil { // 循环直到没有更多匹配
				groups := m.Groups()
				if len(groups) == 3 { // Group 0 是完整匹配, Group 1 是第一个捕获组, Group 2 是第二个
					nodeIndexStr := groups[1].Capture.String()
					rawContent := groups[2].Capture.String()

					// --- 从这里开始是你之前的 rawContent 处理逻辑 ---
					processedContent := rawContent
					if strings.HasPrefix(processedContent, "\n") {
						processedContent = processedContent[len("\n"):]
					}
					if strings.HasSuffix(processedContent, "\n") {
						processedContent = processedContent[:len(processedContent)-len("\n")]
					}
					translation := strings.TrimSpace(processedContent)
					// --- rawContent 处理逻辑结束 ---

					nodeIndex, errAtoi := strconv.Atoi(nodeIndexStr)
					if errAtoi != nil {
						logger.Warn("无法从标记解析节点索引", zap.String("indexStr", nodeIndexStr), zap.Error(errAtoi))
					} else {
						if nodeIndex >= 0 && nodeIndex < len(currentGroupData) {
							if _, exists := currentGroupTranslations[nodeIndex]; !exists {
								currentGroupTranslations[nodeIndex] = translation
							} else {
								logger.Warn("节点索引已被翻译（regexp2）", zap.Int("nodeIndex", nodeIndex))
							}
						} else {
							logger.Warn("解析到的节点索引超出当前组范围（regexp2）", zap.Int("nodeIndex", nodeIndex))
						}
					}
				} else {
					logger.Warn("regexp2 匹配结果的捕获组数量不符合预期", zap.Int("groupsCount", len(groups)))
				}

				m, err = re2.FindNextMatch(m) // 查找下一个匹配
				if err != nil {
					logger.Error("regexp2 查找下一个匹配时出错", zap.Error(err))
					break // 出错则停止查找
				}
			}

			// 确保所有节点都有翻译，如果没有则使用原文
			for i, _ := range currentGroupData { // 获取原始文本以便使用
				if _, ok := currentGroupTranslations[i]; !ok {
					logger.Warn("节点未在翻译结果中找到，使用原文", zap.String("nodeMarker", fmt.Sprintf("@@NODE_%d@@", i)), zap.String("originalText", originalTextsForGroup[i]))
					currentGroupTranslations[i] = originalTextsForGroup[i]
				} else if currentGroupTranslations[i] == "" && strings.TrimSpace(originalTextsForGroup[i]) != "" {
					// 如果翻译结果是空字符串，但原文不为空，也使用原文（或者根据需求定义行为）
					// 这种情况也可能发生在LLM对某些内容（如纯粹的名称/代码）返回空翻译时
					logger.Warn("节点翻译结果为空，原文非空，使用原文", zap.String("nodeMarker", fmt.Sprintf("@@NODE_%d@@", i)), zap.String("originalText", originalTextsForGroup[i]))
					currentGroupTranslations[i] = originalTextsForGroup[i]
				}
			}

			groupTranslationResults[gIdx] = currentGroupTranslations
		}(groupIndex, groupData)
	}

	wg.Wait()

	// 检查是否有翻译错误
	for groupIndex, err := range groupTranslationErrors {
		if err != nil {
			logger.Warn("HTML节点组翻译失败", zap.Error(err), zap.Int("groupIndex", groupIndex))
		}
	}

	// 应用翻译结果到DOM
	for groupIndex, groupData := range groups {
		if groupIndex >= len(groupTranslationResults) {
			continue
		}
		translationsForThisGroup := groupTranslationResults[groupIndex]
		for nodeIdxInGroup, nodeInfo := range groupData {
			translatedContent, ok := translationsForThisGroup[nodeIdxInGroup]
			if !ok || translatedContent == "" {
				translatedContent = nodeInfo.Text
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

	// 注意：我们先不恢复保护的内容，因为后续的XML/HTML清理可能会影响这些内容
	// 我们会在最终输出前恢复这些内容

	// 对于纯XML文件（有XML声明但没有DOCTYPE），goquery可能会添加<html><head></head><body></body></html>结构
	// 我们需要移除这些，只保留body内的内容，或者更精确地，只保留原始XML的根元素下的内容。
	if hasXMLDeclaration { // 如果原始文件有XML声明，我们就认为它更偏向XML
		// 尝试从body获取内容
		bodyContent, err := doc.Find("body").Html()
		if err == nil && strings.TrimSpace(bodyContent) != "" {
			// 检查body内容是否就是原始文档的全部（可能被包裹了）
			// 或者body内容看起来是多个顶层XML元素
			// 这是一个启发式方法：如果body内容不是单个被包裹的元素，就用它
			tempDocForBody, tempErrBody := goquery.NewDocumentFromReader(strings.NewReader(bodyContent))
			if tempErrBody == nil {
				// 计算body下的直接子元素数量
				directChildrenCount := tempDocForBody.Find("body").Children().Length()
				if directChildrenCount == 0 && tempDocForBody.Find("body").Text() != "" { // body只包含文本
					htmlResult = bodyContent
				} else if directChildrenCount > 1 { // 多个兄弟节点直接在body下
					htmlResult = bodyContent
				} else if directChildrenCount == 1 {
					// 单个子元素，检查它是否是goquery自动添加的包装器
					// 如果不是 html, head, body，则认为是有效内容
					childNodeName := tempDocForBody.Find("body").Children().First().Get(0).Data
					if childNodeName != "html" && childNodeName != "head" && childNodeName != "body" {
						htmlResult = bodyContent
					} else {
						// 可能是goquery添加的结构，尝试获取更原始的htmlResult
						fullHtml, fullHtmlErr := doc.Html()
						if fullHtmlErr == nil {
							cleanedFullHtml := cleanGoqueryWrapper(fullHtml)
							if strings.TrimSpace(cleanedFullHtml) != "" {
								htmlResult = cleanedFullHtml
							}
							// 如果清理后还是空的，但body content不是空的，可能body content就是我们想要的
							// 尤其当原始输入就没有html/body标签时。
							if strings.TrimSpace(htmlResult) == "" && strings.TrimSpace(bodyContent) != "" {
								isLikelyFragment := !strings.Contains(strings.ToLower(htmlStr), "<html") && !strings.Contains(strings.ToLower(htmlStr), "<body")
								if isLikelyFragment {
									htmlResult = bodyContent
								}
							}
						}
					}
				}
			}
		} else if err != nil {
			logger.Warn("尝试获取body HTML内容失败（用于XML清理）", zap.Error(err))
			// 获取body失败，尝试从原始doc.Html()清理
			fullHtml, fullHtmlErr := doc.Html()
			if fullHtmlErr == nil {
				cleanedFullHtml := cleanGoqueryWrapper(fullHtml)
				if strings.TrimSpace(cleanedFullHtml) != "" {
					htmlResult = cleanedFullHtml
				}
			}
		}
		// 如果 htmlResult 仍然是空的，但原始 htmlStr 有内容，
		// 并且原始输入看起来是XML片段，尝试只获取body的内容。
		// 这是一种最后的尝试，确保XML片段不会被完全丢弃。
		if strings.TrimSpace(htmlResult) == "" && strings.TrimSpace(htmlStr) != "" {
			isLikelyFragment := !strings.Contains(strings.ToLower(htmlStr), "<html") && !strings.Contains(strings.ToLower(htmlStr), "<body") && !hasDOCTYPE
			if isLikelyFragment {
				bodyContent, err := doc.Find("body").Html()
				if err == nil && strings.TrimSpace(bodyContent) != "" {
					htmlResult = bodyContent
				}
			}
		}
	}

	// 使用 strings.Builder 构建最终输出，确保声明顺序正确
	var finalOutputBuilder strings.Builder
	if hasXMLDeclaration && xmlDeclaration != "" {
		finalOutputBuilder.WriteString(xmlDeclaration)
		finalOutputBuilder.WriteString("\n") // 确保换行
	}
	if hasDOCTYPE && doctypeDeclaration != "" {
		finalOutputBuilder.WriteString(doctypeDeclaration)
		finalOutputBuilder.WriteString("\n") // 确保换行
	}

	// 在添加htmlResult之前，确保它不会意外地以XML或DOCTYPE声明开头
	// （因为这些应该已经被我们显式添加了）
	htmlResult = regexp.MustCompile(`(?i)<\?xml[^>]*>\s*`).ReplaceAllString(htmlResult, "")
	htmlResult = regexp.MustCompile(`(?i)<!DOCTYPE[^>]*>\s*`).ReplaceAllString(htmlResult, "")

	// 恢复所有被保护的内容
	for placeholder, original := range protected {
		htmlResult = strings.ReplaceAll(htmlResult, placeholder, original)
	}

	finalOutputBuilder.WriteString(htmlResult)

	return finalOutputBuilder.String(), nil
}

// cleanGoqueryWrapper 移除 goquery 可能添加的 html/head/body 包装器
func cleanGoqueryWrapper(htmlStr string) string {
	tempDoc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr // 解析失败，返回原样
	}

	// 检查是否存在 <html><body>...</body></html> 结构
	htmlSel := tempDoc.Find("html")
	bodySel := tempDoc.Find("body")

	if htmlSel.Length() > 0 && bodySel.Length() > 0 {
		// 情况1: <html><body>Content</body></html>
		if htmlSel.Children().Length() == 1 && htmlSel.Children().Is("body") {
			bodyHTML, _ := bodySel.Html()
			if strings.TrimSpace(bodyHTML) != "" {
				return bodyHTML
			}
		}
		// 情况2: <html><head>...</head><body>Content</body></html>
		if htmlSel.Children().Length() == 2 && htmlSel.Children().First().Is("head") && htmlSel.Children().Last().Is("body") {
			bodyHTML, _ := bodySel.Html()
			if strings.TrimSpace(bodyHTML) != "" {
				return bodyHTML
			}
		}
	}
	// 如果没有找到典型的html/body包装结构，或者body内容为空，返回原始解析的html
	// （可能是一个片段，或者goquery没有进行包装）
	cleanedHtml, err := tempDoc.Html()
	if err == nil && strings.TrimSpace(cleanedHtml) != "" {
		// 进一步清理，确保在提取内容后，不会意外地保留外层的<html>或<body>标签，如果它们是唯一的顶层标签。
		// 例如，如果结果是 "<html>Actual Content</html>", 我们想要 "Actual Content"
		// 注意：这里的清理比较粗略，主要针对goquery可能产生的特定结构
		reHtml := regexp.MustCompile(`(?is)^\s*<html(?:[^>]*)>(.*)</html>\s*$`)
		if matches := reHtml.FindStringSubmatch(cleanedHtml); len(matches) > 1 {
			cleanedHtml = matches[1]
		}
		reBody := regexp.MustCompile(`(?is)^\s*<body(?:[^>]*)>(.*)</body>\s*$`)
		if matches := reBody.FindStringSubmatch(cleanedHtml); len(matches) > 1 {
			cleanedHtml = matches[1]
		}
		return cleanedHtml
	}
	return htmlStr // 作为最后的保障
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
