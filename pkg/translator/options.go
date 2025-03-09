package translator

import "github.com/schollz/progressbar/v3"

// Option 定义翻译器选项
type Option func(*translatorOptions)

// translatorOptions 包含翻译器选项
type translatorOptions struct {
	cache             Cache
	forceCacheRefresh bool // 强制刷新缓存
	progressBar       *progressbar.ProgressBar
}

// WithCache 设置缓存
func WithCache(cache Cache) Option {
	return func(opts *translatorOptions) {
		opts.cache = cache
	}
}

// WithForceCacheRefresh 设置强制刷新缓存
func WithForceCacheRefresh() Option {
	return func(opts *translatorOptions) {
		opts.forceCacheRefresh = true
	}
}

// WithProgressBar 设置进度条
func WithProgressBar(bar *progressbar.ProgressBar) Option {
	return func(opts *translatorOptions) {
		opts.progressBar = bar
	}
}
