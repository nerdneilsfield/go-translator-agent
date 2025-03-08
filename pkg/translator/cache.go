package translator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileCache 是基于文件系统的缓存实现
type FileCache struct {
	cacheDir string
	mutex    sync.RWMutex
}

// CacheEntry 表示缓存条目
type CacheEntry struct {
	Value      string    `json:"value"`
	CreatedAt  time.Time `json:"created_at"`
	AccessedAt time.Time `json:"accessed_at"`
}

// newFileCache 创建一个新的基于文件的缓存
func newFileCache(cacheDir string) *FileCache {
	return &FileCache{
		cacheDir: cacheDir,
	}
}

// Get 从缓存中获取值
func (c *FileCache) Get(key string) (string, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	filePath := filepath.Join(c.cacheDir, key+".json")

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", false
	}

	// 读取文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", false
	}

	// 解析缓存条目
	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return "", false
	}

	// 更新访问时间
	entry.AccessedAt = time.Now()
	updatedData, _ := json.Marshal(entry)
	_ = os.WriteFile(filePath, updatedData, 0644)

	return entry.Value, true
}

// Set 将值存储到缓存中
func (c *FileCache) Set(key string, value string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 确保缓存目录存在
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %w", err)
	}

	// 创建缓存条目
	entry := CacheEntry{
		Value:      value,
		CreatedAt:  time.Now(),
		AccessedAt: time.Now(),
	}

	// 序列化缓存条目
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("序列化缓存条目失败: %w", err)
	}

	// 写入文件
	filePath := filepath.Join(c.cacheDir, key+".json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入缓存文件失败: %w", err)
	}

	return nil
}

// Clear 清除缓存
func (c *FileCache) Clear() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 删除缓存目录中的所有文件
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 目录不存在，没有什么可清除的
		}
		return fmt.Errorf("读取缓存目录失败: %w", err)
	}

	for _, entry := range entries {
		if err := os.Remove(filepath.Join(c.cacheDir, entry.Name())); err != nil {
			return fmt.Errorf("删除缓存文件失败 %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// MemoryCache 是内存中的缓存实现
type MemoryCache struct {
	cache map[string]CacheEntry
	mutex sync.RWMutex
}

// NewMemoryCache 创建一个新的内存缓存
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{
		cache: make(map[string]CacheEntry),
	}
}

// Get 从缓存中获取值
func (c *MemoryCache) Get(key string) (string, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, ok := c.cache[key]
	if !ok {
		return "", false
	}

	// 更新访问时间
	entry.AccessedAt = time.Now()
	c.cache[key] = entry

	return entry.Value, true
}

// Set 将值存储到缓存中
func (c *MemoryCache) Set(key string, value string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cache[key] = CacheEntry{
		Value:      value,
		CreatedAt:  time.Now(),
		AccessedAt: time.Now(),
	}

	return nil
}

// Clear 清除缓存
func (c *MemoryCache) Clear() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cache = make(map[string]CacheEntry)

	return nil
}
