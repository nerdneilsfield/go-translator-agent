package translator

import (
	"github.com/jedib0t/go-pretty/v6/progress"
)

// Option 定义翻译器选项
type Option func(*translatorOptions)

// translatorOptions 包含翻译器选项
type translatorOptions struct {
	cache             Cache
	forceCacheRefresh bool // 强制刷新缓存
	progressBar       *progress.Writer
	useNewProgressBar bool // 使用新的进度条系统
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
func WithProgressBar(bar *progress.Writer) Option {
	return func(opts *translatorOptions) {
		opts.progressBar = bar
	}
}

// WithNewProgressBar 使用新的进度条系统
func WithNewProgressBar() Option {
	return func(opts *translatorOptions) {
		opts.useNewProgressBar = true
	}
}
