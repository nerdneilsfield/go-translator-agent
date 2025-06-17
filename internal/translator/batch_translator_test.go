package translator

import (
	"context"
	"fmt"
	"testing"

	"github.com/dlclark/regexp2"
	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNodeMarkerRegex(t *testing.T) {
	// 测试正则表达式是否正确匹配节点标记
	pattern := regexp2.MustCompile(`(?s)@@NODE_START_(\d+)@@\s*\r?\n(.*?)\r?\n\s*@@NODE_END_\1@@`, 0)

	testCases := []struct {
		name     string
		input    string
		expected map[string]string // nodeID -> content
	}{
		{
			name: "single node",
			input: `@@NODE_START_1@@
Hello world
@@NODE_END_1@@`,
			expected: map[string]string{
				"1": "Hello world",
			},
		},
		{
			name: "multiple nodes",
			input: `@@NODE_START_1@@
First node
@@NODE_END_1@@

@@NODE_START_2@@
Second node
@@NODE_END_2@@`,
			expected: map[string]string{
				"1": "First node",
				"2": "Second node",
			},
		},
		{
			name: "node with multiline content",
			input: `@@NODE_START_3@@
Line 1
Line 2
Line 3
@@NODE_END_3@@`,
			expected: map[string]string{
				"3": "Line 1\nLine 2\nLine 3",
			},
		},
		{
			name: "mismatched node IDs should not match",
			input: `@@NODE_START_1@@
Content
@@NODE_END_2@@`,
			expected: map[string]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := make(map[string]string)

			match, _ := pattern.FindStringMatch(tc.input)
			for match != nil {
				groups := match.Groups()
				if len(groups) >= 3 {
					nodeID := groups[1].String()
					content := groups[2].String()
					results[nodeID] = content
				}
				match, _ = pattern.FindNextMatch(match)
			}

			// 验证结果
			if len(results) != len(tc.expected) {
				t.Errorf("Expected %d matches, got %d", len(tc.expected), len(results))
			}

			for nodeID, expectedContent := range tc.expected {
				if actualContent, ok := results[nodeID]; !ok {
					t.Errorf("Expected node %s not found", nodeID)
				} else if actualContent != expectedContent {
					t.Errorf("Node %s: expected %q, got %q", nodeID, expectedContent, actualContent)
				}
			}
		})
	}
}

// mockTranslationService 模拟翻译服务
type mockTranslationService struct {
	config *translation.Config
}

func (m *mockTranslationService) Translate(ctx context.Context, req *translation.Request) (*translation.Response, error) {
	// 模拟翻译：如果是英文，返回中文；否则返回原文
	if req.SourceLanguage == "en" && req.TargetLanguage == "zh" {
		// 简单的模拟翻译逻辑
		if req.Text == "Hello world" {
			return &translation.Response{
				Text: "你好世界",
			}, nil
		}
		// 对于批量翻译，模拟返回格式化的结果
		if req.Metadata["is_batch"] == true {
			// 模拟批量翻译结果，部分成功部分失败
			result := ""
			for i := 0; i < 5; i++ {
				if i%2 == 0 {
					// 成功翻译
					result += fmt.Sprintf("@@NODE_START_%d@@\n这是节点%d的翻译结果\n@@NODE_END_%d@@\n\n", i, i, i)
				} else {
					// 失败（返回原文）
					result += fmt.Sprintf("@@NODE_START_%d@@\nThis is node %d original text\n@@NODE_END_%d@@\n\n", i, i, i)
				}
			}
			return &translation.Response{
				Text: result,
			}, nil
		}
	}
	return &translation.Response{
		Text: req.Text,
	}, nil
}

func (m *mockTranslationService) TranslateBatch(ctx context.Context, reqs []*translation.Request) ([]*translation.Response, error) {
	resps := make([]*translation.Response, len(reqs))
	for i, req := range reqs {
		resp, err := m.Translate(ctx, req)
		if err != nil {
			return nil, err
		}
		resps[i] = resp
	}
	return resps, nil
}

// TranslateText 实现translation.Service接口
func (m *mockTranslationService) TranslateText(ctx context.Context, text string) (string, error) {
	// 简单的模拟翻译
	if text == "Hello world" {
		return "你好世界", nil
	}
	if text == "This is node 0 original text" {
		return "这是节点0的翻译结果", nil
	}
	// 默认返回带"翻译："前缀的文本
	return "翻译：" + text, nil
}

func (m *mockTranslationService) GetConfig() *translation.Config {
	if m.config == nil {
		m.config = &translation.Config{
			SourceLanguage: "en",
			TargetLanguage: "zh",
		}
	}
	return m.config
}

func TestBatchTranslatorWithSimilarityCheck(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := TranslatorConfig{
		ChunkSize:      1000,
		Concurrency:    2,
		MaxRetries:     1,
		GroupingMode:   "smart",
		RetryOnFailure: true,
	}

	service := &mockTranslationService{
		config: &translation.Config{
			SourceLanguage: "en",
			TargetLanguage: "zh",
		},
	}
	bt := NewBatchTranslator(cfg, service, logger)

	// 创建测试节点
	nodes := []*document.NodeInfo{
		{
			ID:           0,
			OriginalText: "This is node 0 original text",
			Status:       document.NodeStatusPending,
		},
		{
			ID:           1,
			OriginalText: "This is node 1 original text",
			Status:       document.NodeStatusPending,
		},
		{
			ID:           2,
			OriginalText: "This is node 2 original text",
			Status:       document.NodeStatusPending,
		},
		{
			ID:           3,
			OriginalText: "This is node 3 original text",
			Status:       document.NodeStatusPending,
		},
		{
			ID:           4,
			OriginalText: "This is node 4 original text",
			Status:       document.NodeStatusPending,
		},
	}

	ctx := context.Background()
	err := bt.TranslateNodes(ctx, nodes)
	assert.NoError(t, err)

	// 验证相似度检查的效果
	successCount := 0
	failedCount := 0

	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess {
			successCount++
			// 成功的节点应该有不同的翻译文本
			assert.NotEqual(t, node.OriginalText, node.TranslatedText)
			assert.Contains(t, node.TranslatedText, "这是节点")
			assert.Contains(t, node.TranslatedText, "的翻译结果")
		} else {
			failedCount++
			// 失败的节点应该是因为相似度太高
			assert.NotNil(t, node.Error)
			assert.Contains(t, node.Error.Error(), "translation too similar")
		}
	}

	// 根据模拟逻辑，偶数节点应该成功，奇数节点应该失败
	assert.Equal(t, 3, successCount) // 节点 0, 2, 4
	assert.Equal(t, 2, failedCount)  // 节点 1, 3

	t.Logf("Translation results: %d success, %d failed", successCount, failedCount)
}
