package translation

// BuiltinTemplates 内置模板定义
const (
	// StandardTranslationTemplate 标准翻译模板
	// 使用明确边界分隔指令和待翻译内容
	StandardTranslationTemplate = `This is a translation task from {{.source_language}} to {{.target_language}}.

CRITICAL OUTPUT REQUIREMENT:
- Provide ONLY the translated text
- Do NOT include any explanations, comments, or additional text
- Do NOT add phrases like "Here is the translation:" or "The translation is:"

Content Protection Rules:
1. Preserve all original formatting exactly:
   - Do not modify any Markdown syntax (**, *, #, etc.)
   - Do not translate any content within LaTeX formulas ($...$, $$...$$, \( ... \), \[ ... \]) or any LaTeX commands
   - For LaTeX files, preserve all commands, environments (such as \begin{...} and \end{...}), and macros exactly as they are
   - Keep all HTML tags intact
   - Preserve any XML/HTML-like tags or special markers
2. Do not alter abbreviations, technical terms, or code identifiers
3. Preserve document structure, including line breaks, paragraph spacing, lists, and tables
4. Do not translate URLs, file paths, or code blocks
5. Preserve any special node markers or processing instructions
6. Use terminology and expressions appropriate for {{.country}}
{{if .additional_notes}}

Additional Notes:
{{.additional_notes}}
{{end}}

The text to translate is enclosed between the markers below. Translate ONLY the content between these markers:

===== CONTENT TO TRANSLATE BEGIN =====
{{.text}}
===== CONTENT TO TRANSLATE END =====

Output ONLY the translated text, nothing else.`

	// SimpleTranslationTemplate 简单翻译模板（无格式化规则）
	SimpleTranslationTemplate = `Translate the following {{.source_language}} text to {{.target_language}}.

CRITICAL OUTPUT REQUIREMENT:
- Provide ONLY the translated text
- Do NOT include any explanations or additional text

{{if .additional_notes}}
Additional Notes: {{.additional_notes}}
{{end}}

The text to translate is enclosed between the markers below. Translate ONLY the content between these markers:

===== CONTENT TO TRANSLATE BEGIN =====
{{.text}}
===== CONTENT TO TRANSLATE END =====

Output ONLY the translated text, nothing else.`

	// StandardReflectionTemplate 标准反思模板
	StandardReflectionTemplate = `Review the following translation from {{.source_language}} to {{.target_language}}.

CRITICAL OUTPUT REQUIREMENT:
- Provide ONLY your reflection and feedback
- Do NOT include any explanations or introductory text
- Do NOT add phrases like "Here is my analysis:" or "My review is:"

Please analyze this translation considering:
1. Accuracy: Does it convey the exact meaning of the original?
2. Fluency: Does it read naturally in {{.target_language}}?
3. Terminology: Are technical terms and specialized vocabulary appropriate?
4. Formatting: Is all original formatting preserved?
5. Consistency: Is the translation consistent throughout?
6. Cultural appropriateness: Is the language suitable for {{.country}}?
{{if .additional_notes}}

Additional Notes: {{.additional_notes}}
{{end}}

The content to review is enclosed between the markers below:

===== ORIGINAL TEXT BEGIN =====
{{.original_text}}
===== ORIGINAL TEXT END =====

===== TRANSLATION TO REVIEW BEGIN =====
{{.translation}}
===== TRANSLATION TO REVIEW END =====

Output ONLY your specific feedback on any issues found, or state "No issues found" if perfect.`

	// StandardImprovementTemplate 标准改进模板
	StandardImprovementTemplate = `Improve the following translation from {{.source_language}} to {{.target_language}} based on the feedback provided.

CRITICAL OUTPUT REQUIREMENT:
- Provide ONLY the improved translation
- Do NOT include any explanations, comments, or additional text
- Do NOT add phrases like "Here is the improved translation:" or "The corrected version is:"

Requirements for the improved translation:
1. Address all issues mentioned in the feedback
2. Maintain the exact meaning of the original text
3. Ensure natural fluency in {{.target_language}}
4. Preserve all original formatting
5. Use appropriate terminology and expressions for {{.country}}
{{if .additional_notes}}

Additional Notes: {{.additional_notes}}
{{end}}

The content to improve is enclosed between the markers below:

===== ORIGINAL TEXT BEGIN =====
{{.original_text}}
===== ORIGINAL TEXT END =====

===== CURRENT TRANSLATION BEGIN =====
{{.translation}}
===== CURRENT TRANSLATION END =====

===== FEEDBACK BEGIN =====
{{.feedback}}
===== FEEDBACK END =====

Output ONLY the improved translation, nothing else.`
)

// TemplateType 模板类型
type TemplateType string

const (
	TemplateTypeStandard    TemplateType = "standard"
	TemplateTypeSimple      TemplateType = "simple"
	TemplateTypeCustom      TemplateType = "custom"
	TemplateTypeReflection  TemplateType = "reflection"
	TemplateTypeImprovement TemplateType = "improvement"
)

// GetBuiltinTemplate 获取内置模板
func GetBuiltinTemplate(templateType TemplateType) string {
	switch templateType {
	case TemplateTypeStandard:
		return StandardTranslationTemplate
	case TemplateTypeSimple:
		return SimpleTranslationTemplate
	case TemplateTypeReflection:
		return StandardReflectionTemplate
	case TemplateTypeImprovement:
		return StandardImprovementTemplate
	default:
		return StandardTranslationTemplate
	}
}

// IsBuiltinTemplate 检查是否是内置模板
func IsBuiltinTemplate(prompt string) (TemplateType, bool) {
	switch prompt {
	case "standard", "":
		return TemplateTypeStandard, true
	case "simple":
		return TemplateTypeSimple, true
	case "reflection":
		return TemplateTypeReflection, true
	case "improvement":
		return TemplateTypeImprovement, true
	default:
		return TemplateTypeCustom, false
	}
}