package document

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
	"golang.org/x/net/html"
)

// HTMLSmartExtractor 智能HTML节点提取器
// 参考html_utils.go中GoQueryHTMLTranslator的实现，支持translate属性和SVG跳过
type HTMLSmartExtractor struct {
	logger                 *zap.Logger
	placeholderManager     *HTMLPlaceholderManager
	skipSVG                bool
	respectTranslateAttr   bool
	skipEmptyText          bool
	preserveWhitespace     bool
	minTextLength          int
	extractableAttributes  []string
	skipElements           map[string]bool
}

// SmartExtractorOptions 智能提取器选项
type SmartExtractorOptions struct {
	SkipSVG                bool     // 是否跳过SVG元素
	RespectTranslateAttr   bool     // 是否尊重translate属性
	SkipEmptyText          bool     // 是否跳过空白文本
	PreserveWhitespace     bool     // 是否保留空白字符
	MinTextLength          int      // 最小文本长度
	ExtractableAttributes  []string // 可提取的属性列表
	SkipElements           []string // 跳过的元素列表
}

// DefaultSmartExtractorOptions 默认提取器选项
func DefaultSmartExtractorOptions() SmartExtractorOptions {
	return SmartExtractorOptions{
		SkipSVG:              true,
		RespectTranslateAttr: true,
		SkipEmptyText:        true,
		PreserveWhitespace:   true,
		MinTextLength:        1,
		ExtractableAttributes: []string{
			"alt", "title", "placeholder", "aria-label", "aria-description",
			"data-tooltip", "data-title", "label",
		},
		SkipElements: []string{
			"script", "style", "noscript", "iframe", "object", "embed",
			"canvas", "svg", "math", "code", "pre",
		},
	}
}

// NewHTMLSmartExtractor 创建智能HTML节点提取器
func NewHTMLSmartExtractor(logger *zap.Logger, options SmartExtractorOptions) *HTMLSmartExtractor {
	skipElementsMap := make(map[string]bool)
	for _, elem := range options.SkipElements {
		skipElementsMap[strings.ToLower(elem)] = true
	}

	return &HTMLSmartExtractor{
		logger:                logger,
		placeholderManager:    NewHTMLPlaceholderManager(),
		skipSVG:               options.SkipSVG,
		respectTranslateAttr:  options.RespectTranslateAttr,
		skipEmptyText:         options.SkipEmptyText,
		preserveWhitespace:    options.PreserveWhitespace,
		minTextLength:         options.MinTextLength,
		extractableAttributes: options.ExtractableAttributes,
		skipElements:          skipElementsMap,
	}
}

// ExtractableNode 可提取的节点信息
type ExtractableNode struct {
	Selection      *goquery.Selection // goquery选择器
	Text           string             // 原始文本
	Path           string             // DOM路径
	IsAttribute    bool               // 是否为属性
	AttributeName  string             // 属性名（如果是属性）
	NodeType       string             // 节点类型
	CanTranslate   bool               // 是否可翻译
	ParentTag      string             // 父标签名
	Context        ExtractContext     // 上下文信息
}

// ExtractContext 提取上下文
type ExtractContext struct {
	LeadingWhitespace  string // 前导空白
	TrailingWhitespace string // 尾随空白
	BeforeContext      string // 前文上下文
	AfterContext       string // 后文上下文
}

// ExtractTranslatableNodes 提取可翻译的节点
func (e *HTMLSmartExtractor) ExtractTranslatableNodes(doc *goquery.Document) ([]*ExtractableNode, error) {
	var nodes []*ExtractableNode
	nodeID := 1

	// 遍历所有元素
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		if e.shouldSkipElement(s) {
			return
		}

		// 提取文本节点
		textNodes := e.extractTextNodes(s, fmt.Sprintf("[%d]", i+1), &nodeID)
		nodes = append(nodes, textNodes...)

		// 提取属性节点
		attrNodes := e.extractAttributeNodes(s, fmt.Sprintf("[%d]", i+1), &nodeID)
		nodes = append(nodes, attrNodes...)
	})

	e.logger.Debug("extracted translatable nodes",
		zap.Int("totalNodes", len(nodes)),
		zap.Int("textNodes", e.countNodesByType(nodes, "text")),
		zap.Int("attributeNodes", e.countNodesByType(nodes, "attribute")))

	return nodes, nil
}

// shouldSkipElement 检查是否应跳过元素
func (e *HTMLSmartExtractor) shouldSkipElement(s *goquery.Selection) bool {
	if s.Nodes == nil || len(s.Nodes) == 0 {
		return true
	}

	node := s.Nodes[0]
	if node.Type != html.ElementNode {
		return false
	}

	tagName := strings.ToLower(node.Data)

	// 检查是否在跳过列表中
	if e.skipElements[tagName] {
		e.logger.Debug("skipping element in skip list", zap.String("tag", tagName))
		return true
	}

	// 特殊处理SVG
	if e.skipSVG && (tagName == "svg" || e.isInsideSVG(s)) {
		e.logger.Debug("skipping SVG element", zap.String("tag", tagName))
		return true
	}

	// 检查translate属性
	if e.respectTranslateAttr {
		if translateVal, exists := s.Attr("translate"); exists {
			if strings.ToLower(translateVal) == "no" {
				e.logger.Debug("skipping element with translate=no", zap.String("tag", tagName))
				return true
			}
		}
	}

	return false
}

// isInsideSVG 检查元素是否在SVG内部
func (e *HTMLSmartExtractor) isInsideSVG(s *goquery.Selection) bool {
	parent := s.Parent()
	for parent.Length() > 0 {
		if parent.Nodes != nil && len(parent.Nodes) > 0 {
			if strings.ToLower(parent.Nodes[0].Data) == "svg" {
				return true
			}
		}
		parent = parent.Parent()
	}
	return false
}

// extractTextNodes 提取文本节点
func (e *HTMLSmartExtractor) extractTextNodes(s *goquery.Selection, basePath string, nodeID *int) []*ExtractableNode {
	var nodes []*ExtractableNode

	s.Contents().Each(func(i int, child *goquery.Selection) {
		if child.Nodes == nil || len(child.Nodes) == 0 {
			return
		}

		node := child.Nodes[0]
		currentPath := fmt.Sprintf("%s[%d]", basePath, i+1)

		if node.Type == html.TextNode {
			text := node.Data
			cleanText := strings.TrimSpace(text)

			// 检查是否应跳过空文本
			if e.skipEmptyText && cleanText == "" {
				return
			}

			// 检查最小长度
			if len(cleanText) < e.minTextLength {
				return
			}

			// 创建提取节点
			extractNode := &ExtractableNode{
				Selection:     child,
				Text:          cleanText,
				Path:          currentPath,
				IsAttribute:   false,
				NodeType:      "text",
				CanTranslate:  true,
				ParentTag:     e.getParentTag(child),
				Context: ExtractContext{
					LeadingWhitespace:  e.getLeadingWhitespace(text),
					TrailingWhitespace: e.getTrailingWhitespace(text),
					BeforeContext:      e.getBeforeContext(child),
					AfterContext:       e.getAfterContext(child),
				},
			}

			nodes = append(nodes, extractNode)
			*nodeID++

		} else if node.Type == html.ElementNode {
			// 递归处理子元素
			if !e.shouldSkipElement(child) {
				childNodes := e.extractTextNodes(child, currentPath, nodeID)
				nodes = append(nodes, childNodes...)
			}
		}
	})

	return nodes
}

// extractAttributeNodes 提取属性节点
func (e *HTMLSmartExtractor) extractAttributeNodes(s *goquery.Selection, path string, nodeID *int) []*ExtractableNode {
	var nodes []*ExtractableNode

	for _, attrName := range e.extractableAttributes {
		if attrValue, exists := s.Attr(attrName); exists {
			cleanValue := strings.TrimSpace(attrValue)
			if cleanValue == "" {
				continue
			}

			// 检查最小长度
			if len(cleanValue) < e.minTextLength {
				continue
			}

			// 创建属性节点
			extractNode := &ExtractableNode{
				Selection:     s,
				Text:          cleanValue,
				Path:          fmt.Sprintf("%s/@%s", path, attrName),
				IsAttribute:   true,
				AttributeName: attrName,
				NodeType:      "attribute",
				CanTranslate:  true,
				ParentTag:     e.getElementTag(s),
				Context: ExtractContext{
					BeforeContext: e.getElementContext(s, "before"),
					AfterContext:  e.getElementContext(s, "after"),
				},
			}

			nodes = append(nodes, extractNode)
			*nodeID++
		}
	}

	return nodes
}

// getParentTag 获取父标签名
func (e *HTMLSmartExtractor) getParentTag(s *goquery.Selection) string {
	parent := s.Parent()
	if parent.Nodes != nil && len(parent.Nodes) > 0 {
		return strings.ToLower(parent.Nodes[0].Data)
	}
	return ""
}

// getElementTag 获取元素标签名
func (e *HTMLSmartExtractor) getElementTag(s *goquery.Selection) string {
	if s.Nodes != nil && len(s.Nodes) > 0 {
		return strings.ToLower(s.Nodes[0].Data)
	}
	return ""
}

// getLeadingWhitespace 获取前导空白
func (e *HTMLSmartExtractor) getLeadingWhitespace(text string) string {
	if !e.preserveWhitespace {
		return ""
	}
	
	for i, ch := range text {
		if ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r' {
			return text[:i]
		}
	}
	return text
}

// getTrailingWhitespace 获取尾随空白
func (e *HTMLSmartExtractor) getTrailingWhitespace(text string) string {
	if !e.preserveWhitespace {
		return ""
	}
	
	for i := len(text) - 1; i >= 0; i-- {
		ch := text[i]
		if ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r' {
			return text[i+1:]
		}
	}
	return text
}

// getBeforeContext 获取前文上下文
func (e *HTMLSmartExtractor) getBeforeContext(s *goquery.Selection) string {
	// 获取前一个兄弟节点的文本作为上下文
	prev := s.Prev()
	if prev.Length() > 0 {
		text := strings.TrimSpace(prev.Text())
		if len(text) > 50 {
			return text[len(text)-50:]
		}
		return text
	}
	return ""
}

// getAfterContext 获取后文上下文
func (e *HTMLSmartExtractor) getAfterContext(s *goquery.Selection) string {
	// 获取后一个兄弟节点的文本作为上下文
	next := s.Next()
	if next.Length() > 0 {
		text := strings.TrimSpace(next.Text())
		if len(text) > 50 {
			return text[:50]
		}
		return text
	}
	return ""
}

// getElementContext 获取元素上下文
func (e *HTMLSmartExtractor) getElementContext(s *goquery.Selection, direction string) string {
	switch direction {
	case "before":
		prev := s.Prev()
		if prev.Length() > 0 {
			text := strings.TrimSpace(prev.Text())
			if len(text) > 30 {
				return text[len(text)-30:]
			}
			return text
		}
	case "after":
		next := s.Next()
		if next.Length() > 0 {
			text := strings.TrimSpace(next.Text())
			if len(text) > 30 {
				return text[:30]
			}
			return text
		}
	}
	return ""
}

// countNodesByType 按类型统计节点数量
func (e *HTMLSmartExtractor) countNodesByType(nodes []*ExtractableNode, nodeType string) int {
	count := 0
	for _, node := range nodes {
		if node.NodeType == nodeType {
			count++
		}
	}
	return count
}

// ApplyTranslations 应用翻译结果到节点
func (e *HTMLSmartExtractor) ApplyTranslations(nodes []*ExtractableNode, translations map[string]string) error {
	for _, node := range nodes {
		translatedText, exists := translations[node.Path]
		if !exists {
			continue
		}

		if node.IsAttribute {
			// 更新属性
			node.Selection.SetAttr(node.AttributeName, translatedText)
		} else {
			// 更新文本节点
			if node.Selection.Nodes != nil && len(node.Selection.Nodes) > 0 {
				htmlNode := node.Selection.Nodes[0]
				if htmlNode.Type == html.TextNode {
					// 保留原始空白
					finalText := node.Context.LeadingWhitespace + translatedText + node.Context.TrailingWhitespace
					htmlNode.Data = finalText
				}
			}
		}
	}

	return nil
}

// GetPlaceholderManager 获取占位符管理器
func (e *HTMLSmartExtractor) GetPlaceholderManager() *HTMLPlaceholderManager {
	return e.placeholderManager
}