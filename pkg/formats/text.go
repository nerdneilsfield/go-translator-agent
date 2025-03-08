package formats

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// TextProcessor 是纯文本文件的处理器
type TextProcessor struct {
	BaseProcessor
}

// NewTextProcessor 创建一个新的文本处理器
func NewTextProcessor(t translator.Translator) (*TextProcessor, error) {
	return &TextProcessor{
		BaseProcessor: BaseProcessor{
			Translator: t,
			Name:       "文本",
		},
	}, nil
}

// TranslateFile 翻译文本文件
func (p *TextProcessor) TranslateFile(inputPath, outputPath string) error {
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

	log := p.Translator.GetLogger()
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

	// 创建临时结果变量
	var partialResult string
	var lastSaveTime time.Time = time.Now()

	// 等待翻译完成或超时
	for {
		select {
		case <-ctx.Done():
			// 翻译超时
			log.Error("翻译超时",
				zap.String("输入文件", inputPath),
				zap.Duration("已用时间", time.Since(lastSaveTime)),
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
				return fmt.Errorf("翻译文本失败: %w", result.err)
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
							zap.Duration("距上次保存", time.Since(lastSaveTime)),
						)
						lastSaveTime = time.Now()
					}
				}
			}
		}
	}
}

// TranslateText 翻译文本内容
func (p *TextProcessor) TranslateText(text string) (string, error) {
	log := p.Translator.GetLogger()
	log.Info("开始翻译文本")

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

	// 按段落分割文本
	paragraphs := splitTextWithSizeLimit(text, minSplitSize, maxSplitSize)

	log.Info("文本已分割",
		zap.Int("段落数", len(paragraphs)),
	)

	// 找出需要翻译的段落（非空段落）
	var translatableParagraphs []int
	for i, paragraph := range paragraphs {
		if strings.TrimSpace(paragraph) != "" {
			translatableParagraphs = append(translatableParagraphs, i)
		}
	}

	// 获取配置的并行度
	concurrency := 4 // 默认并行度
	if cfg, ok := p.Translator.(interface{ GetConfig() *config.Config }); ok {
		if cfg.GetConfig().Concurrency > 0 {
			concurrency = cfg.GetConfig().Concurrency
		}
	}

	// 限制并行度不超过需要翻译的段落数量
	if concurrency > len(translatableParagraphs) {
		concurrency = len(translatableParagraphs)
	}

	log.Debug("并行翻译设置",
		zap.Int("需要翻译的段落数", len(translatableParagraphs)),
		zap.Int("并行度", concurrency),
	)

	// 如果没有需要翻译的段落，直接返回原文
	if len(translatableParagraphs) == 0 {
		return text, nil
	}

	// 如果只有一个需要翻译的段落，或者并行度为1，使用串行处理
	if len(translatableParagraphs) == 1 || concurrency == 1 {
		for i, paragraph := range paragraphs {
			// 跳过空段落
			if strings.TrimSpace(paragraph) == "" {
				continue
			}

			log.Debug("翻译段落",
				zap.Int("段落索引", i),
				zap.Int("段落长度", len(paragraph)),
			)

			// 翻译段落
			translated, err := p.Translator.Translate(paragraph)
			if err != nil {
				return "", fmt.Errorf("翻译段落失败: %w", err)
			}

			// 更新翻译后的段落
			paragraphs[i] = translated
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

		jobs := make(chan translationJob, len(translatableParagraphs))
		results := make(chan translationResult, len(translatableParagraphs))

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
		for _, idx := range translatableParagraphs {
			jobs <- translationJob{
				index:   idx,
				content: paragraphs[idx],
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
				return "", fmt.Errorf("翻译段落失败 (索引 %d): %w", result.index, result.err)
			}

			// 更新翻译后的段落
			paragraphs[result.index] = result.translated

			log.Debug("完成段落翻译",
				zap.Int("段落索引", result.index),
				zap.Int("原始长度", len(paragraphs[result.index])),
				zap.Int("翻译长度", len(result.translated)),
			)
		}
	}

	// 组合翻译后的段落
	translatedText := strings.Join(paragraphs, "\n\n")

	log.Info("文本翻译完成",
		zap.Int("原始长度", len(text)),
		zap.Int("翻译长度", len(translatedText)),
	)

	return translatedText, nil
}

// splitTextWithSizeLimit 按段落分割文本，并考虑大小限制
func splitTextWithSizeLimit(text string, minSize, maxSize int) []string {
	// 首先按段落分割
	paragraphs := strings.Split(text, "\n\n")

	// 如果没有设置大小限制，直接返回段落
	if maxSize <= 0 {
		return paragraphs
	}

	var result []string

	for _, paragraph := range paragraphs {
		// 跳过空段落
		if strings.TrimSpace(paragraph) == "" {
			result = append(result, paragraph)
			continue
		}

		// 如果段落小于最大大小，直接添加
		if len(paragraph) <= maxSize {
			result = append(result, paragraph)
			continue
		}

		// 将大段落分割成更小的部分
		var current strings.Builder
		sentences := splitIntoSentences(paragraph)

		for _, sentence := range sentences {
			// 如果单个句子超过最大大小，按最大大小分割
			if len(sentence) > maxSize {
				chunks := splitBySize(sentence, maxSize)
				for _, chunk := range chunks {
					result = append(result, chunk)
				}
				continue
			}

			// 如果添加当前句子会超过最大大小，先保存当前部分
			if current.Len()+len(sentence) > maxSize && current.Len() >= minSize {
				result = append(result, current.String())
				current.Reset()
			}

			// 添加句子
			current.WriteString(sentence)
		}

		// 添加最后一部分
		if current.Len() > 0 {
			result = append(result, current.String())
		}
	}

	// 合并太小的段落
	if minSize > 0 {
		result = mergeSmallParagraphs(result, minSize)
	}

	return result
}

// splitIntoSentences 将文本分割成句子
func splitIntoSentences(text string) []string {
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

// splitBySize 按大小分割文本
func splitBySize(text string, maxSize int) []string {
	var result []string

	for len(text) > 0 {
		if len(text) <= maxSize {
			result = append(result, text)
			break
		}

		// 尝试在空格处分割
		splitIndex := maxSize
		for i := maxSize; i >= maxSize/2; i-- {
			if i < len(text) && (text[i] == ' ' || text[i] == '\n') {
				splitIndex = i + 1
				break
			}
		}

		result = append(result, text[:splitIndex])
		text = text[splitIndex:]
	}

	return result
}

// mergeSmallParagraphs 合并太小的段落
func mergeSmallParagraphs(paragraphs []string, minSize int) []string {
	if len(paragraphs) <= 1 {
		return paragraphs
	}

	var result []string
	var current string

	for i, paragraph := range paragraphs {
		// 跳过空段落
		if strings.TrimSpace(paragraph) == "" {
			if current != "" {
				result = append(result, current)
				current = ""
			}
			result = append(result, paragraph)
			continue
		}

		// 如果是第一个段落或当前段落为空，直接设置
		if i == 0 || current == "" {
			current = paragraph
			continue
		}

		// 如果当前段落太小，合并
		if len(paragraph) < minSize {
			current += "\n\n" + paragraph
		} else {
			// 否则保存当前段落并开始新段落
			result = append(result, current)
			current = paragraph
		}
	}

	// 添加最后一个段落
	if current != "" {
		result = append(result, current)
	}

	return result
}
