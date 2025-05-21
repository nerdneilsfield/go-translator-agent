package formats

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"go.uber.org/zap"
)

// EPUBProcessor 是EPUB电子书的处理器
type EPUBProcessor struct {
	BaseProcessor
}

// NewEPUBProcessor 创建一个新的EPUB处理器
func NewEPUBProcessor(t translator.Translator, predefinedTranslations *config.PredefinedTranslation, progressBar *progress.Writer) (*EPUBProcessor, error) {
	// 获取logger，如果无法转换则创建新的
	zapLogger, _ := zap.NewProduction()
	if loggerProvider, ok := t.GetLogger().(interface{ GetZapLogger() *zap.Logger }); ok {
		if zl := loggerProvider.GetZapLogger(); zl != nil {
			zapLogger = zl
		}
	}

	p := &EPUBProcessor{
		BaseProcessor: BaseProcessor{
			Translator:             t,
			Name:                   "EPUB",
			predefinedTranslations: predefinedTranslations,
			progressBar:            progressBar,
			logger:                 zapLogger,
		},
	}
	p.logger.Debug("Loading predefined translations", zap.Int("count", len(predefinedTranslations.Translations)))
	return p, nil
}

// TranslateFile 翻译EPUB文件
func (p *EPUBProcessor) TranslateFile(inputPath, outputPath string) error {
	var totalChars int
	var htmlFiles []string

	// 计算总字符数并收集HTML文件
	err := filepath.Walk(inputPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".html" || ext == ".xhtml" || ext == ".htm" {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			totalChars += len(data)
			htmlFiles = append(htmlFiles, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	p.logger.Info("EPUB文件分析完成",
		zap.Int("HTML文件数", len(htmlFiles)),
		zap.Int("总字符数", totalChars))

	p.Translator.GetProgressTracker().SetRealTotalChars(totalChars)
	p.Translator.GetProgressTracker().SetTotalChars(totalChars)

	// 初始化翻译器并开始进度跟踪
	p.Translator.InitTranslator()
	defer p.Translator.Finish()

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "epub_work")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// 解压 EPUB
	if err := unzipEPUB(inputPath, tempDir); err != nil {
		return fmt.Errorf("解压EPUB失败: %w", err)
	}

	p.logger.Info("EPUB文件已解压到临时目录", zap.String("临时目录", tempDir))

	// 收集所有需要翻译的HTML文件
	var filesToTranslate []string
	err = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".html" || ext == ".xhtml" || ext == ".htm" {
			filesToTranslate = append(filesToTranslate, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	p.logger.Debug("找到需要翻译的HTML文件", zap.Int("文件数", len(filesToTranslate)))

	// 遍历 HTML/XHTML 文件并翻译
	for i, filePath := range filesToTranslate {
		p.logger.Debug("开始翻译HTML文件",
			zap.Int("当前文件", i+1),
			zap.Int("总文件数", len(filesToTranslate)),
			zap.String("文件路径", filePath))

		// 读取文件内容
		data, err := os.ReadFile(filePath)
		if err != nil {
			p.logger.Error("读取HTML文件失败", zap.String("文件", filePath), zap.Error(err))
			return fmt.Errorf("读取HTML文件失败 %s: %w", filePath, err)
		}

		// 翻译文件内容
		originalContent := string(data)
		translated, err := p.TranslateText(originalContent)
		if err != nil {
			p.logger.Error("翻译HTML文件失败", zap.String("文件", filePath), zap.Error(err))
			return fmt.Errorf("翻译HTML文件失败 %s: %w", filePath, err)
		}

		// 记录翻译结果摘要
		p.logger.Debug("HTML文件翻译结果",
			zap.String("文件", filePath),
			zap.String("原文摘要", Snippet(originalContent)),
			zap.String("译文摘要", Snippet(translated)),
			zap.Int("原文长度", len(originalContent)),
			zap.Int("译文长度", len(translated)))

		// 写入翻译后的内容
		if err := os.WriteFile(filePath, []byte(translated), 0644); err != nil {
			p.logger.Error("写入翻译后的HTML文件失败", zap.String("文件", filePath), zap.Error(err))
			return fmt.Errorf("写入翻译后的HTML文件失败 %s: %w", filePath, err)
		}

		// 格式化HTML文件
		if err := FormatFile(filePath, p.logger); err != nil {
			p.logger.Warn("格式化HTML文件失败", zap.String("文件", filePath), zap.Error(err))
		}

		p.logger.Debug("HTML文件翻译完成",
			zap.Int("当前文件", i+1),
			zap.Int("总文件数", len(filesToTranslate)),
			zap.String("文件路径", filePath))
	}

	p.logger.Info("所有HTML文件翻译完成，开始重新打包EPUB")

	// 重新打包为EPUB
	if err := zipDir(tempDir, outputPath); err != nil {
		return fmt.Errorf("重新打包EPUB失败: %w", err)
	}

	p.logger.Info("EPUB文件重新打包完成", zap.String("输出文件", outputPath))

	return nil
}

// TranslateText 翻译EPUB内容
func (p *EPUBProcessor) TranslateText(text string) (string, error) {
	translated, err := TranslateHTMLDOM(text, p.Translator, p.logger)
	if err != nil {
		return "", err
	}
	return translated, nil
}

// FormatFile 格式化EPUB文件
func (p *EPUBProcessor) FormatFile(inputPath, outputPath string) error {
	// 暂时直接复制文件并使用 Prettier 格式化 HTML 文件
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return err
	}
	if err := FormatFile(outputPath, p.logger); err != nil {
		p.logger.Warn("格式化EPUB文件失败", zap.Error(err))
	}
	return nil
}

// EPUBFormattingProcessor 是 EPUB 格式化处理器
type EPUBFormattingProcessor struct {
	logger *zap.Logger
}

// NewEPUBFormattingProcessor 创建一个新的 EPUB 格式化处理器
func NewEPUBFormattingProcessor() (*EPUBFormattingProcessor, error) {
	zapLogger, _ := zap.NewProduction()
	return &EPUBFormattingProcessor{
		logger: zapLogger,
	}, nil
}

// FormatFile 格式化 EPUB 文件
func (p *EPUBFormattingProcessor) FormatFile(_ string, _ string) error {
	return fmt.Errorf("EPUB格式暂不支持格式化功能")
}

// unzipEPUB 解压 EPUB 文件到指定目录
func unzipEPUB(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, f.Mode()); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			return err
		}
		outFile.Close()
	}
	return nil
}

// zipDir 将目录压缩为 EPUB 文件
func zipDir(dir, dest string) error {
	outFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer outFile.Close()

	w := zip.NewWriter(outFile)
	defer w.Close()

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		// 将路径分隔符转换为ZIP规范的'/'
		zipPath := filepath.ToSlash(relPath)

		if info.IsDir() {
			if zipPath == "." { // 使用转换后的 zipPath
				return nil
			}
			_, err := w.Create(zipPath + "/") // 使用转换后的 zipPath
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		f, err := w.Create(zipPath) // 使用转换后的 zipPath
		if err != nil {
			return err
		}
		_, err = io.Copy(f, file)
		return err
	})
}
