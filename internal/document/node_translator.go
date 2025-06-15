package document

import (
	"context"
	"fmt"
)

// ProgressReporter 进度报告接口（避免循环依赖）
type ProgressReporter interface {
	StartDocument(docID, fileName string, totalNodes int)
	UpdateNode(docID string, nodeID int, status NodeStatus, charCount int, err error)
	CompleteDocument(docID string)
	UpdateStep(docID string, nodeID int, step int, stepName string)
}

// NodeInfoTranslator 节点翻译器，管理节点的翻译流程
type NodeInfoTranslator struct {
	collection       *NodeCollection
	grouper          *NodeGrouper
	contextBuilder   *NodeContextBuilder
	retryManager     *NodeRetryManager
	progressReporter ProgressReporter
}

// NewNodeInfoTranslator 创建节点翻译器
func NewNodeInfoTranslator(maxChunkSize int, contextDistance int, maxRetries int) *NodeInfoTranslator {
	collection := NewNodeCollection()
	return &NodeInfoTranslator{
		collection:     collection,
		grouper:        NewNodeGrouper(maxChunkSize),
		contextBuilder: NewNodeContextBuilder(collection, contextDistance),
		retryManager:   NewNodeRetryManager(collection, maxChunkSize, contextDistance, maxRetries),
	}
}

// NewNodeInfoTranslatorWithProgress 创建带进度报告的节点翻译器
func NewNodeInfoTranslatorWithProgress(maxChunkSize int, contextDistance int, maxRetries int, progressReporter ProgressReporter) *NodeInfoTranslator {
	translator := NewNodeInfoTranslator(maxChunkSize, contextDistance, maxRetries)
	translator.progressReporter = progressReporter
	return translator
}

// GetCollection 获取节点集合
func (t *NodeInfoTranslator) GetCollection() *NodeCollection {
	return t.collection
}

// GetGrouper 获取分组器
func (t *NodeInfoTranslator) GetGrouper() *NodeGrouper {
	return t.grouper
}

// GetRetryManager 获取重试管理器
func (t *NodeInfoTranslator) GetRetryManager() *NodeRetryManager {
	return t.retryManager
}

// GetContextBuilder 获取上下文构建器
func (t *NodeInfoTranslator) GetContextBuilder() *NodeContextBuilder {
	return t.contextBuilder
}

// SetProgressReporter 设置进度报告器
func (t *NodeInfoTranslator) SetProgressReporter(progressReporter ProgressReporter) {
	t.progressReporter = progressReporter
}

// TranslateDocument 翻译文档中的所有节点
func (t *NodeInfoTranslator) TranslateDocument(ctx context.Context, docID, fileName string, nodes []*NodeInfo, translator NodeTranslationFunc) error {
	// 将所有节点添加到集合中
	for _, node := range nodes {
		t.collection.Add(node)
	}
	
	// 报告开始翻译
	if t.progressReporter != nil {
		t.progressReporter.StartDocument(docID, fileName, len(nodes))
	}

	defer func() {
		// 报告完成翻译
		if t.progressReporter != nil {
			t.progressReporter.CompleteDocument(docID)
		}
	}()

	// 分组处理节点
	groups := t.grouper.GroupNodes(nodes)

	for _, group := range groups {
		err := t.translateGroup(ctx, docID, &group, translator)
		if err != nil {
			return fmt.Errorf("failed to translate group: %w", err)
		}
	}

	// 处理重试
	if t.retryManager != nil {
		// 使用重试管理器处理失败节点
		maxRetries := 3
		for retry := 0; retry < maxRetries; retry++ {
			// 准备重试组（包含上下文）
			retryGroups, err := t.retryManager.PrepareRetryGroups()
			if err != nil {
				return fmt.Errorf("failed to prepare retry groups: %w", err)
			}
			
			if len(retryGroups) == 0 {
				break // 没有需要重试的节点
			}
			
			// 重试每个组
			for _, group := range retryGroups {
				err := t.translateGroup(ctx, docID, &group, translator)
				if err != nil {
					// 记录错误但继续处理其他组
					continue
				}
			}
			
			// 重置已处理节点集合，为下一轮重试做准备
			t.retryManager.ResetProcessedNodes()
		}
	}

	return nil
}

// NodeTranslationFunc 节点翻译函数类型
type NodeTranslationFunc func(ctx context.Context, node *NodeInfo) error

// translateGroup 翻译节点组
func (t *NodeInfoTranslator) translateGroup(ctx context.Context, docID string, group *NodeGroup, translator NodeTranslationFunc) error {
	for _, node := range group.Nodes {
		err := t.translateNode(ctx, docID, node, translator)
		if err != nil {
			// 报告失败状态
			if t.progressReporter != nil {
				t.progressReporter.UpdateNode(docID, node.ID, NodeStatusFailed, node.GetCharCount(), err)
			}
			// 继续处理其他节点，不中断整个流程
			continue
		}

		// 报告成功状态
		if t.progressReporter != nil {
			t.progressReporter.UpdateNode(docID, node.ID, NodeStatusSuccess, node.GetCharCount(), nil)
		}
	}
	return nil
}

// translateNode 翻译单个节点
func (t *NodeInfoTranslator) translateNode(ctx context.Context, docID string, node *NodeInfo, translator NodeTranslationFunc) error {
	// 报告开始处理
	if t.progressReporter != nil {
		t.progressReporter.UpdateNode(docID, node.ID, NodeStatusPending, node.GetCharCount(), nil)
	}

	// 执行实际翻译
	err := translator(ctx, node)
	
	// 更新节点状态到集合中
	if err != nil {
		t.collection.Update(node.ID, func(n *NodeInfo) {
			n.Status = NodeStatusFailed
			n.Error = err
		})
	} else {
		t.collection.Update(node.ID, func(n *NodeInfo) {
			n.Status = NodeStatusSuccess
		})
	}
	
	return err
}

// retryFailedNodes 重试失败的节点
func (t *NodeInfoTranslator) retryFailedNodes(ctx context.Context, docID string, failedNodes []*NodeInfo, translator NodeTranslationFunc) error {
	for _, node := range failedNodes {
		// 报告重试状态
		if t.progressReporter != nil {
			t.progressReporter.UpdateNode(docID, node.ID, NodeStatusRetrying, node.GetCharCount(), nil)
		}

		err := translator(ctx, node)
		if err != nil {
			// 报告最终失败
			if t.progressReporter != nil {
				t.progressReporter.UpdateNode(docID, node.ID, NodeStatusFailed, node.GetCharCount(), err)
			}
		} else {
			// 报告重试成功
			if t.progressReporter != nil {
				t.progressReporter.UpdateNode(docID, node.ID, NodeStatusSuccess, node.GetCharCount(), nil)
			}
		}
	}
	return nil
}
