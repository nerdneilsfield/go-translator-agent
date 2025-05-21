package translator

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	customprogress "github.com/nerdneilsfield/go-translator-agent/pkg/progress"
	"go.uber.org/zap"
)

// snippet returns a small excerpt of the provided text for debug logging.
// It includes the beginning, a middle section and the end of the string,
// separated by ellipses if the text is long.
func snippet(s string) string {
	const seg = 20
	if len(s) <= seg*3 {
		return s
	}
	start := s[:seg]
	midIdx := len(s) / 2
	midStart := midIdx - seg/2
	if midStart < seg {
		midStart = seg
	}
	mid := s[midStart : midStart+seg]
	end := s[len(s)-seg:]
	return start + " ... " + mid + " ... " + end
}

type ModelPrice struct {
	InitialModelInputPrice      float64
	InitialModelOutputPrice     float64
	InitialModelPriceUnit       string
	ReflectionModelInputPrice   float64
	ReflectionModelOutputPrice  float64
	ReflectionModelPriceUnit    string
	ImprovementModelInputPrice  float64
	ImprovementModelOutputPrice float64
	ImprovementModelPriceUnit   string
}

type TranslationRecord struct {
	Source string `json:"source"`
	Result string `json:"result"`
}

// StepModelInfo 描述翻译流程各阶段所使用的模型名称
type StepModelInfo struct {
	Initial     string `json:"initial"`
	Reflection  string `json:"reflection"`
	Improvement string `json:"improvement"`
}

// Impl is the default implementation of the Translator interface.
type Impl struct {
	config             *config.Config
	models             map[string]LLMClient
	logger             logger.Logger
	activeSteps        *StepSetConfig
	cache              Cache
	forceCacheRefresh  bool // 强制刷新缓存
	progressBar        *customprogress.Tracker
	progressTracker    *TranslationProgressTracker
	newProgressTracker *NewProgressTracker
	useNewProgressBar  bool // 使用新的进度条系统
	modelPrice         ModelPrice

	debugRecords []TranslationRecord

	client RawClient
	Logger *zap.Logger
}

// StepSetConfig 包含三步翻译流程的配置
type StepSetConfig struct {
	ID                string
	Name              string
	Description       string
	InitialModel      LLMClient
	ReflectionModel   LLMClient
	ImprovementModel  LLMClient
	FastModeThreshold int
}

func NewModelPrice(initialModel LLMClient, reflectionModel LLMClient, improvementModel LLMClient) ModelPrice {
	if initialModel == nil {
		return ModelPrice{}
	}
	if reflectionModel == nil && improvementModel == nil {
		return ModelPrice{
			InitialModelInputPrice:      initialModel.GetInputTokenPrice(),
			InitialModelOutputPrice:     initialModel.GetOutputTokenPrice(),
			InitialModelPriceUnit:       initialModel.GetPriceUnit(),
			ReflectionModelInputPrice:   0,
			ReflectionModelOutputPrice:  0,
			ReflectionModelPriceUnit:    "",
			ImprovementModelInputPrice:  0,
			ImprovementModelOutputPrice: 0,
			ImprovementModelPriceUnit:   "",
		}
	}
	return ModelPrice{
		InitialModelInputPrice:      initialModel.GetInputTokenPrice(),
		InitialModelOutputPrice:     initialModel.GetOutputTokenPrice(),
		InitialModelPriceUnit:       initialModel.GetPriceUnit(),
		ReflectionModelInputPrice:   reflectionModel.GetInputTokenPrice(),
		ReflectionModelOutputPrice:  reflectionModel.GetOutputTokenPrice(),
		ReflectionModelPriceUnit:    reflectionModel.GetPriceUnit(),
		ImprovementModelInputPrice:  improvementModel.GetInputTokenPrice(),
		ImprovementModelOutputPrice: improvementModel.GetOutputTokenPrice(),
		ImprovementModelPriceUnit:   improvementModel.GetPriceUnit(),
	}
}

// translatorOptions 结构体定义 - 确保这是包内唯一的顶级定义
// type translatorOptions struct {
// 	cache             Cache
// 	forceCacheRefresh bool
// 	progressBar       *customprogress.Tracker // 类型应为 *customprogress.Tracker
// 	useNewProgressBar bool
// }

// // Option 类型定义 - 确保这是包内唯一的顶级定义
// type Option func(*translatorOptions)

// New creates a new Impl.
func New(cfg *config.Config, opts ...Option) (*Impl, error) {
	options := translatorOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	var zapLogger *zap.Logger
	var err error
	if cfg.Debug {
		zapLogger, err = zap.NewDevelopment()
	} else {
		zapLogger, err = zap.NewProduction()
	}
	if err != nil {
		return nil, fmt.Errorf("创建 zapLogger 失败: %w", err)
	}

	log := logger.NewZapLogger(cfg.Debug)

	models, err := initModels(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("初始化模型失败: %w", err)
	}

	stepSetConf, ok := cfg.StepSets[cfg.ActiveStepSet]
	if !ok {
		return nil, fmt.Errorf("未找到活动的步骤集配置: %s", cfg.ActiveStepSet)
	}

	initialModel := models[stepSetConf.InitialTranslation.ModelName]
	reflectionModel := models[stepSetConf.Reflection.ModelName]
	improvementModel := models[stepSetConf.Improvement.ModelName]

	activeSteps := &StepSetConfig{
		ID:                stepSetConf.ID,
		Name:              stepSetConf.Name,
		Description:       stepSetConf.Description,
		InitialModel:      initialModel,
		ReflectionModel:   reflectionModel,
		ImprovementModel:  improvementModel,
		FastModeThreshold: stepSetConf.FastModeThreshold,
	}

	modelPrice := NewModelPrice(initialModel, reflectionModel, improvementModel)

	var llmClient RawClient
	if rawModel, exists := models["raw"]; exists {
		if rc, ok := rawModel.(*RawClient); ok {
			llmClient = *rc
		} else {
			llmClient = *NewRawClient()
		}
	} else {
		llmClient = *NewRawClient()
	}

	var cacher Cache
	if cfg.UseCache {
		if options.cache != nil {
			cacher = options.cache
		} else {
			if cfg.CacheDir != "" {
				fileCache := newFileCache(cfg.CacheDir)
				cacher = fileCache
			} else {
				cacher = NewMemoryCache()
				log.Info("缓存目录未配置或为空，使用内存缓存")
			}
		}
	} else {
		log.Info("缓存已禁用")
		cacher = NewMemoryCache()
	}

	translator := &Impl{
		config:            cfg,
		models:            models,
		logger:            log,
		activeSteps:       activeSteps,
		cache:             cacher,
		forceCacheRefresh: options.forceCacheRefresh,
		progressBar:       nil,
		progressTracker:   NewTranslationProgressTracker(0, zapLogger, cfg.TargetCurrency, cfg.UsdRmbRate),
		useNewProgressBar: options.useNewProgressBar,
		modelPrice:        modelPrice,
		client:            llmClient,
		Logger:            zapLogger,
	}

	if cfg.SaveDebugInfo {
		translator.debugRecords = []TranslationRecord{}
	}

	if translator.useNewProgressBar {
		translator.newProgressTracker = NewNewProgressTracker(0, zapLogger)
		translator.newProgressTracker.UpdateModelPrice(modelPrice)
	}

	if translator.forceCacheRefresh && translator.cache != nil {
		translator.logger.Info("正在强制刷新缓存...")
		if err := translator.cache.Clear(); err != nil {
			translator.logger.Warn("强制刷新缓存失败", zap.Error(err))
		}
	}

	return translator, nil
}

func (t *Impl) RemoveUsedTags(text string) string {
	result := text

	// 移除常见的提示词标记
	tagsToRemove := []struct {
		start string
		end   string
	}{
		{start: "<SOURCE_TEXT>", end: "</SOURCE_TEXT>"},
		{start: "<TRANSLATION>", end: "</TRANSLATION>"},
		{start: "<EXPERT_SUGGESTIONS>", end: "</EXPERT_SUGGESTIONS>"},
		{start: "<TEXT TO EDIT>", end: "</TEXT TO EDIT>"},
		{start: "<TEXT TO TRANSLATE>", end: "</TEXT TO TRANSLATE>"},
		{start: "<TRANSLATE_THIS>", end: "</TRANSLATE_THIS>"},
		{start: "<翻译>", end: "</翻译>"},
		{start: "<翻译后的文本>", end: "</翻译后的文本>"},
		{start: "<TEXT TRANSLATED>", end: "</TEXT TRANSLATED>"},
	}

	// 移除成对的标记
	for _, tag := range tagsToRemove {
		// 先尝试移除完整的标记对
		for {
			startIdx := strings.Index(result, tag.start)
			if startIdx == -1 {
				break
			}

			endIdx := strings.Index(result, tag.end)
			if endIdx == -1 || endIdx < startIdx {
				break
			}

			// 保留标记之间的内容，移除标记本身
			content := result[startIdx+len(tag.start) : endIdx]
			result = result[:startIdx] + content + result[endIdx+len(tag.end):]
		}

		// 然后移除任何剩余的单独标记
		result = strings.ReplaceAll(result, tag.start, "")
		result = strings.ReplaceAll(result, tag.end, "")
	}

	// 使用正则表达式移除其他可能的提示词标记
	promptTagsRegex := []*regexp.Regexp{
		regexp.MustCompile(`</?[A-Z_]+>`),                   // 如 <TRANSLATION> 或 </TRANSLATION>
		regexp.MustCompile(`</?[a-z_]+>`),                   // 如 <translation> 或 </translation>
		regexp.MustCompile(`</?[\p{Han}]+>`),                // 中文标记，如 <翻译> 或 </翻译>
		regexp.MustCompile(`</?[\p{Han}][^>]{0,20}>`),       // 带属性的中文标记
		regexp.MustCompile(`\[INTERNAL INSTRUCTIONS:.*?\]`), // 内部指令
	}

	for _, regex := range promptTagsRegex {
		result = regex.ReplaceAllString(result, "")
	}

	// 修复可能的格式问题
	result = t.fixFormatIssues(result)

	return strings.TrimSpace(result)
}

// fixFormatIssues 修复翻译结果中的格式问题
func (t *Impl) fixFormatIssues(text string) string {
	result := text

	// 修复错误的斜体标记（确保*前后有空格或在行首尾）
	italicRegex := regexp.MustCompile(`(\S)\*(\S)`)
	result = italicRegex.ReplaceAllString(result, "$1 * $2")

	// 修复错误的粗体标记
	boldRegex := regexp.MustCompile(`(\S)\*\*(\S)`)
	result = boldRegex.ReplaceAllString(result, "$1 ** $2")

	// 移除多余的空行
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}

	// 移除行首行尾多余的空格
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	result = strings.Join(lines, "\n")

	return result
}

// GetLogger 返回日志记录器
func (t *Impl) GetLogger() logger.Logger {
	return t.logger
}

// shouldUseCache 判断是否应该使用缓存
func (t *Impl) shouldUseCache() bool {
	return t.config.UseCache
}

// refreshCache 刷新缓存
func (t *Impl) refreshCache() error {
	if t.forceCacheRefresh {
		t.logger.Info("正在刷新缓存")
		return t.cache.Clear()
	}
	return nil
}

func (t *Impl) InitTranslator() {
	if t.useNewProgressBar {
		// 使用新的进度条系统
		if t.newProgressTracker == nil {
			t.newProgressTracker = NewNewProgressTracker(0, t.Logger)
			t.newProgressTracker.UpdateModelPrice(t.modelPrice)
		}
		t.newProgressTracker.Start()

		// 从旧的进度跟踪器获取总字符数并设置到新的进度跟踪器
		totalCharsFromOldTracker, _, _, _, _, _, _ := t.progressTracker.GetProgress()
		if totalCharsFromOldTracker > 0 {
			t.newProgressTracker.SetTotalChars(totalCharsFromOldTracker)
			t.Logger.Debug("设置新进度条总字符数", zap.Int("totalChars", totalCharsFromOldTracker))
		}

		// 记录初始化信息
		if logger, ok := t.logger.(interface{ Info(string, ...zap.Field) }); ok {
			logger.Info("翻译器已初始化（使用新进度条系统）",
				zap.String("源语言", t.config.SourceLang),
				zap.String("目标语言", t.config.TargetLang),
				zap.String("活动步骤集", t.config.ActiveStepSet))
		}
	} else {
		// 使用旧的进度条系统
		// 确保进度跟踪器已初始化
		var totalCharsFromDataForUI int // Renamed to avoid conflict
		totalCharsFromDataForUI, _, _, _, _, _, _ = t.progressTracker.GetProgress()
		if totalCharsFromDataForUI <= 0 {
			// 如果总字符数未设置，设置一个默认值，但优先让 InitTranslator 处理初始化
			t.progressTracker.SetTotalChars(1000) // 确保数据跟踪器有值
			// 获取更新后的值
			totalCharsFromDataForUI, _, _, _, _, _, _ = t.progressTracker.GetProgress()
		}
		t.logger.Debug("[Old Progress System] Initializing UI progress bar", zap.Int64("totalCharsForUI", int64(totalCharsFromDataForUI)))

		// 初始化或配置自定义进度条
		if t.progressBar == nil {
			// 如果外部没有提供 progressBar，则创建一个新的
			initialTotal := int64(totalCharsFromDataForUI)
			if initialTotal <= 0 { // 再次检查以防万一
				initialTotal = 1000 // 默认总数
			}
			t.progressBar = customprogress.NewTracker(initialTotal,
				customprogress.WithMessage("翻译中..."),
				customprogress.WithUnit("字符", "chars"),
				customprogress.WithVisibility(true, true, true, true, false, false, true), // 禁用内置ETA和成本
			)
		} else {
			// 如果外部传入了 progressBar, 确保其 Total 是最新的
			t.progressBar.SetTotal(int64(totalCharsFromDataForUI))
		}
		t.progressBar.Start() // 启动自定义进度条的渲染
		t.logger.Info("翻译器已初始化 (使用自定义进度条系统)",
			zap.String("源语言", t.config.SourceLang),
			zap.String("目标语言", t.config.TargetLang),
			zap.String("活动步骤集", t.config.ActiveStepSet))
	}
}

// Translate 将文本从源语言翻译到目标语言
func (t *Impl) Translate(text string, retryFailedParts bool) (string, error) {
	t.logger.Debug("待翻译文本片段", zap.String("snippet", snippet(text)), zap.Int("长度", len(text)))
	// 如果启用了强制刷新缓存，先清除缓存
	if t.forceCacheRefresh {
		if err := t.refreshCache(); err != nil {
			t.logger.Warn("刷新缓存失败", zap.Error(err))
		}
	}

	// 获取活动的步骤集配置
	stepSet := t.config.StepSets[t.config.ActiveStepSet]

	// 检查是否需要重试失败的部分
	maxRetries := 3 // 默认最大重试次数

	// 如果配置中启用了重试失败部分，则强制开启
	if t.config.RetryFailedParts {
		retryFailedParts = true
	}

	// 检查步骤2和步骤3是否都设置为"none"
	skipReflectionAndImprovement := stepSet.Reflection.ModelName == "none" && stepSet.Improvement.ModelName == "none"

	if skipReflectionAndImprovement {
		t.logger.Info("步骤2和步骤3都设置为none，将只执行初始翻译")
	}

	// 如果文本较短且低于快速模式阈值，或者步骤2和步骤3都设置为"none"，跳过反思和改进步骤
	if len(text) < t.activeSteps.FastModeThreshold || skipReflectionAndImprovement {

		if len(text) < t.activeSteps.FastModeThreshold {
			t.logger.Info("文本较短，使用快速模式",
				zap.Int("文本长度", len(text)),
				zap.Int("阈值", t.activeSteps.FastModeThreshold),
			)
		}

		// 检查是否已缓存最终结果
		if t.shouldUseCache() {
			cacheKey := t.generateCacheKey(text, "final")
			if cached, ok := t.cache.Get(cacheKey); ok {
				t.logger.Info("从缓存加载最终翻译结果",
					zap.Int("文本长度", len(text)),
					zap.Int("缓存长度", len(cached)),
				)
				// 更新进度
				if t.useNewProgressBar {
					t.newProgressTracker.UpdateProgress(len(cached))
				} else {
					t.progressTracker.UpdateProgress(len(cached))
					t.updateProgress() // 更新旧UI进度条
				}
				return cached, nil
			}
		}

		result, err := t.initialTranslation(text)
		if err != nil && retryFailedParts {
			// 重试失败的部分
			t.logger.Warn("初始翻译失败，尝试重试",
				zap.Error(err),
			)

			for i := 0; i < maxRetries; i++ {
				t.logger.Info("重试初始翻译",
					zap.Int("尝试次数", i+1),
				)

				result, err = t.initialTranslation(text)
				if err == nil {
					break
				}
			}
		}

		if err != nil {
			return "", fmt.Errorf("快速模式翻译失败: %w", err)
		}

		// 快速模式下，初始翻译即为最终结果，更新进度
		if t.useNewProgressBar {
			t.newProgressTracker.UpdateProgress(len(result))
		} else {
			t.progressTracker.UpdateProgress(len(result))
			t.updateProgress() // 更新旧UI进度条
		}

		// 缓存结果
		if t.shouldUseCache() {
			cacheKey := t.generateCacheKey(text, "final")
			_ = t.cache.Set(cacheKey, result)
		}

		// 记录翻译结果的摘要
		t.logger.Info("翻译完成（快速模式）",
			zap.String("原文摘要", snippet(text)),
			zap.String("译文摘要", snippet(result)),
			zap.Int("原文长度", len(text)),
			zap.Int("译文长度", len(result)))

		t.addDebugRecord(text, result)

		return result, nil
	}

	// 检查是否已缓存最终结果
	if t.shouldUseCache() {
		cacheKey := t.generateCacheKey(text, "final")
		if cached, ok := t.cache.Get(cacheKey); ok {
			t.logger.Info("从缓存加载最终翻译结果",
				zap.Int("文本长度", len(text)),
				zap.Int("缓存长度", len(cached)),
			)
			return cached, nil
		}
	}

	// 第一步：初始翻译
	var initialTranslation string
	var err error

	// 检查是否已缓存初始翻译
	initialCacheKey := t.generateCacheKey(text, "initial")
	if t.shouldUseCache() {
		if cached, ok := t.cache.Get(initialCacheKey); ok {
			t.logger.Info("从缓存加载初始翻译",
				zap.Int("文本长度", len(text)),
				zap.Int("缓存长度", len(cached)),
			)
			initialTranslation = cached
			if t.useNewProgressBar {
				t.newProgressTracker.UpdateProgress(len(initialTranslation))
			} else {
				t.progressTracker.UpdateProgress(len(initialTranslation)) // Corrected: Use UpdateProgress for cumulative UI progress
				t.updateProgress()                                        // 更新UI
			}
		}
	}

	// 如果没有缓存，执行初始翻译
	if initialTranslation == "" {
		initialTranslation, err = t.initialTranslation(text)
		if err != nil {
			if retryFailedParts {
				// 重试失败的部分
				t.logger.Warn("初始翻译失败，尝试重试",
					zap.Error(err),
				)

				for i := 0; i < maxRetries; i++ {
					t.logger.Info("重试初始翻译",
						zap.Int("尝试次数", i+1),
					)

					initialTranslation, err = t.initialTranslation(text)
					if err == nil {
						break
					}
				}
			}

			if err != nil {
				return "", fmt.Errorf("初始翻译步骤失败: %w", err)
			}
		}

		if t.useNewProgressBar {
			t.newProgressTracker.UpdateProgress(len(initialTranslation))
		} else {
			t.progressTracker.UpdateProgress(len(initialTranslation)) // Corrected: Use UpdateProgress for cumulative UI progress
			t.updateProgress()                                        // 更新UI
		}

		// 缓存初始翻译结果
		if t.shouldUseCache() {
			_ = t.cache.Set(initialCacheKey, initialTranslation)
			_ = t.cache.Set(t.generateCacheKey(text, "final"), initialTranslation)
		}
	}

	// 检查步骤2（反思）的模型是否为"none"
	if stepSet.Reflection.ModelName == "none" {
		t.logger.Info("步骤2（反思）的模型设置为none，跳过反思和改进步骤")

		// 将初始翻译结果作为最终结果缓存
		if t.shouldUseCache() {
			_ = t.cache.Set(t.generateCacheKey(text, "final"), initialTranslation)
		}

		return initialTranslation, nil
	}

	// 第二步：反思
	var reflection string

	// 检查是否已缓存反思结果
	reflectionCacheKey := t.generateCacheKey(text+initialTranslation, "reflection")
	if t.shouldUseCache() {
		if cached, ok := t.cache.Get(reflectionCacheKey); ok {
			t.logger.Info("从缓存加载反思结果",
				zap.Int("文本长度", len(text)),
				zap.Int("缓存长度", len(cached)),
			)
			reflection = cached
			// 反思步骤不直接贡献到UI字符进度条，因为它不是翻译文本输出
		}
	}

	// 如果没有缓存，执行反思
	if reflection == "" {
		reflection, err = t.reflection(text, initialTranslation)
		if err != nil {
			if retryFailedParts {
				// 重试失败的部分
				t.logger.Warn("反思步骤失败，尝试重试",
					zap.Error(err),
				)

				for i := 0; i < maxRetries; i++ {
					t.logger.Info("重试反思步骤",
						zap.Int("尝试次数", i+1),
					)

					reflection, err = t.reflection(text, initialTranslation)
					if err == nil {
						break
					}
				}
			}

			if err != nil {
				return "", fmt.Errorf("反思步骤失败: %w", err)
			}
		}

		// 缓存反思结果
		if t.shouldUseCache() {
			_ = t.cache.Set(reflectionCacheKey, reflection)
		}
		// 反思步骤不更新UI字符进度条
	}

	// 第三步：改进
	var improvedTranslation string

	// 检查是否已缓存改进结果
	improvementCacheKey := t.generateCacheKey(text+initialTranslation+reflection, "improvement")
	if t.shouldUseCache() {
		if cached, ok := t.cache.Get(improvementCacheKey); ok {
			t.logger.Info("从缓存加载改进结果",
				zap.Int("文本长度", len(text)),
				zap.Int("缓存长度", len(cached)),
			)
			improvedTranslation = cached
			// 如果从缓存加载改进结果，需要用它的长度更新进度，
			// 但要注意，如果之前initialTranslation已经更新过，这里可能是覆盖或累加的逻辑问题。
			// 假设这里的cached是最终的翻译长度，并且前面的initialTranslation调用UpdateProgress设置的是中间过程。
			// 最简单的方式是，如果走了完整流程，在最后用 improvedTranslation 的长度来更新。
			// 但如果分步缓存，且目标是累加进度，则需要更细致处理。
			// 暂时：如果从缓存获取，我们用它的长度作为当前已翻译字符，并更新UI。
			if t.useNewProgressBar {
				t.newProgressTracker.UpdateProgress(len(improvedTranslation))
			} else {
				t.progressTracker.UpdateProgress(len(improvedTranslation)) // Corrected: Use UpdateProgress for cumulative UI progress
				t.updateProgress()
			}
		}
	}

	// 如果没有缓存，执行改进
	if improvedTranslation == "" {
		improvedTranslation, err = t.improvement(text, initialTranslation, reflection)
		if err != nil {
			if retryFailedParts {
				// 重试失败的部分
				t.logger.Warn("改进步骤失败，尝试重试",
					zap.Error(err),
				)

				for i := 0; i < maxRetries; i++ {
					t.logger.Info("重试改进步骤",
						zap.Int("尝试次数", i+1),
					)

					improvedTranslation, err = t.improvement(text, initialTranslation, reflection)
					if err == nil {
						break
					}
				}
			}

			if err != nil {
				return "", fmt.Errorf("改进步骤失败: %w", err)
			}
		}
		// 改进步骤完成后，用其结果的长度更新进度
		if t.useNewProgressBar {
			t.newProgressTracker.UpdateProgress(len(improvedTranslation))
		} else {
			t.progressTracker.UpdateProgress(len(improvedTranslation)) // Corrected: Use UpdateProgress for cumulative UI progress
			t.updateProgress()
		}

		// 缓存改进结果
		if t.shouldUseCache() {
			_ = t.cache.Set(improvementCacheKey, improvedTranslation)
			_ = t.cache.Set(t.generateCacheKey(text, "final"), improvedTranslation)
		}
	}

	// 记录翻译结果的摘要
	t.logger.Info("翻译完成（完整流程）",
		zap.String("原文摘要", snippet(text)),
		zap.String("译文摘要", snippet(improvedTranslation)),
		zap.Int("原文长度", len(text)),
		zap.Int("译文长度", len(improvedTranslation)))

	t.addDebugRecord(text, improvedTranslation)

	return improvedTranslation, nil
}

// initialTranslation 执行初始翻译
func (t *Impl) initialTranslation(text string) (string, error) {
	// 检查缓存
	if t.shouldUseCache() {
		cacheKey := t.generateCacheKey(text, "initial")
		if cached, ok := t.cache.Get(cacheKey); ok {
			t.logger.Debug("从缓存加载初始翻译结果")
			return cached, nil
		}
	}

	// 获取模型
	model := t.activeSteps.InitialModel

	// 如果模型为 nil（设置为 raw 或 none）或类型为 raw，直接返回原文
	if model == nil || model.Type() == "raw" {
		t.logger.Info("使用 raw/none 模型，直接返回原文")
		return text, nil
	}

	// 构建提示词
	prompt := fmt.Sprintf(`This is a translation task from %s to %s.
[INTERNAL INSTRUCTIONS: The following formatting rules are for internal reference only and must NOT appear in the final output.]

Formatting Rules:
1. Preserve all original formatting exactly:
	- Do not modify any Markdown syntax (**, *, #, etc.).
	- Do not translate any content within LaTeX formulas ($...$, $$...$$, \( ... \), \[ ... \]) or any LaTeX commands.
	- For LaTeX files, preserve all commands, environments (such as \begin{...} and \end{...}), and macros exactly as they are.
	- Keep all HTML tags intact.
2. Do not alter abbreviations, technical terms, or code identifiers.
3. Preserve document structure, including line breaks, paragraph spacing, lists, and tables.
4. IMPORTANT: Do not translate or modify any text matching the following pattern:
  @@PRESERVE_<number>@@...@@/PRESERVE_<number>@@
For example, if you see:
  @@PRESERVE_0@@[1] Author et al.@@/PRESERVE_0@@
or
  @@PRESERVE_1@@$E = mc^2$@@/PRESERVE_1@@
you must leave these parts exactly as they are.
5. IMPORTANT: Preserve all paragraph breaks exactly as they are. Do not convert double newlines ("\n\n") into single newlines ("\n").

[END OF INTERNAL INSTRUCTIONS]

Please provide only the translation of the text below, strictly adhering to the above formatting rules.

<TEXT TO TRANSLATE>
%s
</TEXT TO TRANSLATE>
	`,
		t.config.SourceLang, t.config.TargetLang, text)

	// 调用语言模型
	temperature := t.config.StepSets[t.config.ActiveStepSet].InitialTranslation.Temperature

	t.logger.Debug("执行初始翻译",
		zap.String("模型", model.Name()),
		zap.Float64("温度", temperature),
		zap.Int("最大输出令牌", model.MaxOutputTokens()),
	)

	result, inputTokens, outputTokens, err := model.Complete(prompt, model.MaxOutputTokens(), temperature)
	if err != nil {
		return "", fmt.Errorf("模型调用失败: %w", err)
	}

	t.logger.Debug("初始翻译完成",
		zap.Int("输入令牌数", inputTokens),
		zap.Int("输出令牌数", outputTokens),
	)

	if t.useNewProgressBar && t.newProgressTracker != nil {
		go t.newProgressTracker.UpdateInitialTokenUsage(inputTokens, outputTokens)
	} else {
		go t.progressTracker.UpdateInitialTokenUsage(inputTokens, outputTokens)
	}

	result = t.RemoveUsedTags(result)

	// 缓存结果
	if t.shouldUseCache() {
		cacheKey := t.generateCacheKey(text, "initial")
		_ = t.cache.Set(cacheKey, result)
	}

	return result, nil
}

// reflection 执行反思
func (t *Impl) reflection(sourceText, translation string) (string, error) {
	// 如果反思模型为nil（设置为"none"），则跳过反思步骤
	if t.activeSteps.ReflectionModel == nil {
		t.logger.Info("反思模型设置为none，跳过反思步骤")
		return "", nil
	}

	// 如果反思模型类型为 raw，直接返回空字符串
	if t.activeSteps.ReflectionModel.Type() == "raw" {
		t.logger.Info("反思模型设置为raw，跳过反思步骤")
		return "", nil
	}

	// 检查缓存
	if t.shouldUseCache() {
		cacheKey := t.generateCacheKey(sourceText+translation, "reflection")
		if cached, ok := t.cache.Get(cacheKey); ok {
			t.logger.Debug("从缓存加载反思结果")
			return cached, nil
		}
	}

	// 构建提示词
	prompt := fmt.Sprintf(`
Your task is to review a source text and its translation from %s to %s, and then provide a list of constructive and specific suggestions to improve the translation.
The final style and tone should match the style of %s colloquially spoken in %s.

[INTERNAL INSTRUCTIONS: The following guidelines are for internal use only and must NOT appear in the final output.]

Formatting and Review Guidelines:
1. Preserve all original formatting exactly:
	- Do not modify Markdown syntax (**, *, #, etc.).
	- Do not alter any LaTeX formulas ($...$, $$...$$, \( ... \), \[ ... \]) or any LaTeX commands/environments.
	- Do not change HTML tags (<...>).
2. Maintain all abbreviations, technical terms, and code identifiers exactly as they appear.
3. Preserve the document structure, including line breaks, paragraph spacing, table formatting, and list markers.
4. IMPORTANT: Do not translate or modify any text matching the following pattern:
  @@PRESERVE_<number>@@...@@/PRESERVE_<number>@@
For example, if you see:
  @@PRESERVE_0@@[1] Author et al.@@/PRESERVE_0@@
or
  @@PRESERVE_1@@$E = mc^2$@@/PRESERVE_1@@
you must leave these parts exactly as they are.
5. IMPORTANT: Preserve all paragraph breaks exactly as they are. Do not convert double newlines ("\n\n") into single newlines ("\n").

Review Criteria:
(i) Accuracy: Identify and correct any issues such as additions, mistranslations, omissions, or untranslated segments.
(ii) Fluency: Ensure the translation follows %s grammar, spelling, and punctuation rules, avoiding unnecessary repetitions.
(iii) Style: Verify that the translation reflects the source text's style and cultural context.
(iv) Terminology: Ensure consistency in technical terms and that equivalent idioms in %s are properly used.
(v) Formatting: Confirm that the translation maintains the original formatting, including Markdown, LaTeX, and HTML.

[END OF INTERNAL INSTRUCTIONS]

The source text and the initial translation are delimited by the following XML tags:

<SOURCE_TEXT>
%s
</SOURCE_TEXT>

<TRANSLATION>
%s
</TRANSLATION>

Output only a list of constructive suggestions, each addressing a specific aspect of the translation, and nothing else.`,
		t.config.SourceLang, t.config.TargetLang,
		t.config.TargetLang, t.config.Country,
		t.config.TargetLang,
		t.config.TargetLang,
		sourceText,
		translation)

	// 调用语言模型
	model := t.activeSteps.ReflectionModel
	temperature := t.config.StepSets[t.config.ActiveStepSet].Reflection.Temperature

	t.logger.Debug("执行反思",
		zap.String("模型", model.Name()),
		zap.Float64("温度", temperature),
	)

	result, inputTokens, outputTokens, err := model.Complete(prompt, model.MaxOutputTokens(), temperature)
	if err != nil {
		return "", fmt.Errorf("模型调用失败: %w", err)
	}

	t.logger.Debug("反思完成",
		zap.Int("输入令牌数", inputTokens),
		zap.Int("输出令牌数", outputTokens),
	)

	if t.useNewProgressBar && t.newProgressTracker != nil {
		go t.newProgressTracker.UpdateReflectionTokenUsage(inputTokens, outputTokens)
	} else {
		go t.progressTracker.UpdateReflectionTokenUsage(inputTokens, outputTokens)
	}

	result = t.RemoveUsedTags(result)

	// 缓存结果
	if t.shouldUseCache() {
		cacheKey := t.generateCacheKey(sourceText+translation, "reflection")
		_ = t.cache.Set(cacheKey, result)
	}

	return result, nil
}

// improvement 执行改进
func (t *Impl) improvement(sourceText, translation, reflection string) (string, error) {
	// 如果改进模型为nil（设置为"none"），则跳过改进步骤
	if t.activeSteps.ImprovementModel == nil {
		t.logger.Info("改进模型设置为none，跳过改进步骤")
		return translation, nil
	}

	// 如果改进模型类型为 raw，直接返回原翻译
	if t.activeSteps.ImprovementModel.Type() == "raw" {
		t.logger.Info("改进模型设置为raw，跳过改进步骤")
		return translation, nil
	}

	// 检查缓存
	if t.shouldUseCache() {
		cacheKey := t.generateCacheKey(sourceText+translation+reflection, "improvement")
		if cached, ok := t.cache.Get(cacheKey); ok {
			t.logger.Debug("从缓存加载改进结果")
			return cached, nil
		}
	}

	// 构建提示词
	prompt := fmt.Sprintf(`
Your task is to carefully read, then edit, a translation from %s to %s, taking into account a list of expert suggestions and constructive criticisms.

[INTERNAL INSTRUCTIONS: The following guidelines are for internal use only and must NOT appear in the final output.]

Critical Formatting Requirements:
1. The following elements MUST remain exactly as in the source:
   - All Markdown formatting (**, *, #, etc.)
   - All LaTeX formulas ($...$, $$...$$, \( ... \), \[ ... \])
   - All HTML tags (<...>)
2. Preserve all technical elements:
   - Keep unknown abbreviations in original form
   - Maintain all code identifiers and variables
   - Preserve all URLs and file paths
3. Maintain document structure:
   - Keep all line breaks and spacing
   - Preserve table formatting
   - Keep list markers and numbering
4. IMPORTANT: Do not translate or modify any text matching the following pattern:
  @@PRESERVE_<number>@@...@@/PRESERVE_<number>@@
For example, if you see:
  @@PRESERVE_0@@[1] Author et al.@@/PRESERVE_0@@
or
  @@PRESERVE_1@@$E = mc^2$@@/PRESERVE_1@@
you must leave these parts exactly as they are.
5. IMPORTANT: Preserve all paragraph breaks exactly as they are. Do not convert double newlines ("\n\n") into single newlines ("\n").

Editing Instructions:
Please incorporate the following aspects when editing:
(i) Accuracy: Correct any errors of addition, mistranslation, omission, or untranslated text.
(ii) Fluency: Apply %s grammar, spelling, and punctuation rules and remove any unnecessary repetitions.
(iii) Style: Ensure the translation reflects the style of the source text.
(iv) Terminology: Address any inappropriate or inconsistent terminology.
(v) Formatting: Ensure the translation preserves the original formatting, including Markdown, LaTeX, and HTML.
(vi) Other errors as applicable.

[END OF INTERNAL INSTRUCTIONS]

The source text, the initial translation, and the expert suggestions are provided below:

<SOURCE_TEXT>
%s
</SOURCE_TEXT>

<TRANSLATION>
%s
</TRANSLATION>

<EXPERT_SUGGESTIONS>
%s
</EXPERT_SUGGESTIONS>

Output only the new, edited translation and nothing else.`,
		t.config.SourceLang, t.config.TargetLang,
		t.config.TargetLang,
		sourceText,
		translation,
		reflection)

	// 调用语言模型
	model := t.activeSteps.ImprovementModel
	temperature := t.config.StepSets[t.config.ActiveStepSet].Improvement.Temperature

	t.logger.Debug("执行改进",
		zap.String("模型", model.Name()),
		zap.Float64("温度", temperature),
	)

	result, inputTokens, outputTokens, err := model.Complete(prompt, model.MaxOutputTokens(), temperature)
	if err != nil {
		return "", fmt.Errorf("模型调用失败: %w", err)
	}

	t.logger.Debug("改进完成",
		zap.Int("输入令牌数", inputTokens),
		zap.Int("输出令牌数", outputTokens),
	)

	result = t.RemoveUsedTags(result)

	if t.useNewProgressBar && t.newProgressTracker != nil {
		go t.newProgressTracker.UpdateImprovementTokenUsage(inputTokens, outputTokens)
	} else {
		go t.progressTracker.UpdateImprovementTokenUsage(inputTokens, outputTokens)
	}

	// 缓存结果
	if t.shouldUseCache() {
		cacheKey := t.generateCacheKey(sourceText+translation+reflection, "improvement")
		_ = t.cache.Set(cacheKey, result)
	}

	return result, nil
}

// generateCacheKey 生成缓存键
func (t *Impl) generateCacheKey(text, step string) string {
	key := fmt.Sprintf("%s:%s:%s:%s:%s",
		t.config.SourceLang,
		t.config.TargetLang,
		t.config.ActiveStepSet,
		step,
		text)

	hash := md5.Sum([]byte(key))
	return hex.EncodeToString(hash[:])
}

// GetConfig 返回翻译器配置
func (t *Impl) GetConfig() *config.Config {
	return t.config
}

// GetProgress 返回当前翻译进度
func (t *Impl) GetProgress() string {
	return ""
}

// updateProgress 更新翻译进度
// 这个方法现在不接收参数，它会从 progressTracker 获取最新的累计字符数来更新UI
func (t *Impl) updateProgress() {
	if t.useNewProgressBar {
		// 使用新的进度条系统
		if t.newProgressTracker != nil {
			// 假设 newProgressTracker 内部有自己的逻辑从数据源获取并更新
			// 或者它也依赖一个共享的 TranslationProgressTracker 实例
			// 为简化，我们假设它在被调用时会做正确的事，或者这个分支暂时不关键
			// 如果 newProgressTracker 也依赖 t.progressTracker, 那么它会自动获得更新
			// _, cumulativeChars, _, _, _, _, _ := t.progressTracker.GetProgress()
			// t.newProgressTracker.UpdateProgress(cumulativeChars) // 这种方式可能不适用，取决于 newProgressTracker 设计
		}
		return
	}

	// ----- 使用旧的进度条系统 (现在是自定义进度条) -----
	if t.progressBar == nil {
		t.logger.Warn("自定义进度条未初始化，无法更新进度")
		return
	}

	// 从数据跟踪器获取累计值
	var totalCharsForUICurr, cumulativeIntermediateCharsCurr int
	// 注意：GetProgress 返回的第二个值是 translatedChars，即我们这里关心的 cumulativeIntermediateCharsCurr
	totalCharsForUICurr, cumulativeIntermediateCharsCurr, _, _, _, _, _ = t.progressTracker.GetProgress()

	t.logger.Debug("Updating UI progress bar (old system)",
		zap.Int("totalCharsForUI", totalCharsForUICurr),
		zap.Int("cumulativeIntermediateCharsForUI", cumulativeIntermediateCharsCurr),
	)

	if totalCharsForUICurr > 0 { // 确保总数有效
		t.progressBar.SetTotal(int64(totalCharsForUICurr))
	}
	t.progressBar.Update(int64(cumulativeIntermediateCharsCurr))

}

// GetProgressTracker 返回数据进度跟踪器
func (t *Impl) GetProgressTracker() *TranslationProgressTracker {
	return t.progressTracker
}

// Finish 方法负责结束翻译过程并打印总结信息
func (t *Impl) Finish() {
	var summaryData *customprogress.SummaryStats

	// 声明 Finish 函数作用域内的变量，用于存储从 GetProgress 获取的数据
	var totalCharsForUI, cumulativeIntermediateChars, realTotalCharsFromFile, realTranslatedCharsFinal int
	var tokenUsage TokenUsage
	var estimatedCost EstimatedCost
	var elapsedTime time.Duration
	// estimatedTimeRemainingFromTracker 暂时不直接用于旧版摘要，但我们接收它以匹配签名
	var estimatedTimeRemainingFromTracker float64

	if t.useNewProgressBar && t.newProgressTracker != nil {
		// 从新的进度跟踪器获取数据
		var newEstimatedTimeRemaining float64
		totalCharsForUI, cumulativeIntermediateChars, realTotalCharsFromFile, newEstimatedTimeRemaining, tokenUsage, estimatedCost = t.newProgressTracker.GetProgress()

		// 获取来自旧跟踪器的实际翻译字符数
		_, _, _, realTranslatedCharsFinal, _, _, _ = t.progressTracker.GetProgress()

		// 获取运行时间
		elapsedTime = tokenUsage.ElapsedTime

		t.logger.Debug("[New Progress System] Data for summary in Finish()",
			zap.Int("totalCharsForUI", totalCharsForUI),
			zap.Int("cumulativeIntermediateChars", cumulativeIntermediateChars),
			zap.Int("realTotalCharsFromFile", realTotalCharsFromFile),
			zap.Int("realTranslatedCharsFinal", realTranslatedCharsFinal),
			zap.Float64("estimatedTimeRemainingFromTracker", newEstimatedTimeRemaining),
			zap.Any("tokenUsage", tokenUsage),
			zap.Any("estimatedCost", estimatedCost),
			zap.Duration("elapsedTime", elapsedTime),
		)
	} else if !t.useNewProgressBar && t.progressTracker != nil {
		totalCharsForUI, cumulativeIntermediateChars, realTotalCharsFromFile, realTranslatedCharsFinal, estimatedTimeRemainingFromTracker, tokenUsage, estimatedCost = t.progressTracker.GetProgress()
		if t.progressTracker.startTime.IsZero() { // 旧的 progressTracker 有 startTime 字段
			elapsedTime = 0
		} else {
			elapsedTime = time.Since(t.progressTracker.startTime)
		}
		t.logger.Debug("[Old Progress System] Data for summary in Finish()",
			zap.Int("totalCharsForUI", totalCharsForUI),
			zap.Int("cumulativeIntermediateChars", cumulativeIntermediateChars),
			zap.Int("realTotalCharsFromFile", realTotalCharsFromFile),
			zap.Int("realTranslatedCharsFinal", realTranslatedCharsFinal),
			zap.Float64("estimatedTimeRemainingFromTracker", estimatedTimeRemainingFromTracker),
			zap.Any("tokenUsage", tokenUsage),
			zap.Any("estimatedCost", estimatedCost),
			zap.Duration("elapsedTime", elapsedTime),
		)
	} else {
		if t.progressBar != nil {
			t.progressBar.Done(nil)
		} else if t.newProgressTracker != nil {
			t.newProgressTracker.Done(nil)
		}
		t.logger.Warn("Finish called but no active progress tracker or UI progress bar found to generate summary.")
		return
	}

	// 使用从上面分支获取的数据（realTotalCharsFromFile, realTranslatedCharsFinal, tokenUsage, estimatedCost, elapsedTime）
	if realTotalCharsFromFile > 0 || realTranslatedCharsFinal > 0 || tokenUsage.InitialInputTokens > 0 || tokenUsage.InitialOutputTokens > 0 {
		summaryData = &customprogress.SummaryStats{
			InputTextLength: realTotalCharsFromFile,   // 使用从 GetProgress 获取的原始文件总字符数
			TextTranslated:  realTranslatedCharsFinal, // 使用最终输出文件的字符数
			TotalTime:       elapsedTime,
			Steps:           []customprogress.StepStats{},
			TotalCost:       estimatedCost.TotalCost,
			TotalCostUnit:   estimatedCost.TotalCostUnit,
		}

		if tokenUsage.InitialInputTokens > 0 || tokenUsage.InitialOutputTokens > 0 || estimatedCost.InitialTotalCost > 0 {
			summaryData.Steps = append(summaryData.Steps, customprogress.StepStats{
				StepName:     "Initial Translation",
				InputTokens:  tokenUsage.InitialInputTokens,
				OutputTokens: tokenUsage.InitialOutputTokens,
				TokenSpeed:   tokenUsage.InitialTokenSpeed,
				Cost:         estimatedCost.InitialTotalCost,
				CostUnit:     estimatedCost.InitialCostUnit,
				HasData:      true,
			})
		}

		if tokenUsage.ReflectionInputTokens > 0 || tokenUsage.ReflectionOutputTokens > 0 || estimatedCost.ReflectionTotalCost > 0 {
			summaryData.Steps = append(summaryData.Steps, customprogress.StepStats{
				StepName:     "Reflection",
				InputTokens:  tokenUsage.ReflectionInputTokens,
				OutputTokens: tokenUsage.ReflectionOutputTokens,
				TokenSpeed:   tokenUsage.ReflectionTokenSpeed,
				Cost:         estimatedCost.ReflectionTotalCost,
				CostUnit:     estimatedCost.ReflectionCostUnit,
				HasData:      true,
			})
		}

		if tokenUsage.ImprovementInputTokens > 0 || tokenUsage.ImprovementOutputTokens > 0 || estimatedCost.ImprovementTotalCost > 0 {
			summaryData.Steps = append(summaryData.Steps, customprogress.StepStats{
				StepName:     "Improvement",
				InputTokens:  tokenUsage.ImprovementInputTokens,
				OutputTokens: tokenUsage.ImprovementOutputTokens,
				TokenSpeed:   tokenUsage.ImprovementTokenSpeed,
				Cost:         estimatedCost.ImprovementTotalCost,
				CostUnit:     estimatedCost.ImprovementCostUnit,
				HasData:      true,
			})
		}

	}

	t.finalize(summaryData)
}

func maskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return "****"
	}
	return key[:2] + "****" + key[len(key)-2:]
}

func (t *Impl) addDebugRecord(src, res string) {
	if !t.config.SaveDebugInfo {
		return
	}
	t.debugRecords = append(t.debugRecords, TranslationRecord{Source: src, Result: res})
}

// SaveDebugInfo 将调试信息保存到输出文件对应的 JSON 中
func (t *Impl) SaveDebugInfo(outputPath string) error {
	if !t.config.SaveDebugInfo {
		return nil
	}

	stepCfg := t.config.StepSets[t.config.ActiveStepSet]
	stepModels := StepModelInfo{
		Initial:     stepCfg.InitialTranslation.ModelName,
		Reflection:  stepCfg.Reflection.ModelName,
		Improvement: stepCfg.Improvement.ModelName,
	}

	cfg := struct {
		SourceLang      string `json:"source_lang"`
		TargetLang      string `json:"target_lang"`
		ActiveStepSet   string `json:"active_step_set"`
		Concurrency     int    `json:"concurrency"`
		HTMLConcurrency int    `json:"html_concurrency"`
		EPUBConcurrency int    `json:"epub_concurrency"`
		RequestTimeout  int    `json:"request_timeout"`
	}{
		SourceLang:      t.config.SourceLang,
		TargetLang:      t.config.TargetLang,
		ActiveStepSet:   t.config.ActiveStepSet,
		Concurrency:     t.config.Concurrency,
		HTMLConcurrency: t.config.HtmlConcurrency,
		EPUBConcurrency: t.config.EpubConcurrency,
		RequestTimeout:  t.config.RequestTimeout,
	}

	// 遍历模型配置并屏蔽 key
	models := make(map[string]config.ModelConfig)
	for name, mc := range t.config.ModelConfigs {
		mc.Key = maskKey(mc.Key)
		models[name] = mc
	}

	info := struct {
		Config       interface{}                   `json:"config"`
		StepModels   StepModelInfo                 `json:"step_models"`
		Models       map[string]config.ModelConfig `json:"models"`
		Translations []TranslationRecord           `json:"translations"`
	}{
		Config:       cfg,
		StepModels:   stepModels,
		Models:       models,
		Translations: t.debugRecords,
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	debugPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".json"
	return os.WriteFile(debugPath, data, 0644)
}

func (t *Impl) finalize(summaryData *customprogress.SummaryStats) {
	if t.useNewProgressBar && t.newProgressTracker != nil {
		t.newProgressTracker.Done(summaryData)
	} else if !t.useNewProgressBar && t.progressBar != nil {
		t.progressBar.Done(summaryData)
	}
}
