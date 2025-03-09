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

	// 创建临时结果变量
	var partialResult string
	var lastSaveTime time.Time = time.Now()

	var progress *TranslationProgressTracker

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
			// 自动保存和进度更新
			if progress != nil {
				totalChars, translatedChars, estimatedTimeRemaining := progress.GetProgress()
				log.Info("翻译进度",
					zap.Int("总字数", totalChars),
					zap.Int("已翻译字数", translatedChars),
					zap.Float64("完成百分比", float64(translatedChars)/float64(totalChars)*100),
					zap.Float64("预计剩余时间(秒)", estimatedTimeRemaining),
				)
			}

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

	// 按行分割文本，支持不同的换行符
	paragraphs := splitTextByLines(text, minSplitSize, maxSplitSize)

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

	// 初始化进度跟踪
	var totalChars int
	for _, idx := range translatableParagraphs {
		totalChars += len(paragraphs[idx])
	}
	progress := NewProgressTracker(totalChars)

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

			// 更新进度
			progress.UpdateProgress(len(paragraph))

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

	// 组合翻译后的段落，保持原始换行符
	translatedText := strings.Join(paragraphs, "\n")

	// 获取最终进度信息
	totalChars, translatedChars, _ := progress.GetProgress()

	log.Info("文本翻译完成",
		zap.Int("原始总字数", totalChars),
		zap.Int("已翻译字数", translatedChars),
		zap.Float64("翻译速度(字/秒)", float64(translatedChars)/time.Since(progress.startTime).Seconds()),
	)

	return translatedText, nil
}

// splitTextByLines 按行分割文本，支持不同的换行符
func splitTextByLines(text string, minSize, maxSize int) []string {
	// 统一换行符为 \n
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	// 按换行符分割
	lines := strings.Split(text, "\n")

	var paragraphs []string
	var currentParagraph strings.Builder

	for _, line := range lines {
		// 如果是空行，表示段落结束
		if strings.TrimSpace(line) == "" {
			if currentParagraph.Len() > 0 {
				paragraphs = append(paragraphs, currentParagraph.String())
				currentParagraph.Reset()
			}
			// 保留空行
			paragraphs = append(paragraphs, "")
			continue
		}

		// 如果当前段落加上新行会超过最大大小，且当前段落已达到最小大小
		if currentParagraph.Len()+len(line)+1 > maxSize && currentParagraph.Len() >= minSize {
			paragraphs = append(paragraphs, currentParagraph.String())
			currentParagraph.Reset()
		}

		// 添加新行到当前段落
		if currentParagraph.Len() > 0 {
			currentParagraph.WriteString("\n")
		}
		currentParagraph.WriteString(line)
	}

	// 添加最后一个段落
	if currentParagraph.Len() > 0 {
		paragraphs = append(paragraphs, currentParagraph.String())
	}

	// 合并太小的段落
	if minSize > 0 {
		paragraphs = mergeSmallParagraphs(paragraphs, minSize)
	}

	return paragraphs
}

// mergeSmallParagraphs 合并太小的段落
func mergeSmallParagraphs(paragraphs []string, minSize int) []string {
	if len(paragraphs) <= 1 {
		return paragraphs
	}

	var result []string
	var current string

	for i, paragraph := range paragraphs {
		// 保留空段落
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
			current += "\n" + paragraph
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

// FormatFile 格式化文本文件
func (p *TextProcessor) FormatFile(inputPath, outputPath string) error {
	// 读取输入文件
	content, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("读取文件失败 %s: %w", inputPath, err)
	}

	log := p.Translator.GetLogger()
	log.Info("开始格式化文件",
		zap.String("输入文件", inputPath),
		zap.String("输出文件", outputPath),
	)

	// 格式化文本（简单的行处理）
	lines := strings.Split(string(content), "\n")
	var formattedLines []string
	var currentParagraph []string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			// 空行表示段落结束
			if len(currentParagraph) > 0 {
				formattedLines = append(formattedLines, strings.Join(currentParagraph, " "))
				formattedLines = append(formattedLines, "")
				currentParagraph = nil
			}
		} else {
			currentParagraph = append(currentParagraph, trimmedLine)
		}
	}

	// 处理最后一个段落
	if len(currentParagraph) > 0 {
		formattedLines = append(formattedLines, strings.Join(currentParagraph, " "))
	}

	// 写入输出文件
	formattedText := strings.Join(formattedLines, "\n")
	if err := os.WriteFile(outputPath, []byte(formattedText), 0644); err != nil {
		return fmt.Errorf("写入文件失败 %s: %w", outputPath, err)
	}

	log.Info("格式化完成",
		zap.String("输出文件", outputPath),
	)

	return nil
}

// TextFormattingProcessor 是文本格式化处理器
type TextFormattingProcessor struct {
	logger *zap.Logger
}

// NewTextFormattingProcessor 创建一个新的文本格式化处理器
func NewTextFormattingProcessor() (*TextFormattingProcessor, error) {
	zapLogger, _ := zap.NewProduction()
	return &TextFormattingProcessor{
		logger: zapLogger,
	}, nil
}

// FormatFile 格式化文本文件
func (p *TextFormattingProcessor) FormatFile(inputPath, outputPath string) error {
	// 读取输入文件
	content, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("读取文件失败 %s: %w", inputPath, err)
	}

	// 格式化文本（简单的行处理）
	lines := strings.Split(string(content), "\n")
	var formattedLines []string
	var currentParagraph []string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			// 空行表示段落结束
			if len(currentParagraph) > 0 {
				formattedLines = append(formattedLines, strings.Join(currentParagraph, " "))
				formattedLines = append(formattedLines, "")
				currentParagraph = nil
			}
		} else {
			currentParagraph = append(currentParagraph, trimmedLine)
		}
	}

	// 处理最后一个段落
	if len(currentParagraph) > 0 {
		formattedLines = append(formattedLines, strings.Join(currentParagraph, " "))
	}

	// 写入输出文件
	formattedText := strings.Join(formattedLines, "\n")
	if err := os.WriteFile(outputPath, []byte(formattedText), 0644); err != nil {
		return fmt.Errorf("写入文件失败 %s: %w", outputPath, err)
	}

	return nil
}
