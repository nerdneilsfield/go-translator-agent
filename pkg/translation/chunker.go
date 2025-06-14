package translation

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// defaultChunker 默认文本分块器实现
type defaultChunker struct {
	config ChunkConfig
}

// NewDefaultChunker 创建默认分块器
func NewDefaultChunker(size, overlap int) Chunker {
	if size <= 0 {
		size = 1000
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size / 10 // 重叠不能超过块大小的10%
	}

	return &defaultChunker{
		config: ChunkConfig{
			Size:    size,
			Overlap: overlap,
		},
	}
}

// Chunk 将文本分块
func (c *defaultChunker) Chunk(text string) []string {
	if text == "" {
		return []string{}
	}

	// 如果文本小于块大小，直接返回
	if utf8.RuneCountInString(text) <= c.config.Size {
		return []string{text}
	}

	// 按段落分割
	paragraphs := splitParagraphs(text)

	// 将段落组合成块
	return c.combineParagraphsIntoChunks(paragraphs)
}

// GetConfig 获取分块配置
func (c *defaultChunker) GetConfig() ChunkConfig {
	return c.config
}

// splitParagraphs 按段落分割文本
func splitParagraphs(text string) []string {
	// 标准化换行符
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	// 按双换行符分割段落
	parts := strings.Split(text, "\n\n")

	paragraphs := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			paragraphs = append(paragraphs, part)
		}
	}

	// 如果没有段落分隔，尝试按句子分割
	if len(paragraphs) == 1 && utf8.RuneCountInString(paragraphs[0]) > 1000 {
		return splitSentences(paragraphs[0])
	}

	return paragraphs
}

// splitSentences 按句子分割文本
func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	runes := []rune(text)
	inQuote := false

	for i, r := range runes {
		current.WriteRune(r)

		// 处理引号
		if r == '"' || r == '\'' {
			inQuote = !inQuote
		}

		// 句子结束符
		if !inQuote && isSentenceEnd(r) {
			// 检查下一个字符是否是空格或段落结束
			if i+1 < len(runes) && (unicode.IsSpace(runes[i+1]) || i+1 == len(runes)-1) {
				sentence := strings.TrimSpace(current.String())
				if sentence != "" {
					sentences = append(sentences, sentence)
					current.Reset()
				}
			}
		}
	}

	// 添加剩余的文本
	if current.Len() > 0 {
		sentence := strings.TrimSpace(current.String())
		if sentence != "" {
			sentences = append(sentences, sentence)
		}
	}

	return sentences
}

// isSentenceEnd 判断是否是句子结束符
func isSentenceEnd(r rune) bool {
	return r == '.' || r == '!' || r == '?' || r == '。' || r == '！' || r == '？'
}

// combineParagraphsIntoChunks 将段落组合成块
func (c *defaultChunker) combineParagraphsIntoChunks(paragraphs []string) []string {
	if len(paragraphs) == 0 {
		return []string{}
	}

	var chunks []string
	var currentChunk strings.Builder
	currentSize := 0

	for i, para := range paragraphs {
		paraSize := utf8.RuneCountInString(para)

		// 如果单个段落超过块大小，需要分割
		if paraSize > c.config.Size {
			// 先保存当前块
			if currentChunk.Len() > 0 {
				chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
				currentChunk.Reset()
				currentSize = 0
			}

			// 分割大段落
			subChunks := c.splitLargeParagraph(para)
			chunks = append(chunks, subChunks...)
			continue
		}

		// 如果加入这个段落会超过块大小
		if currentSize > 0 && currentSize+paraSize+1 > c.config.Size {
			// 保存当前块
			chunks = append(chunks, strings.TrimSpace(currentChunk.String()))

			// 处理重叠
			if c.config.Overlap > 0 && i > 0 {
				// 从当前块的末尾获取重叠部分
				overlapText := c.getOverlapText(currentChunk.String())
				currentChunk.Reset()
				currentChunk.WriteString(overlapText)
				if overlapText != "" {
					currentChunk.WriteString("\n\n")
				}
				currentSize = utf8.RuneCountInString(overlapText)
			} else {
				currentChunk.Reset()
				currentSize = 0
			}
		}

		// 添加段落到当前块
		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
			currentSize += 2
		}
		currentChunk.WriteString(para)
		currentSize += paraSize
	}

	// 添加最后一个块
	if currentChunk.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
	}

	return chunks
}

// splitLargeParagraph 分割大段落
func (c *defaultChunker) splitLargeParagraph(para string) []string {
	var chunks []string
	sentences := splitSentences(para)

	var currentChunk strings.Builder
	currentSize := 0

	for _, sentence := range sentences {
		sentenceSize := utf8.RuneCountInString(sentence)

		// 如果单个句子就超过块大小，强制分割
		if sentenceSize > c.config.Size {
			if currentChunk.Len() > 0 {
				chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
				currentChunk.Reset()
				currentSize = 0
			}

			// 按字符强制分割
			forcedChunks := c.forceChunk(sentence)
			chunks = append(chunks, forcedChunks...)
			continue
		}

		// 如果加入这个句子会超过块大小
		if currentSize > 0 && currentSize+sentenceSize+1 > c.config.Size {
			chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
			currentChunk.Reset()
			currentSize = 0
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString(" ")
			currentSize++
		}
		currentChunk.WriteString(sentence)
		currentSize += sentenceSize
	}

	if currentChunk.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
	}

	return chunks
}

// getOverlapText 获取重叠文本
func (c *defaultChunker) getOverlapText(text string) string {
	runes := []rune(text)
	if len(runes) <= c.config.Overlap {
		return text
	}

	// 从末尾获取重叠大小的文本
	overlapStart := len(runes) - c.config.Overlap
	overlapText := string(runes[overlapStart:])

	// 尝试在单词边界处开始
	words := strings.Fields(overlapText)
	if len(words) > 1 {
		// 移除第一个可能不完整的单词
		overlapText = strings.Join(words[1:], " ")
	}

	return overlapText
}

// forceChunk 强制按字符分块
func (c *defaultChunker) forceChunk(text string) []string {
	var chunks []string
	runes := []rune(text)

	for i := 0; i < len(runes); i += c.config.Size {
		end := i + c.config.Size
		if end > len(runes) {
			end = len(runes)
		}

		chunk := string(runes[i:end])
		chunks = append(chunks, chunk)
	}

	return chunks
}

// SmartChunker 智能分块器，保持语义完整性
type smartChunker struct {
	defaultChunker
	preserveCodeBlocks bool
	preserveLists      bool
}

// NewSmartChunker 创建智能分块器
func NewSmartChunker(size, overlap int) Chunker {
	return &smartChunker{
		defaultChunker: defaultChunker{
			config: ChunkConfig{
				Size:    size,
				Overlap: overlap,
			},
		},
		preserveCodeBlocks: true,
		preserveLists:      true,
	}
}

// Chunk 智能分块，保持代码块和列表的完整性
func (sc *smartChunker) Chunk(text string) []string {
	// 识别并保护特殊结构
	blocks := sc.identifyBlocks(text)

	// 对每个块进行分块
	var allChunks []string
	for _, block := range blocks {
		if block.Type == "code" || (block.Type == "list" && sc.preserveLists) {
			// 特殊块尽量保持完整
			if utf8.RuneCountInString(block.Content) <= sc.config.Size {
				allChunks = append(allChunks, block.Content)
			} else {
				// 如果太大，仍然需要分割，但保持块的标记
				chunks := sc.defaultChunker.Chunk(block.Content)
				allChunks = append(allChunks, chunks...)
			}
		} else {
			// 普通文本使用默认分块
			chunks := sc.defaultChunker.Chunk(block.Content)
			allChunks = append(allChunks, chunks...)
		}
	}

	return allChunks
}

// Block 文本块
type Block struct {
	Type    string // "text", "code", "list"
	Content string
}

// identifyBlocks 识别文本中的特殊块
func (sc *smartChunker) identifyBlocks(text string) []Block {
	var blocks []Block
	lines := strings.Split(text, "\n")

	var currentBlock Block
	var inCodeBlock bool
	var codeBlockDelimiter string

	for _, line := range lines {
		// 检查代码块开始/结束
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			if !inCodeBlock {
				// 代码块开始
				if currentBlock.Content != "" {
					currentBlock.Content = strings.TrimSpace(currentBlock.Content)
					blocks = append(blocks, currentBlock)
				}
				currentBlock = Block{Type: "code", Content: line + "\n"}
				inCodeBlock = true
				codeBlockDelimiter = line[:3]
			} else if strings.HasPrefix(line, codeBlockDelimiter) {
				// 代码块结束
				currentBlock.Content += line
				blocks = append(blocks, currentBlock)
				currentBlock = Block{Type: "text", Content: ""}
				inCodeBlock = false
			} else {
				currentBlock.Content += line + "\n"
			}
		} else if inCodeBlock {
			currentBlock.Content += line + "\n"
		} else {
			// 检查是否是列表项
			if sc.isListItem(line) && currentBlock.Type != "list" {
				if currentBlock.Content != "" {
					currentBlock.Content = strings.TrimSpace(currentBlock.Content)
					blocks = append(blocks, currentBlock)
				}
				currentBlock = Block{Type: "list", Content: line + "\n"}
			} else if !sc.isListItem(line) && currentBlock.Type == "list" {
				// 列表结束
				blocks = append(blocks, currentBlock)
				currentBlock = Block{Type: "text", Content: line + "\n"}
			} else {
				currentBlock.Content += line + "\n"
			}
		}
	}

	// 添加最后一个块
	if currentBlock.Content != "" {
		currentBlock.Content = strings.TrimSpace(currentBlock.Content)
		blocks = append(blocks, currentBlock)
	}

	return blocks
}

// isListItem 检查是否是列表项
func (sc *smartChunker) isListItem(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	// 检查无序列表
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
		return true
	}

	// 检查有序列表
	for i, r := range trimmed {
		if !unicode.IsDigit(r) {
			if i > 0 && (r == '.' || r == ')') && i < len(trimmed)-1 && trimmed[i+1] == ' ' {
				return true
			}
			break
		}
	}

	return false
}
