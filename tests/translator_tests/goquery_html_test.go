package translator_tests

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/internal/test"
	"github.com/nerdneilsfield/go-translator-agent/pkg/formats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// 测试使用goquery库解析和翻译HTML
func TestGoQueryHTMLParsing(t *testing.T) {
	// 测试HTML内容
	htmlContent := `<!DOCTYPE html>
<html>
<head>
    <title>Test Page</title>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial; }
    </style>
    <script>
        function test() {
            console.log("Test");
        }
    </script>
</head>
<body>
    <h1>Hello World</h1>
    <p>This is a test paragraph.</p>
    <div>
        <p>This is a nested paragraph.</p>
        <ul>
            <li>Item 1</li>
            <li>Item 2</li>
        </ul>
    </div>
    <script>
        // This is a JavaScript comment
        console.log("Another test");
    </script>
</body>
</html>`

	// 使用goquery解析HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	assert.NoError(t, err)

	// 收集需要翻译的文本节点
	var textNodes []string

	// 遍历所有节点
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		// 获取节点名称
		nodeName := goquery.NodeName(s)

		// 跳过script和style标签
		if nodeName == "script" || nodeName == "style" {
			return
		}

		// 处理当前节点的直接文本内容（不包括子节点）
		s.Contents().Each(func(j int, c *goquery.Selection) {
			if goquery.NodeName(c) == "#text" { // 文本节点
				text := strings.TrimSpace(c.Text())
				if text != "" {
					textNodes = append(textNodes, text)
				}
			}
		})
	})

	// 验证收集到的文本节点包含以下内容
	expectedTexts := []string{
		"Test Page",
		"Hello World",
		"This is a test paragraph.",
		"This is a nested paragraph.",
		"Item 1",
		"Item 2",
	}

	for _, expected := range expectedTexts {
		found := false
		for _, actual := range textNodes {
			if strings.Contains(actual, expected) {
				found = true
				break
			}
		}
		assert.True(t, found, "未找到预期的文本节点: %s", expected)
	}

	// 输出收集到的文本节点，用于调试
	t.Logf("收集到的文本节点: %v", textNodes)
}

// 测试使用goquery库翻译HTML
func TestGoQueryHTMLTranslation(t *testing.T) {
	htmlContent := `<!DOCTYPE html>
<html>
<head>
    <title>Test Page</title>
</head>
<body>
    <h1>Hello World</h1>
    <p>This is a test paragraph.</p>
</body>
</html>`

	cfg := test.CreateTestConfig()
	newLogger := logger.NewZapLogger(true)
	mockTrans := test.NewMockTranslator(cfg, newLogger)
	mockTrans.On("Translate", mock.Anything, mock.Anything).Return("这是翻译后的文本", nil)

	translated, err := formats.TranslateHTMLWithGoQuery(htmlContent, mockTrans, newLogger.GetZapLogger())
	assert.NoError(t, err)
	assert.Contains(t, translated, "这是翻译后的文本")
	assert.NotEmpty(t, translated)
}

// 测试复杂HTML结构的翻译
func TestComplexHTMLTranslation(t *testing.T) {
	htmlContent := `<!DOCTYPE html>
<html>
<head>
    <title>Complex</title>
    <style>body { font-family: Arial; }</style>
    <script>console.log("Test")</script>
</head>
<body>
    <div>
        <p>This is a nested paragraph.</p>
        <ul>
            <li>Item 1</li>
            <li>Item 2</li>
        </ul>
    </div>
</body>
</html>`

	cfg := test.CreateTestConfig()
	newLogger := logger.NewZapLogger(true)
	mockTrans := test.NewMockTranslator(cfg, newLogger)
	mockTrans.On("Translate", mock.Anything, mock.Anything).Return("这是翻译后的文本", nil)

	translated, err := formats.TranslateHTMLWithGoQuery(htmlContent, mockTrans, newLogger.GetZapLogger())
	assert.NoError(t, err)
	assert.Contains(t, translated, "这是翻译后的文本")
	assert.NotEmpty(t, translated)
}
