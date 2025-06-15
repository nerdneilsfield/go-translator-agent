package progress

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"go.uber.org/zap"
)

// Tracker 进度跟踪器实现
type Tracker struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	logger   *zap.Logger
	backend  Backend

	// 配置
	updateInterval time.Duration
	autoSave       bool
	savePath       string
}

// Session 翻译会话
type Session struct {
	ID             string
	FileName       string
	StartTime      time.Time
	LastUpdateTime time.Time
	Status         SessionStatus

	// 统计信息
	TotalNodes      int
	CompletedNodes  int
	FailedNodes     int
	TotalCharacters int
	ProcessedChars  int

	// 节点详情
	NodeProgress map[int]*NodeProgress

	// 错误信息
	Errors []ErrorInfo

	mu sync.RWMutex
}

// SessionStatus 会话状态
type SessionStatus string

const (
	StatusRunning   SessionStatus = "running"
	StatusPaused    SessionStatus = "paused"
	StatusCompleted SessionStatus = "completed"
	StatusFailed    SessionStatus = "failed"
)

// NodeProgress 节点进度
type NodeProgress struct {
	NodeID         int
	Status         document.NodeStatus
	StartTime      time.Time
	CompleteTime   time.Time
	CharacterCount int
	Error          error
	RetryCount     int
}

// ErrorInfo 错误信息
type ErrorInfo struct {
	Time    time.Time
	NodeID  int
	Error   string
	Context map[string]interface{}
}

// Backend 存储后端接口
type Backend interface {
	Save(session *Session) error
	Load(sessionID string) (*Session, error)
	List() ([]*SessionSummary, error)
	Delete(sessionID string) error
}

// SessionSummary 会话摘要
type SessionSummary struct {
	ID        string
	FileName  string
	StartTime time.Time
	Status    SessionStatus
	Progress  float64
}

// NewTracker 创建进度跟踪器
func NewTracker(logger *zap.Logger, savePath string) *Tracker {
	return &Tracker{
		sessions:       make(map[string]*Session),
		logger:         logger,
		updateInterval: 1 * time.Second,
		autoSave:       true,
		savePath:       savePath,
		backend:        NewFileBackend(savePath),
	}
}

// StartTracking 开始跟踪
func (t *Tracker) StartTracking(docID, fileName string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	session := &Session{
		ID:             docID,
		FileName:       fileName,
		StartTime:      time.Now(),
		LastUpdateTime: time.Now(),
		Status:         StatusRunning,
		NodeProgress:   make(map[int]*NodeProgress),
		Errors:         []ErrorInfo{},
	}

	t.sessions[docID] = session

	// 自动保存
	if t.autoSave {
		go t.autoSaveSession(docID)
	}

	t.logger.Info("started tracking",
		zap.String("docID", docID),
		zap.String("fileName", fileName))
}

// StopTracking 停止跟踪
func (t *Tracker) StopTracking(docID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if session, exists := t.sessions[docID]; exists {
		session.mu.Lock()
		if session.FailedNodes > 0 {
			session.Status = StatusFailed
		} else {
			session.Status = StatusCompleted
		}
		session.mu.Unlock()

		// 最终保存
		if t.backend != nil {
			t.backend.Save(session)
		}

		t.logger.Info("stopped tracking",
			zap.String("docID", docID),
			zap.String("status", string(session.Status)))
	}
}

// OnChunkStart 块开始处理
func (t *Tracker) OnChunkStart(size int) {
	// 由于是批量处理，这里主要用于记录
	t.logger.Debug("chunk started", zap.Int("size", size))
}

// OnChunkComplete 块完成处理
func (t *Tracker) OnChunkComplete(size int) {
	t.logger.Debug("chunk completed", zap.Int("size", size))
}

// OnChunkError 块处理错误
func (t *Tracker) OnChunkError(err error) {
	t.logger.Debug("chunk error", zap.Error(err))
}

// StartDocument 开始文档翻译
func (t *Tracker) StartDocument(docID, fileName string, totalNodes int) {
	t.StartTracking(docID, fileName)
	
	t.mu.RLock()
	session, exists := t.sessions[docID]
	t.mu.RUnlock()
	
	if exists {
		session.mu.Lock()
		session.TotalNodes = totalNodes
		session.mu.Unlock()
	}
}

// CompleteDocument 完成文档翻译
func (t *Tracker) CompleteDocument(docID string) {
	t.StopTracking(docID)
}

// UpdateStep 更新步骤信息
func (t *Tracker) UpdateStep(docID string, nodeID int, step int, stepName string) {
	t.mu.RLock()
	session, exists := t.sessions[docID]
	t.mu.RUnlock()

	if !exists {
		return
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// 获取或创建节点进度
	nodeProgress, exists := session.NodeProgress[nodeID]
	if !exists {
		nodeProgress = &NodeProgress{
			NodeID:    nodeID,
			StartTime: time.Now(),
		}
		session.NodeProgress[nodeID] = nodeProgress
	}

	// 记录步骤信息
	t.logger.Debug("step update",
		zap.String("docID", docID),
		zap.Int("nodeID", nodeID),
		zap.Int("step", step),
		zap.String("stepName", stepName))
}

// UpdateNode 更新节点状态（实现 ProgressReporter 接口）
func (t *Tracker) UpdateNode(docID string, nodeID int, status document.NodeStatus, charCount int, err error) {
	t.UpdateNodeProgress(docID, nodeID, status, charCount, err)
}

// UpdateNodeProgress 更新节点进度
func (t *Tracker) UpdateNodeProgress(docID string, nodeID int, status document.NodeStatus, charCount int, err error) {
	t.mu.RLock()
	session, exists := t.sessions[docID]
	t.mu.RUnlock()

	if !exists {
		return
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// 获取或创建节点进度
	nodeProgress, exists := session.NodeProgress[nodeID]
	if !exists {
		nodeProgress = &NodeProgress{
			NodeID:    nodeID,
			StartTime: time.Now(),
		}
		session.NodeProgress[nodeID] = nodeProgress
		session.TotalNodes++
	}

	// 更新状态
	prevStatus := nodeProgress.Status
	nodeProgress.Status = status
	nodeProgress.CharacterCount = charCount

	// 更新统计
	switch status {
	case document.NodeStatusSuccess:
		if prevStatus != document.NodeStatusSuccess {
			session.CompletedNodes++
			session.ProcessedChars += charCount
		}
		nodeProgress.CompleteTime = time.Now()

	case document.NodeStatusFailed:
		if prevStatus != document.NodeStatusFailed {
			session.FailedNodes++
		}
		nodeProgress.Error = err
		nodeProgress.RetryCount++

		// 记录错误
		session.Errors = append(session.Errors, ErrorInfo{
			Time:   time.Now(),
			NodeID: nodeID,
			Error:  err.Error(),
		})
	}

	session.LastUpdateTime = time.Now()
}

// GetProgress 获取进度信息
func (t *Tracker) GetProgress(docID string) *ProgressInfo {
	t.mu.RLock()
	session, exists := t.sessions[docID]
	t.mu.RUnlock()

	if !exists {
		return nil
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	// 计算进度百分比
	progress := float64(0)
	if session.TotalNodes > 0 {
		progress = float64(session.CompletedNodes) / float64(session.TotalNodes) * 100
	}

	// 估算剩余时间
	var estimatedCompletion time.Time
	if session.CompletedNodes > 0 && session.CompletedNodes < session.TotalNodes {
		elapsed := time.Since(session.StartTime)
		avgTimePerNode := elapsed / time.Duration(session.CompletedNodes)
		remainingNodes := session.TotalNodes - session.CompletedNodes
		remainingTime := avgTimePerNode * time.Duration(remainingNodes)
		estimatedCompletion = time.Now().Add(remainingTime)
	}

	return &ProgressInfo{
		DocID:               docID,
		FileName:            session.FileName,
		TotalChunks:         session.TotalNodes,
		CompletedChunks:     session.CompletedNodes,
		FailedChunks:        session.FailedNodes,
		TotalCharacters:     session.TotalCharacters,
		ProcessedCharacters: session.ProcessedChars,
		StartTime:           session.StartTime,
		EstimatedCompletion: estimatedCompletion,
		Progress:            progress,
		Status:              session.Status,
		Errors:              len(session.Errors),
	}
}

// autoSaveSession 自动保存会话
func (t *Tracker) autoSaveSession(docID string) {
	ticker := time.NewTicker(t.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.mu.RLock()
			session, exists := t.sessions[docID]
			t.mu.RUnlock()

			if !exists || session.Status == StatusCompleted || session.Status == StatusFailed {
				return
			}

			if t.backend != nil {
				if err := t.backend.Save(session); err != nil {
					t.logger.Warn("failed to save session",
						zap.String("docID", docID),
						zap.Error(err))
				}
			}
		}
	}
}

// LoadSession 加载会话
func (t *Tracker) LoadSession(sessionID string) error {
	if t.backend == nil {
		return fmt.Errorf("no backend configured")
	}

	session, err := t.backend.Load(sessionID)
	if err != nil {
		return err
	}

	t.mu.Lock()
	t.sessions[sessionID] = session
	t.mu.Unlock()

	return nil
}

// ListSessions 列出所有会话
func (t *Tracker) ListSessions() ([]*SessionSummary, error) {
	if t.backend == nil {
		return nil, fmt.Errorf("no backend configured")
	}

	return t.backend.List()
}

// ProgressInfo 进度信息（扩展版本）
type ProgressInfo struct {
	DocID               string
	FileName            string
	TotalChunks         int
	CompletedChunks     int
	FailedChunks        int
	TotalCharacters     int
	ProcessedCharacters int
	StartTime           time.Time
	EstimatedCompletion time.Time
	Progress            float64
	Status              SessionStatus
	Errors              int
}

// FileBackend 文件存储后端
type FileBackend struct {
	basePath string
}

// NewFileBackend 创建文件后端
func NewFileBackend(basePath string) *FileBackend {
	os.MkdirAll(basePath, 0o755)
	return &FileBackend{basePath: basePath}
}

// Save 保存会话
func (fb *FileBackend) Save(session *Session) error {
	filePath := filepath.Join(fb.basePath, session.ID+".json")

	session.mu.RLock()
	data, err := json.MarshalIndent(session, "", "  ")
	session.mu.RUnlock()

	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0o644)
}

// Load 加载会话
func (fb *FileBackend) Load(sessionID string) (*Session, error) {
	filePath := filepath.Join(fb.basePath, sessionID+".json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	// 初始化mutex和map
	if session.NodeProgress == nil {
		session.NodeProgress = make(map[int]*NodeProgress)
	}

	return &session, nil
}

// List 列出所有会话
func (fb *FileBackend) List() ([]*SessionSummary, error) {
	files, err := filepath.Glob(filepath.Join(fb.basePath, "*.json"))
	if err != nil {
		return nil, err
	}

	var summaries []*SessionSummary
	for _, file := range files {
		sessionID := strings.TrimSuffix(filepath.Base(file), ".json")
		session, err := fb.Load(sessionID)
		if err != nil {
			continue
		}

		progress := float64(0)
		if session.TotalNodes > 0 {
			progress = float64(session.CompletedNodes) / float64(session.TotalNodes) * 100
		}

		summaries = append(summaries, &SessionSummary{
			ID:        session.ID,
			FileName:  session.FileName,
			StartTime: session.StartTime,
			Status:    session.Status,
			Progress:  progress,
		})
	}

	return summaries, nil
}

// Delete 删除会话
func (fb *FileBackend) Delete(sessionID string) error {
	filePath := filepath.Join(fb.basePath, sessionID+".json")
	return os.Remove(filePath)
}
