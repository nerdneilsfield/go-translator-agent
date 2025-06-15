package translator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/nerdneilsfield/go-translator-agent/internal/formatfix"
	"github.com/nerdneilsfield/go-translator-agent/internal/formatfix/loader"
	"github.com/nerdneilsfield/go-translator-agent/internal/formatter"
	"github.com/nerdneilsfield/go-translator-agent/internal/progress"
	"github.com/nerdneilsfield/go-translator-agent/internal/stats"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// TranslationResult 翻译结果
type TranslationResult struct {
	DocID          string                 `json:"doc_id"`
	InputFile      string                 `json:"input_file"`
	OutputFile     string                 `json:"output_file"`
	SourceLanguage string                 `json:"source_language"`
	TargetLanguage string                 `json:"target_language"`
	TotalNodes     int                    `json:"total_nodes"`
	CompletedNodes int                    `json:"completed_nodes"`
	FailedNodes    int                    `json:"failed_nodes"`
	Progress       float64                `json:"progress"`
	Status         string                 `json:"status"`
	StartTime      time.Time              `json:"start_time"`
	EndTime        *time.Time             `json:"end_time,omitempty"`
	Duration       time.Duration          `json:"duration"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// TranslationCoordinator 翻译协调器，直接集成所有新组件
type TranslationCoordinator struct {
	config             *config.Config
	nodeTranslator     *document.NodeInfoTranslator
	progressTracker    *progress.Tracker
	progressReporter   translator.ProgressReporter
	formatManager      *formatter.Manager
	formatFixRegistry  *formatfix.FixerRegistry
	translationService translation.Service
	postProcessor      *TranslationPostProcessor
	batchTranslator    *BatchTranslator
	statsDB            *stats.Database
	logger             *zap.Logger
}

// NewTranslationCoordinator 创建翻译协调器
func NewTranslationCoordinator(cfg *config.Config, logger *zap.Logger, progressPath string) (*TranslationCoordinator, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	// 创建 progress tracker
	if progressPath == "" {
		progressPath = filepath.Join(".", ".translator-progress")
	}
	progressTracker := progress.NewTracker(logger, progressPath)

	// 创建 progress reporter
	progressReporter := translator.NewProgressTrackerReporter(progressTracker, logger)

	// 创建 node translator
	nodeTranslator := document.NewNodeInfoTranslatorWithProgress(
		cfg.ChunkSize,     // maxChunkSize
		2,                 // contextDistance
		cfg.RetryAttempts, // maxRetries
		progressReporter,  // progressReporter
	)

	// 创建 format manager
	formatManager := formatter.NewFormatterManager()

	// 创建格式修复器注册中心
	var formatFixRegistry *formatfix.FixerRegistry
	var err error
	if cfg.EnableFormatFix {
		if cfg.FormatFixInteractive {
			formatFixRegistry, err = loader.CreateRegistry(logger)
		} else {
			formatFixRegistry, err = loader.CreateSilentRegistry(logger)
		}

		if err != nil {
			logger.Warn("failed to initialize format fix registry", zap.Error(err))
			// 不让格式修复器初始化失败阻止翻译器创建
			formatFixRegistry = nil
		} else {
			logger.Info("format fix registry initialized",
				zap.Bool("interactive", cfg.FormatFixInteractive),
				zap.Bool("pre_translation", cfg.PreTranslationFix),
				zap.Bool("post_translation", cfg.PostTranslationFix),
				zap.Strings("supported_formats", formatFixRegistry.GetSupportedFormats()))
		}
	} else {
		logger.Info("format fix is disabled")
	}

	// 创建统计数据库
	statsPath := filepath.Join(progressPath, "statistics.json")
	statsDB, err := stats.NewDatabase(statsPath, logger)
	if err != nil {
		logger.Warn("failed to initialize statistics database", zap.Error(err))
		// 不让统计错误阻止翻译器创建
		statsDB = nil
	}

	// 创建翻译服务
	translationService, err := createTranslationService(cfg, progressPath, logger)
	if err != nil {
		logger.Warn("failed to create translation service, will use mock translation",
			zap.Error(err))
		// 继续使用 nil，在 translateNode 中会使用模拟翻译
		translationService = nil
	}

	// 创建翻译后处理器
	var postProcessor *TranslationPostProcessor
	if cfg.EnablePostProcessing {
		postProcessor = NewTranslationPostProcessor(cfg, logger)
		logger.Info("translation post processor initialized",
			zap.String("glossary_path", cfg.GlossaryPath),
			zap.Bool("content_protection", cfg.ContentProtection),
			zap.Bool("terminology_consistency", cfg.TerminologyConsistency))
	}
	
	// 创建批量翻译器
	var batchTranslator *BatchTranslator
	if translationService != nil {
		batchTranslator = NewBatchTranslator(cfg, translationService, logger)
		logger.Info("batch translator initialized")
	}

	return &TranslationCoordinator{
		config:             cfg,
		nodeTranslator:     nodeTranslator,
		progressTracker:    progressTracker,
		progressReporter:   progressReporter,
		formatManager:      formatManager,
		formatFixRegistry:  formatFixRegistry,
		translationService: translationService,
		postProcessor:      postProcessor,
		batchTranslator:    batchTranslator,
		statsDB:            statsDB,
		logger:             logger,
	}, nil
}

// TranslateFile 翻译文件
func (c *TranslationCoordinator) TranslateFile(ctx context.Context, inputPath, outputPath string) (*TranslationResult, error) {
	startTime := time.Now()

	// 生成文档 ID
	docID := fmt.Sprintf("file-%d", startTime.UnixNano())

	c.logger.Info("starting file translation",
		zap.String("docID", docID),
		zap.String("inputPath", inputPath),
		zap.String("outputPath", outputPath))

	// 读取输入文件
	contentStr, err := c.readFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read input file: %w", err)
	}

	// 预翻译格式修复
	contentBytes, err := c.preTranslationFormatFix(ctx, inputPath, []byte(contentStr))
	if err != nil {
		c.logger.Warn("pre-translation format fix failed", zap.Error(err))
		// 不让格式修复失败阻止翻译过程
		contentBytes = []byte(contentStr)
	}
	content := string(contentBytes)

	// 检测文件格式并解析
	nodes, err := c.parseDocument(inputPath, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse document: %w", err)
	}

	// 计算总字符数并创建进度条
	totalChars := int64(0)
	for _, node := range nodes {
		totalChars += int64(len(node.OriginalText))
	}
	
	// 创建进度条
	progressBar := NewProgressBar(totalChars, fmt.Sprintf("翻译 %s", inputPath))
	defer progressBar.Finish()

	// 创建带进度条的翻译函数
	translateNodeWithProgress := func(ctx context.Context, node *document.NodeInfo) error {
		// 调用原始翻译函数
		err := c.translateNode(ctx, node)
		
		// 更新进度条
		if err == nil {
			progressBar.Update(int64(len(node.OriginalText)))
		}
		
		return err
	}

	// 执行翻译
	if c.batchTranslator != nil {
		// 使用批量翻译器结合 NodeInfoTranslator
		// 创建批量翻译适配器
		adapter := NewBatchTranslateAdapter(c.batchTranslator, 10)
		
		// 使用 NodeInfoTranslator 管理整体流程（包括重试）
		err = c.nodeTranslator.TranslateDocument(ctx, docID, inputPath, nodes, func(ctx context.Context, node *document.NodeInfo) error {
			// 使用适配器翻译
			err := adapter.TranslateNode(ctx, node)
			if err != nil {
				return err
			}
			
			// 更新进度条
			if node.Status == document.NodeStatusSuccess {
				progressBar.Update(int64(len(node.OriginalText)))
			}
			
			return nil
		})
		
		// 确保刷新最后的缓冲区
		if err == nil {
			err = adapter.Flush(ctx)
		}
		
		if err != nil {
			return c.createFailedResult(docID, inputPath, outputPath, startTime, err), err
		}
	} else {
		// 使用原有的节点翻译器
		err = c.nodeTranslator.TranslateDocument(ctx, docID, inputPath, nodes, translateNodeWithProgress)
		if err != nil {
			return c.createFailedResult(docID, inputPath, outputPath, startTime, err), err
		}
	}

	// 重新组装文档
	translatedContent, err := c.assembleDocument(inputPath, nodes)
	if err != nil {
		return c.createFailedResult(docID, inputPath, outputPath, startTime, err), err
	}

	// 后翻译格式修复
	translatedContentBytes, err := c.postTranslationFormatFix(ctx, inputPath, []byte(translatedContent))
	if err != nil {
		c.logger.Warn("post-translation format fix failed", zap.Error(err))
		// 不让格式修复失败阻止翻译过程
		translatedContentBytes = []byte(translatedContent)
	}
	translatedContent = string(translatedContentBytes)

	// 写入输出文件
	err = c.writeFile(outputPath, translatedContent)
	if err != nil {
		return c.createFailedResult(docID, inputPath, outputPath, startTime, err), err
	}

	// 创建成功结果
	endTime := time.Now()
	result := c.createSuccessResult(docID, inputPath, outputPath, startTime, endTime, nodes)

	// 记录统计数据
	c.recordTranslationStats(result, nodes)

	c.logger.Info("file translation completed",
		zap.String("docID", docID),
		zap.Duration("duration", result.Duration),
		zap.Float64("progress", result.Progress))
	
	// 生成并打印翻译汇总
	summary := GenerateSummary(result, nodes, c.config)
	fmt.Println(summary.FormatSummaryTable())

	return result, nil
}

// TranslateText 翻译文本
func (c *TranslationCoordinator) TranslateText(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}

	startTime := time.Now()
	docID := fmt.Sprintf("text-%d", startTime.UnixNano())
	fileName := "inline-text"

	c.logger.Debug("starting text translation",
		zap.String("docID", docID),
		zap.Int("textLength", len(text)))

	// 创建简单的文本节点
	nodes := c.createTextNodes(text)

	// 执行翻译
	err := c.nodeTranslator.TranslateDocument(ctx, docID, fileName, nodes, c.translateNode)
	if err != nil {
		return "", fmt.Errorf("text translation failed: %w", err)
	}

	// 组装结果
	result := c.assembleTextResult(nodes)

	c.logger.Debug("text translation completed",
		zap.String("docID", docID),
		zap.Duration("duration", time.Since(startTime)))

	return result, nil
}

// translateNode 翻译单个节点
func (c *TranslationCoordinator) translateNode(ctx context.Context, node *document.NodeInfo) error {
	// 检查翻译服务是否可用
	if c.translationService == nil {
		// 模拟翻译（用于测试）
		node.TranslatedText = "Translated: " + node.OriginalText
		node.Status = document.NodeStatusSuccess
		return nil
	}

	// 如果有批量翻译器，使用批量翻译器的单节点翻译
	if c.batchTranslator != nil {
		group := &document.NodeGroup{
			Nodes: []*document.NodeInfo{node},
		}
		return c.batchTranslator.translateGroup(ctx, group)
	}

	// 否则使用基本翻译
	req := &translation.Request{
		Text:           node.OriginalText,
		SourceLanguage: c.config.SourceLang,
		TargetLanguage: c.config.TargetLang,
		Metadata: map[string]interface{}{
			"node_id": node.ID,
		},
	}

	resp, err := c.translationService.Translate(ctx, req)
	if err != nil {
		node.Status = document.NodeStatusFailed
		node.Error = err
		return err
	}

	node.TranslatedText = resp.Text
	node.Status = document.NodeStatusSuccess
	return nil
}

// GetProgress 获取翻译进度
func (c *TranslationCoordinator) GetProgress(docID string) *progress.ProgressInfo {
	return c.progressTracker.GetProgress(docID)
}

// ListSessions 列出所有翻译会话
func (c *TranslationCoordinator) ListSessions() ([]*progress.SessionSummary, error) {
	return c.progressTracker.ListSessions()
}

// ResumeSession 恢复翻译会话
func (c *TranslationCoordinator) ResumeSession(ctx context.Context, sessionID string) (*TranslationResult, error) {
	err := c.progressTracker.LoadSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	progressInfo := c.progressTracker.GetProgress(sessionID)
	if progressInfo == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	c.logger.Info("resuming translation session",
		zap.String("sessionID", sessionID),
		zap.String("fileName", progressInfo.FileName),
		zap.Float64("progress", progressInfo.Progress))

	// 这里可以实现会话恢复逻辑
	// 目前返回当前状态
	result := &TranslationResult{
		DocID:          sessionID,
		InputFile:      progressInfo.FileName,
		SourceLanguage: c.config.SourceLang,
		TargetLanguage: c.config.TargetLang,
		TotalNodes:     progressInfo.TotalChunks,
		CompletedNodes: progressInfo.CompletedChunks,
		FailedNodes:    progressInfo.FailedChunks,
		Progress:       progressInfo.Progress,
		Status:         string(progressInfo.Status),
		StartTime:      progressInfo.StartTime,
	}

	return result, nil
}

// GetActiveSession 获取活跃会话
func (c *TranslationCoordinator) GetActiveSession() ([]*TranslationResult, error) {
	sessions, err := c.ListSessions()
	if err != nil {
		return nil, err
	}

	var activeResults []*TranslationResult
	for _, session := range sessions {
		if session.Status == progress.StatusRunning {
			progressInfo := c.GetProgress(session.ID)
			if progressInfo != nil {
				result := &TranslationResult{
					DocID:          session.ID,
					InputFile:      session.FileName,
					SourceLanguage: c.config.SourceLang,
					TargetLanguage: c.config.TargetLang,
					TotalNodes:     progressInfo.TotalChunks,
					CompletedNodes: progressInfo.CompletedChunks,
					FailedNodes:    progressInfo.FailedChunks,
					Progress:       progressInfo.Progress,
					Status:         string(progressInfo.Status),
					StartTime:      progressInfo.StartTime,
				}
				activeResults = append(activeResults, result)
			}
		}
	}

	return activeResults, nil
}

// recordTranslationStats 记录翻译统计数据
func (c *TranslationCoordinator) recordTranslationStats(result *TranslationResult, nodes []*document.NodeInfo) {
	if c.statsDB == nil {
		return
	}

	// 计算字符数
	totalChars := 0
	for _, node := range nodes {
		totalChars += len(node.OriginalText)
	}

	// 检测文件格式
	format := c.detectFileFormat(result.InputFile)

	// 创建翻译记录
	record := &stats.TranslationRecord{
		ID:             result.DocID,
		Timestamp:      result.StartTime,
		InputFile:      result.InputFile,
		OutputFile:     result.OutputFile,
		SourceLanguage: result.SourceLanguage,
		TargetLanguage: result.TargetLanguage,
		Format:         format,
		TotalNodes:     result.TotalNodes,
		CompletedNodes: result.CompletedNodes,
		FailedNodes:    result.FailedNodes,
		CharacterCount: totalChars,
		Duration:       result.Duration,
		Status:         result.Status,
		Progress:       result.Progress,
		ErrorMessage:   result.ErrorMessage,
		Metadata: map[string]interface{}{
			"chunk_size":      c.config.ChunkSize,
			"retry_attempts":  c.config.RetryAttempts,
			"active_step_set": c.config.ActiveStepSet,
		},
	}

	// 添加到统计数据库
	if err := c.statsDB.AddTranslationRecord(record); err != nil {
		c.logger.Warn("failed to record translation statistics",
			zap.String("docID", result.DocID),
			zap.Error(err))
	} else {
		c.logger.Debug("recorded translation statistics",
			zap.String("docID", result.DocID),
			zap.Int("totalChars", totalChars),
			zap.String("format", format))
	}
}

// detectFileFormat 检测文件格式
func (c *TranslationCoordinator) detectFileFormat(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".md", ".markdown":
		return "markdown"
	case ".txt":
		return "text"
	case ".html", ".htm":
		return "html"
	case ".epub":
		return "epub"
	case ".tex":
		return "latex"
	default:
		return "text"
	}
}

// preTranslationFormatFix 预翻译格式修复
func (c *TranslationCoordinator) preTranslationFormatFix(ctx context.Context, filePath string, content []byte) ([]byte, error) {
	// 检查是否启用预翻译修复
	if !c.config.EnableFormatFix || !c.config.PreTranslationFix {
		c.logger.Debug("pre-translation format fix disabled")
		return content, nil
	}

	if c.formatFixRegistry == nil {
		c.logger.Debug("format fix registry not available, skipping pre-translation fix")
		return content, nil
	}

	format := c.detectFileFormat(filePath)
	if !c.formatFixRegistry.IsFormatSupported(format) {
		c.logger.Debug("format not supported by fix registry", zap.String("format", format))
		return content, nil
	}

	// 检查特定格式是否启用
	if !c.isFormatFixEnabled(format) {
		c.logger.Debug("format fix disabled for this format", zap.String("format", format))
		return content, nil
	}

	c.logger.Info("performing pre-translation format fix",
		zap.String("file", filePath),
		zap.String("format", format),
		zap.Int("contentSize", len(content)))

	fixer, err := c.formatFixRegistry.GetFixerForFormat(format)
	if err != nil {
		return content, fmt.Errorf("failed to get fixer for format %s: %w", format, err)
	}

	// 使用静默修复器进行自动修复
	silentInteractor := formatfix.NewSilentInteractor(true)
	fixedContent, issues, err := fixer.PreTranslationFix(ctx, content, silentInteractor)
	if err != nil {
		return content, fmt.Errorf("pre-translation format fix failed: %w", err)
	}

	if len(issues) > 0 {
		c.logger.Info("pre-translation format fixes applied",
			zap.Int("issuesFixed", len(issues)),
			zap.String("format", format))

		for _, issue := range issues {
			c.logger.Debug("format issue fixed",
				zap.String("type", issue.Type),
				zap.String("severity", issue.Severity.String()),
				zap.Int("line", issue.Line),
				zap.String("message", issue.Message))
		}
	}

	return fixedContent, nil
}

// isFormatFixEnabled 检查特定格式的修复是否启用
func (c *TranslationCoordinator) isFormatFixEnabled(format string) bool {
	switch format {
	case "markdown", "md":
		return c.config.FormatFixMarkdown
	case "text", "txt":
		return c.config.FormatFixText
	case "html", "htm":
		return c.config.FormatFixHTML
	case "epub":
		return c.config.FormatFixEPUB
	default:
		return false
	}
}

// postTranslationFormatFix 后翻译格式修复
func (c *TranslationCoordinator) postTranslationFormatFix(ctx context.Context, filePath string, content []byte) ([]byte, error) {
	// 检查是否启用后翻译修复
	if !c.config.EnableFormatFix || !c.config.PostTranslationFix {
		c.logger.Debug("post-translation format fix disabled")
		return content, nil
	}

	if c.formatFixRegistry == nil {
		c.logger.Debug("format fix registry not available, skipping post-translation fix")
		return content, nil
	}

	format := c.detectFileFormat(filePath)
	if !c.formatFixRegistry.IsFormatSupported(format) {
		c.logger.Debug("format not supported by fix registry", zap.String("format", format))
		return content, nil
	}

	// 检查特定格式是否启用
	if !c.isFormatFixEnabled(format) {
		c.logger.Debug("format fix disabled for this format", zap.String("format", format))
		return content, nil
	}

	c.logger.Info("performing post-translation format fix",
		zap.String("file", filePath),
		zap.String("format", format),
		zap.Int("contentSize", len(content)))

	fixer, err := c.formatFixRegistry.GetFixerForFormat(format)
	if err != nil {
		return content, fmt.Errorf("failed to get fixer for format %s: %w", format, err)
	}

	// 使用静默修复器进行自动修复
	silentInteractor := formatfix.NewSilentInteractor(true)
	fixedContent, issues, err := fixer.PostTranslationFix(ctx, content, silentInteractor)
	if err != nil {
		return content, fmt.Errorf("post-translation format fix failed: %w", err)
	}

	if len(issues) > 0 {
		c.logger.Info("post-translation format fixes applied",
			zap.Int("issuesFixed", len(issues)),
			zap.String("format", format))

		for _, issue := range issues {
			c.logger.Debug("format issue fixed",
				zap.String("type", issue.Type),
				zap.String("severity", issue.Severity.String()),
				zap.Int("line", issue.Line),
				zap.String("message", issue.Message))
		}
	}

	return fixedContent, nil
}
