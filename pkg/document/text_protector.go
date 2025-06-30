package document


// TextProtector 纯文本格式的内容保护器
type TextProtector struct {
	*BaseProtector
}

// NewTextProtector 创建文本保护器
func NewTextProtector() *TextProtector {
	return &TextProtector{
		BaseProtector: NewBaseProtector("text"),
	}
}

// ProtectContent 保护纯文本内容
func (tp *TextProtector) ProtectContent(text string, pp PatternProtector) string {
	// 纯文本主要保护通用内容
	text = tp.ProtectCommonContent(text, pp)
	
	// === 纯文本特有保护（较少） ===
	
	// 1. 代码样式的内容（通过缩进识别）
	text = pp.ProtectPattern(text, "(?m)^(?: {4}|\\t)+.*$")
	
	// 2. 特殊分隔符
	text = pp.ProtectPattern(text, "(?m)^[-=]{3,}$")
	text = pp.ProtectPattern(text, "(?m)^[*]{3,}$")
	
	// 3. ASCII艺术和图表（简单识别）
	text = pp.ProtectPattern(text, "(?m)^[+\\-|\\s]+[+\\-|]$")
	
	return text
}

// RestoreContent 恢复保护的文本内容
func (tp *TextProtector) RestoreContent(text string, pp PatternProtector) string {
	// 使用PatternProtector的Restore方法恢复所有占位符
	return pp.Restore(text)
}

// GetProtectedPatterns 获取保护的模式列表
func (tp *TextProtector) GetProtectedPatterns() []string {
	patterns := tp.GetCommonPatterns()
	textPatterns := []string{
		"Indented code-like content",
		"Separator lines (---, ===, ***)",
		"ASCII art and simple diagrams",
	}
	return append(patterns, textPatterns...)
}

// DefaultProtector 默认保护器，当格式未知时使用
type DefaultProtector struct {
	*BaseProtector
}

// NewDefaultProtector 创建默认保护器
func NewDefaultProtector() *DefaultProtector {
	return &DefaultProtector{
		BaseProtector: NewBaseProtector("default"),
	}
}

// ProtectContent 保护内容（使用保守策略）
func (dp *DefaultProtector) ProtectContent(text string, pp PatternProtector) string {
	// 只应用最基本的通用保护
	return dp.ProtectCommonContent(text, pp)
}

// RestoreContent 恢复保护的内容（默认策略）
func (dp *DefaultProtector) RestoreContent(text string, pp PatternProtector) string {
	// 使用PatternProtector的Restore方法恢复所有占位符
	return pp.Restore(text)
}

// GetProtectedPatterns 获取保护的模式列表
func (dp *DefaultProtector) GetProtectedPatterns() []string {
	return dp.GetCommonPatterns()
}