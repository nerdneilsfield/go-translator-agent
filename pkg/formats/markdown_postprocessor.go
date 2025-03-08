package formats

import (
	"regexp"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"go.uber.org/zap"
)

// MarkdownPostProcessor 用于处理翻译后的 Markdown 文件
type MarkdownPostProcessor struct {
	config *config.Config
	logger *zap.Logger
}

// NewMarkdownPostProcessor 创建一个新的 Markdown 后处理器
func NewMarkdownPostProcessor(config *config.Config, logger *zap.Logger) *MarkdownPostProcessor {
	return &MarkdownPostProcessor{
		config: config,
		logger: logger,
	}
}

// ProcessMarkdown 处理翻译后的 Markdown 文本
func (p *MarkdownPostProcessor) ProcessMarkdown(text string) string {
	if !p.config.PostProcessMarkdown {
		return text
	}

	p.logger.Info("开始后处理 Markdown 文本")

	// 应用各种修复
	if p.config.FixMathFormulas {
		text = p.fixMathFormulas(text)
		text = p.unifyMathFormulas(text) // 统一数学公式格式
	}

	if p.config.FixTableFormat {
		text = p.fixTableFormat(text)
	}

	if p.config.FixMixedContent {
		text = p.fixMixedContent(text)
	}

	if p.config.FixPicture {
		text = p.fixPicture(text)
	}

	p.logger.Info("Markdown 后处理完成")
	return text
}

// fixPicture 修复图片格式并确保图片放在新行中
func (p *MarkdownPostProcessor) fixPicture(text string) string {
	p.logger.Debug("修复图片格式")

	// 重要：先将翻译时可能被破坏的图片链接格式恢复
	// 处理图片链接被拆分成片段的情况
	imageExtensions := `\.(?:jpg|jpeg|png|gif|webp|svg|bmp|tiff)`
	// 修复：中文标点符号不能直接作为转义序列使用
	brokenImageRegex := regexp.MustCompile(`(!\[.*?])?\(?([^<>]*?(?:images|img|pics|assets|static|resources|public|uploads|media)\/[a-zA-Z0-9_\-\.\/]+` + imageExtensions + `)[)\s\.,，。]`)

	text = brokenImageRegex.ReplaceAllStringFunc(text, func(match string) string {
		parts := brokenImageRegex.FindStringSubmatch(match)

		altText := "图片"
		if parts[1] != "" {
			// 如果有 alt 文本部分，提取它
			altTextMatch := regexp.MustCompile(`!\[(.*?)\]`).FindStringSubmatch(parts[1])
			if len(altTextMatch) > 1 {
				altText = altTextMatch[1]
				if altText == "" {
					altText = "图片"
				}
			}
		}

		// 清理图片URL
		url := parts[2]
		url = cleanImageURL(url)

		// 前后添加空行以确保图片单独成段
		return "\n\n![" + altText + "](" + url + ")\n\n"
	})

	// 处理文本中包含的图片URL（不在标准Markdown格式中）
	embeddedImageRegex := regexp.MustCompile(`([^!\[])([^<>\s\(\)]*?(?:images|img|pics|assets|static|resources|public|uploads|media)\/[a-zA-Z0-9_\-\.\/]+` + imageExtensions + `\)?)`)
	text = embeddedImageRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := embeddedImageRegex.FindStringSubmatch(match)
		prefix := submatch[1] // 保留第一个字符（非图片URL的部分）
		potentialUrl := submatch[2]

		// 清理URL（移除可能的额外括号和标点）
		url := cleanImageURL(potentialUrl)

		// 如果提取出的文本确实看起来像URL，则处理它
		if isLikelyImageURL(url) {
			return prefix + "\n\n![图片](" + url + ")\n\n"
		}
		return match // 如果不像URL，保持原样
	})

	// 处理 ![图片]() 后面紧跟 URL 的情况
	emptyImageTagRegex := regexp.MustCompile(`!\[(.*?)\]\(\s*\)([^<>\s\(\)]*?(?:images|img|pics|assets|static|resources|public|uploads|media)\/[a-zA-Z0-9_\-\.\/]+` + imageExtensions + `\)?)`)
	text = emptyImageTagRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := emptyImageTagRegex.FindStringSubmatch(match)
		alt := submatch[1]
		potentialUrl := submatch[2]

		// 清理URL
		url := cleanImageURL(potentialUrl)

		// 如果alt为空，使用默认值
		if alt == "" || strings.Contains(alt, "[") || strings.Contains(alt, "]") {
			alt = "图片"
		}

		return "\n\n![" + alt + "](" + url + ")\n\n"
	})

	// 处理正常的 Markdown 图片格式 ![alt](url)
	normalPictureRegex := regexp.MustCompile(`!\[(.*?)\]\((.*?)\)`)
	text = normalPictureRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := normalPictureRegex.FindStringSubmatch(match)
		alt := submatch[1]
		url := submatch[2]

		// 清理 URL（移除可能的额外括号）
		url = cleanImageURL(url)

		// 如果URL为空，不做处理
		if url == "" {
			return match
		}

		// 如果 alt 为空或包含异常字符，使用默认值
		if alt == "" || strings.Contains(alt, "[") || strings.Contains(alt, "]") {
			alt = "图片"
		}

		return "\n\n![" + alt + "](" + url + ")\n\n"
	})

	// 处理缺少 [] 的情况，如 !(url)
	missingBracketsRegex := regexp.MustCompile(`!\((.*?)\)`)
	text = missingBracketsRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := missingBracketsRegex.FindStringSubmatch(match)
		url := submatch[1]
		url = cleanImageURL(url)
		return "\n\n![图片](" + url + ")\n\n"
	})

	// 处理缺少 ![] 的情况，如 (url)，但 URL 看起来像图片路径
	missingPrefixRegex := regexp.MustCompile(`\(((?:[a-zA-Z0-9_\-\.\/]+)?(?:images|img|pics|assets|static|resources|public|uploads|media)\/[^\s\)]+` + imageExtensions + `)\)`)
	text = missingPrefixRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := missingPrefixRegex.FindStringSubmatch(match)
		url := submatch[1]
		url = cleanImageURL(url)
		return "\n\n![图片](" + url + ")\n\n"
	})

	// 处理 HTTP/HTTPS URL
	httpUrlRegex := regexp.MustCompile(`\((https?:\/\/[^\s\)]+` + imageExtensions + `)\)`)
	text = httpUrlRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := httpUrlRegex.FindStringSubmatch(match)
		url := submatch[1]
		url = cleanImageURL(url)
		return "\n\n![图片](" + url + ")\n\n"
	})

	// 处理缺少 () 的情况，如 ![]images/xxx.jpg 或 ![]./images/xxx.jpg
	missingParenthesesRegex := regexp.MustCompile(`!\[(.*?)\]([^(](?:\.\/)?(?:images|img|pics|assets|static|resources|public|uploads|media)\/[^\s]+` + imageExtensions + `)`)
	text = missingParenthesesRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := missingParenthesesRegex.FindStringSubmatch(match)
		alt := submatch[1]
		url := submatch[2]
		url = cleanImageURL(url)

		if alt == "" || strings.Contains(alt, "[") || strings.Contains(alt, "]") {
			alt = "图片"
		}

		return "\n\n![" + alt + "](" + url + ")\n\n"
	})

	// 处理完全异常的情况，尝试提取图片 URL（本地路径）
	imageURLRegex := regexp.MustCompile(`(?:^|\s)((?:\.\/)?(?:images|img|pics|assets|static|resources|public|uploads|media)\/[a-zA-Z0-9_\-\.\/]+` + imageExtensions + `)(?:$|\s|\))`)
	text = imageURLRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := imageURLRegex.FindStringSubmatch(match)
		url := submatch[1]
		url = cleanImageURL(url)
		return "\n\n![图片](" + url + ")\n\n"
	})

	// 处理完全异常的情况，尝试提取 HTTP/HTTPS 图片 URL
	httpImageURLRegex := regexp.MustCompile(`(?:^|\s)(https?:\/\/[a-zA-Z0-9_\-\.\/]+` + imageExtensions + `)(?:$|\s|\))`)
	text = httpImageURLRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := httpImageURLRegex.FindStringSubmatch(match)
		url := submatch[1]
		url = cleanImageURL(url)
		return "\n\n![图片](" + url + ")\n\n"
	})

	// 处理可能的嵌套括号问题 ![图片]([]()url)
	nestedBracketsRegex := regexp.MustCompile(`!\[图片\]\(\[\]\(\)(.*?)\)`)
	text = nestedBracketsRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := nestedBracketsRegex.FindStringSubmatch(match)
		url := submatch[1]
		url = cleanImageURL(url)
		return "\n\n![图片](" + url + ")\n\n"
	})

	// 移除重复的图片标签
	text = removeDuplicateImages(text)

	// 移除多余的空行，确保最多只有两个换行符
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")

	return text
}

// removeDuplicateImages 移除重复的图片标签
func removeDuplicateImages(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	var lastImageLine string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		// 如果是图片标签行
		if regexp.MustCompile(`^!\[.*?\]\(.*?\)$`).MatchString(trimmedLine) {
			// 提取URL部分
			urlMatch := regexp.MustCompile(`!\[.*?\]\((.*?)\)`).FindStringSubmatch(trimmedLine)
			if len(urlMatch) > 1 {
				currentURL := urlMatch[1]

				// 如果这个URL与上一个图片URL相同，跳过
				if lastImageLine != "" {
					lastURLMatch := regexp.MustCompile(`!\[.*?\]\((.*?)\)`).FindStringSubmatch(lastImageLine)
					if len(lastURLMatch) > 1 && lastURLMatch[1] == currentURL {
						continue
					}
				}

				lastImageLine = trimmedLine
			}
		} else {
			// 如果不是图片行，重置lastImageLine
			lastImageLine = ""
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// isLikelyImageURL 判断字符串是否看起来像图片URL
func isLikelyImageURL(s string) bool {
	// 检查是否以常见的图片扩展名结尾
	imageExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".bmp", ".tiff"}
	for _, ext := range imageExtensions {
		if strings.HasSuffix(strings.ToLower(s), ext) {
			return true
		}
	}

	// 检查是否包含常见的图片目录名
	imageDirs := []string{"images/", "img/", "pics/", "assets/", "static/", "resources/", "public/", "uploads/", "media/"}
	for _, dir := range imageDirs {
		if strings.Contains(strings.ToLower(s), dir) {
			return true
		}
	}

	// 检查是否是HTTP/HTTPS URL且包含图片扩展名
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		for _, ext := range imageExtensions {
			if strings.Contains(strings.ToLower(s), ext) {
				return true
			}
		}
	}

	return false
}

// cleanImageURL 清理图片 URL，移除可能的额外括号和其他干扰字符
func cleanImageURL(url string) string {
	// 移除 URL 中可能的额外括号
	url = strings.TrimPrefix(url, "(")
	url = strings.TrimSuffix(url, ")")
	url = strings.TrimPrefix(url, "[]()")

	// 移除可能的结尾标点
	url = strings.TrimRight(url, ",.;:!?")

	// 如果 URL 包含括号，尝试提取实际的 URL 部分
	if strings.Contains(url, "(") || strings.Contains(url, ")") {
		// 尝试匹配常见的图片路径模式
		imagePathRegex := regexp.MustCompile(`((?:\.\/)?(?:images|img|pics|assets|static|resources|public|uploads|media)\/[^\s\(\)]+\.(?:jpg|jpeg|png|gif|webp|svg|bmp|tiff))`)
		if matches := imagePathRegex.FindStringSubmatch(url); len(matches) > 0 {
			url = matches[1]
		}

		// 尝试匹配 HTTP/HTTPS URL
		httpURLRegex := regexp.MustCompile(`(https?:\/\/[^\s\(\)]+\.(?:jpg|jpeg|png|gif|webp|svg|bmp|tiff))`)
		if matches := httpURLRegex.FindStringSubmatch(url); len(matches) > 0 {
			url = matches[1]
		}
	}

	// 移除 URL 中的空格
	url = strings.TrimSpace(url)

	// 确保相对路径正确（处理可能的多余点和斜杠）
	if strings.HasPrefix(url, "./") || strings.HasPrefix(url, "../") {
		url = strings.Replace(url, "./", "", 1)
	}

	return url
}

// unifyMathFormulas 统一数学公式格式
func (p *MarkdownPostProcessor) unifyMathFormulas(text string) string {
	p.logger.Debug("统一数学公式格式 Unify Math Formulas")

	// 将 \(\mathbf{F}\) 格式转换为 $\mathbf{F}$
	inlineLatexRegex1 := regexp.MustCompile(`\\\\?\((.+?)\\\\?\)`)
	text = inlineLatexRegex1.ReplaceAllStringFunc(text, func(match string) string {
		// 提取公式内容
		content := inlineLatexRegex1.FindStringSubmatch(match)[1]
		return " $" + content + "$ "
	})

	// 将 \begin{math}...\end{math} 格式转换为 $...$
	inlineLatexRegex2 := regexp.MustCompile(`\\begin\{math\}(.+?)\\end\{math\}`)
	text = inlineLatexRegex2.ReplaceAllStringFunc(text, func(match string) string {
		// 提取公式内容
		content := inlineLatexRegex2.FindStringSubmatch(match)[1]
		return " $" + content + "$ "
	})

	// // 将 \begin{equation}...\end{equation} 格式转换为 $$...$$
	// blockLatexRegex := regexp.MustCompile(`\\begin\{equation\}(.+?)\\end\{equation\}`)
	// text = blockLatexRegex.ReplaceAllStringFunc(text, func(match string) string {
	// 	// 提取公式内容
	// 	content := blockLatexRegex.FindStringSubmatch(match)[1]
	// 	return "$$" + content + "$$"
	// })

	// // 将 \begin{align}...\end{align} 格式转换为 $$...$$
	// alignLatexRegex := regexp.MustCompile(`\\begin\{align\}(.+?)\\end\{align\}`)
	// text = alignLatexRegex.ReplaceAllStringFunc(text, func(match string) string {
	// 	// 提取公式内容
	// 	content := alignLatexRegex.FindStringSubmatch(match)[1]
	// 	return "$$" + content + "$$"
	// })

	// 将 \[ ... \] 格式转换为 $$ ... $$
	displayLatexRegex := regexp.MustCompile(`\\\\?\[(.+?)\\\\?\]`)
	text = displayLatexRegex.ReplaceAllStringFunc(text, func(match string) string {
		// 提取公式内容
		content := displayLatexRegex.FindStringSubmatch(match)[1]
		return "\n$$" + content + "$$\n"
	})

	return text
}

// fixMathFormulas 修复数学公式
func (p *MarkdownPostProcessor) fixMathFormulas(text string) string {
	p.logger.Debug("修复数学公式")

	// 修复行内公式
	inlineMathRegex := regexp.MustCompile(`\$([^$]+)\$`)
	text = inlineMathRegex.ReplaceAllStringFunc(text, func(match string) string {
		// 移除公式中的空格
		formula := match[1 : len(match)-1]
		// formula = strings.ReplaceAll(formula, "\\", "\\\\") // 转义反斜杠
		return "$" + formula + "$"
	})

	// 修复块级公式
	blockMathRegex := regexp.MustCompile(`(?s)\$\$(.*?)\$\$`)
	text = blockMathRegex.ReplaceAllStringFunc(text, func(match string) string {
		// 移除公式中的空格
		formula := match[2 : len(match)-2]
		// formula = strings.ReplaceAll(formula, "\\", "\\\\") // 转义反斜杠
		return "\n$$" + formula + "$$\n"
	})

	return text
}

// fixTableFormat 修复表格格式
func (p *MarkdownPostProcessor) fixTableFormat(text string) string {
	p.logger.Debug("修复表格格式")

	text = strings.ReplaceAll(text, "<html><body>", "\n")
	text = strings.ReplaceAll(text, "</body></html>", "\n")

	// // 查找 HTML 表格
	// htmlTableRegex := regexp.MustCompile(`(?s)<html><body><table>(.*?)</table></body></html>`)
	// text = htmlTableRegex.ReplaceAllStringFunc(text, func(match string) string {
	// 	// 提取表格内容
	// 	tableContent := match[19 : len(match)-20]

	// 	// 修复表格行
	// 	tableContent = strings.ReplaceAll(tableContent, "<tr>", "")
	// 	tableContent = strings.ReplaceAll(tableContent, "</tr>", "\n")

	// 	// 修复表格单元格
	// 	tableContent = strings.ReplaceAll(tableContent, "<td>", "| ")
	// 	tableContent = strings.ReplaceAll(tableContent, "</td>", " ")

	// 	// 添加表格行结束符
	// 	lines := strings.Split(tableContent, "\n")
	// 	var result strings.Builder

	// 	for i, line := range lines {
	// 		if strings.TrimSpace(line) == "" {
	// 			continue
	// 		}

	// 		result.WriteString(line)
	// 		result.WriteString("|\n")

	// 		// 在第一行后添加分隔行
	// 		if i == 0 {
	// 			cells := strings.Count(line, "|") - 1
	// 			if cells > 0 {
	// 				result.WriteString("|")
	// 				for j := 0; j < cells; j++ {
	// 					result.WriteString(" --- |")
	// 				}
	// 				result.WriteString("\n")
	// 			}
	// 		}
	// 	}

	// 	return result.String()
	// })

	return text
}

// fixMixedContent 修复混合内容（中英文混合）
func (p *MarkdownPostProcessor) fixMixedContent(text string) string {
	// p.logger.Debug("修复混合内容")

	// // 查找中英文混合的内容
	// mixedContentRegex := regexp.MustCompile(`([a-zA-Z0-9]+)([，。！？；：""''（）【】《》])|([，。！？；：""''（）【】《》])([a-zA-Z0-9]+)`)
	// text = mixedContentRegex.ReplaceAllStringFunc(text, func(match string) string {
	// 	// 在中英文之间添加空格
	// 	return strings.ReplaceAll(match, "", " ")
	// })

	// // 修复错误的中英文混合（如 "此 数ne据gl来ig自ib腾le讯 i文m档p-a>cretg iostnra tiaocnc实ur验ac数y"）
	// // 这种情况通常是翻译出错，需要重新翻译
	// // 由于这种情况很复杂，我们只能尝试检测并标记出来
	// suspiciousMixedContentRegex := regexp.MustCompile(`[a-zA-Z][一-龥][a-zA-Z]|[一-龥][a-zA-Z][一-龥]`)
	// text = suspiciousMixedContentRegex.ReplaceAllStringFunc(text, func(match string) string {
	// 	p.logger.Warn("检测到可疑的混合内容", zap.String("内容", match))
	// 	return "【可能的翻译错误：" + match + "】"
	// })

	return text
}

// 删除重复的 splitIntoSentences 函数，因为它已经在 text.go 中定义
