package markdown

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkdownProcessor(t *testing.T) {
	// 创建处理器
	processor, err := NewProcessor(document.ProcessorOptions{
		ChunkSize:    1000,
		ChunkOverlap: 50,
	})
	require.NoError(t, err)
	require.NotNil(t, processor)

	// 测试 Markdown 内容
	markdown := `# Title

This is a paragraph with some **bold** text and *italic* text.

## Section 1

Here's a code block:

` + "```go" + `
func main() {
    fmt.Println("Hello, World!")
}
` + "```" + `

### Subsection 1.1

- Item 1
- Item 2
- Item 3

And a table:

| Column 1 | Column 2 |
|----------|----------|
| Value 1  | Value 2  |
| Value 3  | Value 4  |

Some math: $E = mc^2$

And a block equation:

$$
\int_{-\infty}^{\infty} e^{-x^2} dx = \sqrt{\pi}
$$

> This is a quote block
> with multiple lines`

	ctx := context.Background()

	// 测试解析
	t.Run("Parse", func(t *testing.T) {
		reader := strings.NewReader(markdown)
		doc, err := processor.Parse(ctx, reader)
		require.NoError(t, err)
		require.NotNil(t, doc)

		// 验证文档格式
		assert.Equal(t, document.FormatMarkdown, doc.Format)
		assert.NotEmpty(t, doc.Blocks)

		// 验证块类型
		blockTypes := make(map[document.BlockType]int)
		for _, block := range doc.Blocks {
			blockTypes[block.GetType()]++
		}

		assert.Greater(t, blockTypes[document.BlockTypeHeading], 0)
		assert.Greater(t, blockTypes[document.BlockTypeParagraph], 0)
		assert.Greater(t, blockTypes[document.BlockTypeCode], 0)
		assert.Greater(t, blockTypes[document.BlockTypeList], 0)
		assert.Greater(t, blockTypes[document.BlockTypeTable], 0)
		assert.Greater(t, blockTypes[document.BlockTypeQuote], 0)
	})

	// 测试处理（翻译）
	t.Run("Process", func(t *testing.T) {
		reader := strings.NewReader(markdown)
		doc, err := processor.Parse(ctx, reader)
		require.NoError(t, err)

		// 模拟翻译函数
		translateFunc := func(ctx context.Context, text string) (string, error) {
			// 简单的模拟翻译：添加 [TRANSLATED] 前缀
			return "[TRANSLATED] " + text, nil
		}

		processedDoc, err := processor.Process(ctx, doc, translateFunc)
		require.NoError(t, err)
		require.NotNil(t, processedDoc)

		// 验证可翻译的块被翻译了
		for _, block := range processedDoc.Blocks {
			if block.IsTranslatable() && block.GetType() != document.BlockTypeCode {
				assert.Contains(t, block.GetContent(), "[TRANSLATED]")
			}
		}
	})

	// 测试渲染
	t.Run("Render", func(t *testing.T) {
		reader := strings.NewReader(markdown)
		doc, err := processor.Parse(ctx, reader)
		require.NoError(t, err)

		var output bytes.Buffer
		err = processor.Render(ctx, doc, &output)
		require.NoError(t, err)

		rendered := output.String()
		assert.NotEmpty(t, rendered)

		// 验证关键元素被保留
		assert.Contains(t, rendered, "# Title")
		assert.Contains(t, rendered, "```go")
		assert.Contains(t, rendered, "| Column 1 | Column 2 |")
		assert.Contains(t, rendered, "$E = mc^2$")
		assert.Contains(t, rendered, "$$")
		assert.Contains(t, rendered, "> This is a quote block")
	})

	// 测试端到端流程
	t.Run("EndToEnd", func(t *testing.T) {
		reader := strings.NewReader(markdown)
		doc, err := processor.Parse(ctx, reader)
		require.NoError(t, err)

		// 翻译
		translateFunc := func(ctx context.Context, text string) (string, error) {
			// 对于测试，我们只翻译包含特定词的内容
			if strings.Contains(text, "paragraph") {
				return strings.ReplaceAll(text, "paragraph", "段落"), nil
			}
			return text, nil
		}

		processedDoc, err := processor.Process(ctx, doc, translateFunc)
		require.NoError(t, err)

		// 渲染
		var output bytes.Buffer
		err = processor.Render(ctx, processedDoc, &output)
		require.NoError(t, err)

		result := output.String()
		assert.Contains(t, result, "段落")    // 验证翻译生效
		assert.Contains(t, result, "```go") // 验证代码块保持不变
	})
}

func TestMarkdownParser(t *testing.T) {
	parser := NewParser()
	ctx := context.Background()

	t.Run("SimpleMarkdown", func(t *testing.T) {
		markdown := `# Header

Paragraph text.

- List item 1
- List item 2`

		reader := strings.NewReader(markdown)
		doc, err := parser.Parse(ctx, reader)
		require.NoError(t, err)
		require.NotNil(t, doc)

		assert.Equal(t, 3, len(doc.Blocks)) // Header, Paragraph, List
	})

	t.Run("ComplexStructures", func(t *testing.T) {
		markdown := `## Nested Code

Here's inline ` + "`code`" + ` and a block:

` + "```python" + `
def hello():
    print("Hello")
` + "```" + `

> Quote with **bold** text`

		reader := strings.NewReader(markdown)
		doc, err := parser.Parse(ctx, reader)
		require.NoError(t, err)

		// 验证不同类型的块
		hasCode := false
		hasQuote := false
		for _, block := range doc.Blocks {
			if block.GetType() == document.BlockTypeCode {
				hasCode = true
			}
			if block.GetType() == document.BlockTypeQuote {
				hasQuote = true
			}
		}
		assert.True(t, hasCode)
		assert.True(t, hasQuote)
	})
}
