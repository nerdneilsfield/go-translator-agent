package html

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTMLProcessors(t *testing.T) {
	// 测试 HTML 内容
	htmlContent := `<!DOCTYPE html>
<html>
<head>
    <title>Test Document</title>
    <meta name="description" content="This is a test document">
</head>
<body>
    <h1>Main Title</h1>
    <p>This is a <strong>paragraph</strong> with some <em>emphasis</em> and a <a href="https://example.com">link</a>.</p>
    
    <h2>Section 1</h2>
    <ul>
        <li>First item</li>
        <li>Second item</li>
        <li>Third item</li>
    </ul>
    
    <blockquote>
        This is a quote block with some important text.
    </blockquote>
    
    <h3>Code Example</h3>
    <pre><code class="language-go">
func main() {
    fmt.Println("Hello, World!")
}
    </code></pre>
    
    <table>
        <tr>
            <th>Name</th>
            <th>Value</th>
        </tr>
        <tr>
            <td>Item 1</td>
            <td>Value 1</td>
        </tr>
    </table>
    
    <img src="/image.jpg" alt="Test image description">
</body>
</html>`

	ctx := context.Background()

	// 测试 Markdown 模式
	t.Run("MarkdownMode", func(t *testing.T) {
		processor, err := ProcessorWithMode(ModeMarkdown, document.ProcessorOptions{
			ChunkSize:    1000,
			ChunkOverlap: 50,
		})
		require.NoError(t, err)
		require.NotNil(t, processor)

		testHTMLProcessor(t, processor, htmlContent, ctx)
	})

	// 测试原生模式
	t.Run("NativeMode", func(t *testing.T) {
		processor, err := ProcessorWithMode(ModeNative, document.ProcessorOptions{
			ChunkSize:    1000,
			ChunkOverlap: 50,
		})
		require.NoError(t, err)
		require.NotNil(t, processor)

		testHTMLProcessor(t, processor, htmlContent, ctx)
	})
}

func testHTMLProcessor(t *testing.T, processor document.Processor, htmlContent string, ctx context.Context) {
	// 测试解析
	t.Run("Parse", func(t *testing.T) {
		reader := strings.NewReader(htmlContent)
		doc, err := processor.Parse(ctx, reader)
		require.NoError(t, err)
		require.NotNil(t, doc)

		// 验证文档格式
		assert.Equal(t, document.FormatHTML, doc.Format)
		assert.NotEmpty(t, doc.Blocks)

		// 验证提取了关键内容
		allContent := getAllBlocksAsString(doc.Blocks)
		assert.Contains(t, allContent, "Main Title") // h1
		assert.Contains(t, allContent, "Section 1")  // h2
		assert.Contains(t, allContent, "First item") // list item
	})

	// 测试处理（翻译）
	t.Run("Process", func(t *testing.T) {
		reader := strings.NewReader(htmlContent)
		doc, err := processor.Parse(ctx, reader)
		require.NoError(t, err)

		// 模拟翻译函数
		translateFunc := func(ctx context.Context, text string) (string, error) {
			// 简单的替换翻译
			translated := strings.ReplaceAll(text, "Main Title", "主标题")
			translated = strings.ReplaceAll(translated, "Section", "章节")
			translated = strings.ReplaceAll(translated, "Test", "测试")
			return translated, nil
		}

		processedDoc, err := processor.Process(ctx, doc, translateFunc)
		require.NoError(t, err)
		require.NotNil(t, processedDoc)

		// 验证翻译生效
		allContent := getAllBlocksAsString(processedDoc.Blocks)
		assert.Contains(t, allContent, "主标题")
		assert.Contains(t, allContent, "章节")
	})

	// 测试渲染
	t.Run("Render", func(t *testing.T) {
		reader := strings.NewReader(htmlContent)
		doc, err := processor.Parse(ctx, reader)
		require.NoError(t, err)

		// 翻译
		translateFunc := func(ctx context.Context, text string) (string, error) {
			return "[TRANSLATED] " + text, nil
		}
		processedDoc, err := processor.Process(ctx, doc, translateFunc)
		require.NoError(t, err)

		// 渲染
		var output bytes.Buffer
		err = processor.Render(ctx, processedDoc, &output)
		require.NoError(t, err)

		rendered := output.String()
		assert.NotEmpty(t, rendered)

		// 验证是 HTML 格式
		assert.Contains(t, rendered, "<")
		assert.Contains(t, rendered, ">")

		// 验证翻译的内容
		assert.Contains(t, rendered, "[TRANSLATED]")
	})
}

func TestHTMLToMarkdownConverter(t *testing.T) {
	converter := NewHTMLToMarkdown()

	t.Run("BasicElements", func(t *testing.T) {
		html := `<h1>Title</h1>
<p>Paragraph with <strong>bold</strong> and <em>italic</em>.</p>
<ul>
<li>Item 1</li>
<li>Item 2</li>
</ul>`

		reader := strings.NewReader(html)
		markdown, err := converter.Convert(reader)
		require.NoError(t, err)

		assert.Contains(t, markdown, "# Title")
		assert.Contains(t, markdown, "**bold**")
		assert.Contains(t, markdown, "*italic*")
		assert.Contains(t, markdown, "- Item 1")
		assert.Contains(t, markdown, "- Item 2")
	})

	t.Run("Links", func(t *testing.T) {
		html := `<p>Visit <a href="https://example.com">our website</a> for more info.</p>`

		reader := strings.NewReader(html)
		markdown, err := converter.Convert(reader)
		require.NoError(t, err)

		assert.Contains(t, markdown, "[our website](https://example.com)")
	})

	t.Run("CodeBlocks", func(t *testing.T) {
		html := `<pre><code class="language-python">def hello():
    print("Hello")</code></pre>`

		reader := strings.NewReader(html)
		markdown, err := converter.Convert(reader)
		require.NoError(t, err)

		assert.Contains(t, markdown, "```python")
		assert.Contains(t, markdown, "def hello():")
		assert.Contains(t, markdown, "```")
	})

	t.Run("Tables", func(t *testing.T) {
		html := `<table>
<tr><th>Header 1</th><th>Header 2</th></tr>
<tr><td>Cell 1</td><td>Cell 2</td></tr>
</table>`

		reader := strings.NewReader(html)
		markdown, err := converter.Convert(reader)
		require.NoError(t, err)

		assert.Contains(t, markdown, "| Header 1 | Header 2 |")
		assert.Contains(t, markdown, "|---|---|")
		assert.Contains(t, markdown, "| Cell 1 | Cell 2 |")
	})

	t.Run("NestedElements", func(t *testing.T) {
		html := `<div>
<h2>Section</h2>
<p>Text with <code>inline code</code> and nested <strong><em>bold italic</em></strong>.</p>
<ol>
<li>First <a href="#link">linked item</a></li>
<li>Second item</li>
</ol>
</div>`

		reader := strings.NewReader(html)
		markdown, err := converter.Convert(reader)
		require.NoError(t, err)

		assert.Contains(t, markdown, "## Section")
		assert.Contains(t, markdown, "`inline code`")
		assert.Contains(t, markdown, "***bold italic***")
		// 检查有序列表（可能有格式差异）
		assert.Contains(t, markdown, "First")
		assert.Contains(t, markdown, "[linked item](#link)")
		assert.Contains(t, markdown, "Second item")
	})
}

func TestNativeHTMLProcessing(t *testing.T) {
	parser := NewNativeParser()
	renderer := NewNativeRenderer()
	ctx := context.Background()

	t.Run("PreserveStructure", func(t *testing.T) {
		html := `<div class="container">
<h1 id="main">Title</h1>
<p class="intro">Introduction text</p>
</div>`

		// 解析
		reader := strings.NewReader(html)
		doc, err := parser.Parse(ctx, reader)
		require.NoError(t, err)

		// 修改内容
		for _, block := range doc.Blocks {
			if block.GetContent() == "Title" {
				block.SetContent("Modified Title")
			}
		}

		// 渲染
		var output bytes.Buffer
		err = renderer.Render(ctx, doc, &output)
		require.NoError(t, err)

		rendered := output.String()
		assert.Contains(t, rendered, "Modified Title")
		// 应该保留原始属性
		assert.Contains(t, rendered, `id="main"`)
	})

	t.Run("ExtractMetaAndAlt", func(t *testing.T) {
		html := `<html>
<head>
<meta name="description" content="Page description">
</head>
<body>
<img src="test.jpg" alt="Image description">
</body>
</html>`

		reader := strings.NewReader(html)
		doc, err := parser.Parse(ctx, reader)
		require.NoError(t, err)

		// 验证提取了 meta 和 alt 文本
		contents := getAllBlockContents(doc.Blocks)
		assert.Contains(t, contents, "Page description")
		assert.Contains(t, contents, "Image description")
	})
}

// getAllBlockContents 获取所有块的内容
func getAllBlockContents(blocks []document.Block) []string {
	var contents []string
	for _, block := range blocks {
		contents = append(contents, block.GetContent())
	}
	return contents
}

// getAllBlocksAsString 将所有块内容连接成一个字符串
func getAllBlocksAsString(blocks []document.Block) string {
	contents := getAllBlockContents(blocks)
	return strings.Join(contents, " ")
}
