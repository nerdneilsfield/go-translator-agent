package translator

import (
	"fmt"
	"strings"
	"testing"
	
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

func TestDebugProtection(t *testing.T) {
	// 测试文本，包含列表
	testText := `Here is a list:

1) First item with some text
2) Second item with more text  
3) Third item

And some regular text after the list.`

	// 创建保护管理器
	pm := translation.NewPreserveManager(translation.DefaultPreserveConfig)
	
	// 使用 BatchTranslator 的保护方法
	bt := &BatchTranslator{}
	protectedText := bt.protectContent(testText, pm)
	
	// 打印结果
	fmt.Printf("Original:\n%s\n\n", testText)
	fmt.Printf("Protected:\n%s\n\n", protectedText)
	
	// 模拟翻译后还原
	restoredText := pm.Restore(protectedText)
	fmt.Printf("Restored:\n%s\n\n", restoredText)
	
	// 检查列表标记是否被保护
	markers := []string{"1)", "2)", "3)"}
	for _, marker := range markers {
		if protectedText == testText {
			t.Logf("Text was not protected at all")
		}
		// 列表标记不应该被保护
		if !strings.Contains(protectedText, marker) {
			t.Errorf("List marker '%s' was incorrectly removed from protected text", marker)
		}
	}
	
	// 测试中文翻译文本
	chineseText := `我们提出了一种新的结构：

1) 第一点：新的非递归结构
2) 第二点：SEO-NDT算法
3) 第三点：FPGA加速器架构

本文的其余部分组织如下。`
	
	protectedChinese := bt.protectContent(chineseText, pm)
	fmt.Printf("\nChinese Original:\n%s\n\n", chineseText)
	fmt.Printf("Chinese Protected:\n%s\n\n", protectedChinese)
}