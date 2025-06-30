package document

import (
	"context"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
)

// HTMLAttributeTranslator HTML属性翻译器
// 专门处理HTML元素属性的翻译，参考html_utils.go的实现
type HTMLAttributeTranslator struct {
	logger             *zap.Logger
	placeholderManager *HTMLPlaceholderManager
	attributeConfig    AttributeTranslationConfig
}

// AttributeTranslationConfig 属性翻译配置
type AttributeTranslationConfig struct {
	// 需要翻译的属性列表
	TranslatableAttributes []string
	// 按元素类型的属性映射
	ElementAttributeMap map[string][]string
	// 属性值的预处理规则
	PreprocessRules map[string]func(string) string
	// 属性值的后处理规则
	PostprocessRules map[string]func(string) string
	// 跳过空值
	SkipEmptyValues bool
	// 最小长度要求
	MinLength int
	// 最大长度限制
	MaxLength int
}

// DefaultAttributeTranslationConfig 默认属性翻译配置
func DefaultAttributeTranslationConfig() AttributeTranslationConfig {
	return AttributeTranslationConfig{
		TranslatableAttributes: []string{
			"alt", "title", "placeholder", "aria-label", "aria-description",
			"aria-placeholder", "data-tooltip", "data-title", "label",
			"summary", "caption", "value", // for input[type=submit/button]
		},
		ElementAttributeMap: map[string][]string{
			"img":      {"alt", "title"},
			"input":    {"placeholder", "title", "aria-label", "value"},
			"textarea": {"placeholder", "title", "aria-label"},
			"button":   {"title", "aria-label", "value"},
			"a":        {"title", "aria-label"},
			"abbr":     {"title"},
			"acronym":  {"title"},
			"th":       {"title", "aria-label"},
			"td":       {"title", "aria-label"},
			"div":      {"title", "aria-label", "data-tooltip"},
			"span":     {"title", "aria-label", "data-tooltip"},
			"p":        {"title", "aria-label"},
			"table":    {"summary", "title"},
			"details":  {"title", "aria-label"},
			"summary":  {"title", "aria-label"},
		},
		PreprocessRules: map[string]func(string) string{
			"default": func(s string) string {
				return strings.TrimSpace(s)
			},
		},
		PostprocessRules: map[string]func(string) string{
			"default": func(s string) string {
				return strings.TrimSpace(s)
			},
		},
		SkipEmptyValues: true,
		MinLength:       1,
		MaxLength:       500,
	}
}

// AttributeTranslationInfo 属性翻译信息
type AttributeTranslationInfo struct {
	Element       *goquery.Selection // HTML元素
	AttributeName string             // 属性名
	OriginalValue string             // 原始值
	ProcessedValue string            // 预处理后的值
	TranslatedValue string           // 翻译后的值
	Path          string             // DOM路径
	ElementTag    string             // 元素标签
	CanTranslate  bool               // 是否可翻译
	Priority      int                // 翻译优先级
}

// NewHTMLAttributeTranslator 创建HTML属性翻译器
func NewHTMLAttributeTranslator(logger *zap.Logger, config AttributeTranslationConfig) *HTMLAttributeTranslator {
	return &HTMLAttributeTranslator{
		logger:             logger,
		placeholderManager: NewHTMLPlaceholderManager(),
		attributeConfig:    config,
	}
}

// ExtractTranslatableAttributes 提取可翻译的属性
func (t *HTMLAttributeTranslator) ExtractTranslatableAttributes(doc *goquery.Document) ([]*AttributeTranslationInfo, error) {
	var attributes []*AttributeTranslationInfo

	// 遍历所有元素
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		elementTag := t.getElementTag(s)
		path := fmt.Sprintf("element[%d]", i+1)

		// 获取该元素支持的属性列表
		supportedAttrs := t.getSupportedAttributes(elementTag)

		for _, attrName := range supportedAttrs {
			if attrValue, exists := s.Attr(attrName); exists {
				attrInfo := t.processAttribute(s, attrName, attrValue, path, elementTag)
				if attrInfo != nil && attrInfo.CanTranslate {
					attributes = append(attributes, attrInfo)
				}
			}
		}
	})

	t.logger.Debug("extracted translatable attributes",
		zap.Int("totalAttributes", len(attributes)),
		zap.Int("elements", t.countUniqueElements(attributes)))

	return attributes, nil
}

// processAttribute 处理单个属性
func (t *HTMLAttributeTranslator) processAttribute(element *goquery.Selection, attrName, attrValue, path, elementTag string) *AttributeTranslationInfo {
	// 预处理属性值
	processedValue := t.preprocessAttributeValue(attrName, attrValue)

	// 检查是否应跳空值
	if t.attributeConfig.SkipEmptyValues && processedValue == "" {
		return nil
	}

	// 检查长度限制
	if len(processedValue) < t.attributeConfig.MinLength || len(processedValue) > t.attributeConfig.MaxLength {
		return nil
	}

	// 检查是否应该翻译
	canTranslate := t.shouldTranslateAttribute(element, attrName, processedValue)

	attrInfo := &AttributeTranslationInfo{
		Element:        element,
		AttributeName:  attrName,
		OriginalValue:  attrValue,
		ProcessedValue: processedValue,
		Path:           fmt.Sprintf("%s/@%s", path, attrName),
		ElementTag:     elementTag,
		CanTranslate:   canTranslate,
		Priority:       t.getAttributePriority(attrName, elementTag),
	}

	return attrInfo
}

// shouldTranslateAttribute 检查是否应该翻译属性
func (t *HTMLAttributeTranslator) shouldTranslateAttribute(element *goquery.Selection, attrName, value string) bool {
	// 检查元素的translate属性
	if translateVal, exists := element.Attr("translate"); exists {
		if strings.ToLower(translateVal) == "no" {
			return false
		}
	}

	// 检查父元素的translate属性
	parent := element.Parent()
	for parent.Length() > 0 {
		if translateVal, exists := parent.Attr("translate"); exists {
			if strings.ToLower(translateVal) == "no" {
				return false
			}
		}
		parent = parent.Parent()
	}

	// 特殊属性检查
	switch attrName {
	case "value":
		// 只有按钮类型的input才翻译value
		inputType, _ := element.Attr("type")
		return strings.ToLower(inputType) == "submit" || strings.ToLower(inputType) == "button"
	case "aria-label", "aria-description":
		// 检查是否已有其他可见文本
		if strings.TrimSpace(element.Text()) != "" {
			return false // 如果有可见文本，优先级较低
		}
	}

	// 检查是否包含URL或特殊标识符
	if t.looksLikeURL(value) || t.looksLikeIdentifier(value) {
		return false
	}

	return true
}

// looksLikeURL 检查值是否像URL
func (t *HTMLAttributeTranslator) looksLikeURL(value string) bool {
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "ftp://") ||
		strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "#") ||
		strings.HasPrefix(lower, "/")
}

// looksLikeIdentifier 检查值是否像标识符
func (t *HTMLAttributeTranslator) looksLikeIdentifier(value string) bool {
	// 检查是否只包含字母、数字、下划线、连字符
	for _, char := range value {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-') {
			return false
		}
	}
	return len(value) > 0 && len(value) < 30 // 标识符通常较短
}

// getSupportedAttributes 获取元素支持的属性
func (t *HTMLAttributeTranslator) getSupportedAttributes(elementTag string) []string {
	// 先检查特定元素的属性
	if attrs, exists := t.attributeConfig.ElementAttributeMap[elementTag]; exists {
		return attrs
	}

	// 返回通用属性列表
	return t.attributeConfig.TranslatableAttributes
}

// getElementTag 获取元素标签名
func (t *HTMLAttributeTranslator) getElementTag(s *goquery.Selection) string {
	if s.Nodes != nil && len(s.Nodes) > 0 {
		return strings.ToLower(s.Nodes[0].Data)
	}
	return ""
}

// preprocessAttributeValue 预处理属性值
func (t *HTMLAttributeTranslator) preprocessAttributeValue(attrName, value string) string {
	// 应用特定属性的预处理规则
	if rule, exists := t.attributeConfig.PreprocessRules[attrName]; exists {
		return rule(value)
	}

	// 应用默认预处理规则
	if rule, exists := t.attributeConfig.PreprocessRules["default"]; exists {
		return rule(value)
	}

	return strings.TrimSpace(value)
}

// postprocessAttributeValue 后处理属性值
func (t *HTMLAttributeTranslator) postprocessAttributeValue(attrName, value string) string {
	// 应用特定属性的后处理规则
	if rule, exists := t.attributeConfig.PostprocessRules[attrName]; exists {
		return rule(value)
	}

	// 应用默认后处理规则
	if rule, exists := t.attributeConfig.PostprocessRules["default"]; exists {
		return rule(value)
	}

	return strings.TrimSpace(value)
}

// getAttributePriority 获取属性翻译优先级
func (t *HTMLAttributeTranslator) getAttributePriority(attrName, elementTag string) int {
	// 定义优先级映射
	priorityMap := map[string]int{
		"alt":              10, // 图片替代文本最重要
		"title":            9,  // 标题很重要
		"placeholder":      8,  // 占位符文本重要
		"aria-label":       7,  // 无障碍标签重要
		"aria-description": 6,  // 无障碍描述
		"value":            5,  // 按钮值
		"summary":          4,  // 表格摘要
		"data-tooltip":     3,  // 工具提示
		"data-title":       2,  // 数据标题
		"label":            1,  // 其他标签
	}

	if priority, exists := priorityMap[attrName]; exists {
		return priority
	}

	return 0 // 默认优先级
}

// TranslateAttributes 翻译属性集合
func (t *HTMLAttributeTranslator) TranslateAttributes(ctx context.Context, attributes []*AttributeTranslationInfo, translator func(context.Context, string) (string, error)) error {
	for _, attr := range attributes {
		if !attr.CanTranslate {
			continue
		}

		translatedValue, err := translator(ctx, attr.ProcessedValue)
		if err != nil {
			t.logger.Warn("failed to translate attribute",
				zap.String("element", attr.ElementTag),
				zap.String("attribute", attr.AttributeName),
				zap.String("value", attr.ProcessedValue),
				zap.Error(err))
			continue
		}

		// 后处理翻译结果
		finalValue := t.postprocessAttributeValue(attr.AttributeName, translatedValue)
		attr.TranslatedValue = finalValue

		// 应用翻译到DOM
		attr.Element.SetAttr(attr.AttributeName, finalValue)

		t.logger.Debug("translated attribute",
			zap.String("element", attr.ElementTag),
			zap.String("attribute", attr.AttributeName),
			zap.String("original", attr.OriginalValue),
			zap.String("translated", finalValue))
	}

	return nil
}

// BatchTranslateAttributes 批量翻译属性
func (t *HTMLAttributeTranslator) BatchTranslateAttributes(ctx context.Context, attributes []*AttributeTranslationInfo, batchTranslator func(context.Context, []string) ([]string, error)) error {
	// 收集需要翻译的文本
	var texts []string
	var translatableAttrs []*AttributeTranslationInfo

	for _, attr := range attributes {
		if attr.CanTranslate {
			texts = append(texts, attr.ProcessedValue)
			translatableAttrs = append(translatableAttrs, attr)
		}
	}

	if len(texts) == 0 {
		return nil
	}

	// 批量翻译
	translatedTexts, err := batchTranslator(ctx, texts)
	if err != nil {
		return fmt.Errorf("batch translation failed: %w", err)
	}

	if len(translatedTexts) != len(texts) {
		return fmt.Errorf("translation count mismatch: expected %d, got %d", len(texts), len(translatedTexts))
	}

	// 应用翻译结果
	for i, attr := range translatableAttrs {
		finalValue := t.postprocessAttributeValue(attr.AttributeName, translatedTexts[i])
		attr.TranslatedValue = finalValue
		attr.Element.SetAttr(attr.AttributeName, finalValue)

		t.logger.Debug("batch translated attribute",
			zap.String("element", attr.ElementTag),
			zap.String("attribute", attr.AttributeName),
			zap.String("original", attr.OriginalValue),
			zap.String("translated", finalValue))
	}

	return nil
}

// countUniqueElements 统计唯一元素数量
func (t *HTMLAttributeTranslator) countUniqueElements(attributes []*AttributeTranslationInfo) int {
	elementSet := make(map[*goquery.Selection]bool)
	for _, attr := range attributes {
		elementSet[attr.Element] = true
	}
	return len(elementSet)
}

// GetTranslationStats 获取翻译统计信息
func (t *HTMLAttributeTranslator) GetTranslationStats(attributes []*AttributeTranslationInfo) map[string]interface{} {
	stats := map[string]interface{}{
		"totalAttributes":      len(attributes),
		"translatableCount":    0,
		"translatedCount":      0,
		"attributeTypeStats":   make(map[string]int),
		"elementTypeStats":     make(map[string]int),
		"priorityStats":        make(map[int]int),
	}

	attrTypeStats := stats["attributeTypeStats"].(map[string]int)
	elementTypeStats := stats["elementTypeStats"].(map[string]int)
	priorityStats := stats["priorityStats"].(map[int]int)

	for _, attr := range attributes {
		if attr.CanTranslate {
			stats["translatableCount"] = stats["translatableCount"].(int) + 1
		}
		if attr.TranslatedValue != "" {
			stats["translatedCount"] = stats["translatedCount"].(int) + 1
		}

		attrTypeStats[attr.AttributeName]++
		elementTypeStats[attr.ElementTag]++
		priorityStats[attr.Priority]++
	}

	return stats
}

// GetPlaceholderManager 获取占位符管理器
func (t *HTMLAttributeTranslator) GetPlaceholderManager() *HTMLPlaceholderManager {
	return t.placeholderManager
}