package markdown

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/pkg/document"
)

// Parser Markdown 解析器
type Parser struct {
	// 预编译的正则表达式
	codeBlockRegex  *regexp.Regexp
	multiMathRegex  *regexp.Regexp
	tableBlockRegex *regexp.Regexp
	inlineCodeRegex *regexp.Regexp
	inlineMathRegex *regexp.Regexp
	imageRegex      *regexp.Regexp
	linkRegex       *regexp.Regexp
	headingRegex    *regexp.Regexp
}

// NewParser 创建 Markdown 解析器
func NewParser() *Parser {
	return &Parser{
		codeBlockRegex:  regexp.MustCompile("(?s)```(.*?)```"),
		multiMathRegex:  regexp.MustCompile(`(?s)\$\$(.*?)\$\$`),
		tableBlockRegex: regexp.MustCompile(
			`(?m)^[ \t]*\|.*\|[ \t]*\r?\n` +
				`^[ \t]*\|[ :\-\.\|\t]+?\|[ \t]*\r?\n` +
				`^(?:[ \t]*\|.*\|[ \t]*\r?\n)+`,
		),
		inlineCodeRegex: regexp.MustCompile("`[^`]+`"),
		inlineMathRegex: regexp.MustCompile(`\$[^$]+\$`),
		imageRegex:      regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`),
		linkRegex:       regexp.MustCompile(`\[[^\]]*\]\([^)]*\)`),
		headingRegex:    regexp.MustCompile(`(?m)^(#{1,6})\s+(.*)$`),
	}
}

// Parse 解析 Markdown 内容为文档结构
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
		Format:    document.FormatMarkdown,
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
	return format == document.FormatMarkdown
}

// parseToBlocks 将 Markdown 文本解析为块
func (p *Parser) parseToBlocks(text string) []document.Block {
	blocks := make([]document.Block, 0)
	
	// 先处理代码块和数学块（保护它们不被翻译）
	protectedBlocks := make(map[string]document.Block)
	protectedText := text
	
	// 处理代码块
	codeBlocks := p.codeBlockRegex.FindAllStringSubmatchIndex(protectedText, -1)
	for i := len(codeBlocks) - 1; i >= 0; i-- {
		match := codeBlocks[i]
		start, end := match[0], match[1]
		content := protectedText[start:end]
		
		placeholder := fmt.Sprintf("@@CODE_BLOCK_%d@@", i)
		block := &document.BaseBlock{
			Type:         document.BlockTypeCode,
			Content:      content,
			Translatable: false,
			Metadata: document.BlockMetadata{
				Language: extractCodeLanguage(content),
			},
		}
		protectedBlocks[placeholder] = block
		protectedText = protectedText[:start] + placeholder + protectedText[end:]
	}
	
	// 处理数学块
	mathBlocks := p.multiMathRegex.FindAllStringSubmatchIndex(protectedText, -1)
	for i := len(mathBlocks) - 1; i >= 0; i-- {
		match := mathBlocks[i]
		start, end := match[0], match[1]
		content := protectedText[start:end]
		
		placeholder := fmt.Sprintf("@@MATH_BLOCK_%d@@", i)
		block := &document.BaseBlock{
			Type:         document.BlockTypeMath,
			Content:      content,
			Translatable: false,
		}
		protectedBlocks[placeholder] = block
		protectedText = protectedText[:start] + placeholder + protectedText[end:]
	}
	
	// 处理表格
	tables := p.tableBlockRegex.FindAllStringSubmatchIndex(protectedText, -1)
	for i := len(tables) - 1; i >= 0; i-- {
		match := tables[i]
		start, end := match[0], match[1]
		content := protectedText[start:end]
		
		placeholder := fmt.Sprintf("@@TABLE_BLOCK_%d@@", i)
		block := &document.BaseBlock{
			Type:         document.BlockTypeTable,
			Content:      content,
			Translatable: true, // 表格内容可以翻译
		}
		protectedBlocks[placeholder] = block
		protectedText = protectedText[:start] + placeholder + protectedText[end:]
	}
	
	// 按段落分割剩余内容
	paragraphs := strings.Split(protectedText, "\n\n")
	
	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		
		// 检查是否是占位符
		if block, ok := protectedBlocks[para]; ok {
			blocks = append(blocks, block)
			continue
		}
		
		// 检查是否是标题
		if headingMatch := p.headingRegex.FindStringSubmatch(para); headingMatch != nil {
			blocks = append(blocks, &document.BaseBlock{
				Type:         document.BlockTypeHeading,
				Content:      para,
				Translatable: true,
				Metadata: document.BlockMetadata{
					Level: len(headingMatch[1]), // # 的数量表示级别
				},
			})
			continue
		}
		
		// 检查是否是列表
		if isListBlock(para) {
			blocks = append(blocks, &document.BaseBlock{
				Type:         document.BlockTypeList,
				Content:      para,
				Translatable: true,
				Metadata: document.BlockMetadata{
					ListType: detectListType(para),
				},
			})
			continue
		}
		
		// 检查是否是引用块
		if strings.HasPrefix(para, ">") {
			blocks = append(blocks, &document.BaseBlock{
				Type:         document.BlockTypeQuote,
				Content:      para,
				Translatable: true,
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

// extractCodeLanguage 提取代码块的语言
func extractCodeLanguage(codeBlock string) string {
	lines := strings.Split(codeBlock, "\n")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "```") {
		lang := strings.TrimPrefix(lines[0], "```")
		return strings.TrimSpace(lang)
	}
	return ""
}

// isListBlock 检查是否是列表块
func isListBlock(text string) bool {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return false
	}
	
	// 检查是否所有行都是列表项
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		
		// 检查无序列表
		if strings.HasPrefix(trimmed, "- ") || 
		   strings.HasPrefix(trimmed, "* ") || 
		   strings.HasPrefix(trimmed, "+ ") {
			continue
		}
		
		// 检查有序列表
		if matched, _ := regexp.MatchString(`^\d+\.\s+`, trimmed); matched {
			continue
		}
		
		// 检查缩进的列表项
		if strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t") {
			continue
		}
		
		return false
	}
	
	return true
}

// detectListType 检测列表类型
func detectListType(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		
		if strings.HasPrefix(trimmed, "- ") || 
		   strings.HasPrefix(trimmed, "* ") || 
		   strings.HasPrefix(trimmed, "+ ") {
			return "unordered"
		}
		
		if matched, _ := regexp.MatchString(`^\d+\.\s+`, trimmed); matched {
			return "ordered"
		}
	}
	
	return "unordered"
}

// generateDocumentID 生成文档ID
func generateDocumentID() string {
	return fmt.Sprintf("md_%d", time.Now().UnixNano())
}