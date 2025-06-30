package document

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	pkgdoc "github.com/nerdneilsfield/go-translator-agent/pkg/document"
	"go.uber.org/zap"
)

// MarkdownProcessor Markdown文档处理器，使用 NodeInfo 系统
type MarkdownProcessor struct {
	opts           ProcessorOptions
	logger         *zap.Logger
	nodeTranslator *NodeInfoTranslator
	protector      pkgdoc.ContentProtector
}

// NewMarkdownProcessor 创建Markdown处理器
func NewMarkdownProcessor(opts ProcessorOptions, logger *zap.Logger) (*MarkdownProcessor, error) {
	// 设置默认值
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = 2000
	}
	if opts.ChunkOverlap < 0 {
		opts.ChunkOverlap = 100
	}

	// 创建节点翻译器
	contextDistance := 2
	maxRetries := 3
	nodeTranslator := NewNodeInfoTranslator(opts.ChunkSize, contextDistance, maxRetries)

	// 创建Markdown格式保护器
	protector := pkgdoc.GetProtectorForFormat("markdown")

	return &MarkdownProcessor{
		opts:           opts,
		logger:         logger,
		nodeTranslator: nodeTranslator,
		protector:      protector,
	}, nil
}

// Parse 解析Markdown输入
func (p *MarkdownProcessor) Parse(ctx context.Context, input io.Reader) (*Document, error) {
	// 读取所有内容
	content, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	text := string(content)

	// 修复转义的星号标记
	text = strings.ReplaceAll(text, `\*\*`, `**`)
	text = strings.ReplaceAll(text, `\*`, `*`)

	// 创建文档
	doc := &Document{
		ID:     fmt.Sprintf("markdown-%d", time.Now().Unix()),
		Format: FormatMarkdown,
		Metadata: DocumentMetadata{
			CreatedAt: time.Now(),
		},
		Blocks:    []Block{},
		Resources: make(map[string]Resource),
	}

	// 分割Markdown为块
	blocks := p.splitIntoBlocks(text)

	// 将块转换为文档块和节点
	nodeID := 1
	for i, block := range blocks {
		// 创建文档块
		docBlock := &BaseBlock{
			Type:         p.getBlockType(block.blockType),
			Content:      block.content,
			Translatable: block.translatable,
			Metadata: BlockMetadata{
				Attributes: map[string]interface{}{
					"markdownType": block.blockType,
					"level":        block.level,
					"language":     block.language,
				},
			},
		}
		doc.Blocks = append(doc.Blocks, docBlock)

		// 只为可翻译的块创建节点
		if block.translatable {
			// 提取可翻译的文本部分
			translatableText := p.extractTranslatableText(block)

			node := &NodeInfo{
				ID:           nodeID,
				BlockID:      fmt.Sprintf("block-%d", i),
				OriginalText: translatableText,
				Status:       NodeStatusPending,
				Path:         fmt.Sprintf("/block[%d]", i+1),
				Metadata: map[string]interface{}{
					"blockIndex":  i,
					"blockType":   block.blockType,
					"fullContent": block.content,
					"prefix":      block.prefix,
					"suffix":      block.suffix,
				},
			}
			p.nodeTranslator.collection.Add(node)
			nodeID++
		}
	}

	return doc, nil
}

// Process 处理文档
func (p *MarkdownProcessor) Process(ctx context.Context, doc *Document, translator TranslateFunc) (*Document, error) {
	startTime := time.Now()
	stats := ProcessingStatistics{}

	// 获取所有待翻译的节点
	allNodes := p.nodeTranslator.collection.GetAll()
	stats.TotalBlocks = len(doc.Blocks)
	stats.TranslatedBlocks = len(allNodes)

	// 第一轮翻译
	err := p.translateNodes(ctx, allNodes, translator, &stats)
	if err != nil {
		return nil, fmt.Errorf("initial translation failed: %w", err)
	}

	// 重试失败的节点
	for retry := 0; retry < 3; retry++ {
		retryGroups, err := p.nodeTranslator.retryManager.PrepareRetryGroups()
		if err != nil {
			p.logger.Warn("failed to prepare retry groups", zap.Error(err))
			break
		}

		if len(retryGroups) == 0 {
			break
		}

		p.logger.Info("retrying failed nodes",
			zap.Int("retry", retry+1),
			zap.Int("groups", len(retryGroups)))

		// 翻译重试组
		for _, group := range retryGroups {
			markedText := p.generateMarkedText(group.Nodes)
			translatedText, err := translator(ctx, markedText)
			if err != nil {
				for _, node := range group.Nodes {
					if node.Status == NodeStatusFailed {
						p.nodeTranslator.retryManager.MarkRetryCompleted(node.ID, false, "", err)
					}
				}
				continue
			}

			// 解析翻译结果
			p.parseMarkedText(translatedText, group.Nodes)

			// 标记重试完成
			for _, node := range group.Nodes {
				if node.Status == NodeStatusSuccess {
					p.nodeTranslator.retryManager.MarkRetryCompleted(node.ID, true, node.TranslatedText, nil)
				}
			}
		}

		// 重置已处理节点
		p.nodeTranslator.retryManager.ResetProcessedNodes()
	}

	// 更新文档块的翻译内容
	nodes := p.nodeTranslator.collection.GetAll()
	nodeMap := make(map[string]*NodeInfo)
	for _, node := range nodes {
		nodeMap[node.BlockID] = node
	}

	for i, block := range doc.Blocks {
		if !block.IsTranslatable() {
			stats.SkippedBlocks++
			continue
		}

		// 找到对应的节点
		blockID := fmt.Sprintf("block-%d", i)
		if node, exists := nodeMap[blockID]; exists && node.IsTranslated() {
			// 重建完整的块内容
			newContent := p.reconstructBlock(node)
			block.SetContent(newContent)
		}
	}

	stats.ProcessingTime = time.Since(startTime)

	p.logger.Info("Markdown processing completed",
		zap.Int("totalBlocks", stats.TotalBlocks),
		zap.Int("translatedBlocks", stats.TranslatedBlocks),
		zap.Int("skippedBlocks", stats.SkippedBlocks),
		zap.Duration("processingTime", stats.ProcessingTime))

	return doc, nil
}

// Render 渲染文档
func (p *MarkdownProcessor) Render(ctx context.Context, doc *Document, output io.Writer) error {
	var builder strings.Builder

	for i, block := range doc.Blocks {
		if i > 0 {
			// 根据块类型决定间隔
			prevBlock := doc.Blocks[i-1]
			if p.needsDoubleNewline(prevBlock.GetType(), block.GetType()) {
				builder.WriteString("\n\n")
			} else {
				builder.WriteString("\n")
			}
		}

		builder.WriteString(block.GetContent())
	}

	// 确保文件以换行符结束
	content := builder.String()
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	_, err := output.Write([]byte(content))
	return err
}

// GetFormat 返回支持的格式
func (p *MarkdownProcessor) GetFormat() Format {
	return FormatMarkdown
}

// Markdown块信息
type markdownBlock struct {
	blockType    string
	content      string
	translatable bool
	level        int    // 标题级别
	language     string // 代码块语言
	prefix       string // 前缀（如列表标记）
	suffix       string // 后缀
}

// splitIntoBlocks 分割Markdown为块
func (p *MarkdownProcessor) splitIntoBlocks(text string) []markdownBlock {
	var blocks []markdownBlock
	lines := strings.Split(text, "\n")

	i := 0
	for i < len(lines) {
		line := lines[i]

		// 代码块
		if strings.HasPrefix(line, "```") {
			block := p.parseCodeBlock(lines, &i)
			blocks = append(blocks, block)
			continue
		}

		// 标题
		if match := regexp.MustCompile(`^(#{1,6})\s+(.+)$`).FindStringSubmatch(line); match != nil {
			blocks = append(blocks, markdownBlock{
				blockType:    "heading",
				content:      line,
				translatable: true,
				level:        len(match[1]),
				prefix:       match[1] + " ",
				suffix:       "",
			})
			i++
			continue
		}

		// 列表项
		if match := regexp.MustCompile(`^(\s*[-*+]\s+|\s*\d+\.\s+)(.*)$`).FindStringSubmatch(line); match != nil {
			blocks = append(blocks, markdownBlock{
				blockType:    "list",
				content:      line,
				translatable: true,
				prefix:       match[1],
				suffix:       "",
			})
			i++
			continue
		}

		// 引用
		if strings.HasPrefix(strings.TrimSpace(line), ">") {
			block := p.parseQuoteBlock(lines, &i)
			blocks = append(blocks, block)
			continue
		}

		// 表格
		if p.isTableStart(lines, i) {
			block := p.parseTable(lines, &i)
			blocks = append(blocks, block)
			continue
		}

		// 检查是否是保护块的开始
		if strings.Contains(line, "<!-- REFERENCES_PROTECTED -->") || strings.Contains(line, "<!-- TABLE_PROTECTED -->") {
			block := p.parseProtectedBlock(lines, &i)
			blocks = append(blocks, block)
			continue
		}

		// 水平线
		if match := regexp.MustCompile(`^(\*{3,}|-{3,}|_{3,})$`).FindString(strings.TrimSpace(line)); match != "" {
			blocks = append(blocks, markdownBlock{
				blockType:    "hr",
				content:      line,
				translatable: false,
			})
			i++
			continue
		}

		// 空行
		if strings.TrimSpace(line) == "" {
			blocks = append(blocks, markdownBlock{
				blockType:    "empty",
				content:      line,
				translatable: false,
			})
			i++
			continue
		}

		// 段落
		block := p.parseParagraph(lines, &i)
		blocks = append(blocks, block)
	}

	return blocks
}

// parseCodeBlock 解析代码块
func (p *MarkdownProcessor) parseCodeBlock(lines []string, i *int) markdownBlock {
	startLine := lines[*i]
	language := strings.TrimPrefix(startLine, "```")
	language = strings.TrimSpace(language)

	var content strings.Builder
	content.WriteString(startLine)
	*i++

	for *i < len(lines) {
		content.WriteString("\n")
		content.WriteString(lines[*i])

		if strings.HasPrefix(lines[*i], "```") {
			*i++
			break
		}
		*i++
	}

	return markdownBlock{
		blockType:    "code",
		content:      content.String(),
		translatable: false,
		language:     language,
	}
}

// parseQuoteBlock 解析引用块
func (p *MarkdownProcessor) parseQuoteBlock(lines []string, i *int) markdownBlock {
	var content strings.Builder
	var translatableContent strings.Builder

	for *i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[*i]), ">") {
		if content.Len() > 0 {
			content.WriteString("\n")
		}
		content.WriteString(lines[*i])

		// 提取可翻译部分
		line := strings.TrimSpace(lines[*i])
		line = strings.TrimPrefix(line, ">")
		line = strings.TrimSpace(line)
		if line != "" {
			if translatableContent.Len() > 0 {
				translatableContent.WriteString("\n")
			}
			translatableContent.WriteString(line)
		}

		*i++
	}

	return markdownBlock{
		blockType:    "quote",
		content:      content.String(),
		translatable: true,
		prefix:       "> ",
	}
}

// parseTable 解析表格
func (p *MarkdownProcessor) parseTable(lines []string, i *int) markdownBlock {
	var content strings.Builder
	startIdx := *i

	// 收集表格行
	for *i < len(lines) && (strings.Contains(lines[*i], "|") || *i == startIdx+1) {
		if content.Len() > 0 {
			content.WriteString("\n")
		}
		content.WriteString(lines[*i])
		*i++

		// 如果遇到空行，表格结束
		if *i < len(lines) && strings.TrimSpace(lines[*i]) == "" {
			break
		}
	}

	return markdownBlock{
		blockType:    "table",
		content:      content.String(),
		translatable: true,
	}
}

// parseProtectedBlock 解析保护块
func (p *MarkdownProcessor) parseProtectedBlock(lines []string, i *int) markdownBlock {
	var content strings.Builder
	startMarker := ""
	endMarker := ""
	
	// 确定开始和结束标记
	if strings.Contains(lines[*i], "<!-- REFERENCES_PROTECTED -->") {
		startMarker = "<!-- REFERENCES_PROTECTED -->"
		endMarker = "<!-- /REFERENCES_PROTECTED -->"
	} else if strings.Contains(lines[*i], "<!-- TABLE_PROTECTED -->") {
		startMarker = "<!-- TABLE_PROTECTED -->"
		endMarker = "<!-- /TABLE_PROTECTED -->"
	}
	
	// 收集整个保护块
	foundEnd := false
	for *i < len(lines) {
		if content.Len() > 0 {
			content.WriteString("\n")
		}
		content.WriteString(lines[*i])
		
		if strings.Contains(lines[*i], endMarker) {
			foundEnd = true
			*i++
			break
		}
		*i++
	}
	
	// 如果没有找到结束标记，记录警告
	if !foundEnd && p.logger != nil {
		p.logger.Warn("protected block without end marker",
			zap.String("startMarker", startMarker),
			zap.String("content", content.String()))
	}
	
	return markdownBlock{
		blockType:    "protected",
		content:      content.String(),
		translatable: false, // 保护块不需要翻译
	}
}

// parseParagraph 解析段落
func (p *MarkdownProcessor) parseParagraph(lines []string, i *int) markdownBlock {
	var content strings.Builder

	for *i < len(lines) && strings.TrimSpace(lines[*i]) != "" {
		// 检查是否遇到其他块类型的开始
		line := lines[*i]
		if strings.HasPrefix(line, "```") ||
			regexp.MustCompile(`^#{1,6}\s+`).MatchString(line) ||
			regexp.MustCompile(`^(\s*[-*+]\s+|\s*\d+\.\s+)`).MatchString(line) ||
			strings.HasPrefix(strings.TrimSpace(line), ">") ||
			regexp.MustCompile(`^(\*{3,}|-{3,}|_{3,})$`).MatchString(strings.TrimSpace(line)) {
			break
		}

		if content.Len() > 0 {
			content.WriteString("\n")
		}
		content.WriteString(line)
		*i++
	}

	return markdownBlock{
		blockType:    "paragraph",
		content:      content.String(),
		translatable: true,
	}
}

// isTableStart 检查是否是表格开始
func (p *MarkdownProcessor) isTableStart(lines []string, i int) bool {
	if i >= len(lines)-1 {
		return false
	}

	// 检查当前行和下一行
	currentLine := lines[i]
	nextLine := lines[i+1]

	// 表格必须包含 |
	if !strings.Contains(currentLine, "|") {
		return false
	}

	// 下一行应该是分隔行
	if regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`).MatchString(nextLine) {
		return true
	}

	return false
}

// extractTranslatableText 提取可翻译的文本
func (p *MarkdownProcessor) extractTranslatableText(block markdownBlock) string {
	switch block.blockType {
	case "heading":
		// 移除标题标记
		text := regexp.MustCompile(`^#{1,6}\s+`).ReplaceAllString(block.content, "")
		return strings.TrimSpace(text)

	case "list":
		// 移除列表标记
		text := regexp.MustCompile(`^(\s*[-*+]\s+|\s*\d+\.\s+)`).ReplaceAllString(block.content, "")
		return strings.TrimSpace(text)

	case "quote":
		// 移除引用标记
		lines := strings.Split(block.content, "\n")
		var result []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			line = strings.TrimPrefix(line, ">")
			line = strings.TrimSpace(line)
			if line != "" {
				result = append(result, line)
			}
		}
		return strings.Join(result, "\n")

	case "table":
		// 表格需要特殊处理，保持结构
		return p.extractTableText(block.content)

	default:
		return block.content
	}
}

// extractTableText 提取表格文本
func (p *MarkdownProcessor) extractTableText(tableContent string) string {
	lines := strings.Split(tableContent, "\n")
	var result []string

	for i, line := range lines {
		// 跳过分隔行
		if i == 1 && regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`).MatchString(line) {
			continue
		}

		// 提取单元格内容
		cells := strings.Split(line, "|")
		var cleanCells []string
		for _, cell := range cells {
			cell = strings.TrimSpace(cell)
			if cell != "" {
				cleanCells = append(cleanCells, cell)
			}
		}

		if len(cleanCells) > 0 {
			result = append(result, strings.Join(cleanCells, " | "))
		}
	}

	return strings.Join(result, "\n")
}

// reconstructBlock 重建块内容
func (p *MarkdownProcessor) reconstructBlock(node *NodeInfo) string {
	blockType, _ := node.Metadata["blockType"].(string)
	fullContent, _ := node.Metadata["fullContent"].(string)
	prefix, _ := node.Metadata["prefix"].(string)
	suffix, _ := node.Metadata["suffix"].(string)

	switch blockType {
	case "heading", "list":
		return prefix + node.TranslatedText + suffix

	case "quote":
		// 重建引用块
		lines := strings.Split(node.TranslatedText, "\n")
		var result []string
		for _, line := range lines {
			if line != "" {
				result = append(result, "> "+line)
			}
		}
		return strings.Join(result, "\n")

	case "table":
		// 重建表格
		return p.reconstructTable(fullContent, node.TranslatedText)

	default:
		return node.TranslatedText
	}
}

// reconstructTable 重建表格
func (p *MarkdownProcessor) reconstructTable(originalTable, translatedText string) string {
	originalLines := strings.Split(originalTable, "\n")
	translatedLines := strings.Split(translatedText, "\n")

	var result []string
	translatedIdx := 0

	for i, line := range originalLines {
		// 保留分隔行
		if i == 1 && regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`).MatchString(line) {
			result = append(result, line)
			continue
		}

		// 替换单元格内容
		if translatedIdx < len(translatedLines) {
			cells := strings.Split(line, "|")
			translatedCells := strings.Split(translatedLines[translatedIdx], " | ")

			var newCells []string
			cellIdx := 0

			for j, cell := range cells {
				if j == 0 || j == len(cells)-1 {
					// 保留首尾的空单元格
					newCells = append(newCells, cell)
				} else if cellIdx < len(translatedCells) {
					// 替换为翻译内容，保持原有的空格
					leadingSpace := len(cell) - len(strings.TrimLeft(cell, " "))
					trailingSpace := len(cell) - len(strings.TrimRight(cell, " "))

					newCell := strings.Repeat(" ", leadingSpace) +
						translatedCells[cellIdx] +
						strings.Repeat(" ", trailingSpace)

					newCells = append(newCells, newCell)
					cellIdx++
				} else {
					newCells = append(newCells, cell)
				}
			}

			result = append(result, strings.Join(newCells, "|"))
			translatedIdx++
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// getBlockType 将Markdown块类型转换为文档块类型
func (p *MarkdownProcessor) getBlockType(markdownType string) BlockType {
	switch markdownType {
	case "heading":
		return BlockTypeHeading
	case "paragraph":
		return BlockTypeParagraph
	case "code":
		return BlockTypeCode
	case "list":
		return BlockTypeList
	case "table":
		return BlockTypeTable
	case "quote":
		return BlockTypeQuote
	default:
		return BlockTypeCustom
	}
}

// needsDoubleNewline 判断两个块之间是否需要双换行
func (p *MarkdownProcessor) needsDoubleNewline(prevType, currType BlockType) bool {
	// 空行块不需要额外换行
	if prevType == BlockTypeCustom || currType == BlockTypeCustom {
		return false
	}

	// 列表项之间不需要双换行
	if prevType == BlockTypeList && currType == BlockTypeList {
		return false
	}

	// 其他情况需要双换行
	return true
}

// translateNodes 翻译节点
func (p *MarkdownProcessor) translateNodes(ctx context.Context, nodes []*NodeInfo, translator TranslateFunc, stats *ProcessingStatistics) error {
	// 分组节点
	groups := p.nodeTranslator.grouper.GroupNodes(nodes)
	stats.TotalChunks = len(groups)

	p.logger.Info("translating Markdown nodes",
		zap.Int("totalNodes", len(nodes)),
		zap.Int("groups", len(groups)))

	// 逐组翻译
	for i, group := range groups {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 生成标记文本
		markedText := p.generateMarkedText(group.Nodes)
		stats.TotalCharacters += len(markedText)

		// 翻译
		p.logger.Debug("translating group",
			zap.Int("groupIndex", i),
			zap.Int("nodes", len(group.Nodes)),
			zap.Int("size", group.Size))

		translatedText, err := translator(ctx, markedText)
		if err != nil {
			// 标记组内所有节点为失败
			for _, node := range group.Nodes {
				p.nodeTranslator.collection.Update(node.ID, func(n *NodeInfo) {
					n.Status = NodeStatusFailed
					n.Error = err
				})
			}
			p.logger.Warn("failed to translate group",
				zap.Int("groupIndex", i),
				zap.Error(err))
			continue
		}

		// 解析翻译结果
		err = p.parseMarkedText(translatedText, group.Nodes)
		if err != nil {
			p.logger.Warn("failed to parse translation result",
				zap.Int("groupIndex", i),
				zap.Error(err))
		}
	}

	return nil
}

// generateMarkedText 生成标记文本
func (p *MarkdownProcessor) generateMarkedText(nodes []*NodeInfo) string {
	var builder strings.Builder

	for i, node := range nodes {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(fmt.Sprintf("@@NODE_START_%d@@\n%s\n@@NODE_END_%d@@",
			node.ID, node.OriginalText, node.ID))
	}

	return builder.String()
}

// parseMarkedText 解析标记文本
func (p *MarkdownProcessor) parseMarkedText(markedText string, nodes []*NodeInfo) error {
	// 创建 ID 到节点的映射
	nodeMap := make(map[int]*NodeInfo)
	for _, node := range nodes {
		nodeMap[node.ID] = node
	}

	// 解析标记文本
	lines := strings.Split(markedText, "\n")
	var currentNodeID int
	var textBuilder strings.Builder
	inNode := false

	for _, line := range lines {
		if strings.HasPrefix(line, "@@NODE_START_") && strings.HasSuffix(line, "@@") {
			fmt.Sscanf(line, "@@NODE_START_%d@@", &currentNodeID)
			inNode = true
			textBuilder.Reset()
		} else if strings.HasPrefix(line, "@@NODE_END_") && strings.HasSuffix(line, "@@") {
			if inNode {
				if node, exists := nodeMap[currentNodeID]; exists {
					translatedText := strings.TrimSpace(textBuilder.String())
					p.nodeTranslator.collection.Update(node.ID, func(n *NodeInfo) {
						n.TranslatedText = translatedText
						n.Status = NodeStatusSuccess
					})
				}
			}
			inNode = false
		} else if inNode {
			if textBuilder.Len() > 0 {
				textBuilder.WriteString("\n")
			}
			textBuilder.WriteString(line)
		}
	}

	// 标记未找到翻译的节点为失败
	for _, node := range nodes {
		if node.Status == NodeStatusPending {
			p.nodeTranslator.collection.Update(node.ID, func(n *NodeInfo) {
				n.Status = NodeStatusFailed
				n.Error = fmt.Errorf("translation not found in result")
			})
		}
	}

	return nil
}

// ProtectContent 保护Markdown内容，使用格式特定的保护器
func (p *MarkdownProcessor) ProtectContent(text string, patternProtector interface{}) string {
	pp, ok := patternProtector.(pkgdoc.PatternProtector)
	if !ok {
		p.logger.Warn("invalid pattern protector type, skipping protection")
		return text
	}

	// 使用Markdown特定的保护器
	return p.protector.ProtectContent(text, pp)
}
