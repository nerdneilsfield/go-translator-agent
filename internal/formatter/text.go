package formatter

import (
	"bytes"
	"context"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	xunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// TextFormatter 文本格式化器
type TextFormatter struct {
	name     string
	priority int
}

// NewTextFormatter 创建文本格式化器
func NewTextFormatter() *TextFormatter {
	return &TextFormatter{
		name:     "text",
		priority: 10,
	}
}

// FormatString 格式化文本内容（测试使用的接口）
func (f *TextFormatter) FormatString(ctx context.Context, content string, format string, opts *FormatOptions) (string, error) {
	// 调用内部格式化方法
	formatOpts := FormatOptions{}
	if opts != nil {
		formatOpts = *opts
	}
	result, err := f.FormatBytes([]byte(content), formatOpts)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// FormatBytes 格式化字节内容（实际的格式化实现）
func (f *TextFormatter) FormatBytes(content []byte, opts FormatOptions) ([]byte, error) {
	// 检测并转换编码
	text, err := f.detectAndConvertEncoding(content)
	if err != nil {
		return nil, &FormatError{
			Formatter: f.name,
			Reason:    "failed to detect encoding",
			Err:       err,
		}
	}

	// 基本格式化
	result := f.formatText(text, opts)

	return []byte(result), nil
}

// detectAndConvertEncoding 检测并转换文本编码
func (f *TextFormatter) detectAndConvertEncoding(data []byte) (string, error) {
	// 如果是空数据，直接返回
	if len(data) == 0 {
		return "", nil
	}

	// 检查 UTF-8
	if utf8.Valid(data) {
		return string(data), nil
	}

	// 检查 BOM
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		// UTF-8 BOM
		return string(data[3:]), nil
	}

	if len(data) >= 2 {
		if data[0] == 0xFF && data[1] == 0xFE {
			// UTF-16 LE BOM
			dec := xunicode.UTF16(xunicode.LittleEndian, xunicode.IgnoreBOM).NewDecoder()
			res, err := io.ReadAll(transform.NewReader(bytes.NewReader(data[2:]), dec))
			if err == nil && utf8.Valid(res) {
				return string(res), nil
			}
		} else if data[0] == 0xFE && data[1] == 0xFF {
			// UTF-16 BE BOM
			dec := xunicode.UTF16(xunicode.BigEndian, xunicode.IgnoreBOM).NewDecoder()
			res, err := io.ReadAll(transform.NewReader(bytes.NewReader(data[2:]), dec))
			if err == nil && utf8.Valid(res) {
				return string(res), nil
			}
		}
	}

	// 尝试常见编码
	encodings := []encoding.Encoding{
		simplifiedchinese.GBK,
		simplifiedchinese.GB18030,
		traditionalchinese.Big5,
		japanese.ShiftJIS,
		japanese.EUCJP,
		korean.EUCKR,
		charmap.Windows1252,
		charmap.ISO8859_1,
		xunicode.UTF16(xunicode.LittleEndian, xunicode.IgnoreBOM),
		xunicode.UTF16(xunicode.BigEndian, xunicode.IgnoreBOM),
	}

	for _, enc := range encodings {
		dec := enc.NewDecoder()
		res, err := io.ReadAll(transform.NewReader(bytes.NewReader(data), dec))
		if err == nil && utf8.Valid(res) {
			// 进一步验证是否合理
			if f.isReasonableText(string(res)) {
				return string(res), nil
			}
		}
	}

	// 如果都失败了，返回原始数据
	return string(data), nil
}

// isReasonableText 检查文本是否合理
func (f *TextFormatter) isReasonableText(text string) bool {
	if len(text) == 0 {
		return false
	}

	// 统计可打印字符比例
	printableCount := 0
	for _, r := range text {
		if unicode.IsPrint(r) || unicode.IsSpace(r) {
			printableCount++
		}
	}

	// 如果超过 90% 是可打印字符，认为是合理的文本
	return float64(printableCount)/float64(len([]rune(text))) > 0.9
}

// formatText 格式化文本
func (f *TextFormatter) formatText(text string, opts FormatOptions) string {
	// 标准化换行符
	if opts.LineEnding == "" {
		opts.LineEnding = "\n"
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	// 处理空白字符
	if !opts.PreserveWhitespace {
		// 移除每行末尾的空白
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimRight(line, " \t")
		}
		text = strings.Join(lines, "\n")

		// Tab 转换为空格
		if opts.TabSize > 0 {
			spaces := strings.Repeat(" ", opts.TabSize)
			text = strings.ReplaceAll(text, "\t", spaces)
		}
	}

	// 处理段落格式
	if opts.MaxLineLength > 0 {
		text = f.formatParagraphs(text, opts.MaxLineLength)
	}

	// 移除文件开头和结尾的空行
	text = strings.TrimSpace(text)

	// 确保文件以换行符结尾
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}

	// 应用指定的换行符
	if opts.LineEnding != "\n" {
		text = strings.ReplaceAll(text, "\n", opts.LineEnding)
	}

	return text
}

// formatParagraphs 格式化段落
func (f *TextFormatter) formatParagraphs(content string, maxLineLength int) string {
	paragraphs := strings.Split(content, "\n\n")
	var result []string

	for _, para := range paragraphs {
		// 移除段落内的换行
		para = strings.ReplaceAll(para, "\n", " ")
		para = strings.TrimSpace(para)

		if para == "" {
			result = append(result, "")
			continue
		}

		// 根据内容类型选择格式化方式
		if f.isChineseParagraph(para) {
			// 中文段落按字符数换行
			formatted := f.formatChineseParagraph(para, maxLineLength)
			result = append(result, formatted)
		} else {
			// 英文或混合段落按词换行
			formatted := f.formatEnglishParagraph(para, maxLineLength)
			result = append(result, formatted)
		}
	}

	return strings.Join(result, "\n\n")
}

// isChineseParagraph 判断是否为中文段落
func (f *TextFormatter) isChineseParagraph(s string) bool {
	var chineseCount, asciiCount int
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			chineseCount++
		} else if r < 128 {
			asciiCount++
		}
	}
	// 当中文字符多于 ASCII 字符时，视为中文段落
	return chineseCount > asciiCount
}

// formatChineseParagraph 格式化中文段落
func (f *TextFormatter) formatChineseParagraph(para string, maxLength int) string {
	if len([]rune(para)) <= maxLength {
		return para
	}

	var lines []string
	runes := []rune(para)

	for i := 0; i < len(runes); i += maxLength {
		end := i + maxLength
		if end > len(runes) {
			end = len(runes)
		}
		lines = append(lines, string(runes[i:end]))
	}

	return strings.Join(lines, "\n")
}

// formatEnglishParagraph 格式化英文段落
func (f *TextFormatter) formatEnglishParagraph(para string, maxLength int) string {
	words := strings.Fields(para)
	if len(words) == 0 {
		return ""
	}

	var lines []string
	var currentLine strings.Builder

	for _, word := range words {
		// 如果单个词就超过长度限制，单独成行
		if len(word) > maxLength {
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
			}
			lines = append(lines, word)
			continue
		}

		// 如果当前行加上新词会超出长度限制
		if currentLine.Len() > 0 && currentLine.Len()+1+len(word) > maxLength {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
		}

		// 添加空格（除了行首）
		if currentLine.Len() > 0 {
			currentLine.WriteString(" ")
		}

		currentLine.WriteString(word)
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return strings.Join(lines, "\n")
}

// GetMetadata 返回格式化器元数据
func (f *TextFormatter) GetMetadata() FormatterMetadata {
	return FormatterMetadata{
		Name:        "text",
		Type:        "internal",
		Description: "Text formatter with encoding detection and line ending normalization",
		Formats:     []string{"text", "txt", "plain"},
		Priority:    10,
	}
}

// Format 实现 Formatter 接口
func (f *TextFormatter) Format(content []byte, opts FormatOptions) ([]byte, error) {
	return f.FormatBytes(content, opts)
}

// CanFormat 检查是否支持格式
func (f *TextFormatter) CanFormat(format string) bool {
	return format == "text" || format == "txt" || format == "plain"
}

// Priority 返回优先级
func (f *TextFormatter) Priority() int {
	return f.priority
}

// Name 返回格式化器名称
func (f *TextFormatter) Name() string {
	return f.name
}
