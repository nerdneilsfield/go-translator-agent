package translation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// BatchTranslationConfig 批量翻译配置
type BatchTranslationConfig struct {
	// 是否启用批量翻译
	Enabled bool
	// 节点标记格式
	NodeStartMarker string
	NodeEndMarker   string
	// 是否在prompt中添加标记保护说明
	AddMarkerProtection bool
}

// DefaultBatchConfig 默认批量翻译配置
var DefaultBatchConfig = BatchTranslationConfig{
	Enabled:             true,
	NodeStartMarker:     "@@NODE_START_%d@@",
	NodeEndMarker:       "@@NODE_END_%d@@",
	AddMarkerProtection: true,
}

// AddBatchMarkerProtection 在prompt中添加批量翻译标记保护说明
func AddBatchMarkerProtection(prompt string, config BatchTranslationConfig) string {
	if !config.AddMarkerProtection {
		return prompt
	}
	
	markerInstruction := fmt.Sprintf(`
CRITICAL: Node Markers
- The text contains special markers like %s and %s
- These markers MUST be preserved exactly as they appear
- Do NOT translate or modify these markers
- Each piece of text between matching markers should be translated independently
- Keep the markers and the structure intact
- Example:
  %s
  [Your translation of the text goes here]
  %s`,
		fmt.Sprintf(config.NodeStartMarker, "N"),
		fmt.Sprintf(config.NodeEndMarker, "N"),
		fmt.Sprintf(config.NodeStartMarker, "1"),
		fmt.Sprintf(config.NodeEndMarker, "1"),
	)
	
	return prompt + "\n" + markerInstruction
}

// CombineNodesForBatch 将多个节点文本合并为批量翻译格式
func CombineNodesForBatch(nodes []NodeText, config BatchTranslationConfig) string {
	var builder strings.Builder
	
	for _, node := range nodes {
		builder.WriteString(fmt.Sprintf(config.NodeStartMarker+"\n", node.ID))
		builder.WriteString(node.Text)
		builder.WriteString(fmt.Sprintf("\n"+config.NodeEndMarker+"\n", node.ID))
	}
	
	return builder.String()
}

// NodeText 节点文本信息
type NodeText struct {
	ID   int
	Text string
}

// BatchTranslationResult 批量翻译结果
type BatchTranslationResult struct {
	ID             int
	TranslatedText string
}

// ParseBatchTranslation 解析批量翻译结果
func ParseBatchTranslation(translatedText string, config BatchTranslationConfig) ([]BatchTranslationResult, error) {
	results := make([]BatchTranslationResult, 0)
	
	// 构建正则表达式
	// 使用 %d 作为占位符，需要转义
	startPattern := strings.Replace(config.NodeStartMarker, "%d", `(\d+)`, 1)
	endPattern := strings.Replace(config.NodeEndMarker, "%d", `\1`, 1)
	
	// 完整的模式：开始标记 + 内容 + 结束标记
	fullPattern := fmt.Sprintf(`(?s)%s\n(.*?)\n%s`, startPattern, endPattern)
	
	// 编译正则表达式
	re, err := regexp.Compile(fullPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to compile regex: %w", err)
	}
	
	// 查找所有匹配
	matches := re.FindAllStringSubmatch(translatedText, -1)
	
	// 解析匹配结果
	for _, match := range matches {
		if len(match) >= 2 {
			// match[0] 是完整匹配，match[1] 是内容
			result := BatchTranslationResult{
				ID:              len(results),
				TranslatedText: match[1],
			}
			
			// 尝试从开始标记中提取 ID
			nodeStartPattern := strings.Replace(config.NodeStartMarker, "%d", `(\d+)`, 1)
			if startRe, err := regexp.Compile(nodeStartPattern); err == nil {
				if idMatches := startRe.FindStringSubmatch(match[0]); len(idMatches) >= 2 {
					if id, err := strconv.Atoi(idMatches[1]); err == nil {
						result.ID = id
					}
				}
			}
			
			results = append(results, result)
		}
	}
	
	return results, nil
}

// ExtendedStepConfig 扩展的步骤配置，支持批量翻译
type ExtendedStepConfig struct {
	*StepConfig
	// 批量翻译配置
	BatchConfig *BatchTranslationConfig
}

// PrepareBatchPrompt 准备批量翻译的prompt
func PrepareBatchPrompt(basePrompt string, batchConfig BatchTranslationConfig) string {
	if !batchConfig.Enabled {
		return basePrompt
	}
	
	// 在prompt中添加批量翻译的特殊说明
	batchPrompt := basePrompt
	
	// 添加节点标记保护说明
	batchPrompt = AddBatchMarkerProtection(batchPrompt, batchConfig)
	
	// 添加额外的批量翻译指导
	batchPrompt += `

When translating:
1. Translate each section between node markers independently
2. Maintain consistency across all sections
3. Preserve the exact structure and markers
4. Do not merge or split sections`
	
	return batchPrompt
}