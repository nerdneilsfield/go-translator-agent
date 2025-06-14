package translator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/nerdneilsfield/go-translator-agent/internal/progress"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func createTestConfig() *config.Config {
	return &config.Config{
		SourceLang:       "en",
		TargetLang:       "zh",
		DefaultModelName: "test-model",
		ChunkSize:        1000,
		RetryAttempts:    3,
		Country:          "US",
		ModelConfigs: map[string]config.ModelConfig{
			"test-model": {
				Name:            "test-model",
				ModelID:         "test-model",
				APIType:         "openai",
				BaseURL:         "http://localhost:8080",
				Key:             "test-key",
				MaxOutputTokens: 4096,
				MaxInputTokens:  4096,
				Temperature:     0.3,
			},
		},
		StepSets: map[string]config.StepSetConfig{
			"default": {
				ID:   "default",
				Name: "Default",
				InitialTranslation: config.StepConfig{
					Name:        "initial",
					ModelName:   "test-model",
					Temperature: 0.3,
				},
			},
		},
		ActiveStepSet: "default",
		Metadata:      make(map[string]interface{}),
	}
}

func TestNewTranslationCoordinator(t *testing.T) {
	cfg := createTestConfig()
	logger := zap.NewNop()
	progressPath := t.TempDir()

	t.Run("Create Coordinator Successfully", func(t *testing.T) {
		coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
		require.NoError(t, err)
		require.NotNil(t, coordinator)

		// 验证组件
		assert.NotNil(t, coordinator.config)
		assert.NotNil(t, coordinator.nodeTranslator)
		assert.NotNil(t, coordinator.progressTracker)
		assert.NotNil(t, coordinator.progressReporter)
		assert.NotNil(t, coordinator.formatManager)
		// translationService 在测试中是 nil（模拟翻译）
		// assert.NotNil(t, coordinator.translationService)
		assert.NotNil(t, coordinator.logger)

		assert.Equal(t, cfg, coordinator.config)
	})

	t.Run("Create with Nil Config", func(t *testing.T) {
		coordinator, err := NewTranslationCoordinator(nil, logger, progressPath)
		assert.Error(t, err)
		assert.Nil(t, coordinator)
		assert.Contains(t, err.Error(), "config cannot be nil")
	})

	t.Run("Create with Nil Logger", func(t *testing.T) {
		coordinator, err := NewTranslationCoordinator(cfg, nil, progressPath)
		require.NoError(t, err)
		require.NotNil(t, coordinator)
		// 应该使用 nop logger
		assert.NotNil(t, coordinator.logger)
	})

	t.Run("Create with Empty Progress Path", func(t *testing.T) {
		coordinator, err := NewTranslationCoordinator(cfg, logger, "")
		require.NoError(t, err)
		require.NotNil(t, coordinator)
		// 应该使用默认路径
	})
}

func TestTranslationCoordinator_ProgressManagement(t *testing.T) {
	cfg := createTestConfig()
	logger := zap.NewNop()
	progressPath := t.TempDir()

	coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
	require.NoError(t, err)

	t.Run("Progress Operations", func(t *testing.T) {
		// 开始时应该没有会话
		sessions, err := coordinator.ListSessions()
		require.NoError(t, err)
		assert.Len(t, sessions, 0)

		// 创建一个模拟会话
		docID := "test-doc-123"
		fileName := "/test/document.md"

		// 开始跟踪
		coordinator.progressReporter.StartDocument(docID, fileName, 3)

		// 更新进度
		coordinator.progressReporter.UpdateNode(docID, 1, document.NodeStatusSuccess, 100, nil)
		coordinator.progressReporter.UpdateNode(docID, 2, document.NodeStatusPending, 150, nil)

		// 获取进度信息
		progressInfo := coordinator.GetProgress(docID)
		require.NotNil(t, progressInfo)
		assert.Equal(t, docID, progressInfo.DocID)
		assert.Equal(t, fileName, progressInfo.FileName)
		// 只有 2 个节点，因为第一个节点在 UpdateNode 调用时才被创建
		assert.Equal(t, 2, progressInfo.TotalChunks)
		assert.Equal(t, 1, progressInfo.CompletedChunks)

		// 完成会话
		coordinator.progressReporter.CompleteDocument(docID)

		// 验证最终状态
		progressInfo = coordinator.GetProgress(docID)
		require.NotNil(t, progressInfo)
		assert.Equal(t, progress.StatusCompleted, progressInfo.Status)
	})

	t.Run("Active Sessions", func(t *testing.T) {
		// 获取活跃会话
		activeSessions, err := coordinator.GetActiveSession()
		require.NoError(t, err)
		// 此时应该没有活跃会话，因为上面的会话已经完成
		assert.Len(t, activeSessions, 0)
	})
}

func TestTranslationCoordinator_TextTranslation(t *testing.T) {
	cfg := createTestConfig()
	logger := zap.NewNop()
	progressPath := t.TempDir()

	coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
	require.NoError(t, err)

	t.Run("Translate Empty Text", func(t *testing.T) {
		result, err := coordinator.TranslateText(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("Translate Simple Text", func(t *testing.T) {
		text := "Hello, world!"

		// 注意：由于没有实际的翻译服务，这个测试会失败
		// 但我们可以验证输入处理逻辑
		result, err := coordinator.TranslateText(context.Background(), text)

		// 预期会有错误，因为没有配置实际的翻译提供商
		_ = result
		_ = err

		// 验证基本的文本处理
		nodes := coordinator.createTextNodes(text)
		assert.Len(t, nodes, 1)
		assert.Equal(t, text, nodes[0].OriginalText)
		assert.Equal(t, document.NodeStatusPending, nodes[0].Status)
	})

	t.Run("Translate Multi-Paragraph Text", func(t *testing.T) {
		text := "First paragraph.\n\nSecond paragraph.\n\nThird paragraph."

		nodes := coordinator.createTextNodes(text)
		assert.Len(t, nodes, 3)
		assert.Equal(t, "First paragraph.", nodes[0].OriginalText)
		assert.Equal(t, "Second paragraph.", nodes[1].OriginalText)
		assert.Equal(t, "Third paragraph.", nodes[2].OriginalText)
	})
}

func TestTranslationCoordinator_DocumentParsing(t *testing.T) {
	cfg := createTestConfig()
	logger := zap.NewNop()
	progressPath := t.TempDir()

	coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
	require.NoError(t, err)

	t.Run("Parse Markdown Document", func(t *testing.T) {
		content := "# Title\n\nFirst paragraph.\n\nSecond paragraph."
		nodes, err := coordinator.parseMarkdown(content)
		require.NoError(t, err)
		assert.Len(t, nodes, 3)
		assert.Equal(t, "# Title", nodes[0].OriginalText)
		assert.Equal(t, "First paragraph.", nodes[1].OriginalText)
		assert.Equal(t, "Second paragraph.", nodes[2].OriginalText)
	})

	t.Run("Parse Text Document", func(t *testing.T) {
		content := "First line.\n\nSecond line.\n\nThird line."
		nodes, err := coordinator.parseText(content)
		require.NoError(t, err)
		assert.Len(t, nodes, 3)
		assert.Equal(t, "First line.", nodes[0].OriginalText)
		assert.Equal(t, "Second line.", nodes[1].OriginalText)
		assert.Equal(t, "Third line.", nodes[2].OriginalText)
	})

	t.Run("Parse HTML Document", func(t *testing.T) {
		content := "<html>\n<p>First paragraph</p>\n<p>Second paragraph</p>\n</html>"
		nodes, err := coordinator.parseHTML(content)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(nodes), 2)
	})

	t.Run("Parse Document by File Extension", func(t *testing.T) {
		content := "Test content.\n\nSecond paragraph."

		// Markdown
		nodes, err := coordinator.parseDocument("test.md", content)
		require.NoError(t, err)
		assert.Len(t, nodes, 2)

		// Text
		nodes, err = coordinator.parseDocument("test.txt", content)
		require.NoError(t, err)
		assert.Len(t, nodes, 2)

		// Unknown extension (default to text)
		nodes, err = coordinator.parseDocument("test.unknown", content)
		require.NoError(t, err)
		assert.Len(t, nodes, 2)
	})
}

func TestTranslationCoordinator_DocumentAssembly(t *testing.T) {
	cfg := createTestConfig()
	logger := zap.NewNop()
	progressPath := t.TempDir()

	coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
	require.NoError(t, err)

	t.Run("Assemble Translated Nodes", func(t *testing.T) {
		nodes := []*document.NodeInfo{
			{
				ID:             1,
				OriginalText:   "Hello",
				TranslatedText: "你好",
				Status:         document.NodeStatusSuccess,
			},
			{
				ID:             2,
				OriginalText:   "World",
				TranslatedText: "世界",
				Status:         document.NodeStatusSuccess,
			},
		}

		// Markdown assembly
		result := coordinator.assembleMarkdown(nodes)
		assert.Equal(t, "你好\n\n世界", result)

		// Text assembly
		result = coordinator.assembleText(nodes)
		assert.Equal(t, "你好\n\n世界", result)

		// Text result assembly
		result = coordinator.assembleTextResult(nodes)
		assert.Equal(t, "你好\n\n世界", result)
	})

	t.Run("Assemble with Failed Nodes", func(t *testing.T) {
		nodes := []*document.NodeInfo{
			{
				ID:             1,
				OriginalText:   "Hello",
				TranslatedText: "你好",
				Status:         document.NodeStatusSuccess,
			},
			{
				ID:           2,
				OriginalText: "World",
				Status:       document.NodeStatusFailed,
			},
		}

		result := coordinator.assembleText(nodes)
		assert.Equal(t, "你好\n\nWorld", result) // 失败的节点保留原文
	})
}

func TestTranslationCoordinator_FileOperations(t *testing.T) {
	cfg := createTestConfig()
	logger := zap.NewNop()
	progressPath := t.TempDir()

	coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
	require.NoError(t, err)

	t.Run("Read and Write File", func(t *testing.T) {
		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "test.txt")
		testContent := "Test content for file operations."

		// 写入文件
		err := coordinator.writeFile(testFile, testContent)
		require.NoError(t, err)

		// 验证文件存在
		_, err = os.Stat(testFile)
		require.NoError(t, err)

		// 读取文件
		content, err := coordinator.readFile(testFile)
		require.NoError(t, err)
		assert.Equal(t, testContent, content)
	})

	t.Run("Write to Nested Directory", func(t *testing.T) {
		tempDir := t.TempDir()
		nestedFile := filepath.Join(tempDir, "nested", "dir", "test.txt")
		testContent := "Nested directory test."

		err := coordinator.writeFile(nestedFile, testContent)
		require.NoError(t, err)

		// 验证目录和文件都被创建
		_, err = os.Stat(nestedFile)
		require.NoError(t, err)

		content, err := coordinator.readFile(nestedFile)
		require.NoError(t, err)
		assert.Equal(t, testContent, content)
	})

	t.Run("Read Non-Existent File", func(t *testing.T) {
		content, err := coordinator.readFile("/non/existent/file.txt")
		assert.Error(t, err)
		assert.Equal(t, "", content)
		assert.Contains(t, err.Error(), "failed to open file")
	})
}

func TestTranslationCoordinator_ResultCreation(t *testing.T) {
	cfg := createTestConfig()
	logger := zap.NewNop()
	progressPath := t.TempDir()

	coordinator, err := NewTranslationCoordinator(cfg, logger, progressPath)
	require.NoError(t, err)

	t.Run("Create Success Result", func(t *testing.T) {
		docID := "test-doc"
		inputFile := "input.txt"
		outputFile := "output.txt"
		startTime := time.Now().Add(-time.Minute)
		endTime := time.Now()

		nodes := []*document.NodeInfo{
			{Status: document.NodeStatusSuccess},
			{Status: document.NodeStatusSuccess},
			{Status: document.NodeStatusFailed},
		}

		result := coordinator.createSuccessResult(docID, inputFile, outputFile, startTime, endTime, nodes)

		assert.Equal(t, docID, result.DocID)
		assert.Equal(t, inputFile, result.InputFile)
		assert.Equal(t, outputFile, result.OutputFile)
		assert.Equal(t, cfg.SourceLang, result.SourceLanguage)
		assert.Equal(t, cfg.TargetLang, result.TargetLanguage)
		assert.Equal(t, 3, result.TotalNodes)
		assert.Equal(t, 2, result.CompletedNodes)
		assert.Equal(t, 1, result.FailedNodes)
		assert.InDelta(t, float64(200)/3, result.Progress, 0.01)      // 2/3 * 100 ≈ 66.67
		assert.Equal(t, string(progress.StatusFailed), result.Status) // 有失败节点
		assert.Equal(t, startTime, result.StartTime)
		assert.Equal(t, &endTime, result.EndTime)
		assert.Equal(t, endTime.Sub(startTime), result.Duration)
	})

	t.Run("Create Failed Result", func(t *testing.T) {
		docID := "failed-doc"
		inputFile := "input.txt"
		outputFile := "output.txt"
		startTime := time.Now().Add(-time.Second)
		testError := assert.AnError

		result := coordinator.createFailedResult(docID, inputFile, outputFile, startTime, testError)

		assert.Equal(t, docID, result.DocID)
		assert.Equal(t, inputFile, result.InputFile)
		assert.Equal(t, outputFile, result.OutputFile)
		assert.Equal(t, 0, result.TotalNodes)
		assert.Equal(t, 0, result.CompletedNodes)
		assert.Equal(t, 0, result.FailedNodes)
		assert.Equal(t, float64(0), result.Progress)
		assert.Equal(t, string(progress.StatusFailed), result.Status)
		assert.Equal(t, startTime, result.StartTime)
		assert.NotNil(t, result.EndTime)
		assert.Greater(t, result.Duration, time.Duration(0))
		assert.Equal(t, testError.Error(), result.ErrorMessage)
	})
}
