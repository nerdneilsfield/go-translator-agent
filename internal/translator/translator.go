package translator

import (
	"context"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// Translator 节点翻译管理器接口
// 负责节点分组、并行控制、错误重试等节点级别的翻译管理
type Translator interface {
	// TranslateNodes 翻译节点列表
	// 内部会进行智能分组、并行处理、错误重试
	TranslateNodes(ctx context.Context, nodes []*document.NodeInfo) error
}

// NewTranslatorConfig 从全局配置创建Translator专用配置
func NewTranslatorConfig(cfg *config.Config) TranslatorConfig {
	// 将配置文件中的智能分割配置转换为translation包的配置格式
	smartSplitterConfig := translation.SmartNodeSplitterConfig{
		EnableSmartSplitting: cfg.SmartNodeSplitting.EnableSmartSplitting,
		MaxNodeSizeThreshold: cfg.SmartNodeSplitting.MaxNodeSizeThreshold,
		MinSplitSize:         cfg.SmartNodeSplitting.MinSplitSize,
		MaxSplitSize:         cfg.SmartNodeSplitting.MaxSplitSize,
		PreserveParagraphs:   cfg.SmartNodeSplitting.PreserveParagraphs,
		PreserveSentences:    cfg.SmartNodeSplitting.PreserveSentences,
		OverlapRatio:         cfg.SmartNodeSplitting.OverlapRatio,
	}

	// 如果配置为空，使用默认值
	if smartSplitterConfig.MaxNodeSizeThreshold == 0 {
		defaultConfig := translation.DefaultSmartNodeSplitterConfig()
		smartSplitterConfig = defaultConfig
	}

	return TranslatorConfig{
		ChunkSize:      cfg.ChunkSize,
		Concurrency:    cfg.Concurrency,
		MaxRetries:     cfg.RetryAttempts,
		GroupingMode:   "smart", // 默认智能分组
		RetryOnFailure: cfg.RetryFailedParts,
		SmartSplitter:  smartSplitterConfig,
		SourceLang:     cfg.SourceLang,
		TargetLang:     cfg.TargetLang,
		Verbose:        cfg.Verbose,
	}
}

// ProgressCallback 进度回调函数
type ProgressCallback func(completed, total int, message string)

// TranslatorConfig Translator包专用配置，管理节点分组和并行相关功能
type TranslatorConfig struct {
	// 分组和并行配置
	ChunkSize      int    // 分组时的大小限制（字符数）
	Concurrency    int    // 并行翻译的组数
	MaxRetries     int    // 最大重试次数
	GroupingMode   string // 分组模式: "smart" 或 "fixed"
	RetryOnFailure bool   // 是否在失败时重试

	// 智能节点分割配置
	SmartSplitter translation.SmartNodeSplitterConfig // 智能节点分割器配置

	// 语言配置
	SourceLang string // 源语言
	TargetLang string // 目标语言

	// 进度和调试配置
	Verbose    bool             // 详细模式
	OnProgress ProgressCallback // 进度回调
}
