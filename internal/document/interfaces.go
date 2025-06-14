// Package document 定义了新的文档处理器接口
// 这是重构后的版本，用于替代旧的 formats 包
package document

import (
	"context"
	"io"
)

// Processor 定义文档处理器的核心接口
// 负责解析、分块、重组文档
type Processor interface {
	// Parse 解析输入流为文档结构
	Parse(ctx context.Context, input io.Reader) (*Document, error)

	// Process 处理文档（分块、翻译、重组）
	Process(ctx context.Context, doc *Document, translator TranslateFunc) (*Document, error)

	// Render 将文档渲染为输出格式
	Render(ctx context.Context, doc *Document, output io.Writer) error

	// GetFormat 返回处理器支持的格式
	GetFormat() Format
}

// Parser 文档解析器接口
type Parser interface {
	// Parse 解析输入流为文档结构
	Parse(ctx context.Context, input io.Reader) (*Document, error)

	// CanParse 检查是否能解析该格式
	CanParse(format Format) bool
}

// Renderer 文档渲染器接口
type Renderer interface {
	// Render 将文档渲染为输出格式
	Render(ctx context.Context, doc *Document, output io.Writer) error

	// CanRender 检查是否能渲染该格式
	CanRender(format Format) bool
}

// Chunker 文本分块器接口
type Chunker interface {
	// Chunk 将内容分块
	Chunk(content string, opts ChunkOptions) []Chunk

	// Merge 合并分块结果
	Merge(chunks []TranslatedChunk) string
}

// TranslateFunc 翻译函数类型
type TranslateFunc func(ctx context.Context, text string) (string, error)

// ProcessorFactory 处理器工厂函数
type ProcessorFactory func(opts ProcessorOptions) (Processor, error)

// ProcessorOptions 处理器选项
type ProcessorOptions struct {
	// PreserveTags 需要保护的标签
	PreserveTags []string

	// ChunkSize 分块大小
	ChunkSize int

	// ChunkOverlap 分块重叠
	ChunkOverlap int

	// CustomPatterns 自定义保护模式
	CustomPatterns []Pattern

	// Metadata 元数据
	Metadata map[string]interface{}
}

// Pattern 自定义模式
type Pattern struct {
	Name    string
	Regex   string
	Replace string
}
