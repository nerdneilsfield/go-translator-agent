package translation

import (
	"context"

	"github.com/nerdneilsfield/go-translator-agent/pkg/providers"
)

// Service 翻译服务核心接口 - 专注多阶段翻译
type Service interface {
	// TranslateText 执行多阶段翻译：initial→reflection→improvement
	// 内部处理raw/none步骤，无分块
	TranslateText(ctx context.Context, text string) (string, error)
}

// Chain 翻译链接口
type Chain interface {
	// Execute 执行翻译链
	Execute(ctx context.Context, input string) (*ChainResult, error)

	// AddStep 添加翻译步骤
	AddStep(step Step) Chain

	// GetSteps 获取所有步骤
	GetSteps() []Step
}

// Step 翻译步骤接口
type Step interface {
	// Execute 执行步骤
	Execute(ctx context.Context, input StepInput) (*StepOutput, error)

	// GetName 获取步骤名称
	GetName() string

	// GetConfig 获取步骤配置
	GetConfig() *StepConfig
}

// LLMClient LLM客户端接口
type LLMClient interface {
	// Complete 生成文本补全
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)

	// Chat 对话接口
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// GetModel 获取模型信息
	GetModel() string

	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error
}

// TranslationProvider 翻译提供商接口（支持 DeepL, Google Translate 等专业服务）
// TranslationProvider 直接使用 providers.TranslationProvider 以避免循环依赖
type TranslationProvider = providers.TranslationProvider

// Cache 缓存接口
type Cache interface {
	// Get 获取缓存
	Get(key string) (string, bool)

	// Set 设置缓存
	Set(key string, value string) error

	// Delete 删除缓存
	Delete(key string) error

	// Clear 清除所有缓存
	Clear() error

	// Stats 获取缓存统计信息
	Stats() CacheStats
}

// Provider 翻译提供者接口
type Provider interface {
	// Name 返回提供者名称
	Name() string

	// Translate 执行翻译
	Translate(ctx context.Context, req *Request) (*Response, error)
}

// MetricsCollector 指标收集器接口
type MetricsCollector interface {
	// RecordTranslation 记录翻译指标
	RecordTranslation(metrics *TranslationMetrics)

	// RecordStep 记录步骤指标
	RecordStep(metrics *StepMetrics)

	// RecordError 记录错误
	RecordError(err error, context map[string]string)

	// GetSummary 获取统计摘要
	GetSummary() *MetricsSummary
}

// ProgressTracker 进度跟踪器接口
type ProgressTracker interface {
	// Start 开始跟踪
	Start(total int)

	// Update 更新进度
	Update(completed int, message string)

	// Complete 完成
	Complete()

	// Error 报告错误
	Error(err error)
}

// Chunker 文本分块器接口
type Chunker interface {
	// Chunk 将文本分块
	Chunk(text string) []string

	// GetConfig 获取分块配置
	GetConfig() ChunkConfig
}

// ChunkConfig 分块配置
type ChunkConfig struct {
	Size    int // 块大小
	Overlap int // 重叠大小
}

// CacheStats 缓存统计信息
type CacheStats struct {
	Hits   int64
	Misses int64
	Size   int64
}
