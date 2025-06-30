package document

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeInfo(t *testing.T) {
	t.Run("Basic Node Operations", func(t *testing.T) {
		node := &NodeInfo{
			ID:           1,
			OriginalText: "Hello world",
			Status:       NodeStatusPending,
		}

		assert.Equal(t, 1, node.ID)
		assert.Equal(t, "Hello world", node.OriginalText)
		assert.False(t, node.IsTranslated())

		// 模拟翻译成功
		node.Status = NodeStatusSuccess
		node.TranslatedText = "你好世界"
		assert.True(t, node.IsTranslated())

		// 模拟翻译失败
		node.Status = NodeStatusFailed
		node.Error = fmt.Errorf("translation failed")
		assert.True(t, node.NeedsRetry())

		// 超过重试次数
		node.RetryCount = 3
		assert.False(t, node.NeedsRetry())
	})
}

func TestNodeCollection(t *testing.T) {
	t.Run("Add and Get Nodes", func(t *testing.T) {
		collection := NewNodeCollection()

		// 添加节点
		nodes := []*NodeInfo{
			{ID: 1, OriginalText: "First", Status: NodeStatusPending},
			{ID: 2, OriginalText: "Second", Status: NodeStatusSuccess, TranslatedText: "第二"},
			{ID: 3, OriginalText: "Third", Status: NodeStatusFailed},
		}

		for _, node := range nodes {
			collection.Add(node)
		}

		assert.Equal(t, 3, collection.Size())

		// 获取节点
		node, exists := collection.Get(2)
		assert.True(t, exists)
		assert.Equal(t, "Second", node.OriginalText)
		assert.Equal(t, "第二", node.TranslatedText)

		// 获取不存在的节点
		_, exists = collection.Get(999)
		assert.False(t, exists)

		// 获取所有节点
		allNodes := collection.GetAll()
		assert.Len(t, allNodes, 3)

		// 获取失败的节点
		failedNodes := collection.GetFailedNodes()
		assert.Len(t, failedNodes, 1)
		assert.Equal(t, 3, failedNodes[0].ID)
	})

	t.Run("Update Node", func(t *testing.T) {
		collection := NewNodeCollection()
		node := &NodeInfo{ID: 1, OriginalText: "Test", Status: NodeStatusPending}
		collection.Add(node)

		// 更新节点
		err := collection.Update(1, func(n *NodeInfo) {
			n.Status = NodeStatusSuccess
			n.TranslatedText = "测试"
		})
		require.NoError(t, err)

		// 验证更新
		updated, _ := collection.Get(1)
		assert.Equal(t, NodeStatusSuccess, updated.Status)
		assert.Equal(t, "测试", updated.TranslatedText)

		// 更新不存在的节点
		err = collection.Update(999, func(n *NodeInfo) {})
		assert.Error(t, err)
	})
}

func TestNodeGrouper(t *testing.T) {
	t.Run("Group Nodes by Size", func(t *testing.T) {
		grouper := NewNodeGrouper(50) // 最大 50 字符

		nodes := []*NodeInfo{
			{ID: 1, OriginalText: "Short text"},                                              // 10 chars
			{ID: 2, OriginalText: "Another short text"},                                      // 18 chars
			{ID: 3, OriginalText: "This is a bit longer text that might need its own group"}, // 55 chars
			{ID: 4, OriginalText: "Final text"},                                              // 10 chars
		}

		groups := grouper.GroupNodes(nodes)

		// 应该有3组：
		// 组1: 节点1和2 (28 chars)
		// 组2: 节点3 (55 chars，超过限制单独成组)
		// 组3: 节点4 (10 chars)
		assert.Len(t, groups, 3)

		assert.Len(t, groups[0].Nodes, 2)
		assert.Equal(t, 28, groups[0].Size)

		assert.Len(t, groups[1].Nodes, 1)
		assert.Equal(t, 55, groups[1].Size)

		assert.Len(t, groups[2].Nodes, 1)
		assert.Equal(t, 10, groups[2].Size)
	})
}

func TestNodeContextBuilder(t *testing.T) {
	t.Run("Build Context for Failed Nodes", func(t *testing.T) {
		collection := NewNodeCollection()

		// 添加一系列节点
		for i := 1; i <= 10; i++ {
			status := NodeStatusSuccess
			if i == 5 || i == 8 { // 节点5和8失败
				status = NodeStatusFailed
			}
			collection.Add(&NodeInfo{
				ID:           i,
				OriginalText: fmt.Sprintf("Text %d", i),
				Status:       status,
			})
		}

		builder := NewNodeContextBuilder(collection, 2) // 前后各取2个节点
		failedNodes := collection.GetFailedNodes()

		nodesWithContext, err := builder.BuildContextForFailedNodes(failedNodes)
		require.NoError(t, err)

		// 对于节点5，应该包含节点3,4,5,6,7
		// 对于节点8，应该包含节点6,7,8,9,10
		// 去重后应该是节点3,4,5,6,7,8,9,10
		assert.Len(t, nodesWithContext, 8)

		// 验证顺序
		expectedIDs := []int{3, 4, 5, 6, 7, 8, 9, 10}
		for i, node := range nodesWithContext {
			assert.Equal(t, expectedIDs[i], node.ID)
		}
	})
}

func TestNodeRetryManager(t *testing.T) {
	t.Run("Prepare Retry Groups", func(t *testing.T) {
		collection := NewNodeCollection()

		// 添加节点，其中一些失败
		for i := 1; i <= 10; i++ {
			status := NodeStatusSuccess
			if i == 3 || i == 7 {
				status = NodeStatusFailed
			}
			collection.Add(&NodeInfo{
				ID:           i,
				OriginalText: fmt.Sprintf("Text %d", i),
				Status:       status,
			})
		}

		manager := NewNodeRetryManager(collection, 100, 1, 3)

		// 第一次准备重试组
		groups, err := manager.PrepareRetryGroups()
		require.NoError(t, err)
		assert.NotEmpty(t, groups)

		// 标记节点3重试成功
		manager.MarkRetryCompleted(3, true, "Text 3 translated", nil)

		// 标记节点7重试失败
		manager.MarkRetryCompleted(7, false, "", fmt.Errorf("still failed"))

		// 再次准备重试组，节点3不应该再出现（已成功）
		// 节点7已被标记为已处理，也不应该再出现
		groups, err = manager.PrepareRetryGroups()
		require.NoError(t, err)
		assert.Empty(t, groups)

		// 重置已处理节点
		manager.ResetProcessedNodes()

		// 现在节点7应该可以再次重试（如果还有重试次数）
		node7, _ := collection.Get(7)
		if node7.RetryCount < 3 {
			groups, err = manager.PrepareRetryGroups()
			require.NoError(t, err)
			assert.NotEmpty(t, groups)
		}
	})
}

// 示例：如何在实际翻译流程中使用 NodeInfo 系统
func Example() {
	// 1. 创建节点集合
	collection := NewNodeCollection()

	// 2. 解析文档，创建节点
	// 这里模拟从 HTML/Markdown/Text 解析出的内容
	texts := []string{
		"Welcome to our website",
		"This is a paragraph",
		"Contact us at info@example.com",
		"Copyright 2024",
	}

	for i, text := range texts {
		collection.Add(&NodeInfo{
			ID:           i + 1,
			OriginalText: text,
			Status:       NodeStatusPending,
			Path:         fmt.Sprintf("/body/p[%d]", i+1),
		})
	}

	// 3. 分组翻译
	grouper := NewNodeGrouper(1000) // 假设每组最多1000字符
	groups := grouper.GroupNodes(collection.GetAll())

	for _, group := range groups {
		// 构建批量翻译的文本
		var batchText string
		for _, node := range group.Nodes {
			batchText += fmt.Sprintf("@@NODE_START_%d@@\n%s\n@@NODE_END_%d@@\n\n",
				node.ID, node.OriginalText, node.ID)
		}

		// 调用翻译服务（这里模拟）
		_ = simulateTranslation(batchText)

		// 解析翻译结果，更新节点
		// 实际实现中需要解析 @@NODE_START_n@@ 标记
		for _, node := range group.Nodes {
			collection.Update(node.ID, func(n *NodeInfo) {
				n.Status = NodeStatusSuccess
				n.TranslatedText = "Translated: " + n.OriginalText
			})
		}
	}

	// 4. 处理失败的节点
	retryManager := NewNodeRetryManager(collection, 1000, 1, 3)

	for retry := 0; retry < 3; retry++ {
		retryGroups, _ := retryManager.PrepareRetryGroups()
		if len(retryGroups) == 0 {
			break
		}

		for range retryGroups {
			// 带上下文重试翻译
			// ...
		}
	}

	// 5. 输出最终结果
	for _, node := range collection.GetAll() {
		if node.IsTranslated() {
			fmt.Printf("Node %d: %s -> %s\n", node.ID, node.OriginalText, node.TranslatedText)
		}
	}
}

func simulateTranslation(text string) string {
	// 模拟翻译服务
	return text
}
