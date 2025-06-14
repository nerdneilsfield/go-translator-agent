package epub

import (
	"archive/zip"
	"context"
	"fmt"
	"io"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
)

// Renderer EPUB 渲染器
type Renderer struct {
	preserveStructure bool
}

// NewRenderer 创建 EPUB 渲染器
func NewRenderer() *Renderer {
	return &Renderer{
		preserveStructure: true,
	}
}

// Render 渲染文档为 EPUB
func (r *Renderer) Render(ctx context.Context, doc *document.Document, output io.Writer) error {
	if doc == nil {
		return fmt.Errorf("document is nil")
	}

	// 获取 EPUB 文件结构
	epubFiles, ok := doc.Metadata.CustomFields["epub_files"].(map[string][]byte)
	if !ok {
		return fmt.Errorf("no EPUB files found in document metadata")
	}

	// 创建 ZIP writer
	zipWriter := zip.NewWriter(output)
	defer zipWriter.Close()

	// 写入所有文件
	for filePath, content := range epubFiles {
		writer, err := zipWriter.Create(filePath)
		if err != nil {
			return fmt.Errorf("failed to create file %s in EPUB: %w", filePath, err)
		}

		if _, err := writer.Write(content); err != nil {
			return fmt.Errorf("failed to write file %s in EPUB: %w", filePath, err)
		}
	}

	// 写入资源文件
	for filePath, resource := range doc.Resources {
		writer, err := zipWriter.Create(filePath)
		if err != nil {
			return fmt.Errorf("failed to create resource %s in EPUB: %w", filePath, err)
		}

		if _, err := writer.Write(resource.Data); err != nil {
			return fmt.Errorf("failed to write resource %s in EPUB: %w", filePath, err)
		}
	}

	return nil
}

// CanRender 检查是否能渲染该格式
func (r *Renderer) CanRender(format document.Format) bool {
	return format == document.FormatEPUB
}
