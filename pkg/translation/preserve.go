package translation

import (
	"fmt"
	"regexp"
	"strings"
)

// PreserveConfig 保护块配置
type PreserveConfig struct {
	// 是否启用保护块
	Enabled bool
	// 保护块前缀
	Prefix string
	// 保护块后缀（可选）
	Suffix string
}

// DefaultPreserveConfig 默认保护块配置
var DefaultPreserveConfig = PreserveConfig{
	Enabled: true,
	Prefix:  "@@PRESERVE_",
	Suffix:  "@@",
}

// PreserveManager 保护块管理器
type PreserveManager struct {
	config PreserveConfig
	// 占位符计数器
	counter int
	// 替换映射
	replacements map[string]string
}

// NewPreserveManager 创建保护块管理器
func NewPreserveManager(config PreserveConfig) *PreserveManager {
	return &PreserveManager{
		config:       config,
		counter:      0,
		replacements: make(map[string]string),
	}
}

// GeneratePlaceholder 生成占位符
func (pm *PreserveManager) GeneratePlaceholder() string {
	placeholder := fmt.Sprintf("%s%d%s", pm.config.Prefix, pm.counter, pm.config.Suffix)
	pm.counter++
	return placeholder
}

// Protect 保护指定内容，返回占位符
func (pm *PreserveManager) Protect(content string) string {
	placeholder := pm.GeneratePlaceholder()
	pm.replacements[placeholder] = content
	return placeholder
}

// Restore 还原所有占位符
func (pm *PreserveManager) Restore(text string) string {
	// 从后往前还原，避免嵌套问题
	placeholders := make([]string, 0, len(pm.replacements))
	for placeholder := range pm.replacements {
		placeholders = append(placeholders, placeholder)
	}

	// 按占位符编号降序排序
	// 简单实现：直接遍历，因为通常不会有太多占位符
	for i := pm.counter - 1; i >= 0; i-- {
		placeholder := fmt.Sprintf("%s%d%s", pm.config.Prefix, i, pm.config.Suffix)
		if original, ok := pm.replacements[placeholder]; ok {
			text = strings.ReplaceAll(text, placeholder, original)
		}
	}

	return text
}

// ProtectPattern 使用正则表达式保护匹配的内容
func (pm *PreserveManager) ProtectPattern(text string, pattern string) string {
	if !pm.config.Enabled {
		return text
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return text
	}

	return re.ReplaceAllStringFunc(text, func(match string) string {
		return pm.Protect(match)
	})
}

// GetPreservePrompt 获取保护块相关的 prompt 说明
func GetPreservePrompt(config PreserveConfig) string {
	if !config.Enabled {
		return ""
	}

	// 构建正则模式
	pattern := regexp.QuoteMeta(config.Prefix) + `\d+` + regexp.QuoteMeta(config.Suffix)

	return fmt.Sprintf(`
CRITICAL: Protected Content Markers
- NEVER translate, modify, or alter any text matching: %s
- These markers protect mathematical formulas, code blocks, and special content
- MUST preserve these markers EXACTLY as they appear in the input
- MUST maintain their exact position and format in your translation
- Example: "%s0%s" → keep as "%s0%s" (unchanged)
- WARNING: Modifying these markers will break the translation system`,
		pattern,
		config.Prefix, config.Suffix,
		config.Prefix, config.Suffix,
	)
}

// AppendPreservePrompt 向现有 prompt 追加保护块说明
func AppendPreservePrompt(prompt string, config PreserveConfig) string {
	preservePrompt := GetPreservePrompt(config)
	if preservePrompt == "" {
		return prompt
	}

	return prompt + "\n\n" + preservePrompt
}

// RemoveProtectionMarkers 移除所有保护标记，返回纯净的文本用于相似度比较
func (pm *PreserveManager) RemoveProtectionMarkers(text string) string {
	if !pm.config.Enabled {
		return text
	}
	
	// 构建正则模式匹配所有保护标记
	pattern := regexp.QuoteMeta(pm.config.Prefix) + `\d+` + regexp.QuoteMeta(pm.config.Suffix)
	re := regexp.MustCompile(pattern)
	
	// 移除所有保护标记
	cleaned := re.ReplaceAllString(text, "")
	
	// 清理多余的空白字符
	cleaned = strings.TrimSpace(cleaned)
	// 将多个连续的空白字符替换为单个空格
	cleanedRe := regexp.MustCompile(`\s+`)
	cleaned = cleanedRe.ReplaceAllString(cleaned, " ")
	
	return cleaned
}
