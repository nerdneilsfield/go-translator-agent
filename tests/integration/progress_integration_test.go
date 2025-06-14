package integration

import (
	"context"
	"testing"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/nerdneilsfield/go-translator-agent/internal/progress"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestProgressTranslatorIntegration(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()
	logger := zap.NewNop()

	// 创建 progress tracker
	progressTracker := progress.NewTracker(logger, tmpDir)

	// 创建 progress reporter
	progressReporter := translator.NewProgressTrackerReporter(progressTracker, logger)

	// 创建带进度报告的节点翻译器
	nodeTranslator := document.NewNodeInfoTranslatorWithProgress(1000, 2, 3, progressReporter)

	t.Run("Document Translation with Progress", func(t *testing.T) {
		// 创建测试节点
		nodes := []*document.NodeInfo{
			{
				ID:           1,
				OriginalText: "Hello, world!",
				Status:       document.NodeStatusPending,
			},
			{
				ID:           2,
				OriginalText: "This is a test.",
				Status:       document.NodeStatusPending,
			},
			{
				ID:           3,
				OriginalText: "Testing progress tracking.",
				Status:       document.NodeStatusPending,
			},
		}

		docID := "test-doc-123"
		fileName := "/test/document.md"

		// 模拟翻译函数
		translateFunc := func(ctx context.Context, node *document.NodeInfo) error {
			// 模拟翻译延迟
			time.Sleep(10 * time.Millisecond)

			// 设置翻译结果
			node.TranslatedText = "Translated: " + node.OriginalText
			node.Status = document.NodeStatusSuccess

			return nil
		}

		// 执行翻译
		err := nodeTranslator.TranslateDocument(context.Background(), docID, fileName, nodes, translateFunc)
		require.NoError(t, err)

		// 验证进度信息
		progressInfo := progressTracker.GetProgress(docID)
		require.NotNil(t, progressInfo)

		assert.Equal(t, docID, progressInfo.DocID)
		assert.Equal(t, fileName, progressInfo.FileName)
		assert.Equal(t, 3, progressInfo.TotalChunks)
		assert.Equal(t, 3, progressInfo.CompletedChunks)
		assert.Equal(t, 0, progressInfo.FailedChunks)
		assert.Equal(t, float64(100), progressInfo.Progress)
		assert.Equal(t, progress.StatusCompleted, progressInfo.Status)

		// 验证节点翻译结果
		for _, node := range nodes {
			assert.Equal(t, document.NodeStatusSuccess, node.Status)
			assert.Contains(t, node.TranslatedText, "Translated:")
		}
	})

	t.Run("Translation with Failures and Retries", func(t *testing.T) {
		// 重置 progress tracker
		progressTracker = progress.NewTracker(logger, tmpDir)
		progressReporter = translator.NewProgressTrackerReporter(progressTracker, logger)
		nodeTranslator = document.NewNodeInfoTranslatorWithProgress(1000, 2, 3, progressReporter)

		nodes := []*document.NodeInfo{
			{
				ID:           1,
				OriginalText: "Success node",
				Status:       document.NodeStatusPending,
			},
			{
				ID:           2,
				OriginalText: "Fail node",
				Status:       document.NodeStatusPending,
			},
		}

		docID := "test-doc-456"
		fileName := "/test/fail-document.md"

		// 模拟翻译函数，节点2会失败
		translateFunc := func(ctx context.Context, node *document.NodeInfo) error {
			if node.ID == 2 {
				// 模拟翻译失败
				node.Status = document.NodeStatusFailed
				return assert.AnError
			}

			// 成功节点
			node.TranslatedText = "Translated: " + node.OriginalText
			node.Status = document.NodeStatusSuccess
			return nil
		}

		// 执行翻译（预期会有失败）
		err := nodeTranslator.TranslateDocument(context.Background(), docID, fileName, nodes, translateFunc)
		require.NoError(t, err) // 翻译流程应该继续，不会因为个别节点失败而中断

		// 验证进度信息
		progressInfo := progressTracker.GetProgress(docID)
		require.NotNil(t, progressInfo)

		assert.Equal(t, 1, progressInfo.CompletedChunks)            // 只有1个成功
		assert.Equal(t, 1, progressInfo.FailedChunks)               // 1个失败
		assert.Equal(t, float64(50), progressInfo.Progress)         // 50% 完成
		assert.Equal(t, progress.StatusFailed, progressInfo.Status) // 最终状态为失败
	})
}

func TestProgressReporterInterface(t *testing.T) {
	tmpDir := t.TempDir()
	logger := zap.NewNop()

	progressTracker := progress.NewTracker(logger, tmpDir)
	progressReporter := translator.NewProgressTrackerReporter(progressTracker, logger)

	t.Run("Progress Reporter Methods", func(t *testing.T) {
		docID := "test-doc-789"
		fileName := "/test/reporter-test.md"

		// 开始文档翻译
		progressReporter.StartDocument(docID, fileName, 2)

		// 验证会话已创建
		progressInfo := progressReporter.GetProgress(docID)
		require.NotNil(t, progressInfo)
		assert.Equal(t, docID, progressInfo.DocID)
		assert.Equal(t, fileName, progressInfo.FileName)

		// 更新节点进度
		progressReporter.UpdateNode(docID, 1, document.NodeStatusSuccess, 10, nil)
		progressReporter.UpdateNode(docID, 2, document.NodeStatusFailed, 15, assert.AnError)

		// 验证更新后的进度
		progressInfo = progressReporter.GetProgress(docID)
		require.NotNil(t, progressInfo)
		assert.Equal(t, 1, progressInfo.CompletedChunks)
		assert.Equal(t, 1, progressInfo.FailedChunks)

		// 完成文档翻译
		progressReporter.CompleteDocument(docID)

		// 验证最终状态
		progressInfo = progressReporter.GetProgress(docID)
		require.NotNil(t, progressInfo)
		assert.Equal(t, progress.StatusFailed, progressInfo.Status) // 因为有失败节点
	})
}
