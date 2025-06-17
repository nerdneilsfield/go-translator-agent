package translator

import (
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
	"github.com/stretchr/testify/assert"
)

func TestMathFormulaProtection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string // 期望被保护的公式
	}{
		{
			name:  "用户报告的问题",
			input: "假设$\\mathbf{F}$和$\\mathbf{M}$是两个部分重叠的点云，取自相邻姿态。此外，我们定义$\\mathbf{F}$为一个固定（或参考）点云，而$\\mathbf{M}$为一个移动（或目标）点云。",
			expected: []string{
				"$\\mathbf{F}$",
				"$\\mathbf{M}$",
				"$\\mathbf{F}$",
				"$\\mathbf{M}$",
			},
		},
		{
			name:  "复杂公式",
			input: "方程$E = mc^2$和$F = ma$以及$\\alpha = \\frac{\\beta}{\\gamma}$都应该被保护。",
			expected: []string{
				"$E = mc^2$",
				"$F = ma$",
				"$\\alpha = \\frac{\\beta}{\\gamma}$",
			},
		},
		{
			name:  "行间公式",
			input: "$$\\int_{-\\infty}^{\\infty} e^{-x^2} dx = \\sqrt{\\pi}$$",
			expected: []string{
				"$$\\int_{-\\infty}^{\\infty} e^{-x^2} dx = \\sqrt{\\pi}$$",
			},
		},
		{
			name:  "LaTeX括号",
			input: "公式\\(x^2 + y^2 = z^2\\)和\\[\\sum_{i=1}^n x_i = 0\\]应该被保护。",
			expected: []string{
				"\\(x^2 + y^2 = z^2\\)",
				"\\[\\sum_{i=1}^n x_i = 0\\]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建BatchTranslator和PreserveManager
			bt := &BatchTranslator{}
			pm := translation.NewPreserveManager(translation.DefaultPreserveConfig)

			// 保护内容
			protected := bt.protectContent(tt.input, pm)

			// 验证所有期望的公式都被保护了
			for _, expectedFormula := range tt.expected {
				assert.NotContains(t, protected, expectedFormula,
					"公式 %s 应该被保护，但在保护后的文本中仍然存在", expectedFormula)
			}

			// 还原内容
			restored := pm.Restore(protected)

			// 验证还原后的内容与原始内容相同
			assert.Equal(t, tt.input, restored,
				"还原后的内容应该与原始内容完全相同")

			t.Logf("原始: %s", tt.input)
			t.Logf("保护: %s", protected)
			t.Logf("还原: %s", restored)
		})
	}
}

func TestImprovedRegexPatterns(t *testing.T) {
	// 测试改进后的正则表达式
	bt := &BatchTranslator{}
	pm := translation.NewPreserveManager(translation.DefaultPreserveConfig)

	// 测试边界情况
	tests := []struct {
		name  string
		input string
		desc  string
	}{
		{
			name:  "连续公式",
			input: "$a$$b$连续的公式",
			desc:  "连续的单字符公式应该被正确保护",
		},
		{
			name:  "嵌套美元符号",
			input: "文本$内容\\$符号$更多文本",
			desc:  "包含转义美元符号的公式应该被正确保护",
		},
		{
			name:  "多行行间公式",
			input: "$$\n\\begin{align}\nx &= y + z \\\\\na &= b + c\n\\end{align}\n$$",
			desc:  "多行行间公式应该被正确保护",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protected := bt.protectContent(tt.input, pm)
			restored := pm.Restore(protected)

			assert.Equal(t, tt.input, restored, tt.desc)
			assert.NotEqual(t, tt.input, protected, "内容应该被保护(即有占位符)")

			t.Logf("%s:", tt.desc)
			t.Logf("原始: %q", tt.input)
			t.Logf("保护: %q", protected)
			t.Logf("还原: %q", restored)
		})
	}
}
