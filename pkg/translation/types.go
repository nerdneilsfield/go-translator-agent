package translation

import (
	"time"
)

// Request 翻译请求
type Request struct {
	// Text 要翻译的文本
	Text string `json:"text"`

	// SourceLanguage 源语言（可选，覆盖默认配置）
	SourceLanguage string `json:"source_language,omitempty"`

	// TargetLanguage 目标语言（可选，覆盖默认配置）
	TargetLanguage string `json:"target_language,omitempty"`

	// Model 使用的模型
	Model string `json:"model,omitempty"`

	// Temperature 温度参数
	Temperature float32 `json:"temperature,omitempty"`

	// MaxTokens 最大令牌数
	MaxTokens int `json:"max_tokens,omitempty"`

	// Options 额外选项
	Options map[string]interface{} `json:"options,omitempty"`

	// Metadata 元数据
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Response 翻译响应
type Response struct {
	// Text 翻译后的文本
	Text string `json:"text"`

	// SourceLanguage 源语言
	SourceLanguage string `json:"source_language"`

	// TargetLanguage 目标语言
	TargetLanguage string `json:"target_language"`

	// Usage 使用情况
	Usage Usage `json:"usage,omitempty"`

	// Steps 各步骤的结果
	Steps []StepResult `json:"steps,omitempty"`

	// Metrics 性能指标
	Metrics *TranslationMetrics `json:"metrics,omitempty"`

	// Metadata 元数据
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// StepResult 步骤结果
type StepResult struct {
	Name      string        `json:"name"`
	Output    string        `json:"output"`
	Duration  time.Duration `json:"duration"`
	Model     string        `json:"model"`
	TokensIn  int           `json:"tokens_in"`
	TokensOut int           `json:"tokens_out"`
	Error     string        `json:"error,omitempty"`
}

// ChainResult 翻译链结果
type ChainResult struct {
	// FinalOutput 最终输出
	FinalOutput string `json:"final_output"`

	// Steps 各步骤结果
	Steps []StepResult `json:"steps"`

	// TotalDuration 总耗时
	TotalDuration time.Duration `json:"total_duration"`

	// Success 是否成功
	Success bool `json:"success"`

	// Error 错误信息
	Error error `json:"error,omitempty"`
}

// StepInput 步骤输入
type StepInput struct {
	Text           string            `json:"text"`
	SourceLanguage string            `json:"source_language"`
	TargetLanguage string            `json:"target_language"`
	PreviousOutput string            `json:"previous_output,omitempty"`
	Context        map[string]string `json:"context,omitempty"`
}

// StepOutput 步骤输出
type StepOutput struct {
	Text      string            `json:"text"`
	Model     string            `json:"model"`
	TokensIn  int               `json:"tokens_in"`
	TokensOut int               `json:"tokens_out"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// TranslationMetrics 翻译指标
type TranslationMetrics struct {
	ID             string        `json:"id"`
	StartTime      time.Time     `json:"start_time"`
	EndTime        time.Time     `json:"end_time"`
	Duration       time.Duration `json:"duration"`
	SourceLanguage string        `json:"source_language"`
	TargetLanguage string        `json:"target_language"`
	InputLength    int           `json:"input_length"`
	OutputLength   int           `json:"output_length"`
	TotalTokensIn  int           `json:"total_tokens_in"`
	TotalTokensOut int           `json:"total_tokens_out"`
	Model          string        `json:"model"`
	Success        bool          `json:"success"`
	ErrorType      string        `json:"error_type,omitempty"`
	CacheHit       bool          `json:"cache_hit"`
	ChunkCount     int           `json:"chunk_count"`
}

// StepMetrics 步骤指标
type StepMetrics struct {
	StepName   string        `json:"step_name"`
	StartTime  time.Time     `json:"start_time"`
	EndTime    time.Time     `json:"end_time"`
	Duration   time.Duration `json:"duration"`
	Model      string        `json:"model"`
	TokensIn   int           `json:"tokens_in"`
	TokensOut  int           `json:"tokens_out"`
	Success    bool          `json:"success"`
	ErrorType  string        `json:"error_type,omitempty"`
	RetryCount int           `json:"retry_count"`
}

// MetricsSummary 指标摘要
type MetricsSummary struct {
	TotalTranslations   int            `json:"total_translations"`
	SuccessfulCount     int            `json:"successful_count"`
	FailedCount         int            `json:"failed_count"`
	TotalDuration       time.Duration  `json:"total_duration"`
	AverageDuration     time.Duration  `json:"average_duration"`
	TotalTokensIn       int            `json:"total_tokens_in"`
	TotalTokensOut      int            `json:"total_tokens_out"`
	CacheHitRate        float64        `json:"cache_hit_rate"`
	ErrorsByType        map[string]int `json:"errors_by_type"`
	TranslationsByModel map[string]int `json:"translations_by_model"`
}

// CompletionRequest LLM补全请求
type CompletionRequest struct {
	Prompt      string   `json:"prompt"`
	Model       string   `json:"model"`
	Temperature float32  `json:"temperature"`
	MaxTokens   int      `json:"max_tokens"`
	TopP        float32  `json:"top_p,omitempty"`
	N           int      `json:"n,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// CompletionResponse LLM补全响应
type CompletionResponse struct {
	Text         string `json:"text"`
	Model        string `json:"model"`
	TokensIn     int    `json:"tokens_in"`
	TokensOut    int    `json:"tokens_out"`
	FinishReason string `json:"finish_reason"`
}

// ProviderRequest 翻译提供商请求
type ProviderRequest struct {
	Text           string            `json:"text"`
	SourceLanguage string            `json:"source_language"`
	TargetLanguage string            `json:"target_language"`
	Options        map[string]string `json:"options,omitempty"`
}

// ProviderResponse 翻译提供商响应
type ProviderResponse struct {
	Text      string            `json:"text"`
	Model     string            `json:"model"`
	TokensIn  int               `json:"tokens_in,omitempty"`
	TokensOut int               `json:"tokens_out,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ChatRequest 对话请求
type ChatRequest struct {
	Messages    []ChatMessage `json:"messages"`
	Model       string        `json:"model"`
	Temperature float32       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	TopP        float32       `json:"top_p,omitempty"`
	N           int           `json:"n,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
}

// ChatResponse 对话响应
type ChatResponse struct {
	Message      ChatMessage `json:"message"`
	Model        string      `json:"model"`
	TokensIn     int         `json:"tokens_in"`
	TokensOut    int         `json:"tokens_out"`
	FinishReason string      `json:"finish_reason"`
}

// ChatMessage 对话消息
type ChatMessage struct {
	Role    string `json:"role"` // system, user, assistant
	Content string `json:"content"`
}

// TranslateOption 翻译选项
type TranslateOption struct {
	Key   string
	Value interface{}
}

// Progress 进度信息
type Progress struct {
	Total     int     `json:"total"`
	Completed int     `json:"completed"`
	Current   string  `json:"current"`
	Percent   float64 `json:"percent"`
}

// Usage 使用情况
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
