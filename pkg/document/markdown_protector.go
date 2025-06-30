package document


// MarkdownProtector Markdown格式的内容保护器
type MarkdownProtector struct {
	*BaseProtector
}

// NewMarkdownProtector 创建Markdown保护器
func NewMarkdownProtector() *MarkdownProtector {
	return &MarkdownProtector{
		BaseProtector: NewBaseProtector("markdown"),
	}
}

// ProtectContent 保护Markdown内容
func (mp *MarkdownProtector) ProtectContent(text string, pp PatternProtector) string {
	// 先应用通用保护
	text = mp.ProtectCommonContent(text, pp)
	
	// === Markdown特有保护 ===
	
	// 1. 代码块保护（用户特别要求）
	text = mp.protectCodeBlocks(text, pp)
	
	// 2. LaTeX公式保护
	text = mp.protectLatexFormulas(text, pp)
	
	// 3. Markdown表格保护
	text = mp.protectMarkdownTables(text, pp)
	
	// 4. 图片和链接保护
	text = mp.protectImagesAndLinks(text, pp)
	
	// 5. HTML标签保护（Markdown中可能包含HTML）
	text = mp.protectHTMLTags(text, pp)
	
	// 6. 预处理标记保护（重要！）
	text = mp.protectPreprocessingMarkers(text, pp)
	
	// 7. 文献引用保护
	text = mp.protectCitations(text, pp)
	
	return text
}

// protectCodeBlocks 保护代码块
func (mp *MarkdownProtector) protectCodeBlocks(text string, pp PatternProtector) string {
	// Fenced code blocks (```)
	text = pp.ProtectPattern(text, "(?s)```[^\\n]*\\n.*?\\n```")
	
	// Indented code blocks (4+ spaces)
	text = pp.ProtectPattern(text, "(?m)^(?: {4}|\\t)+.*$")
	
	// Inline code (`code`)
	text = pp.ProtectPattern(text, "`[^`]+`")
	
	return text
}

// protectLatexFormulas 保护LaTeX公式
func (mp *MarkdownProtector) protectLatexFormulas(text string, pp PatternProtector) string {
	// 块级公式 $$...$$
	text = pp.ProtectPattern(text, `(?s)\$\$[^$]*?\$\$`)
	
	// 行内公式 $...$
	text = pp.ProtectPattern(text, `\$[^$\n]*?\$`)
	
	// LaTeX环境 \[...\] 和 \(...\)
	text = pp.ProtectPattern(text, `(?s)\\\[[^\]]*?\\\]`)
	text = pp.ProtectPattern(text, `(?s)\\\([^\)]*?\\\)`)
	
	return text
}

// protectMarkdownTables 保护Markdown表格
func (mp *MarkdownProtector) protectMarkdownTables(text string, pp PatternProtector) string {
	// 完整的Markdown表格
	text = pp.ProtectPattern(text, `(?m)^\|.*\|[ \t]*$\n^\|[ \t]*[-:| ]+[ \t]*\|[ \t]*$(?:\n^\|.*\|[ \t]*$)*`)
	
	return text
}

// protectImagesAndLinks 保护图片和链接
func (mp *MarkdownProtector) protectImagesAndLinks(text string, pp PatternProtector) string {
	// 单行图片（独立成段）
	text = pp.ProtectPattern(text, `(?m)^[ \t]*!\[[^\]]*\]\([^)]+\)[ \t]*$`)
	
	// 图片引用语法
	text = pp.ProtectPattern(text, `!\[[^\]]*\]\[[^\]]*\]`)
	
	// 链接引用定义
	text = pp.ProtectPattern(text, `(?m)^\s*\[[^\]]+\]:\s*\S+.*$`)
	
	return text
}

// protectHTMLTags 保护HTML标签
func (mp *MarkdownProtector) protectHTMLTags(text string, pp PatternProtector) string {
	// HTML表格
	text = pp.ProtectPattern(text, `(?s)<table[^>]*>.*?</table>`)
	
	// Script和Style标签（完整保护）
	text = pp.ProtectPattern(text, `(?s)<script[^>]*>.*?</script>`)
	text = pp.ProtectPattern(text, `(?s)<style[^>]*>.*?</style>`)
	
	// 其他HTML标签
	text = pp.ProtectPattern(text, `<[^>]+>`)
	
	// HTML实体
	text = pp.ProtectPattern(text, `&[a-zA-Z]+;`)
	text = pp.ProtectPattern(text, `&#\d+;`)
	
	return text
}

// protectPreprocessingMarkers 保护预处理标记
func (mp *MarkdownProtector) protectPreprocessingMarkers(text string, pp PatternProtector) string {
	// TABLE_PROTECTED标记
	text = pp.ProtectPattern(text, `(?s)<!-- TABLE_PROTECTED -->.*?<!-- /TABLE_PROTECTED -->`)
	
	// REFERENCES_PROTECTED标记
	text = pp.ProtectPattern(text, `(?s)<!-- REFERENCES_PROTECTED -->.*?<!-- /REFERENCES_PROTECTED -->`)
	
	// 通用保护标记
	text = pp.ProtectPattern(text, `(?s)<!-- [A-Z_]+_PROTECTED -->.*?<!-- /[A-Z_]+_PROTECTED -->`)
	
	return text
}

// protectCitations 保护文献引用
func (mp *MarkdownProtector) protectCitations(text string, pp PatternProtector) string {
	// 数字引用 [1], [2], [1-3], [1,2,3]
	text = pp.ProtectPattern(text, `\[[0-9]+([-,][0-9]+)*\]`)
	
	return text
}

// RestoreContent 恢复保护的Markdown内容
func (mp *MarkdownProtector) RestoreContent(text string, pp PatternProtector) string {
	// 使用PatternProtector的Restore方法恢复所有占位符
	return pp.Restore(text)
}

// GetProtectedPatterns 获取保护的模式列表
func (mp *MarkdownProtector) GetProtectedPatterns() []string {
	patterns := mp.GetCommonPatterns()
	markdownPatterns := []string{
		"Fenced code blocks (```)",
		"Indented code blocks",
		"Inline code (`code`)",
		"LaTeX formulas ($...$ and $$...$$)",
		"Markdown tables",
		"Standalone images",
		"HTML tags and entities",
		"Preprocessing markers",
		"Citations [1], [2]",
	}
	return append(patterns, markdownPatterns...)
}