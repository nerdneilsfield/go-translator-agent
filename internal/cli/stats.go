package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/internal/stats"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	// stats 命令的标志
	statsFormat  string
	recentLimit  int
	cacheCleanup bool
	exportPath   string
	resetStats   bool
)

// NewStatsCommand 创建 stats 命令
func NewStatsCommand() *cobra.Command {
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "View translation statistics and cache information",
		Long: `View comprehensive statistics about your translations, including:
- Overall translation statistics
- Language pair statistics  
- File format statistics
- Recent translation history
- Cache information and statistics
- Performance metrics

Examples:
  # Show overview of all statistics
  translator stats

  # Show recent translations
  translator stats --recent 20

  # Show only cache information
  translator stats --cache

  # Show language pair statistics
  translator stats --languages

  # Show file format statistics  
  translator stats --formats

  # Export statistics to JSON
  translator stats --export stats.json

  # Clean up old cache files
  translator stats --cache --cleanup

  # Reset all statistics
  translator stats --reset`,
		RunE: runStatsCommand,
	}

	// 添加标志
	statsCmd.Flags().StringVar(&statsFormat, "format", "table", "Output format (table, json, csv)")
	statsCmd.Flags().IntVar(&recentLimit, "recent", 10, "Number of recent translations to show")
	statsCmd.Flags().BoolVar(&cacheCleanup, "cleanup", false, "Clean up old cache files")
	statsCmd.Flags().StringVar(&exportPath, "export", "", "Export statistics to file (JSON format)")
	statsCmd.Flags().BoolVar(&resetStats, "reset", false, "Reset all statistics (requires confirmation)")

	// 添加子命令标志
	statsCmd.Flags().Bool("cache", false, "Show only cache statistics")
	statsCmd.Flags().Bool("languages", false, "Show only language pair statistics")
	statsCmd.Flags().Bool("formats", false, "Show only file format statistics")
	statsCmd.Flags().Bool("performance", false, "Show only performance statistics")

	return statsCmd
}

// runStatsCommand 执行 stats 命令
func runStatsCommand(cmd *cobra.Command, args []string) error {
	// 初始化日志
	log := logger.NewLogger(debugMode)
	defer func() {
		_ = log.Sync()
	}()

	// 加载配置
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		log.Warn("failed to load config, using defaults", zap.Error(err))
		cfg = config.NewDefaultConfig()
	}

	// 获取统计数据库路径
	statsPath := getStatsPath(cfg)

	// 创建统计数据库
	db, err := stats.NewDatabase(statsPath, log)
	if err != nil {
		return fmt.Errorf("failed to initialize statistics database: %w", err)
	}

	// 创建可视化器
	visualizer := stats.NewVisualizer(db)

	// 处理重置选项
	if resetStats {
		return handleStatsReset(statsPath, log)
	}

	// 处理导出选项
	if exportPath != "" {
		return handleStatsExport(db, exportPath)
	}

	// 更新缓存统计
	if cfg.UseCache && cfg.CacheDir != "" {
		if err := db.UpdateCacheStats(cfg.CacheDir); err != nil {
			log.Warn("failed to update cache stats", zap.Error(err))
		}

		// 处理缓存清理
		if cacheCleanup {
			return handleCacheCleanup(cfg.CacheDir, log)
		}
	}

	// 检查特定的显示选项
	showCache, _ := cmd.Flags().GetBool("cache")
	showLanguages, _ := cmd.Flags().GetBool("languages")
	showFormats, _ := cmd.Flags().GetBool("formats")
	showPerformance, _ := cmd.Flags().GetBool("performance")

	// 显示统计信息
	if showCache {
		return showCacheStats(db, cfg)
	}

	if showLanguages {
		visualizer.ShowLanguagePairs()
		return nil
	}

	if showFormats {
		visualizer.ShowFormatStats()
		return nil
	}

	if showPerformance {
		return showPerformanceStats(db)
	}

	// 默认显示概览和最近翻译
	visualizer.ShowOverview()

	fmt.Println()
	visualizer.ShowRecentTranslations(recentLimit)

	return nil
}

// getStatsPath 获取统计数据库路径
func getStatsPath(cfg *config.Config) string {
	if cfg.UseCache && cfg.CacheDir != "" {
		return filepath.Join(cfg.CacheDir, "statistics.json")
	}

	// 使用系统缓存目录
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		// 如果无法获取系统缓存目录，使用用户主目录
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "./translator_stats.json"
		}
		return filepath.Join(homeDir, ".translator", "statistics.json")
	}

	return filepath.Join(cacheDir, "translator", "statistics.json")
}

// handleStatsReset 处理统计重置
func handleStatsReset(statsPath string, log *zap.Logger) error {
	fmt.Print("Are you sure you want to reset all statistics? This cannot be undone. (y/N): ")

	var confirmation string
	fmt.Scanln(&confirmation)

	if confirmation != "y" && confirmation != "Y" && confirmation != "yes" {
		fmt.Println("Statistics reset cancelled.")
		return nil
	}

	// 删除统计文件
	if err := os.Remove(statsPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to reset statistics: %w", err)
	}

	fmt.Println("✅ Statistics have been reset.")
	log.Info("statistics reset", zap.String("path", statsPath))

	return nil
}

// handleStatsExport 处理统计导出
func handleStatsExport(db *stats.Database, exportPath string) error {
	statsData := db.GetStats()

	data, err := marshalStats(statsData, statsFormat)
	if err != nil {
		return fmt.Errorf("failed to marshal statistics: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(exportPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create export directory: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(exportPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write export file: %w", err)
	}

	fmt.Printf("✅ Statistics exported to: %s\n", exportPath)
	return nil
}

// marshalStats 序列化统计数据
func marshalStats(statsData *stats.StatisticsDB, format string) ([]byte, error) {
	switch format {
	case "json":
		return json.MarshalIndent(statsData, "", "  ")
	case "csv":
		return marshalStatsCSV(statsData)
	default:
		return json.MarshalIndent(statsData, "", "  ")
	}
}

// marshalStatsCSV 将统计数据转换为 CSV 格式
func marshalStatsCSV(statsData *stats.StatisticsDB) ([]byte, error) {
	var result strings.Builder

	// 最近翻译记录的 CSV
	result.WriteString("timestamp,input_file,output_file,source_language,target_language,format,total_nodes,completed_nodes,failed_nodes,character_count,duration_ms,status,progress\n")

	for _, record := range statsData.RecentTranslations {
		result.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%s,%d,%d,%d,%d,%d,%s,%.2f\n",
			record.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			record.InputFile,
			record.OutputFile,
			record.SourceLanguage,
			record.TargetLanguage,
			record.Format,
			record.TotalNodes,
			record.CompletedNodes,
			record.FailedNodes,
			record.CharacterCount,
			record.Duration.Milliseconds(),
			record.Status,
			record.Progress,
		))
	}

	return []byte(result.String()), nil
}

// handleCacheCleanup 处理缓存清理
func handleCacheCleanup(cacheDir string, log *zap.Logger) error {
	fmt.Printf("Cleaning up cache directory: %s\n", cacheDir)

	// 获取缓存文件
	var cleanedFiles int
	var cleanedSize int64
	cutoffTime := time.Now().AddDate(0, 0, -30) // 30天前

	err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if !info.IsDir() && info.ModTime().Before(cutoffTime) {
			size := info.Size()
			if err := os.Remove(path); err == nil {
				cleanedFiles++
				cleanedSize += size
				log.Debug("removed old cache file",
					zap.String("path", path),
					zap.Time("modified", info.ModTime()))
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to clean cache: %w", err)
	}

	fmt.Printf("✅ Cleaned up %d files (%s) older than 30 days\n",
		cleanedFiles, formatBytes(cleanedSize))

	return nil
}

// showCacheStats 显示缓存统计
func showCacheStats(db *stats.Database, cfg *config.Config) error {
	statsData := db.GetStats()

	// 手动显示缓存统计，因为 printCacheStats 是私有方法
	title := color.New(color.FgCyan, color.Bold)
	title.Println("💾 Cache Statistics")
	title.Println(strings.Repeat("=", 50))

	cache := statsData.CacheStats
	fmt.Printf("  Cache Directory: %s\n", cache.CacheDir)
	fmt.Printf("  Total Cache Files: %s\n", formatNumber(cache.TotalCacheFiles))
	fmt.Printf("  Total Cache Size: %s\n", formatBytes(cache.TotalCacheSize))
	fmt.Printf("  Cache Hit Rate: %.1f%% (%d hits, %d misses)\n",
		cache.CacheHitRate*100, cache.CacheHits, cache.CacheMisses)

	if !cache.OldestCacheEntry.IsZero() {
		fmt.Printf("  Oldest Entry: %s\n", formatTime(cache.OldestCacheEntry))
	}
	if !cache.NewestCacheEntry.IsZero() {
		fmt.Printf("  Newest Entry: %s\n", formatTime(cache.NewestCacheEntry))
	}
	if !cache.LastCleanup.IsZero() {
		fmt.Printf("  Last Cleanup: %s\n", formatTime(cache.LastCleanup))
	}

	// 显示缓存目录内容
	if cfg.UseCache && cfg.CacheDir != "" {
		fmt.Println()
		return showCacheDirectory(cfg.CacheDir)
	}

	return nil
}

// showCacheDirectory 显示缓存目录内容
func showCacheDirectory(cacheDir string) error {
	fmt.Printf("📁 Cache Directory Contents: %s\n", cacheDir)
	fmt.Println(strings.Repeat("-", 60))

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("Cache directory is empty.")
		return nil
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		size := formatBytes(info.Size())
		modTime := info.ModTime().Format("2006-01-02 15:04")

		fmt.Printf("  %s  %8s  %s\n", modTime, size, entry.Name())
	}

	return nil
}

// showPerformanceStats 显示性能统计
func showPerformanceStats(db *stats.Database) error {
	statsData := db.GetStats()

	title := color.New(color.FgRed, color.Bold)
	title.Println("⚡ Performance Statistics")
	title.Println(strings.Repeat("=", 50))

	perf := statsData.PerformanceStats

	fmt.Printf("  Average Translation Speed: %.2f chars/sec\n", perf.AverageTranslationSpeed)
	fmt.Printf("  Average Nodes/Second: %.2f nodes/sec\n", perf.AverageNodesPerSecond)
	fmt.Printf("  Fastest Translation: %s\n", formatDuration(perf.FastestTranslation))
	fmt.Printf("  Slowest Translation: %s\n", formatDuration(perf.SlowestTranslation))
	fmt.Printf("  Peak Memory Usage: %s\n", formatBytes(perf.PeakMemoryUsage))
	fmt.Printf("  Average Memory Usage: %s\n", formatBytes(perf.AverageMemoryUsage))
	fmt.Printf("  Max Concurrent Jobs: %d\n", perf.MaxConcurrentJobs)
	fmt.Printf("  Avg Concurrent Jobs: %.2f\n", perf.AverageConcurrentJobs)

	return nil
}

// formatBytes 辅助函数（如果未在其他地方定义）
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

// formatDuration 辅助函数（如果未在其他地方定义）
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d.Nanoseconds())/1e6)
	}

	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}

	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}

	return fmt.Sprintf("%.1fh", d.Hours())
}

// formatNumber 格式化数字（添加千位分隔符）
func formatNumber(n int64) string {
	if n == 0 {
		return "0"
	}

	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	var result strings.Builder
	for i, char := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteString(",")
		}
		result.WriteRune(char)
	}
	return result.String()
}

// formatTime 格式化时间
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}

	now := time.Now()
	if t.Year() == now.Year() && t.Month() == now.Month() && t.Day() == now.Day() {
		return t.Format("15:04:05")
	}

	if t.Year() == now.Year() {
		return t.Format("Jan 02 15:04")
	}

	return t.Format("2006-01-02 15:04")
}
