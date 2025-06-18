package stats

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/pkg/providers"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// StatisticsMiddleware 统计中间件
type StatisticsMiddleware struct {
	next         providers.Provider
	statsManager *StatsManager
	providerName string
	modelName    string
}

// NewStatisticsMiddleware 创建统计中间件
func NewStatisticsMiddleware(next providers.Provider, statsManager *StatsManager, providerName, modelName string) *StatisticsMiddleware {
	return &StatisticsMiddleware{
		next:         next,
		statsManager: statsManager,
		providerName: providerName,
		modelName:    modelName,
	}
}

// Translate 带统计的翻译方法
func (sm *StatisticsMiddleware) Translate(ctx context.Context, req *providers.ProviderRequest) (*providers.ProviderResponse, error) {
	startTime := time.Now()

	// 分析请求特征
	requestFeatures := sm.analyzeRequest(req)

	// 执行实际翻译
	resp, err := sm.next.Translate(ctx, req)

	latency := time.Since(startTime)

	// 分析响应结果
	result := sm.analyzeResponse(req, resp, err, latency, requestFeatures)

	// 记录统计信息
	sm.statsManager.RecordRequest(sm.providerName, sm.modelName, result)

	return resp, err
}

// analyzeRequest 分析请求特征
func (sm *StatisticsMiddleware) analyzeRequest(req *providers.ProviderRequest) requestFeatures {
	features := requestFeatures{}

	// 检查是否为重试请求
	if req.Metadata != nil {
		if retry, ok := req.Metadata["is_retry"].(bool); ok {
			features.isRetry = retry
		}
		if retryCount, ok := req.Metadata["retry_count"].(int); ok {
			features.retryCount = retryCount
		}
	}

	// 计算期望的节点标记数量
	features.expectedNodeMarkers = sm.countExpectedNodeMarkers(req.Text)

	// 检查是否包含格式化内容
	features.hasFormatting = sm.hasFormattingContent(req.Text)

	return features
}

// analyzeResponse 分析响应结果
func (sm *StatisticsMiddleware) analyzeResponse(req *providers.ProviderRequest, resp *providers.ProviderResponse, err error, latency time.Duration, features requestFeatures) RequestResult {
	result := RequestResult{
		Success:          err == nil,
		Latency:          latency,
		IsRetry:          features.isRetry,
		NodeMarkersFound: features.expectedNodeMarkers,
	}

	if err != nil {
		result.ErrorType = sm.classifyError(err)
		return result
	}

	if resp != nil {
		result.TokensIn = resp.TokensIn
		result.TokensOut = resp.TokensOut
		result.Cost = resp.Cost

		// 分析节点标记保持情况
		actualNodeMarkers := sm.countActualNodeMarkers(resp.Text)
		result.NodeMarkersLost = features.expectedNodeMarkers - actualNodeMarkers

		// 检查格式问题
		result.HasFormatIssues = features.hasFormatting && sm.hasFormatIssues(req.Text, resp.Text)

		// 检查推理标记
		result.HasReasoningTags = translation.HasReasoningTags(resp.Text)

		// 检查翻译相似度
		result.SimilarityTooHigh = sm.checkSimilarity(req.Text, resp.Text)
	}

	return result
}

// countExpectedNodeMarkers 计算期望的节点标记数量
func (sm *StatisticsMiddleware) countExpectedNodeMarkers(text string) int {
	pattern := regexp.MustCompile(`@@NODE_START_\d+@@`)
	matches := pattern.FindAllString(text, -1)
	return len(matches)
}

// countActualNodeMarkers 计算实际找到的节点标记数量
func (sm *StatisticsMiddleware) countActualNodeMarkers(text string) int {
	pattern := regexp.MustCompile(`@@NODE_START_\d+@@`)
	matches := pattern.FindAllString(text, -1)
	return len(matches)
}

// hasFormattingContent 检查是否包含格式化内容
func (sm *StatisticsMiddleware) hasFormattingContent(text string) bool {
	formatPatterns := []string{
		`\*\*.*?\*\*`,              // Markdown bold
		`\*.*?\*`,                  // Markdown italic
		`#+ `,                      // Markdown headers
		"`.*?`",                    // Markdown code
		"```",                      // Code blocks
		"\\$.*?\\$",                // LaTeX inline
		"\\$\\$.*?\\$\\$",          // LaTeX block
		"<[^>]+>",                  // HTML tags
		"\\[[^\\]]+\\]\\([^)]+\\)", // Markdown links
	}

	for _, pattern := range formatPatterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			return true
		}
	}

	return false
}

// hasFormatIssues 检查格式是否有问题
func (sm *StatisticsMiddleware) hasFormatIssues(original, translated string) bool {
	// 检查基本的格式标记是否保持
	formatChecks := []struct {
		pattern string
		name    string
	}{
		{`\*\*`, "bold"},
		{"`", "code"},
		{"```", "code_block"},
		{"#", "header"},
		{"\\$", "math"},
		{"<[^>]+>", "html"},
	}

	for _, check := range formatChecks {
		originalCount := len(regexp.MustCompile(check.pattern).FindAllString(original, -1))
		translatedCount := len(regexp.MustCompile(check.pattern).FindAllString(translated, -1))

		// 如果格式标记数量相差太大，认为有格式问题
		if originalCount > 0 && translatedCount == 0 {
			return true
		}
		if originalCount > 0 && float64(translatedCount)/float64(originalCount) < 0.5 {
			return true
		}
	}

	return false
}

// checkSimilarity 检查翻译相似度
func (sm *StatisticsMiddleware) checkSimilarity(original, translated string) bool {
	// 简单的相似度检查：如果翻译后的文本与原文相似度过高
	originalClean := strings.ToLower(strings.TrimSpace(original))
	translatedClean := strings.ToLower(strings.TrimSpace(translated))

	// 移除节点标记进行比较
	nodePattern := regexp.MustCompile(`@@NODE_(?:START|END)_\d+@@`)
	originalClean = nodePattern.ReplaceAllString(originalClean, "")
	translatedClean = nodePattern.ReplaceAllString(translatedClean, "")

	if len(originalClean) < 10 || len(translatedClean) < 10 {
		return false
	}

	// 使用简单的编辑距离判断
	similarity := calculateSimilarity(originalClean, translatedClean)
	return similarity > 0.95 // 相似度超过95%认为过高
}

// calculateSimilarity 计算字符串相似度（简化版Levenshtein距离）
func calculateSimilarity(s1, s2 string) float64 {
	if len(s1) == 0 && len(s2) == 0 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	// 简化计算：基于公共字符比例
	commonChars := 0
	minLen := len(s1)
	if len(s2) < minLen {
		minLen = len(s2)
	}

	for i := 0; i < minLen; i++ {
		if s1[i] == s2[i] {
			commonChars++
		}
	}

	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}

	return float64(commonChars) / float64(maxLen)
}

// classifyError 分类错误类型
func (sm *StatisticsMiddleware) classifyError(err error) string {
	errStr := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errStr, "timeout"):
		return "timeout"
	case strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "rate_limit"):
		return "rate_limit"
	case strings.Contains(errStr, "context") && strings.Contains(errStr, "canceled"):
		return "context_canceled"
	case strings.Contains(errStr, "connection") || strings.Contains(errStr, "network"):
		return "network_error"
	case strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized"):
		return "auth_error"
	case strings.Contains(errStr, "400") || strings.Contains(errStr, "bad request"):
		return "bad_request"
	case strings.Contains(errStr, "500") || strings.Contains(errStr, "internal server"):
		return "server_error"
	case strings.Contains(errStr, "502") || strings.Contains(errStr, "bad gateway"):
		return "bad_gateway"
	case strings.Contains(errStr, "503") || strings.Contains(errStr, "service unavailable"):
		return "service_unavailable"
	case strings.Contains(errStr, "quota") || strings.Contains(errStr, "limit"):
		return "quota_exceeded"
	default:
		return "unknown_error"
	}
}

// requestFeatures 请求特征
type requestFeatures struct {
	isRetry             bool
	retryCount          int
	expectedNodeMarkers int
	hasFormatting       bool
}

// 实现Provider接口的其他方法
func (sm *StatisticsMiddleware) GetName() string {
	return sm.next.GetName()
}

func (sm *StatisticsMiddleware) SupportsSteps() bool {
	return sm.next.SupportsSteps()
}

func (sm *StatisticsMiddleware) Configure(config interface{}) error {
	return sm.next.Configure(config)
}

func (sm *StatisticsMiddleware) GetCapabilities() providers.Capabilities {
	return sm.next.GetCapabilities()
}

func (sm *StatisticsMiddleware) HealthCheck(ctx context.Context) error {
	return sm.next.HealthCheck(ctx)
}
