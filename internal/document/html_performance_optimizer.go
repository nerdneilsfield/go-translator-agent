package document

import (
	"context"
	"runtime"
	"sync"
	"time"

	"go.uber.org/zap"
)

// HTMLPerformanceOptimizer HTML性能优化器
type HTMLPerformanceOptimizer struct {
	logger           *zap.Logger
	config           PerformanceConfig
	workerPool       *WorkerPool
	cacheManager     *TranslationCacheManager
	batchProcessor   *BatchProcessor
	metrics          *PerformanceMetrics
}

// PerformanceConfig 性能配置
type PerformanceConfig struct {
	// 并发设置
	MaxWorkers           int           `json:"maxWorkers"`
	WorkerQueueSize      int           `json:"workerQueueSize"`
	EnableConcurrency    bool          `json:"enableConcurrency"`
	
	// 批处理设置
	BatchSize            int           `json:"batchSize"`
	MaxBatchWaitTime     time.Duration `json:"maxBatchWaitTime"`
	EnableBatching       bool          `json:"enableBatching"`
	
	// 缓存设置
	EnableCaching        bool          `json:"enableCaching"`
	CacheSize            int           `json:"cacheSize"`
	CacheTTL             time.Duration `json:"cacheTTL"`
	
	// 优化设置
	EnablePrefiltering   bool          `json:"enablePrefiltering"`
	EnableDeduplication  bool          `json:"enableDeduplication"`
	EnableCompression    bool          `json:"enableCompression"`
	
	// 限制设置
	MaxConcurrentRequests int          `json:"maxConcurrentRequests"`
	RequestTimeout       time.Duration `json:"requestTimeout"`
	MemoryLimit          int64         `json:"memoryLimit"`
}

// DefaultPerformanceConfig 默认性能配置
func DefaultPerformanceConfig() PerformanceConfig {
	return PerformanceConfig{
		MaxWorkers:            runtime.NumCPU(),
		WorkerQueueSize:       100,
		EnableConcurrency:     true,
		BatchSize:             10,
		MaxBatchWaitTime:      time.Millisecond * 100,
		EnableBatching:        true,
		EnableCaching:         true,
		CacheSize:             1000,
		CacheTTL:              time.Hour,
		EnablePrefiltering:    true,
		EnableDeduplication:   true,
		EnableCompression:     false,
		MaxConcurrentRequests: 5,
		RequestTimeout:        time.Second * 30,
		MemoryLimit:           100 * 1024 * 1024, // 100MB
	}
}

// WorkerPool 工作池
type WorkerPool struct {
	workers    int
	jobQueue   chan TranslationJob
	resultPool chan TranslationResult
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	logger     *zap.Logger
}

// TranslationJob 翻译任务
type TranslationJob struct {
	ID         string              `json:"id"`
	Node       *ExtractableNode    `json:"node"`
	Translator TranslateFunc       `json:"-"`
	Context    context.Context     `json:"-"`
	Priority   int                 `json:"priority"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// TranslationResult 翻译结果
type TranslationResult struct {
	JobID          string        `json:"jobId"`
	Node           *ExtractableNode `json:"node"`
	TranslatedText string        `json:"translatedText"`
	Error          error         `json:"error"`
	Duration       time.Duration `json:"duration"`
	CacheHit       bool          `json:"cacheHit"`
}

// BatchProcessor 批处理器
type BatchProcessor struct {
	batchSize     int
	maxWaitTime   time.Duration
	pendingJobs   []*TranslationJob
	mu            sync.Mutex
	timer         *time.Timer
	processor     func([]*TranslationJob) []*TranslationResult
	logger        *zap.Logger
}

// TranslationCacheManager 翻译缓存管理器
type TranslationCacheManager struct {
	cache    map[string]*CacheEntry
	mu       sync.RWMutex
	maxSize  int
	ttl      time.Duration
	logger   *zap.Logger
}

// CacheEntry 缓存条目
type CacheEntry struct {
	TranslatedText string    `json:"translatedText"`
	Timestamp      time.Time `json:"timestamp"`
	AccessCount    int       `json:"accessCount"`
	LastAccessed   time.Time `json:"lastAccessed"`
}

// PerformanceMetrics 性能指标
type PerformanceMetrics struct {
	mu                  sync.RWMutex
	TotalJobs           int64         `json:"totalJobs"`
	CompletedJobs       int64         `json:"completedJobs"`
	FailedJobs          int64         `json:"failedJobs"`
	CacheHits           int64         `json:"cacheHits"`
	CacheMisses         int64         `json:"cacheMisses"`
	AverageResponseTime time.Duration `json:"averageResponseTime"`
	TotalResponseTime   time.Duration `json:"totalResponseTime"`
	PeakConcurrency     int           `json:"peakConcurrency"`
	CurrentConcurrency  int           `json:"currentConcurrency"`
	MemoryUsage         int64         `json:"memoryUsage"`
	StartTime           time.Time     `json:"startTime"`
}

// NewHTMLPerformanceOptimizer 创建HTML性能优化器
func NewHTMLPerformanceOptimizer(logger *zap.Logger, config PerformanceConfig) *HTMLPerformanceOptimizer {
	optimizer := &HTMLPerformanceOptimizer{
		logger:         logger,
		config:         config,
		cacheManager:   NewTranslationCacheManager(config.CacheSize, config.CacheTTL, logger),
		batchProcessor: NewBatchProcessor(config.BatchSize, config.MaxBatchWaitTime, logger),
		metrics:        NewPerformanceMetrics(),
	}

	if config.EnableConcurrency {
		optimizer.workerPool = NewWorkerPool(config.MaxWorkers, config.WorkerQueueSize, logger)
	}

	return optimizer
}

// OptimizeTranslation 优化翻译处理
func (opt *HTMLPerformanceOptimizer) OptimizeTranslation(ctx context.Context, nodes []*ExtractableNode, translator TranslateFunc) ([]*TranslationResult, error) {
	startTime := time.Now()
	opt.metrics.TotalJobs += int64(len(nodes))

	// 预过滤无效节点
	if opt.config.EnablePrefiltering {
		nodes = opt.prefilterNodes(nodes)
	}

	// 去重处理
	if opt.config.EnableDeduplication {
		nodes = opt.deduplicateNodes(nodes)
	}

	var results []*TranslationResult

	// 选择处理策略
	if opt.config.EnableConcurrency && len(nodes) > opt.config.BatchSize {
		results = opt.processConcurrently(ctx, nodes, translator)
	} else if opt.config.EnableBatching && len(nodes) > 1 {
		results = opt.processBatched(ctx, nodes, translator)
	} else {
		results = opt.processSequentially(ctx, nodes, translator)
	}

	// 更新性能指标
	duration := time.Since(startTime)
	opt.updateMetrics(results, duration)

	opt.logger.Info("translation optimization completed",
		zap.Int("inputNodes", len(nodes)),
		zap.Int("results", len(results)),
		zap.Duration("duration", duration),
		zap.Int64("cacheHits", opt.metrics.CacheHits),
		zap.Int64("cacheMisses", opt.metrics.CacheMisses))

	return results, nil
}

// prefilterNodes 预过滤节点
func (opt *HTMLPerformanceOptimizer) prefilterNodes(nodes []*ExtractableNode) []*ExtractableNode {
	var filtered []*ExtractableNode

	for _, node := range nodes {
		// 跳过空文本
		if node.Text == "" {
			continue
		}

		// 跳过过短文本
		if len(node.Text) < 2 {
			continue
		}

		// 跳过只包含标点符号的文本
		if opt.isOnlyPunctuation(node.Text) {
			continue
		}

		// 跳过数字
		if opt.isOnlyNumbers(node.Text) {
			continue
		}

		filtered = append(filtered, node)
	}

	opt.logger.Debug("prefiltered nodes",
		zap.Int("original", len(nodes)),
		zap.Int("filtered", len(filtered)))

	return filtered
}

// deduplicateNodes 去重节点
func (opt *HTMLPerformanceOptimizer) deduplicateNodes(nodes []*ExtractableNode) []*ExtractableNode {
	seen := make(map[string]*ExtractableNode)
	var deduplicated []*ExtractableNode

	for _, node := range nodes {
		if existing, exists := seen[node.Text]; exists {
			// 如果已存在相同文本，选择优先级更高的节点
			if opt.getNodePriority(node) > opt.getNodePriority(existing) {
				seen[node.Text] = node
			}
		} else {
			seen[node.Text] = node
		}
	}

	for _, node := range seen {
		deduplicated = append(deduplicated, node)
	}

	opt.logger.Debug("deduplicated nodes",
		zap.Int("original", len(nodes)),
		zap.Int("deduplicated", len(deduplicated)))

	return deduplicated
}

// processConcurrently 并发处理
func (opt *HTMLPerformanceOptimizer) processConcurrently(ctx context.Context, nodes []*ExtractableNode, translator TranslateFunc) []*TranslationResult {
	if opt.workerPool == nil {
		return opt.processSequentially(ctx, nodes, translator)
	}

	// 创建任务
	jobs := make([]*TranslationJob, len(nodes))
	for i, node := range nodes {
		jobs[i] = &TranslationJob{
			ID:         generateJobID(),
			Node:       node,
			Translator: translator,
			Context:    ctx,
			Priority:   opt.getNodePriority(node),
		}
	}

	// 提交任务
	return opt.workerPool.ProcessJobs(jobs)
}

// processBatched 批处理
func (opt *HTMLPerformanceOptimizer) processBatched(ctx context.Context, nodes []*ExtractableNode, translator TranslateFunc) []*TranslationResult {
	var results []*TranslationResult

	// 分批处理
	for i := 0; i < len(nodes); i += opt.config.BatchSize {
		end := i + opt.config.BatchSize
		if end > len(nodes) {
			end = len(nodes)
		}

		batch := nodes[i:end]
		batchResults := opt.processBatch(ctx, batch, translator)
		results = append(results, batchResults...)
	}

	return results
}

// processSequentially 顺序处理
func (opt *HTMLPerformanceOptimizer) processSequentially(ctx context.Context, nodes []*ExtractableNode, translator TranslateFunc) []*TranslationResult {
	results := make([]*TranslationResult, len(nodes))

	for i, node := range nodes {
		result := opt.processNode(ctx, node, translator)
		results[i] = result
	}

	return results
}

// processNode 处理单个节点
func (opt *HTMLPerformanceOptimizer) processNode(ctx context.Context, node *ExtractableNode, translator TranslateFunc) *TranslationResult {
	startTime := time.Now()

	// 检查缓存
	if opt.config.EnableCaching {
		if cached := opt.cacheManager.Get(node.Text); cached != nil {
			opt.metrics.CacheHits++
			return &TranslationResult{
				Node:           node,
				TranslatedText: cached.TranslatedText,
				Duration:       time.Since(startTime),
				CacheHit:       true,
			}
		}
		opt.metrics.CacheMisses++
	}

	// 执行翻译
	translatedText, err := translator(ctx, node.Text)
	duration := time.Since(startTime)

	result := &TranslationResult{
		Node:           node,
		TranslatedText: translatedText,
		Error:          err,
		Duration:       duration,
		CacheHit:       false,
	}

	// 缓存结果
	if opt.config.EnableCaching && err == nil && translatedText != "" {
		opt.cacheManager.Set(node.Text, translatedText)
	}

	return result
}

// processBatch 处理批次
func (opt *HTMLPerformanceOptimizer) processBatch(ctx context.Context, nodes []*ExtractableNode, translator TranslateFunc) []*TranslationResult {
	results := make([]*TranslationResult, len(nodes))

	// 简单实现：逐个处理（实际批处理需要支持批量翻译的translator）
	for i, node := range nodes {
		results[i] = opt.processNode(ctx, node, translator)
	}

	return results
}

// 辅助方法

func (opt *HTMLPerformanceOptimizer) isOnlyPunctuation(text string) bool {
	for _, char := range text {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}

func (opt *HTMLPerformanceOptimizer) isOnlyNumbers(text string) bool {
	for _, char := range text {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (opt *HTMLPerformanceOptimizer) getNodePriority(node *ExtractableNode) int {
	// 基于元素类型和属性计算优先级
	priority := 1

	if node.IsAttribute {
		priority += 2
	}

	switch node.ParentTag {
	case "h1", "h2", "h3":
		priority += 10
	case "title":
		priority += 20
	case "p":
		priority += 3
	case "span", "div":
		priority += 1
	}

	return priority
}

func (opt *HTMLPerformanceOptimizer) updateMetrics(results []*TranslationResult, duration time.Duration) {
	opt.metrics.mu.Lock()
	defer opt.metrics.mu.Unlock()

	for _, result := range results {
		opt.metrics.CompletedJobs++
		if result.Error != nil {
			opt.metrics.FailedJobs++
		}

		opt.metrics.TotalResponseTime += result.Duration
	}

	if opt.metrics.CompletedJobs > 0 {
		opt.metrics.AverageResponseTime = opt.metrics.TotalResponseTime / time.Duration(opt.metrics.CompletedJobs)
	}
}

// GetMetrics 获取性能指标
func (opt *HTMLPerformanceOptimizer) GetMetrics() *PerformanceMetrics {
	opt.metrics.mu.RLock()
	defer opt.metrics.mu.RUnlock()

	// 返回副本
	metrics := *opt.metrics
	return &metrics
}

// 工厂函数

func NewWorkerPool(workers, queueSize int, logger *zap.Logger) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	
	pool := &WorkerPool{
		workers:    workers,
		jobQueue:   make(chan TranslationJob, queueSize),
		resultPool: make(chan TranslationResult, queueSize),
		ctx:        ctx,
		cancel:     cancel,
		logger:     logger,
	}

	pool.start()
	return pool
}

func (wp *WorkerPool) start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
}

func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	for {
		select {
		case job := <-wp.jobQueue:
			result := wp.processJob(job)
			select {
			case wp.resultPool <- result:
			case <-wp.ctx.Done():
				return
			}
		case <-wp.ctx.Done():
			return
		}
	}
}

func (wp *WorkerPool) processJob(job TranslationJob) TranslationResult {
	startTime := time.Now()

	translatedText, err := job.Translator(job.Context, job.Node.Text)

	return TranslationResult{
		JobID:          job.ID,
		Node:           job.Node,
		TranslatedText: translatedText,
		Error:          err,
		Duration:       time.Since(startTime),
	}
}

func (wp *WorkerPool) ProcessJobs(jobs []*TranslationJob) []*TranslationResult {
	// 提交所有任务
	for _, job := range jobs {
		select {
		case wp.jobQueue <- *job:
		case <-wp.ctx.Done():
			return nil
		}
	}

	// 收集结果
	results := make([]*TranslationResult, 0, len(jobs))
	for i := 0; i < len(jobs); i++ {
		select {
		case result := <-wp.resultPool:
			results = append(results, &result)
		case <-wp.ctx.Done():
			return results
		}
	}

	return results
}

func (wp *WorkerPool) Close() {
	wp.cancel()
	close(wp.jobQueue)
	wp.wg.Wait()
	close(wp.resultPool)
}

func NewTranslationCacheManager(maxSize int, ttl time.Duration, logger *zap.Logger) *TranslationCacheManager {
	return &TranslationCacheManager{
		cache:   make(map[string]*CacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
		logger:  logger,
	}
}

func (cm *TranslationCacheManager) Get(key string) *CacheEntry {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	entry, exists := cm.cache[key]
	if !exists {
		return nil
	}

	// 检查TTL
	if time.Since(entry.Timestamp) > cm.ttl {
		delete(cm.cache, key)
		return nil
	}

	// 更新访问信息
	entry.AccessCount++
	entry.LastAccessed = time.Now()

	return entry
}

func (cm *TranslationCacheManager) Set(key, value string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 检查缓存大小限制
	if len(cm.cache) >= cm.maxSize {
		cm.evictLRU()
	}

	cm.cache[key] = &CacheEntry{
		TranslatedText: value,
		Timestamp:      time.Now(),
		AccessCount:    1,
		LastAccessed:   time.Now(),
	}
}

func (cm *TranslationCacheManager) evictLRU() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range cm.cache {
		if oldestKey == "" || entry.LastAccessed.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.LastAccessed
		}
	}

	if oldestKey != "" {
		delete(cm.cache, oldestKey)
	}
}

func NewBatchProcessor(batchSize int, maxWaitTime time.Duration, logger *zap.Logger) *BatchProcessor {
	return &BatchProcessor{
		batchSize:   batchSize,
		maxWaitTime: maxWaitTime,
		logger:      logger,
	}
}

func NewPerformanceMetrics() *PerformanceMetrics {
	return &PerformanceMetrics{
		StartTime: time.Now(),
	}
}

func generateJobID() string {
	return time.Now().Format("20060102150405.000000")
}