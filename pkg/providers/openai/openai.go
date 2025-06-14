package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/pkg/providers"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// Config OpenAI配置
type Config struct {
	providers.BaseConfig
	Model       string  `json:"model"`
	Temperature float32 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		BaseConfig:  providers.DefaultConfig(),
		Model:       "gpt-3.5-turbo",
		Temperature: 0.3,
		MaxTokens:   4096,
	}
}

// Provider OpenAI提供商
type Provider struct {
	config     Config
	httpClient *http.Client
}

// New 创建新的OpenAI提供商
func New(config Config) *Provider {
	if config.APIEndpoint == "" {
		config.APIEndpoint = "https://api.openai.com/v1"
	}

	return &Provider{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
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
func (p *Provider) Translate(ctx context.Context, req *translation.ProviderRequest) (*translation.ProviderResponse, error) {
	// 构建聊天消息
	messages := []Message{
		{
			Role:    "system",
			Content: "You are a professional translator. Translate accurately while preserving the original meaning and tone.",
		},
		{
			Role: "user",
			Content: fmt.Sprintf("Translate the following text from %s to %s:\n\n%s",
				req.SourceLanguage, req.TargetLanguage, req.Text),
		},
	}

	// 如果有额外的上下文或指令
	if instruction, ok := req.Options["instruction"]; ok {
		messages[0].Content += "\n\n" + instruction
	}

	// 创建请求
	chatReq := ChatRequest{
		Model:       p.config.Model,
		Messages:    messages,
		Temperature: p.config.Temperature,
		MaxTokens:   p.config.MaxTokens,
	}

	// 执行请求
	resp, err := p.chat(ctx, chatReq)
	if err != nil {
		return nil, err
	}

	// 返回响应
	return &translation.ProviderResponse{
		Text:      resp.Choices[0].Message.Content,
		Model:     resp.Model,
		TokensIn:  resp.Usage.PromptTokens,
		TokensOut: resp.Usage.CompletionTokens,
		Metadata: map[string]string{
			"finish_reason": resp.Choices[0].FinishReason,
			"id":            resp.ID,
		},
	}, nil
}

// GetName 获取提供商名称
func (p *Provider) GetName() string {
	return "openai"
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
			// OpenAI支持更多语言，这里只列出主要的
		},
		MaxTextLength:      8000, // 取决于模型
		SupportsBatch:      false,
		SupportsFormatting: true,
		RequiresAPIKey:     true,
		RateLimit: &providers.RateLimit{
			RequestsPerMinute: 60, // 取决于账户类型
		},
	}
}

// HealthCheck 健康检查
func (p *Provider) HealthCheck(ctx context.Context) error {
	// 发送一个简单的聊天请求
	req := ChatRequest{
		Model: p.config.Model,
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		MaxTokens: 10,
	}

	_, err := p.chat(ctx, req)
	return err
}

// chat 执行聊天请求
func (p *Provider) chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// 编码请求
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建HTTP请求
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.config.APIEndpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置头部
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	for k, v := range p.config.Headers {
		httpReq.Header.Set(k, v)
	}

	// 执行请求，带重试
	var resp *http.Response
	var lastErr error

	for i := 0; i <= p.config.MaxRetries; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(p.config.RetryDelay * time.Duration(i)):
			}
		}

		resp, err = p.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}

		// 检查状态码
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			break
		}

		// 读取错误响应
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// 解析错误
		var apiErr APIError
		if err := json.Unmarshal(errBody, &apiErr); err == nil {
			lastErr = &apiErr
		} else {
			lastErr = fmt.Errorf("API error: %s", resp.Status)
		}

		// 检查是否可重试
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			continue
		}
		break
	}

	if lastErr != nil {
		return nil, lastErr
	}

	defer resp.Body.Close()

	// 解析响应
	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &chatResp, nil
}

// Message 聊天消息
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest 聊天请求
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int     `json:"index"`
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// APIError API错误
type APIError struct {
	ErrorInfo struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func (e *APIError) Error() string {
	return e.ErrorInfo.Message
}

// 实现 LLMClient 接口以支持三步翻译
type LLMClient struct {
	provider *Provider
}

// NewLLMClient 创建LLMClient
func NewLLMClient(config Config) *LLMClient {
	return &LLMClient{
		provider: New(config),
	}
}

// Chat 实现 translation.LLMClient 接口
func (c *LLMClient) Chat(ctx context.Context, req *translation.ChatRequest) (*translation.ChatResponse, error) {
	// 转换消息格式
	messages := make([]Message, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// 创建请求
	chatReq := ChatRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}

	if chatReq.Model == "" {
		chatReq.Model = c.provider.config.Model
	}

	// 执行请求
	resp, err := c.provider.chat(ctx, chatReq)
	if err != nil {
		return nil, err
	}

	// 转换响应
	return &translation.ChatResponse{
		Message: translation.ChatMessage{
			Role:    resp.Choices[0].Message.Role,
			Content: resp.Choices[0].Message.Content,
		},
		Model:     resp.Model,
		TokensIn:  resp.Usage.PromptTokens,
		TokensOut: resp.Usage.CompletionTokens,
	}, nil
}

// Complete 实现 translation.LLMClient 接口
func (c *LLMClient) Complete(ctx context.Context, req *translation.CompletionRequest) (*translation.CompletionResponse, error) {
	// 将completion请求转换为chat请求
	chatReq := &translation.ChatRequest{
		Messages: []translation.ChatMessage{
			{
				Role:    "user",
				Content: req.Prompt,
			},
		},
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}

	resp, err := c.Chat(ctx, chatReq)
	if err != nil {
		return nil, err
	}

	return &translation.CompletionResponse{
		Text:      resp.Message.Content,
		Model:     resp.Model,
		TokensIn:  resp.TokensIn,
		TokensOut: resp.TokensOut,
	}, nil
}

// GetModel 获取模型
func (c *LLMClient) GetModel() string {
	return c.provider.config.Model
}

// HealthCheck 健康检查
func (c *LLMClient) HealthCheck(ctx context.Context) error {
	return c.provider.HealthCheck(ctx)
}
