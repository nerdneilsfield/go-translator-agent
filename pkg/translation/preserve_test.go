package translation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPreserveManager(t *testing.T) {
	t.Run("Basic Protection and Restoration", func(t *testing.T) {
		pm := NewPreserveManager(DefaultPreserveConfig)

		// 保护一些内容
		code := "func main() { fmt.Println(\"Hello\") }"
		formula := "$E = mc^2$"

		placeholder1 := pm.Protect(code)
		placeholder2 := pm.Protect(formula)

		// 验证占位符格式
		assert.Equal(t, "@@PRESERVE_0@@", placeholder1)
		assert.Equal(t, "@@PRESERVE_1@@", placeholder2)

		// 创建包含占位符的文本
		text := "This is some text with " + placeholder1 + " and a formula " + placeholder2

		// 还原
		restored := pm.Restore(text)

		// 验证还原结果
		assert.Contains(t, restored, code)
		assert.Contains(t, restored, formula)
		assert.NotContains(t, restored, "@@PRESERVE_")
	})

	t.Run("Multiple Occurrences", func(t *testing.T) {
		pm := NewPreserveManager(DefaultPreserveConfig)

		content := "```go\ncode block\n```"
		placeholder := pm.Protect(content)

		// 文本中有多个相同的占位符
		text := placeholder + " some text " + placeholder + " more text " + placeholder

		// 还原
		restored := pm.Restore(text)

		// 验证所有占位符都被还原
		assert.Equal(t, 3, strings.Count(restored, content))
		assert.NotContains(t, restored, "@@PRESERVE_")
	})

	t.Run("Nested Protection", func(t *testing.T) {
		pm := NewPreserveManager(DefaultPreserveConfig)

		// 保护内容中包含类似占位符的文本
		content1 := "This contains @@PRESERVE_999@@ as text"
		content2 := "Normal content"

		placeholder1 := pm.Protect(content1)
		placeholder2 := pm.Protect(content2)

		text := "Text with " + placeholder1 + " and " + placeholder2

		// 还原
		restored := pm.Restore(text)

		// 验证正确还原，包括内容中的假占位符
		assert.Contains(t, restored, "@@PRESERVE_999@@")
		assert.Contains(t, restored, content1)
		assert.Contains(t, restored, content2)
	})

	t.Run("Custom Config", func(t *testing.T) {
		config := PreserveConfig{
			Enabled: true,
			Prefix:  "<<KEEP_",
			Suffix:  ">>",
		}
		pm := NewPreserveManager(config)

		content := "protected content"
		placeholder := pm.Protect(content)

		assert.Equal(t, "<<KEEP_0>>", placeholder)

		text := "Text with " + placeholder
		restored := pm.Restore(text)

		assert.Equal(t, "Text with protected content", restored)
	})
}

func TestGetPreservePrompt(t *testing.T) {
	t.Run("Default Config", func(t *testing.T) {
		prompt := GetPreservePrompt(DefaultPreserveConfig)

		assert.Contains(t, prompt, "@@PRESERVE_")
		assert.Contains(t, prompt, "Do not translate or modify")
		assert.Contains(t, prompt, "Example: @@PRESERVE_0@@")
	})

	t.Run("Custom Config", func(t *testing.T) {
		config := PreserveConfig{
			Enabled: true,
			Prefix:  "[[NO_TRANS_",
			Suffix:  "]]",
		}
		prompt := GetPreservePrompt(config)

		assert.Contains(t, prompt, "[[NO_TRANS_")
		assert.Contains(t, prompt, "]]")
		assert.Contains(t, prompt, "Example: [[NO_TRANS_0]]")
	})

	t.Run("Disabled", func(t *testing.T) {
		config := PreserveConfig{
			Enabled: false,
		}
		prompt := GetPreservePrompt(config)

		assert.Empty(t, prompt)
	})
}

func TestAppendPreservePrompt(t *testing.T) {
	basePrompt := "Translate this text."

	t.Run("With Preserve", func(t *testing.T) {
		result := AppendPreservePrompt(basePrompt, DefaultPreserveConfig)

		assert.Contains(t, result, basePrompt)
		assert.Contains(t, result, "IMPORTANT: Preserve Markers")
		assert.Contains(t, result, "@@PRESERVE_")
	})

	t.Run("Without Preserve", func(t *testing.T) {
		config := PreserveConfig{Enabled: false}
		result := AppendPreservePrompt(basePrompt, config)

		assert.Equal(t, basePrompt, result)
	})
}
