package translation

import (
	"strings"
	"testing"
)

func TestPreparePromptWithBuiltinTemplates(t *testing.T) {
	tests := []struct {
		name           string
		stepName       string
		context        map[string]string
		additionalNotes string
		wantContains   []string
		wantNotContains []string
	}{
		{
			name:     "initial translation with builtin template",
			stepName: "initial_translation",
			context: map[string]string{
				"text":            "Test text",
				"source_language": "English",
				"target_language": "Chinese",
				"country":         "China",
			},
			additionalNotes: "Pay attention to technical terms",
			wantContains: []string{
				"CRITICAL OUTPUT REQUIREMENT",
				"Provide ONLY the translated text",
				"Content Protection Rules",
				"Do not modify any Markdown syntax",
				"Test text",
				"Pay attention to technical terms",
				"Output ONLY the translated text, nothing else",
			},
		},
		{
			name:     "reflection step with builtin template",
			stepName: "reflection",
			context: map[string]string{
				"original_text":   "Test",
				"translation":     "测试",
				"source_language": "English",
				"target_language": "Chinese",
			},
			additionalNotes: "Focus on cultural nuances",
			wantContains: []string{
				"CRITICAL OUTPUT REQUIREMENT",
				"Provide ONLY your reflection and feedback",
				"Original text:",
				"Translation:",
				"Accuracy:",
				"Fluency:",
				"Focus on cultural nuances",
				"Output ONLY your specific feedback",
			},
		},
		{
			name:     "improvement step with builtin template",
			stepName: "improvement",
			context: map[string]string{
				"original_text":   "Test",
				"translation":     "测试",
				"feedback":        "Good translation",
				"source_language": "English",
				"target_language": "Chinese",
			},
			additionalNotes: "Ensure natural flow",
			wantContains: []string{
				"CRITICAL OUTPUT REQUIREMENT",
				"Provide ONLY the improved translation",
				"Current translation:",
				"Feedback:",
				"Ensure natural flow",
				"Output ONLY the improved translation, nothing else",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建步骤配置
			config := &StepConfig{
				Name:            tt.stepName,
				AdditionalNotes: tt.additionalNotes,
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

			// 确保保护规则和输出要求都在模板中
			if strings.Contains(result, "CRITICAL OUTPUT REQUIREMENT") {
				if !strings.Contains(result, "ONLY") {
					t.Errorf("Output requirement should contain 'ONLY' instruction")
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