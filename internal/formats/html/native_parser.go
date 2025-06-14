package html

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"golang.org/x/net/html"
)

// NativeParser 原生 HTML 解析器
type NativeParser struct {
	preserveAttributes bool
	preserveComments   bool
}

// NewNativeParser 创建原生 HTML 解析器
func NewNativeParser() *NativeParser {
	return &NativeParser{
		preserveAttributes: true,
		preserveComments:   false,
	}
}

// Parse 解析 HTML 文档
func (p *NativeParser) Parse(ctx context.Context, input io.Reader) (*document.Document, error) {
	// 解析 HTML
	htmlDoc, err := html.Parse(input)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// 创建文档
	doc := &document.Document{
		ID:        generateDocumentID(),
		Format:    document.FormatHTML,
		Metadata:  document.DocumentMetadata{},
		Blocks:    []document.Block{},
		Resources: make(map[string]document.Resource),
	}

	// 保存原始 HTML 结构到元数据
	doc.Metadata.CustomFields = map[string]interface{}{
		"html_node": htmlDoc,
	}

	// 提取可翻译的文本块
	blocks := p.extractBlocks(htmlDoc)
	doc.Blocks = blocks

	return doc, nil
}

// CanParse 检查是否能解析该格式
func (p *NativeParser) CanParse(format document.Format) bool {
	return format == document.FormatHTML
}

// extractBlocks 提取 HTML 中的可翻译块
func (p *NativeParser) extractBlocks(n *html.Node) []document.Block {
	var blocks []document.Block

	// 使用深度优先遍历提取文本块
	p.traverseNode(n, &blocks, []string{})

	return blocks
}

// traverseNode 遍历 HTML 节点树
func (p *NativeParser) traverseNode(n *html.Node, blocks *[]document.Block, path []string) {
	if n == nil {
		return
	}

	switch n.Type {
	case html.ElementNode:
		// 更新路径
		newPath := append(path, n.Data)

		// 特殊处理某些元素
		switch n.Data {
		case "script", "style", "noscript":
			// 跳过这些元素
			return

		case "h1", "h2", "h3", "h4", "h5", "h6":
			// 标题元素
			text := p.extractTextContent(n)
			if strings.TrimSpace(text) != "" {
				level := int(n.Data[1] - '0')
				*blocks = append(*blocks, &HTMLBlock{
					BaseBlock: document.BaseBlock{
						Type:         document.BlockTypeHeading,
						Content:      text,
						Translatable: true,
						Metadata: document.BlockMetadata{
							Level: level,
							Attributes: map[string]interface{}{
								"path":     newPath,
								"tag":      n.Data,
								"node_ref": n,
							},
						},
					},
					node: n,
				})
			}

		case "p", "div", "span", "li", "td", "th", "dt", "dd":
			// 普通文本容器
			text := p.extractDirectText(n)
			if strings.TrimSpace(text) != "" {
				*blocks = append(*blocks, &HTMLBlock{
					BaseBlock: document.BaseBlock{
						Type:         document.BlockTypeParagraph,
						Content:      text,
						Translatable: true,
						Metadata: document.BlockMetadata{
							Attributes: map[string]interface{}{
								"path":     newPath,
								"tag":      n.Data,
								"node_ref": n,
							},
						},
					},
					node: n,
				})
			}

		case "a":
			// 链接文本
			text := p.extractTextContent(n)
			if strings.TrimSpace(text) != "" {
				href := p.getAttr(n, "href")
				*blocks = append(*blocks, &HTMLBlock{
					BaseBlock: document.BaseBlock{
						Type:         document.BlockTypeParagraph,
						Content:      text,
						Translatable: true,
						Metadata: document.BlockMetadata{
							Attributes: map[string]interface{}{
								"path":     newPath,
								"tag":      n.Data,
								"href":     href,
								"node_ref": n,
							},
						},
					},
					node: n,
				})
			}

		case "img":
			// 图片的 alt 文本
			alt := p.getAttr(n, "alt")
			if alt != "" {
				*blocks = append(*blocks, &HTMLBlock{
					BaseBlock: document.BaseBlock{
						Type:         document.BlockTypeImage,
						Content:      alt,
						Translatable: true,
						Metadata: document.BlockMetadata{
							Attributes: map[string]interface{}{
								"path":     newPath,
								"tag":      n.Data,
								"src":      p.getAttr(n, "src"),
								"node_ref": n,
								"attr":     "alt",
							},
						},
					},
					node: n,
				})
			}

		case "title":
			// 页面标题
			text := p.extractTextContent(n)
			if strings.TrimSpace(text) != "" {
				*blocks = append(*blocks, &HTMLBlock{
					BaseBlock: document.BaseBlock{
						Type:         document.BlockTypeParagraph,
						Content:      text,
						Translatable: true,
						Metadata: document.BlockMetadata{
							Attributes: map[string]interface{}{
								"path":     newPath,
								"tag":      n.Data,
								"node_ref": n,
							},
						},
					},
					node: n,
				})
			}

		case "meta":
			// meta 标签的 content
			name := p.getAttr(n, "name")
			if name == "description" || name == "keywords" {
				content := p.getAttr(n, "content")
				if content != "" {
					*blocks = append(*blocks, &HTMLBlock{
						BaseBlock: document.BaseBlock{
							Type:         document.BlockTypeParagraph,
							Content:      content,
							Translatable: true,
							Metadata: document.BlockMetadata{
								Attributes: map[string]interface{}{
									"path":     newPath,
									"tag":      n.Data,
									"name":     name,
									"node_ref": n,
									"attr":     "content",
								},
							},
						},
						node: n,
					})
				}
			}
		}

		// 递归处理子节点
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			p.traverseNode(c, blocks, newPath)
		}

	case html.TextNode:
		// 处理纯文本节点
		text := strings.TrimSpace(n.Data)
		if text != "" && len(path) > 0 {
			// 检查父元素是否应该被忽略
			parent := path[len(path)-1]
			if parent != "script" && parent != "style" && parent != "noscript" {
				// 检查是否已经在某个块中处理过
				// 这里简化处理，直接创建文本块
				*blocks = append(*blocks, &HTMLBlock{
					BaseBlock: document.BaseBlock{
						Type:         document.BlockTypeParagraph,
						Content:      text,
						Translatable: true,
						Metadata: document.BlockMetadata{
							Attributes: map[string]interface{}{
								"path":      path,
								"node_ref":  n,
								"text_node": true,
							},
						},
					},
					node: n,
				})
			}
		}

	default:
		// 其他节点类型，递归处理子节点
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			p.traverseNode(c, blocks, path)
		}
	}
}

// extractTextContent 提取节点的所有文本内容
func (p *NativeParser) extractTextContent(n *html.Node) string {
	var text strings.Builder
	p.extractTextRecursive(n, &text)
	return strings.TrimSpace(text.String())
}

// extractDirectText 只提取直接子文本节点
func (p *NativeParser) extractDirectText(n *html.Node) string {
	var text strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			text.WriteString(c.Data)
		}
	}
	return strings.TrimSpace(text.String())
}

// extractTextRecursive 递归提取文本
func (p *NativeParser) extractTextRecursive(n *html.Node, text *strings.Builder) {
	if n.Type == html.TextNode {
		text.WriteString(n.Data)
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		// 跳过 script 和 style 标签
		if c.Type == html.ElementNode && (c.Data == "script" || c.Data == "style") {
			continue
		}
		p.extractTextRecursive(c, text)

		// 在块级元素后添加空格
		if c.Type == html.ElementNode && isBlockElement(c.Data) {
			text.WriteString(" ")
		}
	}
}

// getAttr 获取节点属性
func (p *NativeParser) getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// isBlockElement 判断是否是块级元素
func isBlockElement(tag string) bool {
	blockElements := map[string]bool{
		"p": true, "div": true, "h1": true, "h2": true, "h3": true,
		"h4": true, "h5": true, "h6": true, "ul": true, "ol": true,
		"li": true, "blockquote": true, "pre": true, "hr": true,
		"table": true, "tr": true, "td": true, "th": true,
		"form": true, "fieldset": true, "address": true,
		"article": true, "aside": true, "footer": true,
		"header": true, "main": true, "nav": true, "section": true,
	}
	return blockElements[tag]
}

// HTMLBlock HTML 块，保存对原始节点的引用
type HTMLBlock struct {
	document.BaseBlock
	node *html.Node
}

// GetNode 获取原始 HTML 节点
func (b *HTMLBlock) GetNode() *html.Node {
	return b.node
}
