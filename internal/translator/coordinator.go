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
	providerStats "github.com/nerdneilsfield/go-translator-agent/pkg/providers/stats"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"go.uber.org/zap"
)

// CoordinatorConfig Coordinator包专用配置，管理文档解析、格式修复、后处理等功能
type CoordinatorConfig struct {
	// 文档处理配置
	HTMLProcessingMode string // HTML处理模式: "markdown" 或 "native"
	ChunkSize          int    // 文档处理时的分块大小
	SourceLang         string // 源语言（用于文档处理元数据）
	TargetLang         string // 目标语言（用于文档处理元数据）

	// 格式修复配置
	EnableFormatFix      bool
	FormatFixInteractive bool
	PreTranslationFix    bool
	PostTranslationFix   bool
	FormatFixMarkdown    bool
	FormatFixText        bool
	FormatFixHTML        bool
	FormatFixEPUB        bool

	// 后处理配置
	EnablePostProcessing      bool
	GlossaryPath              string
	ContentProtection         bool
	TerminologyConsistency    bool
	MixedLanguageSpacing      bool
	MachineTranslationCleanup bool

	// 进度和调试配置
	Verbose bool // 详细模式
}

// NewCoordinatorConfig 从全局配置创建Coordinator专用配置
func NewCoordinatorConfig(cfg *config.Config) CoordinatorConfig {
	return CoordinatorConfig{
		HTMLProcessingMode: cfg.HTMLProcessingMode,
		ChunkSize:          cfg.ChunkSize,
		SourceLang:         cfg.SourceLang,
		TargetLang:         cfg.TargetLang,

		EnableFormatFix:      cfg.EnableFormatFix,
		FormatFixInteractive: cfg.FormatFixInteractive,
		PreTranslationFix:    cfg.PreTranslationFix,
		PostTranslationFix:   cfg.PostTranslationFix,
		FormatFixMarkdown:    cfg.FormatFixMarkdown,
		FormatFixText:        cfg.FormatFixText,
		FormatFixHTML:        cfg.FormatFixHTML,
		FormatFixEPUB:        cfg.FormatFixEPUB,

		EnablePostProcessing:      cfg.EnablePostProcessing,
		GlossaryPath:              cfg.GlossaryPath,
		ContentProtection:         cfg.ContentProtection,
		TerminologyConsistency:    cfg.TerminologyConsistency,
		MixedLanguageSpacing:      cfg.MixedLanguageSpacing,
		MachineTranslationCleanup: cfg.MachineTranslationCleanup,

		Verbose: cfg.Verbose,
	}
}

// TranslationResult 翻译结果
type TranslationResult struct {
	DocID          string                 `json:"doc_id"`
	InputFile      string                 `json:"input_file"`
	OutputFile     string                 `json:"output_file"`
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

// TranslationCoordinator 翻译协调器，只负责文档解析、组装和工作流协调
type TranslationCoordinator struct {
	coordinatorConfig  CoordinatorConfig   // Coordinator专用配置
	translationService translation.Service // 翻译服务实例
	translator         Translator                     // 节点翻译管理器实例
	progressTracker    *progress.Tracker
	progressReporter   *progress.Tracker
	formatManager      *formatter.Manager
	formatFixRegistry  *formatfix.FixerRegistry
	postProcessor      *TranslationPostProcessor
	statsDB            *stats.Database
	providerStatsManager *providerStats.StatsManager  // Provider性能统计管理器
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

	// 创建Coordinator专用配置
	coordinatorConfig := NewCoordinatorConfig(cfg)

	// 创建 progress tracker
	if progressPath == "" {
		progressPath = filepath.Join(".", ".translator-progress")
	}
	progressTracker := progress.NewTracker(logger, progressPath)

	// 创建 progress reporter
	progressReporter := progressTracker

	// 使用文档处理器注册表
	formats := document.GetRegisteredFormats()
	formatStrings := make([]string, len(formats))
	for i, format := range formats {
		formatStrings[i] = string(format)
	}
	logger.Info("document processor registry initialized",
		zap.Strings("supported_formats", formatStrings))

	// 创建 format manager
	formatManager := formatter.NewFormatterManager()

	// 创建格式修复器注册中心
	var formatFixRegistry *formatfix.FixerRegistry
	var err error
	if coordinatorConfig.EnableFormatFix {
		if coordinatorConfig.FormatFixInteractive {
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
				zap.Bool("interactive", coordinatorConfig.FormatFixInteractive),
				zap.Bool("pre_translation", coordinatorConfig.PreTranslationFix),
				zap.Bool("post_translation", coordinatorConfig.PostTranslationFix),
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

	// 创建 Provider 性能统计管理器
	var providerStatsManager *providerStats.StatsManager
	if cfg.EnableStats {
		providerStatsDBPath := cfg.StatsDBPath
		if providerStatsDBPath == "" {
			providerStatsDBPath = filepath.Join(progressPath, "provider_stats.json")
		}
		providerStatsManager = providerStats.NewStatsManager(providerStatsDBPath, logger)
		
		// 加载已有统计数据
		if err := providerStatsManager.LoadFromDB(); err != nil {
			logger.Warn("failed to load provider stats from database", zap.Error(err))
		}
		
		logger.Info("provider statistics manager initialized", 
			zap.String("db_path", providerStatsDBPath),
			zap.Int("save_interval_seconds", cfg.StatsSaveInterval))
		
		// 启动自动保存協程
		if cfg.StatsSaveInterval > 0 {
			go providerStatsManager.AutoSaveRoutine(context.Background(), time.Duration(cfg.StatsSaveInterval)*time.Second)
		}
	} else {
		logger.Info("provider statistics disabled")
	}

	// 创建翻译服务（内部自己管理providers）
	translationConfig := translation.NewConfigFromGlobal(cfg)
	translationService, err := translation.New(translationConfig, translation.WithLogger(logger))
	if err != nil {
		return nil, fmt.Errorf("failed to create translation service: %w", err)
	}
	logger.Info("translation service initialized successfully",
		zap.String("source_lang", cfg.SourceLang),
		zap.String("target_lang", cfg.TargetLang),
		zap.String("active_step_set", cfg.ActiveStepSet))

	// 创建节点翻译管理器
	translatorConfig := NewTranslatorConfig(cfg)
	translator := NewBatchTranslator(translatorConfig, translationService, logger, providerStatsManager)
	logger.Info("translator initialized",
		zap.Int("chunk_size", translatorConfig.ChunkSize),
		zap.Int("concurrency", translatorConfig.Concurrency),
		zap.Int("max_retries", translatorConfig.MaxRetries),
		zap.Bool("stats_enabled", cfg.EnableStats))

	// 创建翻译后处理器
	var postProcessor *TranslationPostProcessor
	if coordinatorConfig.EnablePostProcessing {
		postProcessor = NewTranslationPostProcessor(cfg, logger)
		logger.Info("translation post processor initialized",
			zap.String("glossary_path", coordinatorConfig.GlossaryPath),
			zap.Bool("content_protection", coordinatorConfig.ContentProtection),
			zap.Bool("terminology_consistency", coordinatorConfig.TerminologyConsistency))
	}

	return &TranslationCoordinator{
		coordinatorConfig:    coordinatorConfig,
		translationService:   translationService,
		translator:           translator,
		progressTracker:      progressTracker,
		progressReporter:     progressReporter,
		formatManager:        formatManager,
		formatFixRegistry:    formatFixRegistry,
		postProcessor:        postProcessor,
		statsDB:              statsDB,
		providerStatsManager: providerStatsManager,
		logger:               logger,
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

	// 使用完善的document processor替代简化解析
	processorOpts := document.ProcessorOptions{
		ChunkSize:    c.coordinatorConfig.ChunkSize,
		ChunkOverlap: 100,
		Metadata: map[string]interface{}{
			"source_language":      c.coordinatorConfig.SourceLang,
			"target_language":      c.coordinatorConfig.TargetLang,
			"logger":               c.logger,
			"html_processing_mode": c.coordinatorConfig.HTMLProcessingMode,
		},
	}

	// 获取适当的document processor
	processor, err := document.GetProcessorByExtension(inputPath, processorOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to get document processor: %w", err)
	}

	c.logger.Info("using document processor",
		zap.String("format", string(processor.GetFormat())),
		zap.Int("chunk_size", processorOpts.ChunkSize))

	// 解析文档
	parseCtx := context.Background()
	doc, err := processor.Parse(parseCtx, strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse document: %w", err)
	}

	// 从文档中提取节点进行翻译
	nodes := c.extractNodesFromDocument(doc)
	if len(nodes) == 0 {
		c.logger.Info("no translatable nodes found in document")
		// 直接写入原始内容
		err = c.writeFile(outputPath, content)
		if err != nil {
			return c.createFailedResult(docID, inputPath, outputPath, startTime, err), err
		}
		return c.createSuccessResult(docID, inputPath, outputPath, startTime, time.Now(), nodes), nil
	}

	// 计算总字符数并创建进度条
	totalChars := int64(0)
	for _, node := range nodes {
		totalChars += int64(len(node.OriginalText))
	}

	// 创建进度条
	progressBar := NewProgressBar(totalChars, fmt.Sprintf("翻译 %s", inputPath))
	defer progressBar.Finish()

	// 使用Translator进行节点分组和并行翻译
	err = c.translator.TranslateNodes(ctx, nodes)
	if err != nil {
		return c.createFailedResult(docID, inputPath, outputPath, startTime, err), err
	}

	// 重建文档结构并渲染
	translatedContent, err := c.assembleDocumentWithProcessor(inputPath, doc, nodes)
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
	// TODO: 修复GenerateSummary函数的参数类型
	// summary := GenerateSummary(result, nodes, c.coordinatorConfig)
	// fmt.Println(summary.FormatSummaryTable())

	return result, nil
}

// TranslateText 翻译文本
func (c *TranslationCoordinator) TranslateText(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}

	startTime := time.Now()
	docID := fmt.Sprintf("text-%d", startTime.UnixNano())
	// fileName := "inline-text" // 暂时不使用

	c.logger.Debug("starting text translation",
		zap.String("docID", docID),
		zap.Int("textLength", len(text)))

	// 直接使用翻译服务翻译文本
	translatedText, err := c.translationService.TranslateText(ctx, text)
	if err != nil {
		return "", fmt.Errorf("text translation failed: %w", err)
	}

	result := translatedText

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

	// 使用完善的翻译服务
	translatedText, err := c.translationService.TranslateText(ctx, node.OriginalText)
	if err != nil {
		node.Status = document.NodeStatusFailed
		node.Error = err
		c.logger.Error("node translation failed",
			zap.Int("nodeID", node.ID),
			zap.String("path", node.Path),
			zap.Error(err))
		return err
	}

	node.TranslatedText = translatedText
	node.Status = document.NodeStatusSuccess

	c.logger.Debug("node translation completed",
		zap.Int("nodeID", node.ID),
		zap.String("path", node.Path),
		zap.Int("inputLength", len(node.OriginalText)),
		zap.Int("outputLength", len(translatedText)))

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
		SourceLanguage: c.coordinatorConfig.SourceLang,
		TargetLanguage: c.coordinatorConfig.TargetLang,
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
			"chunk_size": c.coordinatorConfig.ChunkSize,
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
	if !c.coordinatorConfig.EnableFormatFix || !c.coordinatorConfig.PreTranslationFix {
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
		return c.coordinatorConfig.FormatFixMarkdown
	case "text", "txt":
		return c.coordinatorConfig.FormatFixText
	case "html", "htm":
		return c.coordinatorConfig.FormatFixHTML
	case "epub":
		return c.coordinatorConfig.FormatFixEPUB
	default:
		return false
	}
}

// postTranslationFormatFix 后翻译格式修复
func (c *TranslationCoordinator) postTranslationFormatFix(ctx context.Context, filePath string, content []byte) ([]byte, error) {
	// 检查是否启用后翻译修复
	if !c.coordinatorConfig.EnableFormatFix || !c.coordinatorConfig.PostTranslationFix {
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
