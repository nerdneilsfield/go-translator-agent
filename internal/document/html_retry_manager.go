package document

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
)

// HTMLRetryManager HTML特定的重试管理器
// 参考html_utils.go中的重试机制，提供更智能的错误处理和重试策略
type HTMLRetryManager struct {
	logger             *zap.Logger
	maxRetries         int
	retryDelay         time.Duration
	retryStrategy      RetryStrategy
	errorClassifier    *HTMLErrorClassifier
	contextManager     *HTMLContextManager
	placeholderManager *HTMLPlaceholderManager
	mu                 sync.RWMutex
	retryHistory       map[string]*RetryRecord
}

// RetryStrategy 重试策略
type RetryStrategy string

const (
	RetryStrategyImmediate  RetryStrategy = "immediate"  // 立即重试
	RetryStrategyBackoff    RetryStrategy = "backoff"    // 指数退避
	RetryStrategyAdaptive   RetryStrategy = "adaptive"   // 自适应重试
	RetryStrategyContextual RetryStrategy = "contextual" // 上下文重试
)

// RetryRecord 重试记录
type RetryRecord struct {
	NodePath        string                // 节点路径
	OriginalText    string                // 原始文本
	AttemptCount    int                   // 尝试次数
	LastAttempt     time.Time             // 最后尝试时间
	LastError       error                 // 最后的错误
	ErrorHistory    []RetryError          // 错误历史
	SuccessRate     float64               // 成功率
	Context         *HTMLTranslationContext // 翻译上下文
	Strategy        RetryStrategy         // 使用的策略
}

// RetryError 重试错误信息
type RetryError struct {
	Timestamp   time.Time `json:"timestamp"`
	ErrorType   string    `json:"errorType"`
	ErrorCode   string    `json:"errorCode"`
	Message     string    `json:"message"`
	Severity    string    `json:"severity"`
	Recoverable bool      `json:"recoverable"`
}

// HTMLTranslationContext HTML翻译上下文
type HTMLTranslationContext struct {
	ParentElement   *goquery.Selection `json:"-"`           // 父元素
	SiblingsBefore  []string           `json:"siblingsBefore"` // 前兄弟节点
	SiblingsAfter   []string           `json:"siblingsAfter"`  // 后兄弟节点
	ElementTag      string             `json:"elementTag"`     // 元素标签
	ElementAttrs    map[string]string  `json:"elementAttrs"`   // 元素属性
	DocumentSection string             `json:"documentSection"` // 文档区域（header, main, footer等）
	SemanticHints   []string           `json:"semanticHints"`   // 语义提示
}

// HTMLRetryConfig HTML重试配置
type HTMLRetryConfig struct {
	MaxRetries         int           `json:"maxRetries"`
	RetryDelay         time.Duration `json:"retryDelay"`
	Strategy           RetryStrategy `json:"strategy"`
	EnableContextRetry bool          `json:"enableContextRetry"`
	EnableSmartGrouping bool         `json:"enableSmartGrouping"`
	MaxContextNodes    int           `json:"maxContextNodes"`
	BackoffMultiplier  float64       `json:"backoffMultiplier"`
	MaxBackoffDelay    time.Duration `json:"maxBackoffDelay"`
}

// DefaultHTMLRetryConfig 默认HTML重试配置
func DefaultHTMLRetryConfig() HTMLRetryConfig {
	return HTMLRetryConfig{
		MaxRetries:         3,
		RetryDelay:         time.Second * 1,
		Strategy:           RetryStrategyAdaptive,
		EnableContextRetry: true,
		EnableSmartGrouping: true,
		MaxContextNodes:    2,
		BackoffMultiplier:  2.0,
		MaxBackoffDelay:    time.Second * 30,
	}
}

// NewHTMLRetryManager 创建HTML重试管理器
func NewHTMLRetryManager(logger *zap.Logger, config HTMLRetryConfig) *HTMLRetryManager {
	return &HTMLRetryManager{
		logger:             logger,
		maxRetries:         config.MaxRetries,
		retryDelay:         config.RetryDelay,
		retryStrategy:      config.Strategy,
		errorClassifier:    NewHTMLErrorClassifier(logger),
		contextManager:     NewHTMLContextManager(logger, config.MaxContextNodes),
		placeholderManager: NewHTMLPlaceholderManager(),
		retryHistory:       make(map[string]*RetryRecord),
	}
}

// RecordFailure 记录翻译失败
func (rm *HTMLRetryManager) RecordFailure(node *ExtractableNode, err error, context *HTMLTranslationContext) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	record, exists := rm.retryHistory[node.Path]
	if !exists {
		record = &RetryRecord{
			NodePath:     node.Path,
			OriginalText: node.Text,
			Context:      context,
			Strategy:     rm.retryStrategy,
		}
		rm.retryHistory[node.Path] = record
	}

	// 分类错误
	retryError := rm.errorClassifier.ClassifyError(err)
	record.ErrorHistory = append(record.ErrorHistory, retryError)
	record.LastError = err
	record.LastAttempt = time.Now()
	record.AttemptCount++

	rm.logger.Debug("recorded translation failure",
		zap.String("nodePath", node.Path),
		zap.String("errorType", retryError.ErrorType),
		zap.String("errorCode", retryError.ErrorCode),
		zap.Int("attemptCount", record.AttemptCount),
		zap.Bool("recoverable", retryError.Recoverable))
}

// ShouldRetry 判断是否应该重试
func (rm *HTMLRetryManager) ShouldRetry(nodePath string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	record, exists := rm.retryHistory[nodePath]
	if !exists {
		return false
	}

	// 检查重试次数限制
	if record.AttemptCount >= rm.maxRetries {
		return false
	}

	// 检查最后一个错误是否可恢复
	if len(record.ErrorHistory) > 0 {
		lastError := record.ErrorHistory[len(record.ErrorHistory)-1]
		if !lastError.Recoverable {
			return false
		}
	}

	// 检查重试延迟
	if time.Since(record.LastAttempt) < rm.calculateRetryDelay(record) {
		return false
	}

	return true
}

// PrepareRetryNodes 准备重试节点
func (rm *HTMLRetryManager) PrepareRetryNodes(allNodes []*ExtractableNode) ([]*ExtractableNode, error) {
	var retryNodes []*ExtractableNode

	for _, node := range allNodes {
		if rm.ShouldRetry(node.Path) {
			// 更新节点上下文
			err := rm.enrichNodeContext(node)
			if err != nil {
				rm.logger.Warn("failed to enrich node context",
					zap.String("nodePath", node.Path),
					zap.Error(err))
			}

			retryNodes = append(retryNodes, node)
		}
	}

	// 智能分组重试节点
	if len(retryNodes) > 1 {
		return rm.smartGroupRetryNodes(retryNodes), nil
	}

	return retryNodes, nil
}

// enrichNodeContext 丰富节点上下文
func (rm *HTMLRetryManager) enrichNodeContext(node *ExtractableNode) error {
	if node.Selection == nil {
		return fmt.Errorf("node selection is nil")
	}

	// 获取上下文信息
	context := rm.contextManager.BuildTranslationContext(node.Selection)

	rm.mu.Lock()
	if record, exists := rm.retryHistory[node.Path]; exists {
		record.Context = context
	}
	rm.mu.Unlock()

	// 更新节点的上下文信息
	node.Context.BeforeContext = strings.Join(context.SiblingsBefore, " ")
	node.Context.AfterContext = strings.Join(context.SiblingsAfter, " ")

	return nil
}

// smartGroupRetryNodes 智能分组重试节点
func (rm *HTMLRetryManager) smartGroupRetryNodes(nodes []*ExtractableNode) []*ExtractableNode {
	// 按语义关联性分组
	groups := make(map[string][]*ExtractableNode)

	for _, node := range nodes {
		groupKey := rm.getSemanticGroupKey(node)
		groups[groupKey] = append(groups[groupKey], node)
	}

	// 重新组织节点顺序，相关节点放在一起
	var groupedNodes []*ExtractableNode
	for _, group := range groups {
		groupedNodes = append(groupedNodes, group...)
	}

	return groupedNodes
}

// getSemanticGroupKey 获取语义分组键
func (rm *HTMLRetryManager) getSemanticGroupKey(node *ExtractableNode) string {
	key := node.ParentTag

	// 检查是否在相同的语义区域
	rm.mu.RLock()
	if record, exists := rm.retryHistory[node.Path]; exists && record.Context != nil {
		key += ":" + record.Context.DocumentSection
	}
	rm.mu.RUnlock()

	return key
}

// calculateRetryDelay 计算重试延迟
func (rm *HTMLRetryManager) calculateRetryDelay(record *RetryRecord) time.Duration {
	switch rm.retryStrategy {
	case RetryStrategyImmediate:
		return 0

	case RetryStrategyBackoff:
		// 指数退避
		multiplier := 1.0
		for i := 1; i < record.AttemptCount; i++ {
			multiplier *= 2.0
		}
		delay := time.Duration(float64(rm.retryDelay) * multiplier)
		if delay > time.Second*30 { // 最大30秒
			delay = time.Second * 30
		}
		return delay

	case RetryStrategyAdaptive:
		// 自适应策略：基于错误类型调整
		baseDelay := rm.retryDelay
		if len(record.ErrorHistory) > 0 {
			lastError := record.ErrorHistory[len(record.ErrorHistory)-1]
			switch lastError.ErrorType {
			case "network_error":
				baseDelay *= 3 // 网络错误延迟更长
			case "rate_limit":
				baseDelay *= 5 // 限流错误延迟更长
			case "timeout":
				baseDelay *= 2 // 超时错误适中延迟
			}
		}
		return baseDelay

	case RetryStrategyContextual:
		// 上下文策略：考虑同类节点的成功率
		if record.SuccessRate > 0.5 {
			return rm.retryDelay / 2 // 成功率高的快速重试
		}
		return rm.retryDelay * 2 // 成功率低的延迟重试

	default:
		return rm.retryDelay
	}
}

// RecordSuccess 记录翻译成功
func (rm *HTMLRetryManager) RecordSuccess(nodePath string, translatedText string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if record, exists := rm.retryHistory[nodePath]; exists {
		// 更新成功率
		totalAttempts := record.AttemptCount + 1
		record.SuccessRate = 1.0 / float64(totalAttempts)

		rm.logger.Debug("recorded translation success",
			zap.String("nodePath", nodePath),
			zap.Int("totalAttempts", totalAttempts),
			zap.Float64("successRate", record.SuccessRate))

		// 从重试历史中移除成功的记录
		delete(rm.retryHistory, nodePath)
	}
}

// GetRetryStatistics 获取重试统计信息
func (rm *HTMLRetryManager) GetRetryStatistics() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	stats := map[string]interface{}{
		"totalRetryNodes":  len(rm.retryHistory),
		"errorDistribution": make(map[string]int),
		"strategyStats":     make(map[string]int),
		"avgAttempts":       0.0,
		"recoverableErrors": 0,
		"fatalErrors":       0,
	}

	errorDist := stats["errorDistribution"].(map[string]int)
	strategyStats := stats["strategyStats"].(map[string]int)
	totalAttempts := 0
	recoverableErrors := 0
	fatalErrors := 0

	for _, record := range rm.retryHistory {
		totalAttempts += record.AttemptCount
		strategyStats[string(record.Strategy)]++

		for _, err := range record.ErrorHistory {
			errorDist[err.ErrorType]++
			if err.Recoverable {
				recoverableErrors++
			} else {
				fatalErrors++
			}
		}
	}

	if len(rm.retryHistory) > 0 {
		stats["avgAttempts"] = float64(totalAttempts) / float64(len(rm.retryHistory))
	}
	stats["recoverableErrors"] = recoverableErrors
	stats["fatalErrors"] = fatalErrors

	return stats
}

// CleanupExpiredRetries 清理过期的重试记录
func (rm *HTMLRetryManager) CleanupExpiredRetries(maxAge time.Duration) int {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	now := time.Now()
	cleaned := 0

	for path, record := range rm.retryHistory {
		if now.Sub(record.LastAttempt) > maxAge {
			delete(rm.retryHistory, path)
			cleaned++
		}
	}

	if cleaned > 0 {
		rm.logger.Debug("cleaned up expired retry records", zap.Int("count", cleaned))
	}

	return cleaned
}

// ExecuteRetryWithContext 使用上下文执行重试
func (rm *HTMLRetryManager) ExecuteRetryWithContext(ctx context.Context, node *ExtractableNode, translator TranslateFunc) (string, error) {
	rm.mu.RLock()
	record, exists := rm.retryHistory[node.Path]
	rm.mu.RUnlock()

	if !exists {
		return translator(ctx, node.Text)
	}

	// 构建上下文增强的翻译请求
	enhancedText := rm.buildContextualText(node, record.Context)

	// 执行翻译
	result, err := translator(ctx, enhancedText)
	if err != nil {
		rm.RecordFailure(node, err, record.Context)
		return "", err
	}

	// 提取翻译结果（去除上下文部分）
	translatedText := rm.extractTranslationFromContextual(result, node.Text)

	rm.RecordSuccess(node.Path, translatedText)
	return translatedText, nil
}

// buildContextualText 构建上下文增强的文本
func (rm *HTMLRetryManager) buildContextualText(node *ExtractableNode, context *HTMLTranslationContext) string {
	var builder strings.Builder

	// 添加上下文信息
	if context != nil {
		if len(context.SiblingsBefore) > 0 {
			builder.WriteString("前文: ")
			builder.WriteString(strings.Join(context.SiblingsBefore, " "))
			builder.WriteString("\n\n")
		}
	}

	// 添加要翻译的文本
	builder.WriteString("翻译: ")
	builder.WriteString(node.Text)

	if context != nil {
		if len(context.SiblingsAfter) > 0 {
			builder.WriteString("\n\n后文: ")
			builder.WriteString(strings.Join(context.SiblingsAfter, " "))
		}

		// 添加语义提示
		if len(context.SemanticHints) > 0 {
			builder.WriteString("\n\n语义提示: ")
			builder.WriteString(strings.Join(context.SemanticHints, ", "))
		}
	}

	return builder.String()
}

// extractTranslationFromContextual 从上下文翻译中提取结果
func (rm *HTMLRetryManager) extractTranslationFromContextual(contextualResult, originalText string) string {
	// 简单实现：查找"翻译:"标记后的内容
	lines := strings.Split(contextualResult, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "翻译:") {
			return strings.TrimSpace(line[3:])
		}
	}

	// 如果没有找到标记，返回整个结果
	return strings.TrimSpace(contextualResult)
}

// Reset 重置重试管理器
func (rm *HTMLRetryManager) Reset() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.retryHistory = make(map[string]*RetryRecord)
	rm.logger.Debug("retry manager reset")
}