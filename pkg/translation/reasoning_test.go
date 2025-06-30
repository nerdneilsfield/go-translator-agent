package translation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveReasoningProcess(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		tags     []string
		expected string
	}{
		{
			name: "DeepSeek格式的思考过程",
			input: `<think>
Let me think about this translation carefully.
The text talks about artificial intelligence...
I should consider the context...
</think>

人工智能正在改变我们的世界。`,
			tags:     []string{"<think>", "</think>"},
			expected: `人工智能正在改变我们的世界。`,
		},
		{
			name: "多个思考标签",
			input: `这是第一部分翻译。

<think>
Now I need to think about the second part...
</think>

这是第二部分翻译。

<think>
And finally the third part...
</think>

这是第三部分翻译。`,
			tags: []string{"<think>", "</think>"},
			expected: `这是第一部分翻译。

这是第二部分翻译。

这是第三部分翻译。`,
		},
		{
			name: "不同的标签格式",
			input: `<thinking>
This is my reasoning process...
</thinking>

The translation is: 翻译结果`,
			tags:     []string{"<thinking>", "</thinking>"},
			expected: `The translation is: 翻译结果`,
		},
		{
			name: "自动检测常见标签",
			input: `<think>
Reasoning 1
</think>

Part 1

<thinking>
Reasoning 2
</thinking>

Part 2

[REASONING]
Reasoning 3
[/REASONING]

Part 3`,
			tags: nil, // 使用自动检测
			expected: `Part 1

Part 2

Part 3`,
		},
		{
			name:     "Markdown代码块格式",
			input:    "```thinking\nThis is my thought process\n```\n\n翻译结果在这里。",
			tags:     nil,
			expected: "翻译结果在这里。",
		},
		{
			name: "嵌套和复杂格式",
			input: `开始部分

<think>
这是一个很长的思考过程...
包含多行...
甚至有特殊字符：<>[]{}
和数学公式：$x^2 + y^2 = z^2$
</think>

中间部分

<reflection>
这是反思部分
</reflection>

结束部分`,
			tags: nil,
			expected: `开始部分

中间部分

结束部分`,
		},
		{
			name:     "保留正常的代码块",
			input:    "这是说明文字\n\n```python\ndef hello():\n    print('Hello')\n```\n\n这是翻译结果。",
			tags:     nil,
			expected: "这是说明文字\n\n```python\ndef hello():\n    print('Hello')\n```\n\n这是翻译结果。",
		},
		{
			name:     "没有推理标签的正常文本",
			input:    "这是一段正常的翻译文本，没有任何推理过程。",
			tags:     []string{"<think>", "</think>"},
			expected: "这是一段正常的翻译文本，没有任何推理过程。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveReasoningProcess(tt.input, tt.tags)
			// 标准化空白字符进行比较
			expectedNorm := strings.TrimSpace(tt.expected)
			resultNorm := strings.TrimSpace(result)
			assert.Equal(t, expectedNorm, resultNorm)
		})
	}
}

func TestHasReasoningTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "包含think标签",
			input:    "Some text <think>reasoning</think> more text",
			expected: true,
		},
		{
			name:     "包含thinking标签",
			input:    "Text <thinking>process</thinking> text",
			expected: true,
		},
		{
			name:     "包含大写标签",
			input:    "Text [REASONING]process[/REASONING] text",
			expected: true,
		},
		{
			name:     "包含Markdown推理块",
			input:    "Text\n```thinking\nprocess\n```\ntext",
			expected: true,
		},
		{
			name:     "不包含推理标签",
			input:    "This is just normal text without any reasoning tags",
			expected: false,
		},
		{
			name:     "包含类似但不完全匹配的标签",
			input:    "Text <thinker>not a tag</thinker> text",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasReasoningTags(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractReasoningProcess(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		tags     []string
		expected []string
	}{
		{
			name: "提取单个推理过程",
			input: `Translation: <think>
This is my reasoning
</think>
Result here`,
			tags:     []string{"<think>", "</think>"},
			expected: []string{"This is my reasoning"},
		},
		{
			name: "提取多个推理过程",
			input: `Part 1
<think>First thought</think>
Part 2
<think>Second thought</think>
Part 3`,
			tags:     []string{"<think>", "</think>"},
			expected: []string{"First thought", "Second thought"},
		},
		{
			name: "自动提取不同格式",
			input: `<think>Thought 1</think>
<thinking>Thought 2</thinking>
<reasoning>Thought 3</reasoning>`,
			tags:     nil,
			expected: []string{"Thought 1", "Thought 2", "Thought 3"},
		},
		{
			name:     "没有推理过程",
			input:    "Just normal text without any reasoning",
			tags:     []string{"<think>", "</think>"},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractReasoningProcess(tt.input, tt.tags)
			assert.Equal(t, len(tt.expected), len(result))
			for i, expected := range tt.expected {
				if i < len(result) {
					assert.Equal(t, expected, result[i])
				}
			}
		})
	}
}

func TestCleanupEmptyLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "移除多余空行",
			input: `Line 1



Line 2


Line 3`,
			expected: `Line 1

Line 2

Line 3`,
		},
		{
			name: "保留段落间的单个空行",
			input: `Paragraph 1

Paragraph 2

Paragraph 3`,
			expected: `Paragraph 1

Paragraph 2

Paragraph 3`,
		},
		{
			name:     "清理首尾空白",
			input:    "\n\n\nText\n\n\n",
			expected: "Text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanupEmptyLines(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
