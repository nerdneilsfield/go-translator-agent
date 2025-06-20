package document

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// HTMLPlaceholderManager 管理HTML内容保护的占位符系统
// 专门用于internal/document包内的HTML翻译增强功能
type HTMLPlaceholderManager struct {
	mu           sync.Mutex
	placeholders map[string]string // 占位符 -> 原始内容
	counter      int               // 占位符计数器
	prefix       string            // 占位符前缀
}

// NewHTMLPlaceholderManager 创建新的HTML占位符管理器
func NewHTMLPlaceholderManager() *HTMLPlaceholderManager {
	return &HTMLPlaceholderManager{
		placeholders: make(map[string]string),
		counter:      0,
		prefix:       "@@PROTECTED_",
	}
}

// Protect 保护内容，返回占位符
func (hpm *HTMLPlaceholderManager) Protect(content string) string {
	if content == "" {
		return content
	}

	hpm.mu.Lock()
	defer hpm.mu.Unlock()

	placeholder := fmt.Sprintf("%s%d@@", hpm.prefix, hpm.counter)
	hpm.placeholders[placeholder] = content
	hpm.counter++

	return placeholder
}

// ProtectWithAttributes 保护HTML元素，包含属性信息
func (hpm *HTMLPlaceholderManager) ProtectWithAttributes(tagName, attributes, content string) string {
	if tagName == "" {
		return content
	}

	var fullHTML string
	if content != "" {
		fullHTML = fmt.Sprintf("<%s%s>%s</%s>", tagName, attributes, content, tagName)
	} else {
		// 自闭合标签
		if attributes != "" {
			fullHTML = fmt.Sprintf("<%s%s/>", tagName, attributes)
		} else {
			fullHTML = fmt.Sprintf("<%s/>", tagName)
		}
	}

	return hpm.Protect(fullHTML)
}

// Restore 恢复所有占位符
func (hpm *HTMLPlaceholderManager) Restore(text string) string {
	hpm.mu.Lock()
	defer hpm.mu.Unlock()

	result := text
	for placeholder, original := range hpm.placeholders {
		result = strings.ReplaceAll(result, placeholder, original)
	}

	return result
}

// RestoreSpecific 恢复特定的占位符
func (hpm *HTMLPlaceholderManager) RestoreSpecific(text string, placeholderIDs []int) string {
	hpm.mu.Lock()
	defer hpm.mu.Unlock()

	result := text
	for _, id := range placeholderIDs {
		placeholder := fmt.Sprintf("%s%d@@", hpm.prefix, id)
		if original, exists := hpm.placeholders[placeholder]; exists {
			result = strings.ReplaceAll(result, placeholder, original)
		}
	}

	return result
}

// GetPlaceholderCount 获取当前占位符数量
func (hpm *HTMLPlaceholderManager) GetPlaceholderCount() int {
	hpm.mu.Lock()
	defer hpm.mu.Unlock()
	return len(hpm.placeholders)
}

// Clear 清空所有占位符
func (hpm *HTMLPlaceholderManager) Clear() {
	hpm.mu.Lock()
	defer hpm.mu.Unlock()

	hpm.placeholders = make(map[string]string)
	hpm.counter = 0
}

// HasPlaceholders 检查文本中是否包含占位符
func (hpm *HTMLPlaceholderManager) HasPlaceholders(text string) bool {
	pattern := regexp.MustCompile(regexp.QuoteMeta(hpm.prefix) + `\d+@@`)
	return pattern.MatchString(text)
}

// GetPlaceholderInfo 获取占位符的详细信息
func (hpm *HTMLPlaceholderManager) GetPlaceholderInfo() map[string]string {
	hpm.mu.Lock()
	defer hpm.mu.Unlock()

	// 返回副本以避免并发修改
	info := make(map[string]string)
	for k, v := range hpm.placeholders {
		info[k] = v
	}
	return info
}

// ExtractPlaceholderIDs 从文本中提取所有占位符ID
func (hpm *HTMLPlaceholderManager) ExtractPlaceholderIDs(text string) []int {
	pattern := regexp.MustCompile(regexp.QuoteMeta(hpm.prefix) + `(\d+)@@`)
	matches := pattern.FindAllStringSubmatch(text, -1)
	
	var ids []int
	for _, match := range matches {
		if len(match) > 1 {
			if id, err := strconv.Atoi(match[1]); err == nil {
				ids = append(ids, id)
			}
		}
	}
	
	return ids
}

// IsProtectedContent 检查内容是否为受保护的内容
func (hpm *HTMLPlaceholderManager) IsProtectedContent(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}

	// 检查是否完全匹配占位符模式
	pattern := regexp.MustCompile("^" + regexp.QuoteMeta(hpm.prefix) + `\d+@@$`)
	return pattern.MatchString(text)
}

// GetOriginalContent 获取占位符对应的原始内容
func (hpm *HTMLPlaceholderManager) GetOriginalContent(placeholder string) (string, bool) {
	hpm.mu.Lock()
	defer hpm.mu.Unlock()

	content, exists := hpm.placeholders[placeholder]
	return content, exists
}