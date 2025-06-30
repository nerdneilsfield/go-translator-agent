package formatter

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ExternalFormatter 外部工具格式化器
type ExternalFormatter struct {
	name       string
	command    string
	args       []string
	priority   int
	formats    []string
	workingDir string
}

// NewExternalFormatter 创建外部格式化器
func NewExternalFormatter(command string, args []string) *ExternalFormatter {
	return &ExternalFormatter{
		name:     fmt.Sprintf("external-%s", command),
		command:  command,
		args:     args,
		priority: 100, // 外部工具通常优先级更高
		formats:  detectSupportedFormats(command),
	}
}

// Format 使用外部工具格式化
func (f *ExternalFormatter) Format(content []byte, opts FormatOptions) ([]byte, error) {
	// 检查命令是否存在
	if !IsExternalToolAvailable(f.command) {
		return nil, &FormatError{
			Formatter: f.name,
			Reason:    fmt.Sprintf("command not found: %s", f.command),
		}
	}

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "formatter-*.tmp")
	if err != nil {
		return nil, &FormatError{
			Formatter: f.name,
			Reason:    "failed to create temp file",
			Err:       err,
		}
	}
	defer os.Remove(tmpFile.Name())

	// 写入内容
	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		return nil, &FormatError{
			Formatter: f.name,
			Reason:    "failed to write temp file",
			Err:       err,
		}
	}
	tmpFile.Close()

	// 构建命令参数
	args := f.buildArgs(tmpFile.Name(), opts)

	// 执行命令
	cmd := exec.Command(f.command, args...)
	if f.workingDir != "" {
		cmd.Dir = f.workingDir
	}

	// 捕获输出
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, &FormatError{
			Formatter: f.name,
			Reason:    fmt.Sprintf("command failed: %s", stderr.String()),
			Err:       err,
		}
	}

	// 读取格式化后的内容
	formatted, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		// 某些工具可能输出到 stdout
		if stdout.Len() > 0 {
			return stdout.Bytes(), nil
		}
		return nil, &FormatError{
			Formatter: f.name,
			Reason:    "failed to read formatted content",
			Err:       err,
		}
	}

	return formatted, nil
}

// CanFormat 检查是否支持格式
func (f *ExternalFormatter) CanFormat(format string) bool {
	for _, f := range f.formats {
		if f == format {
			return true
		}
	}
	return false
}

// Priority 返回优先级
func (f *ExternalFormatter) Priority() int {
	return f.priority
}

// Name 返回名称
func (f *ExternalFormatter) Name() string {
	return f.name
}

// GetMetadata 返回格式化器元数据
func (f *ExternalFormatter) GetMetadata() FormatterMetadata {
	return FormatterMetadata{
		Name:        f.name,
		Type:        "external",
		Description: fmt.Sprintf("External formatter using %s", f.command),
		Formats:     f.formats,
		Priority:    f.priority,
	}
}

// SetPriority 设置优先级
func (f *ExternalFormatter) SetPriority(priority int) {
	f.priority = priority
}

// SetFormats 设置支持的格式
func (f *ExternalFormatter) SetFormats(formats []string) {
	f.formats = formats
}

// buildArgs 构建命令参数
func (f *ExternalFormatter) buildArgs(filePath string, opts FormatOptions) []string {
	args := make([]string, len(f.args))
	copy(args, f.args)

	// 添加自定义参数
	if len(opts.ExternalToolArgs) > 0 {
		args = append(args, opts.ExternalToolArgs...)
	}

	// 特殊处理某些工具
	switch f.command {
	case "prettier":
		// Prettier 特定参数
		args = append(args, "--write")
		if opts.TabSize > 0 {
			args = append(args, "--tab-width", fmt.Sprintf("%d", opts.TabSize))
		}
		if opts.MaxLineLength > 0 {
			args = append(args, "--print-width", fmt.Sprintf("%d", opts.MaxLineLength))
		}

	case "latexindent":
		// LaTeX indent 特定参数
		if !contains(args, "-w") {
			args = append(args, "-w")
		}

	case "black":
		// Python Black 格式化器
		if opts.MaxLineLength > 0 {
			args = append(args, "--line-length", fmt.Sprintf("%d", opts.MaxLineLength))
		}
	}

	// 添加文件路径
	args = append(args, filePath)

	return args
}

// IsExternalToolAvailable 检查外部工具是否可用
func IsExternalToolAvailable(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

// detectSupportedFormats 检测工具支持的格式
func detectSupportedFormats(command string) []string {
	switch command {
	case "prettier":
		return []string{
			stringMarkdown,
			stringHTML,
		}
	case "latexindent":
		return []string{
			stringLaTeX,
		}
	case "black":
		return []string{
			// Python 文件，如果将来支持的话
		}
	default:
		return []string{}
	}
}

// contains 检查切片是否包含元素
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// PrettierFormatter Prettier 专用格式化器
type PrettierFormatter struct {
	*ExternalFormatter
}

// NewPrettierFormatter 创建 Prettier 格式化器
func NewPrettierFormatter() *PrettierFormatter {
	return &PrettierFormatter{
		ExternalFormatter: &ExternalFormatter{
			name:     "prettier",
			command:  "prettier",
			priority: 100,
			formats: []string{
				stringMarkdown,
				stringHTML,
			},
		},
	}
}

// Format 使用 Prettier 格式化
func (f *PrettierFormatter) Format(content []byte, opts FormatOptions) ([]byte, error) {
	// 检测文件类型并设置解析器
	ext := f.detectFileType(content)
	parser := f.getParser(ext)

	// 设置 Prettier 特定参数
	f.args = []string{
		"--write",
		"--parser", parser,
	}

	return f.ExternalFormatter.Format(content, opts)
}

// detectFileType 检测文件类型
func (f *PrettierFormatter) detectFileType(content []byte) string {
	// 简单的启发式检测
	str := string(content[:min(1000, len(content))])

	if strings.Contains(str, "<!DOCTYPE html") || strings.Contains(str, "<html") {
		return ".html"
	}
	if strings.Contains(str, "# ") || strings.Contains(str, "## ") {
		return ".md"
	}

	return ".txt"
}

// getParser 获取解析器
func (f *PrettierFormatter) getParser(ext string) string {
	switch ext {
	case ".md", ".markdown":
		return "markdown"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".js":
		return "babel"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return "markdown"
	}
}

// min 返回较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CreateExternalFormatter 创建外部格式化器的便捷函数
func CreateExternalFormatter(config ExternalFormatterConfig) *ExternalFormatter {
	formatter := &ExternalFormatter{
		name:       config.Name,
		command:    config.Command,
		args:       config.Args,
		priority:   config.Priority,
		formats:    config.Formats,
		workingDir: config.WorkingDir,
	}

	if formatter.priority == 0 {
		formatter.priority = 100
	}

	if len(formatter.formats) == 0 {
		formatter.formats = detectSupportedFormats(formatter.command)
	}

	return formatter
}

// ExternalFormatterConfig 外部格式化器配置
type ExternalFormatterConfig struct {
	Name       string
	Command    string
	Args       []string
	Priority   int
	Formats    []string
	WorkingDir string
}

// CommonExternalFormatters 常用外部格式化器配置
var CommonExternalFormatters = map[string]ExternalFormatterConfig{
	"prettier": {
		Name:    "prettier",
		Command: "prettier",
		Args:    []string{"--write"},
		Formats: []string{
			stringMarkdown,
			stringHTML,
		},
	},
	"latexindent": {
		Name:    "latexindent",
		Command: "latexindent",
		Args:    []string{"-w"},
		Formats: []string{
			stringLaTeX,
		},
	},
	"black": {
		Name:    "black",
		Command: "black",
		Args:    []string{},
		Formats: []string{
			// Python files
		},
	},
	"gofmt": {
		Name:    "gofmt",
		Command: "gofmt",
		Args:    []string{"-w"},
		Formats: []string{
			// Go files
		},
	},
}
