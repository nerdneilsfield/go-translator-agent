package formats

import (
	"fmt"
	"regexp"
	"strings"

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
		xmlDeclRegex := regexp.MustCompile(`<\?xml[^>]*\?>`)
		if match := xmlDeclRegex.FindString(htmlStr); match != "" {
			xmlDeclaration = match
			logger.Debug("找到XML声明", zap.String("declaration", xmlDeclaration))
			// 从HTML中移除XML声明，以避免重复
			htmlStr = xmlDeclRegex.ReplaceAllString(htmlStr, "")
		}
	}

	// 检查是否包含DOCTYPE声明（不区分大小写）
	hasDOCTYPE := strings.Contains(strings.ToLower(htmlStr), "<!doctype") || strings.Contains(htmlStr, "<!DOCTYPE")
	var doctypeDeclaration string
	if hasDOCTYPE {
		// 使用不区分大小写的正则表达式匹配DOCTYPE声明
		doctypeRegex := regexp.MustCompile(`(?i)<!DOCTYPE[^>]*>`)
		if match := doctypeRegex.FindString(htmlStr); match != "" {
			doctypeDeclaration = match
			logger.Debug("找到DOCTYPE声明", zap.String("doctype", doctypeDeclaration))
			// 从HTML中移除DOCTYPE声明，以避免重复
			htmlStr = doctypeRegex.ReplaceAllString(htmlStr, "")
		}
	}

	// 使用goquery解析HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return "", fmt.Errorf("解析HTML失败: %w", err)
	}

	// 获取配置
	cfg := t.GetConfig()
	chunkSize := 6000
	if cfg != nil {
		if modelCfg, ok := cfg.ModelConfigs[cfg.DefaultModelName]; ok {
			if modelCfg.MaxInputTokens > 0 {
				chunkSize = modelCfg.MaxInputTokens - 2000
				if chunkSize <= 0 {
					chunkSize = modelCfg.MaxInputTokens / 2
				}
			}
		}
	}

	// 保护特定内容，避免被翻译
	protected := make(map[string]string)
	placeholderIndex := 0

	// 保护脚本、样式、代码块等内容
	protectRegexes := []*regexp.Regexp{
		regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`),
		regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`),
		regexp.MustCompile(`(?s)<pre[^>]*>.*?</pre>`),
		regexp.MustCompile(`(?s)<code[^>]*>.*?</code>`),
		regexp.MustCompile(`(?s)<!--.*?-->`), // 保护HTML注释
	}

	// 记录找到的脚本和样式标签

	for _, re := range protectRegexes {
		matches := re.FindAllString(htmlStr, -1)
		for _, match := range matches {
			placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
			protected[placeholder] = match

			// 记录脚本和样式标签
			if strings.HasPrefix(match, "<script") {
				logger.Debug("保护脚本标签", zap.String("script", match[:30]+"..."))
			} else if strings.HasPrefix(match, "<style") {
				logger.Debug("保护样式标签", zap.String("style", match[:30]+"..."))
			}

			htmlStr = strings.Replace(htmlStr, match, placeholder, 1)
			placeholderIndex++
		}
	}

	// 重新解析处理过的HTML
	doc, err = goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return "", fmt.Errorf("解析HTML失败: %w", err)
	}

	// 创建一个映射来存储需要翻译的文本节点
	type TextNodeInfo struct {
		Selection *goquery.Selection
		Text      string
		Path      string
		Format    NodeFormatInfo // 保存格式信息
	}

	var textNodes []TextNodeInfo
	var skipSelectors = []string{"script", "style", "code", "pre"}

	// 递归处理所有文本节点
	var processNode func(*goquery.Selection, string)
	processNode = func(s *goquery.Selection, path string) {
		// 检查是否是需要跳过的标签
		for _, selector := range skipSelectors {
			if s.Is(selector) {
				return
			}
		}

		// 处理当前节点的直接文本内容（不包括子节点的文本）
		s.Contents().Each(func(i int, child *goquery.Selection) {
			if goquery.NodeName(child) == "#text" {
				text := child.Text()
				trimmedText := strings.TrimSpace(text)
				if trimmedText != "" {
					// 提取前导和尾随空白
					leadingWS := regexp.MustCompile(`^\s*`).FindString(text)
					trailingWS := regexp.MustCompile(`\s*$`).FindString(text)

					nodePath := fmt.Sprintf("%s[%d]", path, i)
					textNodes = append(textNodes, TextNodeInfo{
						Selection: child,
						Text:      trimmedText,
						Path:      nodePath,
						Format: NodeFormatInfo{
							LeadingWhitespace:  leadingWS,
							TrailingWhitespace: trailingWS,
						},
					})
				}
			} else if child.Is("a, p, h1, h2, h3, h4, h5, h6, li, td, th, caption, figcaption, label, button, span, div, title") {
				// 对于这些常见的包含文本的元素，检查是否有直接文本内容
				hasDirectText := false
				child.Contents().Each(func(_ int, grandchild *goquery.Selection) {
					if goquery.NodeName(grandchild) == "#text" {
						if strings.TrimSpace(grandchild.Text()) != "" {
							hasDirectText = true
						}
					}
				})

				if hasDirectText {
					// 如果有直接文本内容，递归处理子节点
					childPath := fmt.Sprintf("%s>%s[%d]", path, goquery.NodeName(child), i)
					processNode(child, childPath)
				} else {
					// 如果没有直接文本内容，但有子元素可能包含文本
					text := strings.TrimSpace(child.Text())
					if text != "" && !containsOnlyHTMLElements(child) {
						// 保存原始HTML以便后续恢复结构
						html, _ := child.Html()
						nodePath := fmt.Sprintf("%s>%s[%d]", path, goquery.NodeName(child), i)
						textNodes = append(textNodes, TextNodeInfo{
							Selection: child,
							Text:      text,
							Path:      nodePath,
							Format: NodeFormatInfo{
								OriginalHTML: html,
							},
						})
					} else {
						// 递归处理子节点
						childPath := fmt.Sprintf("%s>%s[%d]", path, goquery.NodeName(child), i)
						processNode(child, childPath)
					}
				}
			} else {
				// 递归处理其他非文本子节点
				childPath := fmt.Sprintf("%s>%s[%d]", path, goquery.NodeName(child), i)
				processNode(child, childPath)
			}
		})

		// s.Contents 已经递归处理了子节点，无需再次遍历
	}

	// 从body开始处理
	body := doc.Find("body")
	if body.Length() > 0 {
		processNode(body, "body")
	} else {
		// 如果没有body标签，从根节点开始处理
		processNode(doc.Selection, "root")
	}

	// 记录收集到的文本节点数
	logger.Info("收集到的文本节点数", zap.Int("节点数", len(textNodes)))

	// 如果没有文本节点，直接返回原始HTML
	if len(textNodes) == 0 {
		// 恢复被保护的内容
		result := htmlStr
		for placeholder, original := range protected {
			result = strings.Replace(result, placeholder, original, 1)
		}
		return result, nil
	}

	// 将文本节点分组，确保每组不超过模型的输入限制
	// 同时确保相关节点（如同一个父节点下的文本）尽量在同一组中
	var groups [][]TextNodeInfo
	var currentGroup []TextNodeInfo
	currentLen := 0

	// 创建一个映射，将节点按照其路径的前缀分组
	// 这样可以确保相关节点（如同一个父节点下的文本）尽量在同一组中
	pathPrefixMap := make(map[string][]TextNodeInfo)

	// 提取路径前缀（父节点路径）
	for _, node := range textNodes {
		parts := strings.Split(node.Path, ">")
		if len(parts) > 1 {
			// 使用父节点路径作为前缀
			prefix := strings.Join(parts[:len(parts)-1], ">")
			pathPrefixMap[prefix] = append(pathPrefixMap[prefix], node)
		} else {
			// 如果没有父节点，使用节点自身的路径
			pathPrefixMap[node.Path] = append(pathPrefixMap[node.Path], node)
		}
	}

	// 按照前缀分组处理节点
	for _, nodes := range pathPrefixMap {
		prefixTextLen := 0
		for _, node := range nodes {
			prefixTextLen += len(node.Text)
		}

		// 如果当前前缀下的所有节点加起来超过限制，需要单独处理这个前缀
		if prefixTextLen > chunkSize {
			// 如果当前组不为空，先保存
			if len(currentGroup) > 0 {
				groups = append(groups, currentGroup)
				currentGroup = nil
				currentLen = 0
			}

			// 对这个前缀下的节点进行分组
			var prefixGroup []TextNodeInfo
			prefixLen := 0

			for _, node := range nodes {
				nodeLen := len(node.Text)

				// 如果单个节点超过限制，单独处理它
				if nodeLen > chunkSize {
					if len(prefixGroup) > 0 {
						groups = append(groups, prefixGroup)
						prefixGroup = nil
						prefixLen = 0
					}
					groups = append(groups, []TextNodeInfo{node})
					continue
				}

				// 如果添加当前节点会超出限制，先保存当前组
				if prefixLen+nodeLen > chunkSize && len(prefixGroup) > 0 {
					groups = append(groups, prefixGroup)
					prefixGroup = nil
					prefixLen = 0
				}

				// 添加当前节点到新组
				prefixGroup = append(prefixGroup, node)
				prefixLen += nodeLen
			}

			// 保存最后一个前缀组
			if len(prefixGroup) > 0 {
				groups = append(groups, prefixGroup)
			}
		} else {
			// 如果当前前缀下的所有节点加起来不超过限制，尝试添加到当前组
			if currentLen+prefixTextLen > chunkSize && len(currentGroup) > 0 {
				// 如果添加会超出限制，先保存当前组
				groups = append(groups, currentGroup)
				currentGroup = nil
				currentLen = 0
			}

			// 添加当前前缀下的所有节点到当前组
			currentGroup = append(currentGroup, nodes...)
			currentLen += prefixTextLen
		}
	}

	// 保存最后一个组
	if len(currentGroup) > 0 {
		groups = append(groups, currentGroup)
	}

	// 记录分组信息
	logger.Info("文本节点分组完成",
		zap.Int("组数", len(groups)),
		zap.Int("总节点数", len(textNodes)))

	// 逐组翻译文本节点
	for groupIndex, group := range groups {
		// 构建当前组的文本
		var textsToTranslate []string
		var nodeIndices []int // 记录每个文本在原始数组中的索引

		for i, node := range group {
			// 为每个节点添加一个唯一标识符，以便在翻译后能够正确匹配
			nodeMarker := fmt.Sprintf("@@NODE_%d@@", i)
			textsToTranslate = append(textsToTranslate, nodeMarker+"\n"+node.Text)
			nodeIndices = append(nodeIndices, i)
		}

		// 将所有文本合并为一个字符串，用于翻译
		groupText := strings.Join(textsToTranslate, "\n\n")

		logger.Debug("翻译文本组",
			zap.Int("组索引", groupIndex),
			zap.Int("组大小", len(group)),
			zap.Int("文本长度", len(groupText)))

		// 翻译当前组的文本
		translatedText, err := t.Translate(groupText, true)
		if err != nil {
			logger.Warn("翻译HTML节点组失败",
				zap.Error(err),
				zap.Int("组索引", groupIndex),
				zap.Int("组大小", len(group)))
			continue
		}

		// 将翻译结果分割回各个节点
		// 使用双换行符作为分隔符
		translatedParts := strings.Split(translatedText, "\n\n")

		// 创建一个映射，将节点标识符映射到翻译结果
		translationMap := make(map[int]string)

		// 解析翻译结果，提取节点标识符和对应的翻译文本
		for _, part := range translatedParts {
			// 查找节点标识符
			for i := range group {
				nodeMarker := fmt.Sprintf("@@NODE_%d@@", i)
				if strings.Contains(part, nodeMarker) {
					// 移除节点标识符，获取实际翻译文本
					translatedContent := strings.Replace(part, nodeMarker, "", 1)
					translatedContent = strings.TrimSpace(translatedContent)
					if translatedContent != "" {
						translationMap[i] = translatedContent
					}
					break
				}
			}
		}

		// 如果没有找到任何翻译结果，尝试直接使用整个翻译文本
		if len(translationMap) == 0 && len(group) > 0 {
			translationMap[0] = translatedText
		}

		// 如果某些节点没有对应的翻译结果，使用原始文本
		for i, nodeInfo := range group {
			if _, exists := translationMap[i]; !exists {
				translationMap[i] = nodeInfo.Text
				logger.Warn("节点没有对应的翻译结果，使用原始文本",
					zap.Int("组索引", groupIndex),
					zap.Int("节点索引", i),
					zap.String("节点路径", nodeInfo.Path))
			}
		}

		// 将翻译结果应用到各个节点
		for i, nodeInfo := range group {
			translatedContent := translationMap[i]
			if translatedContent != "" {
				// 替换节点的文本内容，保留原始格式
				if goquery.NodeName(nodeInfo.Selection) == "#text" {
					// 对于文本节点，恢复前导和尾随空白
					formattedContent := nodeInfo.Format.LeadingWhitespace +
						translatedContent +
						nodeInfo.Format.TrailingWhitespace
					nodeInfo.Selection.ReplaceWithHtml(formattedContent)
				} else if nodeInfo.Format.OriginalHTML != "" {
					// 对于包含HTML结构的元素，尝试保留原始结构
					// 创建一个临时文档来解析原始HTML
					tempDoc, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + nodeInfo.Format.OriginalHTML + "</div>"))
					if err == nil {
						// 尝试智能替换文本，保留HTML结构
						replaceTextPreservingStructure(tempDoc.Find("div"), translatedContent)
						newHTML, err := tempDoc.Find("div").Html()
						if err == nil {
							nodeInfo.Selection.SetHtml(newHTML)
						} else {
							// 如果失败，退回到简单的文本替换
							nodeInfo.Selection.SetText(translatedContent)
						}
					} else {
						// 如果解析失败，退回到简单的文本替换
						nodeInfo.Selection.SetText(translatedContent)
					}
				} else {
					// 对于其他元素节点，使用SetText方法
					nodeInfo.Selection.SetText(translatedContent)
				}
			}
		}
	}

	// 获取修改后的HTML
	htmlResult, err := doc.Html()
	if err != nil {
		return "", fmt.Errorf("生成HTML失败: %w", err)
	}

	// 恢复被保护的内容
	for placeholder, original := range protected {
		htmlResult = strings.Replace(htmlResult, placeholder, original, 1)
	}

	// 移除所有XML声明和DOCTYPE声明
	htmlResult = regexp.MustCompile(`<\?xml[^>]*\?>\s*`).ReplaceAllString(htmlResult, "")
	htmlResult = regexp.MustCompile(`<!DOCTYPE[^>]*>\s*`).ReplaceAllString(htmlResult, "")

	// 重新添加XML声明和DOCTYPE（如果有）
	if hasXMLDeclaration && xmlDeclaration != "" {
		// 添加XML声明到文件开头
		htmlResult = xmlDeclaration + "\n" + htmlResult
		logger.Debug("重新添加XML声明", zap.String("declaration", xmlDeclaration))
	}

	if hasDOCTYPE && doctypeDeclaration != "" {
		// 添加DOCTYPE声明
		if hasXMLDeclaration {
			// 如果有XML声明，在XML声明后添加DOCTYPE
			htmlResult = strings.Replace(htmlResult, xmlDeclaration, xmlDeclaration+"\n"+doctypeDeclaration, 1)
		} else {
			// 否则添加到文件开头
			htmlResult = doctypeDeclaration + "\n" + htmlResult
		}
		logger.Debug("重新添加DOCTYPE声明", zap.String("doctype", doctypeDeclaration))
	}

	// 检查并修复可能的编码问题
	htmlResult = fixEncodingIssues(htmlResult)

	return htmlResult, nil
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
