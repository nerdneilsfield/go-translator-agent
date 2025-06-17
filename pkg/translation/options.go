package translation

import "go.uber.org/zap"

// Option 服务配置选项函数
type Option func(*serviceOptions)

// serviceOptions 服务内部选项
type serviceOptions struct {
	llmClient        LLMClient
	providers        map[string]TranslationProvider
	cache            Cache
	metricsCollector MetricsCollector
	progressTracker  ProgressTracker
	progressCallback func(*Progress)
	chunker          Chunker
	errorHandler     func(error)
	beforeTranslate  func(*Request)
	afterTranslate   func(*Response)
	logger           *zap.Logger
}

// WithLLMClient 设置LLM客户端
func WithLLMClient(client LLMClient) Option {
	return func(o *serviceOptions) {
		o.llmClient = client
	}
}

// WithCache 设置缓存
func WithCache(cache Cache) Option {
	return func(o *serviceOptions) {
		o.cache = cache
	}
}

// WithMetrics 设置指标收集器
func WithMetrics(collector MetricsCollector) Option {
	return func(o *serviceOptions) {
		o.metricsCollector = collector
	}
}

// WithProgressTracker 设置进度跟踪器
func WithProgressTracker(tracker ProgressTracker) Option {
	return func(o *serviceOptions) {
		o.progressTracker = tracker
	}
}

// WithProgressCallback 设置进度回调函数
func WithProgressCallback(callback func(*Progress)) Option {
	return func(o *serviceOptions) {
		o.progressCallback = callback
	}
}

// WithChunker 设置文本分块器
func WithChunker(chunker Chunker) Option {
	return func(o *serviceOptions) {
		o.chunker = chunker
	}
}

// WithErrorHandler 设置错误处理函数
func WithErrorHandler(handler func(error)) Option {
	return func(o *serviceOptions) {
		o.errorHandler = handler
	}
}

// WithBeforeTranslate 设置翻译前回调
func WithBeforeTranslate(hook func(*Request)) Option {
	return func(o *serviceOptions) {
		o.beforeTranslate = hook
	}
}

// WithAfterTranslate 设置翻译后回调
func WithAfterTranslate(hook func(*Response)) Option {
	return func(o *serviceOptions) {
		o.afterTranslate = hook
	}
}

// WithProviders 设置翻译提供商
func WithProviders(providers map[string]TranslationProvider) Option {
	return func(o *serviceOptions) {
		o.providers = providers
	}
}

// WithSingleProvider 添加单个翻译提供商
func WithSingleProvider(name string, provider TranslationProvider) Option {
	return func(o *serviceOptions) {
		if o.providers == nil {
			o.providers = make(map[string]TranslationProvider)
		}
		o.providers[name] = provider
	}
}

// WithLogger 设置logger
func WithLogger(logger *zap.Logger) Option {
	return func(o *serviceOptions) {
		o.logger = logger
	}
}

// TranslatorOption 翻译器配置选项
type TranslatorOption func(*translatorOptions)

// translatorOptions 翻译器内部选项
type translatorOptions struct {
	model       string
	temperature float32
	maxTokens   int
	systemRole  string
	variables   map[string]string
}

// WithModel 设置模型
func WithModel(model string) TranslatorOption {
	return func(o *translatorOptions) {
		o.model = model
	}
}

// WithTemperature 设置温度
func WithTemperature(temperature float32) TranslatorOption {
	return func(o *translatorOptions) {
		o.temperature = temperature
	}
}

// WithMaxTokens 设置最大token数
func WithMaxTokens(maxTokens int) TranslatorOption {
	return func(o *translatorOptions) {
		o.maxTokens = maxTokens
	}
}

// WithSystemRole 设置系统角色
func WithSystemRole(role string) TranslatorOption {
	return func(o *translatorOptions) {
		o.systemRole = role
	}
}

// WithVariables 设置提示词变量
func WithVariables(vars map[string]string) TranslatorOption {
	return func(o *translatorOptions) {
		if o.variables == nil {
			o.variables = make(map[string]string)
		}
		for k, v := range vars {
			o.variables[k] = v
		}
	}
}

// ChainOption 翻译链配置选项
type ChainOption func(*chainOptions)

// chainOptions 翻译链内部选项
type chainOptions struct {
	skipCache       bool
	continueOnError bool
	maxRetries      int
	parallelSteps   bool
}

// WithSkipCache 跳过缓存
func WithSkipCache(skip bool) ChainOption {
	return func(o *chainOptions) {
		o.skipCache = skip
	}
}

// WithContinueOnError 错误时继续执行
func WithContinueOnError(continueOnError bool) ChainOption {
	return func(o *chainOptions) {
		o.continueOnError = continueOnError
	}
}

// WithMaxRetries 设置最大重试次数
func WithMaxRetries(retries int) ChainOption {
	return func(o *chainOptions) {
		o.maxRetries = retries
	}
}

// WithParallelSteps 并行执行步骤
func WithParallelSteps(parallel bool) ChainOption {
	return func(o *chainOptions) {
		o.parallelSteps = parallel
	}
}
