package stats

import (
	"time"
)

// StatisticsDB 统计数据库结构
type StatisticsDB struct {
	Version     string    `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	LastUpdated time.Time `json:"last_updated"`

	// 总体统计
	TotalTranslations int64         `json:"total_translations"`
	TotalCharacters   int64         `json:"total_characters"`
	TotalFiles        int64         `json:"total_files"`
	TotalErrors       int64         `json:"total_errors"`
	TotalDuration     time.Duration `json:"total_duration"`

	// 缓存统计
	CacheStats CacheStatistics `json:"cache_stats"`

	// 语言对统计
	LanguagePairs map[string]*LanguagePairStats `json:"language_pairs"`

	// 文件格式统计
	FormatStats map[string]*FormatStats `json:"format_stats"`

	// 最近的翻译记录
	RecentTranslations []*TranslationRecord `json:"recent_translations"`

	// 性能统计
	PerformanceStats PerformanceStatistics `json:"performance_stats"`
}

// CacheStatistics 缓存统计信息
type CacheStatistics struct {
	CacheDir         string    `json:"cache_dir"`
	TotalCacheFiles  int64     `json:"total_cache_files"`
	TotalCacheSize   int64     `json:"total_cache_size_bytes"`
	CacheHitRate     float64   `json:"cache_hit_rate"`
	CacheHits        int64     `json:"cache_hits"`
	CacheMisses      int64     `json:"cache_misses"`
	LastCleanup      time.Time `json:"last_cleanup"`
	OldestCacheEntry time.Time `json:"oldest_cache_entry"`
	NewestCacheEntry time.Time `json:"newest_cache_entry"`
}

// LanguagePairStats 语言对统计
type LanguagePairStats struct {
	SourceLanguage   string        `json:"source_language"`
	TargetLanguage   string        `json:"target_language"`
	TranslationCount int64         `json:"translation_count"`
	CharacterCount   int64         `json:"character_count"`
	FileCount        int64         `json:"file_count"`
	ErrorCount       int64         `json:"error_count"`
	AverageDuration  time.Duration `json:"average_duration"`
	LastUsed         time.Time     `json:"last_used"`
}

// FormatStats 文件格式统计
type FormatStats struct {
	Format          string        `json:"format"`
	FileCount       int64         `json:"file_count"`
	CharacterCount  int64         `json:"character_count"`
	AverageFileSize int64         `json:"average_file_size"`
	AverageDuration time.Duration `json:"average_duration"`
	SuccessRate     float64       `json:"success_rate"`
	LastUsed        time.Time     `json:"last_used"`
}

// TranslationRecord 翻译记录
type TranslationRecord struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	InputFile      string    `json:"input_file"`
	OutputFile     string    `json:"output_file"`
	SourceLanguage string    `json:"source_language"`
	TargetLanguage string    `json:"target_language"`
	Format         string    `json:"format"`

	// 统计信息
	TotalNodes     int           `json:"total_nodes"`
	CompletedNodes int           `json:"completed_nodes"`
	FailedNodes    int           `json:"failed_nodes"`
	CharacterCount int           `json:"character_count"`
	Duration       time.Duration `json:"duration"`
	Status         string        `json:"status"`

	// 错误信息
	ErrorMessage string `json:"error_message,omitempty"`

	// 进度信息
	Progress float64 `json:"progress"`

	// 元数据
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// PerformanceStatistics 性能统计
type PerformanceStatistics struct {
	AverageTranslationSpeed float64       `json:"average_translation_speed"` // 字符/秒
	FastestTranslation      time.Duration `json:"fastest_translation"`
	SlowestTranslation      time.Duration `json:"slowest_translation"`
	AverageNodesPerSecond   float64       `json:"average_nodes_per_second"`

	// 内存使用统计
	PeakMemoryUsage    int64 `json:"peak_memory_usage_bytes"`
	AverageMemoryUsage int64 `json:"average_memory_usage_bytes"`

	// 并发统计
	MaxConcurrentJobs     int     `json:"max_concurrent_jobs"`
	AverageConcurrentJobs float64 `json:"average_concurrent_jobs"`
}

// UpdateRequest 更新请求
type UpdateRequest struct {
	TranslationRecord *TranslationRecord `json:"translation_record"`
	CacheUpdate       *CacheUpdate       `json:"cache_update,omitempty"`
	PerformanceData   *PerformanceData   `json:"performance_data,omitempty"`
}

// CacheUpdate 缓存更新数据
type CacheUpdate struct {
	CacheHit        bool  `json:"cache_hit"`
	CacheEntryAdded bool  `json:"cache_entry_added"`
	CacheEntrySize  int64 `json:"cache_entry_size"`
}

// PerformanceData 性能数据
type PerformanceData struct {
	MemoryUsage      int64   `json:"memory_usage"`
	ConcurrentJobs   int     `json:"concurrent_jobs"`
	TranslationSpeed float64 `json:"translation_speed"`
}
