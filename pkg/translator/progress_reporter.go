package translator

import (
	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/nerdneilsfield/go-translator-agent/internal/progress"
	"go.uber.org/zap"
)

// ProgressTrackerReporter 将 ProgressReporter 接口桥接到 progress.Tracker
type ProgressTrackerReporter struct {
	tracker *progress.Tracker
	logger  *zap.Logger
}

// NewProgressTrackerReporter 创建新的进度报告器
func NewProgressTrackerReporter(tracker *progress.Tracker, logger *zap.Logger) *ProgressTrackerReporter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ProgressTrackerReporter{
		tracker: tracker,
		logger:  logger,
	}
}

// StartDocument 开始文档翻译
func (r *ProgressTrackerReporter) StartDocument(docID, fileName string, totalNodes int) {
	if r.tracker == nil {
		return
	}

	r.logger.Info("starting document translation",
		zap.String("docID", docID),
		zap.String("fileName", fileName),
		zap.Int("totalNodes", totalNodes))

	r.tracker.StartTracking(docID, fileName)
}

// UpdateNode 更新节点进度
func (r *ProgressTrackerReporter) UpdateNode(docID string, nodeID int, status document.NodeStatus, charCount int, err error) {
	if r.tracker == nil {
		return
	}

	r.logger.Debug("updating node progress",
		zap.String("docID", docID),
		zap.Int("nodeID", nodeID),
		zap.Any("status", status),
		zap.Int("charCount", charCount),
		zap.Error(err))

	r.tracker.UpdateNodeProgress(docID, nodeID, status, charCount, err)
}

// CompleteDocument 完成文档翻译
func (r *ProgressTrackerReporter) CompleteDocument(docID string) {
	if r.tracker == nil {
		return
	}

	r.logger.Info("completing document translation",
		zap.String("docID", docID))

	r.tracker.StopTracking(docID)
}

// UpdateStep 更新翻译步骤（用于三步翻译流程）
func (r *ProgressTrackerReporter) UpdateStep(docID string, nodeID int, step int, stepName string) {
	if r.tracker == nil {
		return
	}

	r.logger.Debug("updating translation step",
		zap.String("docID", docID),
		zap.Int("nodeID", nodeID),
		zap.Int("step", step),
		zap.String("stepName", stepName))

	// 目前 progress 系统还不支持步骤级别的跟踪
	// 这里可以在日志中记录，未来可以扩展 progress 系统来支持
}

// GetProgress 获取进度信息
func (r *ProgressTrackerReporter) GetProgress(docID string) *progress.ProgressInfo {
	if r.tracker == nil {
		return nil
	}
	return r.tracker.GetProgress(docID)
}
