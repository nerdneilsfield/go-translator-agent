package formatfix

import (
	"context"
)

// FixIssue 表示一个格式问题
type FixIssue struct {
	Type         string   `json:"type"`          // 问题类型，如 "MD010", "SPACING", "STRUCTURE"
	Severity     Severity `json:"severity"`      // 严重程度
	Line         int      `json:"line"`          // 问题所在行号（0表示不适用）
	Column       int      `json:"column"`        // 问题所在列号（0表示不适用）
	Message      string   `json:"message"`       // 问题描述
	Suggestion   string   `json:"suggestion"`    // 修复建议
	CanAutoFix   bool     `json:"can_auto_fix"`  // 是否可以自动修复
	OriginalText string   `json:"original_text"` // 原始文本片段
	FixedText    string   `json:"fixed_text"`    // 修复后的文本片段
}

// Severity 问题严重程度
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// FixAction 用户对修复建议的响应
type FixAction int

const (
	FixActionApply    FixAction = iota // 应用修复
	FixActionSkip                      // 跳过此问题
	FixActionApplyAll                  // 应用此类型的所有修复
	FixActionSkipAll                   // 跳过此类型的所有修复
	FixActionAbort                     // 中止修复过程
)

// UserInteractor 用户交互接口
type UserInteractor interface {
	// ConfirmFix 询问用户是否应用修复
	ConfirmFix(issue *FixIssue) FixAction

	// ShowSummary 显示修复摘要
	ShowSummary(applied, skipped int, issues []*FixIssue)

	// ShowProgress 显示修复进度
	ShowProgress(current, total int, currentIssue string)
}

// FormatFixer 格式修复器接口
type FormatFixer interface {
	// GetName 返回修复器名称
	GetName() string

	// GetSupportedFormats 返回支持的文件格式
	GetSupportedFormats() []string

	// CheckIssues 检查格式问题，但不修复
	CheckIssues(content []byte) ([]*FixIssue, error)

	// PreTranslationFix 翻译前修复
	PreTranslationFix(ctx context.Context, content []byte, interactor UserInteractor) ([]byte, []*FixIssue, error)

	// PostTranslationFix 翻译后修复
	PostTranslationFix(ctx context.Context, content []byte, interactor UserInteractor) ([]byte, []*FixIssue, error)

	// AutoFix 自动修复所有可修复的问题（非交互式）
	AutoFix(content []byte) ([]byte, []*FixIssue, error)
}

// ToolChecker 工具检查接口
type ToolChecker interface {
	// IsToolAvailable 检查外部工具是否可用
	IsToolAvailable(toolName string) bool

	// GetToolVersion 获取工具版本
	GetToolVersion(toolName string) (string, error)

	// GetToolPath 获取工具路径
	GetToolPath(toolName string) (string, error)

	// SuggestInstallation 提供工具安装建议
	SuggestInstallation(toolName string) string
}

// ExternalTool 外部工具执行接口
type ExternalTool interface {
	// Execute 执行外部工具
	Execute(toolName string, args []string, stdin []byte) (stdout, stderr []byte, err error)

	// ExecuteWithTimeout 带超时的执行
	ExecuteWithTimeout(ctx context.Context, toolName string, args []string, stdin []byte) (stdout, stderr []byte, err error)
}

// FixOptions 修复选项
type FixOptions struct {
	// InteractiveMode 是否启用交互模式
	InteractiveMode bool

	// AutoFixTypes 自动修复的问题类型（非交互模式）
	AutoFixTypes []string

	// SkipTypes 跳过的问题类型
	SkipTypes []string

	// MaxIssues 最大处理的问题数量（0表示无限制）
	MaxIssues int

	// StrictMode 严格模式（任何错误都停止处理）
	StrictMode bool

	// PreferExternalTools 优先使用外部工具
	PreferExternalTools bool

	// DryRun 只检查问题，不实际修复
	DryRun bool
}

// FixResult 修复结果
type FixResult struct {
	// Content 修复后的内容
	Content []byte

	// IssuesFound 发现的问题
	IssuesFound []*FixIssue

	// IssuesFixed 已修复的问题
	IssuesFixed []*FixIssue

	// IssuesSkipped 跳过的问题
	IssuesSkipped []*FixIssue

	// Errors 处理过程中的错误
	Errors []error

	// UsedExternalTools 使用的外部工具
	UsedExternalTools []string

	// ProcessingTime 处理时间
	ProcessingTime int64 // nanoseconds
}

// Manager 格式修复管理器
type Manager interface {
	// RegisterFixer 注册格式修复器
	RegisterFixer(fixer FormatFixer)

	// GetFixer 根据文件格式获取修复器
	GetFixer(format string) (FormatFixer, bool)

	// GetAvailableFixers 获取所有可用的修复器
	GetAvailableFixers() map[string]FormatFixer

	// FixFormat 修复指定格式的内容
	FixFormat(ctx context.Context, format string, content []byte, options *FixOptions, interactor UserInteractor) (*FixResult, error)

	// CheckTools 检查所有外部工具的可用性
	CheckTools() map[string]bool
}
