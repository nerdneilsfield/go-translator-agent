package progress

import (
	"sync"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
)

// NodeObserver 观察 NodeCollection 的变化并更新进度
type NodeObserver struct {
	tracker    *Tracker
	docID      string
	collection *document.NodeCollection

	// 缓存节点状态，避免重复更新
	nodeStates map[int]document.NodeStatus
	mu         sync.RWMutex
}

// NewNodeObserver 创建节点观察器
func NewNodeObserver(tracker *Tracker, docID string, collection *document.NodeCollection) *NodeObserver {
	return &NodeObserver{
		tracker:    tracker,
		docID:      docID,
		collection: collection,
		nodeStates: make(map[int]document.NodeStatus),
	}
}

// ObserveCollection 开始观察集合变化
func (o *NodeObserver) ObserveCollection() {
	// 初始化：记录所有节点
	nodes := o.collection.GetAll()
	for _, node := range nodes {
		o.mu.Lock()
		o.nodeStates[node.ID] = node.Status
		o.mu.Unlock()

		// 更新进度跟踪器
		o.tracker.UpdateNodeProgress(o.docID, node.ID, node.Status, len(node.OriginalText), node.Error)
	}
}

// CheckUpdates 检查并报告节点状态更新
func (o *NodeObserver) CheckUpdates() {
	nodes := o.collection.GetAll()

	for _, node := range nodes {
		o.mu.RLock()
		prevStatus, exists := o.nodeStates[node.ID]
		o.mu.RUnlock()

		// 新节点或状态变化
		if !exists || prevStatus != node.Status {
			o.mu.Lock()
			o.nodeStates[node.ID] = node.Status
			o.mu.Unlock()

			// 更新进度跟踪器
			charCount := len(node.OriginalText)
			if node.TranslatedText != "" {
				charCount = len(node.TranslatedText)
			}

			o.tracker.UpdateNodeProgress(o.docID, node.ID, node.Status, charCount, node.Error)
		}
	}
}

// StartContinuousObservation 启动持续观察
func (o *NodeObserver) StartContinuousObservation(stopCh <-chan struct{}) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			o.CheckUpdates()
		case <-stopCh:
			// 最后一次检查
			o.CheckUpdates()
			return
		}
	}
}

// ObservableNodeCollection 可观察的节点集合
type ObservableNodeCollection struct {
	*document.NodeCollection
	observers []func(nodeID int, status document.NodeStatus)
	mu        sync.RWMutex
}

// NewObservableNodeCollection 创建可观察的节点集合
func NewObservableNodeCollection() *ObservableNodeCollection {
	return &ObservableNodeCollection{
		NodeCollection: document.NewNodeCollection(),
		observers:      []func(nodeID int, status document.NodeStatus){},
	}
}

// AddObserver 添加观察者
func (c *ObservableNodeCollection) AddObserver(observer func(nodeID int, status document.NodeStatus)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.observers = append(c.observers, observer)
}

// Update 更新节点（覆盖原方法）
func (c *ObservableNodeCollection) Update(id int, updater func(*document.NodeInfo)) error {
	// 获取更新前的状态
	node, exists := c.Get(id)
	var prevStatus document.NodeStatus
	if exists {
		prevStatus = node.Status
	}

	// 执行更新
	err := c.NodeCollection.Update(id, updater)
	if err != nil {
		return err
	}

	// 获取更新后的状态
	node, _ = c.Get(id)
	if node.Status != prevStatus {
		// 通知观察者
		c.mu.RLock()
		observers := c.observers
		c.mu.RUnlock()

		for _, observer := range observers {
			observer(id, node.Status)
		}
	}

	return nil
}
