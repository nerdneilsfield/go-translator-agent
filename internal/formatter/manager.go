package formatter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// Manager 格式化管理器
type Manager struct {
	registry *Registry
	logger   *zap.Logger
	options  FormatOptions
}

// NewFormatterManager 创建新的格式化管理器
func NewFormatterManager() *Manager {
	registry := NewFormatterRegistry()
	// 自动注册格式化器
	registry.Register(NewTextFormatter())
	registry.Register(NewMarkdownFormatter())

	return &Manager{
		registry: registry,
		logger:   zap.NewNop(), // 使用无操作的logger避免nil
	}
}

// NewManager 创建格式化管理器
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		registry: globalRegistry,
		logger:   logger,
		options:  DefaultFormatOptions(),
	}
}

// NewManagerWithRegistry 使用指定注册表创建管理器
func NewManagerWithRegistry(registry *Registry, logger *zap.Logger) *Manager {
	return &Manager{
		registry: registry,
		logger:   logger,
		options:  DefaultFormatOptions(),
	}
}

// SetOptions 设置默认选项
func (m *Manager) SetOptions(opts FormatOptions) {
	m.options = opts
}

// FormatFile 格式化文件
func (m *Manager) FormatFile(inputPath string, outputPath string, opts *FormatOptions) (*FormatResult, error) {
	// 检测文件格式
	format := m.detectFormat(inputPath)
	if format == stringUnknown {
		return nil, fmt.Errorf("unsupported file format: %s", filepath.Ext(inputPath))
	}

	// 合并选项
	if opts == nil {
		opts = &m.options
	}

	// 读取文件
	content, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// 格式化内容
	result, err := m.FormatBytes(content, format, *opts)
	if err != nil {
		return nil, err
	}

	// 写入文件
	if result.Changed || inputPath != outputPath {
		if err := os.WriteFile(outputPath, result.Content, 0o644); err != nil {
			return nil, fmt.Errorf("failed to write file: %w", err)
		}
	}

	return result, nil
}

// Format 格式化内容（测试接口）
func (m *Manager) Format(ctx context.Context, content string, format string, opts *FormatOptions) (string, error) {
	formatOpts := FormatOptions{}
	if opts != nil {
		formatOpts = *opts
	}
	result, err := m.FormatBytes([]byte(content), format, formatOpts)
	if err != nil {
		return "", err
	}
	return string(result.Content), nil
}

// FormatBytes 格式化字节内容
func (m *Manager) FormatBytes(content []byte, format string, opts FormatOptions) (*FormatResult, error) {
	startTime := time.Now()

	// 获取格式化器
	formatters := m.registry.Get(format)
	if len(formatters) == 0 {
		return &FormatResult{
			Content:       content,
			Changed:       false,
			Duration:      time.Since(startTime),
			FormatterUsed: "none",
		}, nil
	}

	// 保存原始内容用于比较
	originalContent := make([]byte, len(content))
	copy(originalContent, content)

	// 尝试使用格式化器
	var lastError error
	for _, formatter := range formatters {
		m.logger.Debug("trying formatter",
			zap.String("formatter", formatter.Name()),
			zap.Int("priority", formatter.Priority()))

		// 格式化
		formatted, err := formatter.Format(content, opts)
		if err != nil {
			m.logger.Warn("formatter failed",
				zap.String("formatter", formatter.Name()),
				zap.Error(err))
			lastError = err
			continue
		}

		// 成功
		changed := !bytes.Equal(originalContent, formatted)

		result := &FormatResult{
			Content:       formatted,
			Changed:       changed,
			Duration:      time.Since(startTime),
			FormatterUsed: formatter.Name(),
			Statistics: FormatStats{
				OriginalSize:  len(originalContent),
				FormattedSize: len(formatted),
			},
		}

		if changed {
			result.Statistics.LinesChanged = m.countChangedLines(originalContent, formatted)
		}

		m.logger.Info("formatting completed",
			zap.String("formatter", formatter.Name()),
			zap.Bool("changed", changed),
			zap.Duration("duration", result.Duration))

		return result, nil
	}

	// 所有格式化器都失败
	if lastError != nil {
		return nil, &FormatError{
			Formatter: "all",
			Reason:    "all formatters failed",
			Err:       lastError,
		}
	}

	// 返回原始内容
	return &FormatResult{
		Content:       content,
		Changed:       false,
		Duration:      time.Since(startTime),
		FormatterUsed: "none",
	}, nil
}

// FormatStream 流式格式化
func (m *Manager) FormatStream(reader io.Reader, writer io.Writer, format string, opts FormatOptions) error {
	// 读取所有内容
	content, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read stream: %w", err)
	}

	// 格式化
	result, err := m.FormatBytes(content, format, opts)
	if err != nil {
		return err
	}

	// 写入结果
	_, err = writer.Write(result.Content)
	return err
}

// FormatWithPipeline 使用格式化管道
func (m *Manager) FormatWithPipeline(content []byte, pipeline []PipelineStep) (*FormatResult, error) {
	startTime := time.Now()
	currentContent := content
	var totalStats FormatStats
	var formattersUsed []string

	// 执行管道中的每个步骤
	for _, step := range pipeline {
		formatter := m.registry.GetByFormatAndName(step.Format, step.FormatterName)
		if formatter == nil {
			return nil, fmt.Errorf("formatter not found: %s for format %s", step.FormatterName, step.Format)
		}

		// 合并选项
		opts := m.options
		if step.Options != nil {
			opts = *step.Options
		}

		// 格式化
		formatted, err := formatter.Format(currentContent, opts)
		if err != nil {
			if step.ContinueOnError {
				m.logger.Warn("pipeline step failed, continuing",
					zap.String("formatter", step.FormatterName),
					zap.Error(err))
				continue
			}
			return nil, err
		}

		// 更新统计
		if !bytes.Equal(currentContent, formatted) {
			totalStats.LinesChanged += m.countChangedLines(currentContent, formatted)
		}

		currentContent = formatted
		formattersUsed = append(formattersUsed, formatter.Name())
	}

	// 构建结果
	changed := !bytes.Equal(content, currentContent)
	result := &FormatResult{
		Content:       currentContent,
		Changed:       changed,
		Duration:      time.Since(startTime),
		FormatterUsed: fmt.Sprintf("pipeline[%v]", formattersUsed),
		Statistics: FormatStats{
			OriginalSize:  len(content),
			FormattedSize: len(currentContent),
			LinesChanged:  totalStats.LinesChanged,
		},
	}

	return result, nil
}

// PipelineStep 管道步骤
type PipelineStep struct {
	Format          string
	FormatterName   string
	Options         *FormatOptions
	ContinueOnError bool
}

// detectFormat 检测文件格式
func (m *Manager) detectFormat(filePath string) string {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".md", ".markdown":
		return stringMarkdown
	case ".txt", ".text":
		return stringText
	case ".html", ".htm":
		return stringHTML
	case ".epub":
		return stringEPUB
	case ".tex", ".latex":
		return stringLaTeX
	default:
		return stringUnknown
	}
}

// countChangedLines 统计改变的行数
func (m *Manager) countChangedLines(original, formatted []byte) int {
	originalLines := bytes.Split(original, []byte("\n"))
	formattedLines := bytes.Split(formatted, []byte("\n"))

	// 简单的行数差异统计
	changed := 0
	maxLen := len(originalLines)
	if len(formattedLines) > maxLen {
		maxLen = len(formattedLines)
	}

	for i := 0; i < maxLen; i++ {
		if i >= len(originalLines) || i >= len(formattedLines) {
			changed++
		} else if !bytes.Equal(originalLines[i], formattedLines[i]) {
			changed++
		}
	}

	return changed
}

// FormatFileInPlace 原地格式化文件
func (m *Manager) FormatFileInPlace(filePath string, opts *FormatOptions) (*FormatResult, error) {
	return m.FormatFile(filePath, filePath, opts)
}

// CanFormat 检查是否可以格式化指定格式
func (m *Manager) CanFormat(format string) bool {
	formatters := m.registry.Get(format)
	return len(formatters) > 0
}

// ListAvailableFormatters 列出可用的格式化器
func (m *Manager) ListAvailableFormatters() map[string][]string {
	return m.registry.ListFormats()
}
