package stats

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
)

// Visualizer 统计数据可视化器
type Visualizer struct {
	db *Database
}

// NewVisualizer 创建可视化器
func NewVisualizer(db *Database) *Visualizer {
	return &Visualizer{db: db}
}

// ShowOverview 显示总览
func (v *Visualizer) ShowOverview() {
	stats := v.db.GetStats()

	// 标题
	title := color.New(color.FgCyan, color.Bold)
	title.Println("📊 Translation Statistics Overview")
	title.Println(strings.Repeat("=", 50))

	// 总体统计
	fmt.Println()
	v.printSection("🎯 Overall Statistics", [][]string{
		{"Total Translations", formatNumber(stats.TotalTranslations)},
		{"Total Characters", formatNumber(stats.TotalCharacters)},
		{"Total Files", formatNumber(stats.TotalFiles)},
		{"Total Errors", formatNumber(stats.TotalErrors)},
		{"Total Duration", formatDuration(stats.TotalDuration)},
		{"Database Created", formatTime(stats.CreatedAt)},
		{"Last Updated", formatTime(stats.LastUpdated)},
	})

	// 性能统计
	fmt.Println()
	v.printSection("⚡ Performance Statistics", [][]string{
		{"Avg Translation Speed", fmt.Sprintf("%.2f chars/sec", stats.PerformanceStats.AverageTranslationSpeed)},
		{"Avg Nodes/Second", fmt.Sprintf("%.2f nodes/sec", stats.PerformanceStats.AverageNodesPerSecond)},
		{"Fastest Translation", formatDuration(stats.PerformanceStats.FastestTranslation)},
		{"Slowest Translation", formatDuration(stats.PerformanceStats.SlowestTranslation)},
	})

	// 缓存统计
	fmt.Println()
	v.printCacheStats(stats.CacheStats)
}

// ShowLanguagePairs 显示语言对统计
func (v *Visualizer) ShowLanguagePairs() {
	stats := v.db.GetStats()

	title := color.New(color.FgMagenta, color.Bold)
	title.Println("🌍 Language Pair Statistics")
	title.Println(strings.Repeat("=", 50))

	if len(stats.LanguagePairs) == 0 {
		fmt.Println("No language pair data available.")
		return
	}

	// 按翻译数量排序
	pairs := make([]*LanguagePairStats, 0, len(stats.LanguagePairs))
	for _, pair := range stats.LanguagePairs {
		pairs = append(pairs, pair)
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].TranslationCount > pairs[j].TranslationCount
	})

	fmt.Println()
	for i, pair := range pairs {
		if i > 0 {
			fmt.Println()
		}

		langPair := fmt.Sprintf("%s → %s", pair.SourceLanguage, pair.TargetLanguage)
		successRate := float64(pair.TranslationCount-pair.ErrorCount) / float64(pair.TranslationCount) * 100

		v.printSection(fmt.Sprintf("🔄 %s", langPair), [][]string{
			{"Translations", formatNumber(pair.TranslationCount)},
			{"Characters", formatNumber(pair.CharacterCount)},
			{"Files", formatNumber(pair.FileCount)},
			{"Errors", formatNumber(pair.ErrorCount)},
			{"Success Rate", fmt.Sprintf("%.1f%%", successRate)},
			{"Avg Duration", formatDuration(pair.AverageDuration)},
			{"Last Used", formatTime(pair.LastUsed)},
		})
	}
}

// ShowFormatStats 显示格式统计
func (v *Visualizer) ShowFormatStats() {
	stats := v.db.GetStats()

	title := color.New(color.FgGreen, color.Bold)
	title.Println("📄 File Format Statistics")
	title.Println(strings.Repeat("=", 50))

	if len(stats.FormatStats) == 0 {
		fmt.Println("No format data available.")
		return
	}

	// 按文件数量排序
	formats := make([]*FormatStats, 0, len(stats.FormatStats))
	for _, format := range stats.FormatStats {
		formats = append(formats, format)
	}
	sort.Slice(formats, func(i, j int) bool {
		return formats[i].FileCount > formats[j].FileCount
	})

	fmt.Println()
	for i, format := range formats {
		if i > 0 {
			fmt.Println()
		}

		v.printSection(fmt.Sprintf("📋 %s Format", strings.ToUpper(format.Format)), [][]string{
			{"Files Processed", formatNumber(format.FileCount)},
			{"Total Characters", formatNumber(format.CharacterCount)},
			{"Avg File Size", formatNumber(format.AverageFileSize) + " chars"},
			{"Success Rate", fmt.Sprintf("%.1f%%", format.SuccessRate*100)},
			{"Avg Duration", formatDuration(format.AverageDuration)},
			{"Last Used", formatTime(format.LastUsed)},
		})
	}
}

// ShowRecentTranslations 显示最近的翻译
func (v *Visualizer) ShowRecentTranslations(limit int) {
	records := v.db.GetRecentTranslations(limit)

	title := color.New(color.FgBlue, color.Bold)
	title.Printf("🕒 Recent Translations (Last %d)\n", len(records))
	title.Println(strings.Repeat("=", 50))

	if len(records) == 0 {
		fmt.Println("No recent translations found.")
		return
	}

	for i, record := range records {
		if i > 0 {
			fmt.Println()
		}

		status := "✅"
		if record.Status == "failed" || record.FailedNodes > 0 {
			status = "❌"
		}

		title := fmt.Sprintf("%s %s", status, record.InputFile)
		if len(title) > 60 {
			title = title[:57] + "..."
		}

		v.printSection(title, [][]string{
			{"Timestamp", formatTime(record.Timestamp)},
			{"Language", fmt.Sprintf("%s → %s", record.SourceLanguage, record.TargetLanguage)},
			{"Format", record.Format},
			{"Progress", fmt.Sprintf("%.1f%% (%d/%d nodes)", record.Progress, record.CompletedNodes, record.TotalNodes)},
			{"Characters", formatNumber(int64(record.CharacterCount))},
			{"Duration", formatDuration(record.Duration)},
			{"Speed", fmt.Sprintf("%.0f chars/sec", float64(record.CharacterCount)/record.Duration.Seconds())},
		})

		if record.ErrorMessage != "" {
			errorColor := color.New(color.FgRed)
			errorColor.Printf("  ❌ Error: %s\n", record.ErrorMessage)
		}
	}
}

// printCacheStats 打印缓存统计
func (v *Visualizer) printCacheStats(cache CacheStatistics) {
	title := "💾 Cache Statistics"

	data := [][]string{
		{"Cache Directory", cache.CacheDir},
		{"Total Cache Files", formatNumber(cache.TotalCacheFiles)},
		{"Total Cache Size", formatBytes(cache.TotalCacheSize)},
		{"Cache Hit Rate", fmt.Sprintf("%.1f%% (%d hits, %d misses)",
			cache.CacheHitRate*100, cache.CacheHits, cache.CacheMisses)},
	}

	if !cache.OldestCacheEntry.IsZero() {
		data = append(data, []string{"Oldest Entry", formatTime(cache.OldestCacheEntry)})
	}
	if !cache.NewestCacheEntry.IsZero() {
		data = append(data, []string{"Newest Entry", formatTime(cache.NewestCacheEntry)})
	}
	if !cache.LastCleanup.IsZero() {
		data = append(data, []string{"Last Cleanup", formatTime(cache.LastCleanup)})
	}

	v.printSection(title, data)
}

// printSection 打印一个统计部分
func (v *Visualizer) printSection(title string, data [][]string) {
	sectionColor := color.New(color.FgYellow, color.Bold)
	sectionColor.Printf("%s\n", title)

	// 计算最大标签长度
	maxLabelLen := 0
	for _, row := range data {
		if len(row[0]) > maxLabelLen {
			maxLabelLen = len(row[0])
		}
	}

	// 打印数据
	for _, row := range data {
		label := fmt.Sprintf("  %-*s", maxLabelLen, row[0])
		value := row[1]

		labelColor := color.New(color.FgCyan)
		valueColor := color.New(color.FgWhite, color.Bold)

		labelColor.Printf("%s: ", label)
		valueColor.Println(value)
	}
}

// ShowProgressBar 显示进度条
func (v *Visualizer) ShowProgressBar(current, total int, description string) {
	if total == 0 {
		return
	}

	percentage := float64(current) / float64(total) * 100
	barWidth := 40
	filledWidth := int(float64(barWidth) * float64(current) / float64(total))

	bar := strings.Repeat("█", filledWidth) + strings.Repeat("░", barWidth-filledWidth)

	progressColor := color.New(color.FgGreen)
	if percentage < 50 {
		progressColor = color.New(color.FgYellow)
	}
	if percentage < 25 {
		progressColor = color.New(color.FgRed)
	}

	fmt.Printf("\r%s [", description)
	progressColor.Printf("%s", bar)
	fmt.Printf("] %.1f%% (%d/%d)", percentage, current, total)
}

// 辅助函数

// formatNumber 格式化数字（添加千位分隔符）
func formatNumber(n int64) string {
	str := strconv.FormatInt(n, 10)
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

// formatBytes 格式化字节数
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

// formatDuration 格式化持续时间
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
