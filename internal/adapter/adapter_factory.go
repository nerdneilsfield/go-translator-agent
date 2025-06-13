package adapter

import (
	"fmt"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
)

// AdapterType 适配器类型
type AdapterType string

const (
	// ChainAdapterType 链式适配器（默认）
	ChainAdapterType AdapterType = "chain"
	// ThreeStepAdapterType 三步翻译适配器
	ThreeStepAdapterType AdapterType = "three-step"
)

// CreateTranslatorAdapter 创建翻译器适配器
func CreateTranslatorAdapter(cfg *config.Config, adapterType AdapterType) (translator.Translator, error) {
	switch adapterType {
	case ThreeStepAdapterType:
		return NewThreeStepTranslatorAdapter(cfg)
	case ChainAdapterType:
		return NewTranslatorAdapter(cfg)
	default:
		// 默认使用链式适配器
		return NewTranslatorAdapter(cfg)
	}
}

// CreateDefaultTranslatorAdapter 创建默认的翻译器适配器
func CreateDefaultTranslatorAdapter(cfg *config.Config) (translator.Translator, error) {
	// 检查配置中是否指定了适配器类型
	adapterType := ChainAdapterType
	
	// 可以从配置中读取适配器类型
	if cfg.Metadata != nil {
		if at, ok := cfg.Metadata["adapter_type"].(string); ok {
			adapterType = AdapterType(at)
		}
	}
	
	// 如果使用了保护块相关的配置，优先使用三步适配器
	if hasProtectionBlocks(cfg) {
		adapterType = ThreeStepAdapterType
	}
	
	return CreateTranslatorAdapter(cfg, adapterType)
}

// hasProtectionBlocks 检查是否配置了保护块
func hasProtectionBlocks(cfg *config.Config) bool {
	// 检查是否有保护块相关的配置
	if cfg.Metadata != nil {
		if preservePatterns, ok := cfg.Metadata["preserve_patterns"].([]string); ok && len(preservePatterns) > 0 {
			return true
		}
		if preserveEnabled, ok := cfg.Metadata["preserve_enabled"].(bool); ok && preserveEnabled {
			return true
		}
	}
	
	// 检查是否有推理模型（需要保护块支持）
	if stepSet, exists := cfg.StepSets[cfg.ActiveStepSet]; exists {
		modelNames := []string{
			stepSet.InitialTranslation.ModelName,
			stepSet.Reflection.ModelName,
			stepSet.Improvement.ModelName,
		}
		
		for _, modelName := range modelNames {
			if modelConfig, ok := cfg.ModelConfigs[modelName]; ok && modelConfig.IsReasoning {
				return true
			}
		}
	}
	
	return false
}

// AdapterInfo 适配器信息
type AdapterInfo struct {
	Type        AdapterType
	Name        string
	Description string
	Features    []string
}

// GetAvailableAdapters 获取可用的适配器列表
func GetAvailableAdapters() []AdapterInfo {
	return []AdapterInfo{
		{
			Type:        ChainAdapterType,
			Name:        "Chain Adapter",
			Description: "基于责任链模式的翻译适配器，支持灵活的步骤配置",
			Features: []string{
				"支持动态步骤配置",
				"支持混合提供商",
				"支持缓存",
				"向后兼容旧配置",
			},
		},
		{
			Type:        ThreeStepAdapterType,
			Name:        "Three-Step Adapter",
			Description: "专门的三步翻译适配器，支持保护块和推理模型",
			Features: []string{
				"完整的三步翻译流程",
				"内置保护块支持",
				"推理模型思考过程移除",
				"优化的提示词构建",
			},
		},
	}
}