package translator

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/nerdneilsfield/go-translator-agent/internal/progress"
)

// readFile 读取文件内容
func (c *TranslationCoordinator) readFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	return string(content), nil
}

// writeFile 写入文件内容
func (c *TranslationCoordinator) writeFile(filePath, content string) error {
	// 确保目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filePath, err)
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return nil
}

// parseDocument 解析文档为节点
func (c *TranslationCoordinator) parseDocument(filePath, content string) ([]*document.NodeInfo, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".md", ".markdown":
		return c.parseMarkdown(content)
	case ".txt":
		return c.parseText(content)
	case ".html", ".htm":
		return c.parseHTML(content)
	default:
		// 默认按文本处理
		return c.parseText(content)
	}
}

// parseMarkdown 解析 Markdown 文档
func (c *TranslationCoordinator) parseMarkdown(content string) ([]*document.NodeInfo, error) {
	// 简单的段落分割
	paragraphs := strings.Split(content, "\n\n")
	var nodes []*document.NodeInfo

	for i, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		node := &document.NodeInfo{
			ID:           i + 1,
			OriginalText: paragraph,
			Status:       document.NodeStatusPending,
			Path:         fmt.Sprintf("paragraph-%d", i+1),
			Metadata: map[string]interface{}{
				"type":     "paragraph",
				"format":   "markdown",
				"position": i,
			},
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// parseText 解析纯文本文档
func (c *TranslationCoordinator) parseText(content string) ([]*document.NodeInfo, error) {
	// 按段落分割
	paragraphs := strings.Split(content, "\n\n")
	var nodes []*document.NodeInfo

	for i, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		node := &document.NodeInfo{
			ID:           i + 1,
			OriginalText: paragraph,
			Status:       document.NodeStatusPending,
			Path:         fmt.Sprintf("paragraph-%d", i+1),
			Metadata: map[string]interface{}{
				"type":     "paragraph",
				"format":   "text",
				"position": i,
			},
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// parseHTML 解析 HTML 文档
func (c *TranslationCoordinator) parseHTML(content string) ([]*document.NodeInfo, error) {
	// 简化实现，按 <p> 标签分割
	// 在实际应用中，应该使用更复杂的 HTML 解析器
	lines := strings.Split(content, "\n")
	var nodes []*document.NodeInfo
	nodeID := 1

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 简单检查是否包含文本内容
		if strings.Contains(line, "<p>") || (len(line) > 10 && !strings.HasPrefix(line, "<")) {
			node := &document.NodeInfo{
				ID:           nodeID,
				OriginalText: line,
				Status:       document.NodeStatusPending,
				Path:         fmt.Sprintf("line-%d", i+1),
				Metadata: map[string]interface{}{
					"type":     "html-line",
					"format":   "html",
					"position": i,
				},
			}
			nodes = append(nodes, node)
			nodeID++
		}
	}

	return nodes, nil
}

// createTextNodes 为文本创建节点
func (c *TranslationCoordinator) createTextNodes(text string) []*document.NodeInfo {
	// 简单按段落分割
	paragraphs := strings.Split(text, "\n\n")
	var nodes []*document.NodeInfo

	for i, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		node := &document.NodeInfo{
			ID:           i + 1,
			OriginalText: paragraph,
			Status:       document.NodeStatusPending,
			Path:         fmt.Sprintf("inline-paragraph-%d", i+1),
			Metadata: map[string]interface{}{
				"type":   "inline-text",
				"format": "text",
			},
		}
		nodes = append(nodes, node)
	}

	return nodes
}

// assembleDocument 重新组装翻译后的文档
func (c *TranslationCoordinator) assembleDocument(originalPath string, nodes []*document.NodeInfo) (string, error) {
	ext := strings.ToLower(filepath.Ext(originalPath))

	switch ext {
	case ".md", ".markdown":
		return c.assembleMarkdown(nodes), nil
	case ".txt":
		return c.assembleText(nodes), nil
	case ".html", ".htm":
		return c.assembleHTML(nodes), nil
	default:
		return c.assembleText(nodes), nil
	}
}

// assembleMarkdown 组装 Markdown 文档
func (c *TranslationCoordinator) assembleMarkdown(nodes []*document.NodeInfo) string {
	var parts []string

	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess && node.TranslatedText != "" {
			parts = append(parts, node.TranslatedText)
		} else {
			// 翻译失败时保留原文
			parts = append(parts, node.OriginalText)
		}
	}

	return strings.Join(parts, "\n\n")
}

// assembleText 组装纯文本文档
func (c *TranslationCoordinator) assembleText(nodes []*document.NodeInfo) string {
	var parts []string

	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess && node.TranslatedText != "" {
			parts = append(parts, node.TranslatedText)
		} else {
			parts = append(parts, node.OriginalText)
		}
	}

	return strings.Join(parts, "\n\n")
}

// assembleHTML 组装 HTML 文档
func (c *TranslationCoordinator) assembleHTML(nodes []*document.NodeInfo) string {
	var parts []string

	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess && node.TranslatedText != "" {
			parts = append(parts, node.TranslatedText)
		} else {
			parts = append(parts, node.OriginalText)
		}
	}

	return strings.Join(parts, "\n")
}

// assembleTextResult 组装文本翻译结果
func (c *TranslationCoordinator) assembleTextResult(nodes []*document.NodeInfo) string {
	var parts []string

	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess && node.TranslatedText != "" {
			parts = append(parts, node.TranslatedText)
		} else {
			parts = append(parts, node.OriginalText)
		}
	}

	return strings.Join(parts, "\n\n")
}

// createSuccessResult 创建成功结果
func (c *TranslationCoordinator) createSuccessResult(docID, inputFile, outputFile string, startTime, endTime time.Time, nodes []*document.NodeInfo) *TranslationResult {
	totalNodes := len(nodes)
	completedNodes := 0
	failedNodes := 0

	for _, node := range nodes {
		switch node.Status {
		case document.NodeStatusSuccess:
			completedNodes++
		case document.NodeStatusFailed:
			failedNodes++
		}
	}

	progressPercent := float64(0)
	if totalNodes > 0 {
		progressPercent = float64(completedNodes) / float64(totalNodes) * 100
	}

	status := string(progress.StatusCompleted)
	if failedNodes > 0 {
		status = string(progress.StatusFailed)
	}

	return &TranslationResult{
		DocID:          docID,
		InputFile:      inputFile,
		OutputFile:     outputFile,
		SourceLanguage: c.config.SourceLang,
		TargetLanguage: c.config.TargetLang,
		TotalNodes:     totalNodes,
		CompletedNodes: completedNodes,
		FailedNodes:    failedNodes,
		Progress:       progressPercent,
		Status:         status,
		StartTime:      startTime,
		EndTime:        &endTime,
		Duration:       endTime.Sub(startTime),
	}
}

// createFailedResult 创建失败结果
func (c *TranslationCoordinator) createFailedResult(docID, inputFile, outputFile string, startTime time.Time, err error) *TranslationResult {
	endTime := time.Now()

	return &TranslationResult{
		DocID:          docID,
		InputFile:      inputFile,
		OutputFile:     outputFile,
		SourceLanguage: c.config.SourceLang,
		TargetLanguage: c.config.TargetLang,
		TotalNodes:     0,
		CompletedNodes: 0,
		FailedNodes:    0,
		Progress:       0,
		Status:         string(progress.StatusFailed),
		StartTime:      startTime,
		EndTime:        &endTime,
		Duration:       endTime.Sub(startTime),
		ErrorMessage:   err.Error(),
	}
}
