package preformat

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
)

// PreFormatter 预格式化器
type PreFormatter struct {
	logger *zap.Logger
}

// NewPreFormatter 创建预格式化器
func NewPreFormatter(logger *zap.Logger) *PreFormatter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PreFormatter{
		logger: logger,
	}
}

// ProcessFile 处理文件，返回处理后的临时文件路径
func (pf *PreFormatter) ProcessFile(inputPath string) (string, error) {
	// 读取原始文件
	content, err := pf.readFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("failed to read input file: %w", err)
	}

	// 创建临时文件
	tempPath, err := pf.createTempFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	// 应用所有预处理规则
	processedContent := pf.applyAllRules(content)

	// 写入临时文件
	err = pf.writeFile(tempPath, processedContent)
	if err != nil {
		os.Remove(tempPath) // 清理临时文件
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}

	pf.logger.Info("pre-formatting completed",
		zap.String("input", inputPath),
		zap.String("temp", tempPath),
		zap.Int("original_length", len(content)),
		zap.Int("processed_length", len(processedContent)))

	return tempPath, nil
}

// applyAllRules 应用所有预处理规则
func (pf *PreFormatter) applyAllRules(content string) string {
	// 按顺序应用各种格式化规则
	content = pf.separateImages(content)
	content = pf.standardizeFormulas(content)
	content = pf.convertHTMLTables(content)
	content = pf.convertBareLinks(content)
	content = pf.separateReferences(content)
	content = pf.removeMultipleNewlines(content)
	return content
}

func (pf *PreFormatter) removeMultipleNewlines(content string) string {
	pattern := regexp.MustCompile(`\n{3,}`)
	content = pattern.ReplaceAllString(content, "\n\n")
	pattern = regexp.MustCompile(`\n\s+\n`)
	content = pattern.ReplaceAllString(content, "\n\n")
	return content
}

// separateImages 分离图片，使其单独成段
func (pf *PreFormatter) separateImages(content string) string {
	// 匹配图片模式：![...](...)，后面同一行可能跟着标题
	// 使用非贪婪匹配直到行尾，避免在标题中的句点导致截断
	imagePattern := regexp.MustCompile(`(!\[[^\]]*\]\([^)]*\))\s*([^\n]*)`)

	result := imagePattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := imagePattern.FindStringSubmatch(match)
		if len(parts) >= 3 {
			imageLink := parts[1]
			caption := strings.TrimSpace(parts[2])

			// 构造替换文本：图片单独成段，标题（若存在且非空）紧随其后单独成段
			replacement := fmt.Sprintf("\n\n%s\n\n", imageLink)
			// 过滤纯标点或空字符串
			captionClean := strings.TrimSpace(caption)
			if captionClean != "" && captionClean != "." {
				replacement += fmt.Sprintf("%s\n\n", captionClean)
			}
			return replacement
		}
		return match
	})

	pf.logger.Debug("separated images", zap.Int("matches", len(imagePattern.FindAllString(content, -1))))
	return result
}

// standardizeFormulas 标准化行间公式格式
func (pf *PreFormatter) standardizeFormulas(content string) string {
	// 匹配 $$ ... $$ 模式，可能跨多行
	formulaPattern := regexp.MustCompile(`\$\$\s*([^$]+?)\s*\$\$`)

	result := formulaPattern.ReplaceAllStringFunc(content, func(match string) string {
		// 提取公式内容
		parts := formulaPattern.FindStringSubmatch(match)
		if len(parts) >= 2 {
			formulaContent := strings.TrimSpace(parts[1])

			// 标准化格式：前后空行，公式内容单独行
			return fmt.Sprintf("\n\n$$\n%s\n$$\n\n", formulaContent)
		}
		return match
	})

	pf.logger.Debug("standardized formulas", zap.Int("matches", len(formulaPattern.FindAllString(content, -1))))
	return result
}

// convertHTMLTables 转换HTML表格为Markdown格式
func (pf *PreFormatter) convertHTMLTables(content string) string {
	// 检查是否包含HTML表格
	htmlTablePattern := regexp.MustCompile(`<html><body><table>.*?</table></body></html>`)

	result := htmlTablePattern.ReplaceAllStringFunc(content, func(match string) string {
		markdown := pf.htmlTableToMarkdown(match)
		if markdown != "" {
			// 用保护标记包围整个表格
			return fmt.Sprintf("\n\n<!-- TABLE_PROTECTED -->\n%s\n<!-- /TABLE_PROTECTED -->\n\n", markdown)
		}
		return match
	})

	pf.logger.Debug("converted HTML tables", zap.Int("matches", len(htmlTablePattern.FindAllString(content, -1))))
	return result
}

// htmlTableToMarkdown 将HTML表格转换为Markdown
func (pf *PreFormatter) htmlTableToMarkdown(htmlTable string) string {
	// 使用goquery解析HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlTable))
	if err != nil {
		pf.logger.Warn("failed to parse HTML table", zap.Error(err))
		return htmlTable
	}

	var rows [][]string

	doc.Find("tr").Each(func(i int, tr *goquery.Selection) {
		var row []string
		tr.Find("td, th").Each(func(j int, cell *goquery.Selection) {
			cellText := strings.TrimSpace(cell.Text())
			// 清理单元格内容
			cellText = strings.ReplaceAll(cellText, "\n", " ")
			cellText = regexp.MustCompile(`\s+`).ReplaceAllString(cellText, " ")
			row = append(row, cellText)
		})
		if len(row) > 0 {
			rows = append(rows, row)
		}
	})

	if len(rows) == 0 {
		return htmlTable
	}

	// 生成Markdown表格
	var markdown strings.Builder

	// 第一行作为表头
	if len(rows) > 0 {
		markdown.WriteString("| ")
		markdown.WriteString(strings.Join(rows[0], " | "))
		markdown.WriteString(" |\n")

		// 分隔线
		markdown.WriteString("|")
		for range rows[0] {
			markdown.WriteString(" --- |")
		}
		markdown.WriteString("\n")

		// 数据行
		for i := 1; i < len(rows); i++ {
			markdown.WriteString("| ")
			markdown.WriteString(strings.Join(rows[i], " | "))
			markdown.WriteString(" |\n")
		}
	}

	return markdown.String()
}

// convertBareLinks 转换裸链接为Markdown格式
func (pf *PreFormatter) convertBareLinks(content string) string {
	// 匹配各种URL模式
	urlPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\bhttps?://[^\s\[\]()]+`), // HTTP(S) URLs
		regexp.MustCompile(`\bdoi:[^\s\[\]()]+`),      // DOI links
		regexp.MustCompile(`\barXiv:[^\s\[\]()]+`),    // arXiv links
	}

	result := content
	totalMatches := 0

	for _, pattern := range urlPatterns {
		matches := pattern.FindAllString(result, -1)
		totalMatches += len(matches)

		result = pattern.ReplaceAllStringFunc(result, func(url string) string {
			// 检查是否已经在Markdown链接中
			if strings.Contains(content, fmt.Sprintf("(%s)", url)) ||
				strings.Contains(content, fmt.Sprintf("[%s]", url)) {
				return url // 已经是链接格式，不处理
			}

			// 转换为Markdown链接格式并保护
			return fmt.Sprintf("[%s](%s)", url, url)
		})
	}

	pf.logger.Debug("converted bare links", zap.Int("matches", totalMatches))
	return result
}

// separateReferences 分离引用文献
func (pf *PreFormatter) separateReferences(content string) string {
	// 查找REFERENCES部分 - 使用更灵活的匹配
	refPattern := regexp.MustCompile(`(?is)#\s*REFERENCES\s*\n(.*?)(?:\n*$)`)

	result := refPattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := refPattern.FindStringSubmatch(match)
		if len(parts) >= 2 {
			referencesContent := parts[1]

			// 分离各个文献
			separated := pf.separateIndividualReferences(referencesContent)

			// 保护整个引用部分
			return fmt.Sprintf("# REFERENCES\n\n<!-- REFERENCES_PROTECTED -->\n%s\n<!-- /REFERENCES_PROTECTED -->\n", separated)
		}
		return match
	})

	pf.logger.Debug("separated references")
	return result
}

// separateIndividualReferences 分离单个文献条目
func (pf *PreFormatter) separateIndividualReferences(references string) string {
	// 使用简单的分割方法来处理文献
	// 先按 [数字] 模式分割
	refItemPattern := regexp.MustCompile(`\[(\d+)\]`)

	// 找到所有编号位置
	matches := refItemPattern.FindAllStringSubmatchIndex(references, -1)
	if len(matches) == 0 {
		// 如果没有匹配到编号，直接返回原内容
		return strings.TrimSpace(references)
	}

	var separated strings.Builder

	for i, match := range matches {
		if len(match) >= 4 {
			// 提取文献编号
			refNumStart := match[2]
			refNumEnd := match[3]
			refNum := references[refNumStart:refNumEnd]

			// 确定文献内容的开始和结束位置
			contentStart := match[1] // ] 之后的位置
			var contentEnd int
			if i+1 < len(matches) {
				// 下一个 [ 之前
				contentEnd = matches[i+1][0]
			} else {
				// 最后一个文献，到字符串结尾
				contentEnd = len(references)
			}

			// 提取并清理文献内容
			refContent := strings.TrimSpace(references[contentStart:contentEnd])
			refContent = regexp.MustCompile(`\s+`).ReplaceAllString(refContent, " ")

			// 如果文献内容不为空，添加到结果中
			if refContent != "" {
				separated.WriteString(fmt.Sprintf("[%s] %s\n\n", refNum, refContent))
			}
		}
	}

	result := separated.String()
	if result == "" {
		// 如果处理后没有内容，返回原始内容
		return strings.TrimSpace(references)
	}

	return result
}

// readFile 读取文件内容
func (pf *PreFormatter) readFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// createTempFile 创建临时文件
func (pf *PreFormatter) createTempFile(originalPath string) (string, error) {
	dir := filepath.Dir(originalPath)
	base := filepath.Base(originalPath)
	ext := filepath.Ext(originalPath)
	name := strings.TrimSuffix(base, ext)

	tempName := fmt.Sprintf("%s_preformatted%s", name, ext)
	tempPath := filepath.Join(dir, tempName)

	return tempPath, nil
}

// writeFile 写入文件
func (pf *PreFormatter) writeFile(filePath, content string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	return err
}

// CleanupTempFile 清理临时文件
func (pf *PreFormatter) CleanupTempFile(tempPath string) error {
	if tempPath != "" {
		return os.Remove(tempPath)
	}
	return nil
}
