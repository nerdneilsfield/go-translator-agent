package translation

import (
	"fmt"
	"testing"
)

func TestDebugPromptGeneration(t *testing.T) {
	// 三个阶段的配置
	configs := []struct {
		name   string
		prompt string
	}{
		{
			name: "initial_translation",
			prompt: `This is a translation task from {{source_language}} to {{target_language}}.

Formatting Rules:
1. Preserve all original formatting exactly.

Please translate the following text:

{{text}}`,
		},
		{
			name: "reflection",
			prompt: `You are reviewing a translation from {{source_language}} to {{target_language}}.

Original text:
{{original_text}}

Initial translation:
{{translation}}

Please analyze this translation and identify any issues.`,
		},
		{
			name: "improvement",
			prompt: `You are improving a translation from {{source_language}} to {{target_language}}.

Original text:
{{original_text}}

Initial translation:
{{translation}}

Reflection:
{{reflection}}

Please provide an improved translation.`,
		},
	}

	// 测试上下文
	contexts := []map[string]string{
		// 初始翻译
		{
			"text":              "Hello world",
			"source_language":   "English",
			"target_language":   "Chinese",
			"_preserve_enabled": "true",
			"_is_batch":         "true",
		},
		// 反思
		{
			"original_text":     "Hello world",
			"translation":       "你好世界",
			"source_language":   "English",
			"target_language":   "Chinese",
			"_preserve_enabled": "true",
			"_is_batch":         "true",
		},
		// 改进
		{
			"original_text":     "Hello world",
			"translation":       "你好世界",
			"reflection":        "The translation is too literal",
			"source_language":   "English",
			"target_language":   "Chinese",
			"_preserve_enabled": "true",
			"_is_batch":         "true",
		},
	}

	for i, cfg := range configs {
		t.Run(cfg.name, func(t *testing.T) {
			// 创建步骤
			step := &step{
				config: &StepConfig{
					Name:            cfg.name,
					AdditionalNotes: "Debug test",
				},
			}

			// 创建输入
			input := StepInput{
				Text:           contexts[i]["text"],
				SourceLanguage: contexts[i]["source_language"],
				TargetLanguage: contexts[i]["target_language"],
				Context:        contexts[i],
			}

			// 准备提示词
			result := step.preparePrompt(input)

			// 打印结果
			fmt.Printf("\n=== %s ===\n", cfg.name)
			fmt.Printf("%s\n", result)
			fmt.Println("=== End ===")
		})
	}
}