package formats

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// TranslationResult 存储翻译结果和相关信息
type TranslationResult struct {
	RawTranslation string                 `json:"raw_translation"` // 原始翻译结果
	Replacements   []ReplacementInfo      `json:"replacements"`    // 替换信息
	Parts          []markdownPart         `json:"parts"`           // Markdown部分
	Metadata       map[string]interface{} `json:"metadata"`        // 元数据
}

// markdownPart 表示Markdown文本的一部分
type markdownPart struct {
	Content      string `json:"content"`      // 内容
	Translatable bool   `json:"translatable"` // 是否可翻译
	PartType     string `json:"part_type"`    // 部分类型（如代码块、标题等）
}

// MarkdownProcessor 是Markdown文件处理器
type MarkdownProcessor struct {
	BaseProcessor
	logger              *zap.Logger
	currentInputFile    string // 当前正在处理的输入文件路径
	currentReplacements []ReplacementInfo
	config              *config.Config // 添加配置字段
}

// pre-compile regex
var (
	mdCodeBlockRegex  = regexp.MustCompile("(?s)```(.*?)```") // (?s) 让 . 能匹配换行
	mdMultiMathRegex  = regexp.MustCompile(`(?s)\$\$(.*?)\$\$`)
	mdTableBlockRegex = regexp.MustCompile(
		`(?m)^[ \t]*\|.*\|[ \t]*\r?\n` +
			`^[ \t]*\|[ :\-\.\|\t]+?\|[ \t]*\r?\n` +
			`^(?:[ \t]*\|.*\|[ \t]*\r?\n)+`,
	)
	mdInlineCodeRegex = regexp.MustCompile("`[^`]+`")
	mdInlineMathRegex = regexp.MustCompile(`\$[^$]+\$`)
	mdImageRegex      = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	// 匹配"只包含标点、空白、数字" (允许 \r\n 和所有 Unicode 空白(\p{Z})、标点(\p{P})、数字(\p{N}))
	// ^...$ 表示整段文本从头到尾完全匹配
	mdReOnlySymbols = regexp.MustCompile(`^[\p{P}\p{Z}\p{N}\r\n]*$`)
)

// NewMarkdownProcessor 创建一个新的Markdown处理器
func NewMarkdownProcessor(t translator.Translator, predefinedTranslations *config.PredefinedTranslation, progressBar *progress.Writer) (*MarkdownProcessor, error) {
	var cfg *config.Config
	if configProvider, ok := t.(interface{ GetConfig() *config.Config }); ok {
		cfg = configProvider.GetConfig()
	}

	// 获取logger，如果无法转换则创建新的
	zapLogger, _ := zap.NewProduction()
	if loggerProvider, ok := t.GetLogger().(interface{ GetZapLogger() *zap.Logger }); ok {
		if zl := loggerProvider.GetZapLogger(); zl != nil {
			zapLogger = zl
		}
	}

	zapLogger.Debug("Loading predefined translations", zap.Int("count", len(predefinedTranslations.Translations)))

	return &MarkdownProcessor{
		BaseProcessor: BaseProcessor{
			Translator:             t,
			Name:                   "Markdown",
			predefinedTranslations: predefinedTranslations,
			progressBar:            progressBar,
		},
		logger: zapLogger,
		config: cfg,
	}, nil
}

// TranslateFile 翻译Markdown文件
func (p *MarkdownProcessor) TranslateFile(inputPath, outputPath string) error {

	if err := FormatFile(inputPath); err != nil {
		p.logger.Warn("格式化输入文件失败", zap.Error(err))
	}

	// 1. 读取文件内容
	contentBytes, err := os.ReadFile(inputPath)
	if err != nil {
		p.logger.Error("无法读取文件", zap.Error(err), zap.String("文件路径", inputPath))
		return fmt.Errorf("无法读取文件 %s: %v", inputPath, err)
	}
	originalText := string(contentBytes)

	// 2. 先进行占位符保护（先多行再单行）
	protectedText, replacements := p.protectMarkdown(originalText)

	p.currentReplacements = replacements

	// 3.1 合并连续的占位符段落
	protectedText = p.combineConsecutivePlaceholderParagraphs(protectedText)

	// 3. 保存 replacements
	replacementsJson, err := json.MarshalIndent(ReplacementInfoList{Replacements: p.currentReplacements}, "", "    ")
	if err != nil {
		p.logger.Error("无法序列化替换信息", zap.Error(err))
		return fmt.Errorf("无法序列化替换信息: %v", err)
	}
	replacementsPath := strings.TrimSuffix(outputPath, ".md") + ".replacements.json"
	err = os.WriteFile(replacementsPath, replacementsJson, os.ModePerm)
	if err != nil {
		p.logger.Error("无法写出替换信息", zap.Error(err), zap.String("文件路径", replacementsPath))
		return fmt.Errorf("无法写出替换信息 %s: %v", replacementsPath, err)
	}
	protectedTextFile := strings.TrimSuffix(outputPath, ".md") + ".protected.md"

	if !IsFileExists(replacementsPath) || !IsFileExists(protectedTextFile) {

		FormatFile(inputPath)

		// 1. 读取文件内容
		contentBytes, err := os.ReadFile(inputPath)
		if err != nil {
			p.logger.Error("无法读取文件", zap.Error(err), zap.String("文件路径", inputPath))
			return fmt.Errorf("无法读取文件 %s: %v", inputPath, err)
		}
		originalText := string(contentBytes)

		p.Translator.GetProgressTracker().SetRealTotalChars(len(originalText))

		// 2. 先进行占位符保护（先多行再单行）
		protectedText, replacements := p.protectMarkdown(originalText)

		p.currentReplacements = replacements

		// 3.1 合并连续的占位符段落
		protectedText = p.combineConsecutivePlaceholderParagraphs(protectedText)

		p.Translator.GetProgressTracker().SetTotalChars(len(protectedText))

		// 3. 保存 replacements
		replacementsJson, err := json.MarshalIndent(ReplacementInfoList{Replacements: p.currentReplacements}, "", "    ")
		if err != nil {
			p.logger.Error("无法序列化替换信息", zap.Error(err))
			return fmt.Errorf("无法序列化替换信息: %v", err)
		}
		err = os.WriteFile(replacementsPath, replacementsJson, os.ModePerm)
		if err != nil {
			p.logger.Error("无法写出替换信息", zap.Error(err), zap.String("文件路径", replacementsPath))
			return fmt.Errorf("无法写出替换信息 %s: %v", replacementsPath, err)
		}

		err = os.WriteFile(protectedTextFile, []byte(protectedText), os.ModePerm)
		if err != nil {
			p.logger.Error("无法写出保护后的文本", zap.Error(err), zap.String("文件路径", protectedTextFile))
			return fmt.Errorf("无法写出保护后的文本 %s: %v", protectedTextFile, err)
		}

		FormatFile(protectedTextFile)

	} else {
		protectedTextBytes, err := os.ReadFile(protectedTextFile)
		if err != nil {
			p.logger.Error("无法读取保护后的文本", zap.Error(err), zap.String("文件路径", protectedTextFile))
			return fmt.Errorf("无法读取保护后的文本 %s: %v", protectedTextFile, err)
		}
		protectedText = string(protectedTextBytes)

		replacementsJson, err := os.ReadFile(replacementsPath)
		if err != nil {
			p.logger.Error("无法读取替换信息", zap.Error(err), zap.String("文件路径", replacementsPath))
			return fmt.Errorf("无法读取替换信息 %s: %v", replacementsPath, err)
		}
		var replacements ReplacementInfoList
		err = json.Unmarshal(replacementsJson, &replacements)
		if err != nil {
			p.logger.Error("无法反序列化替换信息", zap.Error(err), zap.String("文件路径", replacementsPath))
			return fmt.Errorf("无法反序列化替换信息 %s: %v", replacementsPath, err)
		}
		p.currentReplacements = replacements.Replacements
	}

	p.Translator.InitTranslator()

	// 4. 使用 splitTextToChunks 分段
	chunks := p.splitTextToChunks(protectedText, p.config.MinSplitSize, p.config.MaxSplitSize)
	p.logger.Info("分段结果", zap.Int("Chunk数", len(chunks)))
	p.logger.Debug("分段具体长度", zap.Int("Chunk数", len(chunks)))
	for chunkID, chunk := range chunks {
		p.logger.Debug("分段ID", zap.Int("ID", chunkID), zap.Int("内容长度", len(chunk.Text)), zap.Bool("是否需要翻译", chunk.NeedToTranslate))
	}

	// 5. 调用 TranslateText 翻译每个分段
	var translatedBuilder strings.Builder
	translatedChunks, err := parallelTranslateChunks(chunks, p, p.config.Concurrency)
	if err != nil {
		p.logger.Warn("翻译出错", zap.Error(err))
	}
	for _, translatedChunk := range translatedChunks {
		translatedBuilder.WriteString(translatedChunk + "\n\n")
	}
	translated := translatedBuilder.String()

	// 6. 保存中间结果
	outputPathWithExt := strings.TrimSuffix(outputPath, ".md") + ".intermediate.md"
	err = os.WriteFile(outputPathWithExt, []byte(translated), os.ModePerm)
	if err != nil {
		p.logger.Error("无法写出中间结果", zap.Error(err), zap.String("文件路径", outputPathWithExt))
		return fmt.Errorf("无法写出中间结果 %s: %v", outputPathWithExt, err)
	}

	if err := FormatFile(outputPathWithExt); err != nil {
		p.logger.Warn("格式化中间结果失败", zap.Error(err))
	}

	// 7. 将翻译后的内容中占位符还原
	finalResult := p.restoreMarkdown(translated, p.currentReplacements)

	finalResult = RemoveRedundantNewlines(finalResult)

	// 8. 写出翻译结果到目标文件
	err = os.WriteFile(outputPath, []byte(finalResult), os.ModePerm)
	if err != nil {
		p.logger.Error("无法写出文件", zap.Error(err), zap.String("文件路径", outputPath))
		return fmt.Errorf("无法写出文件 %s: %v", outputPath, err)
	}

	if err := FormatFile(outputPath); err != nil {
		p.logger.Warn("格式化输出文件失败", zap.Error(err))
	}

	p.Translator.GetProgressTracker().UpdateRealTranslatedChars(len(finalResult))

	p.Translator.Finish()

	return nil
}

// TranslateText 翻译Markdown文本
func (p *MarkdownProcessor) TranslateText(text string) (string, error) {
	// 这里实际实现可以直接调用 p.Translator.Translate(...)
	// 或者使用你自定义的逻辑。以下仅示例：
	translated, err := p.Translator.Translate(text, true)
	if err != nil {
		return "", err
	}
	return translated, nil
}

// GetCurrentReplacements 获取当前的替换信息
func (p *MarkdownProcessor) GetCurrentReplacements() []ReplacementInfo {
	return p.currentReplacements
}

// SetCurrentReplacements 设置当前的替换信息
func (p *MarkdownProcessor) SetCurrentReplacements(replacements []ReplacementInfo) {
	p.currentReplacements = replacements
}

// GetCurrentInputFile 获取当前正在处理的输入文件路径
func (p *MarkdownProcessor) GetCurrentInputFile() string {
	return p.currentInputFile
}

// SetCurrentInputFile 设置当前正在处理的输入文件路径
func (p *MarkdownProcessor) SetCurrentInputFile(path string) {
	p.currentInputFile = path
}

// GetConfig 获取配置
func (p *MarkdownProcessor) GetConfig() *config.Config {
	return p.config
}

// GetLogger 获取日志记录器
func (p *MarkdownProcessor) GetLogger() *zap.Logger {
	return p.logger
}

// protectMarkdown 会使用正则或其他方法，先把多行块（代码块、表格、数学公式等）
// 再把单行元素（行内代码、行内数学公式、图片等）替换成占位符。
func (p *MarkdownProcessor) protectMarkdown(text string) (string, []ReplacementInfo) {
	var replacements []ReplacementInfo
	placeholderIndex := 0

	// 如果存在预定义的翻译，则先进行预定义的翻译
	if p.predefinedTranslations != nil {
		for key, value := range p.predefinedTranslations.Translations {
			placeholder := fmt.Sprintf("@@PRESERVE_%d@@", placeholderIndex)
			replacements = append(replacements, ReplacementInfo{
				Placeholder: placeholder,
				Original:    value,
			})
			placeholderIndex++
			text = strings.ReplaceAll(text, key, placeholder)
		}
	}

	/*
	   =========================================
	   1. 多行内容保护
	   =========================================
	*/

	// 1.1 保护三引号包裹的多行代码块 ```...```
	text = mdCodeBlockRegex.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("@@PRESERVE_%d@@", placeholderIndex)
		replacements = append(replacements, ReplacementInfo{
			Placeholder: placeholder,
			Original:    match,
		})
		placeholderIndex++
		return "\n" + placeholder + "\n"
	})

	// 1.2 保护多行数学公式 $$...$$

	text = mdMultiMathRegex.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("@@PRESERVE_%d@@", placeholderIndex)
		replacements = append(replacements, ReplacementInfo{
			Placeholder: placeholder,
			Original:    match,
		})
		placeholderIndex++
		return "\n" + placeholder + "\n"
	})

	// 1.3 保护 Markdown 表格
	//     这里仅示例一种最常见的表格式写法：
	//       | col1 | col2 |
	//       |------|------|
	//       | ...  | ...  |
	//
	//     正则逻辑：尝试匹配表头行 + 分割行 + 至少一行数据
	//     在实际项目中，需要根据更多变体进行增强和测试。
	text = mdTableBlockRegex.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("@@PRESERVE_%d@@", placeholderIndex)
		replacements = append(replacements, ReplacementInfo{
			Placeholder: placeholder,
			Original:    match,
		})
		placeholderIndex++
		return "\n" + placeholder + "\n"
	})

	/*
	   =========================================
	   2. 单行内容保护
	   =========================================
	*/

	// 2.1 保护行内代码块 `...`
	text = mdInlineCodeRegex.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("@@PRESERVE_%d@@", placeholderIndex)
		replacements = append(replacements, ReplacementInfo{
			Placeholder: placeholder,
			Original:    match,
		})
		placeholderIndex++
		return " " + placeholder + " "
	})

	// 2.2 保护行内数学公式 $...$
	text = mdInlineMathRegex.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("@@PRESERVE_%d@@", placeholderIndex)
		replacements = append(replacements, ReplacementInfo{
			Placeholder: placeholder,
			Original:    match,
		})
		placeholderIndex++
		return " " + placeholder + " "
	})

	// 2.3 保护 Markdown 图片（简化，只要匹配到 `![...](...)` 就处理）
	text = mdImageRegex.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("@@PRESERVE_%d@@", placeholderIndex)
		replacements = append(replacements, ReplacementInfo{
			Placeholder: placeholder,
			Original:    match,
		})
		placeholderIndex++
		return "\n" + placeholder + "\n"
	})

	// 如果还有其他需要保护的内容，比如 [1] Author et al.、超链接 [文本](链接)，
	// 也可以使用类似方法继续往下加

	lines := strings.Split(text, "\n")
	find_lines := []string{}
	for _, line := range lines {
		// 保护 Markdown 图片（简化，只要匹配到 `![...](...)` 就处理）
		if strings.HasPrefix(line, "![") {
			placeholder := fmt.Sprintf("@@PRESERVE_%d@@", placeholderIndex)
			replacements = append(replacements, ReplacementInfo{
				Placeholder: placeholder,
				Original:    line,
			})
			find_lines = append(find_lines, "\n\n"+placeholder+"\n\n")
			placeholderIndex++
			continue
		}
		// 保护 [1] Author et al.
		if strings.HasPrefix(line, "[") {
			placeholder := fmt.Sprintf("@@PRESERVE_%d@@", placeholderIndex)
			replacements = append(replacements, ReplacementInfo{
				Placeholder: placeholder,
				Original:    line,
			})
			find_lines = append(find_lines, "\n\n"+placeholder+"\n\n")
			placeholderIndex++
			continue
		}
		find_lines = append(find_lines, line)
	}

	text = strings.Join(find_lines, "\n")

	text = RemoveRedundantNewlines(text)

	return text, replacements
}

// restoreMarkdown 将翻译后的文本里的占位符替换回原来的内容
func (p *MarkdownProcessor) restoreMarkdown(translated string, replacements []ReplacementInfo) string {
	// 1. 为了避免部分占位符还原干扰，这里最好一次性从序号最大的开始恢复
	for i := len(replacements) - 1; i >= 0; i-- {
		r := replacements[i]
		translated = strings.ReplaceAll(translated, r.Placeholder, r.Original)
	}
	// 2. 可能出现 PRESERVE 被翻译的情况，这里再处理一下
	translated = mdRePlaceholderWildcard.ReplaceAllStringFunc(translated, func(match string) string {
		// 提取出数字
		parts := mdRePlaceholderWildcard.FindStringSubmatch(match)
		if len(parts) == 3 {
			wildcard := parts[1]
			number := parts[2]
			numberInt, err := strconv.Atoi(number)
			if err != nil {
				p.logger.Warn("无法将数字字符串转换为整数", zap.String("数字字符串", number))
				return match
			}
			if numberInt < len(replacements) {
				p.logger.Debug("有占位符被翻译了, 还原占位符",
					zap.String("占位符", match),
					zap.String("wildcard", wildcard),
					zap.String("原始内容", replacements[numberInt].Original))
				translated = strings.ReplaceAll(translated, match, replacements[numberInt].Original)
				return translated
			}
		}
		return match
	})

	return translated
}

func (p *MarkdownProcessor) isNeedToTranslate(text string) bool {
	// 1. 去除占位符
	withoutPlaceholders := mdRePlaceholder.ReplaceAllString(text, "")

	// 2. 去除首尾空白
	withoutPlaceholders = strings.TrimSpace(withoutPlaceholders)

	// 3. 用正则判断是否只包含标点、数字、空白（包括换行）
	if mdReOnlySymbols.MatchString(withoutPlaceholders) {
		// 如果完全匹配，只剩标点、空白、数字 => 不需要翻译
		return false
	}
	// 否则 => 需要翻译
	return true
}

// splitTextToChunks 将文本分割为适当大小的块，优先按自然段落分割
func (p *MarkdownProcessor) splitTextToChunks(text string, minSize, maxSize int) []Chunk {
	if len(text) <= maxSize {
		return []Chunk{
			{
				Text:            text,
				NeedToTranslate: p.isNeedToTranslate(text),
			},
		}
	}

	var chunks []Chunk
	var currentChunk strings.Builder
	paragraphs := strings.Split(text, "\n\n") // 使用双换行符分割段落

	for _, para := range paragraphs {
		// 如果段落本身超过最大长度，需要进一步分割
		if len(para) > maxSize {
			// 如果当前chunk不为空，先保存它
			if currentChunk.Len() > 0 {
				chunks = append(chunks, Chunk{
					Text:            currentChunk.String(),
					NeedToTranslate: p.isNeedToTranslate(currentChunk.String()),
				})
				currentChunk.Reset()
			}

			// 按句子分割大段落
			start := 0
			for start < len(para) {
				end := start + maxSize
				if end > len(para) {
					end = len(para)
				} else {
					// 寻找合适的分割点（句号、问号、感叹号）
					for i := end - 1; i > start+minSize; i-- {
						if i < len(para)-1 {
							r := []rune(para[i : i+1])[0]

							// 检查是否是句子结束标记（英文或中文标点）
							if r == '.' || r == '?' || r == '!' ||
								r == '。' || r == '？' || r == '！' {
								end = i + 1
								break
							}
						}
					}
				}
				chunks = append(chunks, Chunk{
					Text:            para[start:end],
					NeedToTranslate: p.isNeedToTranslate(para[start:end]),
				})
				start = end
			}
			continue
		}

		// 检查添加当前段落是否会导致超出最大长度
		if currentChunk.Len()+len(para) > maxSize {
			// 如果当前chunk达到最小长度，保存它
			if currentChunk.Len() >= minSize {
				chunks = append(chunks, Chunk{
					Text:            currentChunk.String(),
					NeedToTranslate: p.isNeedToTranslate(currentChunk.String()),
				})
				currentChunk.Reset()
			}
		}

		// 添加段落和段落分隔符
		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(para)
	}

	// 保存最后一个chunk
	if currentChunk.Len() > 0 {
		chunks = append(chunks, Chunk{
			Text:            currentChunk.String(),
			NeedToTranslate: p.isNeedToTranslate(currentChunk.String()),
		})
	}

	return chunks
}

func (p *MarkdownProcessor) isParagraphAllPlaceholder(text string) bool {
	paragraphs := strings.TrimSpace(text)
	paragraphs = strings.ReplaceAll(paragraphs, "\n", "")
	paragraphs = strings.ReplaceAll(paragraphs, " ", "")
	return mdRestrictedPlaceholder.MatchString(paragraphs)
}

func (p *MarkdownProcessor) combineConsecutivePlaceholderParagraphs(text string) string {
	paragraphs := strings.Split(text, "\n\n")

	var output []string
	var placeholderBuffer []string

	bigPlaceholderIndex := len(p.currentReplacements)

	for _, paragraph := range paragraphs {
		if p.isParagraphAllPlaceholder(paragraph) {
			placeholderBuffer = append(placeholderBuffer, paragraph)
		} else {
			if len(placeholderBuffer) > 0 {
				bigPlaceholderBufferString := strings.Join(placeholderBuffer, "\n\n")
				restored := p.restoreMarkdown(bigPlaceholderBufferString, p.currentReplacements)
				newPlaceholder := fmt.Sprintf("@@PRESERVE_%d@@", bigPlaceholderIndex)
				p.currentReplacements = append(p.currentReplacements, ReplacementInfo{
					Placeholder: newPlaceholder,
					Original:    restored,
				})
				bigPlaceholderIndex++
				output = append(output, newPlaceholder)
				placeholderBuffer = []string{}
			}
			output = append(output, paragraph)
		}
	}

	if len(placeholderBuffer) > 0 {
		bigPlaceholderBufferString := strings.Join(placeholderBuffer, "\n\n")
		restored := p.restoreMarkdown(bigPlaceholderBufferString, p.currentReplacements)
		newPlaceholder := fmt.Sprintf("@@PRESERVE_%d@@", bigPlaceholderIndex)
		p.currentReplacements = append(p.currentReplacements, ReplacementInfo{
			Placeholder: newPlaceholder,
			Original:    restored,
		})
		output = append(output, newPlaceholder)
	}
	return strings.Join(output, "\n\n")
}
