package raw

import (
	"context"

	"github.com/nerdneilsfield/go-translator-agent/pkg/providers"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// Config Raw 提供商配置（实际上不需要任何配置）
type Config struct {
	providers.BaseConfig
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		BaseConfig: providers.DefaultConfig(),
	}
}

// Provider Raw 提供商实现（跳过翻译，直接返回原文）
type Provider struct {
	config Config
}

// New 创建新的 Raw 提供商
func New(config Config) *Provider {
	return &Provider{
		config: config,
	}
}

// Configure 配置提供商
func (p *Provider) Configure(config interface{}) error {
	// Raw 提供商不需要配置
	return nil
}

// Translate 执行翻译（直接返回原文）
func (p *Provider) Translate(ctx context.Context, req *translation.ProviderRequest) (*translation.ProviderResponse, error) {
	// Raw 提供商直接返回原文，不进行任何翻译
	return &translation.ProviderResponse{
		Text:      req.Text,
		Model:     "raw",
		TokensIn:  0,
		TokensOut: 0,
		Metadata: map[string]string{
			"type": "raw_passthrough",
		},
	}, nil
}

// GetName 获取提供商名称
func (p *Provider) GetName() string {
	return "raw"
}

// SupportsSteps 支持多步骤翻译
func (p *Provider) SupportsSteps() bool {
	return true
}

// GetCapabilities 获取提供商能力
func (p *Provider) GetCapabilities() providers.Capabilities {
	return providers.Capabilities{
		SupportedLanguages: []providers.Language{
			// Raw 支持所有语言（因为不进行实际翻译）
			{Code: "*", Name: "All Languages"},
		},
		MaxTextLength:      1000000, // 无限制
		SupportsBatch:      true,
		SupportsFormatting: true,
		RequiresAPIKey:     false,
		RateLimit:          nil, // 无速率限制
	}
}

// HealthCheck 健康检查
func (p *Provider) HealthCheck(ctx context.Context) error {
	// Raw 提供商总是健康的
	return nil
}