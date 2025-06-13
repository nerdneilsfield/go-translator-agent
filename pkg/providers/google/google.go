package google

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/pkg/providers"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// Config Google Translate配置
type Config struct {
	providers.BaseConfig
	ProjectID string `json:"project_id,omitempty"` // 用于Google Cloud Translation API
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	config := Config{
		BaseConfig: providers.DefaultConfig(),
	}
	// Google Translation API endpoint
	config.APIEndpoint = "https://translation.googleapis.com/language/translate/v2"
	return config
}

// Provider Google Translate提供商
type Provider struct {
	config     Config
	httpClient *http.Client
}

// New 创建新的Google Translate提供商
func New(config Config) *Provider {
	if config.APIEndpoint == "" {
		config.APIEndpoint = "https://translation.googleapis.com/language/translate/v2"
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
	translateReq := TranslateRequest{
		Q:      req.Text,
		Source: normalizeLanguageCode(req.SourceLanguage),
		Target: normalizeLanguageCode(req.TargetLanguage),
		Format: "text",
	}
	
	// 检查是否包含HTML格式
	if format, ok := req.Options["format"]; ok && format == "html" {
		translateReq.Format = "html"
	}
	
	// 执行翻译
	resp, err := p.translate(ctx, translateReq)
	if err != nil {
		return nil, err
	}
	
	if len(resp.Data.Translations) == 0 {
		return nil, fmt.Errorf("no translation returned")
	}
	
	// 返回响应
	return &translation.ProviderResponse{
		Text:  resp.Data.Translations[0].TranslatedText,
		Model: "google-translate",
		Metadata: map[string]string{
			"detected_source": resp.Data.Translations[0].DetectedSourceLanguage,
		},
	}, nil
}

// GetName 获取提供商名称
func (p *Provider) GetName() string {
	return "google"
}

// SupportsSteps 不支持多步骤翻译
func (p *Provider) SupportsSteps() bool {
	return false
}

// GetCapabilities 获取提供商能力
func (p *Provider) GetCapabilities() providers.Capabilities {
	return providers.Capabilities{
		SupportedLanguages: []providers.Language{
			{Code: "af", Name: "Afrikaans"},
			{Code: "sq", Name: "Albanian"},
			{Code: "am", Name: "Amharic"},
			{Code: "ar", Name: "Arabic"},
			{Code: "hy", Name: "Armenian"},
			{Code: "az", Name: "Azerbaijani"},
			{Code: "eu", Name: "Basque"},
			{Code: "be", Name: "Belarusian"},
			{Code: "bn", Name: "Bengali"},
			{Code: "bs", Name: "Bosnian"},
			{Code: "bg", Name: "Bulgarian"},
			{Code: "ca", Name: "Catalan"},
			{Code: "zh", Name: "Chinese"},
			{Code: "zh-CN", Name: "Chinese (Simplified)"},
			{Code: "zh-TW", Name: "Chinese (Traditional)"},
			{Code: "hr", Name: "Croatian"},
			{Code: "cs", Name: "Czech"},
			{Code: "da", Name: "Danish"},
			{Code: "nl", Name: "Dutch"},
			{Code: "en", Name: "English"},
			{Code: "et", Name: "Estonian"},
			{Code: "fi", Name: "Finnish"},
			{Code: "fr", Name: "French"},
			{Code: "gl", Name: "Galician"},
			{Code: "ka", Name: "Georgian"},
			{Code: "de", Name: "German"},
			{Code: "el", Name: "Greek"},
			{Code: "gu", Name: "Gujarati"},
			{Code: "ht", Name: "Haitian Creole"},
			{Code: "he", Name: "Hebrew"},
			{Code: "hi", Name: "Hindi"},
			{Code: "hu", Name: "Hungarian"},
			{Code: "is", Name: "Icelandic"},
			{Code: "id", Name: "Indonesian"},
			{Code: "ga", Name: "Irish"},
			{Code: "it", Name: "Italian"},
			{Code: "ja", Name: "Japanese"},
			{Code: "kn", Name: "Kannada"},
			{Code: "kk", Name: "Kazakh"},
			{Code: "km", Name: "Khmer"},
			{Code: "ko", Name: "Korean"},
			{Code: "ky", Name: "Kyrgyz"},
			{Code: "lo", Name: "Lao"},
			{Code: "la", Name: "Latin"},
			{Code: "lv", Name: "Latvian"},
			{Code: "lt", Name: "Lithuanian"},
			{Code: "mk", Name: "Macedonian"},
			{Code: "ms", Name: "Malay"},
			{Code: "ml", Name: "Malayalam"},
			{Code: "mt", Name: "Maltese"},
			{Code: "mr", Name: "Marathi"},
			{Code: "mn", Name: "Mongolian"},
			{Code: "my", Name: "Myanmar (Burmese)"},
			{Code: "ne", Name: "Nepali"},
			{Code: "no", Name: "Norwegian"},
			{Code: "ps", Name: "Pashto"},
			{Code: "fa", Name: "Persian"},
			{Code: "pl", Name: "Polish"},
			{Code: "pt", Name: "Portuguese"},
			{Code: "pa", Name: "Punjabi"},
			{Code: "ro", Name: "Romanian"},
			{Code: "ru", Name: "Russian"},
			{Code: "sr", Name: "Serbian"},
			{Code: "si", Name: "Sinhala"},
			{Code: "sk", Name: "Slovak"},
			{Code: "sl", Name: "Slovenian"},
			{Code: "es", Name: "Spanish"},
			{Code: "sw", Name: "Swahili"},
			{Code: "sv", Name: "Swedish"},
			{Code: "ta", Name: "Tamil"},
			{Code: "te", Name: "Telugu"},
			{Code: "th", Name: "Thai"},
			{Code: "tr", Name: "Turkish"},
			{Code: "uk", Name: "Ukrainian"},
			{Code: "ur", Name: "Urdu"},
			{Code: "uz", Name: "Uzbek"},
			{Code: "vi", Name: "Vietnamese"},
			{Code: "cy", Name: "Welsh"},
			{Code: "yi", Name: "Yiddish"},
		},
		MaxTextLength:      5000, // Google Translate v2 API限制
		SupportsBatch:      true,
		SupportsFormatting: true, // 支持HTML格式
		RequiresAPIKey:     true,
		RateLimit: &providers.RateLimit{
			RequestsPerMinute: 600,          // 取决于配额
			CharactersPerDay:  500000,       // 免费层级限制
		},
	}
}

// HealthCheck 健康检查
func (p *Provider) HealthCheck(ctx context.Context) error {
	// 翻译一个简单的文本
	req := &translation.ProviderRequest{
		Text:           "Hello",
		SourceLanguage: "en",
		TargetLanguage: "es",
	}
	
	_, err := p.Translate(ctx, req)
	return err
}

// translate 执行翻译请求
func (p *Provider) translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	// 构建URL参数
	params := url.Values{}
	params.Set("key", p.config.APIKey)
	params.Set("q", req.Q)
	params.Set("source", req.Source)
	params.Set("target", req.Target)
	params.Set("format", req.Format)
	
	// 创建请求
	httpReq, err := http.NewRequestWithContext(ctx, "POST", 
		p.config.APIEndpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// 设置头部
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
			lastErr = fmt.Errorf("Google API error: %s", apiErr.Error.Message)
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
	var translateResp TranslateResponse
	if err := json.NewDecoder(resp.Body).Decode(&translateResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	return &translateResp, nil
}

// normalizeLanguageCode 标准化语言代码
func normalizeLanguageCode(lang string) string {
	// 转换常见的语言代码格式
	replacements := map[string]string{
		"chinese":    "zh",
		"chinese_simplified": "zh-CN",
		"chinese_traditional": "zh-TW",
		"english":    "en",
		"spanish":    "es",
		"french":     "fr",
		"german":     "de",
		"japanese":   "ja",
		"korean":     "ko",
		"portuguese": "pt",
		"russian":    "ru",
		"italian":    "it",
	}
	
	lower := strings.ToLower(lang)
	if normalized, ok := replacements[lower]; ok {
		return normalized
	}
	
	// 处理 xx_YY 格式到 xx-YY
	if strings.Contains(lang, "_") {
		return strings.Replace(lang, "_", "-", 1)
	}
	
	return lang
}

// TranslateRequest 翻译请求
type TranslateRequest struct {
	Q      string `json:"q"`      // 要翻译的文本
	Source string `json:"source"` // 源语言
	Target string `json:"target"` // 目标语言
	Format string `json:"format"` // 文本格式：text 或 html
}

// TranslateResponse 翻译响应
type TranslateResponse struct {
	Data struct {
		Translations []struct {
			TranslatedText         string `json:"translatedText"`
			DetectedSourceLanguage string `json:"detectedSourceLanguage,omitempty"`
		} `json:"translations"`
	} `json:"data"`
}

// APIError API错误
type APIError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
			Domain  string `json:"domain"`
			Reason  string `json:"reason"`
		} `json:"errors"`
	} `json:"error"`
}