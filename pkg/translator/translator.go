package translator

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
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
	log.Debug("初始化翻译器",
		zap.String("源语言", cfg.SourceLang),
		zap.String("目标语言", cfg.TargetLang),
		zap.String("国家/地区", cfg.Country),
		zap.String("活动步骤集", cfg.ActiveStepSet),
		zap.Int("最大分块令牌数", cfg.MaxTokensPerChunk),
		zap.Bool("使用缓存", cfg.UseCache),
		zap.Int("请求超时(秒)", cfg.RequestTimeout),
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
	if !ok {
		return nil, fmt.Errorf("未找到模型: %s", stepSet.InitialTranslation.ModelName)
	}

	reflectionModel, ok := models[stepSet.Reflection.ModelName]
	if !ok {
		return nil, fmt.Errorf("未找到模型: %s", stepSet.Reflection.ModelName)
	}

	improvementModel, ok := models[stepSet.Improvement.ModelName]
	if !ok {
		return nil, fmt.Errorf("未找到模型: %s", stepSet.Improvement.ModelName)
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

// GetLogger 返回日志记录器
func (t *TranslatorImpl) GetLogger() logger.Logger {
	return t.logger
}

// Translate 将文本从源语言翻译到目标语言
func (t *TranslatorImpl) Translate(text string) (string, error) {
	// 开始跟踪进度
	t.startProgress(text)
	defer t.endProgress()

	// 检查是否需要重试失败的部分
	retryFailedParts := false
	maxRetries := 3 // 默认最大重试次数

	if t.config.RetryFailedParts {
		retryFailedParts = true
	}

	// 如果文本较短且低于快速模式阈值，跳过反思和改进步骤
	if len(text) < t.activeSteps.FastModeThreshold {
		t.logger.Info("文本较短，使用快速模式",
			zap.Int("文本长度", len(text)),
			zap.Int("阈值", t.activeSteps.FastModeThreshold),
		)

		// 检查是否已缓存最终结果
		if t.config.UseCache && !t.forceCacheRefresh {
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
		if t.config.UseCache {
			cacheKey := t.generateCacheKey(text, "final")
			t.cache.Set(cacheKey, result)
		}

		return result, nil
	}

	// 检查是否已缓存最终结果
	if t.config.UseCache && !t.forceCacheRefresh {
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
	if t.config.UseCache && !t.forceCacheRefresh {
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
		if t.config.UseCache {
			t.cache.Set(initialCacheKey, initialTranslation)
		}
	}

	// 更新进度
	t.updateProgress(initialTranslation)

	// 第二步：反思
	var reflection string

	// 检查是否已缓存反思结果
	reflectionCacheKey := t.generateCacheKey(text+initialTranslation, "reflection")
	if t.config.UseCache && !t.forceCacheRefresh {
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
		if t.config.UseCache {
			t.cache.Set(reflectionCacheKey, reflection)
		}
	}

	// 更新进度
	t.updateProgress(initialTranslation)

	// 第三步：改进
	var improvedTranslation string

	// 检查是否已缓存改进结果
	improvementCacheKey := t.generateCacheKey(text+initialTranslation+reflection, "improvement")
	if t.config.UseCache && !t.forceCacheRefresh {
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
		if t.config.UseCache {
			t.cache.Set(improvementCacheKey, improvedTranslation)
			t.cache.Set(t.generateCacheKey(text, "final"), improvedTranslation)
		}
	}

	return improvedTranslation, nil
}

// initialTranslation 执行初始翻译
func (t *TranslatorImpl) initialTranslation(text string) (string, error) {
	// 检查缓存
	if t.config.UseCache {
		cacheKey := t.generateCacheKey(text, "initial")
		if cached, ok := t.cache.Get(cacheKey); ok {
			t.logger.Debug("从缓存加载初始翻译结果")
			return cached, nil
		}
	}

	// 构建提示词
	prompt := fmt.Sprintf(`This is an %s to %s translation. The following are the formatting requirements for the translation:
IMPORTANT FORMATTING REQUIREMENTS:
1. All original formatting must be preserved exactly:
   - Keep all Markdown syntax (**, *, #, etc.) exactly as is
   - DO NOT TRANSLATE ANY CONTENT INSIDE LaTeX formulas ($...$, $$...$$, \(...\), \[...\]) - leave them completely unchanged
   - Keep all HTML tags (<...>) unchanged
2. Keep all abbreviations and technical terms in their original form:
   - Do not translate unknown abbreviations (e.g., FPGA, CPU, NDT)
   - Keep all code identifiers and variables unchanged
3. Maintain document structure:
   - Keep all line breaks and paragraph spacing
   - Preserve table formatting and alignment
   - Keep list markers (numbers, bullets) unchanged
4. DO NOT MODIFY OR TRANSLATE any text between ⚡⚡⚡ symbols (e.g., ⚡⚡⚡UNTRANSLATABLE_0⚡⚡⚡).
   These are special markers that must remain exactly as they are.

DO NOT TRANSLATE THE FORMATTING REQUIREMENTS.


THE SOURCE LANGUAGE IS %s.

THE TEXT TO TRANSLATE IS %s.

FOLLOWING IS THE TEXT TO TRANSLATE, DO NOT TRANSLATE THE FORMATTING REQUIREMENTS.  PLEASE PROVIDE THE %s TRANSLATION FOR THIS TEXT, WHICH INCLUDE IN <TEXT TO TRANSLATE> TAG. 

<TEXT TO TRANSLATE>
%s
</TEXT TO TRANSLATE>
`,
		t.config.SourceLang, t.config.TargetLang, t.config.SourceLang,
		t.config.TargetLang, t.config.TargetLang, text)

	// 调用语言模型
	model := t.activeSteps.InitialModel
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

	// 缓存结果
	if t.config.UseCache {
		cacheKey := t.generateCacheKey(text, "initial")
		if err := t.cache.Set(cacheKey, result); err != nil {
			t.logger.Warn("缓存初始翻译结果失败", zap.Error(err))
		}
	}

	return result, nil
}

// reflection 执行反思
func (t *TranslatorImpl) reflection(sourceText, translation string) (string, error) {
	// 检查缓存
	if t.config.UseCache {
		cacheKey := t.generateCacheKey(sourceText+translation, "reflection")
		if cached, ok := t.cache.Get(cacheKey); ok {
			t.logger.Debug("从缓存加载反思结果")
			return cached, nil
		}
	}

	// 构建提示词
	prompt := fmt.Sprintf(`Your task is to carefully read a source text and a translation from %s to %s, and then give constructive criticism and helpful suggestions to improve the translation. 
The final style and tone of the translation should match the style of %s colloquially spoken in %s.

IMPORTANT FORMATTING REQUIREMENTS:
1. All original formatting must be preserved exactly:
   - Markdown syntax (**, *, #, etc.)
   - LaTeX formulas ($...$, $$...$$, \(...\), \[...\])
   - HTML tags (<...>)
2. All abbreviations and technical terms must remain unchanged:
   - Unknown abbreviations should be kept in original form
   - Code identifiers and variables must be preserved
3. Document structure must be maintained:
   - Line breaks and paragraph spacing
   - Table formatting and alignment
   - List markers and numbering
4. DO NOT MODIFY OR TRANSLATE any text between ⚡⚡⚡ symbols (e.g., ⚡⚡⚡UNTRANSLATABLE_0⚡⚡⚡).
   These are special markers that must remain exactly as they are.

The source text and initial translation, delimited by XML tags <SOURCE_TEXT></SOURCE_TEXT> and <TRANSLATION></TRANSLATION>, are as follows:

<SOURCE_TEXT>
%s
</SOURCE_TEXT>

<TRANSLATION>
%s
</TRANSLATION>

When writing suggestions, pay attention to whether there are ways to improve the translation's
(i) accuracy (by correcting errors of addition, mistranslation, omission, or untranslated text),
(ii) fluency (by applying %s grammar, spelling and punctuation rules, and ensuring there are no unnecessary repetitions),
(iii) style (by ensuring the translations reflect the style of the source text and take into account any cultural context),
(iv) terminology (by ensuring terminology use is consistent and reflects the source text domain; and by only ensuring you use equivalent idioms %s).
(v) formatting (by ensuring the translations reflect the formatting of the source text, including the use of Markdown syntax, LaTeX formulas, and HTML tags and Text formatting).

Write a list of specific, helpful and constructive suggestions for improving the translation.
Each suggestion should address one specific part of the translation.
Output only the suggestions and nothing else.`,
		t.config.SourceLang, t.config.TargetLang,
		t.config.TargetLang, t.config.Country,
		sourceText,
		translation,
		t.config.TargetLang,
		t.config.TargetLang)

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

	// 缓存结果
	if t.config.UseCache {
		cacheKey := t.generateCacheKey(sourceText+translation, "reflection")
		if err := t.cache.Set(cacheKey, result); err != nil {
			t.logger.Warn("缓存反思结果失败", zap.Error(err))
		}
	}

	return result, nil
}

// improvement 执行改进
func (t *TranslatorImpl) improvement(sourceText, translation, reflection string) (string, error) {
	// 检查缓存
	if t.config.UseCache {
		cacheKey := t.generateCacheKey(sourceText+translation+reflection, "improvement")
		if cached, ok := t.cache.Get(cacheKey); ok {
			t.logger.Debug("从缓存加载改进结果")
			return cached, nil
		}
	}

	// 构建提示词
	prompt := fmt.Sprintf(`Your task is to carefully read, then edit, a translation from %s to %s, taking into
account a list of expert suggestions and constructive criticisms.

CRITICAL FORMATTING REQUIREMENTS:
1. The following elements MUST remain exactly as in the source:
   - All Markdown formatting (**, *, #, etc.)
   - All LaTeX formulas ($...$, $$...$$, \(...\), \[...\])
   - All HTML tags (<...>)
2. Preserve all technical elements:
   - Keep unknown abbreviations in original form
   - Maintain all code identifiers and variables
   - Preserve all URLs and file paths
3. Maintain document structure:
   - Keep all line breaks and spacing
   - Preserve table formatting
   - Keep list markers and numbering
4. DO NOT MODIFY OR TRANSLATE any text between ⚡⚡⚡ symbols (e.g., ⚡⚡⚡UNTRANSLATABLE_0⚡⚡⚡).

The source text, the initial translation, and the expert linguist suggestions are delimited by XML tags <SOURCE_TEXT></SOURCE_TEXT>, <TRANSLATION></TRANSLATION> and <EXPERT_SUGGESTIONS></EXPERT_SUGGESTIONS> as follows:

<SOURCE_TEXT>
%s
</SOURCE_TEXT>

<TRANSLATION>
%s
</TRANSLATION>

<EXPERT_SUGGESTIONS>
%s
</EXPERT_SUGGESTIONS>

Please take into account the expert suggestions when editing the translation. Edit the translation by ensuring:

(i) accuracy (by correcting errors of addition, mistranslation, omission, or untranslated text),
(ii) fluency (by applying %s grammar, spelling and punctuation rules and ensuring there are no unnecessary repetitions),
(iii) style (by ensuring the translations reflect the style of the source text)
(iv) terminology (inappropriate for context, inconsistent use), or
(v) formatting (by ensuring the translations reflect the formatting of the source text, including the use of Markdown syntax, LaTeX formulas, and HTML tags and Text formatting).
(vi) other errors.

Output only the new translation and nothing else.`,
		t.config.SourceLang, t.config.TargetLang,
		sourceText,
		translation,
		reflection,
		t.config.TargetLang)

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

	// 缓存结果
	if t.config.UseCache {
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
