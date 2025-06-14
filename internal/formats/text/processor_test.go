package text

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextProcessor(t *testing.T) {
	// 创建处理器
	processor, err := NewProcessor(document.ProcessorOptions{
		ChunkSize:    500,
		ChunkOverlap: 50,
	})
	require.NoError(t, err)
	require.NotNil(t, processor)

	// 测试文本内容
	text := `INTRODUCTION

This is the first paragraph of our document. It contains some important information that needs to be translated accurately.

Chapter 1: Getting Started

This chapter covers the basics of the system. We'll explore the fundamental concepts and provide practical examples.

1. First, we need to understand the core principles.
2. Second, we'll look at real-world applications.
3. Finally, we'll practice with exercises.

Key Points:
- Always start with the basics
- Practice makes perfect
- Don't skip the fundamentals

CONCLUSION

In this document, we've covered the essential topics. Remember to review the material regularly and apply what you've learned in practice.`

	ctx := context.Background()

	// 测试解析
	t.Run("Parse", func(t *testing.T) {
		reader := strings.NewReader(text)
		doc, err := processor.Parse(ctx, reader)
		require.NoError(t, err)
		require.NotNil(t, doc)

		// 验证文档格式
		assert.Equal(t, document.FormatText, doc.Format)
		assert.NotEmpty(t, doc.Blocks)

		// 验证块类型
		blockTypes := make(map[document.BlockType]int)
		for _, block := range doc.Blocks {
			blockTypes[block.GetType()]++
		}

		assert.Greater(t, blockTypes[document.BlockTypeHeading], 0)
		assert.Greater(t, blockTypes[document.BlockTypeParagraph], 0)
		assert.Greater(t, blockTypes[document.BlockTypeList], 0)
	})

	// 测试处理（翻译）
	t.Run("Process", func(t *testing.T) {
		reader := strings.NewReader(text)
		doc, err := processor.Parse(ctx, reader)
		require.NoError(t, err)

		// 模拟翻译函数
		translateFunc := func(ctx context.Context, text string) (string, error) {
			// 简单的模拟翻译：将特定词汇替换
			translated := strings.ReplaceAll(text, "document", "文档")
			translated = strings.ReplaceAll(translated, "chapter", "章节")
			translated = strings.ReplaceAll(translated, "Chapter", "章节")
			return translated, nil
		}

		processedDoc, err := processor.Process(ctx, doc, translateFunc)
		require.NoError(t, err)
		require.NotNil(t, processedDoc)

		// 验证翻译生效
		hasTranslated := false
		for _, block := range processedDoc.Blocks {
			content := block.GetContent()
			if strings.Contains(content, "文档") || strings.Contains(content, "章节") {
				hasTranslated = true
				break
			}
		}
		assert.True(t, hasTranslated)
	})

	// 测试渲染
	t.Run("Render", func(t *testing.T) {
		reader := strings.NewReader(text)
		doc, err := processor.Parse(ctx, reader)
		require.NoError(t, err)

		var output bytes.Buffer
		err = processor.Render(ctx, doc, &output)
		require.NoError(t, err)

		rendered := output.String()
		assert.NotEmpty(t, rendered)

		// 验证关键内容被保留
		assert.Contains(t, rendered, "INTRODUCTION")
		assert.Contains(t, rendered, "Chapter 1")
		assert.Contains(t, rendered, "Key Points:")
		assert.Contains(t, rendered, "CONCLUSION")
	})

	// 测试端到端流程
	t.Run("EndToEnd", func(t *testing.T) {
		reader := strings.NewReader(text)
		doc, err := processor.Parse(ctx, reader)
		require.NoError(t, err)

		// 翻译
		translateFunc := func(ctx context.Context, text string) (string, error) {
			// 模拟翻译
			result := strings.ReplaceAll(text, "Introduction", "介绍")
			result = strings.ReplaceAll(result, "INTRODUCTION", "介绍")
			result = strings.ReplaceAll(result, "Chapter", "章节")
			result = strings.ReplaceAll(result, "Conclusion", "结论")
			result = strings.ReplaceAll(result, "CONCLUSION", "结论")
			return result, nil
		}

		processedDoc, err := processor.Process(ctx, doc, translateFunc)
		require.NoError(t, err)

		// 渲染
		var output bytes.Buffer
		err = processor.Render(ctx, processedDoc, &output)
		require.NoError(t, err)

		result := output.String()
		assert.Contains(t, result, "介绍")
		assert.Contains(t, result, "章节")
		assert.Contains(t, result, "结论")
	})
}

func TestTextParser(t *testing.T) {
	parser := NewParser()
	ctx := context.Background()

	t.Run("SimpleText", func(t *testing.T) {
		text := `Title of Document

This is a paragraph.

Another paragraph here.`

		reader := strings.NewReader(text)
		doc, err := parser.Parse(ctx, reader)
		require.NoError(t, err)
		require.NotNil(t, doc)

		assert.GreaterOrEqual(t, len(doc.Blocks), 3)
	})

	t.Run("TextWithLists", func(t *testing.T) {
		text := `Shopping List

- Apples
- Bananas
- Oranges

Tasks:
1. Complete the report
2. Review the code
3. Submit the changes`

		reader := strings.NewReader(text)
		doc, err := parser.Parse(ctx, reader)
		require.NoError(t, err)

		// 验证列表被识别
		hasUnorderedList := false
		hasOrderedList := false
		for _, block := range doc.Blocks {
			if block.GetType() == document.BlockTypeList {
				content := block.GetContent()
				if strings.Contains(content, "- ") {
					hasUnorderedList = true
				}
				if strings.Contains(content, "1.") {
					hasOrderedList = true
				}
			}
		}
		assert.True(t, hasUnorderedList)
		assert.True(t, hasOrderedList)
	})

	t.Run("AllCapsHeadings", func(t *testing.T) {
		text := `CHAPTER ONE

Content of chapter one.

SECTION 1.1

Details of the section.`

		reader := strings.NewReader(text)
		doc, err := parser.Parse(ctx, reader)
		require.NoError(t, err)

		// 验证全大写标题被识别
		headingCount := 0
		for _, block := range doc.Blocks {
			if block.GetType() == document.BlockTypeHeading {
				headingCount++
			}
		}
		assert.GreaterOrEqual(t, headingCount, 2)
	})

	t.Run("LongTextWithoutParagraphs", func(t *testing.T) {
		// 测试没有明显段落分隔的长文本
		sentences := []string{
			"This is the first sentence.",
			"This is the second sentence.",
			"This is the third sentence.",
			"This is the fourth sentence.",
			"This is the fifth sentence.",
		}

		// 创建一个很长的单行文本
		longText := strings.Join(sentences, " ")
		// 重复多次使其超过分割阈值
		longText = strings.Repeat(longText+" ", 10)

		reader := strings.NewReader(longText)
		doc, err := parser.Parse(ctx, reader)
		require.NoError(t, err)

		// 应该至少有一个段落块
		assert.NotEmpty(t, doc.Blocks)
		assert.Equal(t, document.BlockTypeParagraph, doc.Blocks[0].GetType())
	})
}

func TestTextChunking(t *testing.T) {
	processor, err := NewProcessor(document.ProcessorOptions{
		ChunkSize:    200, // 小块大小用于测试
		ChunkOverlap: 20,
	})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("ChunkingLongParagraph", func(t *testing.T) {
		// 创建一个超长段落
		longParagraph := strings.Repeat("This is a sentence that will be repeated many times. ", 50)

		reader := strings.NewReader(longParagraph)
		doc, err := processor.Parse(ctx, reader)
		require.NoError(t, err)

		// 处理文档（这会触发分块）
		translateFunc := func(ctx context.Context, text string) (string, error) {
			return "[CHUNK] " + text, nil
		}

		processedDoc, err := processor.Process(ctx, doc, translateFunc)
		require.NoError(t, err)

		// 渲染并检查结果
		var output bytes.Buffer
		err = processor.Render(ctx, processedDoc, &output)
		require.NoError(t, err)

		result := output.String()
		// 应该有多个块被处理
		chunkCount := strings.Count(result, "[CHUNK]")
		assert.Greater(t, chunkCount, 1, "Long text should be split into multiple chunks")
	})
}
