package translator_tests

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
)

// 测试HTML节点收集逻辑
func TestHTMLNodeCollection(t *testing.T) {
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
	var nodeParents []string

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
			if c.Get(0).Type == 3 { // 文本节点
				text := strings.TrimSpace(c.Text())
				if text != "" {
					textNodes = append(textNodes, text)
					nodeParents = append(nodeParents, nodeName)
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

	// 打印收集到的所有文本节点，用于调试
	t.Logf("收集到的文本节点: %v", textNodes)

	// 不验证文本节点的总数量，因为可能会收集到额外的空白文本节点
	// 只验证是否包含所有预期的文本节点

	// 验证每个文本节点的内容
	for _, expectedText := range expectedTexts {
		found := false
		for _, text := range textNodes {
			if text == expectedText {
				found = true
				break
			}
		}
		assert.True(t, found, "未找到预期的文本节点: %s", expectedText)
	}
}

// 测试HTML节点替换逻辑
func TestHTMLNodeReplacement(t *testing.T) {
	// 测试HTML内容
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

	// 使用goquery解析HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	assert.NoError(t, err)

	// 翻译映射
	translations := map[string]string{
		"Test Page":              "测试页面",
		"Hello World":            "你好世界",
		"This is a test paragraph.": "这是一个测试段落。",
	}

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
			if c.Get(0).Type == 3 { // 文本节点
				text := strings.TrimSpace(c.Text())
				if text != "" && translations[text] != "" {
					// 替换文本节点
					// 注意：在实际代码中，这里需要保留原始的空白字符
					c.ReplaceWithHtml(translations[text])
				}
			}
		})
	})

	// 获取修改后的HTML
	html, err := doc.Html()
	assert.NoError(t, err)

	// 验证翻译结果
	assert.Contains(t, html, "测试页面")
	assert.Contains(t, html, "你好世界")
	assert.Contains(t, html, "这是一个测试段落。")
	assert.NotContains(t, html, "Test Page")
	assert.NotContains(t, html, "Hello World")
	assert.NotContains(t, html, "This is a test paragraph.")
}

// 测试XML标签处理问题
func TestXMLTagHandling(t *testing.T) {
	// 测试XML内容
	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<root>
    <element id="1">
        <name>Test Name</name>
        <description>This is a test description.</description>
    </element>
    <element id="2">
        <name>Another Test</name>
        <description>This is another test description.</description>
    </element>
</root>`

	// 使用goquery解析XML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(xmlContent))
	assert.NoError(t, err)

	// 收集需要翻译的文本节点
	var textNodes []string

	// 遍历所有节点
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		// 处理当前节点的直接文本内容（不包括子节点）
		s.Contents().Each(func(j int, c *goquery.Selection) {
			if c.Get(0).Type == 3 { // 文本节点
				text := strings.TrimSpace(c.Text())
				if text != "" {
					textNodes = append(textNodes, text)
				}
			}
		})
	})

	// 验证收集到的文本节点包含以下内容
	expectedTexts := []string{
		"Test Name",
		"This is a test description.",
		"Another Test",
		"This is another test description.",
	}

	// 打印收集到的所有文本节点，用于调试
	t.Logf("收集到的文本节点: %v", textNodes)

	// 不验证文本节点的总数量，因为可能会收集到额外的空白文本节点
	// 只验证是否包含所有预期的文本节点

	// 验证每个文本节点的内容
	for _, expectedText := range expectedTexts {
		found := false
		for _, text := range textNodes {
			if text == expectedText {
				found = true
				break
			}
		}
		assert.True(t, found, "未找到预期的文本节点: %s", expectedText)
	}
}
