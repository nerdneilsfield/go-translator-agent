package translator

import (
	"context"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
)

// Translator 节点翻译管理器接口
// 负责节点分组、并行控制、错误重试等节点级别的翻译管理
type Translator interface {
	// TranslateNodes 翻译节点列表
	// 内部会进行智能分组、并行处理、错误重试
	TranslateNodes(ctx context.Context, nodes []*document.NodeInfo) error
}

// ProgressCallback 进度回调函数
type ProgressCallback func(completed, total int, message string)

// TranslatorConfig 翻译器配置
type TranslatorConfig struct {
	ChunkSize      int              // 分组大小（字符数）
	Concurrency    int              // 并行度
	MaxRetries     int              // 最大重试次数
	SourceLang     string           // 源语言
	TargetLang     string           // 目标语言
	Verbose        bool             // 详细模式
	OnProgress     ProgressCallback // 进度回调
}