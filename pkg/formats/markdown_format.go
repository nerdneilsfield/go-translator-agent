package formats

import (
	"fmt"
	"os"
	"strings"

	"github.com/Kunde21/markdownfmt/v3"
	"github.com/Kunde21/markdownfmt/v3/markdown"
	"go.uber.org/zap"
)

func FormatMarkdown(path string, logger *zap.Logger) error {
	if !strings.HasSuffix(path, ".md") {
		return fmt.Errorf("文件不是Markdown文件")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		logger.Error("读取文件失败", zap.String("path", path), zap.Error(err))
		return err
	}

	opts := []markdown.Option{
		// markdown.WithListIndentStyle(markdown.ListIndentAligned),
		markdown.WithCodeFormatters(markdown.GoCodeFormatter),
	}

	logger.Info("开始格式化 Markdown 文件", zap.String("path", path), zap.Int("size", len(content)))

	res, err := markdownfmt.Process(path, content, opts...)
	if err != nil {
		logger.Error("格式化 Markdown 文件失败", zap.String("path", path), zap.Error(err))
		return err
	}

	logger.Info("格式化 Markdown 文件完成", zap.String("path", path), zap.Int("size", len(res)))

	return os.WriteFile(path, []byte(res), 0o644)
}
