package base

import (
	"fmt"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/pkg/document"
)

// SimpleChunker 简单的文本分块器
type SimpleChunker struct{}

// NewSimpleChunker 创建简单分块器
func NewSimpleChunker() *SimpleChunker {
	return &SimpleChunker{}
}

// Chunk 将内容分块
func (c *SimpleChunker) Chunk(content string, opts document.ChunkOptions) []document.Chunk {
	if opts.MaxSize <= 0 {
		// 不分块，返回整个内容
		return []document.Chunk{
			{
				ID:      "0",
				Content: content,
				Start:   0,
				End:     len(content),
			},
		}
	}

	chunks := make([]document.Chunk, 0)
	lines := strings.Split(content, "\n")
	
	currentChunk := strings.Builder{}
	currentStart := 0
	currentPos := 0
	chunkID := 0

	for i, line := range lines {
		lineLen := len(line) + 1 // +1 for newline
		
		// 检查是否会超过最大大小
		if currentChunk.Len() > 0 && currentChunk.Len()+lineLen > opts.MaxSize {
			// 创建新块
			chunks = append(chunks, document.Chunk{
				ID:      fmt.Sprintf("%d", chunkID),
				Content: strings.TrimSpace(currentChunk.String()),
				Start:   currentStart,
				End:     currentPos,
			})
			
			chunkID++
			currentChunk.Reset()
			
			// 处理重叠
			if opts.Overlap > 0 && i > 0 {
				// 从前面的行中取一些作为重叠
				overlapStart := i - 1
				for j := overlapStart; j >= 0 && j >= overlapStart-2; j-- {
					currentChunk.WriteString(lines[j])
					currentChunk.WriteString("\n")
				}
			}
			
			currentStart = currentPos
		}
		
		currentChunk.WriteString(line)
		if i < len(lines)-1 {
			currentChunk.WriteString("\n")
		}
		currentPos += lineLen
	}

	// 添加最后一块
	if currentChunk.Len() > 0 {
		chunks = append(chunks, document.Chunk{
			ID:      fmt.Sprintf("%d", chunkID),
			Content: strings.TrimSpace(currentChunk.String()),
			Start:   currentStart,
			End:     currentPos,
		})
	}

	return chunks
}

// Merge 合并分块结果
func (c *SimpleChunker) Merge(chunks []document.TranslatedChunk) string {
	if len(chunks) == 0 {
		return ""
	}

	var result strings.Builder
	
	for i, chunk := range chunks {
		result.WriteString(chunk.TranslatedContent)
		if i < len(chunks)-1 {
			result.WriteString("\n\n")
		}
	}
	
	return result.String()
}

// SmartChunker 智能文本分块器，保持段落和句子完整性
type SmartChunker struct {
	sentenceDelimiters []string
}

// NewSmartChunker 创建智能分块器
func NewSmartChunker() *SmartChunker {
	return &SmartChunker{
		sentenceDelimiters: []string{"。", "！", "？", ".", "!", "?"},
	}
}

// Chunk 智能分块，保持段落和句子完整性
func (c *SmartChunker) Chunk(content string, opts document.ChunkOptions) []document.Chunk {
	if opts.MaxSize <= 0 {
		return []document.Chunk{
			{
				ID:      "0",
				Content: content,
				Start:   0,
				End:     len(content),
			},
		}
	}

	chunks := make([]document.Chunk, 0)
	paragraphs := strings.Split(content, "\n\n")
	
	currentChunk := strings.Builder{}
	currentStart := 0
	currentPos := 0
	chunkID := 0

	for _, para := range paragraphs {
		paraLen := len(para) + 2 // +2 for double newline
		
		// 如果段落本身就超过最大大小，需要进一步分割
		if len(para) > opts.MaxSize {
			// 按句子分割
			sentences := c.splitSentences(para)
			for _, sentence := range sentences {
				if currentChunk.Len() > 0 && currentChunk.Len()+len(sentence) > opts.MaxSize {
					// 创建新块
					chunks = append(chunks, document.Chunk{
						ID:      fmt.Sprintf("%d", chunkID),
						Content: strings.TrimSpace(currentChunk.String()),
						Start:   currentStart,
						End:     currentPos,
					})
					
					chunkID++
					currentChunk.Reset()
					currentStart = currentPos
				}
				
				currentChunk.WriteString(sentence)
				currentPos += len(sentence)
			}
		} else {
			// 检查是否会超过最大大小
			if currentChunk.Len() > 0 && currentChunk.Len()+paraLen > opts.MaxSize {
				// 创建新块
				chunks = append(chunks, document.Chunk{
					ID:      fmt.Sprintf("%d", chunkID),
					Content: strings.TrimSpace(currentChunk.String()),
					Start:   currentStart,
					End:     currentPos,
				})
				
				chunkID++
				currentChunk.Reset()
				currentStart = currentPos
			}
			
			if currentChunk.Len() > 0 {
				currentChunk.WriteString("\n\n")
			}
			currentChunk.WriteString(para)
			currentPos += paraLen
		}
	}

	// 添加最后一块
	if currentChunk.Len() > 0 {
		chunks = append(chunks, document.Chunk{
			ID:      fmt.Sprintf("%d", chunkID),
			Content: strings.TrimSpace(currentChunk.String()),
			Start:   currentStart,
			End:     currentPos,
		})
	}

	return chunks
}

// splitSentences 分割句子
func (c *SmartChunker) splitSentences(text string) []string {
	sentences := make([]string, 0)
	current := strings.Builder{}
	
	runes := []rune(text)
	for i, r := range runes {
		current.WriteRune(r)
		
		// 检查是否是句子结束符
		for _, delimiter := range c.sentenceDelimiters {
			if strings.HasSuffix(current.String(), delimiter) {
				// 检查下一个字符是否是空格或结束
				if i == len(runes)-1 || runes[i+1] == ' ' || runes[i+1] == '\n' {
					sentences = append(sentences, current.String())
					current.Reset()
					break
				}
			}
		}
	}
	
	// 添加剩余内容
	if current.Len() > 0 {
		sentences = append(sentences, current.String())
	}
	
	return sentences
}

// Merge 合并分块结果
func (c *SmartChunker) Merge(chunks []document.TranslatedChunk) string {
	if len(chunks) == 0 {
		return ""
	}

	var result strings.Builder
	
	for i, chunk := range chunks {
		result.WriteString(chunk.TranslatedContent)
		if i < len(chunks)-1 && !strings.HasSuffix(chunk.TranslatedContent, "\n\n") {
			result.WriteString("\n\n")
		}
	}
	
	return result.String()
}