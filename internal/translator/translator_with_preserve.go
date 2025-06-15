package translator

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"go.uber.org/zap"
)

// translateNodeWithPreserve 带保护块的节点翻译
func (c *TranslationCoordinator) translateNodeWithPreserve(ctx context.Context, node *document.NodeInfo) error {
	// 检查翻译服务是否可用
	if c.translationService == nil {
		// 模拟翻译（用于测试）
		node.TranslatedText = "Translated: " + node.OriginalText
		node.Status = document.NodeStatusSuccess
		return nil
	}

	// 创建保护块管理器
	preserveManager := translation.NewPreserveManager(translation.DefaultPreserveConfig)
	
	// 处理需要保护的内容
	textToTranslate := protectContent(node.OriginalText, preserveManager)

	// 创建翻译请求
	req := &translation.Request{
		Text:           textToTranslate,
		SourceLanguage: c.config.SourceLang,
		TargetLanguage: c.config.TargetLang,
		Metadata: map[string]interface{}{
			"node_id": node.ID,
			"preserve_config": translation.DefaultPreserveConfig,
			"_preserve_enabled": "true", // 添加上下文标记，让 chain.go 知道要添加保护说明
		},
	}

	// 执行翻译
	resp, err := c.translationService.Translate(ctx, req)
	if err != nil {
		node.Status = document.NodeStatusFailed
		node.Error = err
		c.logger.Error("translation failed",
			zap.Int("nodeID", node.ID),
			zap.Error(err))
		return err
	}

	// 还原保护的内容
	translatedText := preserveManager.Restore(resp.Text)
	
	// 应用翻译后处理
	if c.postProcessor != nil && c.config.EnablePostProcessing {
		processedText, err := c.postProcessor.ProcessTranslation(ctx, node.OriginalText, translatedText, nil)
		if err != nil {
			c.logger.Warn("translation post processing failed",
				zap.Int("nodeID", node.ID),
				zap.Error(err))
			// 不让后处理失败阻止翻译过程
		} else {
			translatedText = processedText
		}
	}

	// 更新节点
	node.TranslatedText = translatedText
	node.Status = document.NodeStatusSuccess

	c.logger.Debug("node translation completed",
		zap.Int("nodeID", node.ID),
		zap.Int("originalLength", len(node.OriginalText)),
		zap.Int("translatedLength", len(node.TranslatedText)))

	return nil
}

// protectContent 保护不需要翻译的内容
func protectContent(text string, pm *translation.PreserveManager) string {
	// 保护 LaTeX 公式
	text = protectLatex(text, pm)
	
	// 保护代码块
	text = protectCodeBlocks(text, pm)
	
	// 保护 HTML 标签（可选）
	// text = protectHTMLTags(text, pm)
	
	// 保护 URL
	text = protectURLs(text, pm)
	
	return text
}

// protectLatex 保护 LaTeX 公式
func protectLatex(text string, pm *translation.PreserveManager) string {
	// 保护行内公式 $...$
	text = protectPattern(text, `\$[^$]+\$`, pm)
	
	// 保护行间公式 $$...$$
	text = protectPattern(text, `\$\$[^$]+\$\$`, pm)
	
	// 保护 \(...\)
	text = protectPattern(text, `\\\([^)]+\\\)`, pm)
	
	// 保护 \[...\]
	text = protectPattern(text, `\\\[[^\]]+\\\]`, pm)
	
	return text
}

// protectCodeBlocks 保护代码块
func protectCodeBlocks(text string, pm *translation.PreserveManager) string {
	// 保护行内代码 `...`
	text = protectPattern(text, "`[^`]+`", pm)
	
	// 保护代码块 ```...```
	// 这需要更复杂的处理，因为可能跨行
	lines := strings.Split(text, "\n")
	inCodeBlock := false
	codeBlockContent := []string{}
	result := []string{}
	
	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			if !inCodeBlock {
				// 开始代码块
				inCodeBlock = true
				codeBlockContent = []string{line}
			} else {
				// 结束代码块
				codeBlockContent = append(codeBlockContent, line)
				codeBlock := strings.Join(codeBlockContent, "\n")
				placeholder := pm.Protect(codeBlock)
				result = append(result, placeholder)
				inCodeBlock = false
				codeBlockContent = []string{}
			}
		} else if inCodeBlock {
			codeBlockContent = append(codeBlockContent, line)
		} else {
			result = append(result, line)
		}
	}
	
	// 如果代码块没有正确关闭，把剩余内容加回去
	if inCodeBlock {
		result = append(result, codeBlockContent...)
	}
	
	return strings.Join(result, "\n")
}

// protectURLs 保护 URL
func protectURLs(text string, pm *translation.PreserveManager) string {
	// 简单的 URL 正则
	return protectPattern(text, `https?://[^\s]+`, pm)
}

// protectPattern 使用正则表达式保护匹配的内容
func protectPattern(text string, pattern string, pm *translation.PreserveManager) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		// 如果正则表达式无效，返回原文
		return text
	}
	
	// 查找所有匹配项并替换为占位符
	return re.ReplaceAllStringFunc(text, func(match string) string {
		return pm.Protect(match)
	})
}

// translateGroupWithPreserve 翻译整个节点组（批量翻译）
func (c *TranslationCoordinator) translateGroupWithPreserve(ctx context.Context, group *document.NodeGroup) error {
	// 如果翻译服务不支持批量翻译，则逐个翻译
	if c.translationService == nil {
		for _, node := range group.Nodes {
			if err := c.translateNodeWithPreserve(ctx, node); err != nil {
				return err
			}
		}
		return nil
	}

	// 创建保护块管理器
	preserveManager := translation.NewPreserveManager(translation.DefaultPreserveConfig)
	
	// 使用标记方式合并节点文本（参考 html_utils.go）
	var combinedTextBuilder strings.Builder
	nodeIDMap := make(map[int]*document.NodeInfo)
	
	for _, node := range group.Nodes {
		// 保护不需要翻译的内容
		protectedText := protectContent(node.OriginalText, preserveManager)
		
		// 添加节点标记
		combinedTextBuilder.WriteString(fmt.Sprintf("@@NODE_START_%d@@\n", node.ID))
		combinedTextBuilder.WriteString(protectedText)
		combinedTextBuilder.WriteString(fmt.Sprintf("\n@@NODE_END_%d@@\n", node.ID))
		
		// 保存节点映射
		nodeIDMap[node.ID] = node
	}
	
	combinedText := combinedTextBuilder.String()

	// 创建翻译请求
	req := &translation.Request{
		Text:           combinedText,
		SourceLanguage: c.config.SourceLang,
		TargetLanguage: c.config.TargetLang,
		Metadata: map[string]interface{}{
			"is_batch": true,
			"node_count": len(group.Nodes),
			"preserve_config": translation.DefaultPreserveConfig,
			"preserve_markers": true, // 告诉翻译服务保留节点标记
			"_is_batch": "true", // 添加上下文标记，让 chain.go 知道要添加节点标记保护说明
			"_preserve_enabled": "true", // 添加上下文标记，让 chain.go 知道要添加保护说明
		},
	}

	// 执行翻译
	resp, err := c.translationService.Translate(ctx, req)
	if err != nil {
		// 如果批量翻译失败，回退到单个翻译
		c.logger.Warn("batch translation failed, falling back to individual translation",
			zap.Error(err))
		for _, node := range group.Nodes {
			if err := c.translateNodeWithPreserve(ctx, node); err != nil {
				return err
			}
		}
		return nil
	}

	// 还原保护的内容
	translatedText := preserveManager.Restore(resp.Text)
	
	// 解析翻译结果并恢复到各个节点
	if err := c.parseAndApplyTranslations(ctx, translatedText, nodeIDMap); err != nil {
		c.logger.Error("failed to parse translations, falling back to individual translation",
			zap.Error(err))
		// 回退到单个翻译
		for _, node := range group.Nodes {
			if err := c.translateNodeWithPreserve(ctx, node); err != nil {
				return err
			}
		}
		return nil
	}

	return nil
}

// parseAndApplyTranslations 解析翻译结果并应用到节点
func (c *TranslationCoordinator) parseAndApplyTranslations(ctx context.Context, translatedText string, nodeIDMap map[int]*document.NodeInfo) error {
	// 使用正则表达式解析节点标记
	// 模式：@@NODE_START_(\d+)@@\n(.*?)\n@@NODE_END_\1@@
	pattern := regexp.MustCompile(`(?s)@@NODE_START_(\d+)@@\n(.*?)\n@@NODE_END_\1@@`)
	
	matches := pattern.FindAllStringSubmatch(translatedText, -1)
	if len(matches) == 0 {
		return fmt.Errorf("no node markers found in translated text")
	}
	
	foundNodes := 0
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		
		// match[0] = 完整匹配
		// match[1] = 节点ID
		// match[2] = 翻译内容
		nodeID, err := strconv.Atoi(match[1])
		if err != nil {
			c.logger.Warn("invalid node ID in translation result",
				zap.String("nodeID", match[1]),
				zap.Error(err))
			continue
		}
		
		node, exists := nodeIDMap[nodeID]
		if !exists {
			c.logger.Warn("node ID not found in map",
				zap.Int("nodeID", nodeID))
			continue
		}
		
		translatedContent := strings.TrimSpace(match[2])
		
		// 应用翻译后处理
		if c.postProcessor != nil && c.config.EnablePostProcessing {
			processedText, err := c.postProcessor.ProcessTranslation(ctx, node.OriginalText, translatedContent, nil)
			if err != nil {
				c.logger.Warn("translation post processing failed",
					zap.Int("nodeID", nodeID),
					zap.Error(err))
				// 不让后处理失败阻止翻译过程
			} else {
				translatedContent = processedText
			}
		}
		
		// 更新节点
		node.TranslatedText = translatedContent
		node.Status = document.NodeStatusSuccess
		foundNodes++
		
		c.logger.Debug("applied translation to node",
			zap.Int("nodeID", nodeID),
			zap.Int("originalLength", len(node.OriginalText)),
			zap.Int("translatedLength", len(translatedContent)))
	}
	
	// 检查是否所有节点都被翻译
	if foundNodes != len(nodeIDMap) {
		c.logger.Warn("not all nodes were translated",
			zap.Int("expected", len(nodeIDMap)),
			zap.Int("found", foundNodes))
		
		// 标记未翻译的节点
		for nodeID, node := range nodeIDMap {
			if node.Status != document.NodeStatusSuccess {
				node.Status = document.NodeStatusFailed
				node.Error = fmt.Errorf("node not found in batch translation result")
				c.logger.Warn("node not translated in batch",
					zap.Int("nodeID", nodeID))
			}
		}
	}
	
	return nil
}