package formats

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	unicodeX "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

type TextFormattingProcessor struct {
	MaxLineLength int // 每行最大长度
	TabSize       int // Tab 转换为空格数
}

func NewTextFormattingProcessor() *TextFormattingProcessor {
	return &TextFormattingProcessor{
		MaxLineLength: 80,
		TabSize:       4,
	}
}

func (p *TextFormattingProcessor) FormatFile(inputPath, outputPath string) error {
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("读取文件失败 %s: %w", inputPath, err)
	}
	content, err := p.detectAndConvertEncoding(raw)
	if err != nil {
		return fmt.Errorf("编码转换失败: %w", err)
	}
	formatted := p.FormatText(content)
	if err := os.WriteFile(outputPath, []byte(formatted), 0o644); err != nil {
		return fmt.Errorf("写入文件失败 %s: %w", outputPath, err)
	}
	return nil
}

func (p *TextFormattingProcessor) detectAndConvertEncoding(data []byte) (string, error) {
	if utf8.Valid(data) {
		return string(data), nil
	}
	// 检查 BOM
	if len(data) >= 2 {
		if data[0] == 0xFF && data[1] == 0xFE {
			dec := unicodeX.UTF16(unicodeX.LittleEndian, unicodeX.IgnoreBOM).NewDecoder()
			res, err := io.ReadAll(transform.NewReader(bytes.NewReader(data[2:]), dec))
			if err == nil && utf8.Valid(res) {
				return string(res), nil
			}
		} else if data[0] == 0xFE && data[1] == 0xFF {
			dec := unicodeX.UTF16(unicodeX.BigEndian, unicodeX.IgnoreBOM).NewDecoder()
			res, err := io.ReadAll(transform.NewReader(bytes.NewReader(data[2:]), dec))
			if err == nil && utf8.Valid(res) {
				return string(res), nil
			}
		}
	}
	encs := []encoding.Encoding{
		simplifiedchinese.GBK,
		simplifiedchinese.GB18030,
		traditionalchinese.Big5,
		japanese.ShiftJIS,
		japanese.EUCJP,
		korean.EUCKR,
		charmap.Windows1252,
		charmap.ISO8859_1,
		unicodeX.UTF16(unicodeX.LittleEndian, unicodeX.IgnoreBOM),
		unicodeX.UTF16(unicodeX.BigEndian, unicodeX.IgnoreBOM),
	}
	for _, enc := range encs {
		dec := enc.NewDecoder()
		res, err := io.ReadAll(transform.NewReader(bytes.NewReader(data), dec))
		if err == nil && utf8.Valid(res) && len(strings.TrimSpace(string(res))) > 0 {
			return string(res), nil
		}
	}
	return p.cleanInvalidChars(string(data)), nil
}

func (p *TextFormattingProcessor) cleanInvalidChars(text string) string {
	var out strings.Builder
	for _, r := range text {
		if r == utf8.RuneError {
			continue
		}
		if unicode.IsPrint(r) || r == '\n' || r == '\r' || r == '\t' {
			out.WriteRune(r)
		} else if unicode.IsSpace(r) {
			out.WriteRune(' ')
		}
	}
	return out.String()
}

// 根据中英文比例判断是否主要为中文段落
func isChineseParagraph(s string) bool {
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

func (p *TextFormattingProcessor) FormatText(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	content = strings.ReplaceAll(content, "\t", strings.Repeat(" ", p.TabSize))
	content = p.cleanWhitespace(content)
	return p.formatParagraphs(content)
}

func (p *TextFormattingProcessor) cleanWhitespace(text string) string {
	re := regexp.MustCompile(`[ \t]+$`)
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		lines[i] = re.ReplaceAllString(ln, "")
	}
	text = strings.Join(lines, "\n")
	re2 := regexp.MustCompile(`\n{3,}`)
	text = re2.ReplaceAllString(text, "\n\n")
	return text
}

func (p *TextFormattingProcessor) formatParagraphs(content string) string {
	paras := strings.Split(content, "\n\n")
	var out strings.Builder
	for idx, para := range paras {
		clean := strings.ReplaceAll(para, "\n", " ")
		clean = strings.TrimSpace(clean)
		if clean == "" {
			out.WriteString("\n\n")
			continue
		}
		if isChineseParagraph(clean) {
			// 中文段落按字符数换行
			count := 0
			for _, r := range clean {
				if count >= p.MaxLineLength {
					out.WriteString("\n")
					count = 0
				}
				out.WriteRune(r)
				count++
			}
		} else {
			// 英文或混合段落按词换行
			words := strings.Fields(clean)
			var line strings.Builder
			for _, w := range words {
				if line.Len() > 0 && line.Len()+len(w)+1 > p.MaxLineLength {
					out.WriteString(line.String())
					out.WriteString("\n")
					line.Reset()
				}
				if line.Len() > 0 {
					line.WriteString(" ")
				}
				line.WriteString(w)
			}
			if line.Len() > 0 {
				out.WriteString(line.String())
			}
		}
		if idx < len(paras)-1 {
			out.WriteString("\n\n")
		}
	}
	return out.String()
}
