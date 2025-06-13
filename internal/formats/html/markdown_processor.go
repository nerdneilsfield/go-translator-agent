package html

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/formats/markdown"
	"github.com/nerdneilsfield/go-translator-agent/pkg/document"
	"golang.org/x/net/html"
)

// MarkdownProcessor HTML 处理器（通过 Markdown 转换）
type MarkdownProcessor struct {
	converter    *HTMLToMarkdown
	mdProcessor  document.Processor
	opts         document.ProcessorOptions
}

// NewMarkdownProcessor 创建基于 Markdown 的 HTML 处理器
func NewMarkdownProcessor(opts document.ProcessorOptions) (*MarkdownProcessor, error) {
	// 创建 HTML 到 Markdown 转换器
	converter := NewHTMLToMarkdown()
	
	// 创建 Markdown 处理器
	mdProcessor, err := markdown.NewProcessor(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create markdown processor: %w", err)
	}

	return &MarkdownProcessor{
		converter:   converter,
		mdProcessor: mdProcessor,
		opts:        opts,
	}, nil
}

// Parse 解析 HTML 输入
func (p *MarkdownProcessor) Parse(ctx context.Context, input io.Reader) (*document.Document, error) {
	// 读取 HTML 内容
	htmlContent, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTML: %w", err)
	}

	// 保存原始 HTML 以便后续还原
	doc := &document.Document{
		ID:     generateDocumentID(),
		Format: document.FormatHTML,
		Metadata: document.DocumentMetadata{
			CustomFields: map[string]interface{}{
				"original_html": string(htmlContent),
			},
		},
	}

	// 转换为 Markdown
	markdownContent, err := p.converter.Convert(bytes.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("failed to convert HTML to Markdown: %w", err)
	}

	// 使用 Markdown 处理器解析
	markdownDoc, err := p.mdProcessor.Parse(ctx, strings.NewReader(markdownContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Markdown: %w", err)
	}

	// 复制块但保持 HTML 格式标识
	doc.Blocks = markdownDoc.Blocks
	doc.Resources = markdownDoc.Resources

	return doc, nil
}

// Process 处理文档（翻译）
func (p *MarkdownProcessor) Process(ctx context.Context, doc *document.Document, translator document.TranslateFunc) (*document.Document, error) {
	// 直接使用 Markdown 处理器的处理逻辑
	return p.mdProcessor.Process(ctx, doc, translator)
}

// Render 渲染文档回 HTML
func (p *MarkdownProcessor) Render(ctx context.Context, doc *document.Document, output io.Writer) error {
	// 先渲染为 Markdown
	var markdownBuffer bytes.Buffer
	err := p.mdProcessor.Render(ctx, doc, &markdownBuffer)
	if err != nil {
		return fmt.Errorf("failed to render to Markdown: %w", err)
	}

	// 获取原始 HTML（如果有）
	var originalHTML string
	if doc.Metadata.CustomFields != nil {
		if orig, ok := doc.Metadata.CustomFields["original_html"].(string); ok {
			originalHTML = orig
		}
	}

	// 将 Markdown 转换回 HTML
	htmlContent := p.markdownToHTML(markdownBuffer.String(), originalHTML)
	
	_, err = output.Write([]byte(htmlContent))
	return err
}

// GetFormat 返回处理器格式
func (p *MarkdownProcessor) GetFormat() document.Format {
	return document.FormatHTML
}

// markdownToHTML 将 Markdown 转换回 HTML
func (p *MarkdownProcessor) markdownToHTML(markdown string, originalHTML string) string {
	// 如果有原始 HTML，尝试保留其结构
	if originalHTML != "" {
		// 解析原始 HTML 以保留结构
		doc, err := html.Parse(strings.NewReader(originalHTML))
		if err == nil {
			// 找到 body
			body := findBodyNode(doc)
			if body != nil {
				// 清空 body 内容
				body.FirstChild = nil
				body.LastChild = nil
				
				// 将 Markdown 转换的 HTML 插入 body
				convertedHTML := simpleMarkdownToHTML(markdown)
				contentDoc, err := html.Parse(strings.NewReader("<body>" + convertedHTML + "</body>"))
				if err == nil {
					if contentBody := findBodyNode(contentDoc); contentBody != nil {
						// 移动所有子节点
						for child := contentBody.FirstChild; child != nil; {
							next := child.NextSibling
							// 先从原父节点移除
							contentBody.RemoveChild(child)
							// 然后添加到新父节点
							body.AppendChild(child)
							child = next
						}
					}
				}
				
				// 渲染回 HTML
				var buf bytes.Buffer
				html.Render(&buf, doc)
				return buf.String()
			}
		}
	}

	// 如果没有原始 HTML 或处理失败，创建简单的 HTML
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Translated Document</title>
</head>
<body>
%s
</body>
</html>`, simpleMarkdownToHTML(markdown))
}

// simpleMarkdownToHTML 简单的 Markdown 到 HTML 转换
func simpleMarkdownToHTML(markdown string) string {
	lines := strings.Split(markdown, "\n")
	var result strings.Builder
	inCodeBlock := false
	inList := false
	
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		
		// 代码块
		if strings.HasPrefix(trimmed, "```") {
			if !inCodeBlock {
				lang := strings.TrimPrefix(trimmed, "```")
				if lang != "" {
					result.WriteString(fmt.Sprintf(`<pre><code class="language-%s">`, lang))
				} else {
					result.WriteString("<pre><code>")
				}
				inCodeBlock = true
			} else {
				result.WriteString("</code></pre>\n")
				inCodeBlock = false
			}
			continue
		}
		
		if inCodeBlock {
			result.WriteString(html.EscapeString(line))
			result.WriteString("\n")
			continue
		}
		
		// 标题 - 处理可能被翻译的标题（如 "[TRANSLATED] # Title"）
		headingMatch := false
		headingContent := ""
		headingLevel := 0
		
		// 尝试匹配标准 Markdown 标题
		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for _, r := range trimmed {
				if r == '#' {
					level++
				} else {
					break
				}
			}
			if level > 0 && level <= 6 {
				headingMatch = true
				headingLevel = level
				headingContent = strings.TrimSpace(trimmed[level:])
			}
		} else {
			// 检查是否包含 Markdown 标题（可能在翻译后的文本中）
			for i := 1; i <= 6; i++ {
				prefix := strings.Repeat("#", i) + " "
				if idx := strings.Index(line, prefix); idx >= 0 {
					headingMatch = true
					headingLevel = i
					// 提取标题内容，去掉 # 和前面的内容
					headingContent = strings.TrimSpace(line[idx+len(prefix):])
					break
				}
			}
		}
		
		if headingMatch {
			result.WriteString(fmt.Sprintf("<h%d>%s</h%d>\n", headingLevel, processInlineMarkdown(headingContent), headingLevel))
			continue
		}
		
		// 列表
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				result.WriteString("<ul>\n")
				inList = true
			}
			content := strings.TrimSpace(trimmed[2:])
			result.WriteString(fmt.Sprintf("<li>%s</li>\n", processInlineMarkdown(content)))
			
			// 检查下一行是否还是列表项
			if i+1 >= len(lines) || (!strings.HasPrefix(strings.TrimSpace(lines[i+1]), "- ") && 
				!strings.HasPrefix(strings.TrimSpace(lines[i+1]), "* ")) {
				result.WriteString("</ul>\n")
				inList = false
			}
			continue
		}
		
		// 引用
		if strings.HasPrefix(trimmed, ">") {
			content := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
			result.WriteString(fmt.Sprintf("<blockquote>%s</blockquote>\n", processInlineMarkdown(content)))
			continue
		}
		
		// 空行
		if trimmed == "" {
			if inList {
				result.WriteString("</ul>\n")
				inList = false
			}
			continue
		}
		
		// 普通段落
		result.WriteString(fmt.Sprintf("<p>%s</p>\n", processInlineMarkdown(trimmed)))
	}
	
	return result.String()
}

// processInlineMarkdown 处理内联 Markdown 标记
func processInlineMarkdown(text string) string {
	// 转义 HTML
	text = html.EscapeString(text)
	
	// 粗体
	text = replacePattern(text, `\*\*([^*]+)\*\*`, "<strong>$1</strong>")
	
	// 斜体
	text = replacePattern(text, `\*([^*]+)\*`, "<em>$1</em>")
	
	// 代码
	text = replacePattern(text, "`([^`]+)`", "<code>$1</code>")
	
	// 链接
	text = replacePattern(text, `\[([^\]]+)\]\(([^)]+)\)`, `<a href="$2">$1</a>`)
	
	// 图片
	text = replacePattern(text, `!\[([^\]]*)\]\(([^)]+)\)`, `<img src="$2" alt="$1">`)
	
	return text
}

// replacePattern 替换正则模式
func replacePattern(text, pattern, replacement string) string {
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(text, replacement)
}

// findBodyNode 查找 body 节点
func findBodyNode(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "body" {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if body := findBodyNode(c); body != nil {
			return body
		}
	}
	return nil
}

// generateDocumentID 生成文档 ID
func generateDocumentID() string {
	return fmt.Sprintf("html_%d", time.Now().UnixNano())
}