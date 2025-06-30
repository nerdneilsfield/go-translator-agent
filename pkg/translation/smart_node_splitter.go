package translation

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"go.uber.org/zap"
)

// SmartNodeSplitterConfig 智能节点分割器配置
type SmartNodeSplitterConfig struct {
	EnableSmartSplitting bool    `json:"enable_smart_splitting"`  // 是否启用智能分割
	MaxNodeSizeThreshold int     `json:"max_node_size_threshold"` // 超过这个阈值才进行分割（字符数）
	MinSplitSize         int     `json:"min_split_size"`          // 分割后每部分的最小大小
	MaxSplitSize         int     `json:"max_split_size"`          // 分割后每部分的最大大小
	PreserveParagraphs   bool    `json:"preserve_paragraphs"`     // 是否保持段落完整性
	PreserveSentences    bool    `json:"preserve_sentences"`      // 是否保持句子完整性
	OverlapRatio         float64 `json:"overlap_ratio"`           // 重叠比例（0.0-0.3）
}

// DefaultSmartNodeSplitterConfig 返回默认配置
func DefaultSmartNodeSplitterConfig() SmartNodeSplitterConfig {
	return SmartNodeSplitterConfig{
		EnableSmartSplitting: false, // 默认不启用，需要显式配置
		MaxNodeSizeThreshold: 1500,  // 超过1500字符才分割
		MinSplitSize:         500,   // 最小500字符
		MaxSplitSize:         1000,  // 最大1000字符
		PreserveParagraphs:   true,
		PreserveSentences:    true,
		OverlapRatio:         0.1, // 10%重叠
	}
}

// SmartNodeSplitter 智能节点分割器
type SmartNodeSplitter struct {
	config SmartNodeSplitterConfig
	logger *zap.Logger
}

// NewSmartNodeSplitter 创建智能节点分割器
func NewSmartNodeSplitter(config SmartNodeSplitterConfig, logger *zap.Logger) *SmartNodeSplitter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SmartNodeSplitter{
		config: config,
		logger: logger,
	}
}

// ShouldSplit 判断节点是否需要分割
func (s *SmartNodeSplitter) ShouldSplit(node *document.NodeInfo) bool {
	if !s.config.EnableSmartSplitting {
		return false
	}

	textLength := utf8.RuneCountInString(node.OriginalText)
	return textLength > s.config.MaxNodeSizeThreshold
}

// SplitNode 智能分割节点
func (s *SmartNodeSplitter) SplitNode(node *document.NodeInfo, nextNodeID *int) ([]*document.NodeInfo, error) {
	if !s.ShouldSplit(node) {
		return []*document.NodeInfo{node}, nil
	}

	s.logger.Debug("splitting oversized node",
		zap.Int("nodeID", node.ID),
		zap.Int("originalSize", utf8.RuneCountInString(node.OriginalText)),
		zap.Int("threshold", s.config.MaxNodeSizeThreshold))

	// 根据内容类型选择分割策略
	detector := NewContentTypeDetector()
	contentType := detector.DetectContentType(node.OriginalText)

	var segments []string
	var err error

	switch contentType {
	case ContentTypeCodeBlock:
		segments, err = s.splitCodeBlock(node.OriginalText)
	case ContentTypeList:
		segments, err = s.splitList(node.OriginalText)
	case ContentTypeTable:
		segments, err = s.splitTable(node.OriginalText)
	default:
		segments, err = s.splitPlainText(node.OriginalText)
	}

	if err != nil {
		s.logger.Error("failed to split node",
			zap.Int("nodeID", node.ID),
			zap.Error(err))
		return []*document.NodeInfo{node}, err
	}

	// 创建子节点
	var subNodes []*document.NodeInfo
	for i, segment := range segments {
		subNode := &document.NodeInfo{
			ID:           *nextNodeID,
			Type:         node.Type,
			OriginalText: segment,
			Status:       document.NodeStatusPending,
			Parent:       node,
			SplitIndex:   i,
		}
		subNodes = append(subNodes, subNode)
		*nextNodeID++
	}

	s.logger.Info("node split completed",
		zap.Int("originalNodeID", node.ID),
		zap.Int("originalSize", utf8.RuneCountInString(node.OriginalText)),
		zap.Int("subNodesCount", len(subNodes)),
		zap.String("contentType", string(contentType)))

	return subNodes, nil
}

// ContentType 内容类型
type ContentType string

const (
	ContentTypePlainText ContentType = "plain_text"
	ContentTypeCodeBlock ContentType = "code_block"
	ContentTypeList      ContentType = "list"
	ContentTypeTable     ContentType = "table"
	ContentTypeMath      ContentType = "math"
)

// ContentTypeDetector 内容类型检测器
type ContentTypeDetector struct {
	codeBlockPattern *regexp.Regexp
	listPattern      *regexp.Regexp
	tablePattern     *regexp.Regexp
	mathPattern      *regexp.Regexp
}

// NewContentTypeDetector 创建内容类型检测器
func NewContentTypeDetector() *ContentTypeDetector {
	return &ContentTypeDetector{
		codeBlockPattern: regexp.MustCompile(`(?m)^` + "```" + `[\s\S]*?^` + "```" + `$`),
		listPattern:      regexp.MustCompile(`(?m)^[\s]*[-*+]\s+|^[\s]*\d+\.\s+`),
		tablePattern:     regexp.MustCompile(`(?m)^\|.*\|.*$`),
		mathPattern:      regexp.MustCompile(`\$\$[\s\S]*?\$\$|\$[^$\n]+\$`),
	}
}

// DetectContentType 检测内容类型
func (d *ContentTypeDetector) DetectContentType(text string) ContentType {
	// 检查代码块
	if d.codeBlockPattern.MatchString(text) {
		return ContentTypeCodeBlock
	}

	// 检查表格
	lines := strings.Split(text, "\n")
	tableLineCount := 0
	for _, line := range lines {
		if d.tablePattern.MatchString(line) {
			tableLineCount++
		}
	}
	if tableLineCount >= 2 { // 至少两行表格才认为是表格
		return ContentTypeTable
	}

	// 检查列表
	listLineCount := 0
	for _, line := range lines {
		if d.listPattern.MatchString(line) {
			listLineCount++
		}
	}
	if listLineCount >= 2 { // 至少两个列表项才认为是列表
		return ContentTypeList
	}

	// 检查数学公式
	if d.mathPattern.MatchString(text) {
		return ContentTypeMath
	}

	return ContentTypePlainText
}

// splitPlainText 分割纯文本
func (s *SmartNodeSplitter) splitPlainText(text string) ([]string, error) {
	if s.config.PreserveParagraphs {
		return s.splitByParagraphs(text)
	}
	if s.config.PreserveSentences {
		return s.splitBySentences(text)
	}
	return s.splitByCharacters(text), nil
}

// splitByParagraphs 按段落分割
func (s *SmartNodeSplitter) splitByParagraphs(text string) ([]string, error) {
	paragraphs := strings.Split(text, "\n\n")
	return s.combineSegments(paragraphs), nil
}

// splitBySentences 按句子分割
func (s *SmartNodeSplitter) splitBySentences(text string) ([]string, error) {
	sentences := s.extractSentences(text)
	return s.combineSegments(sentences), nil
}

// extractSentences 提取句子（支持多语言）
func (s *SmartNodeSplitter) extractSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	runes := []rune(text)
	inQuote := false

	for i, r := range runes {
		current.WriteRune(r)

		// 处理引号
		if r == '"' || r == '\'' || r == 0x201C || r == 0x201D || r == 0x2018 || r == 0x2019 {
			inQuote = !inQuote
		}

		// 句子结束符（多语言支持）
		if !inQuote && s.isSentenceEnd(r) {
			// 检查下一个字符
			if i+1 < len(runes) {
				next := runes[i+1]
				// 如果下一个是空格、换行或文本结束，则确认句子结束
				if unicode.IsSpace(next) || i+1 == len(runes)-1 {
					sentence := strings.TrimSpace(current.String())
					if sentence != "" && utf8.RuneCountInString(sentence) > 10 { // 过滤太短的句子
						sentences = append(sentences, sentence)
						current.Reset()
					}
				}
			} else {
				// 文本结束
				sentence := strings.TrimSpace(current.String())
				if sentence != "" {
					sentences = append(sentences, sentence)
					current.Reset()
				}
			}
		}
	}

	// 处理剩余文本
	if current.Len() > 0 {
		sentence := strings.TrimSpace(current.String())
		if sentence != "" {
			sentences = append(sentences, sentence)
		}
	}

	return sentences
}

// isSentenceEnd 判断是否为句子结束符（多语言）
func (s *SmartNodeSplitter) isSentenceEnd(r rune) bool {
	// 英文句号、感叹号、问号
	if r == '.' || r == '!' || r == '?' {
		return true
	}

	// 中文句号、感叹号、问号
	if r == '。' || r == '！' || r == '？' {
		return true
	}

	// 日文句号
	if r == '。' {
		return true
	}

	// 其他语言的句子结束符可以在这里添加

	return false
}

// combineSegments 组合片段到合适的大小
func (s *SmartNodeSplitter) combineSegments(segments []string) []string {
	if len(segments) == 0 {
		return []string{}
	}

	var result []string
	var current strings.Builder
	currentSize := 0

	for i, segment := range segments {
		segmentSize := utf8.RuneCountInString(segment)

		// 如果单个片段就超过最大大小，需要进一步分割
		if segmentSize > s.config.MaxSplitSize {
			// 保存当前块
			if current.Len() > 0 {
				result = append(result, strings.TrimSpace(current.String()))
				current.Reset()
				currentSize = 0
			}

			// 分割大片段
			subSegments := s.splitByCharacters(segment)
			result = append(result, subSegments...)
			continue
		}

		// 如果添加这个片段会超过大小限制
		if currentSize > 0 && currentSize+segmentSize+1 > s.config.MaxSplitSize {
			result = append(result, strings.TrimSpace(current.String()))

			// 处理重叠
			if s.config.OverlapRatio > 0 && i > 0 {
				overlapText := s.getOverlapText(current.String())
				current.Reset()
				if overlapText != "" {
					current.WriteString(overlapText)
					current.WriteString("\n")
					currentSize = utf8.RuneCountInString(overlapText) + 1
				} else {
					currentSize = 0
				}
			} else {
				current.Reset()
				currentSize = 0
			}
		}

		// 添加片段
		if current.Len() > 0 {
			current.WriteString("\n")
			currentSize++
		}
		current.WriteString(segment)
		currentSize += segmentSize
	}

	// 添加最后一个块
	if current.Len() > 0 {
		result = append(result, strings.TrimSpace(current.String()))
	}

	return result
}

// getOverlapText 获取重叠文本
func (s *SmartNodeSplitter) getOverlapText(text string) string {
	if s.config.OverlapRatio <= 0 {
		return ""
	}

	runes := []rune(text)
	overlapSize := int(float64(len(runes)) * s.config.OverlapRatio)

	if overlapSize == 0 || overlapSize >= len(runes) {
		return ""
	}

	// 从末尾获取重叠部分
	overlapStart := len(runes) - overlapSize
	return string(runes[overlapStart:])
}

// splitByCharacters 按字符强制分割
func (s *SmartNodeSplitter) splitByCharacters(text string) []string {
	var segments []string
	runes := []rune(text)

	for i := 0; i < len(runes); i += s.config.MaxSplitSize {
		end := i + s.config.MaxSplitSize
		if end > len(runes) {
			end = len(runes)
		}
		segment := string(runes[i:end])
		segments = append(segments, segment)
	}

	return segments
}

// splitCodeBlock 分割代码块
func (s *SmartNodeSplitter) splitCodeBlock(text string) ([]string, error) {
	lines := strings.Split(text, "\n")
	var segments []string
	var current strings.Builder
	currentSize := 0

	inCodeBlock := false
	codeBlockStart := ""

	for _, line := range lines {
		lineSize := utf8.RuneCountInString(line) + 1 // +1 for newline

		// 检测代码块边界
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			if !inCodeBlock {
				codeBlockStart = line
				inCodeBlock = true
			} else if strings.HasPrefix(line, codeBlockStart[:3]) {
				inCodeBlock = false
			}
		}

		// 如果添加这行会超过限制且不在代码块中
		if !inCodeBlock && currentSize > 0 && currentSize+lineSize > s.config.MaxSplitSize {
			segments = append(segments, strings.TrimSpace(current.String()))
			current.Reset()
			currentSize = 0
		}

		// 添加行
		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
		currentSize += lineSize
	}

	if current.Len() > 0 {
		segments = append(segments, strings.TrimSpace(current.String()))
	}

	return segments, nil
}

// splitList 分割列表
func (s *SmartNodeSplitter) splitList(text string) ([]string, error) {
	lines := strings.Split(text, "\n")
	var segments []string
	var current strings.Builder
	currentSize := 0

	for _, line := range lines {
		lineSize := utf8.RuneCountInString(line) + 1
		isListItem := s.isListItem(line)

		// 如果是新的列表项且会超过限制，保存当前块
		if isListItem && currentSize > 0 && currentSize+lineSize > s.config.MaxSplitSize {
			segments = append(segments, strings.TrimSpace(current.String()))
			current.Reset()
			currentSize = 0
		}

		// 添加行
		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
		currentSize += lineSize
	}

	if current.Len() > 0 {
		segments = append(segments, strings.TrimSpace(current.String()))
	}

	return segments, nil
}

// splitTable 分割表格
func (s *SmartNodeSplitter) splitTable(text string) ([]string, error) {
	lines := strings.Split(text, "\n")
	var segments []string
	var current strings.Builder
	currentSize := 0

	headerProcessed := false

	for i, line := range lines {
		lineSize := utf8.RuneCountInString(line) + 1
		isTableRow := strings.Contains(line, "|")

		// 表格头部（前两行）总是保留在一起
		if !headerProcessed && isTableRow {
			if current.Len() > 0 {
				current.WriteString("\n")
			}
			current.WriteString(line)
			currentSize += lineSize

			// 检查下一行是否是分隔符
			if i+1 < len(lines) && strings.Contains(lines[i+1], "|") && strings.Contains(lines[i+1], "-") {
				current.WriteString("\n")
				current.WriteString(lines[i+1])
				currentSize += utf8.RuneCountInString(lines[i+1]) + 1
				headerProcessed = true
				continue
			}
		} else if isTableRow && currentSize > 0 && currentSize+lineSize > s.config.MaxSplitSize {
			// 保存当前表格块
			segments = append(segments, strings.TrimSpace(current.String()))
			current.Reset()
			currentSize = 0
		}

		// 添加行
		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
		currentSize += lineSize
	}

	if current.Len() > 0 {
		segments = append(segments, strings.TrimSpace(current.String()))
	}

	return segments, nil
}

// isListItem 检查是否为列表项
func (s *SmartNodeSplitter) isListItem(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	// 无序列表
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
		return true
	}

	// 有序列表
	for i, r := range trimmed {
		if !unicode.IsDigit(r) {
			if i > 0 && (r == '.' || r == ')') && i < len(trimmed)-1 && trimmed[i+1] == ' ' {
				return true
			}
			break
		}
	}

	return false
}
