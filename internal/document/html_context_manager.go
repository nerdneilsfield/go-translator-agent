package document

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
	"golang.org/x/net/html"
)

// HTMLContextManager HTML上下文管理器
type HTMLContextManager struct {
	logger          *zap.Logger
	maxContextNodes int
}

// NewHTMLContextManager 创建HTML上下文管理器
func NewHTMLContextManager(logger *zap.Logger, maxContextNodes int) *HTMLContextManager {
	return &HTMLContextManager{
		logger:          logger,
		maxContextNodes: maxContextNodes,
	}
}

// BuildTranslationContext 构建翻译上下文
func (cm *HTMLContextManager) BuildTranslationContext(selection *goquery.Selection) *HTMLTranslationContext {
	if selection == nil || selection.Nodes == nil || len(selection.Nodes) == 0 {
		return &HTMLTranslationContext{}
	}

	context := &HTMLTranslationContext{
		ParentElement:   selection.Parent(),
		SiblingsBefore:  cm.extractSiblingsBefore(selection),
		SiblingsAfter:   cm.extractSiblingsAfter(selection),
		ElementTag:      cm.getElementTag(selection),
		ElementAttrs:    cm.getElementAttributes(selection),
		DocumentSection: cm.getDocumentSection(selection),
		SemanticHints:   cm.getSemanticHints(selection),
	}

	return context
}

// extractSiblingsBefore 提取前兄弟节点
func (cm *HTMLContextManager) extractSiblingsBefore(selection *goquery.Selection) []string {
	var siblings []string
	count := 0

	// 向前遍历兄弟节点
	prev := selection.Prev()
	for prev.Length() > 0 && count < cm.maxContextNodes {
		text := cm.extractMeaningfulText(prev)
		if text != "" {
			siblings = append([]string{text}, siblings...) // 前置插入保持顺序
			count++
		}
		prev = prev.Prev()
	}

	return siblings
}

// extractSiblingsAfter 提取后兄弟节点
func (cm *HTMLContextManager) extractSiblingsAfter(selection *goquery.Selection) []string {
	var siblings []string
	count := 0

	// 向后遍历兄弟节点
	next := selection.Next()
	for next.Length() > 0 && count < cm.maxContextNodes {
		text := cm.extractMeaningfulText(next)
		if text != "" {
			siblings = append(siblings, text)
			count++
		}
		next = next.Next()
	}

	return siblings
}

// extractMeaningfulText 提取有意义的文本
func (cm *HTMLContextManager) extractMeaningfulText(selection *goquery.Selection) string {
	if selection.Nodes == nil || len(selection.Nodes) == 0 {
		return ""
	}

	node := selection.Nodes[0]

	// 跳过脚本、样式等元素
	if cm.shouldSkipForContext(node.Data) {
		return ""
	}

	// 获取文本内容
	text := strings.TrimSpace(selection.Text())
	if len(text) == 0 {
		return ""
	}

	// 限制长度
	if len(text) > 100 {
		text = text[:100] + "..."
	}

	return text
}

// shouldSkipForContext 检查是否应跳过上下文提取
func (cm *HTMLContextManager) shouldSkipForContext(tagName string) bool {
	skipTags := map[string]bool{
		"script":   true,
		"style":    true,
		"noscript": true,
		"iframe":   true,
		"object":   true,
		"embed":    true,
		"canvas":   true,
		"svg":      true,
		"math":     true,
	}

	return skipTags[strings.ToLower(tagName)]
}

// getElementTag 获取元素标签
func (cm *HTMLContextManager) getElementTag(selection *goquery.Selection) string {
	if selection.Nodes != nil && len(selection.Nodes) > 0 {
		if selection.Nodes[0].Type == html.ElementNode {
			return strings.ToLower(selection.Nodes[0].Data)
		}
	}

	// 如果是文本节点，获取父元素标签
	parent := selection.Parent()
	if parent.Nodes != nil && len(parent.Nodes) > 0 {
		return strings.ToLower(parent.Nodes[0].Data)
	}

	return ""
}

// getElementAttributes 获取元素属性
func (cm *HTMLContextManager) getElementAttributes(selection *goquery.Selection) map[string]string {
	attrs := make(map[string]string)

	if selection.Nodes == nil || len(selection.Nodes) == 0 {
		return attrs
	}

	node := selection.Nodes[0]
	if node.Type != html.ElementNode {
		// 如果是文本节点，获取父元素属性
		parent := selection.Parent()
		if parent.Nodes != nil && len(parent.Nodes) > 0 {
			node = parent.Nodes[0]
		} else {
			return attrs
		}
	}

	// 提取重要属性
	importantAttrs := []string{"id", "class", "role", "aria-label", "title", "data-semantic"}
	for _, attr := range importantAttrs {
		if value, exists := selection.Attr(attr); exists {
			attrs[attr] = value
		}
	}

	return attrs
}

// getDocumentSection 获取文档区域
func (cm *HTMLContextManager) getDocumentSection(selection *goquery.Selection) string {
	// 向上遍历找到语义容器
	current := selection
	for current.Length() > 0 {
		if current.Nodes != nil && len(current.Nodes) > 0 {
			tagName := strings.ToLower(current.Nodes[0].Data)
			
			// HTML5语义标签
			switch tagName {
			case "header":
				return "header"
			case "nav":
				return "navigation"
			case "main":
				return "main"
			case "article":
				return "article"
			case "section":
				return "section"
			case "aside":
				return "sidebar"
			case "footer":
				return "footer"
			case "figure":
				return "figure"
			case "figcaption":
				return "caption"
			}

			// 通过class或id推断
			if class, exists := current.Attr("class"); exists {
				classLower := strings.ToLower(class)
				if strings.Contains(classLower, "header") {
					return "header"
				}
				if strings.Contains(classLower, "nav") || strings.Contains(classLower, "menu") {
					return "navigation"
				}
				if strings.Contains(classLower, "main") || strings.Contains(classLower, "content") {
					return "main"
				}
				if strings.Contains(classLower, "sidebar") || strings.Contains(classLower, "aside") {
					return "sidebar"
				}
				if strings.Contains(classLower, "footer") {
					return "footer"
				}
			}

			if id, exists := current.Attr("id"); exists {
				idLower := strings.ToLower(id)
				if strings.Contains(idLower, "header") {
					return "header"
				}
				if strings.Contains(idLower, "nav") || strings.Contains(idLower, "menu") {
					return "navigation"
				}
				if strings.Contains(idLower, "main") || strings.Contains(idLower, "content") {
					return "main"
				}
				if strings.Contains(idLower, "sidebar") || strings.Contains(idLower, "aside") {
					return "sidebar"
				}
				if strings.Contains(idLower, "footer") {
					return "footer"
				}
			}
		}

		current = current.Parent()
	}

	return "body"
}

// getSemanticHints 获取语义提示
func (cm *HTMLContextManager) getSemanticHints(selection *goquery.Selection) []string {
	var hints []string

	// 从元素属性获取提示
	if role, exists := selection.Attr("role"); exists {
		hints = append(hints, "role:"+role)
	}

	if ariaLabel, exists := selection.Attr("aria-label"); exists && ariaLabel != "" {
		hints = append(hints, "aria-label:"+ariaLabel)
	}

	if title, exists := selection.Attr("title"); exists && title != "" {
		hints = append(hints, "title:"+title)
	}

	// 从父元素获取上下文提示
	parent := selection.Parent()
	if parent.Length() > 0 {
		parentTag := cm.getElementTag(parent)
		switch parentTag {
		case "blockquote":
			hints = append(hints, "quote")
		case "code", "pre":
			hints = append(hints, "code")
		case "em", "i":
			hints = append(hints, "emphasis")
		case "strong", "b":
			hints = append(hints, "strong")
		case "h1", "h2", "h3", "h4", "h5", "h6":
			hints = append(hints, "heading")
		case "li":
			hints = append(hints, "list-item")
		case "th", "td":
			hints = append(hints, "table-cell")
		case "figcaption":
			hints = append(hints, "caption")
		case "cite":
			hints = append(hints, "citation")
		case "dfn":
			hints = append(hints, "definition")
		case "abbr":
			hints = append(hints, "abbreviation")
		}

		// 检查是否在表单中
		if cm.isInForm(selection) {
			hints = append(hints, "form-element")
		}

		// 检查是否在列表中
		if cm.isInList(selection) {
			hints = append(hints, "list-content")
		}

		// 检查是否在表格中
		if cm.isInTable(selection) {
			hints = append(hints, "table-content")
		}
	}

	return hints
}

// isInForm 检查是否在表单中
func (cm *HTMLContextManager) isInForm(selection *goquery.Selection) bool {
	current := selection
	for current.Length() > 0 {
		if current.Nodes != nil && len(current.Nodes) > 0 {
			if strings.ToLower(current.Nodes[0].Data) == "form" {
				return true
			}
		}
		current = current.Parent()
	}
	return false
}

// isInList 检查是否在列表中
func (cm *HTMLContextManager) isInList(selection *goquery.Selection) bool {
	current := selection
	for current.Length() > 0 {
		if current.Nodes != nil && len(current.Nodes) > 0 {
			tagName := strings.ToLower(current.Nodes[0].Data)
			if tagName == "ul" || tagName == "ol" || tagName == "dl" {
				return true
			}
		}
		current = current.Parent()
	}
	return false
}

// isInTable 检查是否在表格中
func (cm *HTMLContextManager) isInTable(selection *goquery.Selection) bool {
	current := selection
	for current.Length() > 0 {
		if current.Nodes != nil && len(current.Nodes) > 0 {
			tagName := strings.ToLower(current.Nodes[0].Data)
			if tagName == "table" {
				return true
			}
		}
		current = current.Parent()
	}
	return false
}