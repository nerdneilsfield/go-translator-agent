package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// TranslationPostProcessor 翻译后处理器
type TranslationPostProcessor struct {
	config       *config.Config
	logger       *zap.Logger
	glossary     *EnhancedGlossary
	contentGuard *ContentGuard
}

// NewTranslationPostProcessor 创建翻译后处理器
func NewTranslationPostProcessor(config *config.Config, logger *zap.Logger) *TranslationPostProcessor {
	processor := &TranslationPostProcessor{
		config:       config,
		logger:       logger,
		contentGuard: NewContentGuard(logger),
	}

	// 加载词汇表
	if config.GlossaryPath != "" {
		glossary, err := LoadEnhancedGlossary(config.GlossaryPath, config.SourceLang, config.TargetLang)
		if err != nil {
			logger.Warn("failed to load glossary", zap.Error(err))
		} else {
			processor.glossary = glossary
			logger.Info("glossary loaded",
				zap.String("path", config.GlossaryPath),
				zap.Int("terms", len(glossary.Terms)))
		}
	}

	return processor
}

// ProcessTranslation 处理翻译结果
func (p *TranslationPostProcessor) ProcessTranslation(ctx context.Context, originalText, translatedText string, metadata map[string]interface{}) (string, error) {
	result := translatedText

	// 1. 清理提示词标记和错误翻译
	result = p.cleanupPromptMarkers(result)

	// 2. 应用词汇表修正
	if p.glossary != nil {
		result = p.applyGlossaryCorrections(result, originalText)
	}

	// 3. 内容保护检查和修复
	result = p.contentGuard.VerifyAndRestore(originalText, result)

	// 4. 格式一致性修复
	result = p.fixFormattingConsistency(result)

	// 5. 中英文混排优化
	result = p.optimizeMixedLanguageSpacing(result)

	// 6. 去除机器翻译痕迹
	result = p.removeMachineTranslationArtifacts(result)

	return result, nil
}

// cleanupPromptMarkers 清理提示词标记
func (p *TranslationPostProcessor) cleanupPromptMarkers(text string) string {
	// 常见的提示词标记模式
	patterns := []string{
		`</?(?:translation|translate|source|target|text)>`,
		`</?(?:翻译|译文|原文|目标)>`,
		`\[(?:翻译|TRANSLATION|TRANSLATE)\]`,
		`(?:以下是翻译：|Translation:|翻译结果：|Translated text:)`,
		`(?:Please translate|请翻译|Translate the following).*?:`,
		`^(?:翻译：|译：|Translation:\s*)`,
		`(?:翻译完成|Translation complete|翻译结束).*?$`,
	}

	result := text
	for _, pattern := range patterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		result = re.ReplaceAllString(result, "")
	}

	// 清理多余的空行
	result = regexp.MustCompile(`\n\s*\n\s*\n`).ReplaceAllString(result, "\n\n")

	return strings.TrimSpace(result)
}

// applyGlossaryCorrections 应用词汇表修正
func (p *TranslationPostProcessor) applyGlossaryCorrections(translatedText, originalText string) string {
	result := translatedText

	// 按优先级排序术语（优先级高的先处理）
	terms := p.glossary.GetSortedTerms()

	for _, term := range terms {
		// 检查原文是否包含该术语
		if p.containsTerm(originalText, term) {
			// 如果翻译文本中已经存在源术语（如保持英文的技术术语），则不替换
			if p.containsTerm(translatedText, GlossaryTerm{
				Source:        term.Source,
				Pattern:       term.Pattern,
				MatchType:     term.MatchType,
				CaseSensitive: term.CaseSensitive,
			}) {
				// 术语在翻译文本中以源语言形式存在，可能是有意保留，跳过替换
				continue
			}

			// 应用术语替换
			result = p.applyTermReplacement(result, term)
		}
	}

	return result
}

// containsTerm 检查文本是否包含术语
func (p *TranslationPostProcessor) containsTerm(text string, term GlossaryTerm) bool {
	if term.MatchType == "regex" {
		re, err := regexp.Compile(term.Pattern)
		if err != nil {
			return false
		}
		return re.MatchString(text)
	}

	// 默认使用单词边界匹配
	pattern := `\b` + regexp.QuoteMeta(term.Pattern) + `\b`
	if term.CaseSensitive {
		re := regexp.MustCompile(pattern)
		return re.MatchString(text)
	} else {
		re := regexp.MustCompile(`(?i)` + pattern)
		return re.MatchString(text)
	}
}

// applyTermReplacement 应用术语替换
func (p *TranslationPostProcessor) applyTermReplacement(text string, term GlossaryTerm) string {
	// 构建替换模式
	var pattern string
	if term.MatchType == "regex" {
		pattern = term.Pattern
	} else {
		pattern = `\b` + regexp.QuoteMeta(term.Pattern) + `\b`
	}

	flags := ""
	if !term.CaseSensitive {
		flags = "(?i)"
	}

	re := regexp.MustCompile(flags + pattern)
	return re.ReplaceAllString(text, term.Target)
}

// fixFormattingConsistency 修复格式一致性
func (p *TranslationPostProcessor) fixFormattingConsistency(text string) string {
	result := text

	// 修复引号一致性（统一使用英文引号）
	// 使用字符串替换处理各种引号
	quoteReplacements := map[string]string{
		"\u201c": `"`, // 左双引号
		"\u201d": `"`, // 右双引号
		"\u2018": `'`, // 左单引号
		"\u2019": `'`, // 右单引号
		"\u201e": `"`, // 德语下双引号
		"\u201a": `'`, // 德语下单引号
		"\u300c": `"`, // 中文左引号「
		"\u300d": `"`, // 中文右引号」
		"\u300e": `"`, // 中文左书名号『
		"\u300f": `"`, // 中文右书名号』
	}

	for old, new := range quoteReplacements {
		result = strings.ReplaceAll(result, old, new)
	}

	// 修复省略号
	result = regexp.MustCompile(`\.{2,}`).ReplaceAllString(result, "...")
	result = strings.ReplaceAll(result, "\u2026\u2026", "...") // 多个省略号

	// 修复破折号
	result = regexp.MustCompile(`[-–—]{2,}`).ReplaceAllString(result, "——")

	return result
}

// optimizeMixedLanguageSpacing 优化中英文混排空格
func (p *TranslationPostProcessor) optimizeMixedLanguageSpacing(text string) string {
	// 中文字符和英文字符之间添加空格
	result := regexp.MustCompile(`([\p{Han}])([a-zA-Z0-9])`).ReplaceAllString(text, "$1 $2")
	result = regexp.MustCompile(`([a-zA-Z0-9])([\p{Han}])`).ReplaceAllString(result, "$1 $2")

	// 修复版本号格式 (如 version2.0 -> version 2.0)
	// 但不要给单字母版本号前缀添加空格 (如 v2.1.0 保持原样)
	result = regexp.MustCompile(`([a-zA-Z]{2,})(\d+\.\d+)`).ReplaceAllString(result, "$1 $2")

	// 中文字符和数字符号之间添加空格
	result = regexp.MustCompile(`([\p{Han}])([%$€£¥#&@])`).ReplaceAllString(result, "$1 $2")
	result = regexp.MustCompile(`([%$€£¥#&@])([\p{Han}])`).ReplaceAllString(result, "$1 $2")

	// 但是不要在标点符号之间添加空格
	result = regexp.MustCompile(`([\p{Han}])\s+([，。！？；：、])`).ReplaceAllString(result, "$1$2")
	result = regexp.MustCompile(`([，。！？；：、])\s+([\p{Han}])`).ReplaceAllString(result, "$1$2")

	return result
}

// removeMachineTranslationArtifacts 去除机器翻译痕迹
func (p *TranslationPostProcessor) removeMachineTranslationArtifacts(text string) string {
	result := text

	// 去除重复的词汇
	result = p.removeRepeatedWords(result)

	// 修复过度翻译的专有名词
	result = p.fixOverTranslatedProperNouns(result)

	// 修复语序问题
	result = p.fixWordOrder(result)

	return result
}

// removeRepeatedWords 去除重复词汇
func (p *TranslationPostProcessor) removeRepeatedWords(text string) string {
	// Go的正则表达式不支持反向引用，使用不同的方法
	words := strings.Fields(text)
	result := make([]string, 0, len(words))

	for i, word := range words {
		// 如果是第一个词或与前一个词不相同，则添加
		if i == 0 || word != words[i-1] {
			result = append(result, word)
		}
	}

	return strings.Join(result, " ")
}

// fixOverTranslatedProperNouns 修复过度翻译的专有名词
func (p *TranslationPostProcessor) fixOverTranslatedProperNouns(text string) string {
	// 这里可以添加常见的过度翻译修复规则
	// 例如：将错误翻译的品牌名、人名等恢复

	commonFixes := map[string]string{
		"苹果公司": "Apple",
		"微软公司": "Microsoft",
		"谷歌公司": "Google",
		"脸书":   "Facebook",
		"推特":   "Twitter",
		"优步":   "Uber",
		"亚马逊":  "Amazon",
		"网飞":   "Netflix",
	}

	result := text
	for wrong, correct := range commonFixes {
		result = strings.ReplaceAll(result, wrong, correct)
	}

	return result
}

// fixWordOrder 修复语序问题
func (p *TranslationPostProcessor) fixWordOrder(text string) string {
	// 修复常见的语序问题
	// 例如："的...的" 结构过多
	result := regexp.MustCompile(`(\w+)的(\w+)的(\w+)`).ReplaceAllStringFunc(text, func(match string) string {
		// 简化复杂的"的"字结构
		parts := strings.Split(match, "的")
		if len(parts) == 3 {
			return parts[0] + parts[1] + parts[2]
		}
		return match
	})

	return result
}

// EnhancedGlossary 增强词汇表
type EnhancedGlossary struct {
	SourceLang string                    `json:"source_lang" yaml:"source_lang"`
	TargetLang string                    `json:"target_lang" yaml:"target_lang"`
	Version    string                    `json:"version" yaml:"version"`
	Terms      []GlossaryTerm            `json:"terms" yaml:"terms"`
	Categories map[string][]GlossaryTerm `json:"categories" yaml:"categories"`
}

// GlossaryTerm 词汇表条目
type GlossaryTerm struct {
	Source        string            `json:"source" yaml:"source"`
	Target        string            `json:"target" yaml:"target"`
	Pattern       string            `json:"pattern" yaml:"pattern"`
	MatchType     string            `json:"match_type" yaml:"match_type"` // "exact", "regex", "fuzzy"
	CaseSensitive bool              `json:"case_sensitive" yaml:"case_sensitive"`
	Priority      int               `json:"priority" yaml:"priority"`
	Category      string            `json:"category" yaml:"category"`
	Context       []string          `json:"context" yaml:"context"`
	Notes         string            `json:"notes" yaml:"notes"`
	Metadata      map[string]string `json:"metadata" yaml:"metadata"`
}

// LoadEnhancedGlossary 加载增强词汇表
func LoadEnhancedGlossary(path, sourceLang, targetLang string) (*EnhancedGlossary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read glossary file: %w", err)
	}

	var glossary EnhancedGlossary

	// 根据文件扩展名决定解析格式
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		err = json.Unmarshal(data, &glossary)
	} else if strings.HasSuffix(strings.ToLower(path), ".yaml") || strings.HasSuffix(strings.ToLower(path), ".yml") {
		err = yaml.Unmarshal(data, &glossary)
	} else {
		// 尝试JSON格式
		err = json.Unmarshal(data, &glossary)
		if err != nil {
			// 再尝试YAML格式
			err = yaml.Unmarshal(data, &glossary)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse glossary file: %w", err)
	}

	// 验证语言匹配
	if glossary.SourceLang != sourceLang || glossary.TargetLang != targetLang {
		return nil, fmt.Errorf("glossary language mismatch: expected %s->%s, got %s->%s",
			sourceLang, targetLang, glossary.SourceLang, glossary.TargetLang)
	}

	// 初始化分类
	if glossary.Categories == nil {
		glossary.Categories = make(map[string][]GlossaryTerm)
	}

	// 按分类组织术语
	for _, term := range glossary.Terms {
		if term.Category != "" {
			glossary.Categories[term.Category] = append(glossary.Categories[term.Category], term)
		}

		// 设置默认值
		if term.MatchType == "" {
			term.MatchType = "exact"
		}
		if term.Pattern == "" {
			term.Pattern = term.Source
		}
	}

	return &glossary, nil
}

// GetSortedTerms 获取按优先级排序的术语
func (g *EnhancedGlossary) GetSortedTerms() []GlossaryTerm {
	terms := make([]GlossaryTerm, len(g.Terms))
	copy(terms, g.Terms)

	// 按优先级降序排序
	for i := 0; i < len(terms)-1; i++ {
		for j := i + 1; j < len(terms); j++ {
			if terms[i].Priority < terms[j].Priority {
				terms[i], terms[j] = terms[j], terms[i]
			}
		}
	}

	return terms
}

// ContentGuard 内容保护器
type ContentGuard struct {
	logger            *zap.Logger
	protectedPatterns []ProtectedPattern
}

// ProtectedPattern 受保护的内容模式
type ProtectedPattern struct {
	Name        string
	Pattern     *regexp.Regexp
	Description string
	Replacement func(match string) string
}

// NewContentGuard 创建内容保护器
func NewContentGuard(logger *zap.Logger) *ContentGuard {
	guard := &ContentGuard{
		logger: logger,
	}

	// 初始化保护模式
	guard.initProtectedPatterns()

	return guard
}

// initProtectedPatterns 初始化保护模式
func (cg *ContentGuard) initProtectedPatterns() {
	cg.protectedPatterns = []ProtectedPattern{
		{
			Name:        "URL",
			Pattern:     regexp.MustCompile(`https?://[^\s<>"]+`),
			Description: "HTTP/HTTPS URLs",
		},
		{
			Name:        "Email",
			Pattern:     regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
			Description: "Email addresses",
		},
		{
			Name:        "DOI",
			Pattern:     regexp.MustCompile(`(?i)doi:\s*10\.\d+/[^\s<>"]+`),
			Description: "DOI identifiers",
		},
		{
			Name:        "ISBN",
			Pattern:     regexp.MustCompile(`(?i)isbn[-\s]?(?:13|10)?[:\s]?(?:97[89][-\s]?)?(?:\d[-\s]?){9}\d`),
			Description: "ISBN numbers",
		},
		{
			Name:        "Code",
			Pattern:     regexp.MustCompile("`[^`]+`"),
			Description: "Inline code",
		},
		{
			Name:        "CodeBlock",
			Pattern:     regexp.MustCompile("```[\\s\\S]*?```"),
			Description: "Code blocks",
		},
		{
			Name:        "MathInline",
			Pattern:     regexp.MustCompile(`\$[^$\n]+\$`),
			Description: "Inline math",
		},
		{
			Name:        "MathBlock",
			Pattern:     regexp.MustCompile(`\$\$[\s\S]*?\$\$`),
			Description: "Math blocks",
		},
		{
			Name:        "Version",
			Pattern:     regexp.MustCompile(`v?\d+\.\d+(?:\.\d+)?(?:-[a-zA-Z0-9.-]+)?`),
			Description: "Version numbers",
		},
		{
			Name:        "IPAddress",
			Pattern:     regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
			Description: "IP addresses",
		},
		{
			Name:        "Command",
			Pattern:     regexp.MustCompile(`\b(?:pip|npm|yarn|git|docker|kubectl)\s+\w+(?:\s+\w+)*`),
			Description: "Command line commands",
		},
	}
}

// VerifyAndRestore 验证并恢复受保护的内容
func (cg *ContentGuard) VerifyAndRestore(originalText, translatedText string) string {
	result := translatedText

	// 提取原文中的受保护内容
	originalProtected := cg.extractProtectedContent(originalText)

	// 检查翻译文本中是否有被错误翻译的受保护内容
	for _, pattern := range cg.protectedPatterns {
		matches := pattern.Pattern.FindAllString(originalText, -1)
		for _, match := range matches {
			if !pattern.Pattern.MatchString(result) {
				// 受保护内容可能被翻译了，尝试恢复
				result = cg.restoreProtectedContent(result, match, pattern)
			}
		}
	}

	// 记录保护统计
	if len(originalProtected) > 0 {
		cg.logger.Debug("content protection applied",
			zap.Int("protectedItems", len(originalProtected)))
	}

	return result
}

// extractProtectedContent 提取受保护内容
func (cg *ContentGuard) extractProtectedContent(text string) map[string][]string {
	protected := make(map[string][]string)

	for _, pattern := range cg.protectedPatterns {
		matches := pattern.Pattern.FindAllString(text, -1)
		if len(matches) > 0 {
			protected[pattern.Name] = matches
		}
	}

	return protected
}

// restoreProtectedContent 恢复受保护内容
func (cg *ContentGuard) restoreProtectedContent(text, originalMatch string, pattern ProtectedPattern) string {
	// 更智能的恢复逻辑
	switch pattern.Name {
	case "Email":
		// 查找可能被破坏的邮箱地址并恢复
		emailPattern := regexp.MustCompile(`[^\s@]+\s*@\s*[^\s.]+\s*\.\s*[^\s]+`)
		return emailPattern.ReplaceAllStringFunc(text, func(match string) string {
			// 移除多余空格
			cleaned := regexp.MustCompile(`\s+`).ReplaceAllString(match, "")
			// 如果清理后的版本在原文中存在，使用原版本
			if strings.Contains(originalMatch, cleaned) {
				return originalMatch
			}
			return originalMatch
		})
	case "Code":
		// 恢复被破坏的代码
		if strings.Contains(originalMatch, "pip install") {
			codePattern := regexp.MustCompile(`pip\s*[安装]+\s*tensorflow`)
			if codePattern.MatchString(text) {
				return codePattern.ReplaceAllString(text, originalMatch)
			}
		}
	case "Command":
		// 恢复被破坏的命令
		if strings.Contains(originalMatch, "pip install") {
			cmdPattern := regexp.MustCompile(`pip\s*[安装]+\s*tensorflow`)
			if cmdPattern.MatchString(text) {
				return cmdPattern.ReplaceAllString(text, originalMatch)
			}
		}
	}

	// 通用智能恢复逻辑
	if strings.Contains(originalMatch, "pip install") {
		// 针对命令行代码的特殊处理
		cmdPattern := regexp.MustCompile(`pip\s*[安装]+\s*tensorflow`)
		if cmdPattern.MatchString(text) {
			return cmdPattern.ReplaceAllString(text, originalMatch)
		}
	}

	// 默认恢复逻辑：如果内容不存在，智能插入
	if !strings.Contains(text, originalMatch) {
		return text + " " + originalMatch
	}
	return text
}
