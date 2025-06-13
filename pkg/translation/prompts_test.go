package translation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPromptBuilder(t *testing.T) {
	pb := NewPromptBuilder("English", "Chinese", "China")
	
	t.Run("Initial Translation Prompt", func(t *testing.T) {
		text := "Hello world"
		prompt := pb.BuildInitialTranslationPrompt(text)
		
		// 验证基本内容
		assert.Contains(t, prompt, "translation task from English to Chinese")
		assert.Contains(t, prompt, "Hello world")
		assert.Contains(t, prompt, "China")
		
		// 验证格式规则
		assert.Contains(t, prompt, "Preserve all original formatting")
		assert.Contains(t, prompt, "Markdown syntax")
		assert.Contains(t, prompt, "LaTeX formulas")
		
		// 验证保护块说明
		assert.Contains(t, prompt, "IMPORTANT: Preserve Markers")
		assert.Contains(t, prompt, "@@PRESERVE_")
	})
	
	t.Run("Reflection Prompt", func(t *testing.T) {
		sourceText := "Hello world"
		translation := "你好世界"
		prompt := pb.BuildReflectionPrompt(sourceText, translation)
		
		// 验证内容
		assert.Contains(t, prompt, "reviewing a translation from English to Chinese")
		assert.Contains(t, prompt, sourceText)
		assert.Contains(t, prompt, translation)
		
		// 验证分析要点
		assert.Contains(t, prompt, "Accuracy")
		assert.Contains(t, prompt, "Fluency")
		assert.Contains(t, prompt, "Terminology")
		assert.Contains(t, prompt, "Formatting")
		assert.Contains(t, prompt, "Consistency")
		assert.Contains(t, prompt, "China")
		
		// 验证保护块检查
		assert.Contains(t, prompt, "Check that all preserve markers are intact")
	})
	
	t.Run("Improvement Prompt", func(t *testing.T) {
		sourceText := "Hello world"
		translation := "你好世界"
		reflection := "The translation is too literal."
		prompt := pb.BuildImprovementPrompt(sourceText, translation, reflection)
		
		// 验证内容
		assert.Contains(t, prompt, "improving a translation from English to Chinese")
		assert.Contains(t, prompt, sourceText)
		assert.Contains(t, prompt, translation)
		assert.Contains(t, prompt, reflection)
		
		// 验证改进要求
		assert.Contains(t, prompt, "addresses all the issues")
		assert.Contains(t, prompt, "natural fluency in Chinese")
		assert.Contains(t, prompt, "China")
		
		// 验证保护块说明
		assert.Contains(t, prompt, "@@PRESERVE_")
	})
	
	t.Run("Direct Translation Prompt", func(t *testing.T) {
		text := "Quick translation"
		prompt := pb.BuildDirectTranslationPrompt(text)
		
		// 验证简化的提示词
		assert.Contains(t, prompt, "Translate the following text from English to Chinese")
		assert.Contains(t, prompt, text)
		assert.Contains(t, prompt, "China")
		
		// 验证基本规则
		assert.Contains(t, prompt, "Preserve all formatting")
		assert.Contains(t, prompt, "@@PRESERVE_")
	})
	
	t.Run("With Extra Instructions", func(t *testing.T) {
		pb := NewPromptBuilder("English", "Chinese", "")
		pb.AddInstruction("Use formal language")
		pb.AddInstruction("Avoid slang")
		
		prompt := pb.BuildInitialTranslationPrompt("test")
		
		assert.Contains(t, prompt, "Additional Instructions:")
		assert.Contains(t, prompt, "1. Use formal language")
		assert.Contains(t, prompt, "2. Avoid slang")
	})
	
	t.Run("Without Country", func(t *testing.T) {
		pb := NewPromptBuilder("English", "French", "")
		prompt := pb.BuildInitialTranslationPrompt("test")
		
		// 不应包含国家/地区相关内容
		assert.NotContains(t, prompt, "appropriate for")
	})
}

func TestExtractTranslationFromResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Plain translation",
			input:    "你好世界",
			expected: "你好世界",
		},
		{
			name:     "With prefix",
			input:    "Here is the translation: 你好世界",
			expected: "你好世界",
		},
		{
			name:     "With code block",
			input:    "```\n你好世界\n```",
			expected: "你好世界",
		},
		{
			name:     "Multiple prefixes",
			input:    "Translation: Here is the translation: 你好世界",
			expected: "你好世界",
		},
		{
			name:     "With whitespace",
			input:    "  \n  Translated text: 你好世界  \n  ",
			expected: "你好世界",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractTranslationFromResponse(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPromptBuilderWithCustomPreserveConfig(t *testing.T) {
	customConfig := PreserveConfig{
		Enabled: true,
		Prefix:  "<<PROTECT_",
		Suffix:  ">>",
	}
	
	pb := NewPromptBuilder("English", "Spanish", "").
		WithPreserveConfig(customConfig)
	
	prompt := pb.BuildInitialTranslationPrompt("test")
	
	// 应该包含自定义的保护块标记
	assert.Contains(t, prompt, "<<PROTECT_")
	assert.Contains(t, prompt, ">>")
	assert.NotContains(t, prompt, "@@PRESERVE_")
}