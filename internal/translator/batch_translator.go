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
	var nodeIDsToTranslate []int
	for _, node := range group.Nodes {
		if node.Metadata != nil {
			if ctx, ok := node.Metadata["is_context"].(bool); ok && ctx {
				contextNodes++
				continue
			}
		}
		nodesToTranslate++
		nodeIDsToTranslate = append(nodeIDsToTranslate, node.ID)
	}
	
	if bt.config.Verbose && len(nodeIDsToTranslate) > 0 {
		bt.logger.Info("nodes to translate in this group",
			zap.Ints("nodeIDs", nodeIDsToTranslate))
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
	
	// 在发送前记录详细的请求内容
	bt.logger.Info("sending batch translation request",
		zap.Int("requestLength", len(combinedText)),
		zap.Int("nodeCount", len(group.Nodes)),
		zap.String("requestPreview", truncateText(combinedText, 500)))
	
	// 如果是 verbose 模式，记录完整的节点标记
	if bt.config.Verbose {
		// 提取所有节点标记
		nodeMarkers := []string{}
		lines := strings.Split(combinedText, "\n")
		for _, line := range lines {
			if strings.Contains(line, "@@NODE_START_") || strings.Contains(line, "@@NODE_END_") {
				nodeMarkers = append(nodeMarkers, line)
			}
		}
		bt.logger.Info("node markers in request",
			zap.Strings("markers", nodeMarkers))
	}
	
	// 执行翻译
	resp, err := bt.translationService.Translate(ctx, req)
	if err != nil {
		bt.logger.Error("translation service error",
			zap.Error(err),
			zap.Int("nodeCount", len(group.Nodes)))
		// 标记所有节点失败
		for _, node := range group.Nodes {
			node.Status = document.NodeStatusFailed
			node.Error = err
		}
		return err
	}
	
	// 记录响应详情
	translatedText := resp.Text
	bt.logger.Info("received translation response",
		zap.Int("responseLength", len(translatedText)),
		zap.String("responsePreview", truncateText(translatedText, 500)))
	
	// 检查响应格式
	isJSON := strings.HasPrefix(strings.TrimSpace(translatedText), "{") || strings.HasPrefix(strings.TrimSpace(translatedText), "[")
	isEmpty := len(strings.TrimSpace(translatedText)) == 0
	
	// 检查响应是否包含节点标记
	hasStartMarkers := strings.Contains(translatedText, "@@NODE_START_")
	hasEndMarkers := strings.Contains(translatedText, "@@NODE_END_")
	bt.logger.Info("response format check",
		zap.Bool("hasStartMarkers", hasStartMarkers),
		zap.Bool("hasEndMarkers", hasEndMarkers),
		zap.Bool("isJSON", isJSON),
		zap.Bool("isEmpty", isEmpty))
	
	// 如果响应看起来像 JSON，尝试提取其中的文本
	if isJSON && !hasStartMarkers {
		bt.logger.Warn("response appears to be JSON without node markers",
			zap.String("jsonPreview", truncateText(translatedText, 200)))
	}
	
	// 如果没有找到节点标记，运行诊断
	if !hasStartMarkers || !hasEndMarkers {
		diagnostic := DiagnoseBatchTranslationIssue(combinedText, translatedText)
		bt.logger.Warn("batch translation diagnostic",
			zap.String("diagnostic", diagnostic.Format()))
	}
	
	// 如果是 verbose 模式，记录响应中的所有节点标记
	if bt.config.Verbose {
		responseMarkers := []string{}
		lines := strings.Split(translatedText, "\n")
		for i, line := range lines {
			if strings.Contains(line, "@@NODE_START_") || strings.Contains(line, "@@NODE_END_") {
				responseMarkers = append(responseMarkers, fmt.Sprintf("Line %d: %s", i+1, line))
			}
		}
		bt.logger.Info("node markers in response",
			zap.Strings("markers", responseMarkers))
		
		// 记录前几行和后几行
		if len(lines) > 0 {
			numLines := len(lines)
			firstCount := 10
			if numLines < firstCount {
				firstCount = numLines
			}
			firstLines := lines[:firstCount]
			
			lastStart := numLines - 10
			if lastStart < 0 {
				lastStart = 0
			}
			lastLines := lines[lastStart:]
			bt.logger.Info("response structure",
				zap.Strings("firstLines", firstLines),
				zap.Strings("lastLines", lastLines))
		}
	}
	
	// 解析翻译结果
	pattern := regexp2.MustCompile(`(?s)@@NODE_START_(\d+)@@\n(.*?)\n@@NODE_END_\1@@`, 0)
	
	// 创建结果映射
	translationMap := make(map[int]string)
	
	// 先尝试调试正则表达式
	if bt.config.Verbose {
		// 尝试简单的正则匹配来验证格式
		simplePattern := regexp2.MustCompile(`@@NODE_START_\d+@@`, 0)
		simpleMatch, _ := simplePattern.FindStringMatch(translatedText)
		simpleMatchCount := 0
		for simpleMatch != nil {
			simpleMatchCount++
			simpleMatch, _ = simplePattern.FindNextMatch(simpleMatch)
		}
		bt.logger.Info("simple pattern match test",
			zap.Int("simpleMatchCount", simpleMatchCount))
	}
	
	// 使用 regexp2 查找所有匹配
	match, err := pattern.FindStringMatch(translatedText)
	if err != nil {
		bt.logger.Error("regex error", zap.Error(err))
	}
	
	matchCount := 0
	foundNodeIDs := []int{}
	
	for match != nil {
		groups := match.Groups()
		if bt.config.Verbose && matchCount < 3 {
			// 记录前几个匹配的详细信息
			bt.logger.Info("regex match details",
				zap.Int("matchNumber", matchCount+1),
				zap.Int("groupCount", len(groups)),
				zap.String("fullMatch", groups[0].String()))
		}
		
		if len(groups) >= 3 {
			nodeIDStr := groups[1].String()
			nodeID, err := strconv.Atoi(nodeIDStr)
			if err != nil {
				bt.logger.Warn("invalid node ID", 
					zap.String("nodeID", nodeIDStr),
					zap.Error(err))
				match, _ = pattern.FindNextMatch(match)
				continue
			}
			content := groups[2].String()
			translationMap[nodeID] = strings.TrimSpace(content)
			matchCount++
			foundNodeIDs = append(foundNodeIDs, nodeID)
			
			if bt.config.Verbose && matchCount <= 3 {
				bt.logger.Info("parsed node translation",
					zap.Int("nodeID", nodeID),
					zap.String("contentPreview", truncateText(content, 100)))
			}
		}
		match, _ = pattern.FindNextMatch(match)
	}
	
	if bt.config.Verbose {
		bt.logger.Info("parsed translation results",
			zap.Int("matchCount", matchCount),
			zap.Int("expectedNodes", len(group.Nodes)),
			zap.Int("translatedTextLength", len(translatedText)),
			zap.Int("nodesToTranslate", nodesToTranslate),
			zap.Int("contextNodes", contextNodes),
			zap.Ints("foundNodeIDs", foundNodeIDs))
		
		// 显示翻译片段
		if matchCount > 0 {
			bt.logger.Info("translation snippets",
				zap.Int("totalMatches", matchCount),
				zap.String("firstSnippet", truncateText(translatedText, 300)))
		}
		
		// 显示解析到的节点ID
		if len(translationMap) > 0 {
			var parsedIDs []int
			for id := range translationMap {
				parsedIDs = append(parsedIDs, id)
			}
			bt.logger.Info("parsed node IDs",
				zap.Ints("nodeIDs", parsedIDs))
		}
		
		// 记录期望但未找到的节点
		var missingNodeIDs []int
		for _, node := range group.Nodes {
			// 跳过上下文节点
			if node.Metadata != nil {
				if ctx, ok := node.Metadata["is_context"].(bool); ok && ctx && node.Status == document.NodeStatusSuccess {
					continue
				}
			}
			if _, found := translationMap[node.ID]; !found {
				missingNodeIDs = append(missingNodeIDs, node.ID)
			}
		}
		if len(missingNodeIDs) > 0 {
			bt.logger.Warn("missing node translations",
				zap.Ints("missingNodeIDs", missingNodeIDs),
				zap.Int("missingCount", len(missingNodeIDs)))
		}
	} else {
		bt.logger.Info("parsed translation results",
			zap.Int("matchCount", matchCount),
			zap.Int("expectedNodes", len(group.Nodes)))
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
			// 获取该节点的保护后文本（用于相似度比较）
			protectedOriginal := ""
			if protectedText, ok := protectedTexts[node.ID]; ok {
				protectedOriginal = protectedText
			} else {
				protectedOriginal = node.OriginalText
			}
			
			// 检查翻译质量（使用编辑距离）- 比较保护后的文本
			similarity := bt.calculateSimilarity(protectedOriginal, translatedContent)
			// 相似度阈值 - 对于学术论文等包含大量术语的文本，使用较高的阈值
			similarityThreshold := 0.95 // 只有几乎完全一样时才认为失败
			
			// 还原保护的内容
			restoredText := preserveManager.Restore(translatedContent)
			
				if similarity >= similarityThreshold {
				// 翻译结果与原文太相似，视为失败
				node.Status = document.NodeStatusFailed
				node.Error = fmt.Errorf("translation too similar to original (similarity: %.2f)", similarity)
				node.RetryCount++
				
				if bt.config.Verbose {
					bt.logger.Warn("translation quality check failed",
						zap.Int("nodeID", node.ID),
						zap.Float64("similarity", similarity),
						zap.Float64("threshold", similarityThreshold),
						zap.String("protectedOriginal", truncateText(protectedOriginal, 100)),
						zap.String("translatedContent", truncateText(translatedContent, 100)),
						zap.String("restoredText", truncateText(restoredText, 100)))
				} else {
					bt.logger.Warn("translation quality check failed",
						zap.Int("nodeID", node.ID),
						zap.Float64("similarity", similarity),
						zap.Float64("threshold", similarityThreshold))
				}
			} else {
				// 翻译成功
				node.TranslatedText = restoredText
				node.Status = document.NodeStatusSuccess
				node.Error = nil
				// 增加重试计数
				node.RetryCount++
				
				// 在 verbose 模式下显示成功翻译的片段
				if bt.config.Verbose {
					bt.logger.Info("translation success",
						zap.Int("nodeID", node.ID),
						zap.String("original", truncateText(node.OriginalText, 100)),
						zap.String("translated", truncateText(restoredText, 100)))
				}
			}
		} else {
			node.Status = document.NodeStatusFailed
			node.Error = fmt.Errorf("translation not found in batch result for node %d", node.ID)
			node.RetryCount++
			
			if bt.config.Verbose {
				// 尝试查找可能被修改的节点标记
				alternativePatterns := []string{
					fmt.Sprintf("NODE_START_%d", node.ID),
					fmt.Sprintf("@@NODE_%d@@", node.ID),
					fmt.Sprintf("NODE %d:", node.ID),
					fmt.Sprintf("<%d>", node.ID),
				}
				
				foundAlternative := false
				for _, pattern := range alternativePatterns {
					if strings.Contains(translatedText, pattern) {
						foundAlternative = true
						bt.logger.Warn("found alternative node marker",
							zap.Int("nodeID", node.ID),
							zap.String("pattern", pattern))
						break
					}
				}
				
				bt.logger.Info("node translation not found",
					zap.Int("nodeID", node.ID),
					zap.String("originalText", truncateText(node.OriginalText, 100)),
					zap.Bool("isContext", isContext),
					zap.Bool("foundAlternative", foundAlternative))
			} else {
				bt.logger.Debug("node translation not found",
					zap.Int("nodeID", node.ID))
			}
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

// calculateSimilarity 计算两个文本的相似度（使用编辑距离）
func (bt *BatchTranslator) calculateSimilarity(text1, text2 string) float64 {
	if text1 == "" && text2 == "" {
		return 1.0
	}
	
	if text1 == "" || text2 == "" {
		return 0.0
	}
	
	// 简单的编辑距离实现
	len1 := len([]rune(text1))
	len2 := len([]rune(text2))
	
	// 创建距离矩阵
	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
	}
	
	// 初始化第一行和第一列
	for i := 0; i <= len1; i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len2; j++ {
		matrix[0][j] = j
	}
	
	// 计算编辑距离
	runes1 := []rune(text1)
	runes2 := []rune(text2)
	
	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 0
			if runes1[i-1] != runes2[j-1] {
				cost = 1
			}
			
			matrix[i][j] = min(
				matrix[i-1][j]+1,     // 删除
				matrix[i][j-1]+1,     // 插入
				matrix[i-1][j-1]+cost, // 替换
			)
		}
	}
	
	// 计算相似度
	distance := matrix[len1][len2]
	maxLen := max(len1, len2)
	if maxLen == 0 {
		return 1.0
	}
	
	return 1.0 - float64(distance)/float64(maxLen)
}

// truncateText 截断文本用于日志显示
func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "..."
}

// min 返回三个整数中的最小值
func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// max 返回两个整数中的最大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

