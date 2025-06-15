package translator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/dlclark/regexp2"
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

// TranslateNodes 翻译所有节点（并行版本，包含失败重试）
func (bt *BatchTranslator) TranslateNodes(ctx context.Context, nodes []*document.NodeInfo) error {
	// 批量翻译所有节点
	bt.logger.Info("starting batch translation", zap.Int("totalNodes", len(nodes)))
	
	// 第一轮：分组翻译所有节点
	groups := bt.groupNodes(nodes)
	bt.logger.Info("initial grouping for translation", 
		zap.Int("totalGroups", len(groups)),
		zap.Int("concurrency", bt.config.Concurrency))
	
	// 并行处理第一轮翻译
	bt.processGroups(ctx, groups)
	
	// 收集失败节点并进行重试
	maxRetries := bt.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	
	// 用于跟踪已处理的节点，避免无限递归
	processedNodes := make(map[int]bool)
	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess {
			processedNodes[node.ID] = true
		}
	}
	
	// 重试循环
	for retry := 1; retry <= maxRetries; retry++ {
		// 收集失败节点
		failedNodes := bt.collectFailedNodes(nodes)
		if len(failedNodes) == 0 {
			break
		}
		
		bt.logger.Info("collecting failed nodes for retry",
			zap.Int("retryRound", retry),
			zap.Int("failedNodes", len(failedNodes)))
		
		// 为失败节点添加上下文并重新分组
		retryGroups := bt.groupFailedNodesWithContext(nodes, failedNodes, processedNodes)
		
		if len(retryGroups) == 0 {
			bt.logger.Warn("no retry groups created, stopping retry")
			break
		}
		
		totalRetryNodes := 0
		for _, group := range retryGroups {
			totalRetryNodes += len(group.Nodes)
		}
		
		bt.logger.Info("retry grouping with context",
			zap.Int("retryRound", retry),
			zap.Int("failedNodes", len(failedNodes)),
			zap.Int("retryGroups", len(retryGroups)),
			zap.Int("totalNodesWithContext", totalRetryNodes))
		
		// 并行处理重试组
		bt.processGroups(ctx, retryGroups)
		
		// 更新已处理节点集合
		for _, node := range nodes {
			if node.Status == document.NodeStatusSuccess && !processedNodes[node.ID] {
				processedNodes[node.ID] = true
			}
		}
	}
	
	// 记录最终统计
	successCount := 0
	failedCount := 0
	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess {
			successCount++
		} else {
			failedCount++
		}
	}
	
	bt.logger.Info("batch translation completed",
		zap.Int("totalNodes", len(nodes)),
		zap.Int("successNodes", successCount),
		zap.Int("failedNodes", failedCount))
	
	return nil
}

// processGroups 并行处理节点组
func (bt *BatchTranslator) processGroups(ctx context.Context, groups []*document.NodeGroup) {
	concurrency := bt.config.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	
	// 创建工作队列
	groupChan := make(chan *document.NodeGroup, len(groups))
	errChan := make(chan error, len(groups))
	
	// 将所有组放入队列
	for _, group := range groups {
		groupChan <- group
	}
	close(groupChan)
	
	// 启动工作 goroutines
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for group := range groupChan {
				bt.logger.Debug("worker processing group",
					zap.Int("workerID", workerID),
					zap.Int("groupSize", len(group.Nodes)))
					
				if err := bt.translateGroup(ctx, group); err != nil {
					bt.logger.Warn("group translation failed", 
						zap.Int("workerID", workerID),
						zap.Error(err),
						zap.Int("groupSize", len(group.Nodes)))
					errChan <- err
				}
			}
		}(i)
	}
	
	// 等待所有工作完成
	wg.Wait()
	close(errChan)
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
	needsTranslation := false
	
	for i, node := range group.Nodes {
		// 检查是否是上下文节点（已经翻译过的）
		isContext := false
		if node.Metadata != nil {
			if ctx, ok := node.Metadata["is_context"].(bool); ok && ctx {
				isContext = true
			}
		}
		
		// 如果是上下文节点且已成功，使用已翻译的文本
		if isContext && node.Status == document.NodeStatusSuccess {
			// 添加节点标记
			builder.WriteString(fmt.Sprintf("@@NODE_START_%d@@\n", node.ID))
			builder.WriteString(node.TranslatedText)
			builder.WriteString(fmt.Sprintf("\n@@NODE_END_%d@@", node.ID))
		} else {
			// 需要翻译的节点
			needsTranslation = true
			
			// 保护内容
			protectedText := bt.protectContent(node.OriginalText, preserveManager)
			protectedTexts[node.ID] = protectedText
			
			// 添加节点标记
			builder.WriteString(fmt.Sprintf("@@NODE_START_%d@@\n", node.ID))
			builder.WriteString(protectedText)
			builder.WriteString(fmt.Sprintf("\n@@NODE_END_%d@@", node.ID))
		}
		
		if i < len(group.Nodes)-1 {
			builder.WriteString("\n\n")
		}
	}
	
	// 如果所有节点都是上下文节点，跳过翻译
	if !needsTranslation {
		bt.logger.Debug("skipping group with only context nodes", 
			zap.Int("groupSize", len(group.Nodes)))
		return nil
	}
	
	combinedText := builder.String()
	
	// 统计需要翻译的节点数
	nodesToTranslate := 0
	contextNodes := 0
	for _, node := range group.Nodes {
		if node.Metadata != nil {
			if ctx, ok := node.Metadata["is_context"].(bool); ok && ctx {
				contextNodes++
				continue
			}
		}
		nodesToTranslate++
	}
	
	bt.logger.Debug("preparing batch translation request",
		zap.Int("totalNodes", len(group.Nodes)),
		zap.Int("nodesToTranslate", nodesToTranslate),
		zap.Int("contextNodes", contextNodes),
		zap.Int("textLength", len(combinedText)))
	
	// 创建翻译请求
	req := &translation.Request{
		Text:           combinedText,
		SourceLanguage: bt.config.SourceLang,
		TargetLanguage: bt.config.TargetLang,
		Metadata: map[string]interface{}{
			"is_batch":          true,
			"node_count":        len(group.Nodes),
			"nodes_to_translate": nodesToTranslate,
			"context_nodes":     contextNodes,
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
	pattern := regexp2.MustCompile(`(?s)@@NODE_START_(\d+)@@\n(.*?)\n@@NODE_END_\1@@`, 0)
	
	// 创建结果映射
	translationMap := make(map[int]string)
	
	// 使用 regexp2 查找所有匹配
	match, _ := pattern.FindStringMatch(translatedText)
	for match != nil {
		groups := match.Groups()
		if len(groups) >= 3 {
			nodeIDStr := groups[1].String()
			nodeID, err := strconv.Atoi(nodeIDStr)
			if err != nil {
				bt.logger.Warn("invalid node ID", zap.String("nodeID", nodeIDStr))
				match, _ = pattern.FindNextMatch(match)
				continue
			}
			content := groups[2].String()
			translationMap[nodeID] = strings.TrimSpace(content)
		}
		match, _ = pattern.FindNextMatch(match)
	}
	
	// 应用翻译结果
	for _, node := range group.Nodes {
		// 检查是否是上下文节点
		isContext := false
		if node.Metadata != nil {
			if ctx, ok := node.Metadata["is_context"].(bool); ok && ctx {
				isContext = true
			}
		}
		
		// 上下文节点保持原状
		if isContext && node.Status == document.NodeStatusSuccess {
			continue
		}
		
		// 处理需要翻译的节点
		if translatedContent, ok := translationMap[node.ID]; ok {
			// 还原保护的内容
			restoredText := preserveManager.Restore(translatedContent)
			node.TranslatedText = restoredText
			node.Status = document.NodeStatusSuccess
			node.Error = nil
			
			// 增加重试计数
			node.RetryCount++
		} else {
			node.Status = document.NodeStatusFailed
			node.Error = fmt.Errorf("translation not found in batch result")
			node.RetryCount++
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

// collectFailedNodes 收集失败的节点
func (bt *BatchTranslator) collectFailedNodes(nodes []*document.NodeInfo) []*document.NodeInfo {
	var failed []*document.NodeInfo
	for _, node := range nodes {
		if node.Status != document.NodeStatusSuccess {
			failed = append(failed, node)
		}
	}
	return failed
}

// groupFailedNodesWithContext 为失败节点添加上下文并重新分组
func (bt *BatchTranslator) groupFailedNodesWithContext(allNodes []*document.NodeInfo, failedNodes []*document.NodeInfo, processedNodes map[int]bool) []*document.NodeGroup {
	// 创建节点ID到索引的映射
	nodeIDToIndex := make(map[int]int)
	for i, node := range allNodes {
		nodeIDToIndex[node.ID] = i
	}
	
	// 收集需要包含的节点（失败节点及其上下文）
	includeSet := make(map[int]bool)
	contextNodeCount := 0
	
	for _, failed := range failedNodes {
		idx, exists := nodeIDToIndex[failed.ID]
		if !exists {
			continue
		}
		
		// 添加失败节点本身
		includeSet[failed.ID] = true
		
		// 添加前面的上下文节点（最多2个）
		contextBefore := 0
		for i := idx - 1; i >= 0 && contextBefore < 2; i-- {
			nodeID := allNodes[i].ID
			// 只添加已成功翻译的节点作为上下文
			if !includeSet[nodeID] && processedNodes[nodeID] {
				includeSet[nodeID] = true
				contextBefore++
				contextNodeCount++
			}
		}
		
		// 添加后面的上下文节点（最多2个）
		contextAfter := 0
		for i := idx + 1; i < len(allNodes) && contextAfter < 2; i++ {
			nodeID := allNodes[i].ID
			// 只添加已成功翻译的节点作为上下文
			if !includeSet[nodeID] && processedNodes[nodeID] {
				includeSet[nodeID] = true
				contextAfter++
				contextNodeCount++
			}
		}
	}
	
	bt.logger.Info("context nodes added for retry",
		zap.Int("contextNodes", contextNodeCount),
		zap.Int("totalNodesForRetry", len(includeSet)))
	
	// 收集所有需要翻译的节点，保持原始顺序
	var nodesToTranslate []*document.NodeInfo
	for _, node := range allNodes {
		if includeSet[node.ID] {
			// 为已处理的节点标记，避免重复计算
			newNode := &document.NodeInfo{
				ID:             node.ID,
				BlockID:        node.BlockID,
				OriginalText:   node.OriginalText,
				TranslatedText: node.TranslatedText,
				Status:         node.Status,
				Path:           node.Path,
				Metadata:       node.Metadata,
				Error:          node.Error,
				RetryCount:     node.RetryCount,
			}
			
			// 如果是已成功的上下文节点，添加标记
			if processedNodes[node.ID] && node.Status == document.NodeStatusSuccess {
				if newNode.Metadata == nil {
					newNode.Metadata = make(map[string]interface{})
				}
				newNode.Metadata["is_context"] = true
			}
			
			nodesToTranslate = append(nodesToTranslate, newNode)
		}
	}
	
	// 分组
	return bt.groupNodes(nodesToTranslate)
}

