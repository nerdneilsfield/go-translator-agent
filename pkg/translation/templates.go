package translation

// BuiltinTemplates 内置模板定义
const (
	// StandardTranslationTemplate 标准翻译模板
	// 用户只需要提供 additional_notes，其他格式化规则都是内置的
	StandardTranslationTemplate = `This is a translation task from {{.source_language}} to {{.target_language}}.

Formatting Rules:
1. Preserve all original formatting exactly:
   - Do not modify any Markdown syntax (**, *, #, etc.).
   - Do not translate any content within LaTeX formulas ($...$, $$...$$, \( ... \), \[ ... \]) or any LaTeX commands.
   - For LaTeX files, preserve all commands, environments (such as \begin{...} and \end{...}), and macros exactly as they are.
   - Keep all HTML tags intact.
2. Do not alter abbreviations, technical terms, or code identifiers.
3. Preserve document structure, including line breaks, paragraph spacing, lists, and tables.
4. Use terminology and expressions appropriate for {{.country}}.
{{if .additional_notes}}
Additional Notes:
{{.additional_notes}}
{{end}}

Please translate the following text:

{{.text}}`

	// SimpleTranslationTemplate 简单翻译模板（无格式化规则）
	SimpleTranslationTemplate = `Translate the following {{.source_language}} text to {{.target_language}}.
{{if .additional_notes}}
Additional Notes: {{.additional_notes}}
{{end}}

{{.text}}`

	// StandardReflectionTemplate 标准反思模板
	StandardReflectionTemplate = `Review the following translation from {{.source_language}} to {{.target_language}}.
Identify any issues with accuracy, fluency, cultural appropriateness, or style.

Original text:
{{.original_text}}

Translation:
{{.translation}}
{{if .additional_notes}}
Additional Notes: {{.additional_notes}}
{{end}}

Provide specific feedback on any issues found.`

	// StandardImprovementTemplate 标准改进模板
	StandardImprovementTemplate = `Based on the feedback provided, improve the following translation from {{.source_language}} to {{.target_language}}.

Original text:
{{.original_text}}

Current translation:
{{.translation}}

Feedback:
{{.feedback}}
{{if .additional_notes}}
Additional Notes: {{.additional_notes}}
{{end}}

Provide an improved translation.`
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