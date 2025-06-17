package translator

import (
	"fmt"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/document"
)

// TranslationSummary 翻译汇总信息
type TranslationSummary struct {
	// 基本信息
	DocID          string
	InputFile      string
	OutputFile     string
	SourceLanguage string
	TargetLanguage string

	// 节点统计
	TotalNodes      int
	SuccessfulNodes int
	FailedNodes     int
	SkippedNodes    int

	// 字符统计
	TotalChars      int
	TranslatedChars int

	// 时间统计
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration

	// 消耗统计
	TotalTokensIn  int
	TotalTokensOut int
	EstimatedCost  float64

	// 使用的模型
	ModelsUsed map[string]ModelUsage

	// 错误统计
	ErrorTypes map[string]int

	// 缓存统计
	CacheHits   int
	CacheMisses int
	CacheRatio  float64
}

// ModelUsage 模型使用情况
type ModelUsage struct {
	Model     string
	Provider  string
	TokensIn  int
	TokensOut int
	Cost      float64
	CallCount int
}

// GenerateSummary 生成翻译汇总
func GenerateSummary(result *TranslationResult, nodes []*document.NodeInfo, cfg *config.Config) *TranslationSummary {
	summary := &TranslationSummary{
		DocID:      result.DocID,
		InputFile:  result.InputFile,
		OutputFile: result.OutputFile,
		// SourceLanguage: result.SourceLanguage,
		// TargetLanguage: result.TargetLanguage,
		TotalNodes:      result.TotalNodes,
		SuccessfulNodes: result.CompletedNodes,
		FailedNodes:     result.FailedNodes,
		StartTime:       result.StartTime,
		Duration:        result.Duration,
		ModelsUsed:      make(map[string]ModelUsage),
		ErrorTypes:      make(map[string]int),
	}

	if result.EndTime != nil {
		summary.EndTime = *result.EndTime
	} else {
		summary.EndTime = result.StartTime.Add(result.Duration)
	}

	// 统计字符数
	for _, node := range nodes {
		summary.TotalChars += len(node.OriginalText)
		if node.Status == document.NodeStatusSuccess {
			summary.TranslatedChars += len(node.OriginalText)
		} else if node.Status == document.NodeStatusSkipped {
			summary.SkippedNodes++
		}

		// 统计错误类型
		if node.Error != nil {
			errType := "unknown"
			errorMsg := node.Error.Error()
			if strings.Contains(errorMsg, "timeout") {
				errType = "timeout"
			} else if strings.Contains(errorMsg, "rate limit") {
				errType = "rate_limit"
			} else if strings.Contains(errorMsg, "network") {
				errType = "network"
			} else if strings.Contains(errorMsg, "translation too similar") {
				errType = "similarity_check_failed"
			} else if strings.Contains(errorMsg, "translation not found") {
				errType = "parse_error"
			} else if strings.Contains(errorMsg, "API") || strings.Contains(errorMsg, "api") {
				errType = "api_error"
			}
			summary.ErrorTypes[errType]++
		}
	}

	// 从元数据中提取 token 使用情况
	if metadata, ok := result.Metadata["token_usage"].(map[string]interface{}); ok {
		if tokensIn, ok := metadata["tokens_in"].(int); ok {
			summary.TotalTokensIn = tokensIn
		}
		if tokensOut, ok := metadata["tokens_out"].(int); ok {
			summary.TotalTokensOut = tokensOut
		}
	}

	// 计算估算成本
	summary.EstimatedCost = calculateEstimatedCost(summary, cfg)

	// 缓存统计
	if metadata, ok := result.Metadata["cache_stats"].(map[string]interface{}); ok {
		if hits, ok := metadata["hits"].(int); ok {
			summary.CacheHits = hits
		}
		if misses, ok := metadata["misses"].(int); ok {
			summary.CacheMisses = misses
		}
		if summary.CacheHits+summary.CacheMisses > 0 {
			summary.CacheRatio = float64(summary.CacheHits) / float64(summary.CacheHits+summary.CacheMisses)
		}
	}

	return summary
}

// calculateEstimatedCost 计算估算成本
func calculateEstimatedCost(summary *TranslationSummary, cfg *config.Config) float64 {
	// 这里需要根据配置的模型价格计算
	// 简单示例：假设每1000个token的成本
	costPer1KTokenIn := 0.001  // $0.001 per 1K tokens
	costPer1KTokenOut := 0.002 // $0.002 per 1K tokens

	cost := float64(summary.TotalTokensIn)/1000*costPer1KTokenIn +
		float64(summary.TotalTokensOut)/1000*costPer1KTokenOut

	return cost
}

// FormatSummaryTable 格式化汇总表格
func (s *TranslationSummary) FormatSummaryTable() string {
	var builder strings.Builder

	builder.WriteString("\n╔════════════════════════════════════════════════════════════════╗\n")
	builder.WriteString("║                      翻译汇总报告                               ║\n")
	builder.WriteString("╠════════════════════════════════════════════════════════════════╣\n")

	// 基本信息
	builder.WriteString(fmt.Sprintf("║ 文档ID:     %-50s ║\n", s.DocID))
	builder.WriteString(fmt.Sprintf("║ 输入文件:   %-50s ║\n", truncateString(s.InputFile, 50)))
	builder.WriteString(fmt.Sprintf("║ 输出文件:   %-50s ║\n", truncateString(s.OutputFile, 50)))
	builder.WriteString(fmt.Sprintf("║ 源语言:     %-50s ║\n", s.SourceLanguage))
	builder.WriteString(fmt.Sprintf("║ 目标语言:   %-50s ║\n", s.TargetLanguage))

	builder.WriteString("╠════════════════════════════════════════════════════════════════╣\n")

	// 节点统计
	builder.WriteString("║                         节点统计                                ║\n")
	builder.WriteString("╠════════════════════════════════════════════════════════════════╣\n")
	builder.WriteString(fmt.Sprintf("║ 总节点数:   %-50d ║\n", s.TotalNodes))
	builder.WriteString(fmt.Sprintf("║ 成功节点:   %-50d ║\n", s.SuccessfulNodes))
	builder.WriteString(fmt.Sprintf("║ 失败节点:   %-50d ║\n", s.FailedNodes))
	builder.WriteString(fmt.Sprintf("║ 跳过节点:   %-50d ║\n", s.SkippedNodes))

	// 字符统计
	builder.WriteString("╠════════════════════════════════════════════════════════════════╣\n")
	builder.WriteString("║                         字符统计                                ║\n")
	builder.WriteString("╠════════════════════════════════════════════════════════════════╣\n")
	builder.WriteString(fmt.Sprintf("║ 总字符数:   %-50d ║\n", s.TotalChars))
	builder.WriteString(fmt.Sprintf("║ 已翻译:     %-50d ║\n", s.TranslatedChars))
	builder.WriteString(fmt.Sprintf("║ 翻译率:     %-50.2f%% ║\n", float64(s.TranslatedChars)/float64(s.TotalChars)*100))

	// 时间统计
	builder.WriteString("╠════════════════════════════════════════════════════════════════╣\n")
	builder.WriteString("║                         时间统计                                ║\n")
	builder.WriteString("╠════════════════════════════════════════════════════════════════╣\n")
	builder.WriteString(fmt.Sprintf("║ 开始时间:   %-50s ║\n", s.StartTime.Format("2006-01-02 15:04:05")))
	builder.WriteString(fmt.Sprintf("║ 结束时间:   %-50s ║\n", s.EndTime.Format("2006-01-02 15:04:05")))
	builder.WriteString(fmt.Sprintf("║ 总耗时:     %-50s ║\n", s.Duration.String()))

	// 翻译速度
	if s.Duration.Seconds() > 0 {
		charsPerSecond := float64(s.TranslatedChars) / s.Duration.Seconds()
		builder.WriteString(fmt.Sprintf("║ 翻译速度:   %-45.2f 字符/秒 ║\n", charsPerSecond))
	}

	// Token 消耗
	builder.WriteString("╠════════════════════════════════════════════════════════════════╣\n")
	builder.WriteString("║                        Token 消耗                               ║\n")
	builder.WriteString("╠════════════════════════════════════════════════════════════════╣\n")
	builder.WriteString(fmt.Sprintf("║ 输入 Token: %-50d ║\n", s.TotalTokensIn))
	builder.WriteString(fmt.Sprintf("║ 输出 Token: %-50d ║\n", s.TotalTokensOut))
	builder.WriteString(fmt.Sprintf("║ 总计 Token: %-50d ║\n", s.TotalTokensIn+s.TotalTokensOut))
	builder.WriteString(fmt.Sprintf("║ 估算成本:   $%-49.4f ║\n", s.EstimatedCost))

	// 缓存统计
	if s.CacheHits+s.CacheMisses > 0 {
		builder.WriteString("╠════════════════════════════════════════════════════════════════╣\n")
		builder.WriteString("║                         缓存统计                                ║\n")
		builder.WriteString("╠════════════════════════════════════════════════════════════════╣\n")
		builder.WriteString(fmt.Sprintf("║ 缓存命中:   %-50d ║\n", s.CacheHits))
		builder.WriteString(fmt.Sprintf("║ 缓存未中:   %-50d ║\n", s.CacheMisses))
		builder.WriteString(fmt.Sprintf("║ 命中率:     %-48.2f %% ║\n", s.CacheRatio*100))
	}

	// 错误统计
	if len(s.ErrorTypes) > 0 {
		builder.WriteString("╠════════════════════════════════════════════════════════════════╣\n")
		builder.WriteString("║                         错误统计                                ║\n")
		builder.WriteString("╠════════════════════════════════════════════════════════════════╣\n")
		for errType, count := range s.ErrorTypes {
			builder.WriteString(fmt.Sprintf("║ %-25s: %-35d ║\n", errType, count))
		}
	}

	builder.WriteString("╚════════════════════════════════════════════════════════════════╝\n")

	return builder.String()
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// FormatSummaryMarkdown 格式化为 Markdown 表格
func (s *TranslationSummary) FormatSummaryMarkdown() string {
	var builder strings.Builder

	builder.WriteString("## 翻译汇总报告\n\n")

	// 基本信息
	builder.WriteString("### 基本信息\n\n")
	builder.WriteString("| 项目 | 值 |\n")
	builder.WriteString("|------|----|\n")
	builder.WriteString(fmt.Sprintf("| 文档ID | %s |\n", s.DocID))
	builder.WriteString(fmt.Sprintf("| 输入文件 | %s |\n", s.InputFile))
	builder.WriteString(fmt.Sprintf("| 输出文件 | %s |\n", s.OutputFile))
	builder.WriteString(fmt.Sprintf("| 源语言 | %s |\n", s.SourceLanguage))
	builder.WriteString(fmt.Sprintf("| 目标语言 | %s |\n", s.TargetLanguage))
	builder.WriteString("\n")

	// 统计信息
	builder.WriteString("### 统计信息\n\n")
	builder.WriteString("| 指标 | 数值 |\n")
	builder.WriteString("|------|------|\n")
	builder.WriteString(fmt.Sprintf("| 总节点数 | %d |\n", s.TotalNodes))
	builder.WriteString(fmt.Sprintf("| 成功节点 | %d |\n", s.SuccessfulNodes))
	builder.WriteString(fmt.Sprintf("| 失败节点 | %d |\n", s.FailedNodes))
	builder.WriteString(fmt.Sprintf("| 跳过节点 | %d |\n", s.SkippedNodes))
	builder.WriteString(fmt.Sprintf("| 总字符数 | %d |\n", s.TotalChars))
	builder.WriteString(fmt.Sprintf("| 已翻译字符 | %d |\n", s.TranslatedChars))
	builder.WriteString(fmt.Sprintf("| 翻译率 | %.2f%% |\n", float64(s.TranslatedChars)/float64(s.TotalChars)*100))
	builder.WriteString("\n")

	// 性能指标
	builder.WriteString("### 性能指标\n\n")
	builder.WriteString("| 指标 | 数值 |\n")
	builder.WriteString("|------|------|\n")
	builder.WriteString(fmt.Sprintf("| 开始时间 | %s |\n", s.StartTime.Format("2006-01-02 15:04:05")))
	builder.WriteString(fmt.Sprintf("| 结束时间 | %s |\n", s.EndTime.Format("2006-01-02 15:04:05")))
	builder.WriteString(fmt.Sprintf("| 总耗时 | %s |\n", s.Duration.String()))
	if s.Duration.Seconds() > 0 {
		charsPerSecond := float64(s.TranslatedChars) / s.Duration.Seconds()
		builder.WriteString(fmt.Sprintf("| 翻译速度 | %.2f 字符/秒 |\n", charsPerSecond))
	}
	builder.WriteString("\n")

	// 消耗统计
	builder.WriteString("### 消耗统计\n\n")
	builder.WriteString("| 项目 | 数值 |\n")
	builder.WriteString("|------|------|\n")
	builder.WriteString(fmt.Sprintf("| 输入 Token | %d |\n", s.TotalTokensIn))
	builder.WriteString(fmt.Sprintf("| 输出 Token | %d |\n", s.TotalTokensOut))
	builder.WriteString(fmt.Sprintf("| 总计 Token | %d |\n", s.TotalTokensIn+s.TotalTokensOut))
	builder.WriteString(fmt.Sprintf("| 估算成本 | $%.4f |\n", s.EstimatedCost))

	return builder.String()
}
