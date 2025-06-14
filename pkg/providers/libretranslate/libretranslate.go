package libretranslate

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

// Config LibreTranslate配置
type Config struct {
	providers.BaseConfig
	// LibreTranslate特定配置
	RequiresAPIKey bool `json:"requires_api_key"` // 服务器是否需要API密钥
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	config := Config{
		BaseConfig:     providers.DefaultConfig(),
		RequiresAPIKey: false,
	}
	// 默认使用官方演示服务器
	config.APIEndpoint = "https://libretranslate.com"
	return config
}

// Provider LibreTranslate提供商
type Provider struct {
	config     Config
	httpClient *http.Client
	languages  []Language // 缓存支持的语言
}

// New 创建新的LibreTranslate提供商
func New(config Config) *Provider {
	if config.APIEndpoint == "" {
		config.APIEndpoint = "https://libretranslate.com"
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
	// 获取支持的语言列表（如果还没有）
	if p.languages == nil {
		if err := p.fetchLanguages(ctx); err != nil {
			// 如果获取失败，使用默认映射
			p.languages = getDefaultLanguages()
		}
	}

	// 标准化语言代码
	sourceLang := p.normalizeLanguageCode(req.SourceLanguage)
	targetLang := p.normalizeLanguageCode(req.TargetLanguage)

	// 构建请求
	translateReq := TranslateRequest{
		Q:      req.Text,
		Source: sourceLang,
		Target: targetLang,
		Format: "text",
	}

	// 添加API密钥（如果需要）
	if p.config.RequiresAPIKey && p.config.APIKey != "" {
		translateReq.APIKey = p.config.APIKey
	}

	// 检查选项
	if format, ok := req.Options["format"]; ok && format == "html" {
		translateReq.Format = "html"
	}

	// 执行翻译
	resp, err := p.translate(ctx, translateReq)
	if err != nil {
		return nil, err
	}

	// 返回响应
	metadata := make(map[string]string)
	if resp.DetectedLanguage != nil {
		metadata["detected_source"] = resp.DetectedLanguage.Language
		metadata["confidence"] = fmt.Sprintf("%.2f", resp.DetectedLanguage.Confidence)
	}

	return &translation.ProviderResponse{
		Text:     resp.TranslatedText,
		Model:    "libretranslate",
		Metadata: metadata,
	}, nil
}

// GetName 获取提供商名称
func (p *Provider) GetName() string {
	return "libretranslate"
}

// SupportsSteps 不支持多步骤翻译
func (p *Provider) SupportsSteps() bool {
	return false
}

// GetCapabilities 获取提供商能力
func (p *Provider) GetCapabilities() providers.Capabilities {
	// 如果已经获取了语言列表，使用实际的
	var supportedLangs []providers.Language
	if p.languages != nil {
		for _, lang := range p.languages {
			supportedLangs = append(supportedLangs, providers.Language{
				Code: lang.Code,
				Name: lang.Name,
			})
		}
	} else {
		// 使用默认语言列表
		defaultLangs := getDefaultLanguages()
		for _, lang := range defaultLangs {
			supportedLangs = append(supportedLangs, providers.Language{
				Code: lang.Code,
				Name: lang.Name,
			})
		}
	}

	return providers.Capabilities{
		SupportedLanguages: supportedLangs,
		MaxTextLength:      5000, // LibreTranslate限制
		SupportsBatch:      false,
		SupportsFormatting: true, // 支持HTML
		RequiresAPIKey:     p.config.RequiresAPIKey,
	}
}

// HealthCheck 健康检查
func (p *Provider) HealthCheck(ctx context.Context) error {
	// 获取语言列表作为健康检查
	return p.fetchLanguages(ctx)
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
		p.config.APIEndpoint+"/translate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置头部
	httpReq.Header.Set("Content-Type", "application/json")
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

		// 检查状态码
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			// 解析成功响应
			var translateResp TranslateResponse
			if err := json.Unmarshal(respBody, &translateResp); err != nil {
				return nil, fmt.Errorf("failed to decode response: %w", err)
			}
			return &translateResp, nil
		}

		// 解析错误响应
		var errorResp ErrorResponse
		if err := json.Unmarshal(respBody, &errorResp); err == nil && errorResp.Error != "" {
			lastErr = fmt.Errorf("API error: %s", errorResp.Error)
		} else {
			lastErr = fmt.Errorf("API error: %s", resp.Status)
		}

		// 检查是否可重试
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			continue
		}
		break
	}

	return nil, lastErr
}

// fetchLanguages 获取支持的语言列表
func (p *Provider) fetchLanguages(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET",
		p.config.APIEndpoint+"/languages", nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch languages: %s", resp.Status)
	}

	var languages []Language
	if err := json.NewDecoder(resp.Body).Decode(&languages); err != nil {
		return err
	}

	p.languages = languages
	return nil
}

// normalizeLanguageCode 标准化语言代码
func (p *Provider) normalizeLanguageCode(lang string) string {
	lower := strings.ToLower(lang)

	// 常见映射
	replacements := map[string]string{
		"chinese":    "zh",
		"english":    "en",
		"spanish":    "es",
		"french":     "fr",
		"german":     "de",
		"japanese":   "ja",
		"korean":     "ko",
		"portuguese": "pt",
		"russian":    "ru",
		"italian":    "it",
		"arabic":     "ar",
		"hindi":      "hi",
		"turkish":    "tr",
		"polish":     "pl",
		"dutch":      "nl",
		"swedish":    "sv",
		"danish":     "da",
		"norwegian":  "no",
		"finnish":    "fi",
	}

	if normalized, ok := replacements[lower]; ok {
		return normalized
	}

	// 如果已经是两字母代码，直接返回
	if len(lang) == 2 {
		return lower
	}

	// 尝试从已知语言列表中查找
	for _, l := range p.languages {
		if strings.EqualFold(l.Name, lang) {
			return l.Code
		}
	}

	return lower
}

// getDefaultLanguages 返回默认语言列表
func getDefaultLanguages() []Language {
	return []Language{
		{Code: "en", Name: "English"},
		{Code: "ar", Name: "Arabic"},
		{Code: "zh", Name: "Chinese"},
		{Code: "fr", Name: "French"},
		{Code: "de", Name: "German"},
		{Code: "hi", Name: "Hindi"},
		{Code: "id", Name: "Indonesian"},
		{Code: "ga", Name: "Irish"},
		{Code: "it", Name: "Italian"},
		{Code: "ja", Name: "Japanese"},
		{Code: "ko", Name: "Korean"},
		{Code: "pl", Name: "Polish"},
		{Code: "pt", Name: "Portuguese"},
		{Code: "ru", Name: "Russian"},
		{Code: "es", Name: "Spanish"},
		{Code: "tr", Name: "Turkish"},
		{Code: "vi", Name: "Vietnamese"},
	}
}

// Language 语言信息
type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// TranslateRequest 翻译请求
type TranslateRequest struct {
	Q      string `json:"q"`                 // 要翻译的文本
	Source string `json:"source"`            // 源语言
	Target string `json:"target"`            // 目标语言
	Format string `json:"format"`            // 文本格式
	APIKey string `json:"api_key,omitempty"` // API密钥（如果需要）
}

// TranslateResponse 翻译响应
type TranslateResponse struct {
	TranslatedText   string `json:"translatedText"`
	DetectedLanguage *struct {
		Confidence float64 `json:"confidence"`
		Language   string  `json:"language"`
	} `json:"detectedLanguage,omitempty"`
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Error string `json:"error"`
}
