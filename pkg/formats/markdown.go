package formats

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// MarkdownProcessor 是Markdown文件的处理器
type MarkdownProcessor struct {
	BaseProcessor
	logger *zap.Logger
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

// TranslateFile 翻译Markdown文件
func (p *MarkdownProcessor) TranslateFile(inputPath, outputPath string) error {
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

	// 创建通道，用于接收翻译结果
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

			log.Info("翻译完成",
				zap.String("输出文件", outputPath),
				zap.Int("原始长度", len(content)),
				zap.Int("翻译长度", len(result.text)),
			)

			return nil

		case <-ticker.C:
			// 自动保存
			// 获取当前翻译进度（这里需要一个方法来获取当前进度）
			if progress, ok := p.Translator.(interface{ GetProgress() string }); ok {
				partialResult = progress.GetProgress()

				// 如果有部分结果，保存它
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
}

// TranslateText 翻译Markdown文本
func (p *MarkdownProcessor) TranslateText(text string) (string, error) {
	log := p.logger
	log.Info("开始翻译Markdown文本")

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
	parts, err := p.splitMarkdown(text, minSplitSize, maxSplitSize)
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

	// 如果没有需要翻译的部分，直接返回原文
	if len(translatableParts) == 0 {
		return text, nil
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

				// 更新翻译后的内容
				parts[i].content = translated
			}
		}
	} else {
		// 使用并行处理
		// 创建工作通道和结果通道
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

		// 启动工作协程
		var wg sync.WaitGroup
		for w := 0; w < concurrency; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobs {
					translated, err := p.Translator.Translate(job.content)
					results <- translationResult{
						index:      job.index,
						translated: translated,
						err:        err,
					}
				}
			}()
		}

		// 发送翻译任务
		for _, idx := range translatableParts {
			jobs <- translationJob{
				index:   idx,
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

			// 更新翻译后的内容
			parts[result.index].content = result.translated

			log.Debug("完成Markdown部分翻译",
				zap.Int("部分索引", result.index),
				zap.Int("原始长度", len(parts[result.index].content)),
				zap.Int("翻译长度", len(result.translated)),
			)
		}
	}

	// 组合翻译后的部分
	var result strings.Builder
	for _, part := range parts {
		result.WriteString(part.content)
	}

	// 应用后处理
	finalText := result.String()
	if cfg, ok := p.Translator.(interface{ GetConfig() *config.Config }); ok {
		config := cfg.GetConfig()
		if config.PostProcessMarkdown {
			postProcessor := NewMarkdownPostProcessor(config, p.logger)
			finalText = postProcessor.ProcessMarkdown(finalText)
		}
	}

	log.Info("Markdown文本翻译完成",
		zap.Int("原始长度", len(text)),
		zap.Int("翻译长度", len(finalText)),
	)

	return finalText, nil
}

// markdownPart 表示Markdown文本的一部分
type markdownPart struct {
	content      string // 内容
	translatable bool   // 是否可翻译
	partType     string // 部分类型（如代码块、标题等）
}

// splitMarkdown 将Markdown文本分割成可翻译和不可翻译的部分
func (p *MarkdownProcessor) splitMarkdown(text string, minSplitSize, maxSplitSize int) ([]markdownPart, error) {
	p.logger.Debug("分割Markdown文本",
		zap.Int("最小分割大小", minSplitSize),
		zap.Int("最大分割大小", maxSplitSize),
	)

	// 定义不可翻译的部分的正则表达式
	codeBlockRegex := regexp.MustCompile("(?s)```.*?```")
	inlineCodeRegex := regexp.MustCompile("`[^`]+`")
	linkRegex := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	imageRegex := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	htmlTagRegex := regexp.MustCompile(`<[^>]+>`)

	// 添加：识别可能的图片URL模式，确保它们不被拆分
	imageExtensions := `\.(?:jpg|jpeg|png|gif|webp|svg|bmp|tiff)`
	imageURLRegex := regexp.MustCompile(`(?:images|img|pics|assets|static|resources|public|uploads|media)\/[a-zA-Z0-9_\-\.\/]+` + imageExtensions)
	httpImageURLRegex := regexp.MustCompile(`https?:\/\/[a-zA-Z0-9_\-\.\/]+` + imageExtensions)

	// 标记不可翻译的部分
	translatable := make([]bool, len(text))
	for i := range translatable {
		translatable[i] = true
	}

	// 标记代码块
	codeBlockMatches := codeBlockRegex.FindAllStringIndex(text, -1)
	for _, match := range codeBlockMatches {
		for i := match[0]; i < match[1]; i++ {
			translatable[i] = false
		}
	}

	// 标记行内代码
	inlineCodeMatches := inlineCodeRegex.FindAllStringIndex(text, -1)
	for _, match := range inlineCodeMatches {
		for i := match[0]; i < match[1]; i++ {
			translatable[i] = false
		}
	}

	// 标记链接URL（保留链接文本）
	linkMatches := linkRegex.FindAllStringSubmatchIndex(text, -1)
	for _, match := range linkMatches {
		// 链接文本可翻译，URL不可翻译
		for i := match[4]; i < match[5]; i++ {
			translatable[i] = false
		}
	}

	// 标记图片（整个图片标签都不可翻译，确保完整性）
	imageMatches := imageRegex.FindAllStringIndex(text, -1)
	for _, match := range imageMatches {
		for i := match[0]; i < match[1]; i++ {
			translatable[i] = false
		}
	}

	// 标记HTML标签
	htmlTagMatches := htmlTagRegex.FindAllStringIndex(text, -1)
	for _, match := range htmlTagMatches {
		for i := match[0]; i < match[1]; i++ {
			translatable[i] = false
		}
	}

	// 添加：标记可能的图片URL，确保它们不被拆分
	imageURLMatches := imageURLRegex.FindAllStringIndex(text, -1)
	for _, match := range imageURLMatches {
		for i := match[0]; i < match[1]; i++ {
			translatable[i] = false
		}
	}

	httpImageURLMatches := httpImageURLRegex.FindAllStringIndex(text, -1)
	for _, match := range httpImageURLMatches {
		for i := match[0]; i < match[1]; i++ {
			translatable[i] = false
		}
	}

	// 添加：确保图片标记周围的括号也不被拆分
	// 查找所有可能的图片标记周围的括号
	bracketRegex := regexp.MustCompile(`\((?:images|img|pics|assets|static|resources|public|uploads|media)\/[^\)]+\)|\(https?:\/\/[^\)]+\)`)
	bracketMatches := bracketRegex.FindAllStringIndex(text, -1)
	for _, match := range bracketMatches {
		for i := match[0]; i < match[1]; i++ {
			translatable[i] = false
		}
	}

	// 根据标记分割文本
	var parts []markdownPart
	var currentPart markdownPart
	currentTranslatable := translatable[0]
	currentPart.translatable = currentTranslatable
	currentPart.partType = "text"

	for i, char := range text {
		if translatable[i] == currentTranslatable {
			currentPart.content += string(char)
		} else {
			// 如果当前部分不为空，添加到结果中
			if len(currentPart.content) > 0 {
				parts = append(parts, currentPart)
			}

			// 开始新的部分
			currentTranslatable = translatable[i]
			currentPart = markdownPart{
				content:      string(char),
				translatable: currentTranslatable,
				partType:     "text",
			}
		}
	}

	// 添加最后一个部分
	if len(currentPart.content) > 0 {
		parts = append(parts, currentPart)
	}

	// 合并太小的部分
	if minSplitSize > 0 {
		parts = p.mergeSmallParts(parts, minSplitSize)
	}

	// 分割太大的部分，优先在换行符处分割
	if maxSplitSize > 0 {
		var newParts []markdownPart
		for _, part := range parts {
			if part.translatable && len(part.content) > maxSplitSize {
				// 检查是否包含可能的图片URL，如果包含则不拆分
				if imageURLRegex.MatchString(part.content) || httpImageURLRegex.MatchString(part.content) {
					newParts = append(newParts, part)
					continue
				}

				// 首先尝试按段落（双换行）分割
				paragraphs := splitByPattern(part.content, "\n\n")

				// 如果段落分割后仍有超大段落，则按单个换行符分割
				var processedParagraphs []string
				for _, paragraph := range paragraphs {
					if len(paragraph) > maxSplitSize {
						// 按单个换行符分割
						lines := splitByPattern(paragraph, "\n")
						processedParagraphs = append(processedParagraphs, lines...)
					} else {
						processedParagraphs = append(processedParagraphs, paragraph)
					}
				}

				// 如果按换行符分割后仍有超大段落，则按句子分割
				var currentContent string
				for _, paragraph := range processedParagraphs {
					if len(paragraph) > maxSplitSize {
						// 如果当前已有内容，先添加
						if len(currentContent) > 0 {
							newParts = append(newParts, markdownPart{
								content:      currentContent,
								translatable: true,
								partType:     "text",
							})
							currentContent = ""
						}

						// 按句子分割大段落
						sentences := p.splitIntoSentences(paragraph)
						for _, sentence := range sentences {
							// 检查句子是否包含可能的图片URL，如果包含则单独作为一部分
							if imageURLRegex.MatchString(sentence) || httpImageURLRegex.MatchString(sentence) {
								// 如果当前已有内容，先添加
								if len(currentContent) > 0 {
									newParts = append(newParts, markdownPart{
										content:      currentContent,
										translatable: true,
										partType:     "text",
									})
									currentContent = ""
								}

								// 将包含图片URL的句子作为单独的部分
								newParts = append(newParts, markdownPart{
									content:      sentence,
									translatable: true,
									partType:     "text",
								})
								continue
							}

							if len(currentContent)+len(sentence) <= maxSplitSize || len(currentContent) == 0 {
								currentContent += sentence
							} else {
								newParts = append(newParts, markdownPart{
									content:      currentContent,
									translatable: true,
									partType:     "text",
								})
								currentContent = sentence
							}
						}
					} else {
						// 处理正常大小的段落
						if len(currentContent)+len(paragraph) <= maxSplitSize || len(currentContent) == 0 {
							// 如果当前内容为空，直接添加段落
							if len(currentContent) == 0 {
								currentContent = paragraph
							} else {
								// 否则，确保段落之间有换行符
								if !strings.HasSuffix(currentContent, "\n") {
									currentContent += "\n"
								}
								if !strings.HasPrefix(paragraph, "\n") {
									currentContent += "\n"
								}
								currentContent += paragraph
							}
						} else {
							// 如果添加段落会超过最大大小，先保存当前内容，然后开始新的内容
							newParts = append(newParts, markdownPart{
								content:      currentContent,
								translatable: true,
								partType:     "text",
							})
							currentContent = paragraph
						}
					}
				}

				// 添加最后一个部分
				if len(currentContent) > 0 {
					newParts = append(newParts, markdownPart{
						content:      currentContent,
						translatable: true,
						partType:     "text",
					})
				}
			} else {
				newParts = append(newParts, part)
			}
		}
		parts = newParts
	}

	p.logger.Debug("Markdown文本分割完成",
		zap.Int("部分数量", len(parts)),
	)

	return parts, nil
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
