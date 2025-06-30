package translation

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MemoryCache 内存缓存实现
type MemoryCache struct {
	data  map[string]cacheEntry
	mutex sync.RWMutex
	stats CacheStats
}

// cacheEntry 缓存条目
type cacheEntry struct {
	Value     string        `json:"value"`
	Timestamp time.Time     `json:"timestamp"`
	TTL       time.Duration `json:"ttl,omitempty"`
}

// NewMemoryCache 创建内存缓存
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{
		data: make(map[string]cacheEntry),
	}
}

// Get 获取缓存
func (c *MemoryCache) Get(key string) (string, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.data[key]
	if !exists {
		c.stats.Misses++
		return "", false
	}

	// 检查TTL
	if entry.TTL > 0 && time.Since(entry.Timestamp) > entry.TTL {
		// 过期，需要删除
		delete(c.data, key)
		c.stats.Misses++
		return "", false
	}

	c.stats.Hits++
	return entry.Value, true
}

// Set 设置缓存
func (c *MemoryCache) Set(key string, value string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.data[key] = cacheEntry{
		Value:     value,
		Timestamp: time.Now(),
	}
	c.stats.Size = int64(len(c.data))
	return nil
}

// SetWithTTL 设置带过期时间的缓存
func (c *MemoryCache) SetWithTTL(key string, value string, ttl time.Duration) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.data[key] = cacheEntry{
		Value:     value,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
	c.stats.Size = int64(len(c.data))
	return nil
}

// Delete 删除缓存
func (c *MemoryCache) Delete(key string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.data, key)
	c.stats.Size = int64(len(c.data))
	return nil
}

// Clear 清除所有缓存
func (c *MemoryCache) Clear() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.data = make(map[string]cacheEntry)
	c.stats = CacheStats{}
	return nil
}

// Stats 获取缓存统计信息
func (c *MemoryCache) Stats() CacheStats {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.stats
}

// FileCache 文件缓存实现
type FileCache struct {
	basePath string
	memory   *MemoryCache // 二级缓存
	stats    CacheStats
	mutex    sync.RWMutex
}

// NewFileCache 创建文件缓存
func NewFileCache(basePath string) *FileCache {
	// 确保缓存目录存在
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		// 如果创建目录失败，回退到内存缓存
		return &FileCache{
			basePath: "",
			memory:   NewMemoryCache(),
		}
	}

	return &FileCache{
		basePath: basePath,
		memory:   NewMemoryCache(),
	}
}

// generateFileName 根据key生成文件名
func (c *FileCache) generateFileName(key string) string {
	hash := md5.Sum([]byte(key))
	return fmt.Sprintf("%x.cache", hash)
}

// getFilePath 获取缓存文件路径
func (c *FileCache) getFilePath(key string) string {
	if c.basePath == "" {
		return ""
	}
	return filepath.Join(c.basePath, c.generateFileName(key))
}

// Get 获取缓存
func (c *FileCache) Get(key string) (string, bool) {
	// 先检查内存缓存
	if value, ok := c.memory.Get(key); ok {
		c.stats.Hits++
		return value, true
	}

	// 检查文件缓存
	if c.basePath == "" {
		c.stats.Misses++
		return "", false
	}

	filePath := c.getFilePath(key)
	data, err := os.ReadFile(filePath)
	if err != nil {
		c.stats.Misses++
		return "", false
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		c.stats.Misses++
		return "", false
	}

	// 检查TTL
	if entry.TTL > 0 && time.Since(entry.Timestamp) > entry.TTL {
		// 过期，删除文件
		os.Remove(filePath)
		c.stats.Misses++
		return "", false
	}

	// 将结果放入内存缓存
	c.memory.Set(key, entry.Value)
	c.stats.Hits++
	return entry.Value, true
}

// Set 设置缓存
func (c *FileCache) Set(key string, value string) error {
	// 设置内存缓存
	if err := c.memory.Set(key, value); err != nil {
		return err
	}

	// 设置文件缓存
	if c.basePath == "" {
		return nil
	}

	entry := cacheEntry{
		Value:     value,
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	filePath := c.getFilePath(key)
	err = os.WriteFile(filePath, data, 0o644)
	if err == nil {
		c.mutex.Lock()
		c.stats.Size++
		c.mutex.Unlock()
	}
	return err
}

// SetWithTTL 设置带过期时间的缓存
func (c *FileCache) SetWithTTL(key string, value string, ttl time.Duration) error {
	// 设置内存缓存
	if err := c.memory.SetWithTTL(key, value, ttl); err != nil {
		return err
	}

	// 设置文件缓存
	if c.basePath == "" {
		return nil
	}

	entry := cacheEntry{
		Value:     value,
		Timestamp: time.Now(),
		TTL:       ttl,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	filePath := c.getFilePath(key)
	err = os.WriteFile(filePath, data, 0o644)
	if err == nil {
		c.mutex.Lock()
		c.stats.Size++
		c.mutex.Unlock()
	}
	return err
}

// Delete 删除缓存
func (c *FileCache) Delete(key string) error {
	// 删除内存缓存
	c.memory.Delete(key)

	// 删除文件缓存
	if c.basePath == "" {
		return nil
	}

	filePath := c.getFilePath(key)
	err := os.Remove(filePath)
	if err == nil {
		c.mutex.Lock()
		c.stats.Size--
		c.mutex.Unlock()
	}
	return err
}

// Clear 清除所有缓存
func (c *FileCache) Clear() error {
	// 清除内存缓存
	c.memory.Clear()

	// 清除文件缓存
	if c.basePath == "" {
		return nil
	}

	// 删除缓存目录下的所有.cache文件
	files, err := filepath.Glob(filepath.Join(c.basePath, "*.cache"))
	if err != nil {
		return err
	}

	for _, file := range files {
		os.Remove(file)
	}

	c.mutex.Lock()
	c.stats = CacheStats{}
	c.mutex.Unlock()
	return nil
}

// Stats 获取缓存统计信息
func (c *FileCache) Stats() CacheStats {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	memStats := c.memory.Stats()
	return CacheStats{
		Hits:   c.stats.Hits + memStats.Hits,
		Misses: c.stats.Misses + memStats.Misses,
		Size:   c.stats.Size,
	}
}

// CacheKeyComponents 缓存key组件
type CacheKeyComponents struct {
	Step        string  // 翻译步骤名称 (initial, reflection, improvement)
	Provider    string  // 提供商名称 (openai, deepl, etc.)
	Model       string  // 模型名称 (gpt-4o, deepl, etc.)
	SourceLang  string  // 源语言
	TargetLang  string  // 目标语言
	Text        string  // 待翻译文本
	Context     string  // 额外上下文（如reflection中的初始翻译）
	Temperature float32 // 温度参数
	MaxTokens   int     // 最大token数
}

// GenerateCacheKey 生成基于多个组件的缓存key
func GenerateCacheKey(components CacheKeyComponents) string {
	// 构建包含所有关键信息的字符串
	keyData := fmt.Sprintf("step:%s|provider:%s|model:%s|src:%s|tgt:%s|temp:%.2f|tokens:%d|text:%s",
		components.Step,
		components.Provider,
		components.Model,
		components.SourceLang,
		components.TargetLang,
		components.Temperature,
		components.MaxTokens,
		components.Text,
	)

	// 如果有额外上下文，添加进去
	if components.Context != "" {
		keyData += "|context:" + components.Context
	}

	// 生成MD5哈希作为key
	hash := md5.Sum([]byte(keyData))
	return fmt.Sprintf("%x", hash)
}

// NewCache 根据配置创建缓存实例
func NewCache(useCache bool, cacheDir string) Cache {
	if !useCache {
		return nil
	}

	if cacheDir != "" {
		return NewFileCache(cacheDir)
	}

	return NewMemoryCache()
}
