package formats

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dlclark/regexp2"
	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// TextProcessor 是纯文本文件的处理器
type TextProcessor struct {
	BaseProcessor
	replacements        []ReplacementInfo
	tf                  *TextFormattingProcessor
	numNodes            int
	similarityThreshold float64
	maxSplitSize        int
	minSplitSize        int
	concurrency         int
	enableRetry         bool
	maxRetry            int
}

type TextNode struct {
	ID             int
	Text           string
	TranslatedText string
	Translated     bool
	StartMarker    string
	EndMarker      string
}

// NewTextProcessor 创建一个新的文本处理器
func NewTextProcessor(t translator.Translator, predefinedTranslations *config.PredefinedTranslation, progressBar *progress.Writer) (*TextProcessor, error) {
	// 获取logger，如果无法转换则创建新的
	zapLogger, _ := zap.NewProduction()
	if loggerProvider, ok := t.GetLogger().(interface{ GetZapLogger() *zap.Logger }); ok {
		if zl := loggerProvider.GetZapLogger(); zl != nil {
			zapLogger = zl
		}
	}
	tf := NewTextFormattingProcessor()
	currentConfig := t.GetConfig()
	zapLogger.Debug("从配置文件设置 Text Processor", zap.Int("Concurrency", currentConfig.Concurrency), zap.Int("MinSplitSize", currentConfig.MinSplitSize), zap.Int("MaxSplitSize", currentConfig.MaxSplitSize))
	return &TextProcessor{
		BaseProcessor: BaseProcessor{
			Translator:             t,
			Name:                   "文本",
			predefinedTranslations: predefinedTranslations,
			progressBar:            progressBar,
			logger:                 zapLogger,
		},
		replacements:        []ReplacementInfo{},
		tf:                  tf,
		similarityThreshold: 0.4,
		maxSplitSize:        currentConfig.MaxSplitSize,
		minSplitSize:        currentConfig.MinSplitSize,
		concurrency:         currentConfig.Concurrency,
		enableRetry:         currentConfig.RetryFailedParts,
		maxRetry:            currentConfig.MaxRetries,
	}, nil
}

// TranslateFile 翻译文本文件
func (p *TextProcessor) TranslateFile(inputPath, outputPath string) error {
	// 读取输入文件
	content, err := os.ReadFile(inputPath)
	if err != nil {
		p.logger.Error("读取文件失败", zap.Error(err), zap.String("文件路径", inputPath))
		return fmt.Errorf("读取文件失败 %s: %w", inputPath, err)
	}

	p.logger.Info("开始翻译文本文件",
		zap.String("输入文件", inputPath),
		zap.String("输出文件", outputPath),
		zap.Int("文件大小", len(content)),
	)

	translatedText, err := p.TranslateText(string(content))
	if err != nil {
		p.logger.Error("翻译文本失败", zap.Error(err))
		return fmt.Errorf("翻译文本失败: %w", err)
	}

	// 写入输出文件
	if err := os.WriteFile(outputPath, []byte(translatedText), 0o644); err != nil {
		p.logger.Error("写入文件失败", zap.Error(err), zap.String("文件路径", outputPath))
		return fmt.Errorf("写入文件失败 %s: %w", outputPath, err)
	}
	return nil
}

func (p *TextProcessor) TranslateText(text string) (string, error) {
	formattedText := p.tf.FormatText(text)

	textToTranslate, err := p.protectText(formattedText)
	if err != nil {
		p.logger.Error("保护文本失败", zap.Error(err))
		return "", err
	}

	nodes := p.splitTextToNodesWithMarkers(textToTranslate)
	p.numNodes = len(nodes)

	p.logger.Debug("TranslateText 初始化", zap.Int("节点数目", p.numNodes))

	groups := p.groupNodes(nodes)
	p.logger.Debug("TranslateText 分组数目", zap.Int("分组数目", len(groups)))
	if p.countGroupsNodes(groups) != p.numNodes {
		p.logger.Warn("分组后节点数量不一致", zap.Int("原始节点数量", p.numNodes), zap.Int("分组节点数量", p.countGroupsNodes(groups)))
		return "", fmt.Errorf("分组后节点数量不一致 %d != %d", p.numNodes, p.countGroupsNodes(groups))
	}
	p.logger.Debug("TranslateText 分组完成", zap.Int("组数", len(groups)))

	if p.concurrency <= 0 {
		p.concurrency = 4
	}

	translatedGroups := make([][]TextNode, len(groups))
	var wg sync.WaitGroup
	translatedGroupsMutex := sync.Mutex{}
	sem := make(chan struct{}, p.concurrency)
	for groupIdx, group := range groups {
		wg.Add(1)
		go func(gIdx int, currentGroup []TextNode) {
			defer wg.Done()
			sem <- struct{}{}
			translatedNodes := p.translateNodeGroup(currentGroup)
			translatedGroupsMutex.Lock()
			translatedGroups[gIdx] = translatedNodes
			translatedGroupsMutex.Unlock()
			<-sem
		}(groupIdx, group)
	}
	wg.Wait()

	translatedNodes := p.ungroupNodes(translatedGroups)
	translatedNodes = p.sortNodes(translatedNodes)

	if len(translatedNodes) != p.numNodes {
		p.logger.Warn("第一次翻译后节点数量不一致", zap.Int("原始节点数量", p.numNodes), zap.Int("翻译节点数量", len(translatedNodes)))
		return p.combineNodeGroupsText(translatedGroups), fmt.Errorf("第一次翻译后节点数量不一致")
	}

	failedNodes := p.filterFailedNodes(translatedNodes)

	if len(failedNodes) > 0 {
		p.logger.Warn("第一次翻译后有未翻译的节点", zap.Int("未翻译节点数量", len(failedNodes)))
		// return p.combineNodeGroupsText(translatedGroups), fmt.Errorf("第一次翻译后有未翻译的节点")
	}

	failedNodesWithContext := p.generateFailedNodesWithContext(translatedNodes)
	if p.enableRetry {
		for retryIdx := 0; retryIdx < p.maxRetry; retryIdx++ {
			if len(failedNodesWithContext) == 0 {
				break
			}

			p.logger.Info("重试翻译", zap.String("重试次数", fmt.Sprintf("%d/%d", retryIdx+1, p.maxRetry)), zap.Int("未翻译节点数量(包括上下文)", len(failedNodesWithContext)))

			groupFailedNodes := p.groupNodes(failedNodesWithContext)
			retryTranslatedNodes := make([]TextNode, 0)
			retryTranslatedGroupFailedNodes := make([][]TextNode, len(groupFailedNodes))
			var retryWg sync.WaitGroup
			retrySem := make(chan struct{}, p.concurrency)
			for groupIdx, group := range groupFailedNodes {
				retryWg.Add(1)
				go func(gIdx int, currentGroup []TextNode) {
					defer retryWg.Done()
					retrySem <- struct{}{}
					retryTranslatedGroupFailedNodes[gIdx] = p.translateNodeGroup(currentGroup)
					<-retrySem
				}(groupIdx, group)
			}
			retryWg.Wait()

			retryTranslatedNodes = p.ungroupNodes(retryTranslatedGroupFailedNodes)

			if len(retryTranslatedNodes) != len(failedNodesWithContext) {
				p.logger.Warn("重试后节点数量不一致", zap.Int("原始节点数量", len(failedNodesWithContext)), zap.Int("翻译节点数量", len(retryTranslatedNodes)))
				return p.combineNodeGroupsText(translatedGroups), fmt.Errorf("重试后节点数量不一致")
			}

			translatedNodes = p.mergeTranslatedFailedNodes(translatedNodes, failedNodes, retryTranslatedNodes)
			translatedNodes = p.sortNodes(translatedNodes)

			failedNodes = p.filterFailedNodes(translatedNodes)
			failedNodesWithContext = p.generateFailedNodesWithContext(translatedNodes)
			if len(failedNodes) == 0 {
				break
			}
		}
	}

	if len(failedNodes) > 0 {
		p.logger.Warn("仍有未翻译的节点", zap.Int("未翻译节点数量", len(failedNodes)))
	}

	translatedNodes = p.sortNodes(translatedNodes)
	finalTranslatedText := p.combineTranslatedNodeGroupText(translatedNodes)
	finalTranslatedText = p.tf.FormatText(finalTranslatedText)

	return finalTranslatedText, nil
}

func (p *TextProcessor) countGroupsNodes(nodes [][]TextNode) int {
	count := 0
	for _, group := range nodes {
		count += len(group)
	}
	return count
}

func (p *TextProcessor) sortNodes(nodes []TextNode) []TextNode {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	return nodes
}

func (p *TextProcessor) mergeTranslatedFailedNodes(
	allNodes []TextNode,
	failedNodes []TextNode,
	translatedFailedNodes []TextNode,
) []TextNode {
	// 构建 ID -> translated TextNode 的映射
	translatedMap := make(map[int]TextNode, len(translatedFailedNodes))
	for _, node := range translatedFailedNodes {
		translatedMap[node.ID] = node
	}

	// 遍历所有失败节点，使用映射安全取值
	for _, node := range failedNodes {
		id := node.ID
		if id < 0 || id >= p.numNodes {
			p.logger.Warn("节点索引超出范围，跳过合并", zap.Int("节点ID", id), zap.Int("总节点数", p.numNodes))
			continue
		}
		// 检查是否在重试翻译映射中
		if updated, ok := translatedMap[id]; ok {
			// 仅在未翻译状态下更新
			if !allNodes[id].Translated {
				allNodes[id].TranslatedText = updated.TranslatedText
				allNodes[id].Translated = true
			}
		} else {
			p.logger.Warn(
				"重试翻译结果中未找到对应节点",
				zap.Int("节点ID", id),
				zap.Ints("翻译结果节点ID列表", keysOf(translatedMap)),
			)
		}
	}
	return allNodes
}

func (p *TextProcessor) groupLength(nodes []TextNode) int {
	length := 0
	for _, node := range nodes {
		length += len(node.Text)
	}
	return length
}

func (p *TextProcessor) debugGroupsLength(nodes [][]TextNode) {
	p.logger.Debug("Debug分组长度")
	p.logger.Debug("分组数量", zap.Int("组数量", len(nodes)))
	for i, group := range nodes {
		p.logger.Debug("分组长度", zap.Int("组索引", i), zap.Int("组长度", p.groupLength(group)))
	}
}

func (p *TextProcessor) groupNodes(nodes []TextNode) [][]TextNode {
	groups := make([][]TextNode, 0)
	currentGroup := make([]TextNode, 0)

	groupedNodeCount := 0
	for _, node := range nodes {
		if p.groupLength(currentGroup)+len(node.Text) > p.maxSplitSize && len(currentGroup) > 0 {
			groups = append(groups, currentGroup)
			currentGroup = make([]TextNode, 0)
		}
		currentGroup = append(currentGroup, node)
		groupedNodeCount++
	}

	if len(currentGroup) > 0 {
		groups = append(groups, currentGroup)
	}

	// p.debugGroupsLength(groups)

	if groupedNodeCount != len(nodes) {
		p.logger.Warn("group Nodes节点分组失败， 前后不一致", zap.Int("节点数量", len(nodes)), zap.Int("分组节点数量", groupedNodeCount))
	}

	return groups
}

func (p *TextProcessor) ungroupNodes(nodeGroups [][]TextNode) []TextNode {
	nodes := make([]TextNode, 0)
	for _, nodeGroup := range nodeGroups {
		nodes = append(nodes, nodeGroup...)
	}
	return nodes
}

func (p *TextProcessor) combineNodeGroupText(nodeGroup []TextNode) string {
	var result strings.Builder
	for _, node := range nodeGroup {
		result.WriteString(node.StartMarker)
		result.WriteString("\n")
		result.WriteString(node.Text)
		result.WriteString("\n")
		result.WriteString(node.EndMarker)
		result.WriteString("\n\n")
	}
	return strings.TrimSpace(result.String())
}

func (p *TextProcessor) combineNodeGroupsText(nodeGroups [][]TextNode) string {
	var result strings.Builder
	for _, nodeGroup := range nodeGroups {
		result.WriteString(p.combineNodeGroupText(nodeGroup))
		result.WriteString("\n\n")
	}
	return strings.TrimSpace(result.String())
}

func (p *TextProcessor) combineTranslatedNodeGroupText(nodeGroup []TextNode) string {
	var result strings.Builder
	for _, node := range nodeGroup {
		if node.Translated {
			result.WriteString(node.TranslatedText)
			result.WriteString("\n\n")
		} else {
			result.WriteString(node.Text)
			result.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(result.String())
}

func (p *TextProcessor) translateNodeGroup(nodeGroup []TextNode) []TextNode {
	translatedNodes := make([]TextNode, 0)
	translatedNodes = append(translatedNodes, nodeGroup...)

	toBeTranslatedText := p.combineNodeGroupText(nodeGroup)
	p.logger.Debug("待翻译文本", zap.String("文本", snippet(toBeTranslatedText)), zap.Int("长度", len(toBeTranslatedText)))
	translatedText, err := p.Translator.Translate(toBeTranslatedText, false)
	if err != nil {
		p.logger.Warn("翻译节点组失败", zap.Error(err), zap.String("节点组", snippet(p.combineNodeGroupText(nodeGroup))))
		translatedText = p.combineNodeGroupText(nodeGroup)
		return nodeGroup
	}

	translatedParasWithID := p.parseMarkedParagraphs(translatedText)

	for inputNodeIdx, inputNode := range translatedNodes {
		for translatedNodeIdx, translatedStr := range translatedParasWithID {
			if translatedNodeIdx == inputNode.ID {
				if p.textSimilarity(translatedNodes[inputNodeIdx].Text, translatedStr) >= p.similarityThreshold {
					p.logger.Warn("翻译结果与原文相似度低于阈值，使用原文",
						zap.String("原文", translatedNodes[inputNodeIdx].Text),
						zap.String("译文", translatedStr),
						zap.Float64("相似度", p.textSimilarity(translatedNodes[inputNodeIdx].Text, translatedStr)),
						zap.Float64("阈值", p.similarityThreshold))
				} else {
					translatedStr = strings.TrimSpace(translatedStr)
					translatedStr = strings.Replace(translatedStr, "\n\n", "\n", -1)
					translatedNodes[inputNodeIdx].TranslatedText = translatedStr
					translatedNodes[inputNodeIdx].Translated = true
				}
			}
		}
	}
	return translatedNodes
}

// TranslateText 翻译文本内容
//func (p *TextProcessor) TranslateTextOld(text string) (string, error) {
//	log := p.Translator.GetLogger()
//	log.Info("开始翻译文本")
//
//	// 获取配置的分割大小限制
//	minSplitSize := 100       // 默认最小分割大小
//	maxSplitSize := 1000      // 默认最大分割大小
//	retryFailedParts := false // 默认不重试失败的部分
//
//	if cfg, ok := p.Translator.(interface{ GetConfig() *config.Config }); ok {
//		config := cfg.GetConfig()
//		if config.MinSplitSize > 0 {
//			minSplitSize = config.MinSplitSize
//		}
//		if config.MaxSplitSize > 0 {
//			maxSplitSize = config.MaxSplitSize
//		}
//		retryFailedParts = config.RetryFailedParts
//	}
//
//	log.Debug("分割大小设置",
//		zap.Int("最小分割大小", minSplitSize),
//		zap.Int("最大分割大小", maxSplitSize),
//	)
//
//	// 按行分割文本，支持不同的换行符
//	paragraphs := splitTextByLines(text, minSplitSize, maxSplitSize)
//
//	log.Info("文本已分割",
//		zap.Int("段落数", len(paragraphs)),
//	)
//
//	// 找出需要翻译的段落（非空段落）
//	var translatableParagraphs []int
//	for i, paragraph := range paragraphs {
//		if strings.TrimSpace(paragraph) != "" {
//			translatableParagraphs = append(translatableParagraphs, i)
//		}
//	}
//
//	// 获取配置的并行度
//	concurrency := 4 // 默认并行度
//	if cfg, ok := p.Translator.(interface{ GetConfig() *config.Config }); ok {
//		if cfg.GetConfig().Concurrency > 0 {
//			concurrency = cfg.GetConfig().Concurrency
//		}
//	}
//
//	// 限制并行度不超过需要翻译的段落数量
//	if concurrency > len(translatableParagraphs) {
//		concurrency = len(translatableParagraphs)
//	}
//
//	log.Debug("并行翻译设置",
//		zap.Int("需要翻译的段落数", len(translatableParagraphs)),
//		zap.Int("并行度", concurrency),
//	)
//
//	// 初始化进度跟踪
//	var totalChars int
//	for _, idx := range translatableParagraphs {
//		totalChars += len(paragraphs[idx])
//	}
//
//	// 如果没有需要翻译的段落，直接返回原文
//	if len(translatableParagraphs) == 0 {
//		return text, nil
//	}
//
//	// 如果只有一个需要翻译的段落，或者并行度为1，使用串行处理
//	if len(translatableParagraphs) == 1 || concurrency == 1 {
//		for i, paragraph := range paragraphs {
//			// 跳过空段落
//			if strings.TrimSpace(paragraph) == "" {
//				continue
//			}
//
//			log.Debug("翻译段落",
//				zap.Int("段落索引", i),
//				zap.Int("段落长度", len(paragraph)),
//			)
//
//			// 翻译段落
//			translated, err := p.Translator.Translate(paragraph, retryFailedParts)
//			if err != nil {
//				return "", fmt.Errorf("翻译段落失败: %w", err)
//			}
//
//			// 更新进度
//			p.Translator.GetProgressTracker().UpdateRealTranslatedChars(len(paragraph))
//
//			// 更新翻译后的段落
//			paragraphs[i] = translated
//		}
//	} else {
//		// 使用并行处理
//		// 创建工作通道和结果通道
//		type translationJob struct {
//			index   int
//			content string
//		}
//
//		type translationResult struct {
//			index      int
//			translated string
//			err        error
//		}
//
//		jobs := make(chan translationJob, len(translatableParagraphs))
//		results := make(chan translationResult, len(translatableParagraphs))
//
//		// 启动工作协程
//		var wg sync.WaitGroup
//		for w := 0; w < concurrency; w++ {
//			wg.Add(1)
//			go func() {
//				defer wg.Done()
//				for job := range jobs {
//					translated, err := p.Translator.Translate(job.content, retryFailedParts)
//					if err == nil {
//						// 更新进度
//						p.Translator.GetProgressTracker().UpdateRealTranslatedChars(len(job.content))
//					}
//					results <- translationResult{
//						index:      job.index,
//						translated: translated,
//						err:        err,
//					}
//				}
//			}()
//		}
//
//		// 发送翻译任务
//		for _, idx := range translatableParagraphs {
//			jobs <- translationJob{
//				index:   idx,
//				content: paragraphs[idx],
//			}
//		}
//		close(jobs)
//
//		// 等待所有工作完成
//		go func() {
//			wg.Wait()
//			close(results)
//		}()
//
//		// 收集结果
//		for result := range results {
//			if result.err != nil {
//				return "", fmt.Errorf("翻译段落失败 (索引 %d): %w", result.index, result.err)
//			}
//
//			// 更新翻译后的段落
//			paragraphs[result.index] = result.translated
//
//			log.Debug("完成段落翻译",
//				zap.Int("段落索引", result.index),
//				zap.Int("原始长度", len(paragraphs[result.index])),
//				zap.Int("翻译长度", len(result.translated)),
//			)
//		}
//	}
//
//	// 组合翻译后的段落，保持原始换行符
//	translatedText := strings.Join(paragraphs, "\n")
//
//	return translatedText, nil
//}

func (p *TextProcessor) splitTextToNodesWithMarkers(text string) []TextNode {
	nodes := make([]TextNode, 0)
	paragraphs := strings.Split(text, "\n\n")
	for i, p := range paragraphs {
		textStart := fmt.Sprintf("@@NODE_START_%d@@", i)
		textEnd := fmt.Sprintf("@@NODE_END_%d@@", i)
		nodes = append(nodes, TextNode{
			ID:             i,
			Text:           p,
			Translated:     false,
			TranslatedText: "",
			StartMarker:    textStart,
			EndMarker:      textEnd,
		})
	}
	return nodes
}

func (p *TextProcessor) protectText(text string) (string, error) {
	placeholderIndex := 0

	for key, value := range p.predefinedTranslations.Translations {
		placeholder := fmt.Sprintf("@@PRESERVE_%d@@", placeholderIndex)
		p.replacements = append(p.replacements, ReplacementInfo{
			Placeholder: placeholder,
			Original:    value,
		})
		placeholderIndex++
		text = strings.ReplaceAll(text, key, placeholder)
	}

	return text, nil
}

func (p *TextProcessor) restoreText(text string) (string, error) {
	for _, replacement := range p.replacements {
		text = strings.ReplaceAll(text, replacement.Placeholder, replacement.Original)
	}

	// 2. 可能出现 PRESERVE 被翻译的情况，这里再处理一下
	text = mdRePlaceholderWildcard.ReplaceAllStringFunc(text, func(match string) string {
		// 提取出数字
		parts := mdRePlaceholderWildcard.FindStringSubmatch(match)
		if len(parts) == 3 {
			wildcard := parts[1]
			number := parts[2]
			numberInt, err := strconv.Atoi(number)
			if err != nil {
				p.logger.Warn("无法将数字字符串转换为整数", zap.String("数字字符串", number))
				return match
			}
			if numberInt < len(p.replacements) {
				p.logger.Debug("有占位符被翻译了, 还原占位符",
					zap.String("占位符", match),
					zap.String("wildcard", wildcard),
					zap.String("原始内容", p.replacements[numberInt].Original))
				text = strings.ReplaceAll(text, match, p.replacements[numberInt].Original)
				return text
			}
		}
		return match
	})

	return text, nil
}

func (p *TextProcessor) textSimilarity(text1, text2 string) float64 {
	if text1 == "" && text2 == "" {
		return 0
	}

	dist := fuzzy.LevenshteinDistance(text1, text2)
	maxLen := len(text1)

	if len(text2) > maxLen {
		maxLen = len(text2)
	}

	if maxLen == 0 {
		return 1
	}

	return 1 - float64(dist)/float64(maxLen)
}

func (p *TextProcessor) testTranslated(nodes []TextNode) []TextNode {
	translatedNodes := make([]TextNode, 0)
	translatedNodes = append(translatedNodes, nodes...)
	for i, node := range nodes {
		isNodeTranslated := p.textSimilarity(node.Text, node.TranslatedText) >= p.similarityThreshold
		nodes[i].Translated = isNodeTranslated
	}
	return nodes
}

func (p *TextProcessor) formatText(text string) string {
	return p.tf.FormatText(text)
}

func (p *TextProcessor) formatFile(inputPath, outputPath string) error {
	return p.tf.FormatFile(inputPath, outputPath)
}

func (p *TextProcessor) filterFailedNodes(nodes []TextNode) []TextNode {
	failedNodes := make([]TextNode, 0)
	for _, node := range nodes {
		if !node.Translated {
			failedNodes = append(failedNodes, node)
		}
	}
	return failedNodes
}

func (p *TextProcessor) generateFailedNodesWithContext(nodes []TextNode) []TextNode {
	nodesWithContext := make([]TextNode, 0)

	failedNodes := p.filterFailedNodes(nodes)
	if len(failedNodes) == 0 {
		return nodesWithContext
	}

	intSet := NewIntSet()
	for _, node := range failedNodes {
		intSet.Add(node.ID)

		if node.ID > 1 {
			intSet.Add(node.ID - 1)
		}
		if node.ID < p.numNodes-1 {
			intSet.Add(node.ID + 1)
		}
	}

	for _, node := range nodes {
		if intSet.Contains(node.ID) {
			nodesWithContext = append(nodesWithContext, node)
		}
	}

	return nodesWithContext
}

// parseMarkedParagraphs parses the marked translation text and returns a map
// from node id to translated content.
func (p *TextProcessor) parseMarkedParagraphs(text string) map[int]string {
	result := make(map[int]string)
	re := regexp2.MustCompile(`(?s)@@NODE_START_(\d+)@@\r?\n(.*?)\r?\n@@NODE_END_\1@@`, regexp2.Singleline)
	var m *regexp2.Match
	m, err := re.FindStringMatch(text)
	if err != nil {
		p.logger.Error("regexp2 查找匹配时出错", zap.Error(err))
		return result
	}
	for m != nil {
		groups := m.Groups()
		if len(groups) == 3 {
			idx := groups[1].String()
			id := 0
			fmt.Sscanf(idx, "%d", &id)
			if id >= 0 && id < p.numNodes {
				result[id] = strings.TrimSpace(groups[2].String())
			} else {
				p.logger.Warn("节点索引超出范围", zap.Int("节点索引", id), zap.Int("节点数量", p.numNodes))
			}
		}
		m, err = re.FindNextMatch(m)
	}
	return result
}

// parallelTranslateTextChunks 并行翻译文本块
func parallelTranslateTextChunks(chunks []Chunk, processor *TextProcessor, concurrency int) ([]string, error) {
	if len(chunks) == 0 {
		return nil, nil
	}

	// 如果块数量小于并行度，调整并行度
	if concurrency > len(chunks) {
		concurrency = len(chunks)
	}

	// 创建工作通道和结果通道
	jobs := make(chan Chunk, len(chunks))
	results := make(chan struct {
		index int
		text  string
		err   error
	}, len(chunks))

	// 启动工作协程
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for chunk := range jobs {
				if !chunk.NeedToTranslate {
					results <- struct {
						index int
						text  string
						err   error
					}{0, chunk.Text, nil}
					continue
				}
				translated, err := processor.TranslateText(chunk.Text)
				results <- struct {
					index int
					text  string
					err   error
				}{0, translated, err}
			}
		}()
	}

	// 发送翻译任务
	for _, chunk := range chunks {
		jobs <- chunk
	}
	close(jobs)

	// 等待所有工作完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 收集结果
	translatedTexts := make([]string, 0, len(chunks))
	for result := range results {
		if result.err != nil {
			return nil, result.err
		}
		translatedTexts = append(translatedTexts, result.text)
	}

	return translatedTexts, nil
}
