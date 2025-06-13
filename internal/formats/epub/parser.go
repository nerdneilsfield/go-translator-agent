package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/pkg/document"
)

// Parser EPUB 解析器
type Parser struct {
	htmlProcessor document.Processor
}

// NewParser 创建 EPUB 解析器
func NewParser(htmlProcessor document.Processor) *Parser {
	return &Parser{
		htmlProcessor: htmlProcessor,
	}
}

// Parse 解析 EPUB 文档
func (p *Parser) Parse(ctx context.Context, input io.Reader) (*document.Document, error) {
	// 读取整个 EPUB 文件到内存
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("failed to read EPUB file: %w", err)
	}

	// 创建 ZIP reader
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to open EPUB as ZIP: %w", err)
	}

	// 创建文档
	doc := &document.Document{
		ID:        generateDocumentID(),
		Format:    document.FormatEPUB,
		Metadata:  document.DocumentMetadata{},
		Blocks:    []document.Block{},
		Resources: make(map[string]document.Resource),
	}

	// 存储 EPUB 文件结构
	epubFiles := make(map[string][]byte)
	doc.Metadata.CustomFields = map[string]interface{}{
		"epub_files": epubFiles,
	}

	// 提取所有文件
	for _, file := range zipReader.File {
		fileReader, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open file %s in EPUB: %w", file.Name, err)
		}
		defer fileReader.Close()

		fileData, err := io.ReadAll(fileReader)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s in EPUB: %w", file.Name, err)
		}

		// 存储文件内容
		epubFiles[file.Name] = fileData

		// 解析 HTML/XHTML 文件以提取可翻译内容
		ext := strings.ToLower(path.Ext(file.Name))
		if ext == ".html" || ext == ".xhtml" || ext == ".htm" {
			// 解析 HTML 内容
			reader := bytes.NewReader(fileData)
			htmlDoc, err := p.htmlProcessor.Parse(ctx, reader)
			if err != nil {
				// 记录错误但继续处理其他文件
				continue
			}

			// 将 HTML 文档的块添加到 EPUB 文档中
			for _, block := range htmlDoc.Blocks {
				// 为每个块添加文件路径信息
				metadata := block.GetMetadata()
				if metadata.Attributes == nil {
					metadata.Attributes = make(map[string]interface{})
				}
				metadata.Attributes["epub_file"] = file.Name
				
				doc.Blocks = append(doc.Blocks, block)
			}
		} else if ext == ".opf" {
			// 解析 OPF 文件以获取元数据
			p.parseOPF(fileData, doc)
		} else if ext == ".css" || ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".svg" {
			// 存储为资源
			doc.Resources[file.Name] = document.Resource{
				ID:          file.Name,
				Type:        getResourceType(ext),
				Data:        fileData,
				ContentType: getMIMEType(ext),
			}
		}
	}

	return doc, nil
}

// CanParse 检查是否能解析该格式
func (p *Parser) CanParse(format document.Format) bool {
	return format == document.FormatEPUB
}

// parseOPF 解析 OPF 文件以获取元数据
func (p *Parser) parseOPF(data []byte, doc *document.Document) {
	content := string(data)
	
	// 简单的元数据提取（可以使用 XML 解析器来改进）
	if titleStart := strings.Index(content, "<dc:title>"); titleStart != -1 {
		if titleEnd := strings.Index(content[titleStart:], "</dc:title>"); titleEnd != -1 {
			doc.Metadata.Title = content[titleStart+10 : titleStart+titleEnd]
		}
	}
	
	if authorStart := strings.Index(content, "<dc:creator>"); authorStart != -1 {
		if authorEnd := strings.Index(content[authorStart:], "</dc:creator>"); authorEnd != -1 {
			doc.Metadata.Author = content[authorStart+12 : authorStart+authorEnd]
		}
	}
	
	if langStart := strings.Index(content, "<dc:language>"); langStart != -1 {
		if langEnd := strings.Index(content[langStart:], "</dc:language>"); langEnd != -1 {
			doc.Metadata.Language = content[langStart+13 : langStart+langEnd]
		}
	}
}

// getResourceType 根据文件扩展名获取资源类型
func getResourceType(ext string) document.ResourceType {
	switch ext {
	case ".css":
		return document.ResourceTypeStylesheet
	case ".jpg", ".jpeg", ".png", ".gif", ".svg":
		return document.ResourceTypeImage
	case ".ttf", ".otf", ".woff", ".woff2":
		return document.ResourceTypeFont
	default:
		return document.ResourceTypeOther
	}
}

// getMIMEType 根据文件扩展名获取 MIME 类型
func getMIMEType(ext string) string {
	mimeTypes := map[string]string{
		".html":  "text/html",
		".xhtml": "application/xhtml+xml",
		".css":   "text/css",
		".js":    "application/javascript",
		".jpg":   "image/jpeg",
		".jpeg":  "image/jpeg",
		".png":   "image/png",
		".gif":   "image/gif",
		".svg":   "image/svg+xml",
	}
	
	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}