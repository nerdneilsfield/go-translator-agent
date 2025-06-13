package translation

import (
	"fmt"
	"regexp"
	"strings"
)

// PromptBuilder 提示词构建器
type PromptBuilder struct {
	// 源语言
	SourceLang string
	// 目标语言
	TargetLang string
	// 国家/地区（可选）
	Country string
	// 保护块配置
	PreserveConfig PreserveConfig
	// 额外的指令
	ExtraInstructions []string
}

// NewPromptBuilder 创建提示词构建器
func NewPromptBuilder(sourceLang, targetLang, country string) *PromptBuilder {
	return &PromptBuilder{
		SourceLang:        sourceLang,
		TargetLang:        targetLang,
		Country:           country,
		PreserveConfig:    DefaultPreserveConfig,
		ExtraInstructions: make([]string, 0),
	}
}

// WithPreserveConfig 设置保护块配置
func (pb *PromptBuilder) WithPreserveConfig(config PreserveConfig) *PromptBuilder {
	pb.PreserveConfig = config
	return pb
}

// AddInstruction 添加额外指令
func (pb *PromptBuilder) AddInstruction(instruction string) *PromptBuilder {
	pb.ExtraInstructions = append(pb.ExtraInstructions, instruction)
	return pb
}

// BuildInitialTranslationPrompt 构建初始翻译提示词
func (pb *PromptBuilder) BuildInitialTranslationPrompt(text string) string {
	prompt := fmt.Sprintf(`This is a translation task from %s to %s.

Formatting Rules:
1. Preserve all original formatting exactly:
   - Do not modify any Markdown syntax (**, *, #, etc.).
   - Do not translate any content within LaTeX formulas ($...$, $$...$$, \( ... \), \[ ... \]) or any LaTeX commands.
   - For LaTeX files, preserve all commands, environments (such as \begin{...} and \end{...}), and macros exactly as they are.
   - Keep all HTML tags intact.
2. Do not alter abbreviations, technical terms, or code identifiers.
3. Preserve document structure, including line breaks, paragraph spacing, lists, and tables.`, 
		pb.SourceLang, pb.TargetLang)

	// 添加国家/地区特定说明
	if pb.Country != "" {
		prompt += fmt.Sprintf("\n4. Use terminology and expressions appropriate for %s.", pb.Country)
	}

	// 添加额外指令
	if len(pb.ExtraInstructions) > 0 {
		prompt += "\n\nAdditional Instructions:"
		for i, instruction := range pb.ExtraInstructions {
			prompt += fmt.Sprintf("\n%d. %s", i+1, instruction)
		}
	}

	// 添加保护块说明
	prompt = AppendPreservePrompt(prompt, pb.PreserveConfig)

	// 添加要翻译的文本
	prompt += fmt.Sprintf("\n\nPlease translate the following text:\n\n%s", text)

	return prompt
}

// BuildReflectionPrompt 构建反思提示词
func (pb *PromptBuilder) BuildReflectionPrompt(sourceText, initialTranslation string) string {
	prompt := fmt.Sprintf(`You are reviewing a translation from %s to %s.

Original text:
%s

Initial translation:
%s

Please analyze this translation and identify any issues. Consider:
1. Accuracy: Does the translation convey the exact meaning of the original?
2. Fluency: Does the translation read naturally in %s?
3. Terminology: Are technical terms, proper nouns, and specialized vocabulary translated appropriately?
4. Formatting: Is all original formatting preserved (Markdown, LaTeX, HTML tags, etc.)?
5. Consistency: Is the translation consistent throughout?`,
		pb.SourceLang, pb.TargetLang,
		sourceText,
		initialTranslation,
		pb.TargetLang)

	// 添加国家/地区特定说明
	if pb.Country != "" {
		prompt += fmt.Sprintf("\n6. Regional appropriateness: Is the language appropriate for %s?", pb.Country)
	}

	// 添加保护块说明
	preservePrompt := GetPreservePrompt(pb.PreserveConfig)
	if preservePrompt != "" {
		prompt += "\n\n" + preservePrompt
		prompt += "\nIMPORTANT: Check that all preserve markers are intact in the translation."
	}

	prompt += "\n\nProvide a detailed analysis of any issues found. If the translation is perfect, simply state that no issues were found."

	return prompt
}

// BuildImprovementPrompt 构建改进提示词
func (pb *PromptBuilder) BuildImprovementPrompt(sourceText, initialTranslation, reflection string) string {
	prompt := fmt.Sprintf(`You are improving a translation from %s to %s based on the following reflection.

Original text:
%s

Initial translation:
%s

Reflection/Issues identified:
%s

Please provide an improved translation that addresses all the issues mentioned in the reflection while:
1. Maintaining the exact meaning of the original text
2. Ensuring natural fluency in %s
3. Preserving all original formatting (Markdown, LaTeX, HTML tags, etc.)
4. Using appropriate terminology and expressions`,
		pb.SourceLang, pb.TargetLang,
		sourceText,
		initialTranslation,
		reflection,
		pb.TargetLang)

	// 添加国家/地区特定说明
	if pb.Country != "" {
		prompt += fmt.Sprintf("\n5. Using language appropriate for %s", pb.Country)
	}

	// 添加保护块说明
	prompt = AppendPreservePrompt(prompt, pb.PreserveConfig)

	prompt += "\n\nProvide only the improved translation without any explanation or commentary."

	return prompt
}

// BuildDirectTranslationPrompt 构建直接翻译提示词（用于快速模式）
func (pb *PromptBuilder) BuildDirectTranslationPrompt(text string) string {
	prompt := fmt.Sprintf(`Translate the following text from %s to %s.

Rules:
1. Preserve all formatting (Markdown, LaTeX, HTML, etc.)
2. Do not translate code, formulas, or technical identifiers
3. Maintain the original document structure`,
		pb.SourceLang, pb.TargetLang)

	// 添加国家/地区特定说明
	if pb.Country != "" {
		prompt += fmt.Sprintf("\n4. Use language appropriate for %s", pb.Country)
	}

	// 添加保护块说明
	prompt = AppendPreservePrompt(prompt, pb.PreserveConfig)

	prompt += fmt.Sprintf("\n\nText to translate:\n\n%s", text)

	return prompt
}

// ExtractTranslationFromResponse 从 LLM 响应中提取翻译结果
// 有些模型可能会在翻译前后添加额外的说明，这个函数负责提取纯翻译内容
func ExtractTranslationFromResponse(response string) string {
	// 移除常见的前缀
	prefixes := []string{
		"Here is the translation:",
		"Here's the translation:",
		"Translation:",
		"Translated text:",
		"The translation is:",
	}
	
	result := response
	for _, prefix := range prefixes {
		if strings.HasPrefix(strings.TrimSpace(result), prefix) {
			result = strings.TrimPrefix(strings.TrimSpace(result), prefix)
			result = strings.TrimSpace(result)
		}
	}
	
	// 移除可能的代码块标记
	if strings.HasPrefix(result, "```") && strings.HasSuffix(result, "```") {
		lines := strings.Split(result, "\n")
		if len(lines) >= 3 {
			// 移除第一行和最后一行的 ```
			result = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	
	return strings.TrimSpace(result)
}

// RemoveReasoningMarkers 移除推理模型的思考标记
func RemoveReasoningMarkers(text string) string {
	// 移除 <thinking>...</thinking> 标记
	thinkingRe := regexp.MustCompile(`(?s)<thinking>.*?</thinking>`)
	text = thinkingRe.ReplaceAllString(text, "")
	
	// 移除 <reflection>...</reflection> 标记
	reflectionRe := regexp.MustCompile(`(?s)<reflection>.*?</reflection>`)
	text = reflectionRe.ReplaceAllString(text, "")
	
	// 移除 <answer>...</answer> 标记，但保留内容
	answerRe := regexp.MustCompile(`(?s)<answer>(.*?)</answer>`)
	if matches := answerRe.FindStringSubmatch(text); len(matches) > 1 {
		text = matches[1]
	}
	
	// 移除多余的空行
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")
	
	return strings.TrimSpace(text)
}

// RemoveReasoningProcess 移除推理过程（支持自定义标记）
func RemoveReasoningProcess(text string, tags []string) string {
	// 如果没有指定标记，使用默认的移除方法
	if len(tags) == 0 {
		return RemoveReasoningMarkers(text)
	}
	
	// 为每个标记创建正则表达式并移除
	for _, tag := range tags {
		// 处理成对标记，如 <thinking>...</thinking>
		pattern := fmt.Sprintf(`(?s)<%s>.*?</%s>`, regexp.QuoteMeta(tag), regexp.QuoteMeta(tag))
		re := regexp.MustCompile(pattern)
		text = re.ReplaceAllString(text, "")
		
		// 处理单独标记，如 <answer>...保留内容...</answer>
		if tag == "answer" {
			pattern = fmt.Sprintf(`(?s)<%s>(.*?)</%s>`, regexp.QuoteMeta(tag), regexp.QuoteMeta(tag))
			re = regexp.MustCompile(pattern)
			if matches := re.FindStringSubmatch(text); len(matches) > 1 {
				text = matches[1]
			}
		}
	}
	
	// 移除多余的空行
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")
	
	return strings.TrimSpace(text)
}