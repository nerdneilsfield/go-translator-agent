package deeplx

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
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// Config DeepLX配置
type Config struct {
	providers.BaseConfig
	// DeepLX特定配置
	AccessToken string `json:"access_token,omitempty"` // 可选的访问令牌
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	config := Config{
		BaseConfig: providers.DefaultConfig(),
	}
	// 默认使用公共DeepLX服务
	config.APIEndpoint = "http://localhost:1188/translate"
	return config
}

// Provider DeepLX提供商
type Provider struct {
	config     Config
	httpClient *http.Client
}

// New 创建新的DeepLX提供商
func New(config Config) *Provider {
	if config.APIEndpoint == "" {
		config.APIEndpoint = "http://localhost:1188/translate"
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
	// 构建请求
	deeplxReq := TranslateRequest{
		Text:       req.Text,
		SourceLang: normalizeLanguageCode(req.SourceLanguage),
		TargetLang: normalizeLanguageCode(req.TargetLanguage),
	}
	
	// 执行翻译
	resp, err := p.translate(ctx, deeplxReq)
	if err != nil {
		return nil, err
	}
	
	// 检查响应
	if resp.Code != 200 {
		return nil, fmt.Errorf("translation failed: %s", resp.Message)
	}
	
	// 返回响应
	metadata := make(map[string]string)
	if resp.SourceLang != "" {
		metadata["detected_source"] = resp.SourceLang
	}
	
	return &translation.ProviderResponse{
		Text:     resp.Data,
		Model:    "deeplx",
		Metadata: metadata,
	}, nil
}

// GetName 获取提供商名称
func (p *Provider) GetName() string {
	return "deeplx"
}

// SupportsSteps 不支持多步骤翻译
func (p *Provider) SupportsSteps() bool {
	return false
}

// GetCapabilities 获取提供商能力
func (p *Provider) GetCapabilities() providers.Capabilities {
	// DeepLX支持与DeepL相同的语言
	return providers.Capabilities{
		SupportedLanguages: []providers.Language{
			{Code: "BG", Name: "Bulgarian"},
			{Code: "CS", Name: "Czech"},
			{Code: "DA", Name: "Danish"},
			{Code: "DE", Name: "German"},
			{Code: "EL", Name: "Greek"},
			{Code: "EN", Name: "English"},
			{Code: "ES", Name: "Spanish"},
			{Code: "ET", Name: "Estonian"},
			{Code: "FI", Name: "Finnish"},
			{Code: "FR", Name: "French"},
			{Code: "HU", Name: "Hungarian"},
			{Code: "ID", Name: "Indonesian"},
			{Code: "IT", Name: "Italian"},
			{Code: "JA", Name: "Japanese"},
			{Code: "KO", Name: "Korean"},
			{Code: "LT", Name: "Lithuanian"},
			{Code: "LV", Name: "Latvian"},
			{Code: "NB", Name: "Norwegian"},
			{Code: "NL", Name: "Dutch"},
			{Code: "PL", Name: "Polish"},
			{Code: "PT", Name: "Portuguese"},
			{Code: "RO", Name: "Romanian"},
			{Code: "RU", Name: "Russian"},
			{Code: "SK", Name: "Slovak"},
			{Code: "SL", Name: "Slovenian"},
			{Code: "SV", Name: "Swedish"},
			{Code: "TR", Name: "Turkish"},
			{Code: "UK", Name: "Ukrainian"},
			{Code: "ZH", Name: "Chinese"},
		},
		MaxTextLength:      5000,  // 建议限制
		SupportsBatch:      false,
		SupportsFormatting: false,
		RequiresAPIKey:     false, // DeepLX不需要API密钥
	}
}

// HealthCheck 健康检查
func (p *Provider) HealthCheck(ctx context.Context) error {
	// 尝试翻译一个简单的文本
	req := &translation.ProviderRequest{
		Text:           "Hello",
		SourceLanguage: "EN",
		TargetLanguage: "ZH",
	}
	
	_, err := p.Translate(ctx, req)
	return err
}

// translate 执行翻译请求
func (p *Provider) translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	// 编码请求
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	// 创建HTTP请求
	httpReq, err := http.NewRequestWithContext(ctx, "POST", 
		p.config.APIEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// 设置头部
	httpReq.Header.Set("Content-Type", "application/json")
	if p.config.AccessToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.config.AccessToken)
	}
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
		
		// 读取响应体
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}
		
		// 解析响应
		var translateResp TranslateResponse
		if err := json.Unmarshal(respBody, &translateResp); err != nil {
			lastErr = fmt.Errorf("failed to decode response: %w", err)
			continue
		}
		
		// 检查业务错误
		if translateResp.Code != 200 {
			lastErr = fmt.Errorf("API error: %s", translateResp.Message)
			// 某些错误不应重试
			if translateResp.Code == 400 || translateResp.Code == 404 {
				break
			}
			continue
		}
		
		return &translateResp, nil
	}
	
	return nil, lastErr
}

// normalizeLanguageCode 标准化语言代码
func normalizeLanguageCode(lang string) string {
	// DeepLX使用大写的语言代码，与DeepL兼容
	upper := strings.ToUpper(lang)
	
	// 特殊处理
	replacements := map[string]string{
		"CHINESE":    "ZH",
		"ENGLISH":    "EN",
		"SPANISH":    "ES",
		"FRENCH":     "FR",
		"GERMAN":     "DE",
		"JAPANESE":   "JA",
		"KOREAN":     "KO",
		"PORTUGUESE": "PT",
		"RUSSIAN":    "RU",
		"ITALIAN":    "IT",
	}
	
	if normalized, ok := replacements[upper]; ok {
		return normalized
	}
	
	return upper
}

// TranslateRequest 翻译请求
type TranslateRequest struct {
	Text       string `json:"text"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
}

// TranslateResponse 翻译响应
type TranslateResponse struct {
	Code       int    `json:"code"`
	Message    string `json:"message,omitempty"`
	Data       string `json:"data"`
	SourceLang string `json:"source_lang,omitempty"`
}