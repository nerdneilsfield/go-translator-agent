package document

import (
	"strings"
)

// GetProtectorForFormat 根据格式获取相应的保护器
func GetProtectorForFormat(format string) ContentProtector {
	switch format {
	case "markdown":
		return NewMarkdownProtector()
	case "html":
		return NewHTMLProtector()
	case "text":
		return NewTextProtector()
	case "epub":
		// EPUB基本上是HTML，可以复用HTML保护器
		return NewHTMLProtector()
	default:
		return NewDefaultProtector()
	}
}

// GetProtectorForExtension 根据文件扩展名获取保护器
func GetProtectorForExtension(filename string) ContentProtector {
	filename = strings.ToLower(filename)
	
	if strings.HasSuffix(filename, ".md") || strings.HasSuffix(filename, ".markdown") {
		return NewMarkdownProtector()
	}
	if strings.HasSuffix(filename, ".html") || strings.HasSuffix(filename, ".htm") {
		return NewHTMLProtector()
	}
	if strings.HasSuffix(filename, ".txt") {
		return NewTextProtector()
	}
	if strings.HasSuffix(filename, ".epub") {
		return NewHTMLProtector() // EPUB复用HTML保护器
	}
	
	return NewDefaultProtector()
}

// ListAllProtectors 列出所有可用的保护器
func ListAllProtectors() map[string]ContentProtector {
	return map[string]ContentProtector{
		"markdown": NewMarkdownProtector(),
		"html":     NewHTMLProtector(),
		"text":     NewTextProtector(),
		"epub":     NewHTMLProtector(),
		"default":  NewDefaultProtector(),
	}
}