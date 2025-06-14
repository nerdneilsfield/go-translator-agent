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

// GetPreservePrompt 获取保护块相关的 prompt 说明
func GetPreservePrompt(config PreserveConfig) string {
	if !config.Enabled {
		return ""
	}

	// 构建正则模式
	pattern := regexp.QuoteMeta(config.Prefix) + `\d+` + regexp.QuoteMeta(config.Suffix)

	return fmt.Sprintf(`
IMPORTANT: Preserve Markers
- Do not translate or modify any text that matches the pattern: %s
- These markers protect content that should not be translated (code blocks, formulas, etc.)
- Keep these markers and their content exactly as they are in your output.
- Example: %s0%s should remain unchanged.`,
		pattern,
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
