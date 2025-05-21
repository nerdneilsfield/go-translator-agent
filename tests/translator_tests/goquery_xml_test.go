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

// 测试使用goquery库解析和翻译XML
func TestGoQueryXMLParsing(t *testing.T) {
	// 测试XML内容
	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<root>
    <item id="1">
        <name>Test Name</name>
        <description>This is a test description.</description>
    </item>
    <item id="2">
        <name>Another Test</name>
        <description>This is another test description.</description>
    </item>
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
		"Test Name",
		"This is a test description.",
		"Another Test",
		"This is another test description.",
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

// 测试使用goquery库翻译XML
func TestGoQueryXMLTranslation(t *testing.T) {
	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<root>
    <item id="1">
        <name>Test Name</name>
        <description>This is a test description.</description>
    </item>
</root>`

	cfg := test.CreateTestConfig()
	newLogger := logger.NewZapLogger(true)
	mockTrans := test.NewMockTranslator(cfg, newLogger)
	mockTrans.On("Translate", mock.Anything, mock.Anything).Return("这是翻译后的文本", nil)

	translated, err := formats.TranslateHTMLWithGoQuery(xmlContent, mockTrans, newLogger.GetZapLogger())
	assert.NoError(t, err)
	assert.Contains(t, translated, "这是翻译后的文本")
	assert.NotEmpty(t, translated)
}

// 测试复杂XML结构的翻译
func TestComplexXMLTranslation(t *testing.T) {
	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<root>
    <group>
        <item>Item 1</item>
        <item>Item 2</item>
    </group>
</root>`

	cfg := test.CreateTestConfig()
	newLogger := logger.NewZapLogger(true)
	mockTrans := test.NewMockTranslator(cfg, newLogger)
	mockTrans.On("Translate", mock.Anything, mock.Anything).Return("这是翻译后的文本", nil)

	translated, err := formats.TranslateHTMLWithGoQuery(xmlContent, mockTrans, newLogger.GetZapLogger())
	assert.NoError(t, err)
	assert.Contains(t, translated, "这是翻译后的文本")
	assert.NotEmpty(t, translated)
}
