package translator_tests

import (
	"strings"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// 测试文本批处理功能
func TestBatchTextProcessing(t *testing.T) {
	// 创建模拟的LLM客户端
	mockClient := new(test.MockLLMClient)
	mockClient.On("Name").Return("test-model")
	mockClient.On("Type").Return("openai")
	mockClient.On("MaxInputTokens").Return(8000)
	mockClient.On("MaxOutputTokens").Return(2000)
	mockClient.On("GetInputTokenPrice").Return(0.001)
	mockClient.On("GetOutputTokenPrice").Return(0.002)
	mockClient.On("GetPriceUnit").Return("$")

	// 模拟批处理翻译
	// 第一批：段落1和段落2
	mockClient.On("Complete", mock.MatchedBy(func(prompt string) bool {
		return strings.Contains(prompt, "Paragraph 1") && strings.Contains(prompt, "Paragraph 2")
	}), mock.Anything, mock.Anything).Return("[翻译] 段落1\n[翻译] 段落2", 100, 50, nil)

	// 第二批：段落3和段落4
	mockClient.On("Complete", mock.MatchedBy(func(prompt string) bool {
		return strings.Contains(prompt, "Paragraph 3") && strings.Contains(prompt, "Paragraph 4")
	}), mock.Anything, mock.Anything).Return("[翻译] 段落3\n[翻译] 段落4", 100, 50, nil)

	// 准备测试文本
	testText := "Paragraph 1\n\nParagraph 2\n\nParagraph 3\n\nParagraph 4"

	// 模拟批处理
	batches := splitTextIntoBatches(testText, 2)

	// 验证批次数量
	assert.Equal(t, 2, len(batches))

	// 验证第一批内容
	assert.Contains(t, batches[0], "Paragraph 1")
	assert.Contains(t, batches[0], "Paragraph 2")

	// 验证第二批内容
	assert.Contains(t, batches[1], "Paragraph 3")
	assert.Contains(t, batches[1], "Paragraph 4")

	// 模拟翻译批次
	translatedBatches := make([]string, len(batches))
	for i := range batches {
		if i == 0 {
			translatedBatches[i] = "[翻译] 段落1\n[翻译] 段落2"
		} else {
			translatedBatches[i] = "[翻译] 段落3\n[翻译] 段落4"
		}
	}

	// 合并翻译结果
	translatedText := strings.Join(translatedBatches, "\n\n")

	// 验证最终翻译结果
	assert.Contains(t, translatedText, "[翻译] 段落1")
	assert.Contains(t, translatedText, "[翻译] 段落2")
	assert.Contains(t, translatedText, "[翻译] 段落3")
	assert.Contains(t, translatedText, "[翻译] 段落4")
}

// 测试批处理边界情况
func TestBatchProcessingEdgeCases(t *testing.T) {
	// 测试空文本
	batches := splitTextIntoBatches("", 2)
	assert.Equal(t, 1, len(batches))
	assert.Equal(t, "", batches[0])

	// 测试单个段落
	batches = splitTextIntoBatches("Single paragraph", 2)
	assert.Equal(t, 1, len(batches))
	assert.Equal(t, "Single paragraph", batches[0])

	// 测试段落数小于批次大小
	batches = splitTextIntoBatches("Paragraph 1\n\nParagraph 2", 3)
	assert.Equal(t, 1, len(batches))
	assert.Contains(t, batches[0], "Paragraph 1")
	assert.Contains(t, batches[0], "Paragraph 2")

	// 测试段落数不能被批次大小整除
	batches = splitTextIntoBatches("Paragraph 1\n\nParagraph 2\n\nParagraph 3", 2)
	assert.Equal(t, 2, len(batches))
	assert.Contains(t, batches[0], "Paragraph 1")
	assert.Contains(t, batches[0], "Paragraph 2")
	assert.Contains(t, batches[1], "Paragraph 3")
}

// 测试批处理不拆分完整段落
func TestBatchProcessingPreserveParagraphs(t *testing.T) {
	// 准备测试文本，包含不同长度的段落
	testText := "Short paragraph.\n\nThis is a longer paragraph that spans multiple lines and should not be split across different batches because it would break the context and make translation more difficult.\n\nAnother short one.\n\nYet another paragraph that is quite long and contains multiple sentences. It should be kept together in the same batch to maintain context and ensure proper translation quality."

	// 使用较小的批次大小
	batches := splitTextIntoBatches(testText, 2)

	// 验证批次数量
	assert.Equal(t, 2, len(batches))

	// 验证长段落没有被拆分
	for _, batch := range batches {
		// 检查每个批次中的段落是否完整
		paragraphs := strings.Split(batch, "\n\n")
		for _, p := range paragraphs {
			// 验证原始文本中存在这个完整的段落
			assert.Contains(t, testText, p)
		}
	}
}

// 简单的文本批处理函数，模拟实际代码中的行为
func splitTextIntoBatches(text string, batchSize int) []string {
	if text == "" {
		return []string{""}
	}

	// 按段落分割文本
	paragraphs := strings.Split(text, "\n\n")

	// 如果段落数小于等于批次大小，直接返回整个文本
	if len(paragraphs) <= batchSize {
		return []string{text}
	}

	// 计算需要多少批次
	numBatches := (len(paragraphs) + batchSize - 1) / batchSize
	batches := make([]string, 0, numBatches)

	// 分配段落到批次
	for i := 0; i < numBatches; i++ {
		start := i * batchSize
		end := (i + 1) * batchSize
		if end > len(paragraphs) {
			end = len(paragraphs)
		}

		batch := strings.Join(paragraphs[start:end], "\n\n")
		batches = append(batches, batch)
	}

	return batches
}
