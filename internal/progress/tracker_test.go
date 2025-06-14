package progress

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestProgressTracker(t *testing.T) {
	// 创建临时目录用于测试
	tmpDir := t.TempDir()
	logger := zap.NewNop()

	t.Run("Create and Update Progress", func(t *testing.T) {
		tracker := NewTracker(logger, tmpDir)

		docID := "test-session"
		fileName := "/test/doc.md"

		// 开始跟踪
		tracker.StartTracking(docID, fileName)

		// 获取进度
		progress := tracker.GetProgress(docID)
		require.NotNil(t, progress)
		assert.Equal(t, docID, progress.DocID)
		assert.Equal(t, StatusRunning, progress.Status)

		// 更新节点进度
		tracker.UpdateNodeProgress(docID, 1, document.NodeStatusSuccess, 10, nil)

		// 再次获取进度
		progress = tracker.GetProgress(docID)
		require.NotNil(t, progress)
		assert.Equal(t, 1, progress.CompletedChunks)
	})

	t.Run("Stop Tracking", func(t *testing.T) {
		tracker := NewTracker(logger, tmpDir)

		docID := "test-session-2"
		fileName := "/test/doc2.md"

		// 开始跟踪
		tracker.StartTracking(docID, fileName)

		// 停止跟踪
		tracker.StopTracking(docID)

		// 验证状态
		progress := tracker.GetProgress(docID)
		require.NotNil(t, progress)
		assert.Equal(t, StatusCompleted, progress.Status)
	})

	t.Run("List Sessions", func(t *testing.T) {
		tracker := NewTracker(logger, tmpDir)

		// 创建多个会话并停止跟踪以触发保存
		for i := 0; i < 3; i++ {
			docID := sessionID(i)
			fileName := filepath.Join("/test", docID+".md")
			tracker.StartTracking(docID, fileName)
			tracker.StopTracking(docID) // 触发保存
		}

		// 列出所有会话
		sessions, err := tracker.ListSessions()
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(sessions), 3)
	})

	t.Run("Load Session", func(t *testing.T) {
		tracker := NewTracker(logger, tmpDir)

		docID := "test-load"
		fileName := "/test/load.md"

		// 开始跟踪
		tracker.StartTracking(docID, fileName)

		// 更新一些进度
		tracker.UpdateNodeProgress(docID, 1, document.NodeStatusSuccess, 10, nil)
		tracker.UpdateNodeProgress(docID, 2, document.NodeStatusPending, 5, nil)

		// 停止跟踪（这会保存会话）
		tracker.StopTracking(docID)

		// 创建新的跟踪器并加载会话
		newTracker := NewTracker(logger, tmpDir)
		err := newTracker.LoadSession(docID)
		require.NoError(t, err)

		// 验证加载的数据
		progress := newTracker.GetProgress(docID)
		require.NotNil(t, progress)
		assert.Equal(t, docID, progress.DocID)
		assert.Equal(t, 1, progress.CompletedChunks)
	})
}

func TestFileBackend(t *testing.T) {
	tmpDir := t.TempDir()
	backend := NewFileBackend(tmpDir)

	t.Run("Save and Load Session", func(t *testing.T) {
		session := &Session{
			ID:             "test-backend",
			FileName:       "/test/backend.md",
			StartTime:      time.Now(),
			LastUpdateTime: time.Now(),
			Status:         StatusRunning,
			TotalNodes:     5,
			CompletedNodes: 2,
			NodeProgress:   make(map[int]*NodeProgress),
			Errors:         []ErrorInfo{},
		}

		// 添加一些节点进度
		session.NodeProgress[1] = &NodeProgress{
			NodeID:         1,
			Status:         document.NodeStatusSuccess,
			StartTime:      time.Now(),
			CompleteTime:   time.Now(),
			CharacterCount: 10,
		}

		session.NodeProgress[2] = &NodeProgress{
			NodeID:         2,
			Status:         document.NodeStatusPending,
			StartTime:      time.Now(),
			CharacterCount: 5,
		}

		// 保存
		err := backend.Save(session)
		require.NoError(t, err)

		// 加载
		loaded, err := backend.Load("test-backend")
		require.NoError(t, err)
		assert.Equal(t, session.ID, loaded.ID)
		assert.Equal(t, session.FileName, loaded.FileName)
		assert.Equal(t, session.Status, loaded.Status)
		assert.Equal(t, session.CompletedNodes, loaded.CompletedNodes)

		// 验证节点进度
		assert.Len(t, loaded.NodeProgress, 2)
		assert.Equal(t, document.NodeStatusSuccess, loaded.NodeProgress[1].Status)
		assert.Equal(t, document.NodeStatusPending, loaded.NodeProgress[2].Status)
	})

	t.Run("List Sessions", func(t *testing.T) {
		// 创建多个会话文件
		for i := 0; i < 3; i++ {
			session := &Session{
				ID:             sessionID(i),
				FileName:       filepath.Join("/test", sessionID(i)+".md"),
				StartTime:      time.Now(),
				LastUpdateTime: time.Now(),
				Status:         StatusRunning,
				NodeProgress:   make(map[int]*NodeProgress),
				Errors:         []ErrorInfo{},
			}
			err := backend.Save(session)
			require.NoError(t, err)
		}

		// 列出所有会话
		sessions, err := backend.List()
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(sessions), 3)

		// 验证会话信息
		for _, s := range sessions {
			assert.NotEmpty(t, s.ID)
			assert.NotZero(t, s.StartTime)
		}
	})

	t.Run("Delete Session", func(t *testing.T) {
		sessionID := "test-delete-backend"

		// 创建会话
		session := &Session{
			ID:             sessionID,
			FileName:       "/test/delete.md",
			StartTime:      time.Now(),
			LastUpdateTime: time.Now(),
			Status:         StatusRunning,
			NodeProgress:   make(map[int]*NodeProgress),
			Errors:         []ErrorInfo{},
		}
		err := backend.Save(session)
		require.NoError(t, err)

		// 删除
		err = backend.Delete(sessionID)
		require.NoError(t, err)

		// 验证已删除
		_, err = backend.Load(sessionID)
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))
	})
}

// 示例：如何在实际翻译流程中使用进度跟踪
func Example() {
	// 1. 创建进度跟踪器
	logger := zap.NewNop()
	tmpDir := "/tmp/translator-progress"
	tracker := NewTracker(logger, tmpDir)

	// 2. 开始跟踪翻译会话
	docID := "document-123"
	fileName := "/path/to/document.md"
	tracker.StartTracking(docID, fileName)

	// 3. 模拟翻译过程中的进度更新
	// 节点1开始处理
	tracker.UpdateNodeProgress(docID, 1, document.NodeStatusPending, 0, nil)

	// 节点1完成
	tracker.UpdateNodeProgress(docID, 1, document.NodeStatusSuccess, 100, nil)

	// 节点2失败
	tracker.UpdateNodeProgress(docID, 2, document.NodeStatusFailed, 50,
		fmt.Errorf("translation failed"))

	// 4. 获取当前进度
	progress := tracker.GetProgress(docID)
	if progress != nil {
		fmt.Printf("Progress: %.1f%% (%d/%d nodes completed)\n",
			progress.Progress, progress.CompletedChunks, progress.TotalChunks)
	}

	// 5. 完成跟踪
	tracker.StopTracking(docID)
}

// 辅助函数
func sessionID(i int) string {
	return "test-session-" + string(rune('a'+i))
}
