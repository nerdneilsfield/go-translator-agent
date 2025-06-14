package document

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"go.uber.org/zap"
)

// TextProcessor 文本文档处理器，使用 NodeInfo 系统
type TextProcessor struct {
	opts           ProcessorOptions
	logger         *zap.Logger
	nodeTranslator *NodeInfoTranslator
}

// NewTextProcessor 创建文本处理器
func NewTextProcessor(opts ProcessorOptions, logger *zap.Logger) (*TextProcessor, error) {
	// 设置默认值
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = 2000
	}
	if opts.ChunkOverlap < 0 {
		opts.ChunkOverlap = 100
	}

	// 创建节点翻译器
	contextDistance := 2 // 前后各取2个节点作为上下文
	maxRetries := 3
	nodeTranslator := NewNodeInfoTranslator(opts.ChunkSize, contextDistance, maxRetries)

	return &TextProcessor{
		opts:           opts,
		logger:         logger,
		nodeTranslator: nodeTranslator,
	}, nil
}

// Parse 解析文本输入
func (p *TextProcessor) Parse(ctx context.Context, input io.Reader) (*Document, error) {
	// 读取所有内容
	content, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	text := string(content)

	// 创建文档
	doc := &Document{
		ID:     fmt.Sprintf("text-%d", time.Now().Unix()),
		Format: FormatText,
		Metadata: DocumentMetadata{
			CreatedAt: time.Now(),
		},
		Blocks:    []Block{},
		Resources: make(map[string]Resource),
	}

	// 分割文本为段落
	paragraphs := p.splitIntoParagraphs(text)

	// 将段落转换为块和节点
	nodeID := 1
	for i, para := range paragraphs {
		if strings.TrimSpace(para) == "" {
			continue
		}

		// 创建文档块
		block := &BaseBlock{
			Type:         BlockTypeParagraph,
			Content:      para,
			Translatable: true,
			Metadata: BlockMetadata{
				Attributes: map[string]interface{}{
					"paragraphIndex": i,
				},
			},
		}
		doc.Blocks = append(doc.Blocks, block)

		// 创建对应的 NodeInfo
		node := &NodeInfo{
			ID:           nodeID,
			BlockID:      fmt.Sprintf("block-%d", i),
			OriginalText: para,
			Status:       NodeStatusPending,
			Path:         fmt.Sprintf("/paragraph[%d]", i+1),
			Metadata: map[string]interface{}{
				"blockIndex": i,
				"blockType":  BlockTypeParagraph,
			},
		}
		p.nodeTranslator.collection.Add(node)
		nodeID++
	}

	return doc, nil
}

// Process 处理文档（分块、翻译、重组）
func (p *TextProcessor) Process(ctx context.Context, doc *Document, translator TranslateFunc) (*Document, error) {
	startTime := time.Now()
	stats := ProcessingStatistics{}

	// 获取所有待翻译的节点
	allNodes := p.nodeTranslator.collection.GetAll()
	stats.TotalBlocks = len(allNodes)

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
				// 标记这些节点的重试失败
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

		// 重置已处理节点，为下一轮重试做准备
		p.nodeTranslator.retryManager.ResetProcessedNodes()
	}

	// 更新文档块的翻译内容
	for _, block := range doc.Blocks {
		if !block.IsTranslatable() {
			stats.SkippedBlocks++
			continue
		}

		// 找到对应的节点
		blockIndex, _ := block.GetMetadata().Attributes["paragraphIndex"].(int)
		nodes := p.nodeTranslator.collection.GetAll()
		for _, node := range nodes {
			if nodeBlockIndex, ok := node.Metadata["blockIndex"].(int); ok && nodeBlockIndex == blockIndex {
				if node.IsTranslated() {
					block.SetContent(node.TranslatedText)
					stats.TranslatedBlocks++
				}
				break
			}
		}
	}

	stats.ProcessingTime = time.Since(startTime)

	// 记录处理结果
	p.logger.Info("text processing completed",
		zap.Int("totalBlocks", stats.TotalBlocks),
		zap.Int("translatedBlocks", stats.TranslatedBlocks),
		zap.Int("skippedBlocks", stats.SkippedBlocks),
		zap.Duration("processingTime", stats.ProcessingTime))

	return doc, nil
}

// Render 渲染文档为文本格式
func (p *TextProcessor) Render(ctx context.Context, doc *Document, output io.Writer) error {
	var builder strings.Builder

	for i, block := range doc.Blocks {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(block.GetContent())
	}

	_, err := output.Write([]byte(builder.String()))
	return err
}

// GetFormat 返回处理器支持的格式
func (p *TextProcessor) GetFormat() Format {
	return FormatText
}

// splitIntoParagraphs 将文本分割为段落
func (p *TextProcessor) splitIntoParagraphs(text string) []string {
	// 按双换行符分割段落
	paragraphs := strings.Split(text, "\n\n")

	// 清理每个段落
	result := make([]string, 0, len(paragraphs))
	for _, para := range paragraphs {
		cleaned := strings.TrimSpace(para)
		if cleaned != "" {
			result = append(result, cleaned)
		}
	}

	return result
}

// translateNodes 翻译节点
func (p *TextProcessor) translateNodes(ctx context.Context, nodes []*NodeInfo, translator TranslateFunc, stats *ProcessingStatistics) error {
	// 分组节点
	groups := p.nodeTranslator.grouper.GroupNodes(nodes)
	stats.TotalChunks = len(groups)

	p.logger.Info("translating text nodes",
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
func (p *TextProcessor) generateMarkedText(nodes []*NodeInfo) string {
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
func (p *TextProcessor) parseMarkedText(markedText string, nodes []*NodeInfo) error {
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
			// 开始新节点
			fmt.Sscanf(line, "@@NODE_START_%d@@", &currentNodeID)
			inNode = true
			textBuilder.Reset()
		} else if strings.HasPrefix(line, "@@NODE_END_") && strings.HasSuffix(line, "@@") {
			// 结束当前节点
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
			// 收集节点内容
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
