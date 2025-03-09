package formats

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	formatter "github.com/mdigger/goldmark-formatter"
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"go.uber.org/zap"
)

// MarkdownProcessor 是Markdown文件的处理器
type MarkdownProcessor struct {
	BaseProcessor
	logger              *zap.Logger
	currentInputFile    string // 当前正在处理的输入文件路径
	currentReplacements []ReplacementInfo
	currentParts        []markdownPart
}

// NewMarkdownProcessor 创建一个新的Markdown处理器
func NewMarkdownProcessor(t translator.Translator) (*MarkdownProcessor, error) {
	// 获取 zap.Logger
	var zapLogger *zap.Logger
	if log, ok := t.GetLogger().(interface{ GetZapLogger() *zap.Logger }); ok {
		zapLogger = log.GetZapLogger()
	} else {
		// 如果无法获取 zap.Logger，创建一个新的
		zapLogger, _ = zap.NewProduction()
	}

	return &MarkdownProcessor{
		BaseProcessor: BaseProcessor{
			Translator: t,
			Name:       "Markdown",
		},
		logger: zapLogger,
	}, nil
}

// TranslationResult 保存翻译结果和相关信息
type TranslationResult struct {
	RawTranslation string                 `json:"raw_translation"`
	Replacements   []ReplacementInfo      `json:"replacements"`
	Parts          []markdownPart         `json:"parts"`
	Metadata       map[string]interface{} `json:"metadata"`
}

// ReplacementInfo 保存替换信息
type ReplacementInfo struct {
	Placeholder string `json:"placeholder"`
	Original    string `json:"original"`
}

// TranslateFile 翻译Markdown文件
func (p *MarkdownProcessor) TranslateFile(inputPath, outputPath string) error {
	// 设置当前输入文件路径
	p.currentInputFile = inputPath

	// 读取输入文件
	content, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("读取文件失败 %s: %w", inputPath, err)
	}

	// 获取配置
	var autoSaveInterval int = 300    // 默认5分钟
	var translationTimeout int = 1800 // 默认30分钟

	if cfg, ok := p.Translator.(interface{ GetConfig() *config.Config }); ok {
		config := cfg.GetConfig()
		if config.AutoSaveInterval > 0 {
			autoSaveInterval = config.AutoSaveInterval
		}
		if config.TranslationTimeout > 0 {
			translationTimeout = config.TranslationTimeout
		}
	}

	log := p.logger
	log.Info("开始翻译文件",
		zap.String("输入文件", inputPath),
		zap.String("输出文件", outputPath),
		zap.Int("自动保存间隔", autoSaveInterval),
		zap.Int("翻译超时时间", translationTimeout),
	)

	// 创建上下文，用于超时控制
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(translationTimeout)*time.Second)
	defer cancel()

	// 创建通道，用于接收翻译结果和进度更新
	resultCh := make(chan struct {
		text string
		err  error
	})

	// 启动翻译协程
	go func() {
		translated, err := p.TranslateText(string(content))
		resultCh <- struct {
			text string
			err  error
		}{translated, err}
	}()

	// 创建自动保存计时器
	ticker := time.NewTicker(time.Duration(autoSaveInterval) * time.Second)
	defer ticker.Stop()

	// 用于存储部分结果
	var partialResult string

	// 等待翻译完成或超时
	for {
		select {
		case <-ctx.Done():
			// 翻译超时
			log.Error("翻译超时",
				zap.String("输入文件", inputPath),
				zap.Error(ctx.Err()),
			)

			// 如果有部分结果，保存它
			if partialResult != "" {
				tempOutputPath := outputPath + ".partial"
				if err := os.WriteFile(tempOutputPath, []byte(partialResult), 0644); err != nil {
					log.Error("保存部分结果失败",
						zap.String("输出文件", tempOutputPath),
						zap.Error(err),
					)
				} else {
					log.Info("已保存部分结果",
						zap.String("输出文件", tempOutputPath),
					)
				}
			}

			return fmt.Errorf("翻译超时: %w", ctx.Err())

		case result := <-resultCh:
			// 翻译完成
			if result.err != nil {
				return fmt.Errorf("翻译Markdown失败: %w", result.err)
			}

			// 写入输出文件
			if err := os.WriteFile(outputPath, []byte(result.text), 0644); err != nil {
				return fmt.Errorf("写入文件失败 %s: %w", outputPath, err)
			}

			// 保存原始翻译结果和替换信息
			rawOutputPath := strings.TrimSuffix(outputPath, ".md") + "_raw.json"
			translationResult := &TranslationResult{
				RawTranslation: result.text,
				Replacements:   p.currentReplacements,
				Parts:          p.currentParts,
				Metadata: map[string]interface{}{
					"input_file":       inputPath,
					"output_file":      outputPath,
					"translation_time": time.Now().Format(time.RFC3339),
				},
			}

			// 将结果转换为JSON并保存
			jsonData, err := json.MarshalIndent(translationResult, "", "  ")
			if err != nil {
				log.Error("转换JSON失败", zap.Error(err))
			} else {
				if err := os.WriteFile(rawOutputPath, jsonData, 0644); err != nil {
					log.Error("保存原始翻译结果失败",
						zap.String("文件路径", rawOutputPath),
						zap.Error(err))
				} else {
					log.Info("已保存原始翻译结果",
						zap.String("文件路径", rawOutputPath))
				}
			}

			log.Info("翻译完成",
				zap.String("输出文件", outputPath),
			)

			return nil
		case <-ticker.C:
			// 自动保存和进度更新
			if partialResult != "" {
				tempOutputPath := outputPath + ".partial"
				if err := os.WriteFile(tempOutputPath, []byte(partialResult), 0644); err != nil {
					log.Error("自动保存失败",
						zap.String("输出文件", tempOutputPath),
						zap.Error(err),
					)
				} else {
					log.Info("自动保存成功",
						zap.String("输出文件", tempOutputPath),
					)
				}
			}
		}
	}
}

// translationProgress 用于跟踪翻译进度
type translationProgress struct {
	mu sync.Mutex
	// 总字数
	totalChars int
	// 已翻译字数
	translatedChars int
	// 开始时间
	startTime time.Time
	// 预计剩余时间（秒）
	estimatedTimeRemaining float64
}

// updateProgress 更新翻译进度
func (tp *translationProgress) updateProgress(chars int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.translatedChars += chars

	// 计算预计剩余时间
	elapsed := time.Since(tp.startTime).Seconds()
	if tp.translatedChars > 0 {
		charsPerSecond := float64(tp.translatedChars) / elapsed
		remainingChars := tp.totalChars - tp.translatedChars
		tp.estimatedTimeRemaining = float64(remainingChars) / charsPerSecond
	}
}

// getProgress 获取当前进度信息
func (tp *translationProgress) getProgress() (int, int, float64) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.totalChars, tp.translatedChars, tp.estimatedTimeRemaining
}

// formatMarkdown 格式化 Markdown 文本
func (p *MarkdownProcessor) formatMarkdown(text string) (string, error) {
	// 预处理文本，处理图片换行和清理无效标记
	text = processImages(text)
	text = cleanInvalidImageTags(text)
	text = fixTitleSeparators(text)
	text = addMathSpaces(text)

	// 创建一个新的 goldmark 实例，使用 formatter.Markdown 渲染器
	md := goldmark.New(
		goldmark.WithRenderer(formatter.Markdown), // markdown output
		goldmark.WithExtensions(
			extension.GFM,            // GitHub Flavored Markdown
			extension.Typographer,    // 排版优化
			extension.TaskList,       // 任务列表
			extension.Table,          // 表格
			extension.Strikethrough,  // 删除线
			extension.DefinitionList, // 定义列表
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(), // 自动生成标题ID
			parser.WithAttribute(),     // 允许属性设置
		),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(text), &buf); err != nil {
		return "", fmt.Errorf("格式化Markdown失败: %w", err)
	}

	return buf.String(), nil
}

// processImages 在图片标记后添加换行符
func processImages(text string) string {
	// 匹配完整的图片标记，确保后面没有换行符时添加换行符
	re := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)([^\n]|$)`)
	return re.ReplaceAllString(text, "![$1]($2)\n$3")
}

// cleanInvalidImageTags 清理无效的图片标记
func cleanInvalidImageTags(text string) string {
	// 清理不完整的图片标记
	patterns := []string{
		`!\[图片\]\s*\n`,           // 匹配 ![图片]\n
		`!\[图片\]\s*\n\s*!\[图片\]`, // 匹配 ![图片]\n![图片]
		`!\[图片\]\s*$`,            // 匹配末尾的 ![图片]
		`\(\s*\n\s*\)`,           // 匹配 (\n)
	}

	result := text
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		result = re.ReplaceAllString(result, "")
	}
	return result
}

// fixTitleSeparators 修正标题分隔符的格式
func fixTitleSeparators(text string) string {
	// 匹配标题和分隔符的模式
	re := regexp.MustCompile(`(?m)^([^\n]+)\n\s*(={3,}|-{3,})\s*$`)

	return re.ReplaceAllStringFunc(text, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}

		content := parts[1]
		separator := parts[2]

		// 检查内容中是否包含句号
		lastDotIndex := -1
		for _, dot := range []string{"。", "."} {
			if idx := strings.LastIndex(content, dot); idx > lastDotIndex {
				lastDotIndex = idx
			}
		}

		// 如果找到句号，将内容分成两部分
		if lastDotIndex != -1 {
			sentence := content[:lastDotIndex+1]
			title := strings.TrimSpace(content[lastDotIndex+1:])
			return fmt.Sprintf("%s\n\n%s\n\n%s\n\n", sentence, title, separator)
		}

		// 如果没有找到句号，在标题和分隔符之间添加换行
		return fmt.Sprintf("%s\n\n%s\n\n", content, separator)
	})
}

// addMathSpaces 在数学公式前后添加空格
func addMathSpaces(text string) string {
	// 处理行内公式
	inlineMathRegex := regexp.MustCompile(`([^\s$])\$([^$]+)\$([^\s$])`)
	text = inlineMathRegex.ReplaceAllString(text, "$1 $$$2$$ $3")

	// 处理只缺少前面空格的情况
	text = regexp.MustCompile(`([^\s$])\$([^$]+)\$\s`).ReplaceAllString(text, "$1 $$$2$$ ")

	// 处理只缺少后面空格的情况
	text = regexp.MustCompile(`\s\$([^$]+)\$([^\s$])`).ReplaceAllString(text, " $$$1$$ $2")

	// 处理块级公式，添加双换行符
	blockMathRegex := regexp.MustCompile(`([^\n])\$\$([^$]+)\$\$([^\n])`)
	text = blockMathRegex.ReplaceAllString(text, "$1\n\n$$$$2$$$$\n\n$3")

	// 处理只缺少前面换行的块级公式
	text = regexp.MustCompile(`([^\n])\$\$([^$]+)\$\$\n`).ReplaceAllString(text, "$1\n\n$$$$2$$$$\n")

	// 处理只缺少后面换行的块级公式
	text = regexp.MustCompile(`\n\$\$([^$]+)\$\$([^\n])`).ReplaceAllString(text, "\n$$$$1$$$$\n\n$2")

	return text
}

// TranslateText 翻译Markdown文本
func (p *MarkdownProcessor) TranslateText(text string) (string, error) {
	log := p.logger
	log.Info("开始翻译Markdown文本")

	// 重置当前状态
	p.currentParts = nil
	p.currentReplacements = nil

	// 首先格式化 Markdown
	formattedText, err := p.formatMarkdown(text)
	if err != nil {
		log.Warn("Markdown格式化失败，将使用原始文本", zap.Error(err))
		formattedText = text
	}

	// 保存格式化后的文件
	if strings.HasSuffix(p.currentInputFile, ".md") {
		formattedFilePath := strings.TrimSuffix(p.currentInputFile, ".md") + "_formatted.md"
		if err := os.WriteFile(formattedFilePath, []byte(formattedText), 0644); err != nil {
			log.Warn("保存格式化文件失败",
				zap.String("文件路径", formattedFilePath),
				zap.Error(err))
		} else {
			log.Info("已保存格式化文件",
				zap.String("文件路径", formattedFilePath))
		}
	}

	// 获取配置的分割大小限制
	minSplitSize := 100  // 默认最小分割大小
	maxSplitSize := 1000 // 默认最大分割大小

	if cfg, ok := p.Translator.(interface{ GetConfig() *config.Config }); ok {
		config := cfg.GetConfig()
		if config.MinSplitSize > 0 {
			minSplitSize = config.MinSplitSize
		}
		if config.MaxSplitSize > 0 {
			maxSplitSize = config.MaxSplitSize
		}
	}

	log.Debug("分割大小设置",
		zap.Int("最小分割大小", minSplitSize),
		zap.Int("最大分割大小", maxSplitSize),
	)

	// 分割Markdown文本
	parts, replacements, err := p.splitMarkdown(formattedText, minSplitSize, maxSplitSize)
	if err != nil {
		return "", fmt.Errorf("分割Markdown失败: %w", err)
	}

	// 找出需要翻译的部分
	var translatableParts []int
	for i, part := range parts {
		if part.translatable {
			translatableParts = append(translatableParts, i)
		}
	}

	// 获取配置的并行度
	concurrency := 4 // 默认并行度
	if cfg, ok := p.Translator.(interface{ GetConfig() *config.Config }); ok {
		if cfg.GetConfig().Concurrency > 0 {
			concurrency = cfg.GetConfig().Concurrency
		}
	}

	// 限制并行度不超过需要翻译的部分数量
	if concurrency > len(translatableParts) {
		concurrency = len(translatableParts)
	}

	log.Debug("并行翻译设置",
		zap.Int("需要翻译的部分", len(translatableParts)),
		zap.Int("并行度", concurrency),
	)

	// 初始化进度跟踪
	var totalChars int
	for _, idx := range translatableParts {
		totalChars += len(parts[idx].content)
	}
	progress := NewProgressTracker(totalChars)

	// 如果没有需要翻译的部分，直接返回原文
	if len(translatableParts) == 0 {
		return formattedText, nil
	}

	// 如果只有一个需要翻译的部分，或者并行度为1，使用串行处理
	if len(translatableParts) == 1 || concurrency == 1 {
		for i, part := range parts {
			if part.translatable {
				log.Debug("翻译Markdown部分",
					zap.Int("部分索引", i),
					zap.Int("部分长度", len(part.content)),
					zap.String("部分类型", part.partType),
				)

				// 翻译内容
				translated, err := p.Translator.Translate(part.content)
				if err != nil {
					return "", fmt.Errorf("翻译Markdown部分失败: %w", err)
				}

				// 更新进度
				progress.UpdateProgress(len(part.content))

				// 更新翻译后的内容
				parts[i].content = translated
			}
		}
	} else {
		// 使用并行处理
		type translationJob struct {
			index   int
			content string
		}

		type translationResult struct {
			index      int
			translated string
			err        error
		}

		jobs := make(chan translationJob, len(translatableParts))
		results := make(chan translationResult, len(translatableParts))
		translatedParts := make([]string, len(translatableParts))

		// 启动工作协程
		var wg sync.WaitGroup
		for w := 0; w < concurrency; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobs {
					translated, err := p.Translator.Translate(job.content)
					if err == nil {
						// 更新进度
						progress.UpdateProgress(len(job.content))
					}
					results <- translationResult{
						index:      job.index,
						translated: translated,
						err:        err,
					}
				}
			}()
		}

		// 发送翻译任务
		for i, idx := range translatableParts {
			jobs <- translationJob{
				index:   i, // 使用切片索引而不是部分索引
				content: parts[idx].content,
			}
		}
		close(jobs)

		// 等待所有工作完成
		go func() {
			wg.Wait()
			close(results)
		}()

		// 收集结果
		for result := range results {
			if result.err != nil {
				return "", fmt.Errorf("翻译Markdown部分失败 (索引 %d): %w", result.index, result.err)
			}

			// 将结果存储在正确的位置
			translatedParts[result.index] = result.translated
		}

		// 按顺序更新翻译后的内容
		for i, idx := range translatableParts {
			parts[idx].content = translatedParts[i]
		}
	}

	// 组合翻译后的部分
	var result strings.Builder
	var lastPartWasEmpty bool

	for i, part := range parts {
		content := part.content

		// 移除翻译指令
		content = strings.TrimPrefix(content, "IMPORTANT: Translate the following text while strictly following these rules:")
		content = strings.TrimPrefix(content, "Original text:")

		// 如果是翻译部分，清理掉指令部分
		if part.translatable {
			if idx := strings.Index(content, "Placeholder mappings"); idx != -1 {
				if endIdx := strings.Index(content[idx:], "Original text:"); endIdx != -1 {
					content = content[idx+endIdx+len("Original text:"):]
				}
			}
		}

		// 清理空白
		content = strings.TrimSpace(content)

		// 如果当前部分不为空
		if content != "" {
			// 如果上一个部分不为空且当前部分不以换行开始，添加换行
			if !lastPartWasEmpty && !strings.HasPrefix(content, "\n") {
				result.WriteString("\n")
			}

			// 写入内容
			result.WriteString(content)

			// 如果不是最后一个部分且不以换行结束，添加换行
			if i < len(parts)-1 && !strings.HasSuffix(content, "\n") {
				result.WriteString("\n")
			}

			lastPartWasEmpty = false
		} else {
			lastPartWasEmpty = true
		}
	}

	// 获取最终进度信息
	totalChars, translatedChars, _ := progress.GetProgress()

	log.Info("Markdown文本翻译完成",
		zap.Int("原始总字数", totalChars),
		zap.Int("已翻译字数", translatedChars),
		zap.Float64("翻译速度(字/秒)", float64(translatedChars)/time.Since(progress.startTime).Seconds()),
	)

	// 获取合并后的文本
	finalText := result.String()

	// 还原占位符
	for _, replacement := range p.currentReplacements {
		// 使用正则表达式进行更精确的匹配
		pattern := regexp.MustCompile(`⚡⚡⚡UNTRANSLATABLE_\d+⚡⚡⚡`)
		matches := pattern.FindAllString(finalText, -1)

		// 如果找不到精确匹配，尝试更宽松的匹配
		if len(matches) == 0 {
			// 提取占位符中的数字部分
			numPattern := regexp.MustCompile(`UNTRANSLATABLE_(\d+)`)
			numMatch := numPattern.FindStringSubmatch(replacement.Placeholder)

			if len(numMatch) > 1 {
				placeholderNum := numMatch[1]
				// 构建更宽松的正则表达式
				loosePattern := regexp.MustCompile(`⚡⚡⚡[^⚡]*` + placeholderNum + `[^⚡]*⚡⚡⚡`)
				finalText = loosePattern.ReplaceAllString(finalText, replacement.Original)
			}
		} else {
			// 使用精确的占位符进行替换
			finalText = strings.ReplaceAll(finalText, replacement.Placeholder, replacement.Original)
		}
	}

	// 移除翻译标记
	finalText = strings.ReplaceAll(finalText, "<TEXT TO TRANSLATE>\n", "")
	finalText = strings.ReplaceAll(finalText, "</TEXT TO TRANSLATE>", "")
	finalText = strings.ReplaceAll(finalText, "</SOURCE_TEXT>", "")
	finalText = strings.ReplaceAll(finalText, "<SOURCE_TEXT>", "")
	finalText = strings.ReplaceAll(finalText, "<html><body>", "")
	finalText = strings.ReplaceAll(finalText, "</html></body>", "")

	// 保存原始翻译结果
	if strings.HasSuffix(p.currentInputFile, ".md") {
		rawMdPath := strings.TrimSuffix(p.currentInputFile, ".md") + "_raw.md"
		if err := os.WriteFile(rawMdPath, []byte(finalText), 0644); err != nil {
			log.Error("保存原始翻译结果失败",
				zap.String("文件路径", rawMdPath),
				zap.Error(err))
		} else {
			log.Info("已保存原始翻译结果",
				zap.String("文件路径", rawMdPath))
		}
	}

	// 应用后处理
	if cfg, ok := p.Translator.(interface{ GetConfig() *config.Config }); ok {
		config := cfg.GetConfig()
		if config.PostProcessMarkdown {
			postProcessor := NewMarkdownPostProcessor(config, p.logger)
			finalText = postProcessor.ProcessMarkdown(finalText)
		}
	}

	p.currentParts = parts
	p.currentReplacements = replacements

	return finalText, nil
}

// markdownPart 表示Markdown文本的一部分
type markdownPart struct {
	content      string // 内容
	translatable bool   // 是否可翻译
	partType     string // 部分类型（如代码块、标题等）
}

// splitMarkdown 将Markdown文本分割成可翻译和不可翻译的部分
func (p *MarkdownProcessor) splitMarkdown(text string, minSplitSize, maxSplitSize int) ([]markdownPart, []ReplacementInfo, error) {
	p.logger.Debug("分割Markdown文本",
		zap.Int("最小分割大小", minSplitSize),
		zap.Int("最大分割大小", maxSplitSize),
	)

	// 存储原始内容和对应的标记
	var replacements []ReplacementInfo
	placeholderCount := 0

	// 替换函数
	replacePlaceholder := func(match string) string {
		placeholder := fmt.Sprintf("⚡⚡⚡UNTRANSLATABLE_%d⚡⚡⚡", placeholderCount)
		replacements = append(replacements, ReplacementInfo{
			Placeholder: placeholder,
			Original:    match,
		})
		placeholderCount++
		return placeholder
	}

	// 定义需要替换的模式
	patterns := []struct {
		regex *regexp.Regexp
		name  string
	}{
		{regexp.MustCompile("(?s)```.*?```"), "code_block"},
		{regexp.MustCompile("`[^`]+`"), "inline_code"},
		{regexp.MustCompile(`\$\$[^$]+\$\$`), "block_math"},
		{regexp.MustCompile(`\$[^$]+\$`), "inline_math"},
		{regexp.MustCompile(`\\\\?\(.*?\\\\?\)`), "latex_inline"},
		{regexp.MustCompile(`\\\\?\[.*?\\\\?\]`), "latex_display"},
		{regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`), "image"},
		{regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`), "link"},
		{regexp.MustCompile(`<[^>]+>`), "html_tag"},
		{regexp.MustCompile(`\b[A-Z]{2,}(?:-[A-Z]{2,})*(?:\d+)?(?:-\d+)?\b`), "abbreviation"},
		{regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`), "heading"},
		// 添加学术引用文献格式的识别
		{regexp.MustCompile(`\[\d+\]\s+[A-Z][^,]+,(?:\s+[^,]+,)*\s+"[^"]+,"\s+[^,]+,\s+vol\.\s+\d+`), "citation"},
		{regexp.MustCompile(`\[\d+\]\s+[A-Z][^,]+,(?:\s+and\s+[^,]+)*,\s+"[^"]+,"\s+[^,]+,\s+\d{4}`), "citation"},
	}

	// 替换所有特殊格式为占位符
	processedText := text
	for _, pattern := range patterns {
		if pattern.name == "heading" {
			// 特殊处理标题，保留#符号
			processedText = pattern.regex.ReplaceAllStringFunc(processedText, func(match string) string {
				parts := pattern.regex.FindStringSubmatch(match)
				if len(parts) == 3 {
					placeholder := replacePlaceholder(parts[2])
					return parts[1] + " " + placeholder
				}
				return match
			})
		} else {
			processedText = pattern.regex.ReplaceAllStringFunc(processedText, replacePlaceholder)
		}
	}

	// 按行分割处理后的文本
	lines := strings.Split(processedText, "\n")
	var parts []markdownPart
	var currentPart markdownPart
	currentPart.translatable = true
	currentPart.partType = "text"

	// 按行组合文本
	for _, line := range lines {
		newContent := currentPart.content
		if len(newContent) > 0 {
			newContent += "\n"
		}
		newContent += line

		if len(newContent) > maxSplitSize && len(currentPart.content) >= minSplitSize {
			// 如果添加会超过最大大小且当前部分已达到最小大小，保存当前部分
			parts = append(parts, currentPart)
			currentPart = markdownPart{
				content:      line + "\n",
				translatable: true,
				partType:     "text",
			}
		} else {
			// 否则添加到当前部分
			currentPart.content = newContent
		}
	}

	// 添加最后一个部分（如果不为空且达到最小大小）
	if len(currentPart.content) >= minSplitSize {
		parts = append(parts, currentPart)
	}

	p.currentParts = parts
	p.currentReplacements = replacements

	p.logger.Debug("Markdown文本分割完成",
		zap.Int("部分数量", len(parts)),
	)

	return parts, replacements, nil
}

// splitByPattern 按指定模式分割文本
func splitByPattern(text, pattern string) []string {
	// 处理特殊情况：空文本或空模式
	if text == "" || pattern == "" {
		return []string{text}
	}

	// 分割文本
	parts := strings.Split(text, pattern)

	// 重新添加分隔符（除了最后一个部分）
	for i := 0; i < len(parts)-1; i++ {
		parts[i] = parts[i] + pattern
	}

	// 过滤空部分
	var result []string
	for _, part := range parts {
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}

// mergeSmallParts 合并太小的部分
func (p *MarkdownProcessor) mergeSmallParts(parts []markdownPart, minSize int) []markdownPart {
	if len(parts) <= 1 {
		return parts
	}

	var result []markdownPart
	var current markdownPart
	current = parts[0]

	for i := 1; i < len(parts); i++ {
		// 如果当前部分和下一个部分都是可翻译的，并且当前部分太小，则合并
		if current.translatable && parts[i].translatable && len(current.content) < minSize {
			current.content += parts[i].content
		} else {
			// 否则，添加当前部分到结果中，并开始新的部分
			result = append(result, current)
			current = parts[i]
		}
	}

	// 添加最后一个部分
	result = append(result, current)

	return result
}

// splitIntoSentences 将文本分割成句子
func (p *MarkdownProcessor) splitIntoSentences(text string) []string {
	// 简单的句子分割，可以根据需要改进
	sentenceEnders := []string{". ", "! ", "? ", "。", "！", "？", "\n"}
	var sentences []string

	remaining := text
	for len(remaining) > 0 {
		bestIndex := len(remaining)

		for _, ender := range sentenceEnders {
			index := strings.Index(remaining, ender)
			if index >= 0 && index < bestIndex {
				bestIndex = index + len(ender)
			}
		}

		if bestIndex < len(remaining) {
			sentences = append(sentences, remaining[:bestIndex])
			remaining = remaining[bestIndex:]
		} else {
			sentences = append(sentences, remaining)
			break
		}
	}

	return sentences
}

// isLikelyCodeIdentifier 判断是否可能是代码标识符
func isLikelyCodeIdentifier(s string) bool {
	// 检查是否包含典型的代码结构特征
	if strings.Contains(s, ".") || strings.Contains(s, "_") {
		return true
	}

	// 检查是否是驼峰命名
	if regexp.MustCompile(`[a-z][A-Z]`).MatchString(s) {
		return true
	}

	// 检查是否可能是常见的编程语言关键词或函数
	commonCodeTerms := []string{"get", "set", "init", "func", "var", "const", "val", "let", "fn", "def", "func", "append", "remove", "add"}
	for _, term := range commonCodeTerms {
		if strings.HasPrefix(strings.ToLower(s), term) || strings.HasSuffix(strings.ToLower(s), term) {
			return true
		}
	}

	return false
}

// FormatFile 格式化Markdown文件
func (p *MarkdownProcessor) FormatFile(inputPath, outputPath string) error {
	// 读取输入文件
	content, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("读取文件失败 %s: %w", inputPath, err)
	}

	log := p.logger
	log.Info("开始格式化文件",
		zap.String("输入文件", inputPath),
		zap.String("输出文件", outputPath),
	)

	// 格式化 Markdown
	formattedText, err := p.formatMarkdown(string(content))
	if err != nil {
		return fmt.Errorf("格式化Markdown失败: %w", err)
	}

	// 写入输出文件
	if err := os.WriteFile(outputPath, []byte(formattedText), 0644); err != nil {
		return fmt.Errorf("写入文件失败 %s: %w", outputPath, err)
	}

	log.Info("格式化完成",
		zap.String("输出文件", outputPath),
	)

	return nil
}

// MarkdownFormattingProcessor 是 Markdown 格式化处理器
type MarkdownFormattingProcessor struct {
	logger *zap.Logger
}

// NewMarkdownFormattingProcessor 创建一个新的 Markdown 格式化处理器
func NewMarkdownFormattingProcessor() (*MarkdownFormattingProcessor, error) {
	zapLogger, _ := zap.NewProduction()
	return &MarkdownFormattingProcessor{
		logger: zapLogger,
	}, nil
}

// FormatFile 格式化 Markdown 文件
func (p *MarkdownFormattingProcessor) FormatFile(inputPath, outputPath string) error {
	// 读取输入文件
	content, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("读取文件失败 %s: %w", inputPath, err)
	}

	// 创建一个新的 goldmark 实例，使用 formatter.Markdown 渲染器
	md := goldmark.New(
		goldmark.WithRenderer(formatter.Markdown), // markdown output
		goldmark.WithExtensions(
			extension.GFM,            // GitHub Flavored Markdown
			extension.Typographer,    // 排版优化
			extension.TaskList,       // 任务列表
			extension.Table,          // 表格
			extension.Strikethrough,  // 删除线
			extension.DefinitionList, // 定义列表
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(), // 自动生成标题ID
			parser.WithAttribute(),     // 允许属性设置
		),
	)

	var buf bytes.Buffer
	if err := md.Convert(content, &buf); err != nil {
		return fmt.Errorf("格式化Markdown失败: %w", err)
	}

	// 写入输出文件
	if err := os.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("写入文件失败 %s: %w", outputPath, err)
	}

	return nil
}
