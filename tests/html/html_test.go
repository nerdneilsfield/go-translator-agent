package html_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/dlclark/regexp2"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/internal/test"
	"github.com/nerdneilsfield/go-translator-agent/pkg/formats"
)

// 创建一个模拟翻译器，用于测试

// 辅助函数
func containsPattern(text, pattern string) bool {
	re := regexp.MustCompile(pattern)
	return re.MatchString(text)
}

func containsXMLDeclaration(text string) bool {
	// add more test detail
	if !containsPattern(text, `<\?xml`) {
		fmt.Println("text:\n", text)
		fmt.Println("text contains <?xml: ", containsPattern(text, `<\?xml`))
		return false
	}
	return true
}

func containsDOCTYPE(text string) bool {
	// 不区分大小写地检查DOCTYPE声明
	return containsPattern(text, `(?i)<!DOCTYPE`) || containsPattern(text, `(?i)<!doctype`)
}

func containsScriptContent(text string) bool {
	// 检查是否包含JavaScript代码的特征
	return containsPattern(text, `function`) ||
		containsPattern(text, `console\.log`) ||
		containsPattern(text, `alert\(`) ||
		containsPattern(text, `// This is a JavaScript`)
}

func containsStyleContent(text string) bool {
	// 检查是否包含CSS样式的特征
	return containsPattern(text, `body \{`) ||
		containsPattern(text, `font-family:`) ||
		containsPattern(text, `margin:`) ||
		containsPattern(text, `padding:`)
}

func TestHTMLTranslation(t *testing.T) {
	// 创建logger
	logger := logger.NewZapLogger(true)

	cfg := test.CreateTestConfig()

	// 创建模拟翻译器
	mockTranslator := test.NewMockTranslator(cfg, logger)

	// 测试用例
	testCases := []struct {
		name     string
		filename string
	}{
		{
			name:     "Advanced HTML Test",
			filename: "test4.html",
		},
		{
			name:     "Complex Nested Structure Test",
			filename: "test5.html",
		},
		{
			name:     "XML Test",
			filename: "test_xml.xml",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 读取测试文件
			inputPath := filepath.Join(".", tc.filename)
			content, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatalf("读取文件失败 %s: %v", inputPath, err)
			}

			// 使用改进的HTML处理器翻译
			htmlTranslator := formats.NewGoQueryHTMLTranslator(mockTranslator, logger.GetZapLogger())
			translated, err := htmlTranslator.Translate(string(content), tc.filename)
			if err != nil {
				t.Fatalf("翻译失败: %v", err)
			}

			// 保存翻译结果
			outputPath := filepath.Join(".", tc.filename+"_translated")
			if err := os.WriteFile(outputPath, []byte(translated), 0644); err != nil {
				t.Fatalf("写入文件失败 %s: %v", outputPath, err)
			}

			// 验证翻译结果
			// 1. 检查文件是否存在
			if _, err := os.Stat(outputPath); os.IsNotExist(err) {
				t.Fatalf("翻译后的文件不存在: %s", outputPath)
			}

			// 2. 检查翻译后的文件是否包含"[翻译]"标记
			translatedContent, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("读取翻译后的文件失败 %s: %v", outputPath, err)
			}

			if len(translatedContent) == 0 {
				t.Fatalf("翻译后的文件为空: %s", outputPath)
			}

			translatedStr := string(translatedContent)
			if !strings.Contains(translatedStr, "[翻译]") {
				t.Errorf("翻译后的文件不包含翻译标记: %s", outputPath)
			}

			// 3. 检查是否保留了DOCTYPE和XML声明
			if tc.filename == "test_xml.xml" {
				if !containsXMLDeclaration(translatedStr) {
					t.Errorf("翻译后的XML文件丢失了XML声明")
				}
			} else {
				if !containsDOCTYPE(translatedStr) {
					t.Errorf("翻译后的HTML文件丢失了DOCTYPE声明")
				}
			}

			// 4. 检查是否保留了脚本和样式
			contentStr := string(content)
			if containsScriptContent(contentStr) && !containsScriptContent(translatedStr) {
				t.Errorf("翻译后的文件丢失了脚本内容")
			}

			if containsStyleContent(contentStr) && !containsStyleContent(translatedStr) {
				t.Errorf("翻译后的文件丢失了样式内容")
			}

			t.Logf("成功翻译文件: %s -> %s", inputPath, outputPath)
		})
	}
}

// TestParseNodeWrappedText 使用我们讨论的正则表达式来测试文本解析
func TestParseNodeWrappedText(t *testing.T) {
	// 正确的正则表达式 (使用 $1 或 ${1} 进行反向引用，这里为了清晰用 $1)
	// (?s) 允许 . 匹配换行符
	// @@NODE_START_(\d+)@@ 匹配开始标记并捕获数字索引 (group 1)
	// \n 匹配开始标记后的换行符
	// (.*?) 懒惰匹配翻译内容，直到遇到下一个模式 (group 2)
	// \n 匹配结束标记前的换行符
	// @@NODE_END_$1@@ 使用反向引用 $1确保结束标记的数字与开始标记的数字一致
	pattern := `(?s)@@NODE_START_(\d+)@@\r?\n(.*?)\r?\n@@NODE_END_\1@@`
	re := regexp2.MustCompile(pattern, 0)

	testCases := []struct {
		name           string
		inputText      string
		expectedOutput map[int]string // map[nodeIndex]expectedText
		expectedError  bool           // 是否期望在解析过程中出现错误（例如Atoi转换失败）
	}{
		{
			name:      "Single valid node",
			inputText: "@@NODE_START_0@@\nTranslated Text 0\n@@NODE_END_0@@",
			expectedOutput: map[int]string{
				0: "Translated Text 0",
			},
			expectedError: false,
		},
		{
			name: "Multiple valid nodes separated by double newlines",
			inputText: "@@NODE_START_0@@\nTranslated Text 0\n@@NODE_END_0@@\n\n" +
				"@@NODE_START_1@@\nTranslated Text 1 with\nmultiple lines\n@@NODE_END_1@@\n\n" +
				"@@NODE_START_42@@\nAnother translation for 42\n@@NODE_END_42@@",
			expectedOutput: map[int]string{
				0:  "Translated Text 0",
				1:  "Translated Text 1 with\nmultiple lines",
				42: "Another translation for 42",
			},
			expectedError: false,
		},
		{
			name: "Nodes without double newline separators (regex should still find them individually)",
			inputText: "@@NODE_START_0@@\nText 0\n@@NODE_END_0@@" + // 注意这里没有 \n\n
				"@@NODE_START_1@@\nText 1\n@@NODE_END_1@@",
			expectedOutput: map[int]string{
				0: "Text 0",
				1: "Text 1",
			},
			expectedError: false,
		},
		{
			name:      "Node with empty content",
			inputText: "@@NODE_START_5@@\n\n@@NODE_END_5@@", // 内容是空的，但有一个换行符
			expectedOutput: map[int]string{
				5: "", // TrimSpace 后为空
			},
			expectedError: false,
		},
		{
			name:      "Node with only whitespace content",
			inputText: "@@NODE_START_6@@\n   \t   \n@@NODE_END_6@@",
			expectedOutput: map[int]string{
				6: "", // TrimSpace 后为空
			},
			expectedError: false,
		},
		{
			name:           "Mismatched end marker (should not match)",
			inputText:      "@@NODE_START_7@@\nText 7\n@@NODE_END_8@@", // 结束标记的数字不匹配
			expectedOutput: map[int]string{},                           // 期望不匹配任何内容
			expectedError:  false,
		},
		{
			name:           "No newline before end marker (should not match based on current regex)",
			inputText:      "@@NODE_START_9@@\nText 9@@NODE_END_9@@", // @@NODE_END_9@@ 前缺少 \n
			expectedOutput: map[int]string{},
			expectedError:  false,
		},
		{
			name:           "No newline after start marker (should not match based on current regex)",
			inputText:      "@@NODE_START_10@@Text 10\n@@NODE_END_10@@", // @@NODE_START_10@@ 后缺少 \n
			expectedOutput: map[int]string{},
			expectedError:  false,
		},
		{
			name:      "Text containing PRESERVE markers",
			inputText: "@@NODE_START_11@@\nTranslated text with @@PRESERVE_0@@don't translate@@/PRESERVE_0@@ and more @@PRESERVE_1@@also this@@/PRESERVE_1@@.\n@@NODE_END_11@@",
			expectedOutput: map[int]string{
				11: "Translated text with @@PRESERVE_0@@don't translate@@/PRESERVE_0@@ and more @@PRESERVE_1@@also this@@/PRESERVE_1@@.",
			},
			expectedError: false,
		},
		{
			name: "Mixed valid and invalid (mismatched end marker)",
			inputText: "@@NODE_START_0@@\nValid 0\n@@NODE_END_0@@\n\n" +
				"@@NODE_START_1@@\nInvalid due to end marker\n@@NODE_END_2@@\n\n" + // 这里的2使其无效
				"@@NODE_START_3@@\nValid 3\n@@NODE_END_3@@",
			expectedOutput: map[int]string{
				0: "Valid 0",
				3: "Valid 3",
			},
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parsedOutput := make(map[int]string)

			// 首先找到第一个匹配
			match, err := re.FindStringMatch(tc.inputText)
			if err != nil {
				t.Fatalf("regexp2 查找失败: %v", err)
			}

			// 迭代所有匹配
			for match != nil {
				// 组1：数字索引；组2：原始内容（包含换行）
				idxStr := match.GroupByNumber(1).String()
				raw := match.GroupByNumber(2).String()

				idx, err := strconv.Atoi(idxStr)
				if err != nil {
					t.Errorf("无法将索引 '%s' 转为整数: %v", idxStr, err)
				}
				// 去掉首尾换行并 TrimSpace
				text := strings.Trim(raw, "\r\n")
				text = strings.TrimSpace(text)

				parsedOutput[idx] = text

				// 继续下一个
				match, _ = re.FindNextMatch(match)
			}

			// 验证结果数量
			if len(parsedOutput) != len(tc.expectedOutput) {
				t.Errorf("解析数量不一致，期望 %d 个，实际 %d 个: %v", len(tc.expectedOutput), len(parsedOutput), parsedOutput)
			}
			// 逐项比对内容
			for wantIdx, wantText := range tc.expectedOutput {
				if gotText, ok := parsedOutput[wantIdx]; !ok {
					t.Errorf("未找到索引 %d 对应的文本，期望: %q", wantIdx, wantText)
				} else if gotText != wantText {
					t.Errorf("索引 %d 文本不匹配，期望: %q，实际: %q", wantIdx, wantText, gotText)
				}
			}
		})
	}
}
