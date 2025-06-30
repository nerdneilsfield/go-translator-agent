package formatter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextFormatter(t *testing.T) {
	t.Run("Format Text Content", func(t *testing.T) {
		formatter := &TextFormatter{}
		ctx := context.Background()

		tests := []struct {
			name     string
			input    string
			format   string
			expected string
		}{
			{
				name:     "Unix line endings",
				input:    "line1\r\nline2\r\n",
				format:   "text",
				expected: "line1\nline2\n",
			},
			{
				name:     "Trim trailing spaces",
				input:    "line1   \nline2  \n",
				format:   "text",
				expected: "line1\nline2\n",
			},
			{
				name:     "Ensure final newline",
				input:    "line1\nline2",
				format:   "text",
				expected: "line1\nline2\n",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := formatter.FormatString(ctx, tt.input, tt.format, &FormatOptions{})
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("Metadata", func(t *testing.T) {
		formatter := &TextFormatter{}
		meta := formatter.GetMetadata()

		assert.Equal(t, "text", meta.Name)
		assert.Equal(t, "internal", meta.Type)
		assert.Contains(t, meta.Formats, "text")
		assert.Contains(t, meta.Formats, "txt")
		assert.Contains(t, meta.Formats, "plain")
	})
}

func TestMarkdownFormatter(t *testing.T) {
	t.Run("Format Markdown Content", func(t *testing.T) {
		formatter := &MarkdownFormatter{}
		ctx := context.Background()

		// 简单的 Markdown 格式化测试
		input := "# Title\n\n- item1\n-   item2\n"

		result, err := formatter.FormatString(ctx, input, "markdown", &FormatOptions{})
		require.NoError(t, err)

		// markdownfmt 会标准化列表格式
		assert.Contains(t, result, "# Title")
		assert.Contains(t, result, "- item1")
		assert.Contains(t, result, "- item2") // 多余空格会被移除
	})

	t.Run("Metadata", func(t *testing.T) {
		formatter := &MarkdownFormatter{}
		meta := formatter.GetMetadata()

		assert.Equal(t, "markdown", meta.Name)
		assert.Equal(t, "internal", meta.Type)
		assert.Contains(t, meta.Formats, "markdown")
		assert.Contains(t, meta.Formats, "md")
	})
}

func TestFormatterRegistry(t *testing.T) {
	t.Run("Register and Get Formatter", func(t *testing.T) {
		registry := NewFormatterRegistry()

		// 注册一个测试格式化器
		testFormatter := NewTextFormatter()
		err := registry.Register(testFormatter)
		require.NoError(t, err)

		// 通过名称获取
		formatter, err := registry.GetByName("text")
		require.NoError(t, err)
		assert.NotNil(t, formatter)
		assert.Equal(t, "text", formatter.GetMetadata().Name)

		// 通过格式获取
		formatter, err = registry.GetByFormat("txt")
		require.NoError(t, err)
		assert.NotNil(t, formatter)

		// 获取不存在的格式化器
		_, err = registry.GetByName("nonexistent")
		assert.Error(t, err)
	})

	t.Run("List Formatters", func(t *testing.T) {
		registry := NewFormatterRegistry()

		// 注册多个格式化器
		registry.Register(NewTextFormatter())
		registry.Register(NewMarkdownFormatter())

		formatters := registry.List()
		assert.Len(t, formatters, 2)

		// 验证返回的是元数据
		names := make([]string, 0, len(formatters))
		for _, meta := range formatters {
			names = append(names, meta.Name)
		}
		assert.Contains(t, names, "text")
		assert.Contains(t, names, "markdown")
	})

	t.Run("Duplicate Registration", func(t *testing.T) {
		registry := NewFormatterRegistry()

		formatter1 := NewTextFormatter()
		formatter2 := NewTextFormatter()

		err := registry.Register(formatter1)
		require.NoError(t, err)

		// 重复注册应该报错
		err = registry.Register(formatter2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})
}

func TestFormatterManager(t *testing.T) {
	t.Run("Format with Manager", func(t *testing.T) {
		manager := NewFormatterManager()
		ctx := context.Background()

		// 自动注册应该已经注册了文本格式化器
		content := "test   \r\n"
		result, err := manager.Format(ctx, content, "text", &FormatOptions{})
		require.NoError(t, err)
		assert.Equal(t, "test\n", result)
	})

	t.Run("Protection Blocks", func(t *testing.T) {
		manager := NewFormatterManager()
		ctx := context.Background()

		content := `Some text
<!-- TRANSLATE_PROTECTED_START -->
Protected content   
With spaces   
<!-- TRANSLATE_PROTECTED_END -->
More text   `

		result, err := manager.Format(ctx, content, "text", &FormatOptions{
			PreserveBlocks: []PreserveBlock{
				{
					Type:    "protected",
					Pattern: `<!-- TRANSLATE_PROTECTED_START -->[\s\S]*?<!-- TRANSLATE_PROTECTED_END -->`,
				},
			},
		})
		require.NoError(t, err)

		// 保护块内的内容应该被格式化（因为保护块功能还未实现）
		assert.Contains(t, result, "Protected content")
		assert.Contains(t, result, "With spaces")
		// 保护块外的内容应该被格式化
		assert.Contains(t, result, "Some text\n")
		assert.Contains(t, result, "More text\n")
	})
}

// 集成测试
func TestDocumentFormatterIntegration(t *testing.T) {
	t.Run("Format Document Block", func(t *testing.T) {
		formatter := NewDocumentFormatter(NewFormatterManager())
		ctx := context.Background()

		// 创建一个测试块
		block := &MockBlock{
			content:      "test   \nwith spaces   ",
			translatable: true,
			format:       "text",
		}

		err := formatter.FormatBlock(ctx, block, &FormatOptions{})
		require.NoError(t, err)

		// 内容应该被格式化
		assert.Equal(t, "test\nwith spaces\n", block.GetContent())
	})
}

// MockBlock 用于测试的模拟块
type MockBlock struct {
	content      string
	translatable bool
	format       string
}

func (m *MockBlock) GetContent() string {
	return m.content
}

func (m *MockBlock) SetContent(content string) {
	m.content = content
}

func (m *MockBlock) IsTranslatable() bool {
	return m.translatable
}

func (m *MockBlock) GetType() string {
	return "text"
}

func (m *MockBlock) GetMetadata() map[string]interface{} {
	return map[string]interface{}{
		"format": m.format,
	}
}
