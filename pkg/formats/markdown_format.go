package formats

import (
	"fmt"
	"os"
	"strings"

	"github.com/Kunde21/markdownfmt/v3"
	"github.com/Kunde21/markdownfmt/v3/markdown"
)

func FormatMarkdown(path string) error {
	if !strings.HasSuffix(path, ".md") {
		return fmt.Errorf("文件不是Markdown文件")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("读取文件失败", err)
		return err
	}

	opts := []markdown.Option{
		// markdown.WithListIndentStyle(markdown.ListIndentAligned),
		markdown.WithCodeFormatters(markdown.GoCodeFormatter),
	}

	fmt.Printf("开始格式化 %s， 大小: %d\n", path, len(content))

	res, err := markdownfmt.Process(path, content, opts...)
	if err != nil {
		fmt.Println("格式化失败", err)
		return err
	}

	fmt.Printf("格式化完成 %s， 大小: %d\n", path, len(res))

	return os.WriteFile(path, []byte(res), 0644)
}
