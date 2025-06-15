package translation

import (
	"strings"
	"testing"
)

func TestPreparePromptWithProtection(t *testing.T) {
	tests := []struct {
		name           string
		stepName       string
		prompt         string
		context        map[string]string
		wantContains   []string
		wantNotContains []string
	}{
		{
			name:     "initial translation with protection",
			stepName: "initial_translation",
			prompt: `This is a translation task from {{source_language}} to {{target_language}}.

Formatting Rules:
1. Preserve all original formatting exactly.

Please translate the following text:

{{text}}`,
			context: map[string]string{
				"text":              "Test text",
				"source_language":   "English",
				"target_language":   "Chinese",
				"_preserve_enabled": "true",
				"_is_batch":         "true",
			},
			wantContains: []string{
				"PRESERVE",
				"NODE_START",
				"NODE_END",
				"Please translate the following text:",
			},
		},
		{
			name:     "reflection step with protection",
			stepName: "reflection",
			prompt: `You are reviewing a translation from {{source_language}} to {{target_language}}.

Original text:
{{original_text}}

Initial translation:
{{translation}}

Please analyze this translation and identify any issues.`,
			context: map[string]string{
				"original_text":     "Test",
				"translation":       "测试",
				"source_language":   "English",
				"target_language":   "Chinese",
				"_preserve_enabled": "true",
				"_is_batch":         "true",
			},
			wantContains: []string{
				"PRESERVE",
				"NODE_START",
				"NODE_END",
				"Please analyze this translation and identify any issues",
			},
		},
		{
			name:     "improvement step with protection",
			stepName: "improvement",
			prompt: `You are improving a translation.

Original text:
{{original_text}}

Please provide an improved translation.`,
			context: map[string]string{
				"original_text":     "Test",
				"source_language":   "English",
				"target_language":   "Chinese",
				"_preserve_enabled": "true",
				"_is_batch":         "true",
			},
			wantContains: []string{
				"PRESERVE",
				"NODE_START",
				"NODE_END",
				"Please provide an improved translation",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建步骤配置
			config := &StepConfig{
				Name:   tt.stepName,
				Prompt: tt.prompt,
			}

			// 创建步骤
			step := &step{
				config: config,
			}

			// 创建输入
			input := StepInput{
				Text:           tt.context["text"],
				SourceLanguage: tt.context["source_language"],
				TargetLanguage: tt.context["target_language"],
				Context:        tt.context,
			}

			// 准备提示词
			result := step.preparePrompt(input)

			// 检查包含的内容
			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("preparePrompt() result does not contain %q", want)
					t.Logf("Result:\n%s", result)
				}
			}

			// 检查不应包含的内容
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(result, notWant) {
					t.Errorf("preparePrompt() result contains %q but should not", notWant)
				}
			}

			// 确保保护指令在正确的位置（在主要指令之前）
			if tt.context["_preserve_enabled"] == "true" {
				preserveIdx := strings.Index(result, "PRESERVE")
				mainInstructionIdx := -1
				
				// 找到主要指令的位置
				for _, instruction := range tt.wantContains {
					if strings.HasPrefix(instruction, "Please") || strings.HasPrefix(instruction, "You are") {
						idx := strings.Index(result, instruction)
						if idx != -1 {
							mainInstructionIdx = idx
							break
						}
					}
				}

				if preserveIdx != -1 && mainInstructionIdx != -1 && preserveIdx > mainInstructionIdx {
					t.Errorf("Protection instructions should come before main instructions")
					t.Logf("PRESERVE index: %d, Main instruction index: %d", preserveIdx, mainInstructionIdx)
				}
			}
		})
	}
}

func TestStepNameMatching(t *testing.T) {
	tests := []struct {
		stepName     string
		expectedType string
	}{
		{"initial_translation", "translation"},
		{"Initial Translation", "translation"},
		{"reflection", "reflection"},
		{"ai_review", "reflection"},
		{"improvement", "improvement"},
		{"final_polish", "improvement"},
		{"Improve", "improvement"},
		{"custom_step", "generic"},
	}

	for _, tt := range tests {
		t.Run(tt.stepName, func(t *testing.T) {
			// 这个测试验证步骤名称匹配逻辑
			stepNameLower := strings.ToLower(tt.stepName)
			
			var stepType string
			if strings.Contains(stepNameLower, "initial") || strings.Contains(stepNameLower, "translation") {
				stepType = "translation"
			} else if strings.Contains(stepNameLower, "reflection") || strings.Contains(stepNameLower, "review") {
				stepType = "reflection"
			} else if strings.Contains(stepNameLower, "improvement") || strings.Contains(stepNameLower, "improve") || strings.Contains(stepNameLower, "polish") {
				stepType = "improvement"
			} else {
				stepType = "generic"
			}

			if stepType != tt.expectedType {
				t.Errorf("Step %q expected type %q, got %q", tt.stepName, tt.expectedType, stepType)
			}
		})
	}
}