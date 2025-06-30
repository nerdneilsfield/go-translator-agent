package document

import (
	"fmt"
	"sync"
)

// NodeStatus 节点翻译状态
type NodeStatus int

const (
	NodeStatusPending  NodeStatus = iota // 待翻译
	NodeStatusRetrying                   // 正在重试
	NodeStatusSuccess                    // 翻译成功
	NodeStatusFailed                     // 翻译失败
	NodeStatusSkipped                    // 跳过翻译
)

// NodeInfo 通用的翻译节点信息
type NodeInfo struct {
	// ID 节点的全局唯一标识符
	ID int

	// BlockID 关联的文档块ID（如果有）
	BlockID string

	// OriginalText 原始文本
	OriginalText string

	// TranslatedText 翻译后的文本
	TranslatedText string

	// Status 翻译状态
	Status NodeStatus

	// Path 节点在文档中的路径（如 HTML 的 DOM 路径）
	Path string

	// ContextBefore 上文内容（用于重试时提供上下文）
	ContextBefore string

	// ContextAfter 下文内容（用于重试时提供上下文）
	ContextAfter string

	// Metadata 节点的元数据（如格式信息、属性等）
	Metadata map[string]interface{}

	// Error 翻译错误（如果有）
	Error error

	// RetryCount 重试次数
	RetryCount int

	// Parent 父节点（用于智能分割的子节点）
	Parent *NodeInfo

	// SplitIndex 分割索引（如果是分割子节点）
	SplitIndex int

	// Type 节点类型（用于智能分割识别）
	Type string
}

// GetCharCount 获取节点的字符数
func (n *NodeInfo) GetCharCount() int {
	return len([]rune(n.OriginalText))
}

// IsTranslated 检查节点是否已翻译
func (n *NodeInfo) IsTranslated() bool {
	return n.Status == NodeStatusSuccess && n.TranslatedText != ""
}

// NeedsRetry 检查节点是否需要重试
func (n *NodeInfo) NeedsRetry() bool {
	return n.Status == NodeStatusFailed && n.RetryCount < 3
}

// NodeCollection 节点集合，管理所有翻译节点
type NodeCollection struct {
	nodes    map[int]*NodeInfo
	nodeList []*NodeInfo // 保持顺序
	mu       sync.RWMutex
}

// NewNodeCollection 创建新的节点集合
func NewNodeCollection() *NodeCollection {
	return &NodeCollection{
		nodes:    make(map[int]*NodeInfo),
		nodeList: make([]*NodeInfo, 0),
	}
}

// Add 添加节点
func (nc *NodeCollection) Add(node *NodeInfo) {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if _, exists := nc.nodes[node.ID]; !exists {
		nc.nodes[node.ID] = node
		nc.nodeList = append(nc.nodeList, node)
	}
}

// Get 获取节点
func (nc *NodeCollection) Get(id int) (*NodeInfo, bool) {
	nc.mu.RLock()
	defer nc.mu.RUnlock()

	node, exists := nc.nodes[id]
	return node, exists
}

// GetAll 获取所有节点（按添加顺序）
func (nc *NodeCollection) GetAll() []*NodeInfo {
	nc.mu.RLock()
	defer nc.mu.RUnlock()

	result := make([]*NodeInfo, len(nc.nodeList))
	copy(result, nc.nodeList)
	return result
}

// GetByStatus 获取指定状态的节点
func (nc *NodeCollection) GetByStatus(status NodeStatus) []*NodeInfo {
	nc.mu.RLock()
	defer nc.mu.RUnlock()

	var result []*NodeInfo
	for _, node := range nc.nodeList {
		if node.Status == status {
			result = append(result, node)
		}
	}
	return result
}

// GetFailedNodes 获取翻译失败的节点
func (nc *NodeCollection) GetFailedNodes() []*NodeInfo {
	return nc.GetByStatus(NodeStatusFailed)
}

// Update 更新节点信息
func (nc *NodeCollection) Update(id int, updater func(*NodeInfo)) error {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	node, exists := nc.nodes[id]
	if !exists {
		return fmt.Errorf("node %d not found", id)
	}

	updater(node)
	return nil
}

// Size 返回节点总数
func (nc *NodeCollection) Size() int {
	nc.mu.RLock()
	defer nc.mu.RUnlock()

	return len(nc.nodes)
}

// NodeGroup 节点组，用于批量翻译
type NodeGroup struct {
	Nodes []*NodeInfo
	Size  int // 组内文本总大小
}

// NodeGrouper 节点分组器
type NodeGrouper struct {
	maxChunkSize int
}

// NewNodeGrouper 创建节点分组器
func NewNodeGrouper(maxChunkSize int) *NodeGrouper {
	return &NodeGrouper{
		maxChunkSize: maxChunkSize,
	}
}

// GroupNodes 将节点分组
func (g *NodeGrouper) GroupNodes(nodes []*NodeInfo) []NodeGroup {
	var groups []NodeGroup
	var currentGroup NodeGroup
	currentSize := 0

	for _, node := range nodes {
		nodeSize := len(node.OriginalText)

		// 如果单个节点超过限制，单独成组
		if nodeSize > g.maxChunkSize {
			// 先保存当前组
			if len(currentGroup.Nodes) > 0 {
				currentGroup.Size = currentSize
				groups = append(groups, currentGroup)
				currentGroup = NodeGroup{}
				currentSize = 0
			}

			// 单独成组
			groups = append(groups, NodeGroup{
				Nodes: []*NodeInfo{node},
				Size:  nodeSize,
			})
			continue
		}

		// 如果加入当前节点会超出限制，先保存当前组
		if currentSize+nodeSize > g.maxChunkSize && len(currentGroup.Nodes) > 0 {
			currentGroup.Size = currentSize
			groups = append(groups, currentGroup)
			currentGroup = NodeGroup{}
			currentSize = 0
		}

		// 添加到当前组
		currentGroup.Nodes = append(currentGroup.Nodes, node)
		currentSize += nodeSize
	}

	// 保存最后一组
	if len(currentGroup.Nodes) > 0 {
		currentGroup.Size = currentSize
		groups = append(groups, currentGroup)
	}

	return groups
}

// NodeContextBuilder 节点上下文构建器
type NodeContextBuilder struct {
	collection      *NodeCollection
	contextDistance int // 上下文距离（前后各取几个节点）
}

// NewNodeContextBuilder 创建上下文构建器
func NewNodeContextBuilder(collection *NodeCollection, contextDistance int) *NodeContextBuilder {
	return &NodeContextBuilder{
		collection:      collection,
		contextDistance: contextDistance,
	}
}

// BuildContextForFailedNodes 为失败节点构建上下文
func (b *NodeContextBuilder) BuildContextForFailedNodes(failedNodes []*NodeInfo) ([]*NodeInfo, error) {
	// 使用 map 去重
	nodeSet := make(map[int]*NodeInfo)
	allNodes := b.collection.GetAll()

	// 创建 ID 到索引的映射
	idToIndex := make(map[int]int)
	for i, node := range allNodes {
		idToIndex[node.ID] = i
	}

	// 为每个失败节点添加上下文
	for _, failedNode := range failedNodes {
		idx, exists := idToIndex[failedNode.ID]
		if !exists {
			continue
		}

		// 添加前面的节点
		for i := idx - b.contextDistance; i <= idx-1; i++ {
			if i >= 0 && i < len(allNodes) {
				nodeSet[allNodes[i].ID] = allNodes[i]
			}
		}

		// 添加失败节点本身
		nodeSet[failedNode.ID] = failedNode

		// 添加后面的节点
		for i := idx + 1; i <= idx+b.contextDistance; i++ {
			if i < len(allNodes) {
				nodeSet[allNodes[i].ID] = allNodes[i]
			}
		}
	}

	// 转换为有序列表
	var result []*NodeInfo
	for _, node := range allNodes {
		if _, exists := nodeSet[node.ID]; exists {
			result = append(result, node)
		}
	}

	return result, nil
}

// IntSet 整数集合，用于去重（已在 utils.go 中定义，这里引用说明）
// 使用 IntSet 来避免重复添加节点，防止无限递归

// NodeRetryManager 节点重试管理器
type NodeRetryManager struct {
	collection     *NodeCollection
	contextBuilder *NodeContextBuilder
	grouper        *NodeGrouper
	maxRetries     int
	processedNodes *IntSet // 使用 IntSet 追踪已处理的节点，避免无限递归
}

// NewNodeRetryManager 创建重试管理器
func NewNodeRetryManager(collection *NodeCollection, maxChunkSize int, contextDistance int, maxRetries int) *NodeRetryManager {
	return &NodeRetryManager{
		collection:     collection,
		contextBuilder: NewNodeContextBuilder(collection, contextDistance),
		grouper:        NewNodeGrouper(maxChunkSize),
		maxRetries:     maxRetries,
		processedNodes: NewIntSet(),
	}
}

// PrepareRetryGroups 准备重试组
func (m *NodeRetryManager) PrepareRetryGroups() ([]NodeGroup, error) {
	// 获取失败的节点
	failedNodes := m.collection.GetFailedNodes()
	if len(failedNodes) == 0 {
		return nil, nil
	}

	// 过滤出需要重试的节点
	var nodesToRetry []*NodeInfo
	for _, node := range failedNodes {
		if node.NeedsRetry() && !m.processedNodes.Contains(node.ID) {
			nodesToRetry = append(nodesToRetry, node)
			// 标记为已处理，避免重复处理
			m.processedNodes.Add(node.ID)
		}
	}

	if len(nodesToRetry) == 0 {
		return nil, nil
	}

	// 为失败节点构建上下文
	nodesWithContext, err := m.contextBuilder.BuildContextForFailedNodes(nodesToRetry)
	if err != nil {
		return nil, fmt.Errorf("failed to build context: %w", err)
	}

	// 分组
	groups := m.grouper.GroupNodes(nodesWithContext)

	return groups, nil
}

// MarkRetryCompleted 标记重试完成
func (m *NodeRetryManager) MarkRetryCompleted(nodeID int, success bool, translatedText string, err error) {
	m.collection.Update(nodeID, func(node *NodeInfo) {
		node.RetryCount++
		if success {
			node.Status = NodeStatusSuccess
			node.TranslatedText = translatedText
			node.Error = nil
		} else {
			node.Status = NodeStatusFailed
			node.Error = err
		}
	})
}

// ResetProcessedNodes 重置已处理节点集合（用于新一轮重试）
func (m *NodeRetryManager) ResetProcessedNodes() {
	m.processedNodes = NewIntSet()
}
