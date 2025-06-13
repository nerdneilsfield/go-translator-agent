package document

import (
	"time"
)

// Format 文档格式类型
type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatText     Format = "text"
	FormatHTML     Format = "html"
	FormatEPUB     Format = "epub"
	FormatLaTeX    Format = "latex"
	FormatPDF      Format = "pdf"
	FormatDOCX     Format = "docx"
	FormatUnknown  Format = "unknown"
)

// Document 表示一个文档
type Document struct {
	// ID 文档唯一标识
	ID string
	
	// Format 文档格式
	Format Format
	
	// Metadata 文档元数据
	Metadata DocumentMetadata
	
	// Blocks 文档块列表
	Blocks []Block
	
	// Resources 文档资源（图片、样式等）
	Resources map[string]Resource
}

// DocumentMetadata 文档元数据
type DocumentMetadata struct {
	Title        string
	Author       string
	Language     string
	CreatedAt    time.Time
	ModifiedAt   time.Time
	Tags         []string
	CustomFields map[string]interface{}
}

// Block 文档块接口
type Block interface {
	// GetType 获取块类型
	GetType() BlockType
	
	// GetContent 获取块内容
	GetContent() string
	
	// SetContent 设置块内容
	SetContent(content string)
	
	// IsTranslatable 是否可翻译
	IsTranslatable() bool
	
	// GetMetadata 获取块元数据
	GetMetadata() BlockMetadata
}

// BlockType 块类型
type BlockType string

const (
	BlockTypeParagraph BlockType = "paragraph"
	BlockTypeHeading   BlockType = "heading"
	BlockTypeCode      BlockType = "code"
	BlockTypeList      BlockType = "list"
	BlockTypeTable     BlockType = "table"
	BlockTypeImage     BlockType = "image"
	BlockTypeQuote     BlockType = "quote"
	BlockTypeMath      BlockType = "math"
	BlockTypeHTML      BlockType = "html"
	BlockTypeCustom    BlockType = "custom"
)

// BlockMetadata 块元数据
type BlockMetadata struct {
	Level      int                    // 标题级别（仅用于标题块）
	Language   string                 // 代码语言（仅用于代码块）
	ListType   string                 // 列表类型（仅用于列表块）
	Attributes map[string]interface{} // 其他属性
}

// BaseBlock 基础块实现
type BaseBlock struct {
	Type        BlockType
	Content     string
	Translatable bool
	Metadata    BlockMetadata
}

func (b *BaseBlock) GetType() BlockType {
	return b.Type
}

func (b *BaseBlock) GetContent() string {
	return b.Content
}

func (b *BaseBlock) SetContent(content string) {
	b.Content = content
}

func (b *BaseBlock) IsTranslatable() bool {
	return b.Translatable
}

func (b *BaseBlock) GetMetadata() BlockMetadata {
	return b.Metadata
}

// Resource 文档资源
type Resource struct {
	ID          string
	Type        ResourceType
	ContentType string
	Data        []byte
	URL         string
}

// ResourceType 资源类型
type ResourceType string

const (
	ResourceTypeImage      ResourceType = "image"
	ResourceTypeStylesheet ResourceType = "stylesheet"
	ResourceTypeFont       ResourceType = "font"
	ResourceTypeOther      ResourceType = "other"
)

// Chunk 文本分块
type Chunk struct {
	ID       string
	Content  string
	Start    int
	End      int
	Metadata map[string]interface{}
}

// TranslatedChunk 翻译后的分块
type TranslatedChunk struct {
	Chunk
	TranslatedContent string
	Error            error
}

// ChunkOptions 分块选项
type ChunkOptions struct {
	MaxSize      int
	Overlap      int
	PreserveTags []string
	Delimiter    string
}

// ProcessingResult 处理结果
type ProcessingResult struct {
	Document   *Document
	Statistics ProcessingStatistics
	Errors     []ProcessingError
}

// ProcessingStatistics 处理统计
type ProcessingStatistics struct {
	TotalBlocks      int
	TranslatedBlocks int
	SkippedBlocks    int
	TotalChunks      int
	TotalCharacters  int
	ProcessingTime   time.Duration
}

// ProcessingError 处理错误
type ProcessingError struct {
	BlockID string
	Error   error
	Context map[string]interface{}
}