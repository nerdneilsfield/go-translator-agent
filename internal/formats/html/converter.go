package html

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// HTMLToMarkdown 将 HTML 转换为 Markdown
type HTMLToMarkdown struct {
	// 配置选项
	preserveLinks   bool
	preserveImages  bool
	preserveClasses bool
}

// NewHTMLToMarkdown 创建 HTML 到 Markdown 转换器
func NewHTMLToMarkdown() *HTMLToMarkdown {
	return &HTMLToMarkdown{
		preserveLinks:  true,
		preserveImages: true,
	}
}

// Convert 将 HTML 转换为 Markdown
func (h *HTMLToMarkdown) Convert(reader io.Reader) (string, error) {
	doc, err := html.Parse(reader)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	// 找到 body 节点
	body := h.findBody(doc)
	if body == nil {
		// 如果没有 body，使用整个文档
		body = doc
	}

	// 转换为 Markdown
	var builder strings.Builder
	h.nodeToMarkdown(body, &builder, 0)

	// 清理多余的空行
	result := h.cleanupMarkdown(builder.String())
	return result, nil
}

// findBody 查找 body 节点
func (h *HTMLToMarkdown) findBody(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "body" {
		return n
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if body := h.findBody(c); body != nil {
			return body
		}
	}

	return nil
}

// nodeToMarkdown 将 HTML 节点转换为 Markdown
func (h *HTMLToMarkdown) nodeToMarkdown(n *html.Node, builder *strings.Builder, depth int) {
	switch n.Type {
	case html.TextNode:
		// 处理文本节点
		text := strings.TrimSpace(n.Data)
		if text != "" {
			builder.WriteString(text)
		}

	case html.ElementNode:
		switch n.Data {
		// 标题
		case "h1":
			builder.WriteString("\n\n# ")
			h.childrenToMarkdown(n, builder, depth)
			builder.WriteString("\n\n")
		case "h2":
			builder.WriteString("\n\n## ")
			h.childrenToMarkdown(n, builder, depth)
			builder.WriteString("\n\n")
		case "h3":
			builder.WriteString("\n\n### ")
			h.childrenToMarkdown(n, builder, depth)
			builder.WriteString("\n\n")
		case "h4":
			builder.WriteString("\n\n#### ")
			h.childrenToMarkdown(n, builder, depth)
			builder.WriteString("\n\n")
		case "h5":
			builder.WriteString("\n\n##### ")
			h.childrenToMarkdown(n, builder, depth)
			builder.WriteString("\n\n")
		case "h6":
			builder.WriteString("\n\n###### ")
			h.childrenToMarkdown(n, builder, depth)
			builder.WriteString("\n\n")

		// 段落
		case "p":
			builder.WriteString("\n\n")
			h.childrenToMarkdown(n, builder, depth)
			builder.WriteString("\n\n")

		// 强调
		case "strong", "b":
			builder.WriteString("**")
			h.childrenToMarkdown(n, builder, depth)
			builder.WriteString("**")
		case "em", "i":
			builder.WriteString("*")
			h.childrenToMarkdown(n, builder, depth)
			builder.WriteString("*")
		case "code":
			builder.WriteString("`")
			h.childrenToMarkdown(n, builder, depth)
			builder.WriteString("`")

		// 链接
		case "a":
			if h.preserveLinks {
				href := h.getAttr(n, "href")
				builder.WriteString("[")
				h.childrenToMarkdown(n, builder, depth)
				builder.WriteString("](")
				builder.WriteString(href)
				builder.WriteString(")")
			} else {
				h.childrenToMarkdown(n, builder, depth)
			}

		// 图片
		case "img":
			if h.preserveImages {
				src := h.getAttr(n, "src")
				alt := h.getAttr(n, "alt")
				builder.WriteString("![")
				builder.WriteString(alt)
				builder.WriteString("](")
				builder.WriteString(src)
				builder.WriteString(")")
			}

		// 列表
		case "ul":
			builder.WriteString("\n")
			h.listToMarkdown(n, builder, depth, false)
			builder.WriteString("\n")
		case "ol":
			builder.WriteString("\n")
			h.listToMarkdown(n, builder, depth, true)
			builder.WriteString("\n")

		// 引用
		case "blockquote":
			builder.WriteString("\n\n")
			h.blockquoteToMarkdown(n, builder, depth)
			builder.WriteString("\n\n")

		// 代码块
		case "pre":
			builder.WriteString("\n\n```")
			// 检查是否有语言标记
			if code := h.findChild(n, "code"); code != nil {
				lang := h.getClassLanguage(code)
				if lang != "" {
					builder.WriteString(lang)
				}
			}
			builder.WriteString("\n")
			h.childrenToMarkdown(n, builder, depth)
			builder.WriteString("\n```\n\n")

		// 换行
		case "br":
			builder.WriteString("  \n")

		// 分隔线
		case "hr":
			builder.WriteString("\n\n---\n\n")

		// 表格
		case "table":
			builder.WriteString("\n\n")
			h.tableToMarkdown(n, builder)
			builder.WriteString("\n\n")

		// 默认情况，递归处理子节点
		default:
			h.childrenToMarkdown(n, builder, depth)
		}

	default:
		// 其他节点类型，递归处理子节点
		h.childrenToMarkdown(n, builder, depth)
	}
}

// childrenToMarkdown 处理子节点
func (h *HTMLToMarkdown) childrenToMarkdown(n *html.Node, builder *strings.Builder, depth int) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		h.nodeToMarkdown(c, builder, depth)
	}
}

// listToMarkdown 处理列表
func (h *HTMLToMarkdown) listToMarkdown(n *html.Node, builder *strings.Builder, depth int, ordered bool) {
	counter := 1
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "li" {
			// 添加缩进
			for i := 0; i < depth; i++ {
				builder.WriteString("  ")
			}
			
			// 添加列表标记
			if ordered {
				builder.WriteString(fmt.Sprintf("%d. ", counter))
				counter++
			} else {
				builder.WriteString("- ")
			}
			
			// 处理列表项内容
			h.childrenToMarkdown(c, builder, depth+1)
			builder.WriteString("\n")
		}
	}
}

// blockquoteToMarkdown 处理引用块
func (h *HTMLToMarkdown) blockquoteToMarkdown(n *html.Node, builder *strings.Builder, depth int) {
	lines := h.extractText(n)
	for _, line := range strings.Split(lines, "\n") {
		if strings.TrimSpace(line) != "" {
			builder.WriteString("> ")
			builder.WriteString(strings.TrimSpace(line))
			builder.WriteString("\n")
		}
	}
}

// tableToMarkdown 处理表格
func (h *HTMLToMarkdown) tableToMarkdown(n *html.Node, builder *strings.Builder) {
	// 查找 thead 和 tbody
	var thead, tbody *html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			switch c.Data {
			case "thead":
				thead = c
			case "tbody":
				tbody = c
			}
		}
	}

	// 如果没有明确的 thead/tbody，尝试直接查找 tr
	var rows []*html.Node
	if thead == nil && tbody == nil {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "tr" {
				rows = append(rows, c)
			}
		}
	} else {
		// 收集所有行
		if thead != nil {
			for c := thead.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "tr" {
					rows = append(rows, c)
				}
			}
		}
		if tbody != nil {
			for c := tbody.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "tr" {
					rows = append(rows, c)
				}
			}
		}
	}

	if len(rows) == 0 {
		return
	}

	// 处理表格行
	for i, row := range rows {
		builder.WriteString("|")
		for cell := row.FirstChild; cell != nil; cell = cell.NextSibling {
			if cell.Type == html.ElementNode && (cell.Data == "td" || cell.Data == "th") {
				builder.WriteString(" ")
				text := h.extractText(cell)
				builder.WriteString(strings.TrimSpace(text))
				builder.WriteString(" |")
			}
		}
		builder.WriteString("\n")

		// 在第一行后添加分隔线
		if i == 0 {
			builder.WriteString("|")
			for cell := row.FirstChild; cell != nil; cell = cell.NextSibling {
				if cell.Type == html.ElementNode && (cell.Data == "td" || cell.Data == "th") {
					builder.WriteString("---|")
				}
			}
			builder.WriteString("\n")
		}
	}
}

// getAttr 获取节点属性
func (h *HTMLToMarkdown) getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// getClassLanguage 从 class 属性中提取语言
func (h *HTMLToMarkdown) getClassLanguage(n *html.Node) string {
	class := h.getAttr(n, "class")
	// 查找类似 "language-go" 的模式
	if strings.HasPrefix(class, "language-") {
		return strings.TrimPrefix(class, "language-")
	}
	// 查找类似 "lang-go" 的模式
	if strings.HasPrefix(class, "lang-") {
		return strings.TrimPrefix(class, "lang-")
	}
	return ""
}

// findChild 查找特定标签的子节点
func (h *HTMLToMarkdown) findChild(n *html.Node, tag string) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return c
		}
	}
	return nil
}

// extractText 提取节点的纯文本内容
func (h *HTMLToMarkdown) extractText(n *html.Node) string {
	var builder strings.Builder
	h.extractTextRecursive(n, &builder)
	return builder.String()
}

// extractTextRecursive 递归提取文本
func (h *HTMLToMarkdown) extractTextRecursive(n *html.Node, builder *strings.Builder) {
	if n.Type == html.TextNode {
		builder.WriteString(n.Data)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		h.extractTextRecursive(c, builder)
	}
}

// cleanupMarkdown 清理 Markdown 文本
func (h *HTMLToMarkdown) cleanupMarkdown(markdown string) string {
	// 移除多余的空行
	re := regexp.MustCompile(`\n{3,}`)
	markdown = re.ReplaceAllString(markdown, "\n\n")
	
	// 移除开头和结尾的空白
	markdown = strings.TrimSpace(markdown)
	
	return markdown
}