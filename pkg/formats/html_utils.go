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
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// GoQueryNodeFormatInfo 保存节点的原始格式信息/GoQueryNodeFormatInfo stores the original format information of a node
type GoQueryNodeFormatInfo struct {
	LeadingWhitespace  string // 前导空白字符/Leading whitespace characters
	TrailingWhitespace string // 尾部空白字符/Trailing whitespace characters
	OriginalHTML       string // 用于保存原始HTML结构/Used to store the original HTML structure
}

// GoQueryTextNodeInfo 表示单个可翻译文本节点的信息/GoQueryTextNodeInfo represents information of a single translatable text node
type GoQueryTextNodeInfo struct {
	Selection      *goquery.Selection    // goquery节点选择器/Goquery node selector
	Text           string                // 原始文本/Original text
	TranslatedText string                // 翻译后的文本/Translated text
	Path           string                // 节点在DOM中的路径/Node's path in the DOM
	Format         GoQueryNodeFormatInfo // 格式信息/Format information
	IsAttribute    bool                  // 是否为属性文本/Whether this is attribute text
	AttributeName  string                // 属性名/Attribute name
	GlobalID       int                   // 全局唯一ID/Globally unique ID
	ContextBefore  string                // 上下文前文/Context before
	ContextAfter   string                // 上下文后文/Context after
}

// GoQueryTextNodeGroup 表示一组可翻译文本节点/GoQueryTextNodeGroup represents a group of translatable text nodes
type GoQueryTextNodeGroup []GoQueryTextNodeInfo

// GoQueryHTMLTranslator 用于基于goquery实现的HTML翻译/GoQueryHTMLTranslator is used for HTML translation based on goquery
type GoQueryHTMLTranslator struct {
	translator      translator.Translator // 翻译器/Translator
	fileName        string                // 当前处理的文件名/Current file name being processed
	logger          *zap.Logger           // 日志对象/Logger
	originalHTML    string                // 原始HTML/Original HTML
	shortFileName   string                // 文件短名/Short file name
	chunkSize       int                   // 分块大小/Chunk size
	concurrency     int                   // 并发度/Concurrency
	enableRetry     bool                  // 是否启用重试/Whether to enable retry
	maxRetries      int                   // 最大重试次数/Maximum retries
	nodeExtractRegx *regexp2.Regexp       // 节点提取正则表达式/Regular expression for node extraction
	nodeCount       int                   // 节点数量/Node count
}

// NewGoQueryHTMLTranslator 创建一个新的GoQueryHTMLTranslator实例/Create a new instance of GoQueryHTMLTranslator
func NewGoQueryHTMLTranslator(t translator.Translator, logger *zap.Logger) *GoQueryHTMLTranslator {
	// 获取配置/Get configuration
	agentConfig := t.GetConfig()
	chunkSize := 6000
	concurrency := 1 // 默认为1，即不进行文件内并行/Default is 1, meaning no concurrency within the file
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

		logger.Debug("Chunk Size 被设置为/Chunk size set", zap.Int("chunk_size", chunkSize))

		// 使用 HtmlConcurrency 控制单个HTML文件内部的并发/Use HtmlConcurrency to control concurrency within a single HTML file
		if agentConfig.HtmlConcurrency > 0 {
			concurrency = agentConfig.HtmlConcurrency
		} else {
			logger.Debug("HtmlConcurrency未配置或为0，单个HTML文件内节点翻译将串行执行。/HtmlConcurrency is not configured or set to 0, node translation within a single HTML file will be executed sequentially.", zap.Int("resolved_concurrency", concurrency))
		}
		enableRetry = t.GetConfig().RetryFailedParts
		maxRetries = t.GetConfig().MaxRetries
	}
	// 正则表达式：/Regular expression:
	// (?s) 允许 . 匹配换行符/(?s) allows . to match newline characters
	// @@NODE_START_(\d+)@@ 匹配开始标记并捕获数字索引 (group 1)/@@NODE_START_(\d+)@@ matches start tag and captures numeric index (group 1)
	// \n 匹配开始标记后的换行符/\n matches newline after start tag
	// (.*?) 懒惰匹配翻译内容，直到遇到下一个模式 (group 2)/(.*?) lazily matches translation content until next pattern (group 2)
	// \n 匹配结束标记前的换行符/\n matches newline before end tag
	// @@NODE_END_\1@@ 使用反向引用 \1确保结束标记的数字与开始标记的数字一致/@@NODE_END_\1@@ uses backreference \1 to ensure the numeric index matches
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

// replaceTextPreservingStructure 尝试替换文本内容，同时保留HTML结构/replaceTextPreservingStructure tries to replace text content while preserving HTML structure
// 这个函数会尝试智能地将翻译后的文本分配到原始HTML结构中的文本节点/This function attempts to smartly distribute translated text into the original HTML structure's text nodes
func (t *GoQueryHTMLTranslator) replaceTextPreservingStructure(selection *goquery.Selection, translatedText string) {
	// 收集所有文本节点/Collect all text nodes
	var textNodes []*goquery.Selection
	selection.Contents().Each(func(_ int, s *goquery.Selection) {
		if goquery.NodeName(s) == "#text" {
			if strings.TrimSpace(s.Text()) != "" {
				textNodes = append(textNodes, s)
			}
		} else {
			// 递归处理子元素/Recursively process child elements
			t.replaceTextPreservingStructure(s, "")
		}
	})

	// 如果没有文本节点或没有提供翻译文本，直接返回/Return directly if there are no text nodes or no translated text is provided
	if len(textNodes) == 0 || translatedText == "" {
		return
	}

	// 如果只有一个文本节点，直接替换/If there is only one text node, replace it directly
	if len(textNodes) == 1 {
		textNodes[0].ReplaceWithHtml(translatedText)
		return
	}

	// 如果有多个文本节点，尝试智能分配翻译文本/If there are multiple text nodes, try to allocate translated text intelligently
	// 这里使用一个简单的启发式方法：按照原始文本长度的比例分配翻译文本/Use a simple heuristic: allocate translated text according to the proportion of original text length
	totalOriginalLength := 0
	for _, node := range textNodes {
		totalOriginalLength += len(strings.TrimSpace(node.Text()))
	}

	// 如果原始文本总长度为0，无法按比例分配，直接返回/If total original text length is 0, can't allocate proportionally, return directly
	if totalOriginalLength == 0 {
		return
	}

	// 按比例分配翻译文本/Distribute translated text proportionally
	translatedWords := strings.Split(translatedText, " ")
	if len(translatedWords) == 0 {
		return
	}

	// 计算每个节点应该分配的单词数/Calculate number of words to allocate to each node
	wordsPerNode := make([]int, len(textNodes))
	for i, node := range textNodes {
		nodeTextLength := len(strings.TrimSpace(node.Text()))
		ratio := float64(nodeTextLength) / float64(totalOriginalLength)
		wordsPerNode[i] = int(ratio * float64(len(translatedWords)))
	}

	// 确保所有单词都被分配/Ensure all words are allocated
	totalAllocated := 0
	for _, count := range wordsPerNode {
		totalAllocated += count
	}
	remaining := len(translatedWords) - totalAllocated
	if remaining > 0 {
		// 将剩余的单词分配给最长的文本节点/Allocate remaining words to the longest text node
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

	// 分配翻译文本到各个节点/Allocate translated text to each node
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

// containsOnlyHTMLElements 检查一个节点是否只包含HTML元素，没有文本内容/containsOnlyHTMLElements checks whether a node contains only HTML elements and no text content
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

// processHTML 处理HTML字符串，保护特殊内容块并提取声明/processHTML processes the HTML string, protects special content blocks, and extracts declarations
func (t *GoQueryHTMLTranslator) processHTML(htmlStr string) (*goquery.Document, map[string]string, string, string, error) {
	var initialDoc *goquery.Document
	protectedContent := make(map[string]string)
	var xmlDecl string
	var doctypeDecl string

	// 保存XML声明和DOCTYPE，因为goquery可能会移除它们/Save XML declaration and DOCTYPE because goquery may remove them
	hasXMLDeclaration := strings.Contains(htmlStr, "<?xml")
	if hasXMLDeclaration {
		xmlDeclRegex := regexp.MustCompile(`<\?xml[^>]*\?>`)
		if match := xmlDeclRegex.FindString(htmlStr); match != "" {
			xmlDecl = match
			// t.logger.Debug("找到XML声明/Found XML declaration", zap.String("declaration", xmlDecl))
			htmlStr = strings.Replace(htmlStr, xmlDecl, "", 1) //移除一次，避免影响解析/Remove once to avoid affecting parsing
		}
	}

	hasDOCTYPE := strings.Contains(strings.ToLower(htmlStr), "<!doctype")
	if hasDOCTYPE {
		doctypeRegex := regexp.MustCompile(`(?i)<!DOCTYPE[^>]*>`)
		if match := doctypeRegex.FindString(htmlStr); match != "" {
			doctypeDecl = match
			// t.logger.Debug("找到DOCTYPE声明/Found DOCTYPE", zap.String("doctype", doctypeDecl))
			htmlStr = strings.Replace(htmlStr, doctypeDecl, "", 1) //移除一次/Remove once
		}
	}

	t.logger.Debug("文档声明处理完成/Document declaration processing complete",
		zap.Bool("hasXMLDeclaration", hasXMLDeclaration),
		zap.Bool("hasDOCTYPE", hasDOCTYPE),
		zap.String("fileName", t.shortFileName),
		zap.String("preservedXmlDecl", xmlDecl),
		zap.String("preservedDoctype", doctypeDecl))

	// 使用goquery解析HTML/Use goquery to parse HTML
	initialDoc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return initialDoc, protectedContent, xmlDecl, doctypeDecl, fmt.Errorf("解析HTML失败/Failed to parse HTML: %w", err)
	}

	// 重要：在同一个文档实例上进行内容保护，而不是重新创建文档/Important: Protect content in the same document instance instead of recreating the document
	// 这样可以确保后续获取的Selection对象仍然有效/This ensures that subsequent Selection objects are still valid
	placeholderIndex := 0

	// 保护script标签/Protect script tags
	initialDoc.Find("script").Each(func(i int, s *goquery.Selection) {
		if html, err := s.Html(); err == nil {
			placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
			protectedContent[placeholder] = fmt.Sprintf("<script%s>%s</script>",
				getAttributesString(s), html)
			s.ReplaceWithHtml(placeholder)
			placeholderIndex++
		}
	})

	// 保护style标签/Protect style tags
	initialDoc.Find("style").Each(func(i int, s *goquery.Selection) {
		if html, err := s.Html(); err == nil {
			placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
			protectedContent[placeholder] = fmt.Sprintf("<style%s>%s</style>",
				getAttributesString(s), html)
			s.ReplaceWithHtml(placeholder)
			placeholderIndex++
		}
	})

	// 保护pre标签/Protect pre tags
	initialDoc.Find("pre").Each(func(i int, s *goquery.Selection) {
		if html, err := goquery.OuterHtml(s); err == nil {
			placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
			protectedContent[placeholder] = html
			s.ReplaceWithHtml(placeholder)
			placeholderIndex++
		}
	})

	// 保护code标签/Protect code tags
	initialDoc.Find("code").Each(func(i int, s *goquery.Selection) {
		if html, err := goquery.OuterHtml(s); err == nil {
			placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
			protectedContent[placeholder] = html
			s.ReplaceWithHtml(placeholder)
			placeholderIndex++
		}
	})

	// 保护页面锚点标签（如 <a class="page" id="p59"/>）/Protect page anchor tags (such as <a class="page" id="p59"/>)
	// 这些标签对电子书导航至关重要，必须保持原始位置和格式/These tags are crucial for ebook navigation and must retain original position and format
	initialDoc.Find("a.page").Each(func(i int, s *goquery.Selection) {
		if html, err := goquery.OuterHtml(s); err == nil {
			placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
			protectedContent[placeholder] = html
			s.ReplaceWithHtml(placeholder)
			placeholderIndex++
			// t.logger.Debug("保护页面锚点标签/Protect page anchor tag", zap.String("html", html), zap.String("placeholder", placeholder), zap.String("fileName", t.shortFileName))
		}
	})

	// 保护其他重要的锚点标签（如带有特定class的导航锚点）/Protect other important anchor tags (such as navigation anchors with specific class)
	initialDoc.Find("a[class*='xref'], a[class*='anchor'], a[id]").Each(func(i int, s *goquery.Selection) {
		// 检查是否为空的锚点标签（只有ID或class，没有文本内容）/Check if this is an empty anchor tag (only ID or class, no text content)
		text := strings.TrimSpace(s.Text())
		if text == "" {
			if html, err := goquery.OuterHtml(s); err == nil {
				placeholder := fmt.Sprintf("@@PROTECTED_%d@@", placeholderIndex)
				protectedContent[placeholder] = html
				s.ReplaceWithHtml(placeholder)
				placeholderIndex++
				// t.logger.Debug("保护空锚点标签/Protect empty anchor tag", zap.String("html", html), zap.String("placeholder", placeholder), zap.String("fileName", t.shortFileName))
			}
		}
	})

	// TODO: 暂时跳过HTML注释保护，因为goquery处理注释节点比较复杂/TODO: Skip HTML comment protection for now, as goquery handles comment nodes in a complex way
	// 注释通常不包含需要翻译的内容，我们可以稍后用更安全的方式处理/Comments usually do not contain content that needs to be translated, can handle more safely later
	// 如果确实需要保护注释，可以考虑在字符串级别处理，而不是DOM级别/If protection is needed, consider handling at string level, not DOM level

	t.logger.Debug("保护了内容块/Protected content blocks", zap.Int("count", len(protectedContent)))

	return initialDoc, protectedContent, xmlDecl, doctypeDecl, nil
}

// getAttributesString 辅助函数：获取元素的属性字符串/getAttributesString is a helper function: get the attribute string of an element
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

// extractTextNodes 收集所有可翻译的文本节点和属性/Extract all translatable text nodes and attributes
// 返回值包括所有可翻译节点的信息、节点ID到原始文本的映射、以及错误信息/Returns all translatable node info, a mapping from node ID to original text, and error info
func (t *GoQueryHTMLTranslator) extractTextNodes(doc *goquery.Document) ([]GoQueryTextNodeInfo, map[int]string, error) {
	var textNodes []GoQueryTextNodeInfo
	var globalIDCounter int = 0
	allOriginalTexts := make(map[int]string)
	skipSelectors := []string{"script", "style", "code", "pre"}

	var processNode func(*goquery.Selection, string, bool)
	processNode = func(s *goquery.Selection, currentPath string, parentTranslates bool) {
		nodeName := goquery.NodeName(s)

		// 跳过不需要翻译的元素/Skip elements that should not be translated
		for _, skip := range skipSelectors {
			if nodeName == skip {
				return
			}
		}
		if IsSVGElement(s) {
			return
		}

		// 检查节点的 translate 属性/Check the node's translate attribute
		currentNodeTranslates := parentTranslates
		if translateAttr, exists := s.Attr("translate"); exists {
			attrValLower := strings.ToLower(translateAttr)
			if attrValLower == "no" || attrValLower == "false" {
				currentNodeTranslates = false
			} else if attrValLower == "yes" || attrValLower == "true" {
				currentNodeTranslates = true
			}
		}

		// 仅在节点可翻译时处理属性/Process attributes only if node is translatable
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

		// 处理子节点/Process child nodes
		s.Contents().Each(func(_ int, child *goquery.Selection) {
			childNodeName := goquery.NodeName(child)
			if childNodeName == "#text" {
				// 仅当父节点可翻译时翻译文本节点/Text nodes are translated only if their direct parent is translatable
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
			} else if child.Is("*") {
				// 递归处理子元素，子元素的可翻译性由自身 translate 属性和父节点共同决定/Recursively process child elements, child's translatability is determined by itself and parent
				processNode(child, fmt.Sprintf("%s/%s", currentPath, childNodeName), currentNodeTranslates)
			}
		})
	}

	// 处理 body 下的所有直接子元素/Process all direct children of body
	doc.Find("body").Children().Each(func(i int, s *goquery.Selection) {
		processNode(s, "body/"+goquery.NodeName(s), true)
	})
	// 如果未找到任何节点，则尝试 head > title/If no node found, try head > title
	if len(textNodes) == 0 {
		doc.Find("head > title").Each(func(i int, s *goquery.Selection) {
			processNode(s, "head/title", true)
		})
	}

	t.nodeCount = len(textNodes)

	t.logger.Debug("收集到的可翻译节点数量/Collected translatable node count", zap.Int("count", len(textNodes)), zap.String("fileName", t.shortFileName))

	return textNodes, allOriginalTexts, nil
}

// ExampleExtractTextNodes 展示如何使用 extractTextNodes 函数/Show how to use extractTextNodes function
func ExampleGoQueryHTMLTranslator_extractTextNodes() {
	html := `<html><body><p>Hello <b>world</b></p></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	translator_ := &GoQueryHTMLTranslator{}
	nodes, originalTexts, err := translator_.extractTextNodes(doc)
	fmt.Println(len(nodes) > 0, len(originalTexts) > 0, err == nil)
	// Output: true true true
}

// groupNodes 按照设定的 chunkSize 对文本节点进行分组/Group text nodes according to the set chunkSize
// 返回节点组切片/Returns a slice of node groups
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

	t.logger.Debug("分组后的节点数量/Grouped node count", zap.Int("groups", len(groups)))
	return groups
}

// ExampleGroupNodes 展示如何使用 groupNodes 将节点分组/Show how to use groupNodes to group nodes
func ExampleGoQueryHTMLTranslator_groupNodes() {
	nodes := []GoQueryTextNodeInfo{
		{Text: "A"}, {Text: "B"}, {Text: "C"}, {Text: "D"},
	}
	translator := &GoQueryHTMLTranslator{chunkSize: 2}
	groups := translator.groupNodes(nodes)
	fmt.Println(len(groups))
	// Output: 4
}

// parseGroupTranslation 解析分组翻译结果，将翻译内容按节点ID分配/Parse group translation result and assign translation contents by node ID
// 返回节点ID到翻译文本的映射和错误信息/Returns a map from node ID to translation text and error info
func (t *GoQueryHTMLTranslator) parseGroupTranslation(groupTranslatedText string, validNodeIDs []int) (map[int]string, error) {
	nodeIDWithTranslation := make(map[int]string)

	// 创建有效节点ID的快速查找map/Create a fast lookup map for valid node IDs
	validIDsMap := make(map[int]bool)
	for _, id := range validNodeIDs {
		validIDsMap[id] = true
	}

	// 调试信息/Debug info: 记录收到的翻译文本/Log received translation text
	// t.logger.Debug("解析翻译结果/Parse translation result",
	// 	zap.Int("textLength", len(groupTranslatedText)),
	// 	zap.Ints("validNodeIDs", validNodeIDs),
	// 	zap.String("textSnippet", snippet(groupTranslatedText)))

	var m *regexp2.Match
	m, err := t.nodeExtractRegx.FindStringMatch(groupTranslatedText) // 找到第一个匹配/Find first match
	if err != nil {
		t.logger.Error("regexp2 查找匹配时出错/Error occurred while finding match with regexp2", zap.Error(err), zap.String("fileName", t.shortFileName))
		return nodeIDWithTranslation, fmt.Errorf("regexp2 查找匹配时出错/Error occurred while finding match: %w", err)
	}
	// matches 是一个 [][]string，每个元素是：[ 完整匹配, 捕获的数字N, 捕获的翻译内容 ]
	// matches is a [][]string, each element: [full match, captured number N, captured translation content]

	detectedNodeCount := 0

	if m == nil {
		t.logger.Debug("没有找到匹配/No match found", zap.String("fileName", t.shortFileName))
		return nodeIDWithTranslation, nil
	}

	for m != nil {
		groups := m.Groups()
		if len(groups) == 3 { // Group 0 是完整匹配/Group 0 is the full match, Group 1 is the first capture, Group 2 is the second
			nodeIndexStr := groups[1].Capture.String()
			rawContent := groups[2].Capture.String()

			// 处理 rawContent/Process rawContent
			processedContent := strings.TrimSpace(rawContent)
			translation := strings.TrimSpace(processedContent)

			nodeIndex, errAtoi := strconv.Atoi(nodeIndexStr)
			if errAtoi != nil {
				t.logger.Warn("无法从标记解析节点索引/Failed to parse node index from marker", zap.String("indexStr", nodeIndexStr), zap.Error(errAtoi), zap.String("fileName", t.shortFileName))
			} else {
				// 仅在节点索引有效时赋值/Assign only if node index is valid
				if validIDsMap[nodeIndex] {
					//t.logger.Debug("检测到的节点/Detected node", zap.Int("nodeIndex", nodeIndex), zap.String("translation", translation))
					nodeIDWithTranslation[nodeIndex] = translation
					detectedNodeCount++
				} else {
					t.logger.Warn("解析到的节点索引不在当前组的有效范围内/Parsed node index is out of valid range",
						zap.Int("nodeIndex", nodeIndex),
						zap.Ints("validNodeIDs", validNodeIDs),
						zap.String("nodeIndexStr", nodeIndexStr),
						zap.String("translationSnippet", snippet(translation)),
						zap.String("fileName", t.shortFileName))
				}
			}
		} else {
			t.logger.Warn("regexp2 匹配结果的捕获组数量不符合预期/regexp2 match result group count is not as expected", zap.Int("groupsCount", len(groups)), zap.String("fileName", t.shortFileName))
		}

		m, err = t.nodeExtractRegx.FindNextMatch(m) // 查找下一个匹配/Find next match
		if err != nil {
			t.logger.Error("regexp2 查找下一个匹配时出错/Error occurred while finding next match with regexp2", zap.Error(err))
			break // 出错则停止/Stop on error
		}
	}
	// t.logger.Debug("检测到的节点数量/Detected node count", zap.Int("detectedNodeCount", detectedNodeCount), zap.String("fileName", t.shortFileName))
	return nodeIDWithTranslation, nil
}

// ExampleParseGroupTranslation 展示如何使用 parseGroupTranslation 解析翻译结果/Show how to use parseGroupTranslation to parse translation results
func ExampleGoQueryHTMLTranslator_parseGroupTranslation() {
	translator := &GoQueryHTMLTranslator{
		nodeExtractRegx: regexp2.MustCompile(`@@NODE_(\d+)@@(.*?)@@END_NODE@@`, regexp2.RE2),
	}
	groupText := "@@NODE_1@@hello@@END_NODE@@@@NODE_2@@world@@END_NODE@@"
	validIDs := []int{1, 2}
	result, err := translator.parseGroupTranslation(groupText, validIDs)
	fmt.Println(result[1], result[2], err == nil)
	// Output: hello world true
}

// translateGroup 翻译节点组，将翻译结果绑定到对应节点上/translateGroup translates a group of nodes and binds the translation results to the corresponding nodes
func (t *GoQueryHTMLTranslator) translateGroup(nodeGroup GoQueryTextNodeGroup, textNodes []GoQueryTextNodeInfo) (GoQueryTextNodeGroup, []error) {
	var errors []error

	// 调试信息：显示当前组的节点ID范围/Debug info: show node ID range of the current group
	var nodeIDs []int
	for _, node := range nodeGroup {
		nodeIDs = append(nodeIDs, node.GlobalID)
	}
	// t.logger.Debug("开始翻译节点组/Start translating node group",
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

	// t.logger.Debug("开始翻译HTML文本/Start translating HTML text", zap.Int("textLength", len(textsToTranslate)))
	translatedText, err := t.translator.Translate(textsToTranslate, t.enableRetry)
	if err != nil {
		errors = append(errors, err)
		t.logger.Warn("翻译HTML节点组失败/Failed to translate HTML node group", zap.Error(err))
		return returnNodeGroup, errors
	}

	// 把翻译结果绑定到 Node 上面去/Bind translation results to Node

	nodeIDWithTranslation, err := t.parseGroupTranslation(translatedText, nodeIDs)
	if err != nil {
		t.logger.Error("解析翻译结果失败/Failed to parse translation results", zap.Error(err))
	}

	// 验证翻译结果的完整性/Verify the completeness of translation results
	processedNodes := 0
	missingNodes := []int{}
	for _, node := range nodeGroup {
		if translation, exists := nodeIDWithTranslation[node.GlobalID]; exists {
			if translation == textNodes[node.GlobalID].Text {
				// 即使翻译结果与原文相同，也要设置TranslatedText/Set TranslatedText even if the translation is the same as the original
				// 这样可以确保节点被标记为"已处理"/This ensures the node is marked as "processed"
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
		t.logger.Warn("某些节点没有被翻译器处理/Some nodes were not handled by the translator",
			zap.Ints("missingNodeIDs", missingNodes),
			zap.Int("processedNodes", processedNodes),
			zap.Int("totalNodes", len(nodeGroup)))
	}

	// t.logger.Debug("节点组翻译完成/Node group translation completed",
	// 	zap.Int("processedNodes", processedNodes),
	// 	zap.Int("totalNodes", len(nodeGroup)),
	// 	zap.Int("missingNodes", len(missingNodes)))

	return returnNodeGroup, errors
}

// ExampleGoQueryHTMLTranslator_translateGroup 展示 translateGroup 的用法/Example of using translateGroup
// func ExampleGoQueryHTMLTranslator_translateGroup() {
// 	translator := translator.NewRawTranslator(nil, nil, nil)
// 	goQueryHTMLTranslator := NewGoQueryHTMLTranslator(translator, nil)
// 	nodeGroup := GoQueryTextNodeGroup{
// 		{GlobalID: 0, Text: "Hello"},
// 		{GlobalID: 1, Text: "World"},
// 	}
// 	textNodes := []GoQueryTextNodeInfo{
// 		{GlobalID: 0, Text: "Hello"},
// 		{GlobalID: 1, Text: "World"},
// 	}
// 	result, errs := translator.translateGroup(nodeGroup, textNodes)
// 	fmt.Println(result[0].TranslatedText, result[1].TranslatedText, len(errs) == 0)
// 	// Output: Hello World true
// }

// collectFailedNodes 收集未被翻译的节点ID/collectFailedNodes collects the IDs of nodes that were not translated
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

// ExampleGoQueryHTMLTranslator_collectFailedNodes 展示 collectFailedNodes 的用法/Example of using collectFailedNodes
func ExampleGoQueryHTMLTranslator_collectFailedNodes() {
	translator := &GoQueryHTMLTranslator{}
	nodeGroup := GoQueryTextNodeGroup{
		{GlobalID: 0, TranslatedText: ""},
		{GlobalID: 1, TranslatedText: "翻译内容/Translated"},
		{GlobalID: 2, TranslatedText: ""},
	}
	failedIDs := translator.collectFailedNodes(nodeGroup)
	fmt.Println(failedIDs)
	// Output: [0 2]
}

// groupFailedNodes 将失败节点及其上下文分组/groupFailedNodes groups failed nodes and their context
func (t *GoQueryHTMLTranslator) groupFailedNodes(totalNodes GoQueryTextNodeGroup, failedNodeIDs []int) []GoQueryTextNodeGroup {
	// 使用IntSet来去重/Use IntSet to deduplicate
	nodeSet := NewIntSet()

	// 为每个失败的节点添加上下文（前一个、当前、后一个）/Add context for each failed node (previous, current, next)
	for _, nodeID := range failedNodeIDs {
		// 添加前一个节点（如果存在）/Add previous node if exists
		if nodeID > 0 {
			nodeSet.Add(nodeID - 1)
		}
		// 添加当前失败的节点/Add current failed node
		nodeSet.Add(nodeID)
		// 添加后一个节点（如果存在）/Add next node if exists
		if nodeID < len(totalNodes)-1 {
			nodeSet.Add(nodeID + 1)
		}
	}

	// 获取去重并排序后的节点ID/Get deduplicated and sorted node IDs
	uniqueNodeIDs := nodeSet.ToSlice()

	// 构建失败节点及其上下文的数组/Build array of failed nodes and their context
	var failedNodeWithContext GoQueryTextNodeGroup
	for _, nodeID := range uniqueNodeIDs {
		if nodeID >= 0 && nodeID < len(totalNodes) {
			failedNodeWithContext = append(failedNodeWithContext, totalNodes[nodeID])
		}
	}

	// t.logger.Debug("重试节点去重统计/Retry node deduplication statistics",
	// 	zap.Int("原始失败节点数/Original failed node count", len(failedNodeIDs)),
	// 	zap.Int("加上下文后总节点数/Total nodes after adding context", len(failedNodeWithContext)),
	// 	zap.Int("set大小/Set size", nodeSet.Size()),
	// 	zap.Ints("失败节点IDs/Failed node IDs", failedNodeIDs),
	// 	zap.Ints("去重后节点IDs/Deduplicated node IDs", uniqueNodeIDs))

	return t.groupNodes(failedNodeWithContext)
}

// ExampleGoQueryHTMLTranslator_groupFailedNodes 展示 groupFailedNodes 的用法/Example of using groupFailedNodes
func ExampleGoQueryHTMLTranslator_groupFailedNodes() {
	translator := &GoQueryHTMLTranslator{}
	totalNodes := GoQueryTextNodeGroup{
		{GlobalID: 0}, {GlobalID: 1}, {GlobalID: 2}, {GlobalID: 3}, {GlobalID: 4},
	}
	failedNodeIDs := []int{2}
	groups := translator.groupFailedNodes(totalNodes, failedNodeIDs)
	for _, group := range groups {
		for _, node := range group {
			fmt.Print(node.GlobalID, " ")
		}
	}
	// Output: 1 2 3
}

// resortNodes 按 GlobalID 对节点组重新排序，并返回新的节点组/resortNodes reorders the node group by GlobalID and returns a new node group
func (t *GoQueryHTMLTranslator) resortNodes(nodeGroup GoQueryTextNodeGroup, length int) GoQueryTextNodeGroup {
	newNodeGroup := make(GoQueryTextNodeGroup, length)
	for _, node := range nodeGroup {
		if node.GlobalID >= length {
			t.logger.Warn("节点ID超出范围/Node ID out of range", zap.Int("nodeID", node.GlobalID), zap.Int("len", length))
			continue
		}
		newNodeGroup[node.GlobalID] = node
	}
	return newNodeGroup
}

// ExampleGoQueryHTMLTranslator_resortNodes 展示 resortNodes 的用法/Example of using resortNodes
func ExampleGoQueryHTMLTranslator_resortNodes() {
	translator := &GoQueryHTMLTranslator{}
	nodeGroup := GoQueryTextNodeGroup{
		{GlobalID: 2, Text: "C"},
		{GlobalID: 0, Text: "A"},
		{GlobalID: 1, Text: "B"},
	}
	resorted := translator.resortNodes(nodeGroup, 3)
	for _, node := range resorted {
		fmt.Print(node.Text, " ")
	}
	// Output: A B C
}

// updateNodes 根据新节点组更新原始节点组的翻译文本/Update the translation text in the original node group with values from the new node group.
// 该方法会遍历 newNodeGroup，将其中每个节点的 TranslatedText 替换到 originalNodeGroup 中对应 GlobalID 的节点上。/This method iterates over newNodeGroup and updates the TranslatedText field for the node with the same GlobalID in originalNodeGroup.
func (t *GoQueryHTMLTranslator) updateNodes(originalNodeGroup GoQueryTextNodeGroup, newNodeGroup GoQueryTextNodeGroup) GoQueryTextNodeGroup {
	for _, node := range newNodeGroup {
		// 将新节点组中的翻译文本赋值到原始节点组的对应节点/Assign translated text from the new node group to the corresponding node in the original group
		originalNodeGroup[node.GlobalID].TranslatedText = node.TranslatedText
	}
	return originalNodeGroup
}

// ExampleGoQueryHTMLTranslator_updateNodes 展示 updateNodes 的典型用法/Example of using updateNodes.
func ExampleGoQueryHTMLTranslator_updateNodes() {
	translator := &GoQueryHTMLTranslator{}
	original := GoQueryTextNodeGroup{
		{GlobalID: 0, TranslatedText: ""},
		{GlobalID: 1, TranslatedText: ""},
	}
	newNodes := GoQueryTextNodeGroup{
		{GlobalID: 0, TranslatedText: "你好/Hello"},
		{GlobalID: 1, TranslatedText: "世界/World"},
	}
	updated := translator.updateNodes(original, newNodes)
	for _, node := range updated {
		fmt.Print(node.TranslatedText, " ")
	}
	// Output: 你好/Hello 世界/World
}

// updateNodesWithFailedIDs 仅根据失败节点ID更新原始节点组/Update the original node group only for nodes with failed IDs.
// 只处理在 failedNodeIDs 中指定的节点，如果 newNodeGroup 中该节点的 TranslatedText 非空，则将其写入 originalNodeGroup。/Only processes nodes specified in failedNodeIDs; if TranslatedText in newNodeGroup is non-empty, it is written to originalNodeGroup.
func (t *GoQueryHTMLTranslator) updateNodesWithFailedIDs(originalNodeGroup GoQueryTextNodeGroup, newNodeGroup GoQueryTextNodeGroup, failedNodeIDs []int) GoQueryTextNodeGroup {
	for _, failedNodeID := range failedNodeIDs {
		// 检查ID有效并且新节点组有翻译文本/Check ID validity and presence of translated text in new group
		if failedNodeID < len(newNodeGroup) && newNodeGroup[failedNodeID].TranslatedText != "" {
			originalNodeGroup[failedNodeID].TranslatedText = newNodeGroup[failedNodeID].TranslatedText
		}
	}
	return originalNodeGroup
}

// ExampleGoQueryHTMLTranslator_updateNodesWithFailedIDs 展示 updateNodesWithFailedIDs 的典型用法/Example of using updateNodesWithFailedIDs.
func ExampleGoQueryHTMLTranslator_updateNodesWithFailedIDs() {
	translator := &GoQueryHTMLTranslator{}
	original := GoQueryTextNodeGroup{
		{GlobalID: 0, TranslatedText: ""},
		{GlobalID: 1, TranslatedText: ""},
		{GlobalID: 2, TranslatedText: ""},
	}
	newNodes := GoQueryTextNodeGroup{
		{GlobalID: 0, TranslatedText: ""},
		{GlobalID: 1, TranslatedText: "成功/Success"},
		{GlobalID: 2, TranslatedText: ""},
	}
	failedIDs := []int{1}
	updated := translator.updateNodesWithFailedIDs(original, newNodes, failedIDs)
	for _, node := range updated {
		fmt.Print(node.TranslatedText, " ")
	}
	// Output:  成功/Success
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

	for _, nodeInfo := range translatedNodes {
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

// cleanGoqueryWrapper 尝试移除 goquery 可能为 XML 片段添加的多余的 html/body 包装/Try to remove extra html/body wrappers that goquery may add for XML fragments.
// 根据原始输入判断是否应还原为片段内容，主要用于格式化和输出友好的 HTML/XML 片段/Restore to fragment content if the original input looks like a fragment.
// 如果无法识别为片段，则返回 goquery 生成（可能已规范化）的 HTML/If unable to identify as a fragment, returns the goquery-generated (possibly normalized) HTML.
func (t *GoQueryHTMLTranslator) cleanGoqueryWrapper(htmlStr string, originalFullHTMLString string) string {
	// 这是一个复杂的启发式过程，可能需要根据你的具体 XML 结构进行调整/This is a heuristic process and may need adjustments for your XML structure.
	// 目标：如果原始输入是XML片段，但goquery将其包装在html/body中，尝试还原/Goal: If the original input is an XML fragment but goquery wraps it in html/body, try to restore.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		// cleanGoqueryWrapper: 解析htmlStr失败/warning for failed parsing htmlStr
		t.logger.Warn("cleanGoqueryWrapper: 解析htmlStr失败/cleanGoqueryWrapper: failed to parse htmlStr", zap.Error(err), zap.String("html_snippet", snippet(htmlStr)))
		return htmlStr // 解析失败，返回原样/Parsing failed, return as is.
	}

	// 判断原始字符串是否像片段/Determine whether the original string looks like a fragment
	isLikelyFragment := !strings.Contains(strings.ToLower(originalFullHTMLString), "<html") &&
		!strings.Contains(strings.ToLower(originalFullHTMLString), "<body")

	bodySelection := doc.Find("body")
	if bodySelection.Length() > 0 {
		// 检查 body 是否是 goquery 添加的唯一顶层元素 (在 html > body 结构中)/Check if body is the only top-level element added by goquery (html > body structure)
		// 并且原始输入看起来像片段/And the original input seems like a fragment
		if isLikelyFragment && bodySelection.Parent().Is("html") && bodySelection.Parent().Children().Length() == 1 { // body是html唯一的孩子/body is the only child of html
			if bodyHtml, err := bodySelection.Html(); err == nil {
				// cleanGoqueryWrapper: 原始输入像片段，且body是html唯一子元素，返回body内容/Input looks like a fragment and body is the only child of html, return body content
				t.logger.Debug("cleanGoqueryWrapper: 原始输入像片段，且body是html唯一子元素，返回body内容/cleanGoqueryWrapper: input looks like a fragment and body is the only child, returning body content", zap.String("body_html_snippet", snippet(bodyHtml)))
				return bodyHtml
			}
		}
		// 另一种情况：如果body内容本身看起来就是多个XML元素或者纯文本/Another case: if body content looks like multiple XML elements or plain text
		children := bodySelection.Children()
		if children.Length() > 1 || (children.Length() == 0 && strings.TrimSpace(bodySelection.Text()) != "") {
			if bodyHtml, err := bodySelection.Html(); err == nil {
				// cleanGoqueryWrapper: body包含多个子元素或纯文本，返回body内容/body contains multiple children or plain text, return body content
				t.logger.Debug("cleanGoqueryWrapper: body包含多个子元素或纯文本，返回body内容/cleanGoqueryWrapper: body contains multiple children or plain text, returning body content", zap.String("body_html_snippet", snippet(bodyHtml)))
				return bodyHtml
			}
		}
		// 如果body只有一个子元素，且这个子元素不是html/head/body，也可能是有效的XML片段/If body has one child, and it's not html/head/body, it may be a valid XML fragment
		if children.Length() == 1 && children.Filter("html,head,body").Length() == 0 {
			if bodyHtml, err := bodySelection.Html(); err == nil {
				// cleanGoqueryWrapper: body只有一个有效子元素，返回body内容/body has only one valid child, return body content
				t.logger.Debug("cleanGoqueryWrapper: body只有一个有效子元素，返回body内容/cleanGoqueryWrapper: body has only one valid child, returning body content", zap.String("body_html_snippet", snippet(bodyHtml)))
				return bodyHtml
			}
		}
	}

	// 如果以上启发式方法都不适用，或者解析/获取body内容失败/If none of the above heuristics apply or failed to get body content
	// 返回由 doc.Html() 生成的（可能已被goquery规范化的）完整HTML/Return the complete HTML generated by doc.Html() (possibly normalized by goquery)
	// 这意味着对于某些复杂的XML结构，可能仍然会保留html/body包装/For some complex XML, html/body wrappers may still be kept
	t.logger.Debug("cleanGoqueryWrapper: 未应用特定清理规则，返回goquery生成的HTML/cleanGoqueryWrapper: no specific cleanup rule applied, returning goquery-generated HTML", zap.String("html_snippet", snippet(htmlStr)))
	return htmlStr
}

// ExampleGoQueryHTMLTranslator_cleanGoqueryWrapper 展示 cleanGoqueryWrapper 的典型用法/ExampleGoQueryHTMLTranslator_cleanGoqueryWrapper demonstrates typical usage of cleanGoqueryWrapper.
func ExampleGoQueryHTMLTranslator_cleanGoqueryWrapper() {
	translator := &GoQueryHTMLTranslator{}
	// 案例1：原始内容为XML片段/Case 1: original content is an XML fragment
	original := `<div>foo</div><span>bar</span>`
	htmlWrapped := `<html><body><div>foo</div><span>bar</span></body></html>`
	cleaned := translator.cleanGoqueryWrapper(htmlWrapped, original)
	fmt.Println(strings.TrimSpace(cleaned))
	// Output: <div>foo</div><span>bar</span>
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

// debugPrintGroup 打印 GoQueryTextNodeGroup 的节点详情，便于调试/Prints details of GoQueryTextNodeGroup nodes for debugging.
func (t *GoQueryHTMLTranslator) debugPrintGroup(group GoQueryTextNodeGroup) {
	for nodeIdx, node := range group {
		if (nodeIdx)%5 == 0 {
			t.logger.Debug("--------")
			t.logger.Debug("节点详情/Node Details",
				zap.Int("globalID", node.GlobalID),
				zap.String("path", node.Path),
				zap.String("originalText/原文片段", snippet(node.Text)),
				zap.String("translatedText/翻译片段", snippet(node.TranslatedText)))
		}
	}
}

// Translate 翻译 HTML 文档，使用 goquery 保留 HTML 结构/Translate HTML document, preserving structure with goquery.
func (t *GoQueryHTMLTranslator) Translate(htmlStr string, fileName string) (string, error) {
	// 保存原始 HTML 和文件名/Store original HTML and file names
	t.originalHTML = htmlStr
	t.fileName = fileName
	t.shortFileName = filepath.Base(fileName)

	// 预处理 HTML，获取文档和声明/Preprocess HTML, obtain document and declarations
	initialDoc, protectedContent, xmlDeclaration, doctypeDeclaration, err := t.processHTML(htmlStr)
	if err != nil {
		t.logger.Error("预处理HTML失败/Failed to preprocess HTML", zap.Error(err), zap.String("fileName/文件名", t.shortFileName))
		return "", err
	}

	// 提取文本节点/Extract text nodes
	textNodes, _, err := t.extractTextNodes(initialDoc)
	if err != nil {
		t.logger.Error("提取文本节点失败/Failed to extract text nodes", zap.Error(err), zap.String("fileName/文件名", t.shortFileName))
		return "", err
	}

	if len(textNodes) == 0 {
		// 没有可翻译内容，直接后处理/No translatable content, postprocess and return
		finalHTML, err := t.postprocessHTML(initialDoc, protectedContent, xmlDeclaration, doctypeDeclaration, htmlStr)
		if err != nil {
			t.logger.Error("没有可翻译内容时后处理HTML失败/Postprocessing failed when no translatable content", zap.Error(err), zap.String("fileName/文件名", t.shortFileName))
			return "", err
		}
		t.logger.Warn("没有可翻译内容，直接返回后处理结果/No translatable content, directly returning postprocessed result", zap.String("fileName/文件名", t.shortFileName))
		return finalHTML, nil
	}

	// 对节点分组/Group nodes
	groups := t.groupNodes(textNodes)

	// 并行翻译各个组/Translate groups in parallel
	var wg sync.WaitGroup
	// 创建信号量控制并发/Create semaphore for concurrency control
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
			// 翻译当前分组/Translate current group
			wgTranslatedNodeGroup, errors := t.translateGroup(currentGroupData, translatedNodeGroup)
			translatorGroupMergeMux.Lock()
			// 安全地合并翻译结果/Safely merge translation results
			for _, node := range currentGroupData {
				nodeGlobalID := node.GlobalID
				if nodeGlobalID >= 0 && nodeGlobalID < len(wgTranslatedNodeGroup) {
					translatedText := wgTranslatedNodeGroup[nodeGlobalID].TranslatedText
					if translatedText != "" {
						translatedNodeGroup[nodeGlobalID].TranslatedText = translatedText
					} else {
						t.logger.Warn("节点翻译结果为空，跳过合并/Node translation result empty, skipping merge",
							zap.Int("groupIndex/分组序号", gIdx),
							zap.Int("nodeGlobalID/节点ID", nodeGlobalID),
							zap.String("originalSnippet/原文片段", snippet(node.Text)),
							zap.String("fileName/文件名", t.shortFileName))
					}
				} else {
					t.logger.Error("节点索引超出范围，无法合并翻译结果/Node index out of range, cannot merge translation result",
						zap.Int("groupIndex/分组序号", gIdx),
						zap.Int("nodeGlobalID/节点ID", nodeGlobalID),
						zap.Int("returnedGroupLength/返回分组长度", len(wgTranslatedNodeGroup)),
						zap.String("fileName/文件名", t.shortFileName))
				}
			}
			returnErrors = append(returnErrors, errors...)
			translatorGroupMergeMux.Unlock()
		}(groupIndex, groupData)
	}
	wg.Wait()

	if len(translatedNodeGroup) != len(textNodes) {
		t.logger.Fatal("翻译后的节点数量与原始节点数量不一致/Translated node count does not match original", zap.Int("original/原始数量", len(textNodes)), zap.Int("translated/翻译数量", len(translatedNodeGroup)), zap.String("fileName/文件名", t.shortFileName))
		return htmlStr, nil
	}

	// 收集未翻译节点/Collect untranslated node IDs
	untranslatedNodeIds := t.collectFailedNodes(translatedNodeGroup)

	// 不断重试失败的节点/Retry failed nodes until max retries reached
	retriesCount := 0
	for {
		retriesCount++
		if retriesCount > t.maxRetries {
			break
		}
		if len(untranslatedNodeIds) == 0 {
			break
		}
		t.logger.Warn("进行重试失败的节点/Retrying failed nodes", zap.Int("重试次数/Retry count", retriesCount), zap.Int("失败节点数/Failed node count", len(untranslatedNodeIds)), zap.String("fileName/文件名", t.shortFileName))
		translatedNodeGroup = t.resortNodes(translatedNodeGroup, len(textNodes))
		untranslatedNodeGroups := t.groupFailedNodes(translatedNodeGroup, untranslatedNodeIds)
		for _, group := range untranslatedNodeGroups {
			reTranslatedNodeGroup, errors := t.translateGroup(group, translatedNodeGroup)
			// 合并重试翻译结果/Merge retry translation results
			for _, node := range group {
				nodeGlobalID := node.GlobalID
				if nodeGlobalID >= 0 && nodeGlobalID < len(reTranslatedNodeGroup) {
					translatedText := reTranslatedNodeGroup[nodeGlobalID].TranslatedText
					if translatedText != "" {
						translatedNodeGroup[nodeGlobalID].TranslatedText = translatedText
					} else {
						t.logger.Warn("重试节点翻译结果为空，跳过合并/Retry node translation result empty, skipping merge",
							zap.Int("retry/重试次数", retriesCount),
							zap.Int("nodeGlobalID/节点ID", nodeGlobalID),
							zap.String("originalSnippet/原文片段", snippet(node.Text)),
							zap.String("fileName/文件名", t.shortFileName))
					}
				} else {
					t.logger.Error("重试节点索引超出范围，无法合并翻译结果/Retry node index out of range, cannot merge translation result",
						zap.Int("retry/重试次数", retriesCount),
						zap.Int("nodeGlobalID/节点ID", nodeGlobalID),
						zap.Int("returnedGroupLength/返回分组长度", len(reTranslatedNodeGroup)),
						zap.String("fileName/文件名", t.shortFileName))
				}
			}
			returnErrors = append(returnErrors, errors...)
		}
		untranslatedNodeIds = t.collectFailedNodes(translatedNodeGroup)
	}

	if len(untranslatedNodeIds) != 0 {
		t.logger.Error("依然有节点未翻译/There are still untranslated nodes", zap.Int("失败节点数/Failed node count", len(untranslatedNodeIds)), zap.String("典型失败节点/Typical failed node", translatedNodeGroup[untranslatedNodeIds[0]].Text), zap.String("fileName/文件名", t.shortFileName))
	}

	// 应用翻译到文档/Apply translations to document
	afterDoc, err := t.applyTranslations(initialDoc, translatedNodeGroup)
	if err != nil {
		t.logger.Error("应用翻译到文档失败/Failed to apply translations to document", zap.Error(err), zap.Int("TranslatedNodeGroup/翻译节点数", len(translatedNodeGroup)), zap.String("fileName/文件名", t.shortFileName))
		return "", err
	}

	t.logger.Debug("应用翻译到文档成功/Successfully applied translations to document", zap.Int("translatedNodeGroup/翻译节点数", len(translatedNodeGroup)), zap.String("fileName/文件名", t.shortFileName))

	// 后处理生成最终 HTML/Postprocess to generate final HTML
	finalHTML, err := t.postprocessHTML(afterDoc, protectedContent, xmlDeclaration, doctypeDeclaration, htmlStr)
	if err != nil {
		t.logger.Error("后处理HTML失败/Failed to postprocess HTML", zap.Error(err), zap.String("fileName/文件名", t.shortFileName))
		return "", err
	}

	t.logger.Debug("后处理HTML成功/Postprocessed HTML successfully", zap.String("finalHTML/最终HTML片段", snippetWithLength(finalHTML, 300)), zap.String("fileName/文件名", t.shortFileName))

	return finalHTML, nil
}

// shouldTranslateAttr 判断属性名是否应被翻译/Determine if an attribute should be translated based on its name.
func shouldTranslateAttr(attrName string) bool {
	switch strings.ToLower(attrName) {
	case "title", "alt", "label", "aria-label", "placeholder", "summary":
		return true
	}
	return false
}

// IsSVGElement 检查节点是否为 SVG 元素（SVG 内容通常不翻译）/Check if a node is an SVG element (SVG content usually not translated).
func IsSVGElement(s *goquery.Selection) bool {
	if goquery.NodeName(s) == "svg" {
		return true
	}
	// 也检查父节点是否为 SVG，以处理 SVG 内部的元素/Also check parent nodes for SVG to handle elements inside SVG.
	if s.ParentsFiltered("svg").Length() > 0 {
		return true
	}
	return false
}

// ExampleGoQueryHTMLTranslator_Translate 展示 Translate 方法的典型用法/Showcases typical usage of the Translate method.
func ExampleGoQueryHTMLTranslator_Translate() {
	translator := &GoQueryHTMLTranslator{
		logger:      logger.NewLogger(true), // 需实现 NewLogger() 返回符合 logger 接口的对象/Implement NewLogger() to return a logger object.
		concurrency: 2,                      // 并发数/Concurrency level.
		maxRetries:  2,                      // 最大重试次数/Max retry count.
	}
	html := `<div title="hello">Hello <span>world</span></div>`
	res, err := translator.Translate(html, "example.html")
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Println(strings.TrimSpace(res))
	}
	// Output:
	// <div title="hello">Hello <span>world</span></div>
}
