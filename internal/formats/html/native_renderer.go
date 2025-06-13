package html

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/pkg/document"
	"golang.org/x/net/html"
)

// NativeRenderer 原生 HTML 渲染器
type NativeRenderer struct {
	preserveStructure bool
}

// NewNativeRenderer 创建原生 HTML 渲染器
func NewNativeRenderer() *NativeRenderer {
	return &NativeRenderer{
		preserveStructure: true,
	}
}

// Render 渲染文档为 HTML
func (r *NativeRenderer) Render(ctx context.Context, doc *document.Document, output io.Writer) error {
	if doc == nil {
		return fmt.Errorf("document is nil")
	}

	// 如果有原始 HTML 结构，使用它
	if doc.Metadata.CustomFields != nil {
		if htmlNode, ok := doc.Metadata.CustomFields["html_node"].(*html.Node); ok {
			// 更新 HTML 节点中的文本
			r.updateHTMLNodes(doc.Blocks)
			
			// 渲染 HTML
			return html.Render(output, htmlNode)
		}
	}

	// 否则创建新的 HTML
	return r.renderNewHTML(doc, output)
}

// CanRender 检查是否能渲染该格式
func (r *NativeRenderer) CanRender(format document.Format) bool {
	return format == document.FormatHTML
}

// updateHTMLNodes 更新 HTML 节点中的翻译文本
func (r *NativeRenderer) updateHTMLNodes(blocks []document.Block) {
	for _, block := range blocks {
		// 检查是否是 HTMLBlock
		if htmlBlock, ok := block.(*HTMLBlock); ok {
			node := htmlBlock.GetNode()
			if node == nil {
				continue
			}

			content := block.GetContent()
			metadata := block.GetMetadata()
			
			// 根据元数据确定如何更新
			if attrs := metadata.Attributes; attrs != nil {
				// 检查是否是文本节点
				if isTextNode, ok := attrs["text_node"].(bool); ok && isTextNode {
					// 直接更新文本节点
					node.Data = content
					continue
				}
				
				// 检查是否是属性更新
				if attrName, ok := attrs["attr"].(string); ok && attrName != "" {
					// 更新属性值
					r.updateNodeAttribute(node, attrName, content)
					continue
				}
				
				// 默认更新元素的文本内容
				r.updateElementText(node, content)
			}
		}
	}
}

// updateNodeAttribute 更新节点属性
func (r *NativeRenderer) updateNodeAttribute(node *html.Node, attrName, value string) {
	for i, attr := range node.Attr {
		if attr.Key == attrName {
			node.Attr[i].Val = value
			return
		}
	}
	// 如果属性不存在，添加它
	node.Attr = append(node.Attr, html.Attribute{
		Key: attrName,
		Val: value,
	})
}

// updateElementText 更新元素的文本内容
func (r *NativeRenderer) updateElementText(node *html.Node, text string) {
	// 移除所有子节点
	node.FirstChild = nil
	node.LastChild = nil
	
	// 添加新的文本节点
	textNode := &html.Node{
		Type: html.TextNode,
		Data: text,
	}
	node.AppendChild(textNode)
}

// renderNewHTML 创建新的 HTML 文档
func (r *NativeRenderer) renderNewHTML(doc *document.Document, output io.Writer) error {
	// 创建基本的 HTML 结构
	htmlContent := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Translated Document</title>
</head>
<body>
`

	// 渲染每个块
	for _, block := range doc.Blocks {
		blockHTML := r.renderBlock(block)
		htmlContent += blockHTML + "\n"
	}

	htmlContent += `</body>
</html>`

	_, err := output.Write([]byte(htmlContent))
	return err
}

// renderBlock 渲染单个块
func (r *NativeRenderer) renderBlock(block document.Block) string {
	if block == nil {
		return ""
	}

	content := html.EscapeString(block.GetContent())
	
	switch block.GetType() {
	case document.BlockTypeHeading:
		level := block.GetMetadata().Level
		if level == 0 {
			level = 1
		}
		return fmt.Sprintf("<h%d>%s</h%d>", level, content, level)
		
	case document.BlockTypeParagraph:
		// 检查元数据中的原始标签
		if attrs := block.GetMetadata().Attributes; attrs != nil {
			if tag, ok := attrs["tag"].(string); ok {
				switch tag {
				case "div":
					return fmt.Sprintf("<div>%s</div>", content)
				case "span":
					return fmt.Sprintf("<span>%s</span>", content)
				case "li":
					return fmt.Sprintf("<li>%s</li>", content)
				case "td":
					return fmt.Sprintf("<td>%s</td>", content)
				case "th":
					return fmt.Sprintf("<th>%s</th>", content)
				case "a":
					href := ""
					if h, ok := attrs["href"].(string); ok {
						href = h
					}
					return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(href), content)
				}
			}
		}
		return fmt.Sprintf("<p>%s</p>", content)
		
	case document.BlockTypeCode:
		return fmt.Sprintf("<pre><code>%s</code></pre>", content)
		
	case document.BlockTypeList:
		// 简单处理，创建无序列表
		items := strings.Split(content, "\n")
		result := "<ul>\n"
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item != "" {
				// 移除列表标记
				item = strings.TrimPrefix(item, "- ")
				item = strings.TrimPrefix(item, "* ")
				item = strings.TrimPrefix(item, "+ ")
				result += fmt.Sprintf("  <li>%s</li>\n", html.EscapeString(item))
			}
		}
		result += "</ul>"
		return result
		
	case document.BlockTypeTable:
		// 简单的表格处理
		return fmt.Sprintf("<table>\n%s\n</table>", content)
		
	case document.BlockTypeQuote:
		return fmt.Sprintf("<blockquote>%s</blockquote>", content)
		
	case document.BlockTypeImage:
		// 从元数据获取 src
		src := ""
		if attrs := block.GetMetadata().Attributes; attrs != nil {
			if s, ok := attrs["src"].(string); ok {
				src = s
			}
		}
		return fmt.Sprintf(`<img src="%s" alt="%s">`, html.EscapeString(src), content)
		
	default:
		return fmt.Sprintf("<div>%s</div>", content)
	}
}