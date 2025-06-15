package translator

import (
	"context"
	
	"github.com/nerdneilsfield/go-translator-agent/internal/document"
)

// BatchTranslateAdapter 适配器，将 BatchTranslator 适配到 NodeTranslationFunc 接口
type BatchTranslateAdapter struct {
	batchTranslator *BatchTranslator
	nodeBuffer      []*document.NodeInfo
	bufferSize      int
}

// NewBatchTranslateAdapter 创建批量翻译适配器
func NewBatchTranslateAdapter(batchTranslator *BatchTranslator, bufferSize int) *BatchTranslateAdapter {
	if bufferSize <= 0 {
		bufferSize = 10 // 默认缓冲区大小
	}
	return &BatchTranslateAdapter{
		batchTranslator: batchTranslator,
		nodeBuffer:      make([]*document.NodeInfo, 0, bufferSize),
		bufferSize:      bufferSize,
	}
}

// TranslateNode 实现 NodeTranslationFunc 接口
func (a *BatchTranslateAdapter) TranslateNode(ctx context.Context, node *document.NodeInfo) error {
	// 将节点添加到缓冲区
	a.nodeBuffer = append(a.nodeBuffer, node)
	
	// 如果缓冲区满了，执行批量翻译
	if len(a.nodeBuffer) >= a.bufferSize {
		return a.flushBuffer(ctx)
	}
	
	return nil
}

// Flush 强制执行缓冲区中的翻译
func (a *BatchTranslateAdapter) Flush(ctx context.Context) error {
	if len(a.nodeBuffer) == 0 {
		return nil
	}
	return a.flushBuffer(ctx)
}

// flushBuffer 执行缓冲区中的批量翻译
func (a *BatchTranslateAdapter) flushBuffer(ctx context.Context) error {
	if len(a.nodeBuffer) == 0 {
		return nil
	}
	
	// 创建节点组
	group := &document.NodeGroup{
		Nodes: a.nodeBuffer,
		Size:  0,
	}
	
	// 计算总大小
	for _, node := range a.nodeBuffer {
		group.Size += len(node.OriginalText)
	}
	
	// 执行批量翻译
	err := a.batchTranslator.translateGroup(ctx, group)
	
	// 清空缓冲区
	a.nodeBuffer = a.nodeBuffer[:0]
	
	return err
}

// TranslateNodeFunc 创建一个包装的翻译函数
func (a *BatchTranslateAdapter) TranslateNodeFunc() document.NodeTranslationFunc {
	return func(ctx context.Context, node *document.NodeInfo) error {
		return a.TranslateNode(ctx, node)
	}
}