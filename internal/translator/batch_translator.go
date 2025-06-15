package translator

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"go.uber.org/zap"
)

// BatchTranslator 批量翻译器，集成所有保护功能
type BatchTranslator struct {
	config             *config.Config
	translationService translation.Service
	logger             *zap.Logger
	preserveManager    *translation.PreserveManager
}

// NewBatchTranslator 创建批量翻译器
func NewBatchTranslator(cfg *config.Config, service translation.Service, logger *zap.Logger) *BatchTranslator {
	return &BatchTranslator{
		config:             cfg,
		translationService: service,
		logger:             logger,
		preserveManager:    translation.NewPreserveManager(translation.DefaultPreserveConfig),
	}
}

// TranslateNodes 翻译所有节点（简化版，不包含重试逻辑）
func (bt *BatchTranslator) TranslateNodes(ctx context.Context, nodes []*document.NodeInfo) error {
	// 批量翻译所有节点
	bt.logger.Info("starting batch translation", zap.Int("totalNodes", len(nodes)))
	
	// 分组翻译
	groups := bt.groupNodes(nodes)
	for _, group := range groups {
		if err := bt.translateGroup(ctx, group); err != nil {
			bt.logger.Warn("group translation failed", 
				zap.Error(err),
				zap.Int("groupSize", len(group.Nodes)))
		}
	}
	
	// 记录最终统计
	successCount := 0
	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess {
			successCount++
		}
	}
	
	bt.logger.Info("batch translation completed",
		zap.Int("totalNodes", len(nodes)),
		zap.Int("successNodes", successCount),
		zap.Int("failedNodes", len(nodes)-successCount))
	
	return nil
}

// translateGroup 翻译一个节点组
func (bt *BatchTranslator) translateGroup(ctx context.Context, group *document.NodeGroup) error {
	if bt.translationService == nil {
		// 模拟翻译
		for _, node := range group.Nodes {
			node.TranslatedText = "Translated: " + node.OriginalText
			node.Status = document.NodeStatusSuccess
		}
		return nil
	}
	
	// 保护不需要翻译的内容
	protectedTexts := make(map[int]string) // nodeID -> protected text
	preserveManager := translation.NewPreserveManager(translation.DefaultPreserveConfig)
	
	// 构建批量翻译文本
	var builder strings.Builder
	for i, node := range group.Nodes {
		// 保护内容
		protectedText := bt.protectContent(node.OriginalText, preserveManager)
		protectedTexts[node.ID] = protectedText
		
		// 添加节点标记
		builder.WriteString(fmt.Sprintf("@@NODE_START_%d@@\n", node.ID))
		builder.WriteString(protectedText)
		builder.WriteString(fmt.Sprintf("\n@@NODE_END_%d@@", node.ID))
		
		if i < len(group.Nodes)-1 {
			builder.WriteString("\n\n")
		}
	}
	
	combinedText := builder.String()
	
	// 创建翻译请求
	req := &translation.Request{
		Text:           combinedText,
		SourceLanguage: bt.config.SourceLang,
		TargetLanguage: bt.config.TargetLang,
		Metadata: map[string]interface{}{
			"is_batch":          true,
			"node_count":        len(group.Nodes),
			"_is_batch":         "true",         // 内部标记
			"_preserve_enabled": "true",         // 内部标记
		},
	}
	
	// 执行翻译
	resp, err := bt.translationService.Translate(ctx, req)
	if err != nil {
		// 标记所有节点失败
		for _, node := range group.Nodes {
			node.Status = document.NodeStatusFailed
			node.Error = err
		}
		return err
	}
	
	// 解析翻译结果
	translatedText := resp.Text
	pattern := regexp.MustCompile(`(?s)@@NODE_START_(\d+)@@\n(.*?)\n@@NODE_END_\1@@`)
	matches := pattern.FindAllStringSubmatch(translatedText, -1)
	
	// 创建结果映射
	translationMap := make(map[int]string)
	for _, match := range matches {
		if len(match) >= 3 {
			nodeID, err := strconv.Atoi(match[1])
			if err != nil {
				bt.logger.Warn("invalid node ID", zap.String("nodeID", match[1]))
				continue
			}
			translationMap[nodeID] = strings.TrimSpace(match[2])
		}
	}
	
	// 应用翻译结果
	for _, node := range group.Nodes {
		if translatedContent, ok := translationMap[node.ID]; ok {
			// 还原保护的内容
			restoredText := preserveManager.Restore(translatedContent)
			node.TranslatedText = restoredText
			node.Status = document.NodeStatusSuccess
		} else {
			node.Status = document.NodeStatusFailed
			node.Error = fmt.Errorf("translation not found in batch result")
		}
	}
	
	return nil
}

// protectContent 保护不需要翻译的内容
func (bt *BatchTranslator) protectContent(text string, pm *translation.PreserveManager) string {
	// LaTeX 公式
	text = pm.ProtectPattern(text, `\$[^$]+\$`)                // 行内公式
	text = pm.ProtectPattern(text, `\$\$[^$]+\$\$`)          // 行间公式
	text = pm.ProtectPattern(text, `\\\([^)]+\\\)`)          // \(...\)
	text = pm.ProtectPattern(text, `\\\[[^\]]+\\\]`)         // \[...\]
	
	// 代码块
	text = pm.ProtectPattern(text, "`[^`]+`")                // 行内代码
	text = protectCodeBlocks(text, pm)                       // 多行代码块
	
	// HTML 标签
	text = pm.ProtectPattern(text, `<[^>]+>`)                // HTML 标签
	text = pm.ProtectPattern(text, `&[a-zA-Z]+;`)            // HTML 实体
	text = pm.ProtectPattern(text, `&#\d+;`)                 // 数字实体
	
	// URL
	text = pm.ProtectPattern(text, `(?i)(https?|ftp|file)://[^\s\)]+`)
	text = pm.ProtectPattern(text, `(?i)www\.[^\s\)]+`)
	
	// 文件路径
	text = pm.ProtectPattern(text, `(?:^|[\s(])/(?:[^/\s]+/)*[^/\s]+(?:\.[a-zA-Z0-9]+)?`)
	text = pm.ProtectPattern(text, `[A-Za-z]:\\(?:[^\\/:*?"<>|\r\n]+\\)*[^\\/:*?"<>|\r\n]+`)
	text = pm.ProtectPattern(text, `\.{1,2}/(?:[^/\s]+/)*[^/\s]+(?:\.[a-zA-Z0-9]+)?`)
	
	// 引用标记
	text = pm.ProtectPattern(text, `\[\d+\]`)                                    // [1], [2]
	text = pm.ProtectPattern(text, `\[[A-Za-z]+(?:\s+et\s+al\.)?,\s*\d{4}\]`)  // [Author, Year]
	text = pm.ProtectPattern(text, `\\cite\{[^}]+\}`)                           // \cite{}
	text = pm.ProtectPattern(text, `\\ref\{[^}]+\}`)                            // \ref{}
	text = pm.ProtectPattern(text, `\\label\{[^}]+\}`)                          // \label{}
	
	// 其他
	text = pm.ProtectPattern(text, `\{\{[^}]+\}\}`)                             // {{variable}}
	text = pm.ProtectPattern(text, `<%[^%]+%>`)                                 // <% %>
	text = pm.ProtectPattern(text, `<!--[\s\S]*?-->`)                           // <!-- -->
	text = pm.ProtectPattern(text, `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`) // 邮箱
	
	return text
}

// protectCodeBlocks 保护多行代码块
func protectCodeBlocks(text string, pm *translation.PreserveManager) string {
	lines := strings.Split(text, "\n")
	inCodeBlock := false
	codeBlockContent := []string{}
	result := []string{}
	
	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			if !inCodeBlock {
				inCodeBlock = true
				codeBlockContent = []string{line}
			} else {
				codeBlockContent = append(codeBlockContent, line)
				codeBlock := strings.Join(codeBlockContent, "\n")
				placeholder := pm.Protect(codeBlock)
				result = append(result, placeholder)
				inCodeBlock = false
				codeBlockContent = []string{}
			}
		} else if inCodeBlock {
			codeBlockContent = append(codeBlockContent, line)
		} else {
			result = append(result, line)
		}
	}
	
	if inCodeBlock {
		result = append(result, codeBlockContent...)
	}
	
	return strings.Join(result, "\n")
}

// groupNodes 将节点分组
func (bt *BatchTranslator) groupNodes(nodes []*document.NodeInfo) []*document.NodeGroup {
	var groups []*document.NodeGroup
	var currentGroup []*document.NodeInfo
	currentSize := 0
	
	maxSize := bt.config.ChunkSize
	if maxSize <= 0 {
		maxSize = 1000
	}
	
	for _, node := range nodes {
		nodeSize := len(node.OriginalText)
		
		// 如果当前组加上这个节点会超过限制，先保存当前组
		if currentSize > 0 && currentSize+nodeSize > maxSize {
			groups = append(groups, &document.NodeGroup{
				Nodes: currentGroup,
				Size:  currentSize,
			})
			currentGroup = nil
			currentSize = 0
		}
		
		currentGroup = append(currentGroup, node)
		currentSize += nodeSize
	}
	
	// 保存最后一组
	if len(currentGroup) > 0 {
		groups = append(groups, &document.NodeGroup{
			Nodes: currentGroup,
			Size:  currentSize,
		})
	}
	
	return groups
}

