package translator

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/schollz/progressbar/v3"
	"go.uber.org/zap"
)

// TranslatorImpl 是 Translator 接口的实现
type TranslatorImpl struct {
	config            *config.Config
	models            map[string]LLMClient
	logger            logger.Logger
	activeSteps       *StepSetConfig
	cache             Cache
	forceCacheRefresh bool // 强制刷新缓存
	progressBar       *progressbar.ProgressBar

	// 进度跟踪
	progressMu     sync.RWMutex
	currentText    string    // 当前正在翻译的文本
	partialResult  string    // 部分翻译结果
	startTime      time.Time // 开始时间
	lastUpdateTime time.Time // 最后更新时间
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

// New 创建一个新的翻译器实例
func New(cfg *config.Config, options ...Option) (*TranslatorImpl, error) {
	// 创建日志记录器
	log := logger.NewZapLogger(cfg.Debug)

	// 应用选项
	opts := &translatorOptions{
		cache:             newFileCache(cfg.CacheDir),
		forceCacheRefresh: false,
	}

	for _, option := range options {
		option(opts)
	}

	// 记录配置信息
	log.Info("初始化翻译器",
		zap.String("源语言", cfg.SourceLang),
		zap.String("目标语言", cfg.TargetLang),
		zap.String("国家/地区", cfg.Country),
		zap.String("默认模型", cfg.DefaultModelName),
		zap.String("活动步骤集", cfg.ActiveStepSet),
	)

	// 初始化所有模型
	models, err := initModels(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("初始化模型失败: %w", err)
	}

	// 获取步骤集配置
	stepSet, ok := cfg.StepSets[cfg.ActiveStepSet]
	if !ok {
		return nil, fmt.Errorf("未找到步骤集: %s", cfg.ActiveStepSet)
	}

	// 获取步骤模型
	initialModel, ok := models[stepSet.InitialTranslation.ModelName]
	if !ok && stepSet.InitialTranslation.ModelName != "none" && stepSet.InitialTranslation.ModelName != "raw" {
		return nil, fmt.Errorf("未找到模型: %s", stepSet.InitialTranslation.ModelName)
	}

	// 处理反思模型
	var reflectionModel LLMClient
	if stepSet.Reflection.ModelName != "none" && stepSet.Reflection.ModelName != "raw" {
		var ok bool
		reflectionModel, ok = models[stepSet.Reflection.ModelName]
		if !ok {
			return nil, fmt.Errorf("未找到模型: %s", stepSet.Reflection.ModelName)
		}
	} else {
		log.Info("反思步骤的模型设置为none或raw，将跳过此步骤")
	}

	// 处理改进模型
	var improvementModel LLMClient
	if stepSet.Improvement.ModelName != "none" && stepSet.Improvement.ModelName != "raw" {
		var ok bool
		improvementModel, ok = models[stepSet.Improvement.ModelName]
		if !ok {
			return nil, fmt.Errorf("未找到模型: %s", stepSet.Improvement.ModelName)
		}
	} else {
		log.Info("改进步骤的模型设置为none或raw，将跳过此步骤")
	}

	// 创建步骤集配置
	activeSteps := &StepSetConfig{
		ID:                stepSet.ID,
		Name:              stepSet.Name,
		Description:       stepSet.Description,
		InitialModel:      initialModel,
		ReflectionModel:   reflectionModel,
		ImprovementModel:  improvementModel,
		FastModeThreshold: stepSet.FastModeThreshold,
	}

	return &TranslatorImpl{
		config:            cfg,
		models:            models,
		logger:            log,
		activeSteps:       activeSteps,
		cache:             opts.cache,
		forceCacheRefresh: opts.forceCacheRefresh,
		progressBar:       opts.progressBar,
	}, nil
}

func (t *TranslatorImpl) RemoveUsedTags(text string) string {
	result := text
	result = strings.ReplaceAll(result, "<SOURCE_TEXT>", "")
	result = strings.ReplaceAll(result, "</SOURCE_TEXT>", "")
	result = strings.ReplaceAll(result, "<TRANSLATION>", "")
	result = strings.ReplaceAll(result, "</TRANSLATION>", "")
	result = strings.ReplaceAll(result, "<EXPERT_SUGGESTIONS>", "")
	result = strings.ReplaceAll(result, "</EXPERT_SUGGESTIONS>", "")
	result = strings.ReplaceAll(result, "<TEXT TO EDIT>", "")
	result = strings.ReplaceAll(result, "</TEXT TO EDIT>", "")
	result = strings.ReplaceAll(result, "<TEXT TO TRANSLATE>", "")
	result = strings.ReplaceAll(result, "</TEXT TO TRANSLATE>", "")
	return result
}

// GetLogger 返回日志记录器
func (t *TranslatorImpl) GetLogger() logger.Logger {
	return t.logger
}

// shouldUseCache 判断是否应该使用缓存
func (t *TranslatorImpl) shouldUseCache() bool {
	return t.config.UseCache
}

// refreshCache 刷新缓存
func (t *TranslatorImpl) refreshCache() error {
	if t.forceCacheRefresh {
		t.logger.Info("正在刷新缓存")
		return t.cache.Clear()
	}
	return nil
}

// Translate 将文本从源语言翻译到目标语言
func (t *TranslatorImpl) Translate(text string, retryFailedParts bool) (string, error) {
	// 如果启用了强制刷新缓存，先清除缓存
	if t.forceCacheRefresh {
		if err := t.refreshCache(); err != nil {
			t.logger.Warn("刷新缓存失败", zap.Error(err))
		}
	}

	// 获取活动的步骤集配置
	stepSet := t.config.StepSets[t.config.ActiveStepSet]

	// 开始跟踪进度
	t.startProgress(text)
	defer t.endProgress()

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

		// 缓存结果
		if t.shouldUseCache() {
			cacheKey := t.generateCacheKey(text, "final")
			t.cache.Set(cacheKey, result)
		}

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

		// 缓存初始翻译结果
		if t.shouldUseCache() {
			t.cache.Set(initialCacheKey, initialTranslation)
		}
	}

	// 更新进度
	t.updateProgress(initialTranslation)

	// 检查步骤2（反思）的模型是否为"none"
	if stepSet.Reflection.ModelName == "none" {
		t.logger.Info("步骤2（反思）的模型设置为none，跳过反思和改进步骤")

		// 将初始翻译结果作为最终结果缓存
		if t.shouldUseCache() {
			t.cache.Set(t.generateCacheKey(text, "final"), initialTranslation)
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
			t.cache.Set(reflectionCacheKey, reflection)
		}
	}

	// 更新进度
	t.updateProgress(initialTranslation)

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

		// 缓存改进结果
		if t.shouldUseCache() {
			t.cache.Set(improvementCacheKey, improvedTranslation)
			t.cache.Set(t.generateCacheKey(text, "final"), improvedTranslation)
		}
	}

	return improvedTranslation, nil
}

// initialTranslation 执行初始翻译
func (t *TranslatorImpl) initialTranslation(text string) (string, error) {
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
	)

	result, inputTokens, outputTokens, err := model.Complete(prompt, model.MaxOutputTokens(), temperature)
	if err != nil {
		return "", fmt.Errorf("模型调用失败: %w", err)
	}

	t.logger.Debug("初始翻译完成",
		zap.Int("输入令牌数", inputTokens),
		zap.Int("输出令牌数", outputTokens),
	)

	result = t.RemoveUsedTags(result)

	// 缓存结果
	if t.shouldUseCache() {
		cacheKey := t.generateCacheKey(text, "initial")
		if err := t.cache.Set(cacheKey, result); err != nil {
			t.logger.Warn("缓存初始翻译结果失败", zap.Error(err))
		}
	}

	return result, nil
}

// reflection 执行反思
func (t *TranslatorImpl) reflection(sourceText, translation string) (string, error) {
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

	result = t.RemoveUsedTags(result)

	// 缓存结果
	if t.shouldUseCache() {
		cacheKey := t.generateCacheKey(sourceText+translation, "reflection")
		if err := t.cache.Set(cacheKey, result); err != nil {
			t.logger.Warn("缓存反思结果失败", zap.Error(err))
		}
	}

	return result, nil
}

// improvement 执行改进
func (t *TranslatorImpl) improvement(sourceText, translation, reflection string) (string, error) {
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

	// 缓存结果
	if t.shouldUseCache() {
		cacheKey := t.generateCacheKey(sourceText+translation+reflection, "improvement")
		if err := t.cache.Set(cacheKey, result); err != nil {
			t.logger.Warn("缓存改进结果失败", zap.Error(err))
		}
	}

	return result, nil
}

// generateCacheKey 生成缓存键
func (t *TranslatorImpl) generateCacheKey(text, step string) string {
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
func (t *TranslatorImpl) GetConfig() *config.Config {
	return t.config
}

// GetProgress 返回当前翻译进度
func (t *TranslatorImpl) GetProgress() string {
	t.progressMu.RLock()
	defer t.progressMu.RUnlock()

	return t.partialResult
}

// updateProgress 更新翻译进度
func (t *TranslatorImpl) updateProgress(text string) {
	t.progressMu.Lock()
	defer t.progressMu.Unlock()

	t.partialResult = text
	t.lastUpdateTime = time.Now()

	// 更新进度条
	if t.progressBar != nil {
		_ = t.progressBar.Add(len(text))
	}
}

// startProgress 开始跟踪翻译进度
func (t *TranslatorImpl) startProgress(text string) {
	t.progressMu.Lock()
	defer t.progressMu.Unlock()

	t.currentText = text
	t.partialResult = ""
	t.startTime = time.Now()
	t.lastUpdateTime = time.Now()

	// 重置进度条
	if t.progressBar != nil {
		t.progressBar.Reset()
	}
}

// endProgress 结束翻译进度跟踪
func (t *TranslatorImpl) endProgress() {
	t.progressMu.Lock()
	defer t.progressMu.Unlock()

	t.currentText = ""
	t.partialResult = ""

	// 完成进度条
	if t.progressBar != nil {
		_ = t.progressBar.Finish()
	}
}
