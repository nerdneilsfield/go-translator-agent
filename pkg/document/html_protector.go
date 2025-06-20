package document


// HTMLProtector HTML格式的内容保护器
type HTMLProtector struct {
	*BaseProtector
}

// NewHTMLProtector 创建HTML保护器
func NewHTMLProtector() *HTMLProtector {
	return &HTMLProtector{
		BaseProtector: NewBaseProtector("html"),
	}
}

// ProtectContent 保护HTML内容
func (hp *HTMLProtector) ProtectContent(text string, pp PatternProtector) string {
	// 先应用通用保护
	text = hp.ProtectCommonContent(text, pp)
	
	// === HTML特有保护 ===
	
	// 1. Script标签（完整保护）
	text = pp.ProtectPattern(text, `(?s)<script[^>]*>.*?</script>`)
	
	// 2. Style标签（完整保护）
	text = pp.ProtectPattern(text, `(?s)<style[^>]*>.*?</style>`)
	
	// 3. 代码相关标签
	text = pp.ProtectPattern(text, `(?s)<pre[^>]*>.*?</pre>`)
	text = pp.ProtectPattern(text, `(?s)<code[^>]*>.*?</code>`)
	
	// 4. 表格结构保护
	text = pp.ProtectPattern(text, `(?s)<table[^>]*>.*?</table>`)
	
	// 5. 表单元素保护
	text = pp.ProtectPattern(text, `(?s)<form[^>]*>.*?</form>`)
	text = pp.ProtectPattern(text, `<input[^>]*>`)
	text = pp.ProtectPattern(text, `<button[^>]*>.*?</button>`)
	
	// 6. 媒体标签保护
	text = pp.ProtectPattern(text, `<img[^>]*>`)
	text = pp.ProtectPattern(text, `(?s)<video[^>]*>.*?</video>`)
	text = pp.ProtectPattern(text, `(?s)<audio[^>]*>.*?</audio>`)
	
	// 7. 元数据标签保护
	text = pp.ProtectPattern(text, `<meta[^>]*>`)
	text = pp.ProtectPattern(text, `<link[^>]*>`)
	
	// 8. 页面锚点和导航元素保护（新增）
	text = hp.protectPageAnchors(text, pp)
	text = hp.protectNavigationElements(text, pp)
	
	// 9. SVG和数学公式保护（新增）
	text = pp.ProtectPattern(text, `(?s)<svg[^>]*>.*?</svg>`)
	text = pp.ProtectPattern(text, `(?s)<math[^>]*>.*?</math>`)
	
	// 10. HTML属性保护
	text = pp.ProtectPattern(text, `\s+[a-zA-Z-]+="[^"]*"`)
	text = pp.ProtectPattern(text, `\s+[a-zA-Z-]+='[^']*'`)
	
	// 11. HTML实体
	text = pp.ProtectPattern(text, `&[a-zA-Z]+;`)
	text = pp.ProtectPattern(text, `&#\d+;`)
	text = pp.ProtectPattern(text, `&#x[0-9a-fA-F]+;`)
	
	// 12. 注释保护
	text = pp.ProtectPattern(text, `(?s)<!--.*?-->`)
	
	return text
}

// protectPageAnchors 保护页面锚点
func (hp *HTMLProtector) protectPageAnchors(text string, pp PatternProtector) string {
	// 保护页面锚点（如 <a class="page" id="p59"/>）
	text = pp.ProtectPattern(text, `<a\s+class=["']page["'][^>]*/>`)
	text = pp.ProtectPattern(text, `<a\s+[^>]*class=["'][^"']*page[^"']*["'][^>]*/>`)
	
	// 保护锚点元素（如 <a class="anchor" id="section1"/>）
	text = pp.ProtectPattern(text, `<a\s+class=["']anchor["'][^>]*/>`)
	text = pp.ProtectPattern(text, `<a\s+[^>]*class=["'][^"']*anchor[^"']*["'][^>]*/>`)
	
	// 保护ID锚点（如 <a id="toc" name="toc"/>）
	text = pp.ProtectPattern(text, `<a\s+id=["'][^"']+["'][^>]*/>`)
	text = pp.ProtectPattern(text, `<a\s+name=["'][^"']+["'][^>]*/>`)
	
	return text
}

// protectNavigationElements 保护导航元素
func (hp *HTMLProtector) protectNavigationElements(text string, pp PatternProtector) string {
	// 保护交叉引用链接（如 <a class="xref">）
	text = pp.ProtectPattern(text, `<a\s+class=["']xref["'][^>]*>.*?</a>`)
	text = pp.ProtectPattern(text, `<a\s+[^>]*class=["'][^"']*xref[^"']*["'][^>]*>.*?</a>`)
	
	// 保护导航菜单
	text = pp.ProtectPattern(text, `(?s)<nav[^>]*>.*?</nav>`)
	text = pp.ProtectPattern(text, `(?s)<div\s+class=["']nav[^"']*["'][^>]*>.*?</div>`)
	
	// 保护目录结构
	text = pp.ProtectPattern(text, `(?s)<div\s+class=["']toc[^"']*["'][^>]*>.*?</div>`)
	text = pp.ProtectPattern(text, `(?s)<div\s+id=["']toc["'][^>]*>.*?</div>`)
	
	// 保护书签和引用
	text = pp.ProtectPattern(text, `<a\s+href=["']#[^"']*["'][^>]*>.*?</a>`)
	
	return text
}

// RestoreContent 恢复保护的HTML内容
func (hp *HTMLProtector) RestoreContent(text string, pp PatternProtector) string {
	// 使用PatternProtector的Restore方法恢复所有占位符
	return pp.Restore(text)
}

// GetProtectedPatterns 获取保护的模式列表
func (hp *HTMLProtector) GetProtectedPatterns() []string {
	patterns := hp.GetCommonPatterns()
	htmlPatterns := []string{
		"Script tags",
		"Style tags", 
		"Pre and code tags",
		"Table structures",
		"Form elements",
		"Media tags (img, video, audio)",
		"Meta and link tags",
		"Page anchors and navigation elements",
		"SVG and math formulas",
		"HTML attributes",
		"HTML entities",
		"HTML comments",
	}
	return append(patterns, htmlPatterns...)
}

