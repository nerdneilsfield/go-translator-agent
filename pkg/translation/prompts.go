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

	// 添加节点标记保护说明
	prompt += `

🚨 CRITICAL INSTRUCTION - NODE MARKER PRESERVATION 🚨
You MUST follow this instruction EXACTLY or the entire system will FAIL:

1. **PRESERVE ALL NODE MARKERS**: Every @@NODE_START_X@@ and @@NODE_END_X@@ marker MUST appear in your output
2. **EXACT FORMAT**: Copy markers EXACTLY - same spacing, same format, same position
3. **NO MODIFICATION**: Do NOT translate, change, or modify these markers in ANY way
4. **REQUIRED OUTPUT FORMAT**: Your response must maintain this structure:

   @@NODE_START_1@@
   [Your translation of the content here]
   @@NODE_END_1@@
   
   @@NODE_START_2@@
   [Your translation of the content here]
   @@NODE_END_2@@

5. **EXAMPLE - CORRECT**:
   Input:  @@NODE_START_42@@\nHello world\n@@NODE_END_42@@
   Output: @@NODE_START_42@@\n你好世界\n@@NODE_END_42@@

6. **FAILURE CONSEQUENCES**: If you remove or modify ANY marker, the translation will be LOST and the system will FAIL

TRANSLATE ONLY THE CONTENT BETWEEN MARKERS - NEVER THE MARKERS THEMSELVES!`

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

	// 添加节点标记保护说明
	prompt += `

🚨 CRITICAL INSTRUCTION - NODE MARKER PRESERVATION 🚨
You MUST follow this instruction EXACTLY or the entire system will FAIL:

1. **PRESERVE ALL NODE MARKERS**: Every @@NODE_START_X@@ and @@NODE_END_X@@ marker MUST appear in your output
2. **EXACT FORMAT**: Copy markers EXACTLY - same spacing, same format, same position
3. **NO MODIFICATION**: Do NOT translate, change, or modify these markers in ANY way
4. **REQUIRED OUTPUT FORMAT**: Your response must maintain this structure:

   @@NODE_START_1@@
   [Your improved translation here]
   @@NODE_END_1@@
   
   @@NODE_START_2@@
   [Your improved translation here]
   @@NODE_END_2@@

5. **FAILURE CONSEQUENCES**: If you remove or modify ANY marker, the translation will be LOST

TRANSLATE ONLY THE CONTENT BETWEEN MARKERS - NEVER THE MARKERS THEMSELVES!`

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

	// 添加节点标记保护说明
	prompt += `

🚨 CRITICAL INSTRUCTION - NODE MARKER PRESERVATION 🚨
You MUST follow this instruction EXACTLY or the entire system will FAIL:

1. **PRESERVE ALL NODE MARKERS**: Every @@NODE_START_X@@ and @@NODE_END_X@@ marker MUST appear in your output
2. **EXACT FORMAT**: Copy markers EXACTLY - same spacing, same format, same position
3. **NO MODIFICATION**: Do NOT translate, change, or modify these markers in ANY way
4. **REQUIRED OUTPUT FORMAT**: Your response must maintain this structure:

   @@NODE_START_1@@
   [Your translation here]
   @@NODE_END_1@@
   
   @@NODE_START_2@@
   [Your translation here]
   @@NODE_END_2@@

5. **FAILURE CONSEQUENCES**: If you remove or modify ANY marker, the translation will be LOST

TRANSLATE ONLY THE CONTENT BETWEEN MARKERS - NEVER THE MARKERS THEMSELVES!`

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

// RemoveReasoningMarkers 移除推理模型的思考标记和内容（只处理开头的推理标记，避免误删翻译内容）
func RemoveReasoningMarkers(text string) string {
	// 内置所有常见的推理标记对
	commonReasoningTags := []struct{
		start, end string
		preserveContent bool // 是否保留标记内的内容
	}{
		{"<think>", "</think>", false},
		{"<thinking>", "</thinking>", false},
		{"<thought>", "</thought>", false},
		{"<reasoning>", "</reasoning>", false},
		{"<reflection>", "</reflection>", false},
		{"<internal>", "</internal>", false},
		{"<scratch>", "</scratch>", false},
		{"<analysis>", "</analysis>", false},
		{"<brainstorm>", "</brainstorm>", false},
		{"[THINKING]", "[/THINKING]", false},
		{"[REASONING]", "[/REASONING]", false},
		{"[INTERNAL]", "[/INTERNAL]", false},
		{"[SCRATCH]", "[/SCRATCH]", false},
		{"<answer>", "</answer>", true}, // answer标记保留内容
		{"<result>", "</result>", true}, // result标记保留内容
		{"<output>", "</output>", true}, // output标记保留内容
	}

	result := strings.TrimSpace(text)

	// 只处理出现在文本开头的推理标记（只移除一次，避免误删翻译内容中的相似标记）
	for _, tag := range commonReasoningTags {
		// 检查是否以该标记开头
		if strings.HasPrefix(result, tag.start) {
			if tag.preserveContent {
				// 保留内容，只移除标记 - 只处理开头的第一个匹配
				pattern := fmt.Sprintf(`^%s(.*?)%s`, regexp.QuoteMeta(tag.start), regexp.QuoteMeta(tag.end))
				re := regexp.MustCompile(fmt.Sprintf(`(?s)%s`, pattern))
				if matches := re.FindStringSubmatch(result); len(matches) > 1 {
					// 只替换第一个匹配，保留内容，清理开头的空白
					result = strings.TrimSpace(matches[1])
					break // 找到并处理了一个，停止处理其他标记
				}
			} else {
				// 移除整个标记和内容 - 只处理开头的第一个匹配
				pattern := fmt.Sprintf(`^%s.*?%s`, regexp.QuoteMeta(tag.start), regexp.QuoteMeta(tag.end))
				re := regexp.MustCompile(fmt.Sprintf(`(?s)%s`, pattern))
				if re.MatchString(result) {
					// 只替换第一个匹配（开头的）
					result = re.ReplaceAllString(result, "")
					result = strings.TrimSpace(result)
					break // 找到并处理了一个，停止处理其他标记
				}
			}
		}
	}

	// 处理截断的思考标记（只有开始没有结束，且在开头）
	truncatedPatterns := []string{"<think>", "<thinking>", "<thought>", "<reasoning>", "<reflection>"}
	for _, pattern := range truncatedPatterns {
		if strings.HasPrefix(result, pattern) {
			// 检查是否有对应的结束标记
			endPattern := strings.Replace(pattern, "<", "</", 1)
			if !strings.Contains(result, endPattern) {
				// 移除从开始标记到文末的所有内容
				escapedPattern := regexp.QuoteMeta(pattern)
				re := regexp.MustCompile(fmt.Sprintf(`(?s)^%s.*`, escapedPattern))
				result = re.ReplaceAllString(result, "")
				result = strings.TrimSpace(result)
				break
			}
		}
	}

	// 清理多余的空行
	result = regexp.MustCompile(`\n{3,}`).ReplaceAllString(result, "\n\n")

	return strings.TrimSpace(result)
}

// RemoveReasoningProcess 移除推理过程（支持用户自定义标记，可选覆盖内置标记）
func RemoveReasoningProcess(text string, userTags []string) string {
	// 如果用户没有指定标记，使用内置的全套标记
	if len(userTags) == 0 {
		// 直接使用 RemoveReasoningMarkers 中的逻辑
		return RemoveReasoningMarkers(text)
	}

	// 用户指定了标记，使用用户的配置
	result := text

	// 处理用户配置的标记对
	for i := 0; i < len(userTags); i += 2 {
		if i+1 < len(userTags) {
			startTag := userTags[i]
			endTag := userTags[i+1]
			
			// 特殊处理：保留 answer/result/output 标记的内容
			preserveContent := strings.Contains(startTag, "answer") || 
								 strings.Contains(startTag, "result") || 
								 strings.Contains(startTag, "output")
			
			if preserveContent {
				// 保留内容，只移除标记
				pattern := fmt.Sprintf(`(?s)%s(.*?)%s`, regexp.QuoteMeta(startTag), regexp.QuoteMeta(endTag))
				re := regexp.MustCompile(pattern)
				if matches := re.FindStringSubmatch(result); len(matches) > 1 {
					result = re.ReplaceAllString(result, matches[1])
				}
			} else {
				// 移除整个标记和内容
				pattern := fmt.Sprintf(`(?s)%s.*?%s`, regexp.QuoteMeta(startTag), regexp.QuoteMeta(endTag))
				re := regexp.MustCompile(pattern)
				result = re.ReplaceAllString(result, "")
			}
		}
	}

	// 处理截断的标记（只有开始没有结束）
	for i := 0; i < len(userTags); i += 2 {
		startTag := userTags[i]
		if strings.HasPrefix(strings.TrimSpace(result), startTag) {
			// 检查是否有对应的结束标记
			if i+1 < len(userTags) {
				endTag := userTags[i+1]
				if !strings.Contains(result, endTag) {
					// 移除从开始标记到文末的所有内容
					escapedPattern := regexp.QuoteMeta(startTag)
					re := regexp.MustCompile(fmt.Sprintf(`(?s)^%s.*`, escapedPattern))
					result = re.ReplaceAllString(result, "")
					break
				}
			}
		}
	}

	// 清理多余的空行
	result = regexp.MustCompile(`\n{3,}`).ReplaceAllString(result, "\n\n")

	return strings.TrimSpace(result)
}
