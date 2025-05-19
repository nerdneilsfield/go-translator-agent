package formats

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
	"golang.org/x/net/html"

	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
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
				p.logger.Debug("需要翻译", zap.Int("idx", idx))
				translatedChunk, err := p.TranslateText(textChunk.Text)
				if err != nil {
					mu.Lock()
					translateErr = err
					mu.Unlock()
					p.logger.Warn("翻译出错", zap.Error(err), zap.Int("idx", idx))
					results[idx] = textChunk.Text // 如果翻译出错，则返回原始内容
				} else {
					p.logger.Debug("翻译成功", zap.Int("idx", idx))
					results[idx] = translatedChunk
				}
			} else {
				p.logger.Debug("不需要翻译", zap.Int("idx", idx))
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

func IsFileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// TranslateHTMLDOM 翻译 HTML 字符串中的文本节点，保持原有的 DOM 结构
// TranslateHTMLDOM 翻译 HTML 字符串中的文本节点，保持原有的 DOM 结构
func TranslateHTMLDOM(htmlStr string, t translator.Translator, logger *zap.Logger) (string, error) {
	root, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return "", err
	}

	cfg := t.GetConfig()
	chunkSize := 6000
	if cfg != nil {
		if modelCfg, ok := cfg.ModelConfigs[cfg.DefaultModelName]; ok {
			if modelCfg.MaxInputTokens > 0 {
				chunkSize = modelCfg.MaxInputTokens - 2000
				if chunkSize <= 0 {
					chunkSize = modelCfg.MaxInputTokens
				}
			}
		}
	}

	var textNodes []*html.Node
	collectTextNodes(root, &textNodes)
	groups := groupTextNodes(textNodes, chunkSize)

	for _, group := range groups {
		var builder strings.Builder
		for i, n := range group {
			if i > 0 {
				builder.WriteString("\n\n")
			}
			builder.WriteString(strings.TrimSpace(n.Data))
		}

		translated, err := t.Translate(builder.String(), true)
		if err != nil {
			logger.Warn("translate html node group failed", zap.Error(err))
			continue
		}

		parts := strings.SplitN(translated, "\n\n", len(group))
		for i, n := range group {
			translatedText := strings.TrimSpace(n.Data)
			if i < len(parts) {
				translatedText = strings.TrimSpace(parts[i])
			}
			leading := leadingWhitespace(n.Data)
			trailing := trailingWhitespace(n.Data)
			n.Data = leading + translatedText + trailing
		}
	}

	var buf bytes.Buffer
	if err := html.Render(&buf, root); err != nil {
		return "", err
	}
	doc, err := goquery.NewDocumentFromReader(&buf)
	if err != nil {
		return "", err
	}
	htmlResult, err := doc.Html()
	if err != nil {
		return "", err
	}
	return htmlResult, nil
}

func translateHTMLNode(n *html.Node, t translator.Translator, logger *zap.Logger) {
	if n.Type == html.ElementNode {
		if n.Data == "script" || n.Data == "style" {
			return
		}
	}

	if n.Type == html.TextNode {
		if strings.TrimSpace(n.Data) != "" {
			leading := leadingWhitespace(n.Data)
			trailing := trailingWhitespace(n.Data)
			core := strings.TrimSpace(n.Data)
			translated, err := t.Translate(core, true)
			if err != nil {
				logger.Warn("translate html node failed", zap.Error(err))
			} else {
				n.Data = leading + translated + trailing
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		translateHTMLNode(c, t, logger)
	}
}

func leadingWhitespace(s string) string {
	trimmed := strings.TrimLeftFunc(s, unicode.IsSpace)
	return s[:len(s)-len(trimmed)]
}

func trailingWhitespace(s string) string {
	trimmed := strings.TrimRightFunc(s, unicode.IsSpace)
	return s[len(trimmed):]
}

func collectTextNodes(n *html.Node, nodes *[]*html.Node) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "style", "pre", "code":
			return
		}
	}

	if n.Type == html.TextNode {
		if strings.TrimSpace(n.Data) != "" {
			*nodes = append(*nodes, n)
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectTextNodes(c, nodes)
	}
}

func groupTextNodes(nodes []*html.Node, limit int) [][]*html.Node {
	var groups [][]*html.Node
	var current []*html.Node
	currentLen := 0
	for _, n := range nodes {
		text := strings.TrimSpace(n.Data)
		if currentLen+len(text) > limit && len(current) > 0 {
			groups = append(groups, current)
			current = nil
			currentLen = 0
		}
		current = append(current, n)
		currentLen += len(text)
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}
