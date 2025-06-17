package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/pkg/providers"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/retry"
)

// Config Ollama配置
type Config struct {
	providers.BaseConfig
	Model       string            `json:"model"`
	Temperature float32           `json:"temperature"`
	MaxTokens   int               `json:"max_tokens"`
	Stream      bool              `json:"stream"`
	RetryConfig retry.RetryConfig `json:"retry_config"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		BaseConfig:  providers.DefaultConfig(),
		Model:       "llama2",
		Temperature: 0.3,
		MaxTokens:   4096,
		Stream:      false,
		RetryConfig: retry.DefaultRetryConfig(),
	}
}

// Provider Ollama提供商
type Provider struct {
	config      Config
	httpClient  *http.Client
	retryClient *retry.RetryableHTTPClient
}

// New 创建新的Ollama提供商
func New(config Config) *Provider {
	if config.APIEndpoint == "" {
		config.APIEndpoint = "http://localhost:11434"
	}

	httpClient := &http.Client{
		Timeout: config.Timeout,
	}

	// 创建网络重试器
	networkRetrier := retry.NewNetworkRetrier(config.RetryConfig)
	retryClient := networkRetrier.WrapHTTPClient(httpClient)

	return &Provider{
		config:      config,
		httpClient:  httpClient,
		retryClient: retryClient,
	}
}

// Configure 配置提供商
func (p *Provider) Configure(config interface{}) error {
	cfg, ok := config.(Config)
	if !ok {
		return fmt.Errorf("invalid config type: expected Config")
	}
	p.config = cfg
	return nil
}

// Translate 执行翻译
func (p *Provider) Translate(ctx context.Context, req *providers.ProviderRequest) (*providers.ProviderResponse, error) {
	var prompt string
	
	// 检查是否有预构建的完整提示词（优先使用）
	if p.isFullPrompt(req.Text) {
		prompt = req.Text
	} else {
		// 否则使用传统方式构建
		prompt = fmt.Sprintf("Translate the following text from %s to %s. Please only return the translated text without any additional explanations:\n\n%s",
			req.SourceLanguage, req.TargetLanguage, req.Text)

		// 如果有额外的上下文或指令
		if req.Metadata != nil {
			if instruction, ok := req.Metadata["instruction"]; ok {
				if instructionStr, ok := instruction.(string); ok {
					prompt = instructionStr + "\n\n" + prompt
				}
			}
		}
	}

	// 创建请求
	generateReq := GenerateRequest{
		Model:  p.config.Model,
		Prompt: prompt,
		Stream: false,
		Options: map[string]interface{}{
			"temperature": p.config.Temperature,
		},
	}

	// 如果设置了 MaxTokens，添加到选项中
	if p.config.MaxTokens > 0 {
		generateReq.Options["num_predict"] = p.config.MaxTokens
	}

	// 执行请求
	resp, err := p.generate(ctx, generateReq)
	if err != nil {
		return nil, err
	}

	// 返回响应
	return &providers.ProviderResponse{
		Text:      resp.Response,
		TokensIn:  resp.PromptEvalCount,
		TokensOut: resp.EvalCount,
		Metadata: map[string]interface{}{
			"model":          resp.Model,
			"created_at":     resp.CreatedAt,
			"total_duration": resp.TotalDuration,
			"eval_duration":  resp.EvalDuration,
		},
	}, nil
}

// GetName 获取提供商名称
func (p *Provider) GetName() string {
	return "ollama"
}

// SupportsSteps 支持多步骤翻译
func (p *Provider) SupportsSteps() bool {
	return true
}

// GetCapabilities 获取提供商能力
func (p *Provider) GetCapabilities() providers.Capabilities {
	return providers.Capabilities{
		SupportedLanguages: []providers.Language{
			{Code: "en", Name: "English"},
			{Code: "zh", Name: "Chinese"},
			{Code: "ja", Name: "Japanese"},
			{Code: "ko", Name: "Korean"},
			{Code: "es", Name: "Spanish"},
			{Code: "fr", Name: "French"},
			{Code: "de", Name: "German"},
			{Code: "ru", Name: "Russian"},
			{Code: "pt", Name: "Portuguese"},
			{Code: "it", Name: "Italian"},
			{Code: "ar", Name: "Arabic"},
			{Code: "hi", Name: "Hindi"},
			// Ollama支持的语言取决于所使用的模型
		},
		MaxTextLength:      8000, // 取决于模型的上下文长度
		SupportsBatch:      false,
		SupportsFormatting: true,
		RequiresAPIKey:     false, // Ollama通常不需要API密钥
		RateLimit: &providers.RateLimit{
			RequestsPerMinute: 60, // 本地部署通常没有严格限制
		},
	}
}

// HealthCheck 健康检查
func (p *Provider) HealthCheck(ctx context.Context) error {
	// 发送一个简单的生成请求
	req := GenerateRequest{
		Model:  p.config.Model,
		Prompt: "Hello",
		Stream: false,
		Options: map[string]interface{}{
			"num_predict": 5,
		},
	}

	_, err := p.generate(ctx, req)
	return err
}

// generate 执行生成请求
func (p *Provider) generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	// 编码请求
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建HTTP请求
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.config.APIEndpoint+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置头部
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range p.config.Headers {
		httpReq.Header.Set(k, v)
	}

	// 执行请求，使用智能重试
	resp, err := p.retryClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 读取错误响应
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// 解析错误
		var apiErr APIError
		if json.Unmarshal(errBody, &apiErr) == nil {
			return nil, &apiErr
		}
		return nil, fmt.Errorf("API error: %s", resp.Status)
	}

	defer resp.Body.Close()

	// 解析响应
	var generateResp GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&generateResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &generateResp, nil
}

// isFullPrompt 检查是否为完整的预构建提示词
func (p *Provider) isFullPrompt(text string) bool {
	// 检查是否包含系统指令和翻译指令的关键标识
	return strings.Contains(text, "You are a professional translator") && 
		   strings.Contains(text, "🚨 CRITICAL INSTRUCTION")
}

// GenerateRequest 生成请求
type GenerateRequest struct {
	Model   string                 `json:"model"`
	Prompt  string                 `json:"prompt"`
	Stream  bool                   `json:"stream"`
	Options map[string]interface{} `json:"options,omitempty"`
}

// GenerateResponse 生成响应
type GenerateResponse struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Response           string    `json:"response"`
	Done               bool      `json:"done"`
	TotalDuration      int64     `json:"total_duration"`
	LoadDuration       int64     `json:"load_duration"`
	PromptEvalCount    int       `json:"prompt_eval_count"`
	PromptEvalDuration int64     `json:"prompt_eval_duration"`
	EvalCount          int       `json:"eval_count"`
	EvalDuration       int64     `json:"eval_duration"`
}

// APIError API错误
type APIError struct {
	ErrorMsg string `json:"error"`
}

func (e *APIError) Error() string {
	return e.ErrorMsg
}
