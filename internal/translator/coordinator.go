package translator

import (
	"context"
	"fmt"
	"os"
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

// FailedNodeDetail 失败节点详细信息
type FailedNodeDetail struct {
	NodeID        int       `json:"node_id"`
	OriginalText  string    `json:"original_text"`
	Path          string    `json:"path"`
	ErrorType     string    `json:"error_type"`
	ErrorMessage  string    `json:"error_message"`
	Step          string    `json:"step,omitempty"`       // 失败的翻译步骤
	StepIndex     int       `json:"step_index,omitempty"` // 步骤索引 (1=初始翻译, 2=反思, 3=改进)
	RetryCount    int       `json:"retry_count"`
	FailureTime   time.Time `json:"failure_time"`
}

// TranslationRoundResult 单轮翻译结果
type TranslationRoundResult struct {
	RoundNumber      int                `json:"round_number"`
	RoundType        string             `json:"round_type"` // "initial" 或 "retry"
	TotalNodes       int                `json:"total_nodes"`
	SuccessNodes     []int              `json:"success_nodes"`     // 本轮成功的节点ID列表
	FailedNodes      []int              `json:"failed_nodes"`      // 本轮失败的节点ID列表
	SuccessCount     int                `json:"success_count"`
	FailedCount      int                `json:"failed_count"`
	Duration         time.Duration      `json:"duration"`
	FailedDetails    []*FailedNodeDetail `json:"failed_details,omitempty"`
}

// DetailedTranslationSummary 详细翻译汇总
type DetailedTranslationSummary struct {
	TotalNodes       int                       `json:"total_nodes"`
	FinalSuccess     int                       `json:"final_success"`
	FinalFailed      int                       `json:"final_failed"`
	TotalRounds      int                       `json:"total_rounds"`
	Rounds           []*TranslationRoundResult `json:"rounds"`
	FinalFailedNodes []*FailedNodeDetail       `json:"final_failed_nodes"`
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
	FailedNodeDetails []*FailedNodeDetail `json:"failed_node_details,omitempty"`
	DetailedSummary   *DetailedTranslationSummary `json:"detailed_summary,omitempty"`
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
			// 使用cache目录作为默认路径
			providerStatsDBPath = filepath.Join(cfg.CacheDir, "provider_stats.json")
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

	// 创建翻译缓存
	var cache translation.Cache
	if cfg.UseCache {
		cacheDir := cfg.CacheDir
		if cacheDir == "" {
			// 使用默认缓存目录
			cacheDir = filepath.Join(progressPath, "translation_cache")
		}
		
		// 如果需要刷新缓存，先清空缓存目录
		if cfg.RefreshCache {
			logger.Info("refreshing translation cache",
				zap.String("cache_dir", cacheDir))
			if err := clearCacheDirectory(cacheDir); err != nil {
				logger.Warn("failed to clear cache directory",
					zap.String("cache_dir", cacheDir),
					zap.Error(err))
			} else {
				logger.Info("cache directory cleared successfully",
					zap.String("cache_dir", cacheDir))
			}
		}
		
		cache = translation.NewCache(cfg.UseCache, cacheDir)
		logger.Info("translation cache initialized",
			zap.Bool("enabled", cfg.UseCache),
			zap.String("cache_dir", cacheDir),
			zap.Bool("refreshed", cfg.RefreshCache))
	} else {
		logger.Info("translation cache disabled")
	}

	// 创建翻译服务（内部自己管理providers）
	translationConfig := translation.NewConfigFromGlobal(cfg)
	var translationServiceOptions []translation.Option
	translationServiceOptions = append(translationServiceOptions, translation.WithLogger(logger))
	if cache != nil {
		translationServiceOptions = append(translationServiceOptions, translation.WithCache(cache))
	}
	
	translationService, err := translation.New(translationConfig, translationServiceOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create translation service: %w", err)
	}
	logger.Info("translation service initialized successfully",
		zap.String("source_lang", cfg.SourceLang),
		zap.String("target_lang", cfg.TargetLang),
		zap.String("active_step_set", cfg.ActiveStepSet),
		zap.Bool("cache_enabled", cache != nil))

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

	// 为BatchTranslator设置进度回调
	if batchTranslator, ok := c.translator.(*BatchTranslator); ok {
		batchTranslator.SetProgressCallback(func(completed, total int, message string) {
			// 更新描述信息（即使completed=0也要更新）
			if message != "" {
				progressBar.SetDescription(fmt.Sprintf("翻译 %s - %s", inputPath, message))
			}
			
			// 只有在有实际进度时才更新进度条数值
			if completed > 0 && total > 0 {
				// 根据完成的节点数量估算已处理的字符数
				avgCharsPerNode := float64(totalChars) / float64(len(nodes))
				processedChars := int64(float64(completed) * avgCharsPerNode)
				
				// 更新进度条（但不超过总字符数）
				if processedChars <= totalChars {
					// 使用SetCurrent直接设置当前进度，避免累积误差
					progressBar.bar.ChangeMax64(totalChars)
					progressBar.bar.Set64(processedChars)
					progressBar.processedChars = processedChars
				}
			}
		})
	}

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

// clearCacheDirectory 清空缓存目录
func clearCacheDirectory(cacheDir string) error {
	if cacheDir == "" {
		return fmt.Errorf("cache directory is empty")
	}
	
	// 检查目录是否存在
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		// 目录不存在，直接返回成功
		return nil
	}
	
	// 清空目录内容（保留目录本身）
	return filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// 跳过根目录本身
		if path == cacheDir {
			return nil
		}
		
		// 删除文件或目录
		return os.RemoveAll(path)
	})
}

// PrintDetailedTranslationSummary 打印详细的翻译汇总信息
func (c *TranslationCoordinator) PrintDetailedTranslationSummary(result *TranslationResult) {
	if result.DetailedSummary == nil {
		// 回退到简单的失败节点显示
		c.PrintFailedNodesSummary(result)
		return
	}
	
	summary := result.DetailedSummary
	
	fmt.Printf("\n📊 详细翻译汇总报告\n")
	fmt.Println(strings.Repeat("=", 80))
	
	// 总体统计
	fmt.Printf("📈 总体统计:\n")
	fmt.Printf("  📋 总节点数: %d\n", summary.TotalNodes)
	fmt.Printf("  ✅ 最终成功: %d (%.1f%%)\n", summary.FinalSuccess, 
		float64(summary.FinalSuccess)/float64(summary.TotalNodes)*100)
	fmt.Printf("  ❌ 最终失败: %d (%.1f%%)\n", summary.FinalFailed,
		float64(summary.FinalFailed)/float64(summary.TotalNodes)*100)
	fmt.Printf("  🔄 翻译轮次: %d\n", summary.TotalRounds)
	fmt.Println()
	
	// 每轮翻译详情
	fmt.Printf("🔄 每轮翻译详情:\n")
	for i, round := range summary.Rounds {
		fmt.Printf("\n第 %d 轮 (%s):\n", round.RoundNumber, getRoundTypeDisplayName(round.RoundType))
		fmt.Printf("  📊 处理节点: %d\n", round.TotalNodes)
		fmt.Printf("  ✅ 成功: %d", round.SuccessCount)
		if len(round.SuccessNodes) > 0 {
			fmt.Printf(" (节点ID: %v)", round.SuccessNodes)
		}
		fmt.Println()
		fmt.Printf("  ❌ 失败: %d", round.FailedCount)
		if len(round.FailedNodes) > 0 {
			fmt.Printf(" (节点ID: %v)", round.FailedNodes)
		}
		fmt.Println()
		fmt.Printf("  ⏱️  耗时: %v\n", round.Duration)
		
		// 如果是最后一轮或有失败，显示错误类型统计
		if round.FailedCount > 0 && (i == len(summary.Rounds)-1 || round.RoundType == "retry") {
			errorTypes := make(map[string]int)
			for _, detail := range round.FailedDetails {
				errorTypes[detail.ErrorType]++
			}
			if len(errorTypes) > 0 {
				fmt.Printf("  📋 错误类型分布:\n")
				for errorType, count := range errorTypes {
					fmt.Printf("    - %s: %d个\n", getErrorTypeDisplayName(errorType), count)
				}
			}
		}
	}
	
	// 最终失败节点详情
	if len(summary.FinalFailedNodes) > 0 {
		fmt.Printf("\n❌ 最终失败节点详情 (%d个):\n", len(summary.FinalFailedNodes))
		
		maxDisplay := 5 // 只显示前5个最终失败的节点
		if len(summary.FinalFailedNodes) < maxDisplay {
			maxDisplay = len(summary.FinalFailedNodes)
		}
		
		for i := 0; i < maxDisplay; i++ {
			detail := summary.FinalFailedNodes[i]
			fmt.Printf("\n失败节点 #%d (ID: %d):\n", i+1, detail.NodeID)
			fmt.Printf("  📍 路径: %s\n", detail.Path)
			fmt.Printf("  🔄 重试次数: %d\n", detail.RetryCount)
			fmt.Printf("  ⚠️  错误类型: %s\n", getErrorTypeDisplayName(detail.ErrorType))
			
			// 显示失败的翻译步骤信息
			if detail.Step != "" {
				stepName := getStepDisplayName(detail.Step)
				fmt.Printf("  🔧 失败步骤: %s", stepName)
				if detail.StepIndex > 0 {
					fmt.Printf(" (第%d步)", detail.StepIndex)
				}
				fmt.Printf("\n")
			}
			
			fmt.Printf("  💬 错误信息: %s\n", detail.ErrorMessage)
			fmt.Printf("  📝 原文预览: %s\n", detail.OriginalText)
		}
		
		if len(summary.FinalFailedNodes) > maxDisplay {
			fmt.Printf("\n... 还有 %d 个失败节点未显示\n", len(summary.FinalFailedNodes)-maxDisplay)
		}
	}
	
	fmt.Println(strings.Repeat("=", 80))
}

// PrintFailedNodesSummary 打印失败节点的详细信息（简化版，用作回退）
func (c *TranslationCoordinator) PrintFailedNodesSummary(result *TranslationResult) {
	if len(result.FailedNodeDetails) == 0 {
		return
	}
	
	fmt.Printf("\n❌ 失败节点详细信息 (%d个):\n", len(result.FailedNodeDetails))
	fmt.Println(strings.Repeat("=", 80))
	
	// 按错误类型分组统计
	errorTypeCount := make(map[string]int)
	for _, detail := range result.FailedNodeDetails {
		errorTypeCount[detail.ErrorType]++
	}
	
	// 显示错误类型统计
	fmt.Println("错误类型统计:")
	for errorType, count := range errorTypeCount {
		fmt.Printf("  - %s: %d个\n", getErrorTypeDisplayName(errorType), count)
	}
	fmt.Println()
	
	// 显示前10个失败节点的详细信息
	maxDisplay := 10
	if len(result.FailedNodeDetails) < maxDisplay {
		maxDisplay = len(result.FailedNodeDetails)
	}
	
	fmt.Printf("前 %d 个失败节点详情:\n", maxDisplay)
	for i := 0; i < maxDisplay; i++ {
		detail := result.FailedNodeDetails[i]
		fmt.Printf("\n节点 #%d (ID: %d):\n", i+1, detail.NodeID)
		fmt.Printf("  📍 路径: %s\n", detail.Path)
		fmt.Printf("  🔄 重试次数: %d\n", detail.RetryCount)
		fmt.Printf("  ⚠️  错误类型: %s\n", getErrorTypeDisplayName(detail.ErrorType))
		fmt.Printf("  💬 错误信息: %s\n", detail.ErrorMessage)
		fmt.Printf("  📝 原文预览: %s\n", detail.OriginalText)
		fmt.Printf("  ⏰ 失败时间: %s\n", detail.FailureTime.Format("2006-01-02 15:04:05"))
	}
	
	if len(result.FailedNodeDetails) > maxDisplay {
		fmt.Printf("\n... 还有 %d 个失败节点未显示\n", len(result.FailedNodeDetails)-maxDisplay)
	}
	
	fmt.Println(strings.Repeat("=", 80))
}

// getRoundTypeDisplayName 获取轮次类型的显示名称
func getRoundTypeDisplayName(roundType string) string {
	switch roundType {
	case "initial":
		return "初始翻译"
	case "retry":
		return "重试翻译"
	default:
		return roundType
	}
}

// getErrorTypeDisplayName 获取错误类型的显示名称
func getErrorTypeDisplayName(errorType string) string {
	switch errorType {
	case "timeout":
		return "超时错误"
	case "rate_limit":
		return "频率限制"
	case "network":
		return "网络错误"
	case "canceled":
		return "操作取消"
	case "similarity_check_failed":
		return "相似度检查失败"
	case "invalid_response":
		return "无效响应"
	case "auth_error":
		return "认证错误"
	case "quota_exceeded":
		return "配额超出"
	case "unknown":
		return "未知错误"
	default:
		return errorType
	}
}

// getStepDisplayName 获取翻译步骤的显示名称
func getStepDisplayName(step string) string {
	switch step {
	case "initial_translation":
		return "初始翻译"
	case "reflection":
		return "反思阶段"
	case "improvement":
		return "改进阶段"
	default:
		return step
	}
}