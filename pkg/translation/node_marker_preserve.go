package translation

import (
	"fmt"
	"strings"
)

// NodeMarkerPreserveConfig 节点标记保护配置
type NodeMarkerPreserveConfig struct {
	Enabled         bool
	StartMarkerFmt  string // 格式如 "@@NODE_START_%d@@"
	EndMarkerFmt    string // 格式如 "@@NODE_END_%d@@"
}

// DefaultNodeMarkerConfig 默认节点标记配置
var DefaultNodeMarkerConfig = NodeMarkerPreserveConfig{
	Enabled:        true,
	StartMarkerFmt: "@@NODE_START_%d@@",
	EndMarkerFmt:   "@@NODE_END_%d@@",
}

// GetNodeMarkerPrompt 获取节点标记保护的 prompt 说明
func GetNodeMarkerPrompt(config NodeMarkerPreserveConfig) string {
	if !config.Enabled {
		return ""
	}

	return fmt.Sprintf(`
CRITICAL: Node Boundary Markers
- The text contains special node markers: %s and %s (where N is a number)
- These markers indicate boundaries between different text nodes
- You MUST preserve these markers exactly as they appear
- DO NOT translate, modify, or remove these markers
- Each section between matching start/end markers should be translated independently
- Keep the exact same structure with all markers in place
- Example:
  %s
  [Translate this content]
  %s`,
		strings.Replace(config.StartMarkerFmt, "%d", "N", 1),
		strings.Replace(config.EndMarkerFmt, "%d", "N", 1),
		fmt.Sprintf(config.StartMarkerFmt, 1),
		fmt.Sprintf(config.EndMarkerFmt, 1),
	)
}

// AppendNodeMarkerPrompt 向现有 prompt 追加节点标记保护说明
func AppendNodeMarkerPrompt(prompt string, config NodeMarkerPreserveConfig) string {
	markerPrompt := GetNodeMarkerPrompt(config)
	if markerPrompt == "" {
		return prompt
	}
	return prompt + "\n" + markerPrompt
}

// CombinePromptWithPreserves 组合 prompt 与多种保护说明
func CombinePromptWithPreserves(basePrompt string, preserveConfig PreserveConfig, nodeMarkerConfig NodeMarkerPreserveConfig) string {
	// 先添加内容保护块说明（如 LaTeX、代码块等）
	prompt := AppendPreservePrompt(basePrompt, preserveConfig)
	
	// 再添加节点标记保护说明
	prompt = AppendNodeMarkerPrompt(prompt, nodeMarkerConfig)
	
	return prompt
}

// ExtractNodeID 从节点标记中提取ID
func ExtractNodeID(marker string, config NodeMarkerPreserveConfig) (int, error) {
	// 这是一个辅助函数，用于从标记中提取节点ID
	var id int
	_, err := fmt.Sscanf(marker, config.StartMarkerFmt, &id)
	if err != nil {
		_, err = fmt.Sscanf(marker, config.EndMarkerFmt, &id)
	}
	return id, err
}

// WrapTextWithNodeMarkers 用节点标记包装文本
func WrapTextWithNodeMarkers(nodeID int, text string, config NodeMarkerPreserveConfig) string {
	return fmt.Sprintf("%s\n%s\n%s",
		fmt.Sprintf(config.StartMarkerFmt, nodeID),
		text,
		fmt.Sprintf(config.EndMarkerFmt, nodeID),
	)
}