package formatter

import (
	"io"
	"time"
)

// Formatter 格式化器接口
type Formatter interface {
	// Format 格式化内容
	Format(content []byte, opts FormatOptions) ([]byte, error)

	// CanFormat 检查是否支持该格式
	CanFormat(format string) bool

	// Priority 返回格式化器优先级（数字越大优先级越高）
	Priority() int

	// Name 返回格式化器名称
	Name() string

	// GetMetadata 返回格式化器元数据
	GetMetadata() FormatterMetadata
}

// FileFormatter 文件格式化器接口
type FileFormatter interface {
	Formatter

	// FormatFile 格式化文件
	FormatFile(inputPath, outputPath string, opts FormatOptions) error
}

// StreamFormatter 流式格式化器接口
type StreamFormatter interface {
	Formatter

	// FormatStream 流式格式化
	FormatStream(reader io.Reader, writer io.Writer, opts FormatOptions) error
}

// FormatOptions 格式化选项
type FormatOptions struct {
	// 基础选项
	PreserveWhitespace bool   // 保留空白字符
	MaxLineLength      int    // 最大行长度
	TabSize            int    // Tab 转换为空格数
	LineEnding         string // 行结束符（\n, \r\n）
	Encoding           string // 字符编码

	// 外部工具选项
	UseExternalTool  bool     // 是否使用外部工具
	ExternalToolPath string   // 外部工具路径
	ExternalToolArgs []string // 外部工具参数

	// 高级选项
	PreserveBlocks []PreserveBlock        // 需要保护的块
	CustomRules    map[string]string      // 自定义规则
	Metadata       map[string]interface{} // 元数据
}

// PreserveBlock 保护块定义
type PreserveBlock struct {
	Type    string // 类型（code, latex, custom）
	Pattern string // 正则表达式模式
	Marker  string // 标记模板
}

// FormatResult 格式化结果
type FormatResult struct {
	Content       []byte        // 格式化后的内容
	Changed       bool          // 内容是否发生变化
	Duration      time.Duration // 格式化耗时
	FormatterUsed string        // 使用的格式化器
	Statistics    FormatStats   // 统计信息
	Warnings      []string      // 警告信息
}

// FormatStats 格式化统计
type FormatStats struct {
	OriginalSize    int // 原始大小
	FormattedSize   int // 格式化后大小
	LinesChanged    int // 改变的行数
	BlocksPreserved int // 保护的块数
}

// FormatError 格式化错误
type FormatError struct {
	Formatter string
	Reason    string
	Err       error
}

func (e *FormatError) Error() string {
	if e.Err != nil {
		return e.Formatter + ": " + e.Reason + ": " + e.Err.Error()
	}
	return e.Formatter + ": " + e.Reason
}

// FormatterMetadata 格式化器元数据
type FormatterMetadata struct {
	Name        string   // 格式化器名称
	Type        string   // 类型：internal/external
	Description string   // 描述
	Formats     []string // 支持的格式
	Priority    int      // 优先级
}

// DefaultFormatOptions 返回默认格式化选项
func DefaultFormatOptions() FormatOptions {
	return FormatOptions{
		PreserveWhitespace: false,
		MaxLineLength:      80,
		TabSize:            4,
		LineEnding:         "\n",
		Encoding:           "utf-8",
		UseExternalTool:    true,
		PreserveBlocks: []PreserveBlock{
			{Type: "code", Pattern: "```[\\s\\S]*?```"},
			{Type: "latex", Pattern: "\\$\\$[\\s\\S]*?\\$\\$"},
		},
	}
}
