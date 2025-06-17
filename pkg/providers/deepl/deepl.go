package deepl

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
)

// Config DeepL配置
type Config struct {
	providers.BaseConfig
	UseFreeAPI bool `json:"use_free_api"` // 是否使用免费API
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	config := Config{
		BaseConfig: providers.DefaultConfig(),
		UseFreeAPI: false,
	}
	config.APIEndpoint = "https://api.deepl.com/v2"
	return config
}

// Provider DeepL提供商
type Provider struct {
	config     Config
	httpClient *http.Client
}

// 确保 Provider 实现 providers.TranslationProvider 接口
var _ providers.TranslationProvider = (*Provider)(nil)

// New 创建新的DeepL提供商
func New(config Config) *Provider {
	if config.APIEndpoint == "" {
		if config.UseFreeAPI {
			config.APIEndpoint = "https://api-free.deepl.com/v2"
		} else {
			config.APIEndpoint = "https://api.deepl.com/v2"
		}
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
func (p *Provider) Translate(ctx context.Context, req *providers.ProviderRequest) (*providers.ProviderResponse, error) {
	// 构建请求参数
	params := url.Values{}
	params.Set("text", req.Text)
	params.Set("source_lang", normalizeLanguageCode(req.SourceLanguage, true))
	params.Set("target_lang", normalizeLanguageCode(req.TargetLanguage, false))

	// 可选参数
	if req.Metadata != nil {
		if formality, ok := req.Metadata["formality"]; ok {
			if formalityStr, ok := formality.(string); ok {
				params.Set("formality", formalityStr)
			}
		}
		if preserveFormatting, ok := req.Metadata["preserve_formatting"]; ok {
			if preserveFormattingStr, ok := preserveFormatting.(string); ok && preserveFormattingStr == "true" {
				params.Set("preserve_formatting", "1")
			}
		}
		if tagHandling, ok := req.Metadata["tag_handling"]; ok {
			if tagHandlingStr, ok := tagHandling.(string); ok {
				params.Set("tag_handling", tagHandlingStr)
			}
		}
	}

	// 执行翻译
	resp, err := p.translate(ctx, params)
	if err != nil {
		return nil, err
	}

	if len(resp.Translations) == 0 {
		return nil, fmt.Errorf("no translation returned")
	}

	// 返回响应
	metadata := make(map[string]interface{})
	if resp.Translations[0].DetectedSourceLanguage != "" {
		metadata["detected_source"] = resp.Translations[0].DetectedSourceLanguage
	}

	return &providers.ProviderResponse{
		Text:     resp.Translations[0].Text,
		Metadata: metadata,
	}, nil
}

// GetName 获取提供商名称
func (p *Provider) GetName() string {
	return "deepl"
}

// SupportsSteps 不支持多步骤翻译
func (p *Provider) SupportsSteps() bool {
	return false
}

// GetCapabilities 获取提供商能力
func (p *Provider) GetCapabilities() providers.Capabilities {
	return providers.Capabilities{
		SupportedLanguages: []providers.Language{
			{Code: "BG", Name: "Bulgarian"},
			{Code: "CS", Name: "Czech"},
			{Code: "DA", Name: "Danish"},
			{Code: "DE", Name: "German"},
			{Code: "EL", Name: "Greek"},
			{Code: "EN", Name: "English"},
			{Code: "EN-GB", Name: "English (British)"},
			{Code: "EN-US", Name: "English (American)"},
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
			{Code: "NB", Name: "Norwegian (Bokmål)"},
			{Code: "NL", Name: "Dutch"},
			{Code: "PL", Name: "Polish"},
			{Code: "PT", Name: "Portuguese"},
			{Code: "PT-BR", Name: "Portuguese (Brazilian)"},
			{Code: "PT-PT", Name: "Portuguese (European)"},
			{Code: "RO", Name: "Romanian"},
			{Code: "RU", Name: "Russian"},
			{Code: "SK", Name: "Slovak"},
			{Code: "SL", Name: "Slovenian"},
			{Code: "SV", Name: "Swedish"},
			{Code: "TR", Name: "Turkish"},
			{Code: "UK", Name: "Ukrainian"},
			{Code: "ZH", Name: "Chinese"},
		},
		MaxTextLength:      130000, // DeepL Pro限制
		SupportsBatch:      true,
		SupportsFormatting: true,
		RequiresAPIKey:     true,
		RateLimit: &providers.RateLimit{
			CharactersPerDay: 500000, // 免费版限制
		},
	}
}

// HealthCheck 健康检查
func (p *Provider) HealthCheck(ctx context.Context) error {
	// 检查使用量
	req, err := http.NewRequestWithContext(ctx, "GET",
		p.config.APIEndpoint+"/usage", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "DeepL-Auth-Key "+p.config.APIKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: %s", resp.Status)
	}

	return nil
}

// translate 执行翻译请求
func (p *Provider) translate(ctx context.Context, params url.Values) (*TranslateResponse, error) {
	// 创建请求
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.config.APIEndpoint+"/translate",
		strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置头部
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Authorization", "DeepL-Auth-Key "+p.config.APIKey)
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

		// 处理特定错误码
		switch resp.StatusCode {
		case 400:
			lastErr = fmt.Errorf("bad request: %s", string(errBody))
		case 403:
			lastErr = fmt.Errorf("authentication failed")
		case 404:
			lastErr = fmt.Errorf("requested resource not found")
		case 413:
			lastErr = fmt.Errorf("request size exceeded")
		case 414:
			lastErr = fmt.Errorf("request URI too long")
		case 429:
			lastErr = fmt.Errorf("too many requests")
		case 456:
			lastErr = fmt.Errorf("quota exceeded")
		case 503:
			lastErr = fmt.Errorf("service temporarily unavailable")
		default:
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

// normalizeLanguageCode 标准化语言代码为DeepL格式
func normalizeLanguageCode(lang string, isSource bool) string {
	// DeepL使用大写的语言代码
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

	// 对于英语和葡萄牙语，目标语言需要指定变体
	if !isSource {
		switch upper {
		case "EN":
			return "EN-US" // 默认美式英语
		case "PT":
			return "PT-BR" // 默认巴西葡萄牙语
		}
	}

	// 处理 xx_YY 格式到 XX-YY
	if strings.Contains(upper, "_") {
		parts := strings.Split(upper, "_")
		if len(parts) == 2 {
			return parts[0] + "-" + parts[1]
		}
	}

	return upper
}

// TranslateResponse 翻译响应
type TranslateResponse struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	} `json:"translations"`
}
