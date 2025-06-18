package translator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dlclark/regexp2"
	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/stats"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"go.uber.org/zap"
)

// BatchTranslator 实现Translator接口，负责节点分组和并行翻译
type BatchTranslator struct {
	config             TranslatorConfig
	translationService translation.Service
	logger             *zap.Logger
	preserveManager    *translation.PreserveManager
	smartSplitter      *translation.SmartNodeSplitter // 智能节点分割器
	statsManager       *stats.StatsManager            // 统计管理器

	// 详细翻译过程跟踪
	translationRounds []*TranslationRoundResult // 每轮翻译的详细结果
	mu                sync.Mutex                // 保护translationRounds的并发访问

	// 进度回调
	progressCallback ProgressCallback // 进度回调函数
}

// NewBatchTranslator 创建批量翻译器
func NewBatchTranslator(cfg TranslatorConfig, service translation.Service, logger *zap.Logger, statsManager *stats.StatsManager) *BatchTranslator {
	return &BatchTranslator{
		config:             cfg,
		translationService: service,
		logger:             logger,
		preserveManager:    translation.NewPreserveManager(translation.DefaultPreserveConfig),
		smartSplitter:      translation.NewSmartNodeSplitter(cfg.SmartSplitter, logger),
		statsManager:       statsManager,
		translationRounds:  make([]*TranslationRoundResult, 0),
	}
}

// SetProgressCallback 设置进度回调函数
func (bt *BatchTranslator) SetProgressCallback(callback ProgressCallback) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.progressCallback = callback
}

// callProgressCallback 安全地调用进度回调
func (bt *BatchTranslator) callProgressCallback(completed, total int, message string) {
	bt.mu.Lock()
	callback := bt.progressCallback
	bt.mu.Unlock()

	if callback != nil {
		callback(completed, total, message)
	}
}

// TranslateNodes 翻译所有节点（并行版本，包含失败重试）
func (bt *BatchTranslator) TranslateNodes(ctx context.Context, nodes []*document.NodeInfo) error {
	// 重置翻译轮次记录
	bt.mu.Lock()
	bt.translationRounds = make([]*TranslationRoundResult, 0)
	bt.mu.Unlock()

	// 批量翻译所有节点
	bt.logger.Info("starting batch translation", zap.Int("totalNodes", len(nodes)))

	// 第一轮：分组翻译所有节点
	groups := bt.groupNodes(nodes)
	bt.logger.Debug("initial grouping for translation",
		zap.Int("totalGroups", len(groups)),
		zap.Int("concurrency", bt.config.Concurrency))

	// 并行处理第一轮翻译
	initialRoundStart := time.Now()
	bt.callProgressCallback(0, len(nodes), "第1轮：初始翻译")
	bt.processGroups(ctx, groups)
	initialRoundDuration := time.Since(initialRoundStart)

	// 统计第一轮成功的节点数
	successCount := 0
	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess {
			successCount++
		}
	}
	bt.callProgressCallback(successCount, len(nodes), fmt.Sprintf("第1轮完成：成功 %d/%d", successCount, len(nodes)))

	// 记录第一轮翻译结果
	bt.recordTranslationRound(1, "initial", len(nodes), nodes, initialRoundDuration)

	// 检查是否启用失败重试功能
	if !bt.config.RetryOnFailure {
		bt.logger.Info("retry disabled by configuration",
			zap.String("retryOnFailure", "false"))

		// 统计最终结果但不重试
		successCount := 0
		failedCount := 0
		for _, node := range nodes {
			if node.Status == document.NodeStatusSuccess {
				successCount++
			} else {
				failedCount++
			}
		}

		bt.logger.Info("translation completed without retry",
			zap.Int("completed", successCount),
			zap.Int("failed", failedCount),
			zap.Int("total", len(nodes)),
			zap.Float64("progress", float64(successCount)/float64(len(nodes))*100))

		return nil
	}

	// 收集失败节点并进行重试
	maxRetries := bt.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	// 统计初始翻译结果
	initialSuccessCount := 0
	initialFailedCount := 0
	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess {
			initialSuccessCount++
		} else {
			initialFailedCount++
		}
	}

	bt.logger.Info("initial translation round completed",
		zap.Int("successful", initialSuccessCount),
		zap.Int("failed", initialFailedCount),
		zap.Int("total", len(nodes)),
		zap.Float64("successRate", float64(initialSuccessCount)/float64(len(nodes))*100))

	bt.logger.Info("file-level retry mechanism enabled",
		zap.Int("maxRetries", maxRetries),
		zap.Bool("retryOnFailure", bt.config.RetryOnFailure))

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

		// 检查是否有可重试的失败节点
		retryableNodes := 0
		nonRetryableNodes := 0
		for _, node := range failedNodes {
			if node.Error != nil {
				if transErr, ok := node.Error.(*translation.TranslationError); ok {
					if transErr.IsRetryable() {
						retryableNodes++
					} else {
						nonRetryableNodes++
					}
				} else {
					// 非TranslationError默认作为可重试处理
					retryableNodes++
				}
			} else {
				retryableNodes++
			}
		}

		// 如果没有可重试的节点，停止重试
		if retryableNodes == 0 {
			bt.logger.Warn("no retryable nodes found, stopping retry",
				zap.Int("retryRound", retry),
				zap.Int("totalFailedNodes", len(failedNodes)),
				zap.Int("nonRetryableNodes", nonRetryableNodes),
				zap.String("reason", "all failed nodes have non-retryable errors"))
			break
		}

		bt.logger.Info("retryable nodes analysis",
			zap.Int("retryRound", retry),
			zap.Int("totalFailedNodes", len(failedNodes)),
			zap.Int("retryableNodes", retryableNodes),
			zap.Int("nonRetryableNodes", nonRetryableNodes))

		// 使用INFO级别记录重试开始信息
		bt.logger.Info("starting retry for failed nodes",
			zap.Int("retryRound", retry),
			zap.Int("maxRetries", maxRetries),
			zap.Int("failedNodes", len(failedNodes)))

		// 通知开始重试
		bt.callProgressCallback(0, len(failedNodes), fmt.Sprintf("第%d轮：重试 %d 个失败节点", retry+1, len(failedNodes)))

		bt.logger.Debug("collecting failed nodes for retry details",
			zap.Int("retryRound", retry),
			zap.Int("failedNodes", len(failedNodes)))

		// 为失败节点添加上下文并重新分组
		retryGroups := bt.groupFailedNodesWithContext(nodes, failedNodes, processedNodes)

		if len(retryGroups) == 0 {
			bt.logger.Warn("no retry groups created, stopping retry",
				zap.Int("retryRound", retry),
				zap.Int("failedNodes", len(failedNodes)),
				zap.String("reason", "failed to create retry groups with context"))
			break
		}

		totalRetryNodes := 0
		for _, group := range retryGroups {
			totalRetryNodes += len(group.Nodes)
		}

		bt.logger.Debug("retry grouping with context",
			zap.Int("retryRound", retry),
			zap.Int("failedNodes", len(failedNodes)),
			zap.Int("retryGroups", len(retryGroups)),
			zap.Int("totalNodesWithContext", totalRetryNodes))

		// 并行处理重试组
		retryRoundStart := time.Now()
		bt.processGroups(ctx, retryGroups)
		retryRoundDuration := time.Since(retryRoundStart)

		// 记录重试轮次结果
		bt.recordTranslationRound(retry+1, "retry", len(failedNodes), failedNodes, retryRoundDuration)

		// 统计重试结果
		retrySuccessCount := 0
		retryFailedCount := 0
		for _, failed := range failedNodes {
			if failed.Status == document.NodeStatusSuccess {
				retrySuccessCount++
			} else {
				retryFailedCount++
			}
		}

		// 记录重试结果
		bt.logger.Info("retry round completed",
			zap.Int("retryRound", retry),
			zap.Int("originalFailedNodes", len(failedNodes)),
			zap.Int("nowSuccessful", retrySuccessCount),
			zap.Int("stillFailed", retryFailedCount))

		// 更新进度：计算总的成功节点数
		totalSuccessCount := 0
		for _, node := range nodes {
			if node.Status == document.NodeStatusSuccess {
				totalSuccessCount++
			}
		}
		bt.callProgressCallback(totalSuccessCount, len(nodes),
			fmt.Sprintf("第%d轮完成：本轮成功 %d，总成功 %d/%d", retry+1, retrySuccessCount, totalSuccessCount, len(nodes)))

		// 更新已处理节点集合
		for _, node := range nodes {
			if node.Status == document.NodeStatusSuccess && !processedNodes[node.ID] {
				processedNodes[node.ID] = true
			}
		}
	}

	// 记录最终统计
	successCount = 0
	failedCount := 0
	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess {
			successCount++
		} else {
			failedCount++
		}
	}

	// 计算进度百分比
	progressPercent := float64(successCount) / float64(len(nodes)) * 100

	// 最终进度更新
	if failedCount == 0 {
		bt.callProgressCallback(successCount, len(nodes), "翻译完成：全部成功")
	} else {
		bt.callProgressCallback(successCount, len(nodes), fmt.Sprintf("翻译完成：%d 成功，%d 失败", successCount, failedCount))
	}

	// 使用 INFO 级别显示翻译完成信息（包含重试信息）
	bt.logger.Info("translation completed",
		zap.Int("completed", successCount),
		zap.Int("failed", failedCount),
		zap.Int("total", len(nodes)),
		zap.Float64("progress", progressPercent),
		zap.Int("maxRetries", maxRetries),
		zap.String("retryStatus", func() string {
			if failedCount > 0 {
				return "some nodes failed after retries"
			}
			return "all retries successful"
		}()))

	// 显示统计表格（如果配置了统计管理器）
	if bt.statsManager != nil && bt.config.ShowStatsTable {
		bt.statsManager.PrintStatsTable()
	}

	return nil
}

// processGroups 并行处理节点组
func (bt *BatchTranslator) processGroups(ctx context.Context, groups []*document.NodeGroup) {
	concurrency := bt.config.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}

	// 创建工作队列和进度追踪
	groupChan := make(chan *document.NodeGroup, len(groups))
	errChan := make(chan error, len(groups))
	progressChan := make(chan int, len(groups))

	// 计算总节点数
	totalNodes := 0
	for _, group := range groups {
		totalNodes += len(group.Nodes)
	}

	// 将所有组放入队列
	for _, group := range groups {
		groupChan <- group
	}
	close(groupChan)

	// 启动进度监控 goroutine
	processedGroups := 0
	processedNodes := 0
	go func() {
		for completedInGroup := range progressChan {
			processedGroups++
			processedNodes += completedInGroup
			progress := float64(processedGroups) / float64(len(groups)) * 100

			// 调用进度回调
			bt.callProgressCallback(processedNodes, totalNodes,
				fmt.Sprintf("处理中：%d/%d 组，%d/%d 节点", processedGroups, len(groups), processedNodes, totalNodes))

			bt.logger.Debug("translation progress",
				zap.Int("completed_groups", processedGroups),
				zap.Int("total_groups", len(groups)),
				zap.Int("completed_nodes", processedNodes),
				zap.Int("total_nodes", totalNodes),
				zap.Int("nodes_in_current_group", completedInGroup),
				zap.Float64("progress", progress))
		}
	}()

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
					// 提取详细错误信息
					var detailedError string
					var errorType string
					var isRetryable bool

					if transErr, ok := err.(*translation.TranslationError); ok {
						detailedError = transErr.Error()
						errorType = transErr.Code
						isRetryable = transErr.IsRetryable()
						if transErr.Cause != nil {
							detailedError += " (cause: " + transErr.Cause.Error() + ")"
						}
					} else {
						detailedError = err.Error()
						errorType = "UNKNOWN_ERROR"
					}

					bt.logger.Error("group translation failed",
						zap.Int("workerID", workerID),
						zap.Int("groupSize", len(group.Nodes)),
						zap.String("errorType", errorType),
						zap.String("detailedError", detailedError),
						zap.Bool("retryable", isRetryable))
					errChan <- err
				}

				// 发送进度更新
				progressChan <- len(group.Nodes)
			}
		}(i)
	}

	// 等待所有工作完成
	wg.Wait()
	close(errChan)
	close(progressChan)
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

	for _, node := range group.Nodes {
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
			if builder.Len() > 0 {
				builder.WriteString("\n\n")
			}
			builder.WriteString(fmt.Sprintf("@@NODE_START_%d@@\n", node.ID))
			builder.WriteString(node.TranslatedText)
			builder.WriteString(fmt.Sprintf("\n@@NODE_END_%d@@", node.ID))
		} else {
			// 保护内容
			protectedText := bt.protectContent(node.OriginalText, preserveManager)
			protectedTexts[node.ID] = protectedText

			// 检查是否是纯保护内容
			if bt.isOnlyProtectedContent(protectedText, preserveManager) {
				// 纯保护内容节点，直接标记为成功并使用原文
				node.Status = document.NodeStatusSuccess
				node.TranslatedText = node.OriginalText
				
				bt.logger.Debug("skipping protected-only node - not included in translation request",
					zap.Int("nodeID", node.ID),
					zap.String("content", truncateText(node.OriginalText, 100)))
				
				// 不添加到 combinedText 中，完全跳过
			} else {
				// 需要翻译的节点
				needsTranslation = true

				// 在详细模式下记录保护信息
				if bt.config.Verbose && protectedText != node.OriginalText {
					bt.logger.Debug("content protection applied",
						zap.Int("nodeID", node.ID),
						zap.String("originalLength", fmt.Sprintf("%d chars", len(node.OriginalText))),
						zap.String("protectedLength", fmt.Sprintf("%d chars", len(protectedText))),
						zap.String("originalSample", truncateText(node.OriginalText, 100)),
						zap.String("protectedSample", truncateText(protectedText, 100)))
				}

				// 只有需要翻译的节点才添加到 combinedText
				if builder.Len() > 0 {
					builder.WriteString("\n\n")
				}
				builder.WriteString(fmt.Sprintf("@@NODE_START_%d@@\n", node.ID))
				builder.WriteString(protectedText)
				builder.WriteString(fmt.Sprintf("\n@@NODE_END_%d@@", node.ID))
			}
		}
	}

	// 如果所有节点都是上下文节点或保护内容，跳过翻译
	if !needsTranslation {
		bt.logger.Debug("skipping group - all nodes are context or protected-only",
			zap.Int("groupSize", len(group.Nodes)))
		return nil
	}

	combinedText := builder.String()

	// 检查最终的组合文本是否为空（所有节点都被跳过了）
	if strings.TrimSpace(combinedText) == "" {
		bt.logger.Debug("skipping group - final combinedText is empty after selective filtering",
			zap.Int("groupSize", len(group.Nodes)),
			zap.Bool("needsTranslation", needsTranslation))
		return nil
	}

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

	// 记录准备翻译的节点信息
	bt.logger.Info("preparing batch translation request",
		zap.Ints("inputNodeIDs", nodeIDsToTranslate),
		zap.Int("totalNodes", len(group.Nodes)),
		zap.Int("nodesToTranslate", nodesToTranslate),
		zap.Int("contextNodes", contextNodes),
		zap.Int("textLength", len(combinedText)))

	// 在发送前记录详细的请求内容
	bt.logger.Debug("sending batch translation request",
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
		bt.logger.Debug("node markers in request",
			zap.Strings("markers", nodeMarkers))
	}

	// 执行翻译 - 使用简化的接口，无分块
	startTime := time.Now()
	translatedText, err := bt.translationService.TranslateText(ctx, combinedText)
	latency := time.Since(startTime)
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
	bt.logger.Debug("received translation response",
		zap.Int("responseLength", len(translatedText)),
		zap.String("responsePreview", truncateText(translatedText, 500)))
	// 检查响应格式
	isJSON := strings.HasPrefix(strings.TrimSpace(translatedText), "{") || strings.HasPrefix(strings.TrimSpace(translatedText), "[")
	isEmpty := len(strings.TrimSpace(translatedText)) == 0

	// 检查响应是否包含节点标记
	hasStartMarkers := strings.Contains(translatedText, "@@NODE_START_")
	hasEndMarkers := strings.Contains(translatedText, "@@NODE_END_")

	// 根据节点标记存在情况选择日志级别
	if !hasStartMarkers || !hasEndMarkers {
		bt.logger.Warn("response format check - missing node markers",
			zap.Bool("hasStartMarkers", hasStartMarkers),
			zap.Bool("hasEndMarkers", hasEndMarkers),
			zap.Bool("isJSON", isJSON),
			zap.Bool("isEmpty", isEmpty),
			zap.Int("responseLength", len(translatedText)))
	} else {
		bt.logger.Debug("response format check - markers found",
			zap.Bool("hasStartMarkers", hasStartMarkers),
			zap.Bool("hasEndMarkers", hasEndMarkers))
	}

	// 如果响应看起来像 JSON，尝试提取其中的文本
	if isJSON && !hasStartMarkers {
		bt.logger.Warn("response appears to be JSON without node markers",
			zap.String("jsonPreview", truncateText(translatedText, 200)))
	}

	// 如果没有找到节点标记，运行诊断并尝试重试
	if !hasStartMarkers || !hasEndMarkers {
		diagnostic := DiagnoseBatchTranslationIssue(combinedText, translatedText)
		bt.logger.Warn("batch translation diagnostic",
			zap.String("diagnostic", diagnostic.Format()))

		// 检查是否可以重试（避免无限重试）
		maxNodeMarkerRetries := 2
		currentRetries := 0
		if context, ok := ctx.Value("node_marker_retries").(int); ok {
			currentRetries = context
		}

		if currentRetries < maxNodeMarkerRetries {
			bt.logger.Warn("node markers missing, attempting retry with enhanced prompt",
				zap.Int("currentRetry", currentRetries),
				zap.Int("maxRetries", maxNodeMarkerRetries))

			// 创建新的上下文，增加重试计数
			newCtx := context.WithValue(ctx, "node_marker_retries", currentRetries+1)

			// 使用增强的提示词重试
			enhancedRequest := bt.buildEnhancedNodeMarkerRequest(combinedText)
			retryResponseText, retryErr := bt.translationService.TranslateText(newCtx, enhancedRequest)

			if retryErr == nil && retryResponseText != "" {
				bt.logger.Info("retry with enhanced prompt completed",
					zap.Int("retryAttempt", currentRetries+1),
					zap.Int("responseLength", len(retryResponseText)))

				// 检查重试的响应是否包含标记
				retryHasMarkers := strings.Contains(retryResponseText, "@@NODE_START_") &&
					strings.Contains(retryResponseText, "@@NODE_END_")

				if retryHasMarkers {
					bt.logger.Info("enhanced prompt retry successful - node markers found")
					// 使用重试的响应替换原始响应
					translatedText = retryResponseText
					hasStartMarkers = true
					hasEndMarkers = true
				} else {
					bt.logger.Warn("enhanced prompt retry failed - still no node markers")
				}
			} else {
				bt.logger.Error("enhanced prompt retry failed",
					zap.Error(retryErr))
			}
		} else {
			bt.logger.Error("node marker retry limit exceeded",
				zap.Int("maxRetries", maxNodeMarkerRetries),
				zap.String("issue", "LLM consistently ignores node markers"))
		}
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
		bt.logger.Debug("node markers in response",
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
			bt.logger.Debug("response structure",
				zap.Strings("firstLines", firstLines),
				zap.Strings("lastLines", lastLines))
		}
	}

	// 解析翻译结果 - 使用更强健的正则表达式处理不同换行符和空格
	pattern := regexp2.MustCompile(`(?s)@@NODE_START_(\d+)@@\s*\r?\n(.*?)\r?\n\s*@@NODE_END_\1@@`, 0)

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
		bt.logger.Debug("simple pattern match test",
			zap.Int("simpleMatchCount", simpleMatchCount))

		// 额外调试：检查响应中的换行符类型
		bt.logger.Debug("response newline analysis",
			zap.Bool("containsCRLF", strings.Contains(translatedText, "\r\n")),
			zap.Bool("containsLF", strings.Contains(translatedText, "\n")),
			zap.Bool("containsCR", strings.Contains(translatedText, "\r")))
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
			bt.logger.Debug("regex match details",
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
				bt.logger.Debug("parsed node translation",
					zap.Int("nodeID", nodeID),
					zap.String("contentPreview", truncateText(content, 100)))
			}
		}
		match, _ = pattern.FindNextMatch(match)
	}

	// 重用之前计算的nodeIDsToTranslate作为inputNodeIDs
	inputNodeIDs := nodeIDsToTranslate

	// 记录期望但未找到的节点
	var missingNodeIDs []int
	for _, nodeID := range inputNodeIDs {
		if _, found := translationMap[nodeID]; !found {
			missingNodeIDs = append(missingNodeIDs, nodeID)
		}
	}

	// 计算解析成功率
	successRate := float64(len(foundNodeIDs)) / float64(len(inputNodeIDs)) * 100

	// 根据结果选择合适的日志级别
	if len(missingNodeIDs) > 0 {
		bt.logger.Warn("batch translation parsing results",
			zap.Ints("inputNodeIDs", inputNodeIDs),
			zap.Ints("foundNodeIDs", foundNodeIDs),
			zap.Ints("missingNodeIDs", missingNodeIDs),
			zap.Int("inputCount", len(inputNodeIDs)),
			zap.Int("foundCount", len(foundNodeIDs)),
			zap.Int("missingCount", len(missingNodeIDs)),
			zap.Float64("successRate", successRate),
			zap.Int("responseLength", len(translatedText)))
	} else {
		bt.logger.Info("batch translation parsing successful",
			zap.Ints("inputNodeIDs", inputNodeIDs),
			zap.Ints("foundNodeIDs", foundNodeIDs),
			zap.Int("nodeCount", len(foundNodeIDs)),
			zap.Float64("successRate", successRate))
	}

	if bt.config.Verbose {
		// 显示翻译片段（清理推理标记后显示）
		if matchCount > 0 {
			cleanedText := translation.RemoveReasoningMarkers(translatedText)
			bt.logger.Debug("translation snippets",
				zap.Int("totalMatches", matchCount),
				zap.String("firstSnippet", truncateText(cleanedText, 300)))
		}

		// 详细的解析统计
		bt.logger.Debug("detailed parsing stats",
			zap.Int("totalNodes", len(group.Nodes)),
			zap.Int("nodesToTranslate", nodesToTranslate),
			zap.Int("contextNodes", contextNodes),
			zap.Int("translatedTextLength", len(translatedText)))
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

			// 检查翻译质量 - 去掉保护符号后比较原文和译文
			cleanOriginal := preserveManager.RemoveProtectionMarkers(node.OriginalText)
			cleanTranslated := preserveManager.RemoveProtectionMarkers(restoredText)

			// 如果去掉保护符号后内容太短，跳过相似度检查
			cleanOriginalLen := len(strings.TrimSpace(cleanOriginal))
			cleanTranslatedLen := len(strings.TrimSpace(cleanTranslated))
			minLengthForCheck := 20 // 少于20字符的内容不检查相似度

			shouldCheckSimilarity := cleanOriginalLen >= minLengthForCheck && cleanTranslatedLen >= minLengthForCheck

			var similarity float64
			if shouldCheckSimilarity {
				similarity = bt.calculateSimilarity(cleanOriginal, cleanTranslated)
			} else {
				similarity = 0.0 // 短内容默认通过检查
			}

			// 相似度阈值 - 对于学术论文等包含大量术语的文本，使用较高的阈值
			similarityThreshold := 0.95 // 只有几乎完全一样时才认为失败

			if shouldCheckSimilarity && similarity >= similarityThreshold {
				// 翻译结果与原文太相似，视为失败
				node.Status = document.NodeStatusFailed
				node.Error = fmt.Errorf("translation too similar to original (similarity: %.2f)", similarity)
				node.RetryCount++

				if bt.config.Verbose {
					bt.logger.Warn("translation quality check failed",
						zap.Int("nodeID", node.ID),
						zap.Float64("similarity", similarity),
						zap.Float64("threshold", similarityThreshold),
						zap.Int("cleanOriginalLen", cleanOriginalLen),
						zap.Int("cleanTranslatedLen", cleanTranslatedLen),
						zap.String("cleanOriginal", truncateText(cleanOriginal, 100)),
						zap.String("cleanTranslated", truncateText(cleanTranslated, 100)),
						zap.String("restoredText", truncateText(restoredText, 100)))
				} else {
					bt.logger.Warn("translation quality check failed",
						zap.Int("nodeID", node.ID),
						zap.Float64("similarity", similarity),
						zap.Float64("threshold", similarityThreshold),
						zap.Int("cleanOriginalLen", cleanOriginalLen),
						zap.Int("cleanTranslatedLen", cleanTranslatedLen))
				}
			} else {
				// 翻译成功
				// 移除推理模型的思考标记
				finalText := translation.RemoveReasoningMarkers(restoredText)
				node.TranslatedText = finalText
				node.Status = document.NodeStatusSuccess
				node.Error = nil
				// 增加重试计数
				node.RetryCount++

				// 在 verbose 模式下显示成功翻译的片段
				if bt.config.Verbose {
					bt.logger.Debug("translation success",
						zap.Int("nodeID", node.ID),
						zap.String("original", truncateText(node.OriginalText, 100)),
						zap.String("translated", truncateText(finalText, 100)))
				}
			}
		} else {
			node.Status = document.NodeStatusFailed
			node.Error = fmt.Errorf("translation not found in batch result for node %d", node.ID)
			node.RetryCount++

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

			// 节点翻译失败是重要信息，使用WARN级别
			bt.logger.Warn("node translation not found",
				zap.Int("nodeID", node.ID),
				zap.String("originalText", truncateText(node.OriginalText, 100)),
				zap.Bool("isContext", isContext),
				zap.Bool("foundAlternative", foundAlternative))
		}
	}

	// 记录Provider性能统计
	if bt.statsManager != nil {
		bt.recordGroupStats(ctx, group, nodeIDsToTranslate, err, latency)
	}

	return nil
}

// recordGroupStats 记录组翻译统计信息
func (bt *BatchTranslator) recordGroupStats(ctx context.Context, group *document.NodeGroup, nodeIDsToTranslate []int, translationErr error, latency time.Duration) {
	if bt.statsManager == nil {
		return
	}

	// 默认使用通用Provider名称（实际应用中可以从配置或service中获取）
	providerName := "unknown"
	modelName := "unknown"

	// 计算节点标记统计
	expectedMarkers := len(nodeIDsToTranslate)
	actualMarkers := 0
	lostMarkers := 0

	// 计算成功和失败节点
	successfulNodes := 0
	failedNodes := 0
	hasSimilarityIssues := false
	hasFormatIssues := false
	hasReasoningTags := false

	for _, node := range group.Nodes {
		if node.Status == document.NodeStatusSuccess {
			successfulNodes++
			// 检查节点标记
			if strings.Contains(node.TranslatedText, fmt.Sprintf("@@NODE_START_%d@@", node.ID)) {
				actualMarkers++
			}
			// 检查是否有推理标记
			if translation.HasReasoningTags(node.TranslatedText) {
				hasReasoningTags = true
			}
		} else {
			failedNodes++
			// 检查失败原因
			if node.Error != nil {
				errorStr := strings.ToLower(node.Error.Error())
				if strings.Contains(errorStr, "similarity") {
					hasSimilarityIssues = true
				}
			}
		}
	}

	lostMarkers = expectedMarkers - actualMarkers

	// 创建统计结果
	result := stats.RequestResult{
		Success:           translationErr == nil && failedNodes == 0,
		Latency:           latency,
		NodeMarkersFound:  actualMarkers,
		NodeMarkersLost:   lostMarkers,
		HasFormatIssues:   hasFormatIssues,
		HasReasoningTags:  hasReasoningTags,
		SimilarityTooHigh: hasSimilarityIssues,
		IsRetry:           false, // TODO: 可以从上下文中获取
	}

	if translationErr != nil {
		result.ErrorType = bt.classifyTranslationError(translationErr)
	}

	// 记录统计
	bt.statsManager.RecordRequest(providerName, modelName, result)

	bt.logger.Debug("recorded translation statistics",
		zap.String("provider", providerName),
		zap.String("model", modelName),
		zap.Bool("success", result.Success),
		zap.Duration("latency", latency),
		zap.Int("expectedMarkers", expectedMarkers),
		zap.Int("actualMarkers", actualMarkers),
		zap.Int("lostMarkers", lostMarkers))
}

// classifyTranslationError 分类翻译错误
func (bt *BatchTranslator) classifyTranslationError(err error) string {
	if err == nil {
		return ""
	}

	errorStr := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errorStr, "timeout"):
		return "timeout"
	case strings.Contains(errorStr, "rate limit"):
		return "rate_limit"
	case strings.Contains(errorStr, "context"):
		return "context_error"
	case strings.Contains(errorStr, "network"):
		return "network_error"
	case strings.Contains(errorStr, "auth"):
		return "auth_error"
	case strings.Contains(errorStr, "quota"):
		return "quota_exceeded"
	default:
		return "unknown_error"
	}
}

// protectContent 保护不需要翻译的内容
func (bt *BatchTranslator) protectContent(text string, pm *translation.PreserveManager) string {
	// LaTeX 公式 - 使用更精确的正则表达式
	text = pm.ProtectPattern(text, `\$[^$\n]+\$`)      // 行内公式 (不包含换行)
	text = pm.ProtectPattern(text, `\$\$[\s\S]*?\$\$`) // 行间公式 (非贪婪匹配)
	text = pm.ProtectPattern(text, `\\\([\s\S]*?\\\)`) // \(...\) (非贪婪匹配)
	text = pm.ProtectPattern(text, `\\\[[\s\S]*?\\\]`) // \[...\] (非贪婪匹配)

	// 代码块
	text = pm.ProtectPattern(text, "`[^`]+`") // 行内代码
	text = protectCodeBlocks(text, pm)        // 多行代码块

	// HTML 标签
	text = pm.ProtectPattern(text, `<[^>]+>`)     // HTML 标签
	text = pm.ProtectPattern(text, `&[a-zA-Z]+;`) // HTML 实体
	text = pm.ProtectPattern(text, `&#\d+;`)      // 数字实体

	// URL
	text = pm.ProtectPattern(text, `(?i)(https?|ftp|file)://[^\s\)]+`)
	text = pm.ProtectPattern(text, `(?i)www\.[^\s\)]+`)

	// 文件路径
	text = pm.ProtectPattern(text, `(?:^|[\s(])/(?:[^/\s]+/)*[^/\s]+(?:\.[a-zA-Z0-9]+)?`)
	text = pm.ProtectPattern(text, `[A-Za-z]:\\(?:[^\\/:*?"<>|\r\n]+\\)*[^\\/:*?"<>|\r\n]+`)
	text = pm.ProtectPattern(text, `\.{1,2}/(?:[^/\s]+/)*[^/\s]+(?:\.[a-zA-Z0-9]+)?`)

	// Markdown 图片和链接
	text = pm.ProtectPattern(text, `!\[[^\]]*\]\([^)]+\)`)      // ![alt text](image url)
	text = pm.ProtectPattern(text, `\[[^\]]+\]\([^)]+\)`)       // [link text](url)
	text = pm.ProtectPattern(text, `\[[^\]]+\]\[[^\]]*\]`)      // [link text][ref]
	text = pm.ProtectPattern(text, `(?m)^\s*\[[^\]]+\]:\s*.+$`) // [ref]: url (引用定义，多行模式)

	// 引用标记
	text = pm.ProtectPattern(text, `\[\d+\]`)                                 // [1], [2]
	text = pm.ProtectPattern(text, `\[[A-Za-z]+(?:\s+et\s+al\.)?,\s*\d{4}\]`) // [Author, Year]
	text = pm.ProtectPattern(text, `\\cite\{[^}]+\}`)                         // \cite{}
	text = pm.ProtectPattern(text, `\\ref\{[^}]+\}`)                          // \ref{}
	text = pm.ProtectPattern(text, `\\label\{[^}]+\}`)                        // \label{}

	// 其他
	text = pm.ProtectPattern(text, `\{\{[^}]+\}\}`)                                  // {{variable}}
	text = pm.ProtectPattern(text, `<%[^%]+%>`)                                      // <% %>
	text = pm.ProtectPattern(text, `<!--[\s\S]*?-->`)                                // <!-- -->
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

// groupNodes 将节点分组（支持智能分割）
func (bt *BatchTranslator) groupNodes(nodes []*document.NodeInfo) []*document.NodeGroup {
	// 第一步：智能分割超大节点
	processedNodes := bt.preprocessNodesWithSplitting(nodes)

	// 第二步：进行常规分组
	var groups []*document.NodeGroup
	var currentGroup []*document.NodeInfo
	currentSize := 0

	maxSize := bt.config.ChunkSize
	if maxSize <= 0 {
		maxSize = 1000
	}

	for _, node := range processedNodes {
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

// preprocessNodesWithSplitting 预处理节点，对超大节点进行智能分割
func (bt *BatchTranslator) preprocessNodesWithSplitting(nodes []*document.NodeInfo) []*document.NodeInfo {
	if !bt.config.SmartSplitter.EnableSmartSplitting {
		// 智能分割未启用，直接返回原节点
		return nodes
	}

	var processedNodes []*document.NodeInfo
	nextNodeID := bt.getMaxNodeID(nodes) + 1

	splitNodeCount := 0
	originalOversizedCount := 0

	for _, node := range nodes {
		if bt.smartSplitter.ShouldSplit(node) {
			originalOversizedCount++
			bt.logger.Debug("processing oversized node for smart splitting",
				zap.Int("nodeID", node.ID),
				zap.Int("nodeSize", len(node.OriginalText)),
				zap.Int("threshold", bt.config.SmartSplitter.MaxNodeSizeThreshold))

			// 分割节点
			subNodes, err := bt.smartSplitter.SplitNode(node, &nextNodeID)
			if err != nil {
				bt.logger.Warn("failed to split oversized node, using original",
					zap.Int("nodeID", node.ID),
					zap.Error(err))
				processedNodes = append(processedNodes, node)
			} else {
				bt.logger.Info("successfully split oversized node",
					zap.Int("originalNodeID", node.ID),
					zap.Int("originalSize", len(node.OriginalText)),
					zap.Int("subNodesCount", len(subNodes)))
				processedNodes = append(processedNodes, subNodes...)
				splitNodeCount++
			}
		} else {
			// 节点大小合适，无需分割
			processedNodes = append(processedNodes, node)
		}
	}

	// 记录智能分割统计信息
	if originalOversizedCount > 0 {
		bt.logger.Info("smart node splitting completed",
			zap.Int("originalNodesCount", len(nodes)),
			zap.Int("processedNodesCount", len(processedNodes)),
			zap.Int("oversizedNodesFound", originalOversizedCount),
			zap.Int("nodesSplit", splitNodeCount),
			zap.Bool("splittingEnabled", bt.config.SmartSplitter.EnableSmartSplitting),
			zap.Int("maxSizeThreshold", bt.config.SmartSplitter.MaxNodeSizeThreshold))
	}

	return processedNodes
}

// getMaxNodeID 获取节点列表中的最大ID
func (bt *BatchTranslator) getMaxNodeID(nodes []*document.NodeInfo) int {
	maxID := 0
	for _, node := range nodes {
		if node.ID > maxID {
			maxID = node.ID
		}
	}
	return maxID
}

// buildEnhancedNodeMarkerRequest 构建增强的节点标记请求，强调标记保留
func (bt *BatchTranslator) buildEnhancedNodeMarkerRequest(originalText string) string {
	// 创建极其强调节点标记保留的提示词
	enhancedPrompt := fmt.Sprintf(`🚨🚨🚨 EMERGENCY INSTRUCTION - SYSTEM WILL FAIL WITHOUT COMPLIANCE 🚨🚨🚨

You are a translation system component. Your task is to translate text while preserving special markers.

⚠️ ABSOLUTE REQUIREMENT - NO EXCEPTIONS:
- You MUST copy every @@NODE_START_X@@ marker to your output EXACTLY as shown
- You MUST copy every @@NODE_END_X@@ marker to your output EXACTLY as shown  
- These markers are computer code - DO NOT translate them
- DO NOT remove them, modify them, or change them in any way

REQUIRED FORMAT - Your output must look EXACTLY like this:
@@NODE_START_1@@
[translated content here]
@@NODE_END_1@@

@@NODE_START_2@@  
[translated content here]
@@NODE_END_2@@

CRITICAL: If ANY marker is missing or changed, the entire system will crash and all work will be lost.

TRANSLATE FROM %s TO %s:

%s

Remember: Copy ALL @@NODE_START_X@@ and @@NODE_END_X@@ markers EXACTLY. Only translate the text between markers.`,
		bt.config.SourceLang,
		bt.config.TargetLang,
		originalText)

	return enhancedPrompt
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

		// 添加前面的上下文节点（最多1个）
		if idx > 0 {
			prevNodeID := allNodes[idx-1].ID
			// 只添加已成功翻译的节点作为上下文
			if !includeSet[prevNodeID] && processedNodes[prevNodeID] {
				includeSet[prevNodeID] = true
				contextNodeCount++
			}
		}

		// 添加后面的上下文节点（最多1个）
		if idx < len(allNodes)-1 {
			nextNodeID := allNodes[idx+1].ID
			// 只添加已成功翻译的节点作为上下文
			if !includeSet[nextNodeID] && processedNodes[nextNodeID] {
				includeSet[nextNodeID] = true
				contextNodeCount++
			}
		}
	}

	bt.logger.Debug("context nodes added for retry",
		zap.Int("contextNodes", contextNodeCount),
		zap.Int("totalNodesForRetry", len(includeSet)),
		zap.Int("failedNodesCount", len(failedNodes)),
		zap.Int("processedNodesCount", len(processedNodes)))

	// 收集所有需要翻译的节点，保持原始顺序
	var nodesToTranslate []*document.NodeInfo
	for _, node := range allNodes {
		if includeSet[node.ID] {
			// 如果是已成功的上下文节点，添加标记
			if processedNodes[node.ID] && node.Status == document.NodeStatusSuccess {
				if node.Metadata == nil {
					node.Metadata = make(map[string]interface{})
				}
				node.Metadata["is_context"] = true
			}

			// 直接使用原始节点引用，这样状态更新会反映到原始数组中
			nodesToTranslate = append(nodesToTranslate, node)
		}
	}

	// 检查是否有节点需要重试
	if len(nodesToTranslate) == 0 {
		bt.logger.Warn("no nodes collected for retry",
			zap.Int("originalFailedNodes", len(failedNodes)),
			zap.Int("includeSetSize", len(includeSet)),
			zap.String("possibleCause", "all failed nodes might have been filtered out or context collection failed"))
		return []*document.NodeGroup{}
	}

	bt.logger.Debug("nodes collected for retry",
		zap.Int("nodesToTranslate", len(nodesToTranslate)),
		zap.Int("failedNodes", len(failedNodes)),
		zap.Int("contextNodes", contextNodeCount))

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
				matrix[i-1][j]+1,      // 删除
				matrix[i][j-1]+1,      // 插入
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

// recordTranslationRound 记录单轮翻译的详细结果
func (bt *BatchTranslator) recordTranslationRound(roundNumber int, roundType string, totalNodes int, nodes []*document.NodeInfo, duration time.Duration) {
	var successNodes []int
	var failedNodes []int
	var failedDetails []*FailedNodeDetail

	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess {
			successNodes = append(successNodes, node.ID)
		} else {
			failedNodes = append(failedNodes, node.ID)

			// 收集失败节点详情，包括步骤信息
			errorType, step, stepIndex := extractErrorDetails(node.Error)
			detail := &FailedNodeDetail{
				NodeID:       node.ID,
				OriginalText: truncateText(node.OriginalText, 200),
				Path:         node.Path,
				ErrorType:    errorType,
				ErrorMessage: func() string {
					if node.Error != nil {
						return node.Error.Error()
					}
					return "unknown error"
				}(),
				Step:        step,
				StepIndex:   stepIndex,
				RetryCount:  node.RetryCount,
				FailureTime: time.Now(),
			}
			failedDetails = append(failedDetails, detail)
		}
	}

	roundResult := &TranslationRoundResult{
		RoundNumber:   roundNumber,
		RoundType:     roundType,
		TotalNodes:    totalNodes,
		SuccessNodes:  successNodes,
		FailedNodes:   failedNodes,
		SuccessCount:  len(successNodes),
		FailedCount:   len(failedNodes),
		Duration:      duration,
		FailedDetails: failedDetails,
	}

	bt.mu.Lock()
	bt.translationRounds = append(bt.translationRounds, roundResult)
	bt.mu.Unlock()

	bt.logger.Info("recorded translation round",
		zap.Int("roundNumber", roundNumber),
		zap.String("roundType", roundType),
		zap.Int("totalNodes", totalNodes),
		zap.Int("successCount", len(successNodes)),
		zap.Int("failedCount", len(failedNodes)),
		zap.Duration("duration", duration))
}

// GetDetailedTranslationSummary 获取详细的翻译汇总
func (bt *BatchTranslator) GetDetailedTranslationSummary(nodes []*document.NodeInfo) *DetailedTranslationSummary {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	// 统计最终结果
	finalSuccess := 0
	finalFailed := 0
	var finalFailedNodes []*FailedNodeDetail

	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess {
			finalSuccess++
		} else {
			finalFailed++
			errorType, step, stepIndex := extractErrorDetails(node.Error)
			detail := &FailedNodeDetail{
				NodeID:       node.ID,
				OriginalText: truncateText(node.OriginalText, 200),
				Path:         node.Path,
				ErrorType:    errorType,
				ErrorMessage: func() string {
					if node.Error != nil {
						return node.Error.Error()
					}
					return "unknown error"
				}(),
				Step:        step,
				StepIndex:   stepIndex,
				RetryCount:  node.RetryCount,
				FailureTime: time.Now(),
			}
			finalFailedNodes = append(finalFailedNodes, detail)
		}
	}

	return &DetailedTranslationSummary{
		TotalNodes:       len(nodes),
		FinalSuccess:     finalSuccess,
		FinalFailed:      finalFailed,
		TotalRounds:      len(bt.translationRounds),
		Rounds:           bt.translationRounds,
		FinalFailedNodes: finalFailedNodes,
	}
}

// isOnlyProtectedContent 检查文本是否只包含保护标记和空白字符
func (bt *BatchTranslator) isOnlyProtectedContent(text string, preserveManager *translation.PreserveManager) bool {
	// 移除所有保护标记
	cleanedText := preserveManager.RemoveProtectionMarkers(text)
	
	// 如果移除保护标记后只剩下空白字符，则认为是纯保护内容
	trimmedText := strings.TrimSpace(cleanedText)
	return trimmedText == ""
}
