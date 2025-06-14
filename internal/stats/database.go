package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	StatsDBVersion   = "1.0.0"
	MaxRecentRecords = 100
)

// Database 统计数据库
type Database struct {
	filePath string
	data     *StatisticsDB
	mutex    sync.RWMutex
	logger   *zap.Logger
}

// NewDatabase 创建统计数据库
func NewDatabase(filePath string, logger *zap.Logger) (*Database, error) {
	db := &Database{
		filePath: filePath,
		logger:   logger,
	}

	// 确保目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create stats directory: %w", err)
	}

	// 加载或创建数据
	if err := db.load(); err != nil {
		return nil, fmt.Errorf("failed to load stats database: %w", err)
	}

	return db, nil
}

// load 加载统计数据
func (db *Database) load() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	// 检查文件是否存在
	if _, err := os.Stat(db.filePath); os.IsNotExist(err) {
		// 创建新的统计数据
		db.data = &StatisticsDB{
			Version:            StatsDBVersion,
			CreatedAt:          time.Now(),
			LastUpdated:        time.Now(),
			LanguagePairs:      make(map[string]*LanguagePairStats),
			FormatStats:        make(map[string]*FormatStats),
			RecentTranslations: make([]*TranslationRecord, 0),
		}
		return db.saveUnsafe()
	}

	// 读取现有文件
	data, err := os.ReadFile(db.filePath)
	if err != nil {
		return fmt.Errorf("failed to read stats file: %w", err)
	}

	// 解析 JSON
	var statsDB StatisticsDB
	if err := json.Unmarshal(data, &statsDB); err != nil {
		return fmt.Errorf("failed to parse stats file: %w", err)
	}

	// 初始化可能为 nil 的字段
	if statsDB.LanguagePairs == nil {
		statsDB.LanguagePairs = make(map[string]*LanguagePairStats)
	}
	if statsDB.FormatStats == nil {
		statsDB.FormatStats = make(map[string]*FormatStats)
	}
	if statsDB.RecentTranslations == nil {
		statsDB.RecentTranslations = make([]*TranslationRecord, 0)
	}

	db.data = &statsDB
	db.logger.Info("loaded statistics database",
		zap.String("version", statsDB.Version),
		zap.Time("created_at", statsDB.CreatedAt),
		zap.Int64("total_translations", statsDB.TotalTranslations))

	return nil
}

// Save 保存统计数据
func (db *Database) Save() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	return db.saveUnsafe()
}

// saveUnsafe 不安全的保存（需要已持有锁）
func (db *Database) saveUnsafe() error {
	db.data.LastUpdated = time.Now()

	data, err := json.MarshalIndent(db.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal stats data: %w", err)
	}

	// 原子写入
	tempFile := db.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write temp stats file: %w", err)
	}

	if err := os.Rename(tempFile, db.filePath); err != nil {
		return fmt.Errorf("failed to rename stats file: %w", err)
	}

	return nil
}

// AddTranslationRecord 添加翻译记录
func (db *Database) AddTranslationRecord(record *TranslationRecord) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	// 更新总体统计
	db.data.TotalTranslations++
	db.data.TotalCharacters += int64(record.CharacterCount)
	db.data.TotalFiles++
	db.data.TotalDuration += record.Duration

	if record.Status == "failed" || record.FailedNodes > 0 {
		db.data.TotalErrors++
	}

	// 更新语言对统计
	langPairKey := fmt.Sprintf("%s-%s", record.SourceLanguage, record.TargetLanguage)
	langPair, exists := db.data.LanguagePairs[langPairKey]
	if !exists {
		langPair = &LanguagePairStats{
			SourceLanguage: record.SourceLanguage,
			TargetLanguage: record.TargetLanguage,
		}
		db.data.LanguagePairs[langPairKey] = langPair
	}

	langPair.TranslationCount++
	langPair.CharacterCount += int64(record.CharacterCount)
	langPair.FileCount++
	langPair.LastUsed = record.Timestamp

	if record.Status == "failed" || record.FailedNodes > 0 {
		langPair.ErrorCount++
	}

	// 计算平均持续时间
	if langPair.TranslationCount > 0 {
		totalDuration := time.Duration(int64(langPair.AverageDuration) * (langPair.TranslationCount - 1))
		langPair.AverageDuration = (totalDuration + record.Duration) / time.Duration(langPair.TranslationCount)
	}

	// 更新格式统计
	formatStats, exists := db.data.FormatStats[record.Format]
	if !exists {
		formatStats = &FormatStats{
			Format: record.Format,
		}
		db.data.FormatStats[record.Format] = formatStats
	}

	formatStats.FileCount++
	formatStats.CharacterCount += int64(record.CharacterCount)
	formatStats.LastUsed = record.Timestamp

	// 计算平均文件大小
	formatStats.AverageFileSize = formatStats.CharacterCount / formatStats.FileCount

	// 计算成功率
	if record.Status == "completed" && record.FailedNodes == 0 {
		successCount := int64(formatStats.SuccessRate * float64(formatStats.FileCount-1))
		formatStats.SuccessRate = float64(successCount+1) / float64(formatStats.FileCount)
	} else {
		successCount := int64(formatStats.SuccessRate * float64(formatStats.FileCount-1))
		formatStats.SuccessRate = float64(successCount) / float64(formatStats.FileCount)
	}

	// 计算平均持续时间
	if formatStats.FileCount > 0 {
		totalDuration := time.Duration(int64(formatStats.AverageDuration) * (formatStats.FileCount - 1))
		formatStats.AverageDuration = (totalDuration + record.Duration) / time.Duration(formatStats.FileCount)
	}

	// 添加到最近记录
	db.data.RecentTranslations = append(db.data.RecentTranslations, record)

	// 保持最近记录数量限制
	if len(db.data.RecentTranslations) > MaxRecentRecords {
		// 按时间排序
		sort.Slice(db.data.RecentTranslations, func(i, j int) bool {
			return db.data.RecentTranslations[i].Timestamp.After(db.data.RecentTranslations[j].Timestamp)
		})
		db.data.RecentTranslations = db.data.RecentTranslations[:MaxRecentRecords]
	}

	// 更新性能统计
	db.updatePerformanceStats(record)

	return db.saveUnsafe()
}

// updatePerformanceStats 更新性能统计
func (db *Database) updatePerformanceStats(record *TranslationRecord) {
	if record.Duration > 0 && record.CharacterCount > 0 {
		speed := float64(record.CharacterCount) / record.Duration.Seconds()

		// 更新平均翻译速度
		if db.data.TotalTranslations > 1 {
			totalSpeed := db.data.PerformanceStats.AverageTranslationSpeed * float64(db.data.TotalTranslations-1)
			db.data.PerformanceStats.AverageTranslationSpeed = (totalSpeed + speed) / float64(db.data.TotalTranslations)
		} else {
			db.data.PerformanceStats.AverageTranslationSpeed = speed
		}

		// 更新最快/最慢翻译
		if db.data.PerformanceStats.FastestTranslation == 0 || record.Duration < db.data.PerformanceStats.FastestTranslation {
			db.data.PerformanceStats.FastestTranslation = record.Duration
		}

		if record.Duration > db.data.PerformanceStats.SlowestTranslation {
			db.data.PerformanceStats.SlowestTranslation = record.Duration
		}

		// 更新节点处理速度
		if record.TotalNodes > 0 {
			nodesPerSecond := float64(record.TotalNodes) / record.Duration.Seconds()
			if db.data.TotalTranslations > 1 {
				totalNodesSpeed := db.data.PerformanceStats.AverageNodesPerSecond * float64(db.data.TotalTranslations-1)
				db.data.PerformanceStats.AverageNodesPerSecond = (totalNodesSpeed + nodesPerSecond) / float64(db.data.TotalTranslations)
			} else {
				db.data.PerformanceStats.AverageNodesPerSecond = nodesPerSecond
			}
		}
	}
}

// UpdateCacheStats 更新缓存统计
func (db *Database) UpdateCacheStats(cacheDir string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	db.data.CacheStats.CacheDir = cacheDir

	// 扫描缓存目录
	var totalSize int64
	var fileCount int64
	var oldestTime, newestTime time.Time

	err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 忽略错误，继续处理
		}

		if !info.IsDir() {
			fileCount++
			totalSize += info.Size()

			modTime := info.ModTime()
			if oldestTime.IsZero() || modTime.Before(oldestTime) {
				oldestTime = modTime
			}
			if newestTime.IsZero() || modTime.After(newestTime) {
				newestTime = modTime
			}
		}

		return nil
	})
	if err != nil {
		db.logger.Warn("failed to scan cache directory", zap.Error(err))
	}

	db.data.CacheStats.TotalCacheFiles = fileCount
	db.data.CacheStats.TotalCacheSize = totalSize
	db.data.CacheStats.OldestCacheEntry = oldestTime
	db.data.CacheStats.NewestCacheEntry = newestTime

	// 计算命中率
	total := db.data.CacheStats.CacheHits + db.data.CacheStats.CacheMisses
	if total > 0 {
		db.data.CacheStats.CacheHitRate = float64(db.data.CacheStats.CacheHits) / float64(total)
	}

	return db.saveUnsafe()
}

// GetStats 获取统计数据（只读副本）
func (db *Database) GetStats() *StatisticsDB {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	// 创建深拷贝
	data, _ := json.Marshal(db.data)
	var copy StatisticsDB
	json.Unmarshal(data, &copy)

	return &copy
}

// GetRecentTranslations 获取最近的翻译记录
func (db *Database) GetRecentTranslations(limit int) []*TranslationRecord {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	if limit <= 0 || limit > len(db.data.RecentTranslations) {
		limit = len(db.data.RecentTranslations)
	}

	// 按时间排序（最新的在前）
	sorted := make([]*TranslationRecord, len(db.data.RecentTranslations))
	copy(sorted, db.data.RecentTranslations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.After(sorted[j].Timestamp)
	})

	return sorted[:limit]
}

// RecordCacheHit 记录缓存命中
func (db *Database) RecordCacheHit() {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	db.data.CacheStats.CacheHits++
}

// RecordCacheMiss 记录缓存未命中
func (db *Database) RecordCacheMiss() {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	db.data.CacheStats.CacheMisses++
}
