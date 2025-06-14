package document

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
	"golang.org/x/net/html"
)

// HTMLProcessor HTML文档处理器，使用 NodeInfo 系统
type HTMLProcessor struct {
	opts           ProcessorOptions
	logger         *zap.Logger
	nodeTranslator *NodeInfoTranslator
	mode           HTMLProcessingMode
}

// HTMLProcessingMode HTML处理模式
type HTMLProcessingMode string

const (
	// HTMLModeMarkdown 先转换为Markdown处理再转回HTML
	HTMLModeMarkdown HTMLProcessingMode = "markdown"
	// HTMLModeNative 原生HTML处理
	HTMLModeNative HTMLProcessingMode = "native"
)

// NewHTMLProcessor 创建HTML处理器
func NewHTMLProcessor(opts ProcessorOptions, logger *zap.Logger, mode HTMLProcessingMode) (*HTMLProcessor, error) {
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

	return &HTMLProcessor{
		opts:           opts,
		logger:         logger,
		nodeTranslator: nodeTranslator,
		mode:           mode,
	}, nil
}

// Parse 解析HTML输入
func (p *HTMLProcessor) Parse(ctx context.Context, input io.Reader) (*Document, error) {
	// 读取所有内容
	content, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	htmlStr := string(content)

	// 创建文档
	doc := &Document{
		ID:     fmt.Sprintf("html-%d", time.Now().Unix()),
		Format: FormatHTML,
		Metadata: DocumentMetadata{
			CreatedAt: time.Now(),
		},
		Blocks:    []Block{},
		Resources: make(map[string]Resource),
	}

	// 根据模式选择处理方式
	if p.mode == HTMLModeMarkdown {
		return p.parseAsMarkdown(ctx, htmlStr, doc)
	}

	return p.parseNative(ctx, htmlStr, doc)
}

// parseAsMarkdown 将HTML转换为Markdown后处理
func (p *HTMLProcessor) parseAsMarkdown(ctx context.Context, htmlStr string, doc *Document) (*Document, error) {
	// 使用 goquery 解析 HTML
	reader := strings.NewReader(htmlStr)
	gqDoc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// 提取 body 内容
	body := gqDoc.Find("body")
	if body.Length() == 0 {
		body = gqDoc.Selection
	}

	// 转换为 Markdown
	markdown := p.htmlToMarkdown(body)

	// 保存原始HTML结构
	doc.Metadata.CustomFields = map[string]interface{}{
		"originalHTML": htmlStr,
		"bodyHTML":     getOuterHTML(body),
		"mode":         "markdown",
	}

	// 创建一个Markdown块
	block := &BaseBlock{
		Type:         BlockTypeCustom,
		Content:      markdown,
		Translatable: true,
		Metadata: BlockMetadata{
			Attributes: map[string]interface{}{
				"format": "markdown",
			},
		},
	}
	doc.Blocks = append(doc.Blocks, block)

	// 创建对应的 NodeInfo
	node := &NodeInfo{
		ID:           1,
		BlockID:      "markdown-content",
		OriginalText: markdown,
		Status:       NodeStatusPending,
		Path:         "/markdown",
		Metadata: map[string]interface{}{
			"type": "markdown",
		},
	}
	p.nodeTranslator.collection.Add(node)

	return doc, nil
}

// parseNative 原生解析HTML
func (p *HTMLProcessor) parseNative(ctx context.Context, htmlStr string, doc *Document) (*Document, error) {
	// 使用 goquery 解析 HTML
	reader := strings.NewReader(htmlStr)
	gqDoc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// 保存原始HTML
	doc.Metadata.CustomFields = map[string]interface{}{
		"originalHTML": htmlStr,
		"mode":         "native",
	}

	// 收集所有文本节点
	nodeID := 1
	var collectNodes func(*goquery.Selection, string)
	collectNodes = func(sel *goquery.Selection, path string) {
		sel.Contents().Each(func(i int, s *goquery.Selection) {
			currentPath := fmt.Sprintf("%s[%d]", path, i+1)

			// 检查是否是文本节点
			if s.Nodes != nil && len(s.Nodes) > 0 {
				node := s.Nodes[0]
				if node.Type == html.TextNode {
					text := strings.TrimSpace(node.Data)
					if text != "" {
						// 创建块
						block := &BaseBlock{
							Type:         BlockTypeCustom,
							Content:      node.Data, // 保留原始空白
							Translatable: true,
							Metadata: BlockMetadata{
								Attributes: map[string]interface{}{
									"nodeType": "text",
									"path":     currentPath,
								},
							},
						}
						doc.Blocks = append(doc.Blocks, block)

						// 创建 NodeInfo
						nodeInfo := &NodeInfo{
							ID:           nodeID,
							BlockID:      fmt.Sprintf("text-node-%d", nodeID),
							OriginalText: text,
							Status:       NodeStatusPending,
							Path:         currentPath,
							Metadata: map[string]interface{}{
								"originalData":  node.Data,
								"leadingSpace":  getLeadingSpace(node.Data),
								"trailingSpace": getTrailingSpace(node.Data),
								"selection":     s,
								"parentTag":     s.Parent().Nodes[0].Data,
							},
						}
						p.nodeTranslator.collection.Add(nodeInfo)
						nodeID++
					}
				} else if node.Type == html.ElementNode {
					// 递归处理子元素
					collectNodes(s, currentPath)
				}
			}
		})

		// 处理属性中的文本
		if sel.Nodes != nil && len(sel.Nodes) > 0 && sel.Nodes[0].Type == html.ElementNode {
			// 检查需要翻译的属性
			translatableAttrs := []string{"alt", "title", "placeholder", "aria-label"}
			for _, attr := range translatableAttrs {
				if val, exists := sel.Attr(attr); exists && val != "" {
					// 创建属性块
					block := &BaseBlock{
						Type:         BlockTypeCustom,
						Content:      val,
						Translatable: true,
						Metadata: BlockMetadata{
							Attributes: map[string]interface{}{
								"nodeType":      "attribute",
								"attributeName": attr,
								"path":          path,
							},
						},
					}
					doc.Blocks = append(doc.Blocks, block)

					// 创建 NodeInfo
					nodeInfo := &NodeInfo{
						ID:           nodeID,
						BlockID:      fmt.Sprintf("attr-%d", nodeID),
						OriginalText: val,
						Status:       NodeStatusPending,
						Path:         fmt.Sprintf("%s/@%s", path, attr),
						Metadata: map[string]interface{}{
							"isAttribute":   true,
							"attributeName": attr,
							"selection":     sel,
						},
					}
					p.nodeTranslator.collection.Add(nodeInfo)
					nodeID++
				}
			}
		}
	}

	// 从根元素开始收集
	collectNodes(gqDoc.Selection, "")

	return doc, nil
}

// Process 处理文档
func (p *HTMLProcessor) Process(ctx context.Context, doc *Document, translator TranslateFunc) (*Document, error) {
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
		blockID := fmt.Sprintf("text-node-%d", i+1)
		if i == 0 && p.mode == HTMLModeMarkdown {
			blockID = "markdown-content"
		} else if strings.HasPrefix(block.GetMetadata().Attributes["nodeType"].(string), "attr") {
			blockID = fmt.Sprintf("attr-%d", i+1)
		}

		if node, exists := nodeMap[blockID]; exists && node.IsTranslated() {
			block.SetContent(node.TranslatedText)
			stats.TranslatedBlocks++
		} else {
			stats.SkippedBlocks++
		}
	}

	stats.ProcessingTime = time.Since(startTime)

	p.logger.Info("HTML processing completed",
		zap.Int("totalBlocks", stats.TotalBlocks),
		zap.Int("translatedBlocks", stats.TranslatedBlocks),
		zap.Int("skippedBlocks", stats.SkippedBlocks),
		zap.Duration("processingTime", stats.ProcessingTime))

	return doc, nil
}

// Render 渲染文档
func (p *HTMLProcessor) Render(ctx context.Context, doc *Document, output io.Writer) error {
	if p.mode == HTMLModeMarkdown {
		return p.renderFromMarkdown(ctx, doc, output)
	}
	return p.renderNative(ctx, doc, output)
}

// renderFromMarkdown 从Markdown渲染回HTML
func (p *HTMLProcessor) renderFromMarkdown(ctx context.Context, doc *Document, output io.Writer) error {
	// 获取翻译后的Markdown内容
	if len(doc.Blocks) == 0 {
		return fmt.Errorf("no content to render")
	}

	translatedMarkdown := doc.Blocks[0].GetContent()

	// 转换回HTML
	htmlContent := p.markdownToHTML(translatedMarkdown)

	// 获取原始HTML结构
	originalHTML, _ := doc.Metadata.CustomFields["originalHTML"].(string)
	bodyHTML, _ := doc.Metadata.CustomFields["bodyHTML"].(string)

	// 替换body内容
	if originalHTML != "" && bodyHTML != "" {
		// 创建新的body内容
		newBody := fmt.Sprintf("<body>%s</body>", htmlContent)

		// 在原始HTML中替换body
		result := strings.Replace(originalHTML, bodyHTML, newBody, 1)
		_, err := output.Write([]byte(result))
		return err
	}

	// 如果没有原始结构，直接输出转换后的HTML
	_, err := output.Write([]byte(htmlContent))
	return err
}

// renderNative 原生渲染HTML
func (p *HTMLProcessor) renderNative(ctx context.Context, doc *Document, output io.Writer) error {
	// 获取原始HTML
	originalHTML, _ := doc.Metadata.CustomFields["originalHTML"].(string)
	if originalHTML == "" {
		return fmt.Errorf("no original HTML found")
	}

	// 解析HTML
	reader := strings.NewReader(originalHTML)
	gqDoc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return fmt.Errorf("failed to parse HTML: %w", err)
	}

	// 应用翻译
	nodes := p.nodeTranslator.collection.GetAll()
	for _, node := range nodes {
		if !node.IsTranslated() {
			continue
		}

		// 获取selection
		if sel, ok := node.Metadata["selection"].(*goquery.Selection); ok {
			if isAttr, _ := node.Metadata["isAttribute"].(bool); isAttr {
				// 更新属性
				if attrName, _ := node.Metadata["attributeName"].(string); attrName != "" {
					sel.SetAttr(attrName, node.TranslatedText)
				}
			} else {
				// 更新文本节点
				if sel.Nodes != nil && len(sel.Nodes) > 0 {
					htmlNode := sel.Nodes[0]
					if htmlNode.Type == html.TextNode {
						// 保留原始空白
						leading, _ := node.Metadata["leadingSpace"].(string)
						trailing, _ := node.Metadata["trailingSpace"].(string)
						htmlNode.Data = leading + node.TranslatedText + trailing
					}
				}
			}
		}
	}

	// 输出HTML
	htmlStr, err := gqDoc.Html()
	if err != nil {
		return fmt.Errorf("failed to render HTML: %w", err)
	}

	_, err = output.Write([]byte(htmlStr))
	return err
}

// GetFormat 返回支持的格式
func (p *HTMLProcessor) GetFormat() Format {
	return FormatHTML
}

// translateNodes 翻译节点
func (p *HTMLProcessor) translateNodes(ctx context.Context, nodes []*NodeInfo, translator TranslateFunc, stats *ProcessingStatistics) error {
	// 分组节点
	groups := p.nodeTranslator.grouper.GroupNodes(nodes)
	stats.TotalChunks = len(groups)

	p.logger.Info("translating HTML nodes",
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
func (p *HTMLProcessor) generateMarkedText(nodes []*NodeInfo) string {
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
func (p *HTMLProcessor) parseMarkedText(markedText string, nodes []*NodeInfo) error {
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

// htmlToMarkdown 简单的HTML到Markdown转换
func (p *HTMLProcessor) htmlToMarkdown(sel *goquery.Selection) string {
	var builder strings.Builder
	p.convertNodeToMarkdown(sel, &builder, 0)
	return strings.TrimSpace(builder.String())
}

// convertNodeToMarkdown 递归转换节点
func (p *HTMLProcessor) convertNodeToMarkdown(sel *goquery.Selection, builder *strings.Builder, level int) {
	sel.Contents().Each(func(i int, s *goquery.Selection) {
		if s.Nodes != nil && len(s.Nodes) > 0 {
			node := s.Nodes[0]

			switch node.Type {
			case html.TextNode:
				text := strings.TrimSpace(node.Data)
				if text != "" {
					builder.WriteString(text)
				}

			case html.ElementNode:
				switch node.Data {
				case "p":
					builder.WriteString("\n\n")
					p.convertNodeToMarkdown(s, builder, level)
					builder.WriteString("\n\n")

				case "h1", "h2", "h3", "h4", "h5", "h6":
					headerLevel := int(node.Data[1] - '0')
					builder.WriteString("\n\n")
					builder.WriteString(strings.Repeat("#", headerLevel))
					builder.WriteString(" ")
					p.convertNodeToMarkdown(s, builder, level)
					builder.WriteString("\n\n")

				case "strong", "b":
					builder.WriteString("**")
					p.convertNodeToMarkdown(s, builder, level)
					builder.WriteString("**")

				case "em", "i":
					builder.WriteString("*")
					p.convertNodeToMarkdown(s, builder, level)
					builder.WriteString("*")

				case "code":
					builder.WriteString("`")
					p.convertNodeToMarkdown(s, builder, level)
					builder.WriteString("`")

				case "pre":
					builder.WriteString("\n```\n")
					p.convertNodeToMarkdown(s, builder, level)
					builder.WriteString("\n```\n")

				case "ul", "ol":
					builder.WriteString("\n")
					p.convertNodeToMarkdown(s, builder, level+1)

				case "li":
					builder.WriteString("\n")
					builder.WriteString(strings.Repeat("  ", level-1))
					if s.Parent().Nodes[0].Data == "ol" {
						builder.WriteString("1. ")
					} else {
						builder.WriteString("- ")
					}
					p.convertNodeToMarkdown(s, builder, level)

				case "a":
					builder.WriteString("[")
					p.convertNodeToMarkdown(s, builder, level)
					builder.WriteString("](")
					if href, exists := s.Attr("href"); exists {
						builder.WriteString(href)
					}
					builder.WriteString(")")

				case "img":
					builder.WriteString("![")
					if alt, exists := s.Attr("alt"); exists {
						builder.WriteString(alt)
					}
					builder.WriteString("](")
					if src, exists := s.Attr("src"); exists {
						builder.WriteString(src)
					}
					builder.WriteString(")")

				case "br":
					builder.WriteString("  \n")

				default:
					p.convertNodeToMarkdown(s, builder, level)
				}
			}
		}
	})
}

// markdownToHTML 简单的Markdown到HTML转换
func (p *HTMLProcessor) markdownToHTML(markdown string) string {
	lines := strings.Split(markdown, "\n")
	var builder strings.Builder
	inList := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 标题
		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for i, ch := range trimmed {
				if ch == '#' {
					level++
				} else {
					text := strings.TrimSpace(trimmed[i:])
					builder.WriteString(fmt.Sprintf("<h%d>%s</h%d>\n", level, text, level))
					break
				}
			}
			continue
		}

		// 列表
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				builder.WriteString("<ul>\n")
				inList = true
			}
			text := strings.TrimSpace(trimmed[2:])
			builder.WriteString(fmt.Sprintf("  <li>%s</li>\n", p.processInlineMarkdown(text)))
			continue
		} else if inList && trimmed == "" {
			builder.WriteString("</ul>\n")
			inList = false
		}

		// 段落
		if trimmed != "" {
			builder.WriteString(fmt.Sprintf("<p>%s</p>\n", p.processInlineMarkdown(trimmed)))
		}
	}

	if inList {
		builder.WriteString("</ul>\n")
	}

	return builder.String()
}

// processInlineMarkdown 处理内联Markdown元素
func (p *HTMLProcessor) processInlineMarkdown(text string) string {
	// 粗体
	text = strings.ReplaceAll(text, "**", "<strong>")
	text = strings.ReplaceAll(text, "<strong>", "</strong>")

	// 斜体
	text = strings.ReplaceAll(text, "*", "<em>")
	text = strings.ReplaceAll(text, "<em>", "</em>")

	// 代码
	text = strings.ReplaceAll(text, "`", "<code>")
	text = strings.ReplaceAll(text, "<code>", "</code>")

	return text
}

// 辅助函数
func getLeadingSpace(s string) string {
	for i, ch := range s {
		if ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r' {
			return s[:i]
		}
	}
	return s
}

func getTrailingSpace(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		ch := s[i]
		if ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r' {
			return s[i+1:]
		}
	}
	return s
}

func getOuterHTML(sel *goquery.Selection) string {
	html, _ := sel.Html()
	if sel.Nodes != nil && len(sel.Nodes) > 0 {
		node := sel.Nodes[0]
		if node.Type == 1 { // html.ElementNode = 1
			return fmt.Sprintf("<%s>%s</%s>", node.Data, html, node.Data)
		}
	}
	return html
}
