package translator

import (
	"fmt"
	"strings"
)

// DiagnoseBatchTranslationIssue 诊断批量翻译问题
func DiagnoseBatchTranslationIssue(request string, response string) *BatchDiagnostic {
	diag := &BatchDiagnostic{
		RequestLength:  len(request),
		ResponseLength: len(response),
	}

	// 检查请求中的节点标记
	diag.RequestStartMarkers = countOccurrences(request, "@@NODE_START_")
	diag.RequestEndMarkers = countOccurrences(request, "@@NODE_END_")

	// 检查响应中的节点标记
	diag.ResponseStartMarkers = countOccurrences(response, "@@NODE_START_")
	diag.ResponseEndMarkers = countOccurrences(response, "@@NODE_END_")

	// 检查标记是否平衡
	diag.RequestMarkersBalanced = diag.RequestStartMarkers == diag.RequestEndMarkers
	diag.ResponseMarkersBalanced = diag.ResponseStartMarkers == diag.ResponseEndMarkers

	// 检查标记是否被保留
	diag.MarkersPreserved = diag.RequestStartMarkers == diag.ResponseStartMarkers

	// 检查响应格式
	diag.ResponseIsJSON = strings.HasPrefix(strings.TrimSpace(response), "{") ||
		strings.HasPrefix(strings.TrimSpace(response), "[")
	diag.ResponseIsEmpty = len(strings.TrimSpace(response)) == 0

	// 检查可能的标记变体
	variants := []string{
		"NODE_START_",
		"@@NODE_",
		"NODE ",
		"<NODE",
		"[NODE",
		"{NODE",
	}

	diag.AlternativeMarkers = make(map[string]int)
	for _, variant := range variants {
		count := countOccurrences(response, variant)
		if count > 0 {
			diag.AlternativeMarkers[variant] = count
		}
	}

	// 诊断可能的问题
	diag.Issues = diagnoseProbableIssues(diag)

	return diag
}

// BatchDiagnostic 批量翻译诊断结果
type BatchDiagnostic struct {
	RequestLength           int
	ResponseLength          int
	RequestStartMarkers     int
	RequestEndMarkers       int
	ResponseStartMarkers    int
	ResponseEndMarkers      int
	RequestMarkersBalanced  bool
	ResponseMarkersBalanced bool
	MarkersPreserved        bool
	ResponseIsJSON          bool
	ResponseIsEmpty         bool
	AlternativeMarkers      map[string]int
	Issues                  []string
}

// countOccurrences 计算字符串出现次数
func countOccurrences(text, substr string) int {
	return strings.Count(text, substr)
}

// diagnoseProbableIssues 诊断可能的问题
func diagnoseProbableIssues(diag *BatchDiagnostic) []string {
	var issues []string

	if diag.ResponseIsEmpty {
		issues = append(issues, "响应为空")
		return issues
	}

	if diag.ResponseIsJSON {
		issues = append(issues, "响应是JSON格式而不是纯文本")
	}

	if !diag.RequestMarkersBalanced {
		issues = append(issues, fmt.Sprintf("请求中的标记不平衡: %d个开始标记, %d个结束标记",
			diag.RequestStartMarkers, diag.RequestEndMarkers))
	}

	if diag.ResponseStartMarkers == 0 && diag.ResponseEndMarkers == 0 {
		issues = append(issues, "响应中完全没有节点标记")
		if len(diag.AlternativeMarkers) > 0 {
			issues = append(issues, fmt.Sprintf("但发现了可能的变体标记: %v", diag.AlternativeMarkers))
		}
	} else if !diag.ResponseMarkersBalanced {
		issues = append(issues, fmt.Sprintf("响应中的标记不平衡: %d个开始标记, %d个结束标记",
			diag.ResponseStartMarkers, diag.ResponseEndMarkers))
	}

	if diag.RequestStartMarkers > 0 && !diag.MarkersPreserved {
		issues = append(issues, fmt.Sprintf("标记数量不匹配: 请求有%d个节点，响应有%d个节点",
			diag.RequestStartMarkers, diag.ResponseStartMarkers))
	}

	if diag.ResponseLength < diag.RequestLength/2 {
		issues = append(issues, "响应长度异常短，可能翻译不完整")
	}

	return issues
}

// FormatDiagnostic 格式化诊断结果
func (diag *BatchDiagnostic) Format() string {
	var sb strings.Builder

	sb.WriteString("=== 批量翻译诊断结果 ===\n")
	sb.WriteString(fmt.Sprintf("请求长度: %d\n", diag.RequestLength))
	sb.WriteString(fmt.Sprintf("响应长度: %d\n", diag.ResponseLength))
	sb.WriteString(fmt.Sprintf("\n请求标记:\n"))
	sb.WriteString(fmt.Sprintf("  开始标记: %d\n", diag.RequestStartMarkers))
	sb.WriteString(fmt.Sprintf("  结束标记: %d\n", diag.RequestEndMarkers))
	sb.WriteString(fmt.Sprintf("  平衡: %v\n", diag.RequestMarkersBalanced))
	sb.WriteString(fmt.Sprintf("\n响应标记:\n"))
	sb.WriteString(fmt.Sprintf("  开始标记: %d\n", diag.ResponseStartMarkers))
	sb.WriteString(fmt.Sprintf("  结束标记: %d\n", diag.ResponseEndMarkers))
	sb.WriteString(fmt.Sprintf("  平衡: %v\n", diag.ResponseMarkersBalanced))
	sb.WriteString(fmt.Sprintf("\n格式检查:\n"))
	sb.WriteString(fmt.Sprintf("  标记保留: %v\n", diag.MarkersPreserved))
	sb.WriteString(fmt.Sprintf("  响应是JSON: %v\n", diag.ResponseIsJSON))
	sb.WriteString(fmt.Sprintf("  响应为空: %v\n", diag.ResponseIsEmpty))

	if len(diag.AlternativeMarkers) > 0 {
		sb.WriteString(fmt.Sprintf("\n发现的变体标记:\n"))
		for variant, count := range diag.AlternativeMarkers {
			sb.WriteString(fmt.Sprintf("  %s: %d次\n", variant, count))
		}
	}

	if len(diag.Issues) > 0 {
		sb.WriteString(fmt.Sprintf("\n诊断出的问题:\n"))
		for _, issue := range diag.Issues {
			sb.WriteString(fmt.Sprintf("  - %s\n", issue))
		}
	} else {
		sb.WriteString(fmt.Sprintf("\n未发现明显问题\n"))
	}

	return sb.String()
}
