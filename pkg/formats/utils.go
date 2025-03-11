package formats

import (
	"strings"
	"sync"

	"go.uber.org/zap"
)

type Chunk struct {
	Text            string
	NeedToTranslate bool
}

// NodeToTranslate 表示要翻译的节点
type NodeToTranslate struct {
	ID      int    // 唯一标识符
	Content string // 原始内容
}

// 如果有多余的 \n\n\n 换行符替换成 \n\n 递归进行
func RemoveRedundantNewlines(text string) string {
	for {
		if strings.Contains(text, "\n\n\n") {
			text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
		} else {
			break
		}
	}
	return text
}

func parallelTranslateChunks(chunks []Chunk, p *MarkdownProcessor, concurrency int) ([]string, error) {
	// concurrency 表示最大并发量
	results := make([]string, len(chunks))

	var wg sync.WaitGroup
	var translateErr error
	var mu sync.Mutex // 用来保护 translateErr

	// 建一个带缓冲的信号量通道，容量为 concurrency
	sem := make(chan struct{}, concurrency)

	for i, chunk := range chunks {
		wg.Add(1)

		go func(idx int, textChunk Chunk) {
			defer wg.Done()

			// 向信号量通道发送一个空结构体，若通道满则会阻塞
			sem <- struct{}{}

			// 翻译
			if textChunk.NeedToTranslate {
				p.logger.Debug("需要翻译", zap.String("text", textChunk.Text))
				translatedChunk, err := p.TranslateText(textChunk.Text)
				if err != nil {
					mu.Lock()
					translateErr = err
					mu.Unlock()
					p.logger.Warn("翻译出错", zap.Error(err), zap.String("text", textChunk.Text))
					results[idx] = textChunk.Text // 如果翻译出错，则返回原始内容
				} else {
					p.logger.Debug("翻译成功", zap.String("text", translatedChunk))
					results[idx] = translatedChunk
				}
			} else {
				p.logger.Debug("不需要翻译", zap.String("text", textChunk.Text))
				results[idx] = textChunk.Text
			}

			// 释放一个信号量
			<-sem
		}(i, chunk)
	}

	wg.Wait()

	if translateErr != nil {
		return nil, translateErr
	}
	return results, nil
}
