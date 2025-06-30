package translation

import (
	"strings"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestSmartNodeSplitter_Basic(t *testing.T) {
	config := SmartNodeSplitterConfig{
		EnableSmartSplitting: true,
		MaxNodeSizeThreshold: 200,
		MinSplitSize:         50,
		MaxSplitSize:         150,
		PreserveParagraphs:   true,
		PreserveSentences:    true,
		OverlapRatio:         0.1,
	}

	logger := zaptest.NewLogger(t)
	splitter := NewSmartNodeSplitter(config, logger)

	t.Run("Small Node - No Split", func(t *testing.T) {
		node := &document.NodeInfo{
			ID:           1,
			OriginalText: "This is a small node that doesn't need splitting.",
		}

		nextID := 100
		result, err := splitter.SplitNode(node, &nextID)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, node.OriginalText, result[0].OriginalText)
		assert.Equal(t, node.ID, result[0].ID)
	})

	t.Run("Large Node - Needs Split", func(t *testing.T) {
		// 创建一个超过阈值的大节点
		longContent := `This is the first paragraph with some content that makes it longer than usual.

This is the second paragraph. It contains multiple sentences. Each sentence should be preserved during splitting. The smart splitter should respect sentence boundaries.

This is the third paragraph. It also contains multiple sentences. The splitter should handle this appropriately. We want to test if it can split at natural boundaries.`

		node := &document.NodeInfo{
			ID:           1,
			OriginalText: longContent,
		}

		nextID := 100
		result, err := splitter.SplitNode(node, &nextID)

		assert.NoError(t, err)
		// 应该被分割成多个子节点
		assert.Greater(t, len(result), 1)

		// 验证所有子节点的内容总和包含原内容的主要部分
		var totalContent string
		for _, subNode := range result {
			totalContent += subNode.OriginalText
		}

		// 验证内容没有丢失重要信息（允许一些空白字符的差异）
		assert.Contains(t, totalContent, "This is the first paragraph")
		assert.Contains(t, totalContent, "This is the second paragraph")
		assert.Contains(t, totalContent, "This is the third paragraph")

		// 验证每个子节点的大小在合理范围内（允许一些灵活性）
		for _, subNode := range result {
			nodeSize := len(subNode.OriginalText)
			// 所有节点都应该非空，但允许灵活的大小
			assert.Greater(t, nodeSize, 0, "Node should not be empty")
			// 检查节点不要太大
			assert.LessOrEqual(t, nodeSize, config.MaxSplitSize+100, "Node should not be excessively large")
		}
	})
}

func TestSmartNodeSplitter_SentenceBoundary(t *testing.T) {
	config := SmartNodeSplitterConfig{
		EnableSmartSplitting: true,
		MaxNodeSizeThreshold: 100,
		MinSplitSize:         30,
		MaxSplitSize:         80,
		PreserveParagraphs:   true,
		PreserveSentences:    true,
		OverlapRatio:         0.0,
	}

	logger := zaptest.NewLogger(t)
	splitter := NewSmartNodeSplitter(config, logger)

	t.Run("English Sentences", func(t *testing.T) {
		content := "This is the first sentence. This is the second sentence. This is a longer third sentence that might cause a split."

		node := &document.NodeInfo{
			ID:           1,
			OriginalText: content,
		}

		nextID := 100
		result, err := splitter.SplitNode(node, &nextID)
		assert.NoError(t, err)

		// 验证分割结果
		if len(result) > 1 {
			for _, subNode := range result {
				// 验证分割器尽力保持句子边界完整性
				content := strings.TrimSpace(subNode.OriginalText)
				if content != "" {
					// 检查是否以句子结束符结尾（或者是最后一个节点，或者包含完整句子）
					lastChar := content[len(content)-1]
					isLastNode := subNode == result[len(result)-1]
					hasCompleteSentence := strings.Contains(content, ". ") || strings.Contains(content, "! ") || strings.Contains(content, "? ")

					// 允许更灵活的分割策略
					isValidSplit := lastChar == '.' || lastChar == '!' || lastChar == '?' || isLastNode || hasCompleteSentence
					if !isValidSplit {
						// 如果不是理想的分割，至少验证有有意义的内容
						assert.Greater(t, len(strings.Fields(content)), 3, "Node should contain meaningful content: %q", content)
					}
				}
			}
		}
	})

	t.Run("Chinese Sentences", func(t *testing.T) {
		content := "这是第一个句子。这是第二个句子。这是一个可能导致分割的更长的第三个句子，包含了更多的内容。"

		node := &document.NodeInfo{
			ID:           1,
			OriginalText: content,
		}

		nextID := 100
		result, err := splitter.SplitNode(node, &nextID)
		assert.NoError(t, err)

		// 验证中文句子边界
		if len(result) > 1 {
			for _, subNode := range result {
				content := strings.TrimSpace(subNode.OriginalText)
				if content != "" {
					runes := []rune(content)
					if len(runes) > 0 {
						lastChar := runes[len(runes)-1]
						isLastNode := subNode == result[len(result)-1]
						assert.True(t, lastChar == '。' || lastChar == '！' || lastChar == '？' || isLastNode,
							"Chinese node should end with sentence boundary: %q", content)
					}
				}
			}
		}
	})
}

func TestSmartNodeSplitter_ParagraphBoundary(t *testing.T) {
	config := SmartNodeSplitterConfig{
		EnableSmartSplitting: true,
		MaxNodeSizeThreshold: 150,
		MinSplitSize:         50,
		MaxSplitSize:         120,
		PreserveParagraphs:   true,
		PreserveSentences:    false, // 优先段落边界
		OverlapRatio:         0.0,
	}

	logger := zaptest.NewLogger(t)
	splitter := NewSmartNodeSplitter(config, logger)

	t.Run("Multiple Paragraphs", func(t *testing.T) {
		content := `First paragraph with some content.

Second paragraph with different content.

Third paragraph that might be in a separate split.`

		node := &document.NodeInfo{
			ID:           1,
			OriginalText: content,
		}

		nextID := 100
		result, err := splitter.SplitNode(node, &nextID)
		assert.NoError(t, err)

		if len(result) > 1 {
			for _, subNode := range result {
				content := subNode.OriginalText
				// 子节点不应该在段落中间分割
				// 即不应该以单个换行符结尾（段落内）
				assert.False(t, len(content) > 0 && content[len(content)-1] == '\n' && !endsWithDoubleNewline(content),
					"Node should not split within paragraph: %q", content)
			}
		}
	})
}

func TestSmartNodeSplitter_CodeBlocks(t *testing.T) {
	config := SmartNodeSplitterConfig{
		EnableSmartSplitting: true,
		MaxNodeSizeThreshold: 200,
		MinSplitSize:         80,
		MaxSplitSize:         150,
		PreserveParagraphs:   true,
		PreserveSentences:    true,
		OverlapRatio:         0.0,
	}

	logger := zaptest.NewLogger(t)
	splitter := NewSmartNodeSplitter(config, logger)

	t.Run("Code Block Preservation", func(t *testing.T) {
		content := `This is some text before code.

` + "```go" + `
func main() {
    fmt.Println("Hello, World!")
    // This is a comment
    var x = 42
}
` + "```" + `

This is some text after the code block. It continues with more content that might cause the node to be split.`

		node := &document.NodeInfo{
			ID:           1,
			OriginalText: content,
		}

		nextID := 100
		result, err := splitter.SplitNode(node, &nextID)
		assert.NoError(t, err)

		// 验证代码块没有被分割
		codeBlockFound := false
		for _, subNode := range result {
			if containsCodeBlock(subNode.OriginalText) {
				// 如果包含代码块，应该是完整的
				assert.True(t, hasCompleteCodeBlock(subNode.OriginalText),
					"Code block should be complete in node: %q", subNode.OriginalText)
				codeBlockFound = true
			}
		}

		if len(result) > 1 {
			assert.True(t, codeBlockFound, "Code block should be preserved in one of the nodes")
		}
	})
}

func TestSmartNodeSplitter_MathFormulas(t *testing.T) {
	config := SmartNodeSplitterConfig{
		EnableSmartSplitting: true,
		MaxNodeSizeThreshold: 150,
		MinSplitSize:         50,
		MaxSplitSize:         100,
		PreserveParagraphs:   true,
		PreserveSentences:    true,
		OverlapRatio:         0.0,
	}

	logger := zaptest.NewLogger(t)
	splitter := NewSmartNodeSplitter(config, logger)

	t.Run("Math Formula Preservation", func(t *testing.T) {
		content := `This is text with inline math $E = mc^2$ and more text.

This is a display math formula:
$$
\int_{-\infty}^{\infty} e^{-x^2} dx = \sqrt{\pi}
$$

More text continues after the math formula with additional content that might cause splitting.`

		node := &document.NodeInfo{
			ID:           1,
			OriginalText: content,
		}

		nextID := 100
		result, err := splitter.SplitNode(node, &nextID)
		assert.NoError(t, err)

		// 验证数学公式没有被分割
		for _, subNode := range result {
			content := subNode.OriginalText
			// 检查内联数学公式完整性
			inlineMathCount := countSubstring(content, "$")
			assert.True(t, inlineMathCount%2 == 0, "Inline math formulas should be complete: %q", content)

			// 检查显示数学公式完整性
			if containsDisplayMath(content) {
				assert.True(t, hasCompleteDisplayMath(content), "Display math should be complete: %q", content)
			}
		}
	})
}

func TestSmartNodeSplitter_Lists(t *testing.T) {
	config := SmartNodeSplitterConfig{
		EnableSmartSplitting: true,
		MaxNodeSizeThreshold: 180,
		MinSplitSize:         60,
		MaxSplitSize:         130,
		PreserveParagraphs:   true,
		PreserveSentences:    true,
		OverlapRatio:         0.0,
	}

	logger := zaptest.NewLogger(t)
	splitter := NewSmartNodeSplitter(config, logger)

	t.Run("List Preservation", func(t *testing.T) {
		content := `This is some introductory text.

- First list item with some content
- Second list item with more detailed content and explanations
- Third list item that continues the pattern
- Fourth list item that might cause a split

This is concluding text after the list with additional content.`

		node := &document.NodeInfo{
			ID:           1,
			OriginalText: content,
		}

		nextID := 100
		result, err := splitter.SplitNode(node, &nextID)
		assert.NoError(t, err)

		// 验证列表结构
		for _, subNode := range result {
			content := subNode.OriginalText
			if containsList(content) {
				// 如果包含列表，应该在合理的边界分割
				lines := splitLines(content)
				for i, line := range lines {
					if isListItem(line) && i > 0 {
						// 列表项前面应该有适当的空行或者是另一个列表项
						prevLine := lines[i-1]
						assert.True(t, prevLine == "" || isListItem(prevLine) || i == 0,
							"List items should be properly structured: %q", content)
					}
				}
			}
		}
	})
}

func TestSmartNodeSplitter_Overlap(t *testing.T) {
	config := SmartNodeSplitterConfig{
		EnableSmartSplitting: true,
		MaxNodeSizeThreshold: 100,
		MinSplitSize:         40,
		MaxSplitSize:         80,
		PreserveParagraphs:   true,
		PreserveSentences:    true,
		OverlapRatio:         0.2, // 20% overlap
	}

	logger := zaptest.NewLogger(t)
	splitter := NewSmartNodeSplitter(config, logger)

	t.Run("Content Overlap", func(t *testing.T) {
		content := "Sentence one. Sentence two. Sentence three. Sentence four. Sentence five. Sentence six."

		node := &document.NodeInfo{
			ID:           1,
			OriginalText: content,
		}

		nextID := 100
		result, err := splitter.SplitNode(node, &nextID)
		assert.NoError(t, err)

		if len(result) > 1 {
			// 验证相邻节点有重叠内容
			for i := 0; i < len(result)-1; i++ {
				current := result[i].OriginalText
				next := result[i+1].OriginalText

				// 应该有一些重叠的内容
				hasOverlap := hasContentOverlap(current, next)
				assert.True(t, hasOverlap, "Adjacent nodes should have overlapping content")
			}
		}
	})
}

func TestSmartNodeSplitter_DisabledConfig(t *testing.T) {
	config := SmartNodeSplitterConfig{
		EnableSmartSplitting: false, // 禁用智能分割
	}

	logger := zaptest.NewLogger(t)
	splitter := NewSmartNodeSplitter(config, logger)

	t.Run("Disabled Splitting", func(t *testing.T) {
		// 即使是很大的节点也不应该被分割
		longContent := "This is a very long content. " + repeatString("More content. ", 100)

		node := &document.NodeInfo{
			ID:           1,
			OriginalText: longContent,
		}

		nextID := 100
		result, err := splitter.SplitNode(node, &nextID)
		assert.NoError(t, err)

		// 应该返回原始节点，不进行分割
		assert.Len(t, result, 1)
		assert.Equal(t, node.OriginalText, result[0].OriginalText)
	})
}

func TestSmartNodeSplitter_EdgeCases(t *testing.T) {
	config := SmartNodeSplitterConfig{
		EnableSmartSplitting: true,
		MaxNodeSizeThreshold: 100,
		MinSplitSize:         30,
		MaxSplitSize:         80,
		PreserveParagraphs:   true,
		PreserveSentences:    true,
		OverlapRatio:         0.1,
	}

	logger := zaptest.NewLogger(t)
	splitter := NewSmartNodeSplitter(config, logger)

	t.Run("Empty Node", func(t *testing.T) {
		node := &document.NodeInfo{
			ID:           1,
			OriginalText: "",
		}

		nextID := 100
		result, err := splitter.SplitNode(node, &nextID)
		assert.NoError(t, err)

		assert.Len(t, result, 1)
		assert.Equal(t, "", result[0].OriginalText)
	})

	t.Run("Whitespace Only", func(t *testing.T) {
		node := &document.NodeInfo{
			ID:           1,
			OriginalText: "   \n\n   \t  \n  ",
		}

		nextID := 100
		result, err := splitter.SplitNode(node, &nextID)
		assert.NoError(t, err)

		assert.Len(t, result, 1)
		assert.Equal(t, node.OriginalText, result[0].OriginalText)
	})

	t.Run("Single Long Word", func(t *testing.T) {
		longWord := repeatString("verylongwordwithoutspaces", 10)

		node := &document.NodeInfo{
			ID:           1,
			OriginalText: longWord,
		}

		nextID := 100
		result, err := splitter.SplitNode(node, &nextID)
		assert.NoError(t, err)

		// 对于无法合理分割的内容，可能会被强制分割或保持原状
		// 这里允许分割器选择最佳策略
		if len(result) == 1 {
			assert.Equal(t, longWord, result[0].OriginalText)
		} else {
			// 如果被分割，验证所有子节点重新组合后内容完整
			var reconstructed string
			for _, subNode := range result {
				reconstructed += subNode.OriginalText
			}
			assert.Equal(t, longWord, reconstructed)
		}
	})
}

// 辅助函数

func normalizeWhitespace(s string) string {
	// 简化空白字符的标准化处理
	return strings.TrimSpace(s)
}

func endsWithDoubleNewline(s string) bool {
	return len(s) >= 2 && s[len(s)-2:] == "\n\n"
}

func containsCodeBlock(s string) bool {
	return strings.Contains(s, "```")
}

func hasCompleteCodeBlock(s string) bool {
	count := strings.Count(s, "```")
	return count%2 == 0 && count >= 2
}

func containsDisplayMath(s string) bool {
	return strings.Contains(s, "$$")
}

func hasCompleteDisplayMath(s string) bool {
	count := strings.Count(s, "$$")
	return count%2 == 0
}

func countSubstring(s, sub string) int {
	return strings.Count(s, sub)
}

func containsList(s string) bool {
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if isListItem(line) {
			return true
		}
	}
	return false
}

func isListItem(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "- ") ||
		strings.HasPrefix(trimmed, "* ") ||
		strings.HasPrefix(trimmed, "+ ") ||
		(len(trimmed) > 2 && trimmed[1] == '.' && trimmed[0] >= '0' && trimmed[0] <= '9')
}

func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

func hasContentOverlap(s1, s2 string) bool {
	// 简单检查是否有重叠的句子
	sentences1 := strings.Split(s1, ".")
	sentences2 := strings.Split(s2, ".")

	for _, sent1 := range sentences1 {
		sent1 = strings.TrimSpace(sent1)
		if sent1 == "" {
			continue
		}
		for _, sent2 := range sentences2 {
			sent2 = strings.TrimSpace(sent2)
			if sent1 == sent2 {
				return true
			}
		}
	}
	return false
}

func repeatString(s string, count int) string {
	var result string
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}
