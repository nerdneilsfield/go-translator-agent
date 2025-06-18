package translator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/nerdneilsfield/go-translator-agent/internal/progress"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"go.uber.org/zap"
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

// assembleDocument 简化组装方法（作为回退）
func (c *TranslationCoordinator) assembleDocument(originalPath string, nodes []*document.NodeInfo) (string, error) {
	var parts []string

	for _, node := range nodes {
		if node.Status == document.NodeStatusSuccess && node.TranslatedText != "" {
			parts = append(parts, node.TranslatedText)
		} else {
			// 翻译失败时保留原文
			parts = append(parts, node.OriginalText)
		}
	}

	// 根据文件类型决定连接方式
	ext := strings.ToLower(filepath.Ext(originalPath))
	switch ext {
	case ".html", ".htm":
		return strings.Join(parts, "\n"), nil
	default:
		return strings.Join(parts, "\n\n"), nil
	}
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

	// 收集 token 使用情况和其他元数据
	metadata := make(map[string]interface{})

	// TODO: 从翻译服务收集实际的 token 使用情况
	// 这里暂时使用估算值
	totalChars := 0
	for _, node := range nodes {
		totalChars += len(node.OriginalText)
	}
	// 粗略估算：1个字符约等于0.5个token
	estimatedTokens := totalChars / 2
	metadata["token_usage"] = map[string]interface{}{
		"tokens_in":  estimatedTokens,
		"tokens_out": estimatedTokens,
	}

	// 缓存统计（如果有的话）
	// TODO: 从缓存服务收集实际的缓存统计
	metadata["cache_stats"] = map[string]interface{}{
		"hits":   0,
		"misses": totalNodes,
	}

	// 收集失败节点详细信息
	var failedDetails []*FailedNodeDetail
	for _, node := range nodes {
		if node.Status == document.NodeStatusFailed {
			errorType, step, stepIndex := extractErrorDetails(node.Error)
			detail := &FailedNodeDetail{
				NodeID:       node.ID,
				OriginalText: truncateText(node.OriginalText, 200), // 限制原文长度为200字符
				Path:         node.Path,
				ErrorType:    errorType,
				ErrorMessage: func() string {
					if node.Error != nil {
						return node.Error.Error()
					}
					return "unknown error"
				}(),
				Step:         step,
				StepIndex:    stepIndex,
				RetryCount:   node.RetryCount,
				FailureTime:  endTime,
			}
			failedDetails = append(failedDetails, detail)
		}
	}
	
	// 获取详细的翻译汇总（如果是BatchTranslator的话）
	var detailedSummary *DetailedTranslationSummary
	if batchTranslator, ok := c.translator.(*BatchTranslator); ok {
		detailedSummary = batchTranslator.GetDetailedTranslationSummary(nodes)
	}

	return &TranslationResult{
		DocID:      docID,
		InputFile:  inputFile,
		OutputFile: outputFile,
		// SourceLanguage: c.coordinatorConfig.SourceLang,
		// TargetLanguage: c.coordinatorConfig.TargetLang,
		TotalNodes:     totalNodes,
		CompletedNodes: completedNodes,
		FailedNodes:    failedNodes,
		Progress:       progressPercent,
		Status:         status,
		StartTime:      startTime,
		EndTime:        &endTime,
		Duration:       endTime.Sub(startTime),
		Metadata:       metadata,
		FailedNodeDetails: failedDetails,
		DetailedSummary:   detailedSummary,
	}
}

// extractNodesFromDocument 从Document中提取NodeInfo节点
func (c *TranslationCoordinator) extractNodesFromDocument(doc *document.Document) []*document.NodeInfo {
	var nodes []*document.NodeInfo
	nodeID := 1

	for i, block := range doc.Blocks {
		if !block.IsTranslatable() {
			continue
		}

		node := &document.NodeInfo{
			ID:           nodeID,
			BlockID:      fmt.Sprintf("block-%d", i),
			OriginalText: block.GetContent(),
			Status:       document.NodeStatusPending,
			Path:         fmt.Sprintf("/block[%d]", i+1),
			Metadata: map[string]interface{}{
				"blockIndex": i,
				"blockType":  string(block.GetType()),
				"document":   doc.ID,
				"format":     string(doc.Format),
			},
		}
		nodes = append(nodes, node)
		nodeID++
	}

	c.logger.Info("extracted nodes from document",
		zap.String("docID", doc.ID),
		zap.String("format", string(doc.Format)),
		zap.Int("totalBlocks", len(doc.Blocks)),
		zap.Int("translatableNodes", len(nodes)))

	return nodes
}

// assembleDocumentWithProcessor 使用document processor重建并渲染文档
func (c *TranslationCoordinator) assembleDocumentWithProcessor(inputPath string, doc *document.Document, nodes []*document.NodeInfo) (string, error) {
	// 创建节点映射，按BlockID索引
	nodeMap := make(map[string]*document.NodeInfo)
	for _, node := range nodes {
		nodeMap[node.BlockID] = node
	}

	// 更新文档中的块内容
	for i, block := range doc.Blocks {
		blockID := fmt.Sprintf("block-%d", i)
		if node, exists := nodeMap[blockID]; exists && node.Status == document.NodeStatusSuccess {
			// 更新块内容为翻译后的文本
			block.SetContent(node.TranslatedText)
		}
		// 如果没有翻译或翻译失败，保留原始内容
	}

	// 重新获取processor进行渲染
	processorOpts := document.ProcessorOptions{
		ChunkSize:    c.coordinatorConfig.ChunkSize,
		ChunkOverlap: 100,
		Metadata: map[string]interface{}{
			"source_language":      c.coordinatorConfig.SourceLang,
			"target_language":      c.coordinatorConfig.TargetLang,
			"logger":               c.logger,
			"html_processing_mode": c.coordinatorConfig.HTMLProcessingMode,
		},
	}

	processor, err := document.GetProcessorByExtension(inputPath, processorOpts)
	if err != nil {
		c.logger.Warn("failed to get processor for rendering, using fallback", zap.Error(err))
		// 使用简化的组装方法作为回退
		return c.assembleDocument(inputPath, nodes)
	}

	// 使用processor渲染文档
	var buffer strings.Builder
	err = processor.Render(context.Background(), doc, &buffer)
	if err != nil {
		c.logger.Warn("failed to render document with processor, using fallback", zap.Error(err))
		// 使用简化的组装方法作为回退
		return c.assembleDocument(inputPath, nodes)
	}

	c.logger.Info("document assembled with processor",
		zap.String("format", string(doc.Format)),
		zap.Int("totalBlocks", len(doc.Blocks)),
		zap.Int("outputLength", buffer.Len()))

	return buffer.String(), nil
}

// createFailedResult 创建失败结果
func (c *TranslationCoordinator) createFailedResult(docID, inputFile, outputFile string, startTime time.Time, err error) *TranslationResult {
	endTime := time.Now()

	return &TranslationResult{
		DocID:      docID,
		InputFile:  inputFile,
		OutputFile: outputFile,
		// SourceLanguage: c.coordinatorConfig.SourceLang,
		// TargetLanguage: c.coordinatorConfig.TargetLang,
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

// classifyErrorType 分类错误类型，支持 TranslationError 结构化错误
func classifyErrorType(err error) string {
	if err == nil {
		return "unknown"
	}
	
	// 优先检查是否是 TranslationError
	if transErr, ok := err.(*translation.TranslationError); ok {
		// 直接使用结构化的错误代码
		switch transErr.Code {
		case translation.ErrCodeTimeout:
			return "timeout"
		case translation.ErrCodeRateLimit:
			return "rate_limit"
		case translation.ErrCodeNetwork:
			return "network"
		case translation.ErrCodeLLM:
			return "llm_error"
		case translation.ErrCodeConfig:
			return "config_error"
		case translation.ErrCodeValidation:
			return "validation_error"
		case translation.ErrCodeCache:
			return "cache_error"
		case translation.ErrCodeStep:
			return "step_error"
		case translation.ErrCodeChain:
			return "chain_error"
		default:
			return strings.ToLower(transErr.Code)
		}
	}
	
	// 递归检查嵌套错误
	if unwrapped := errors.Unwrap(err); unwrapped != nil {
		if result := classifyErrorType(unwrapped); result != "unknown" {
			return result
		}
	}
	
	// 回退到字符串匹配
	errorMsg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errorMsg, "timeout"):
		return "timeout"
	case strings.Contains(errorMsg, "rate limit"):
		return "rate_limit"
	case strings.Contains(errorMsg, "network"):
		return "network"
	case strings.Contains(errorMsg, "context deadline exceeded"):
		return "timeout"
	case strings.Contains(errorMsg, "context canceled"):
		return "canceled"
	case strings.Contains(errorMsg, "translation too similar"):
		return "相似度检查失败"
	case strings.Contains(errorMsg, "invalid response"):
		return "invalid_response"
	case strings.Contains(errorMsg, "authentication"):
		return "auth_error"
	case strings.Contains(errorMsg, "quota"):
		return "quota_exceeded"
	case strings.Contains(errorMsg, "translation not found"):
		return "translation_not_found"
	default:
		return "未知错误"
	}
}

// extractErrorDetails 提取错误详细信息，包括步骤信息
func extractErrorDetails(err error) (errorType, step string, stepIndex int) {
	if err == nil {
		return "unknown", "", -1
	}
	
	// 检查是否是 TranslationError
	if transErr, ok := err.(*translation.TranslationError); ok {
		errorType := classifyErrorType(transErr)
		step := transErr.Step
		stepIndex := getStepIndex(step)
		return errorType, step, stepIndex
	}
	
	// 递归检查嵌套错误
	if unwrapped := errors.Unwrap(err); unwrapped != nil {
		if errorType, step, stepIndex := extractErrorDetails(unwrapped); errorType != "unknown" {
			return errorType, step, stepIndex
		}
	}
	
	return classifyErrorType(err), "", -1
}

// getStepIndex 获取步骤索引
func getStepIndex(step string) int {
	switch step {
	case "initial_translation":
		return 1
	case "reflection":
		return 2
	case "improvement":
		return 3
	default:
		return -1
	}
}
