package formats

import (
	"fmt"
	"path/filepath"

	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/dlclark/regexp2"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// GoQueryNodeFormatInfo 保存节点的原始格式信息
type GoQueryNodeFormatInfo struct {
	LeadingWhitespace  string
	TrailingWhitespace string
	OriginalHTML       string // 用于保存原始HTML结构
}

type GoQueryTextNodeInfo struct {
	Selection      *goquery.Selection
	Text           string
	TranslatedText string
	Path           string
	Format         GoQueryNodeFormatInfo
	IsAttribute    bool
	AttributeName  string
	GlobalID       int
	ContextBefore  string
	ContextAfter   string
}

type GoQueryTextNodeGroup []GoQueryTextNodeInfo

type GoQueryHTMLTranslator struct {
	translator      translator.Translator
	fileName        string
	logger          *zap.Logger
	originalHTML    string
	translatedHTML  string
	shortFileName   string
	chunkSize       int
	concurrency     int
	enableRetry     bool
	maxRetries      int
	nodeExtractRegx *regexp2.Regexp
	nodeCount       int
}

func NewGoQueryHTMLTranslator(t translator.Translator, logger *zap.Logger) *GoQueryHTMLTranslator {
	// 获取配置
	agentConfig := t.GetConfig()
	chunkSize := 6000
	concurrency := 1 // 默认为1，即不进行文件内并行
	enableRetry := false
	maxRetries := 3
	if agentConfig != nil {
		if modelCfg, ok := agentConfig.ModelConfigs[agentConfig.DefaultModelName]; ok {
			if modelCfg.MaxInputTokens > 0 {
				chunkSize = modelCfg.MaxInputTokens - 1000
				if chunkSize <= 0 {
					chunkSize = modelCfg.MaxInputTokens / 2
				}
			}
		}

		if chunkSize > agentConfig.MaxSplitSize {
			chunkSize = agentConfig.MaxSplitSize
		}

		if chunkSize < agentConfig.MinSplitSize {
			chunkSize = agentConfig.MinSplitSize
		}

		logger.Debug("Chunk Size 被设置为", zap.Int("chunk_size", chunkSize))

		// 使用 HtmlConcurrency 控制单个HTML文件内部的并发
		if agentConfig.HtmlConcurrency > 0 {
			concurrency = agentConfig.HtmlConcurrency
		} else {
			logger.Debug("HtmlConcurrency未配置或为0，单个HTML文件内节点翻译将串行执行。", zap.Int("resolved_concurrency", concurrency))
		}
		enableRetry = t.GetConfig().RetryFailedParts
		maxRetries = t.GetConfig().MaxRetries
	}
	// 正则表达式：
	// (?s) 允许 . 匹配换行符
	// @@NODE_START_(\d+)@@ 匹配开始标记并捕获数字索引 (group 1)
	// \n 匹配开始标记后的换行符
	// (.*?) 懒惰匹配翻译内容，直到遇到下一个模式 (group 2)
	// \n 匹配结束标记前的换行符
	// @@NODE_END_\1@@ 使用反向引用 \1确保结束标记的数字与开始标记的数字一致
	pattern := `(?s)@@NODE_START_(\d+)@@\r?\n(.*?)\r?\n@@NODE_END_\1@@`
	re := regexp2.MustCompile(pattern, 0)
	return &GoQueryHTMLTranslator{
		translator:      t,
		logger:          logger,
		chunkSize:       chunkSize,
		concurrency:     concurrency,
		enableRetry:     enableRetry,
		maxRetries:      maxRetries,
		nodeExtractRegx: re,
	}
}

// replaceTextPreservingStructure 尝试替换文本内容，同时保留HTML结构
// 这个函数会尝试智能地将翻译后的文本分配到原始HTML结构中的文本节点
func (t *GoQueryHTMLTranslator) replaceTextPreservingStructure(selection *goquery.Selection, translatedText string) {
	// 收集所有文本节点
	var textNodes []*goquery.Selection
	selection.Contents().Each(func(_ int, s *goquery.Selection) {
		if goquery.NodeName(s) == "#text" {
			if strings.TrimSpace(s.Text()) != "" {
				textNodes = append(textNodes, s)
			}
		} else {
			// 递归处理子元素
			t.replaceTextPreservingStructure(s, "")
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
func (t *GoQueryHTMLTranslator) containsOnlyHTMLElements(s *goquery.Selection) bool {
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

func (t *GoQueryHTMLTranslator) processHTML(htmlStr string) (*goquery.Document, map[string]string, string, string, error) {
	var initialDoc *goquery.Document
	var protectedContent map[string]string
	protectedContent = make(map[string]string)
	var xmlDecl string
	var doctypeDecl string

	// 保存XML声明和DOCTYPE，因为goquery可能会移除它们
	hasXMLDeclaration := strings.Contains(htmlStr, "<?xml")
	if hasXMLDeclaration {
		xmlDeclRegex := regexp.MustCompile(`<\?xml[^>]*\?>`)
		if match := xmlDeclRegex.FindString(htmlStr); match != "" {
			xmlDecl = match
			// t.logger.Debug("找到XML声明", zap.String("declaration", xmlDecl))
			htmlStr = strings.Replace(htmlStr, xmlDecl, "", 1) //移除一次，避免影响解析
		}
	}

	hasDOCTYPE := strings.Contains(strings.ToLower(htmlStr), "<!doctype")
	if hasDOCTYPE {
		doctypeRegex := regexp.MustCompile(`(?i)<!DOCTYPE[^>]*>`)
		if match := doctypeRegex.FindString(htmlStr); match != "" {
			doctypeDecl = match
			// t.logger.Debug("找到DOCTYPE声明", zap.String("doctype", doctypeDecl))
			htmlStr = strings.Replace(htmlStr, doctypeDecl, "", 1) //移除一次
		}
	}

	t.logger.Debug("文档声明处理完成",
		zap.Bool("hasXMLDeclaration", hasXMLDeclaration),
		zap.Bool("hasDOCTYPE", hasDOCTYPE),
		zap.String("fileName", t.shortFileName),
		zap.String("preservedXmlDecl", xmlDecl),
		zap.String("preservedDoctype", doctypeDecl))

	// 使用goquery解析HTML
	initialDoc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return initialDoc, protectedContent, xmlDecl, doctypeDecl, fmt.Errorf("解析HTML失败: %w", err)
	}

	// 重要：在同一个文档实例上进行内容保护，而不是重新创建文档
	// 这样可以确保后续获取的Selection对象仍然有效
	placeholderIndex := 0

	// 保护script标签
	initialDoc.Find("script").Each(func(i int, s *goquery.Selection) {
		if html, err := s.Html(); err == nil {
			placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
			protectedContent[placeholder] = fmt.Sprintf("<script%s>%s</script>",
				getAttributesString(s), html)
			s.ReplaceWithHtml(placeholder)
			placeholderIndex++
		}
	})

	// 保护style标签
	initialDoc.Find("style").Each(func(i int, s *goquery.Selection) {
		if html, err := s.Html(); err == nil {
			placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
			protectedContent[placeholder] = fmt.Sprintf("<style%s>%s</style>",
				getAttributesString(s), html)
			s.ReplaceWithHtml(placeholder)
			placeholderIndex++
		}
	})

	// 保护pre标签
	initialDoc.Find("pre").Each(func(i int, s *goquery.Selection) {
		if html, err := goquery.OuterHtml(s); err == nil {
			placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
			protectedContent[placeholder] = html
			s.ReplaceWithHtml(placeholder)
			placeholderIndex++
		}
	})

	// 保护code标签
	initialDoc.Find("code").Each(func(i int, s *goquery.Selection) {
		if html, err := goquery.OuterHtml(s); err == nil {
			placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
			protectedContent[placeholder] = html
			s.ReplaceWithHtml(placeholder)
			placeholderIndex++
		}
	})

	// 保护页面锚点标签（如 <a class="page" id="p59"/>）
	// 这些标签对电子书导航至关重要，必须保持原始位置和格式
	initialDoc.Find("a.page").Each(func(i int, s *goquery.Selection) {
		if html, err := goquery.OuterHtml(s); err == nil {
			placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
			protectedContent[placeholder] = html
			s.ReplaceWithHtml(placeholder)
			placeholderIndex++
			// t.logger.Debug("保护页面锚点标签", zap.String("html", html), zap.String("placeholder", placeholder), zap.String("fileName", t.shortFileName))
		}
	})

	// 保护其他重要的锚点标签（如带有特定class的导航锚点）
	initialDoc.Find("a[class*='xref'], a[class*='anchor'], a[id]").Each(func(i int, s *goquery.Selection) {
		// 检查是否为空的锚点标签（只有ID或class，没有文本内容）
		text := strings.TrimSpace(s.Text())
		if text == "" {
			if html, err := goquery.OuterHtml(s); err == nil {
				placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
				protectedContent[placeholder] = html
				s.ReplaceWithHtml(placeholder)
				placeholderIndex++
				// t.logger.Debug("保护空锚点标签", zap.String("html", html), zap.String("placeholder", placeholder), zap.String("fileName", t.shortFileName))
			}
		}
	})

	// TODO: 暂时跳过HTML注释保护，因为goquery处理注释节点比较复杂
	// 注释通常不包含需要翻译的内容，我们可以稍后用更安全的方式处理
	// 如果确实需要保护注释，可以考虑在字符串级别处理，而不是DOM级别

	t.logger.Debug("保护了内容块", zap.Int("count", len(protectedContent)))

	return initialDoc, protectedContent, xmlDecl, doctypeDecl, nil
}

// 辅助函数：获取元素的属性字符串
func getAttributesString(s *goquery.Selection) string {
	if s.Length() == 0 {
		return ""
	}

	var attrs []string
	for _, attr := range s.Get(0).Attr {
		attrs = append(attrs, fmt.Sprintf(`%s="%s"`, attr.Key, attr.Val))
	}

	if len(attrs) > 0 {
		return " " + strings.Join(attrs, " ")
	}
	return ""
}

func (t *GoQueryHTMLTranslator) extractTextNodes(doc *goquery.Document) ([]GoQueryTextNodeInfo, map[int]string, error) {
	var textNodes []GoQueryTextNodeInfo
	var globalIDCounter int = 0
	allOriginalTexts := make(map[int]string)
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
					currentGlobalID := globalIDCounter
					textNodes = append(textNodes, GoQueryTextNodeInfo{
						Selection:     s,
						Text:          attr.Val,
						Path:          fmt.Sprintf("%s[@%s]", currentPath, attr.Key),
						IsAttribute:   true,
						AttributeName: attr.Key,
						GlobalID:      currentGlobalID,
					})
					allOriginalTexts[currentGlobalID] = attr.Val
					globalIDCounter = globalIDCounter + 1
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
						currentGlobalID := globalIDCounter
						formatInfo := GoQueryNodeFormatInfo{
							LeadingWhitespace:  text[:len(text)-len(strings.TrimLeft(text, " \t\n\r"))],
							TrailingWhitespace: text[len(strings.TrimRight(text, " \t\n\r")):],
						}
						textNodes = append(textNodes, GoQueryTextNodeInfo{
							Selection: child,
							Text:      trimmedText,
							Path:      fmt.Sprintf("%s/#text", currentPath),
							Format:    formatInfo,
							GlobalID:  currentGlobalID,
						})
						allOriginalTexts[currentGlobalID] = trimmedText
						globalIDCounter = globalIDCounter + 1
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

	t.nodeCount = len(textNodes)

	t.logger.Debug("收集到的可翻译节点数量", zap.Int("count", len(textNodes)), zap.String("fileName", t.shortFileName))

	return textNodes, allOriginalTexts, nil
}

func (t *GoQueryHTMLTranslator) groupNodes(textNodes []GoQueryTextNodeInfo) []GoQueryTextNodeGroup {
	var groups []GoQueryTextNodeGroup
	var currentGroup []GoQueryTextNodeInfo
	currentLength := 0
	for _, node := range textNodes {
		nodeTextLength := len(node.Text)
		if currentLength+nodeTextLength > t.chunkSize && len(currentGroup) > 0 {
			groups = append(groups, currentGroup)
			currentGroup = []GoQueryTextNodeInfo{node}
			currentLength = nodeTextLength
		} else {
			currentGroup = append(currentGroup, node)
			currentLength += nodeTextLength
		}
	}
	if len(currentGroup) > 0 {
		groups = append(groups, currentGroup)
	}

	t.logger.Debug("分组后的节点数量", zap.Int("groups", len(groups)))
	return groups
}

func (t *GoQueryHTMLTranslator) parseGroupTranslation(groupTranslatedText string, validNodeIDs []int) (map[int]string, error) {
	nodeIDWithTranslation := make(map[int]string)

	// 创建有效节点ID的快速查找map
	validIDsMap := make(map[int]bool)
	for _, id := range validNodeIDs {
		validIDsMap[id] = true
	}

	// 调试信息：记录收到的翻译文本
	// t.logger.Debug("解析翻译结果",
	// 	zap.Int("textLength", len(groupTranslatedText)),
	// 	zap.Ints("validNodeIDs", validNodeIDs),
	// 	zap.String("textSnippet", snippet(groupTranslatedText)))

	var m *regexp2.Match
	m, err := t.nodeExtractRegx.FindStringMatch(groupTranslatedText) // 找到第一个匹配
	if err != nil {
		t.logger.Error("regexp2 查找匹配时出错", zap.Error(err), zap.String("fileName", t.shortFileName))
		return nodeIDWithTranslation, fmt.Errorf("regexp2 查找匹配时出错: %w", err)
	}
	// matches 是一个 [][]string，每个元素是：
	// [ 完整匹配的字符串, 捕获的数字N, 捕获的翻译内容 ]

	detectedNodeCount := 0

	if m == nil {
		t.logger.Debug("没有找到匹配", zap.String("fileName", t.shortFileName))
		return nodeIDWithTranslation, nil
	}

	for m != nil { // 循环直到没有更多匹配
		groups := m.Groups()
		if len(groups) == 3 { // Group 0 是完整匹配, Group 1 是第一个捕获组, Group 2 是第二个
			nodeIndexStr := groups[1].Capture.String()
			rawContent := groups[2].Capture.String()

			// --- 从这里开始是你之前的 rawContent 处理逻辑 ---
			processedContent := strings.TrimSpace(rawContent)
			translation := strings.TrimSpace(processedContent)
			// --- rawContent 处理逻辑结束 ---

			nodeIndex, errAtoi := strconv.Atoi(nodeIndexStr)
			if errAtoi != nil {
				t.logger.Warn("无法从标记解析节点索引", zap.String("indexStr", nodeIndexStr), zap.Error(errAtoi), zap.String("fileName", t.shortFileName))
			} else {
				// 使用validIDsMap而不是全局范围检查
				if validIDsMap[nodeIndex] {
					//t.logger.Debug("检测到的节点", zap.Int("nodeIndex", nodeIndex), zap.String("translation", translation))
					nodeIDWithTranslation[nodeIndex] = translation
					detectedNodeCount++
				} else {
					t.logger.Warn("解析到的节点索引不在当前组的有效范围内",
						zap.Int("nodeIndex", nodeIndex),
						zap.Ints("validNodeIDs", validNodeIDs),
						zap.String("nodeIndexStr", nodeIndexStr),
						zap.String("translationSnippet", snippet(translation)),
						zap.String("fileName", t.shortFileName))
				}
			}
		} else {
			t.logger.Warn("regexp2 匹配结果的捕获组数量不符合预期", zap.Int("groupsCount", len(groups)), zap.String("fileName", t.shortFileName))
		}

		m, err = t.nodeExtractRegx.FindNextMatch(m) // 查找下一个匹配
		if err != nil {
			t.logger.Error("regexp2 查找下一个匹配时出错", zap.Error(err))
			break // 出错则停止查找
		}
	}
	t.logger.Debug("检测到的节点数量", zap.Int("detectedNodeCount", detectedNodeCount), zap.String("fileName", t.shortFileName))
	return nodeIDWithTranslation, nil
}

func (t *GoQueryHTMLTranslator) translateGroup(nodeGroup GoQueryTextNodeGroup, textNodes []GoQueryTextNodeInfo) (GoQueryTextNodeGroup, []error) {
	var errors []error

	// 调试信息：显示当前组的节点ID范围
	var nodeIDs []int
	for _, node := range nodeGroup {
		nodeIDs = append(nodeIDs, node.GlobalID)
	}
	// t.logger.Debug("开始翻译节点组",
	// 	zap.Ints("nodeIDs", nodeIDs),
	// 	zap.Int("nodeCount", t.nodeCount),
	// 	zap.Int("groupSize", len(nodeGroup)))

	var textsToTranslate string

	var returnNodeGroup GoQueryTextNodeGroup
	returnNodeGroup = append(returnNodeGroup, textNodes...)

	var groupTextNodesToTranslate []string
	for _, node := range nodeGroup {
		currentGlobalID := node.GlobalID
		startMarker := fmt.Sprintf("@@NODE_START_%d@@", currentGlobalID)
		endMarker := fmt.Sprintf("@@NODE_END_%d@@", currentGlobalID)
		groupTextNodesToTranslate = append(groupTextNodesToTranslate, startMarker+"\n"+node.Text+"\n"+endMarker)
	}
	groupText := strings.Join(groupTextNodesToTranslate, "\n\n")
	groupText = strings.TrimSpace(groupText)
	textsToTranslate = groupText

	// t.logger.Debug("开始翻译HTML文本", zap.Int("textLength", len(textsToTranslate)))
	translatedText, err := t.translator.Translate(textsToTranslate, t.enableRetry)
	if err != nil {
		errors = append(errors, err)
		t.logger.Warn("翻译HTML节点组失败", zap.Error(err))
		return returnNodeGroup, errors
	}

	// 把翻译结果绑定到 Node 上面去

	nodeIDWithTranslation, err := t.parseGroupTranslation(translatedText, nodeIDs)
	if err != nil {
		t.logger.Error("解析翻译结果失败", zap.Error(err))
	}

	// 验证翻译结果的完整性
	processedNodes := 0
	missingNodes := []int{}
	for _, node := range nodeGroup {
		if translation, exists := nodeIDWithTranslation[node.GlobalID]; exists {
			if translation == textNodes[node.GlobalID].Text {
				// 即使翻译结果与原文相同，也要设置TranslatedText
				// 这样可以确保节点被标记为"已处理"
				returnNodeGroup[node.GlobalID].TranslatedText = translation
			} else {
				returnNodeGroup[node.GlobalID].TranslatedText = translation
			}
			processedNodes++
		} else {
			missingNodes = append(missingNodes, node.GlobalID)
		}
	}

	if len(missingNodes) > 0 {
		t.logger.Warn("某些节点没有被翻译器处理",
			zap.Ints("missingNodeIDs", missingNodes),
			zap.Int("processedNodes", processedNodes),
			zap.Int("totalNodes", len(nodeGroup)))
	}

	// t.logger.Debug("节点组翻译完成",
	// 	zap.Int("processedNodes", processedNodes),
	// 	zap.Int("totalNodes", len(nodeGroup)),
	// 	zap.Int("missingNodes", len(missingNodes)))

	return returnNodeGroup, errors
}

func (t *GoQueryHTMLTranslator) collectFailedNodes(nodeGroup GoQueryTextNodeGroup) []int {
	var failedGroup GoQueryTextNodeGroup
	for _, node := range nodeGroup {
		if node.TranslatedText == "" {
			failedGroup = append(failedGroup, node)
		}
	}
	var failedNodeIDs []int
	for _, node := range failedGroup {
		failedNodeIDs = append(failedNodeIDs, node.GlobalID)
	}
	return failedNodeIDs
}

func (t *GoQueryHTMLTranslator) groupFailedNodes(totalNodes GoQueryTextNodeGroup, failedNodeIDs []int) []GoQueryTextNodeGroup {
	// 使用IntSet来去重
	nodeSet := NewIntSet()

	// 为每个失败的节点添加上下文（前一个、当前、后一个）
	for _, nodeID := range failedNodeIDs {
		// 添加前一个节点（如果存在）
		if nodeID > 0 {
			nodeSet.Add(nodeID - 1)
		}
		// 添加当前失败的节点
		nodeSet.Add(nodeID)
		// 添加后一个节点（如果存在）
		if nodeID < len(totalNodes)-1 {
			nodeSet.Add(nodeID + 1)
		}
	}

	// 获取去重并排序后的节点ID
	uniqueNodeIDs := nodeSet.ToSlice()

	// 构建失败节点及其上下文的数组
	var failedNodeWithContext GoQueryTextNodeGroup
	for _, nodeID := range uniqueNodeIDs {
		if nodeID >= 0 && nodeID < len(totalNodes) {
			failedNodeWithContext = append(failedNodeWithContext, totalNodes[nodeID])
		}
	}

	// t.logger.Debug("重试节点去重统计",
	// 	zap.Int("原始失败节点数", len(failedNodeIDs)),
	// 	zap.Int("加上下文后总节点数", len(failedNodeWithContext)),
	// 	zap.Int("set大小", nodeSet.Size()),
	// 	zap.Ints("失败节点IDs", failedNodeIDs),
	// 	zap.Ints("去重后节点IDs", uniqueNodeIDs))

	return t.groupNodes(failedNodeWithContext)
}

func (t *GoQueryHTMLTranslator) resortNodes(nodeGroup GoQueryTextNodeGroup, len int) GoQueryTextNodeGroup {
	newNodeGroup := make(GoQueryTextNodeGroup, len)
	for _, node := range nodeGroup {
		if node.GlobalID >= len {
			t.logger.Warn("节点ID超出范围", zap.Int("nodeID", node.GlobalID), zap.Int("len", len))
			continue
		}
		newNodeGroup[node.GlobalID] = node
	}
	return newNodeGroup
}

func (t *GoQueryHTMLTranslator) updateNodes(originalNodeGroup GoQueryTextNodeGroup, newNodeGroup GoQueryTextNodeGroup) GoQueryTextNodeGroup {
	for _, node := range newNodeGroup {
		originalNodeGroup[node.GlobalID].TranslatedText = node.TranslatedText
	}
	return originalNodeGroup
}

func (t *GoQueryHTMLTranslator) updateNodesWithFailedIDs(originalNodeGroup GoQueryTextNodeGroup, newNodeGroup GoQueryTextNodeGroup, failedNodeIDs []int) GoQueryTextNodeGroup {
	for _, failedNodeID := range failedNodeIDs {
		if failedNodeID < len(newNodeGroup) && newNodeGroup[failedNodeID].TranslatedText != "" {
			originalNodeGroup[failedNodeID].TranslatedText = newNodeGroup[failedNodeID].TranslatedText
		}
	}
	return originalNodeGroup
}

func (t *GoQueryHTMLTranslator) applyTranslations(doc *goquery.Document, translatedNodes GoQueryTextNodeGroup) (*goquery.Document, error) {
	if (doc == nil) || (len(translatedNodes) == 0) {
		t.logger.Error("无法应用翻译，原始文档或节点组为空", zap.Int("nodeGroup", len(translatedNodes)), zap.String("fileName", t.shortFileName))
		return nil, fmt.Errorf("无法应用翻译，原始文档或节点组为空")
	}

	// 统计翻译状态
	var nodesWithTranslation int
	var nodesWithoutTranslation int
	var nodesWithNilSelection int

	for _, nodeInfo := range translatedNodes {
		if nodeInfo.Selection == nil {
			nodesWithNilSelection++
		} else if nodeInfo.TranslatedText != "" {
			nodesWithTranslation++
		} else {
			nodesWithoutTranslation++
		}
	}

	// t.logger.Debug("应用翻译统计信息",
	// 	zap.Int("totalNodes", len(translatedNodes)),
	// 	zap.Int("nodesWithTranslation", nodesWithTranslation),
	// 	zap.Int("nodesWithoutTranslation", nodesWithoutTranslation),
	// 	zap.Int("nodesWithNilSelection", nodesWithNilSelection))

	for i, nodeInfo := range translatedNodes {
		if nodeInfo.Selection == nil {
			// t.logger.Warn("跳过节点，因为其Selection为nil",
			// 	zap.Int("node_index_in_group", i),
			// 	zap.Int("global_id", nodeInfo.GlobalID),
			// 	zap.String("path", nodeInfo.Path))
			continue
		}

		// 确定最终要使用的文本：
		// 优先使用 TranslatedText。如果 TranslatedText 为空但 OriginalText 不为空，
		// 这表示翻译失败或LLM返回空，并且之前的逻辑已经决定使用原文。
		// 这里的假设是：translatedNodes 列表中的 TranslatedText 已经是最终应该写入DOM的文本。
		// 如果 TranslatedText 可能为空代表"删除此内容"，则逻辑需要调整。
		// 但通常，如果翻译为空而原文不为空，之前的步骤应该已经将 OriginalText 填充到 TranslatedText 中了。
		finalTextToApply := nodeInfo.TranslatedText
		// isUsingOriginalText := false
		if finalTextToApply == "" && strings.TrimSpace(nodeInfo.Text) != "" {
			// 这一步是一个额外的保险，理论上在填充 translatedNodes 列表时就应该处理好这种情况了
			// 即，如果翻译结果为空，TranslatedText 字段应该已经被设置为 OriginalText。
			// 但如果 TranslatedText 可能有意为空（例如，要删除某段文字），则此处的逻辑需要调整。
			// 为了安全，如果 TranslatedText 为空但原始文本不为空，我们这里可以再次确认使用原始文本。
			// 但更推荐的是，调用此函数前，translatedNodes[i].TranslatedText 就已是最终确定的文本。
			// t.logger.Debug("节点的TranslatedText为空，但OriginalText非空。将使用OriginalText。",
			// 	zap.Int("global_id", nodeInfo.GlobalID),
			// 	zap.String("original_text_snippet", snippet(nodeInfo.Text)))
			finalTextToApply = nodeInfo.Text
			// isUsingOriginalText = true
		}

		if nodeInfo.IsAttribute {
			// 更新属性值
			if nodeInfo.AttributeName == "" {
				// t.logger.Warn("跳过属性更新，因为AttributeName为空",
				// 	zap.Int("global_id", nodeInfo.GlobalID),
				// 	zap.String("path", nodeInfo.Path))
				continue
			}
			nodeInfo.Selection.SetAttr(nodeInfo.AttributeName, finalTextToApply)
			// t.logger.Debug("已更新属性",
			// 	zap.Int("global_id", nodeInfo.GlobalID),
			// 	zap.String("path", nodeInfo.Path),
			// 	zap.String("attribute", nodeInfo.AttributeName),
			// 	zap.Bool("isUsingOriginalText", isUsingOriginalText),
			// 	zap.String("new_value_snippet", snippet(finalTextToApply)))
		} else {
			// 更新文本节点内容，同时保留原始的前后空白
			// 确保 finalTextToApply 不包含意外的前后空白（除非这些空白是翻译内容的一部分）
			// NodeFormatInfo 中的 LeadingWhitespace 和 TrailingWhitespace 是基于原始文本的
			// 我们应该将它们应用于 finalTextToApply
			contentWithOriginalWhitespace := nodeInfo.Format.LeadingWhitespace + finalTextToApply + nodeInfo.Format.TrailingWhitespace

			// 使用 goquery 的 ReplaceWithHtml 或 SetHtml 来替换文本节点内容
			// ReplaceWithHtml 更适合替换整个节点，包括其HTML结构（如果内容是HTML的话）
			// 对于纯文本节点，SetHtml(content) 或 Text(content) 通常也可以，
			// 但 ReplaceWithHtml 可以更好地处理需要转义的字符，或者如果内容意外地是HTML。
			// 为了保留原始文本节点的"位置"并只改变其内容，
			// 且考虑到内容可能是纯文本并需要保留前后空白，
			// 直接修改文本节点的内容通常是 goquery.Selection.SetText() 或 .SetHtml()
			// 如果 Selection 指向的是 #text 节点本身：
			if goquery.NodeName(nodeInfo.Selection) == "#text" {
				// 对于文本节点，使用 ReplaceWithHtml 完全替换节点内容
				nodeInfo.Selection.ReplaceWithHtml(contentWithOriginalWhitespace)

				// 立即验证更新是否生效 - 注意：由于节点被替换了，我们需要重新获取
				// ReplaceWithHtml 会替换当前节点，所以我们无法直接验证
				// 但可以通过父元素来验证
				// parent := nodeInfo.Selection.Parent()
				// updatedContent := parent.Text()

				// t.logger.Trace("已更新文本节点 (直接#text)",
				// 	zap.Int("global_id", nodeInfo.GlobalID),
				// 	zap.String("path", nodeInfo.Path),
				// 	zap.Bool("isUsingOriginalText", isUsingOriginalText),
				// 	zap.String("new_content_snippet", snippet(contentWithOriginalWhitespace)),
				// 	zap.String("parent_content_snippet", snippet(updatedContent)))
			} else {
				// 如果 Selection 指向的是元素节点，而我们要更新的是它内部的文本（通常 textNodes 应该直接指向 #text 节点）
				// 这种情况理论上不应该发生，因为 TextNodeInfo.Selection 应该就是文本节点本身或包含属性的元素。
				// 但为了健壮性，如果它是一个元素，并且你想替换它的所有文本内容：
				// nodeInfo.Selection.SetText(contentWithOriginalWhitespace) // 这会移除所有子元素
				// 更安全的是，如果 TextNodeInfo.Selection 确实指向包含该文本的元素，
				// 而不是文本节点本身，那么最初的 Selection 就应该调整。
				// 假设你的 TextNodeInfo.Selection 总是精确指向要修改的 #text 节点（对于非属性情况）
				// 或者指向要修改属性的元素节点（对于属性情况）。
				// 如果上面的 if goquery.NodeName(nodeInfo.Selection) == "#text" 不成立，说明 IsAttribute=false 但 Selection 不是文本节点，这可能是一个逻辑问题。
				// t.logger.Warn("尝试更新非属性、非文本节点的文本内容，这可能不符合预期",
				// 	zap.Int("global_id", nodeInfo.GlobalID),
				// 	zap.String("path", nodeInfo.Path),
				// 	zap.String("node_name", goquery.NodeName(nodeInfo.Selection)))
				// 作为一种回退或通用处理，可以尝试 nodeInfo.Selection.SetText()
				// 但要注意这会清除该元素的所有子节点，只留下文本。
				// 如果你的 TextNodeInfo.Format.OriginalHTML 字段有意义并且被填充了（用于复杂结构替换），
				// 那么这里可以加入那个逻辑。
				// if nodeInfo.Format.OriginalHTML != "" { ... }
				nodeInfo.Selection.SetText(finalTextToApply) // 简化处理，但可能不适用于所有情况
				// t.logger.Debug("已更新非文本节点 (回退处理)",
				// 	zap.Int("global_id", nodeInfo.GlobalID),
				// 	zap.String("path", nodeInfo.Path),
				// 	zap.String("node_name", goquery.NodeName(nodeInfo.Selection)),
				// 	zap.Bool("isUsingOriginalText", isUsingOriginalText),
				// 	zap.String("new_content_snippet", snippet(finalTextToApply)))
			}
		}
	}

	// 验证整个文档的更新状态
	t.logger.Debug("验证文档更新状态...")
	docHTML, err := doc.Html()
	if err != nil {
		t.logger.Error("验证时获取文档HTML失败", zap.Error(err))
	} else {
		// 检查是否包含一些翻译后的中文内容
		containsChinese := len(docHTML) != len([]rune(docHTML)) // 简单检查是否包含非ASCII字符
		t.logger.Debug("文档验证结果",
			zap.Bool("containsNonASCII", containsChinese),
			zap.Int("docLength", len(docHTML)),
			zap.String("docSnippet", snippetWithLength(docHTML, 500)))
	}

	return doc, nil
}

// cleanGoqueryWrapper 辅助函数 (你可能需要根据实际情况调整或完善)
// 它的目的是尝试移除 goquery 可能为 XML 片段添加的多余的 html/body 包装
func (t *GoQueryHTMLTranslator) cleanGoqueryWrapper(htmlStr string, originalFullHTMLString string) string {
	// 这是一个复杂的启发式过程，可能需要根据你的具体 XML 结构进行调整
	// 目标：如果原始输入是XML片段，但goquery将其包装在html/body中，尝试还原。
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		t.logger.Warn("cleanGoqueryWrapper: 解析htmlStr失败", zap.Error(err), zap.String("html_snippet", snippet(htmlStr)))
		return htmlStr // 解析失败，返回原样
	}

	// 如果原始字符串本身就不像一个完整的HTML文档
	isLikelyFragment := !strings.Contains(strings.ToLower(originalFullHTMLString), "<html") &&
		!strings.Contains(strings.ToLower(originalFullHTMLString), "<body")

	bodySelection := doc.Find("body")
	if bodySelection.Length() > 0 {
		// 检查 body 是否是 goquery 添加的唯一顶层元素 (在 html > body 结构中)
		// 并且原始输入看起来像片段
		if isLikelyFragment && bodySelection.Parent().Is("html") && bodySelection.Parent().Children().Length() == 1 { // body是html唯一的孩子
			if bodyHtml, err := bodySelection.Html(); err == nil {
				t.logger.Debug("cleanGoqueryWrapper: 原始输入像片段，且body是html唯一子元素，返回body内容", zap.String("body_html_snippet", snippet(bodyHtml)))
				return bodyHtml
			}
		}
		// 另一种情况：如果body内容本身看起来就是多个XML元素或者纯文本
		children := bodySelection.Children()
		if children.Length() > 1 || (children.Length() == 0 && strings.TrimSpace(bodySelection.Text()) != "") {
			if bodyHtml, err := bodySelection.Html(); err == nil {
				t.logger.Debug("cleanGoqueryWrapper: body包含多个子元素或纯文本，返回body内容", zap.String("body_html_snippet", snippet(bodyHtml)))
				return bodyHtml
			}
		}
		// 如果body只有一个子元素，且这个子元素不是html/head/body，也可能是有效的XML片段
		if children.Length() == 1 && children.Filter("html,head,body").Length() == 0 {
			if bodyHtml, err := bodySelection.Html(); err == nil {
				t.logger.Debug("cleanGoqueryWrapper: body只有一个有效子元素，返回body内容", zap.String("body_html_snippet", snippet(bodyHtml)))
				return bodyHtml
			}
		}
	}

	// 如果以上启发式方法都不适用，或者解析/获取body内容失败，
	// 返回由 doc.Html() 生成的（可能已被goquery规范化的）完整HTML。
	// 这意味着对于某些复杂的XML结构，可能仍然会保留html/body包装。
	t.logger.Debug("cleanGoqueryWrapper: 未应用特定清理规则，返回goquery生成的HTML", zap.String("html_snippet", snippet(htmlStr)))
	return htmlStr
}

func (t *GoQueryHTMLTranslator) postprocessHTML(doc *goquery.Document, protectedContent map[string]string, xmlDecl string, doctypeDecl string, originalFullHTMLString string) (string, error) {
	if t.logger == nil {
		return "", fmt.Errorf("logger is not initialized in GoQueryHTMLTranslator for postprocessing")
	}

	htmlResult, err := doc.Html()
	if err != nil {
		return "", fmt.Errorf("生成最终HTML失败 (postprocess阶段初始获取): %w", err)
	}
	t.logger.Debug("postprocessHTML: 从doc获取的初始htmlResult", zap.String("snippet", snippetWithLength(htmlResult, 300)))

	// 暂时移除复杂的XML清理逻辑，因为它会导致CSS样式丢失
	// 对于XHTML文档，保留完整的HTML结构
	t.logger.Debug("postprocessHTML: 保留完整HTML结构，不进行XML清理")

	// --- 恢复 @@PROTECTED_N@@ 内容 ---
	if len(protectedContent) > 0 {
		t.logger.Debug("postprocessHTML: 开始恢复受保护内容", zap.Int("count", len(protectedContent)))
		for placeholder, original := range protectedContent {
			htmlResult = strings.ReplaceAll(htmlResult, placeholder, original)
		}
		t.logger.Debug("postprocessHTML: 受保护内容已恢复", zap.String("snippet_after_restore", snippet(htmlResult)))
	}
	// --- 恢复结束 ---

	// --- 构建最终输出，确保声明顺序正确 ---
	var finalOutputBuilder strings.Builder
	if xmlDecl != "" {
		finalOutputBuilder.WriteString(xmlDecl)
		finalOutputBuilder.WriteString("\n") // 确保换行
	}
	if doctypeDecl != "" {
		finalOutputBuilder.WriteString(doctypeDecl)
		finalOutputBuilder.WriteString("\n") // 确保换行
	}

	// 在添加htmlResult之前，再次确保它不会意外地以XML或DOCTYPE声明开头
	// （因为这些应该已经被我们显式添加了）
	tempHtmlResultForDeclCleaning := regexp.MustCompile(`(?i)<\?xml[^>]*>\s*`).ReplaceAllString(htmlResult, "")
	tempHtmlResultForDeclCleaning = regexp.MustCompile(`(?i)<!DOCTYPE[^>]*>\s*`).ReplaceAllString(tempHtmlResultForDeclCleaning, "")

	finalOutputBuilder.WriteString(tempHtmlResultForDeclCleaning)
	t.logger.Debug("postprocessHTML: 最终构建的输出", zap.String("snippet", snippet(finalOutputBuilder.String())))
	// --- 构建结束 ---

	return finalOutputBuilder.String(), nil
}

func (t *GoQueryHTMLTranslator) debugPrintGroup(group GoQueryTextNodeGroup) {
	for nodeIdx, node := range group {
		if (nodeIdx)%5 == 0 {
			t.logger.Debug("--------")
			t.logger.Debug("节点详情",
				zap.Int("globalID", node.GlobalID),
				zap.String("path", node.Path),
				zap.String("originalText", snippet(node.Text)),
				zap.String("translatedText", snippet(node.TranslatedText)))
		}
	}
}

// TranslateHTMLWithGoQuery 使用goquery库翻译HTML文档，更好地保留HTML结构
func (t *GoQueryHTMLTranslator) Translate(htmlStr string, fileName string) (string, error) {

	t.originalHTML = htmlStr
	t.fileName = fileName
	t.shortFileName = filepath.Base(fileName)

	initialDoc, protectedContent, xmlDeclaration, doctypeDeclaration, err := t.processHTML(htmlStr)
	if err != nil {
		t.logger.Error("预处理HTML失败", zap.Error(err), zap.String("fileName", t.shortFileName))
		return "", err
	}

	textNodes, _, err := t.extractTextNodes(initialDoc)

	if err != nil {
		t.logger.Error("提取文本节点失败", zap.Error(err), zap.String("fileName", t.shortFileName))
		return "", err
	}

	if len(textNodes) == 0 {
		// 没有可翻译内容，直接进行后处理并返回
		finalHTML, err := t.postprocessHTML(initialDoc, protectedContent, xmlDeclaration, doctypeDeclaration, htmlStr)
		if err != nil {
			t.logger.Error("没有可翻译内容时后处理HTML失败", zap.Error(err), zap.String("fileName", t.shortFileName))
			return "", err
		}
		t.logger.Warn("没有可翻译内容，直接返回后处理结果", zap.String("fileName", t.shortFileName))
		return finalHTML, nil
	}

	groups := t.groupNodes(textNodes)

	// 并行翻译各个组
	var wg sync.WaitGroup

	// 创建信号量控制并发
	sem := make(chan struct{}, t.concurrency)

	var translatedNodeGroup GoQueryTextNodeGroup
	translatedNodeGroup = append(translatedNodeGroup, textNodes...)
	var returnErrors []error
	translatorGroupMergeMux := &sync.Mutex{}
	for groupIndex, groupData := range groups {
		wg.Add(1)
		go func(gIdx int, currentGroupData GoQueryTextNodeGroup) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			wgTranslatedNodeGroup, errors := t.translateGroup(currentGroupData, translatedNodeGroup)
			translatorGroupMergeMux.Lock()
			// t.logger.Debug("----go routine----翻译后的节点", zap.Int("groupIndex", gIdx))
			// t.debugPrintGroup(wgTranslatedNodeGroup)
			// 安全地更新当前组中的节点，确保从正确的位置获取翻译文本
			for _, node := range currentGroupData {
				nodeGlobalID := node.GlobalID
				if nodeGlobalID >= 0 && nodeGlobalID < len(wgTranslatedNodeGroup) {
					// 从返回的结果中获取对应节点的翻译文本
					translatedText := wgTranslatedNodeGroup[nodeGlobalID].TranslatedText
					if translatedText != "" {
						// 只有当翻译文本非空时才更新
						translatedNodeGroup[nodeGlobalID].TranslatedText = translatedText
						// t.logger.Debug("成功合并翻译结果",
						// 	zap.Int("groupIndex", gIdx),
						// 	zap.Int("nodeGlobalID", nodeGlobalID),
						// 	zap.String("originalSnippet", snippet(node.Text)),
						// 	zap.String("translatedSnippet", snippet(translatedText)),
						// 	zap.String("fileName", t.shortFileName))
					} else {
						t.logger.Warn("节点翻译结果为空，跳过合并",
							zap.Int("groupIndex", gIdx),
							zap.Int("nodeGlobalID", nodeGlobalID),
							zap.String("originalSnippet", snippet(node.Text)),
							zap.String("fileName", t.shortFileName))
					}
				} else {
					t.logger.Error("节点索引超出范围，无法合并翻译结果",
						zap.Int("groupIndex", gIdx),
						zap.Int("nodeGlobalID", nodeGlobalID),
						zap.Int("returnedGroupLength", len(wgTranslatedNodeGroup)),
						zap.String("fileName", t.shortFileName))
				}
			}
			returnErrors = append(returnErrors, errors...)
			translatorGroupMergeMux.Unlock()
		}(groupIndex, groupData)
	}
	wg.Wait()

	if len(translatedNodeGroup) != len(textNodes) {
		t.logger.Fatal("翻译后的节点数量与原始节点数量不一致", zap.Int("original", len(textNodes)), zap.Int("translated", len(translatedNodeGroup)), zap.String("fileName", t.shortFileName))
		return htmlStr, nil
	}
	// t.logger.Debug("第一步翻译后的节点")
	// t.debugPrintGroup(translatedNodeGroup)

	untranslatedNodeIds := t.collectFailedNodes(translatedNodeGroup)

	// 不断重试失败的节点
	retriesCount := 0
	for {
		retriesCount++
		if retriesCount > t.maxRetries {
			break
		}
		if len(untranslatedNodeIds) == 0 {
			break
		}
		t.logger.Warn("进行重试失败的节点", zap.Int("重试次数", retriesCount), zap.Int("失败节点数", len(untranslatedNodeIds)), zap.String("fileName", t.shortFileName))
		translatedNodeGroup = t.resortNodes(translatedNodeGroup, len(textNodes))
		untranslatedNodeGroups := t.groupFailedNodes(translatedNodeGroup, untranslatedNodeIds)
		for _, group := range untranslatedNodeGroups {
			reTranslatedNodeGroup, errors := t.translateGroup(group, translatedNodeGroup)
			// 采用和主翻译逻辑相同的安全合并策略
			for _, node := range group {
				nodeGlobalID := node.GlobalID
				if nodeGlobalID >= 0 && nodeGlobalID < len(reTranslatedNodeGroup) {
					// 从返回的结果中获取对应节点的翻译文本
					translatedText := reTranslatedNodeGroup[nodeGlobalID].TranslatedText
					if translatedText != "" {
						// 只有当翻译文本非空时才更新
						translatedNodeGroup[nodeGlobalID].TranslatedText = translatedText
						// t.logger.Debug("成功合并重试翻译结果",
						// 	zap.Int("retry", retriesCount),
						// 	zap.Int("nodeGlobalID", nodeGlobalID),
						// 	zap.String("originalSnippet", snippet(node.Text)),
						// 	zap.String("translatedSnippet", snippet(translatedText)))
					} else {
						t.logger.Warn("重试节点翻译结果为空，跳过合并",
							zap.Int("retry", retriesCount),
							zap.Int("nodeGlobalID", nodeGlobalID),
							zap.String("originalSnippet", snippet(node.Text)),
							zap.String("fileName", t.shortFileName))
					}
				} else {
					t.logger.Error("重试节点索引超出范围，无法合并翻译结果",
						zap.Int("retry", retriesCount),
						zap.Int("nodeGlobalID", nodeGlobalID),
						zap.Int("returnedGroupLength", len(reTranslatedNodeGroup)),
						zap.String("fileName", t.shortFileName))
				}
			}
			returnErrors = append(returnErrors, errors...)
		}
		untranslatedNodeIds = t.collectFailedNodes(translatedNodeGroup)
	}

	if len(untranslatedNodeIds) != 0 {
		t.logger.Error("依然有节点未翻译", zap.Int("失败节点数", len(untranslatedNodeIds)), zap.String("典型失败节点: ", translatedNodeGroup[untranslatedNodeIds[0]].Text), zap.String("fileName", t.shortFileName))
	}

	// t.logger.Debug("应用翻译到文档前", zap.Int("translatedNodeGroup", len(translatedNodeGroup)))
	// t.debugPrintGroup(translatedNodeGroup)

	afterDoc, err := t.applyTranslations(initialDoc, translatedNodeGroup)
	if err != nil {
		t.logger.Error("应用翻译到文档失败", zap.Error(err), zap.Int("TranslatedNodeGroup", len(translatedNodeGroup)), zap.String("fileName", t.shortFileName))
		return "", err
	}

	t.logger.Debug("应用翻译到文档成功", zap.Int("translatedNodeGroup", len(translatedNodeGroup)), zap.String("fileName", t.shortFileName))

	finalHTML, err := t.postprocessHTML(afterDoc, protectedContent, xmlDeclaration, doctypeDeclaration, htmlStr)
	if err != nil {
		t.logger.Error("后处理HTML失败", zap.Error(err), zap.String("fileName", t.shortFileName))
		return "", err
	}

	t.logger.Debug("后处理HTML成功", zap.String("finalHTML", snippetWithLength(finalHTML, 300)), zap.String("fileName", t.shortFileName))

	return finalHTML, nil
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
