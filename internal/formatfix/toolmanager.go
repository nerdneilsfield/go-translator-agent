package formatfix

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Tool 外部工具信息
type Tool struct {
	Name            string            // 工具名称
	Command         string            // 执行命令
	VersionFlag     string            // 版本查询参数
	InstallCommands map[string]string // 安装命令（按操作系统）
	Description     string            // 工具描述
}

// DefaultToolManager 默认工具管理器实现
type DefaultToolManager struct {
	tools        map[string]*Tool
	toolStatus   map[string]bool   // 工具可用性缓存
	toolVersions map[string]string // 工具版本缓存
	toolPaths    map[string]string // 工具路径缓存
	mutex        sync.RWMutex
	logger       *zap.Logger
}

// NewDefaultToolManager 创建默认工具管理器
func NewDefaultToolManager(logger *zap.Logger) *DefaultToolManager {
	tm := &DefaultToolManager{
		tools:        make(map[string]*Tool),
		toolStatus:   make(map[string]bool),
		toolVersions: make(map[string]string),
		toolPaths:    make(map[string]string),
		logger:       logger,
	}

	// 注册默认工具
	tm.registerDefaultTools()

	return tm
}

// registerDefaultTools 注册默认工具
func (tm *DefaultToolManager) registerDefaultTools() {
	tools := []*Tool{
		{
			Name:        "markdownlint",
			Command:     "markdownlint",
			VersionFlag: "--version",
			InstallCommands: map[string]string{
				"linux":   "npm install -g markdownlint-cli",
				"darwin":  "npm install -g markdownlint-cli",
				"windows": "npm install -g markdownlint-cli",
			},
			Description: "Markdown 代码检查工具",
		},
		{
			Name:        "prettier",
			Command:     "prettier",
			VersionFlag: "--version",
			InstallCommands: map[string]string{
				"linux":   "npm install -g prettier",
				"darwin":  "npm install -g prettier",
				"windows": "npm install -g prettier",
			},
			Description: "代码格式化工具，支持 Markdown、HTML、CSS 等",
		},
		{
			Name:        "htmlhint",
			Command:     "htmlhint",
			VersionFlag: "--version",
			InstallCommands: map[string]string{
				"linux":   "npm install -g htmlhint",
				"darwin":  "npm install -g htmlhint",
				"windows": "npm install -g htmlhint",
			},
			Description: "HTML 代码检查工具",
		},
	}

	for _, tool := range tools {
		tm.tools[tool.Name] = tool
	}
}

// IsToolAvailable 检查工具是否可用
func (tm *DefaultToolManager) IsToolAvailable(toolName string) bool {
	tm.mutex.RLock()
	if status, exists := tm.toolStatus[toolName]; exists {
		tm.mutex.RUnlock()
		return status
	}
	tm.mutex.RUnlock()

	// 检查工具可用性
	available := tm.checkToolAvailability(toolName)

	tm.mutex.Lock()
	tm.toolStatus[toolName] = available
	tm.mutex.Unlock()

	return available
}

// checkToolAvailability 实际检查工具可用性
func (tm *DefaultToolManager) checkToolAvailability(toolName string) bool {
	tool, exists := tm.tools[toolName]
	if !exists {
		return false
	}

	// 尝试找到工具路径
	path, err := exec.LookPath(tool.Command)
	if err != nil {
		tm.logger.Debug("tool not found in PATH",
			zap.String("tool", toolName),
			zap.String("command", tool.Command))
		return false
	}

	// 缓存工具路径
	tm.mutex.Lock()
	tm.toolPaths[toolName] = path
	tm.mutex.Unlock()

	// 尝试执行版本命令验证工具
	if tool.VersionFlag != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, path, tool.VersionFlag)
		output, err := cmd.Output()
		if err != nil {
			tm.logger.Debug("tool version check failed",
				zap.String("tool", toolName),
				zap.Error(err))
			return false
		}

		version := strings.TrimSpace(string(output))
		tm.mutex.Lock()
		tm.toolVersions[toolName] = version
		tm.mutex.Unlock()

		tm.logger.Debug("tool available",
			zap.String("tool", toolName),
			zap.String("version", version))
	}

	return true
}

// GetToolVersion 获取工具版本
func (tm *DefaultToolManager) GetToolVersion(toolName string) (string, error) {
	tm.mutex.RLock()
	if version, exists := tm.toolVersions[toolName]; exists {
		tm.mutex.RUnlock()
		return version, nil
	}
	tm.mutex.RUnlock()

	if !tm.IsToolAvailable(toolName) {
		return "", fmt.Errorf("tool %s is not available", toolName)
	}

	tm.mutex.RLock()
	version := tm.toolVersions[toolName]
	tm.mutex.RUnlock()

	return version, nil
}

// GetToolPath 获取工具路径
func (tm *DefaultToolManager) GetToolPath(toolName string) (string, error) {
	tm.mutex.RLock()
	if path, exists := tm.toolPaths[toolName]; exists {
		tm.mutex.RUnlock()
		return path, nil
	}
	tm.mutex.RUnlock()

	if !tm.IsToolAvailable(toolName) {
		return "", fmt.Errorf("tool %s is not available", toolName)
	}

	tm.mutex.RLock()
	path := tm.toolPaths[toolName]
	tm.mutex.RUnlock()

	return path, nil
}

// SuggestInstallation 提供工具安装建议
func (tm *DefaultToolManager) SuggestInstallation(toolName string) string {
	tool, exists := tm.tools[toolName]
	if !exists {
		return fmt.Sprintf("未知工具: %s", toolName)
	}

	osName := runtime.GOOS
	installCmd, exists := tool.InstallCommands[osName]
	if !exists {
		// 尝试通用的 linux 命令
		if cmd, ok := tool.InstallCommands["linux"]; ok {
			installCmd = cmd
		} else {
			return fmt.Sprintf("暂不支持在 %s 系统上安装 %s", osName, toolName)
		}
	}

	return fmt.Sprintf("安装 %s:\n  %s\n\n%s", toolName, installCmd, tool.Description)
}

// Execute 执行外部工具
func (tm *DefaultToolManager) Execute(toolName string, args []string, stdin []byte) (stdout, stderr []byte, err error) {
	ctx := context.Background()
	return tm.ExecuteWithTimeout(ctx, toolName, args, stdin)
}

// ExecuteWithTimeout 带超时的执行
func (tm *DefaultToolManager) ExecuteWithTimeout(ctx context.Context, toolName string, args []string, stdin []byte) (stdout, stderr []byte, err error) {
	path, err := tm.GetToolPath(toolName)
	if err != nil {
		return nil, nil, fmt.Errorf("tool %s not available: %w", toolName, err)
	}

	cmd := exec.CommandContext(ctx, path, args...)

	if len(stdin) > 0 {
		cmd.Stdin = strings.NewReader(string(stdin))
	}

	tm.logger.Debug("executing tool",
		zap.String("tool", toolName),
		zap.Strings("args", args))

	stdout, err = cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = exitErr.Stderr
		}
		return stdout, stderr, fmt.Errorf("tool execution failed: %w", err)
	}

	return stdout, nil, nil
}

// RefreshToolStatus 刷新工具状态缓存
func (tm *DefaultToolManager) RefreshToolStatus() {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	tm.toolStatus = make(map[string]bool)
	tm.toolVersions = make(map[string]string)
	tm.toolPaths = make(map[string]string)
}

// GetAllTools 获取所有已注册的工具
func (tm *DefaultToolManager) GetAllTools() map[string]*Tool {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	result := make(map[string]*Tool)
	for name, tool := range tm.tools {
		result[name] = tool
	}

	return result
}

// GetAvailableTools 获取所有可用的工具
func (tm *DefaultToolManager) GetAvailableTools() map[string]*Tool {
	result := make(map[string]*Tool)

	for name, tool := range tm.tools {
		if tm.IsToolAvailable(name) {
			result[name] = tool
		}
	}

	return result
}

// RegisterTool 注册新工具
func (tm *DefaultToolManager) RegisterTool(tool *Tool) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	tm.tools[tool.Name] = tool

	// 清除该工具的缓存状态，下次检查时会重新验证
	delete(tm.toolStatus, tool.Name)
	delete(tm.toolVersions, tool.Name)
	delete(tm.toolPaths, tool.Name)
}
