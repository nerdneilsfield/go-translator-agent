package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ProviderStats Provider性能统计
type ProviderStats struct {
	ProviderName       string                 `json:"provider_name"`
	ModelName          string                 `json:"model_name"`
	TotalRequests      int64                  `json:"total_requests"`
	SuccessfulRequests int64                  `json:"successful_requests"`
	FailedRequests     int64                  `json:"failed_requests"`
	TotalTokensIn      int64                  `json:"total_tokens_in"`
	TotalTokensOut     int64                  `json:"total_tokens_out"`
	TotalCost          float64                `json:"total_cost"`
	
	// 指令遵循性能
	NodeMarkerSuccess  int64                  `json:"node_marker_success"`   // 节点标记保持成功次数
	NodeMarkerFailed   int64                  `json:"node_marker_failed"`    // 节点标记丢失次数
	FormatPreserved    int64                  `json:"format_preserved"`      // 格式保持成功次数
	FormatCorrupted    int64                  `json:"format_corrupted"`      // 格式损坏次数
	
	// 性能指标
	AverageLatency     time.Duration          `json:"average_latency"`
	MinLatency         time.Duration          `json:"min_latency"`
	MaxLatency         time.Duration          `json:"max_latency"`
	TotalLatency       time.Duration          `json:"total_latency"`
	
	// 错误统计
	ErrorTypes         map[string]int64       `json:"error_types"`           // 按错误类型统计
	ReasoningTagIssues int64                  `json:"reasoning_tag_issues"`  // 推理标记处理问题
	
	// 时间统计
	FirstRequestTime   time.Time              `json:"first_request_time"`
	LastRequestTime    time.Time              `json:"last_request_time"`
	
	// 质量指标
	SimilarityFailures int64                  `json:"similarity_failures"`   // 翻译相似度过高失败次数
	RetryAttempts      int64                  `json:"retry_attempts"`        // 总重试次数
	
	mu sync.RWMutex `json:"-"`
}

// RequestResult 单次请求结果
type RequestResult struct {
	Success           bool
	Latency           time.Duration
	TokensIn          int
	TokensOut         int
	Cost              float64
	ErrorType         string
	NodeMarkersFound  int     // 期望找到的节点标记数
	NodeMarkersLost   int     // 丢失的节点标记数
	HasFormatIssues   bool    // 是否有格式问题
	HasReasoningTags  bool    // 是否包含推理标记
	SimilarityTooHigh bool    // 翻译相似度过高
	IsRetry           bool    // 是否为重试请求
}

// StatsManager 统计管理器
type StatsManager struct {
	stats    map[string]*ProviderStats // key: provider:model
	dbPath   string
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewStatsManager 创建统计管理器
func NewStatsManager(dbPath string, logger *zap.Logger) *StatsManager {
	return &StatsManager{
		stats:  make(map[string]*ProviderStats),
		dbPath: dbPath,
		logger: logger,
	}
}

// getKey 获取统计键
func (sm *StatsManager) getKey(provider, model string) string {
	return fmt.Sprintf("%s:%s", provider, model)
}

// getOrCreateStats 获取或创建统计对象
func (sm *StatsManager) getOrCreateStats(provider, model string) *ProviderStats {
	key := sm.getKey(provider, model)
	
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if stats, exists := sm.stats[key]; exists {
		return stats
	}
	
	stats := &ProviderStats{
		ProviderName:   provider,
		ModelName:      model,
		ErrorTypes:     make(map[string]int64),
		MinLatency:     time.Hour, // 初始设为很大的值
	}
	
	sm.stats[key] = stats
	return stats
}

// RecordRequest 记录请求结果
func (sm *StatsManager) RecordRequest(provider, model string, result RequestResult) {
	stats := sm.getOrCreateStats(provider, model)
	
	stats.mu.Lock()
	defer stats.mu.Unlock()
	
	now := time.Now()
	if stats.FirstRequestTime.IsZero() {
		stats.FirstRequestTime = now
	}
	stats.LastRequestTime = now
	
	stats.TotalRequests++
	if result.IsRetry {
		stats.RetryAttempts++
	}
	
	if result.Success {
		stats.SuccessfulRequests++
	} else {
		stats.FailedRequests++
		if result.ErrorType != "" {
			stats.ErrorTypes[result.ErrorType]++
		}
	}
	
	// Token和成本统计
	stats.TotalTokensIn += int64(result.TokensIn)
	stats.TotalTokensOut += int64(result.TokensOut)
	stats.TotalCost += result.Cost
	
	// 延迟统计
	stats.TotalLatency += result.Latency
	if result.Latency < stats.MinLatency {
		stats.MinLatency = result.Latency
	}
	if result.Latency > stats.MaxLatency {
		stats.MaxLatency = result.Latency
	}
	if stats.TotalRequests > 0 {
		stats.AverageLatency = stats.TotalLatency / time.Duration(stats.TotalRequests)
	}
	
	// 指令遵循性能
	if result.NodeMarkersLost > 0 {
		stats.NodeMarkerFailed++
	} else if result.NodeMarkersFound > 0 {
		stats.NodeMarkerSuccess++
	}
	
	if result.HasFormatIssues {
		stats.FormatCorrupted++
	} else {
		stats.FormatPreserved++
	}
	
	if result.HasReasoningTags {
		stats.ReasoningTagIssues++
	}
	
	if result.SimilarityTooHigh {
		stats.SimilarityFailures++
	}
}

// GetStats 获取指定Provider的统计信息
func (sm *StatsManager) GetStats(provider, model string) *ProviderStats {
	key := sm.getKey(provider, model)
	
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	if stats, exists := sm.stats[key]; exists {
		// 返回副本，避免并发问题
		stats.mu.RLock()
		defer stats.mu.RUnlock()
		
		statsCopy := *stats
		statsCopy.ErrorTypes = make(map[string]int64)
		for k, v := range stats.ErrorTypes {
			statsCopy.ErrorTypes[k] = v
		}
		return &statsCopy
	}
	
	return nil
}

// GetAllStats 获取所有统计信息
func (sm *StatsManager) GetAllStats() map[string]*ProviderStats {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	result := make(map[string]*ProviderStats)
	for key, stats := range sm.stats {
		stats.mu.RLock()
		statsCopy := *stats
		statsCopy.ErrorTypes = make(map[string]int64)
		for k, v := range stats.ErrorTypes {
			statsCopy.ErrorTypes[k] = v
		}
		stats.mu.RUnlock()
		result[key] = &statsCopy
	}
	
	return result
}

// CalculateMetrics 计算性能指标
func (ps *ProviderStats) CalculateMetrics() map[string]interface{} {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	
	metrics := make(map[string]interface{})
	
	// 基础成功率
	if ps.TotalRequests > 0 {
		metrics["success_rate"] = float64(ps.SuccessfulRequests) / float64(ps.TotalRequests) * 100
		metrics["error_rate"] = float64(ps.FailedRequests) / float64(ps.TotalRequests) * 100
	}
	
	// 指令遵循率
	totalNodeMarkerTests := ps.NodeMarkerSuccess + ps.NodeMarkerFailed
	if totalNodeMarkerTests > 0 {
		metrics["node_marker_compliance_rate"] = float64(ps.NodeMarkerSuccess) / float64(totalNodeMarkerTests) * 100
	}
	
	totalFormatTests := ps.FormatPreserved + ps.FormatCorrupted
	if totalFormatTests > 0 {
		metrics["format_preservation_rate"] = float64(ps.FormatPreserved) / float64(totalFormatTests) * 100
	}
	
	// 质量指标
	if ps.TotalRequests > 0 {
		metrics["similarity_failure_rate"] = float64(ps.SimilarityFailures) / float64(ps.TotalRequests) * 100
		metrics["reasoning_tag_issue_rate"] = float64(ps.ReasoningTagIssues) / float64(ps.TotalRequests) * 100
		metrics["retry_rate"] = float64(ps.RetryAttempts) / float64(ps.TotalRequests) * 100
	}
	
	// Token效率
	if ps.TotalTokensIn > 0 {
		metrics["token_efficiency"] = float64(ps.TotalTokensOut) / float64(ps.TotalTokensIn)
	}
	
	// 成本效率 (每千Token成本)
	if ps.TotalTokensOut > 0 {
		metrics["cost_per_1k_tokens"] = ps.TotalCost / float64(ps.TotalTokensOut) * 1000
	}
	
	// 性能指标
	metrics["average_latency_ms"] = ps.AverageLatency.Milliseconds()
	metrics["min_latency_ms"] = ps.MinLatency.Milliseconds()
	metrics["max_latency_ms"] = ps.MaxLatency.Milliseconds()
	
	return metrics
}

// SaveToDB 保存统计数据到数据库
func (sm *StatsManager) SaveToDB() error {
	if sm.dbPath == "" {
		return nil
	}
	
	// 确保目录存在
	dir := filepath.Dir(sm.dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create stats directory: %w", err)
	}
	
	data := sm.GetAllStats()
	
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal stats data: %w", err)
	}
	
	tempPath := sm.dbPath + ".tmp"
	if err := os.WriteFile(tempPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write stats file: %w", err)
	}
	
	if err := os.Rename(tempPath, sm.dbPath); err != nil {
		return fmt.Errorf("failed to rename stats file: %w", err)
	}
	
	sm.logger.Info("stats saved to database", zap.String("path", sm.dbPath))
	return nil
}

// LoadFromDB 从数据库加载统计数据
func (sm *StatsManager) LoadFromDB() error {
	if sm.dbPath == "" {
		return nil
	}
	
	if _, err := os.Stat(sm.dbPath); os.IsNotExist(err) {
		sm.logger.Info("stats database not found, starting fresh", zap.String("path", sm.dbPath))
		return nil
	}
	
	data, err := os.ReadFile(sm.dbPath)
	if err != nil {
		return fmt.Errorf("failed to read stats file: %w", err)
	}
	
	var statsData map[string]*ProviderStats
	if err := json.Unmarshal(data, &statsData); err != nil {
		return fmt.Errorf("failed to unmarshal stats data: %w", err)
	}
	
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	for key, stats := range statsData {
		if stats.ErrorTypes == nil {
			stats.ErrorTypes = make(map[string]int64)
		}
		sm.stats[key] = stats
	}
	
	sm.logger.Info("stats loaded from database", 
		zap.String("path", sm.dbPath),
		zap.Int("providers", len(statsData)))
	
	return nil
}

// PrintStatsTable 打印统计表格
func (sm *StatsManager) PrintStatsTable() {
	allStats := sm.GetAllStats()
	if len(allStats) == 0 {
		fmt.Println("No statistics available.")
		return
	}
	
	fmt.Println("\n📊 Provider Performance Statistics")
	fmt.Println(strings.Repeat("=", 150))
	
	// 表头
	fmt.Printf("%-20s %-15s %8s %8s %8s %8s %8s %8s %8s %10s %8s\n",
		"Provider", "Model", "Requests", "Success%", "Error%", "NodeMark%", "Format%", "AvgLatency", "Tokens", "Cost", "Retry%")
	fmt.Println(strings.Repeat("-", 150))
	
	for _, stats := range allStats {
		metrics := stats.CalculateMetrics()
		
		successRate := getFloat(metrics, "success_rate")
		errorRate := getFloat(metrics, "error_rate")
		nodeMarkRate := getFloat(metrics, "node_marker_compliance_rate")
		formatRate := getFloat(metrics, "format_preservation_rate")
		avgLatency := getInt(metrics, "average_latency_ms")
		retryRate := getFloat(metrics, "retry_rate")
		
		fmt.Printf("%-20s %-15s %8d %7.1f%% %7.1f%% %8.1f%% %7.1f%% %7dms %8d $%7.2f %7.1f%%\n",
			truncateString(stats.ProviderName, 20),
			truncateString(stats.ModelName, 15),
			stats.TotalRequests,
			successRate,
			errorRate,
			nodeMarkRate,
			formatRate,
			avgLatency,
			stats.TotalTokensOut,
			stats.TotalCost,
			retryRate)
	}
	
	fmt.Println(strings.Repeat("=", 150))
	fmt.Println()
}

// 辅助函数
func getFloat(m map[string]interface{}, key string) float64 {
	if val, ok := m[key]; ok {
		if f, ok := val.(float64); ok {
			return f
		}
	}
	return 0.0
}

func getInt(m map[string]interface{}, key string) int64 {
	if val, ok := m[key]; ok {
		if i, ok := val.(int64); ok {
			return i
		}
	}
	return 0
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// AutoSaveRoutine 定期自动保存统计数据
func (sm *StatsManager) AutoSaveRoutine(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			// 最后一次保存
			if err := sm.SaveToDB(); err != nil {
				sm.logger.Error("failed to save stats on shutdown", zap.Error(err))
			}
			return
		case <-ticker.C:
			if err := sm.SaveToDB(); err != nil {
				sm.logger.Error("failed to auto-save stats", zap.Error(err))
			}
		}
	}
}