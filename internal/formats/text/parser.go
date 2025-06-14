package text

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
)

// Parser 纯文本解析器
type Parser struct{}

// NewParser 创建纯文本解析器
func NewParser() *Parser {
	return &Parser{}
}

// Parse 解析纯文本内容为文档结构
func (p *Parser) Parse(ctx context.Context, input io.Reader) (*document.Document, error) {
	// 读取内容
	content, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	text := string(content)

	// 创建文档
	doc := &document.Document{
		ID:        generateDocumentID(),
		Format:    document.FormatText,
		Metadata:  document.DocumentMetadata{},
		Blocks:    []document.Block{},
		Resources: make(map[string]document.Resource),
	}

	// 解析文档为块
	blocks := p.parseToBlocks(text)
	doc.Blocks = blocks

	return doc, nil
}

// CanParse 检查是否能解析该格式
func (p *Parser) CanParse(format document.Format) bool {
	return format == document.FormatText
}

// parseToBlocks 将纯文本解析为块
func (p *Parser) parseToBlocks(text string) []document.Block {
	blocks := make([]document.Block, 0)

	// 对于纯文本，我们按段落分割
	// 段落由两个或更多换行符分隔
	paragraphs := splitParagraphs(text)

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// 检查是否像是标题（例如全大写或特定格式）
		if looksLikeHeading(para) {
			blocks = append(blocks, &document.BaseBlock{
				Type:         document.BlockTypeHeading,
				Content:      para,
				Translatable: true,
				Metadata: document.BlockMetadata{
					Level: guessHeadingLevel(para),
				},
			})
			continue
		}

		// 检查是否像是列表
		if looksLikeList(para) {
			blocks = append(blocks, &document.BaseBlock{
				Type:         document.BlockTypeList,
				Content:      para,
				Translatable: true,
				Metadata: document.BlockMetadata{
					ListType: "unordered",
				},
			})
			continue
		}

		// 默认作为段落处理
		blocks = append(blocks, &document.BaseBlock{
			Type:         document.BlockTypeParagraph,
			Content:      para,
			Translatable: true,
		})
	}

	return blocks
}

// splitParagraphs 分割段落
func splitParagraphs(text string) []string {
	// 标准化换行符
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	// 使用双换行作为段落分隔符
	paragraphs := strings.Split(text, "\n\n")

	// 如果没有双换行，尝试其他启发式方法
	if len(paragraphs) == 1 && len(text) > 500 {
		// 对于长文本，尝试按句子结束后的换行分割
		lines := strings.Split(text, "\n")
		var result []string
		var current strings.Builder

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				if current.Len() > 0 {
					result = append(result, current.String())
					current.Reset()
				}
				continue
			}

			if current.Len() > 0 {
				current.WriteString(" ")
			}
			current.WriteString(line)

			// 如果行以句号、问号或感叹号结束，可能是段落结束
			if endsWithSentence(line) && current.Len() > 100 {
				result = append(result, current.String())
				current.Reset()
			}
		}

		if current.Len() > 0 {
			result = append(result, current.String())
		}

		return result
	}

	return paragraphs
}

// looksLikeHeading 检查是否像标题
func looksLikeHeading(text string) bool {
	// 短文本
	if len(text) < 100 && !strings.Contains(text, ".") {
		// 全大写
		if text == strings.ToUpper(text) && len(text) > 3 {
			return true
		}

		// 以数字开头（如 "1. Introduction"）
		if len(text) > 0 && text[0] >= '0' && text[0] <= '9' {
			return true
		}

		// 首字母大写的短语
		words := strings.Fields(text)
		if len(words) <= 5 {
			allCapitalized := true
			for _, word := range words {
				if len(word) > 0 && word[0] >= 'a' && word[0] <= 'z' {
					allCapitalized = false
					break
				}
			}
			if allCapitalized {
				return true
			}
		}
	}

	return false
}

// guessHeadingLevel 猜测标题级别
func guessHeadingLevel(text string) int {
	// 基于一些启发式规则
	if text == strings.ToUpper(text) {
		return 1 // 全大写可能是主标题
	}

	if strings.HasPrefix(text, "Chapter") || strings.HasPrefix(text, "CHAPTER") {
		return 1
	}

	if len(text) < 30 {
		return 2 // 短标题可能是二级标题
	}

	return 3 // 默认三级标题
}

// looksLikeList 检查是否像列表
func looksLikeList(text string) bool {
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return false
	}

	// 检查是否多行都以相似的模式开始
	listCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 检查常见的列表标记
		if strings.HasPrefix(line, "- ") ||
			strings.HasPrefix(line, "* ") ||
			strings.HasPrefix(line, "• ") ||
			strings.HasPrefix(line, "+ ") ||
			startsWithNumber(line) {
			listCount++
		}
	}

	return listCount >= 2
}

// startsWithNumber 检查是否以数字开头（如 "1. item"）
func startsWithNumber(text string) bool {
	if len(text) < 3 {
		return false
	}

	// 检查模式: 数字 + . 或 ) + 空格
	for i, r := range text {
		if i == 0 && r >= '0' && r <= '9' {
			continue
		}
		if i > 0 && i < 3 && (r == '.' || r == ')') {
			if i+1 < len(text) && text[i+1] == ' ' {
				return true
			}
		}
		if i > 2 {
			break
		}
	}

	return false
}

// endsWithSentence 检查是否以句子结束
func endsWithSentence(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}

	runes := []rune(text)
	lastRune := runes[len(runes)-1]
	return lastRune == '.' || lastRune == '!' || lastRune == '?' ||
		lastRune == '。' || lastRune == '！' || lastRune == '？'
}

// generateDocumentID 生成文档ID
func generateDocumentID() string {
	return fmt.Sprintf("txt_%d", time.Now().UnixNano())
}
