package cli_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLIHelp 测试帮助信息
func TestCLIHelp(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/translator/main.go", "--help")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to run help command")
	
	outputStr := string(output)
	
	// 检查基本帮助信息
	assert.Contains(t, outputStr, "翻译工具是一个高质量、灵活的多语言翻译系统")
	assert.Contains(t, outputStr, "Usage:")
	assert.Contains(t, outputStr, "translator [flags] input_file output_file")
	
	// 检查新增的选项
	assert.Contains(t, outputStr, "--provider")
	assert.Contains(t, outputStr, "--stream")
	assert.Contains(t, outputStr, "openai")
	assert.Contains(t, outputStr, "deepl")
}

// TestCLIListProviders 测试列出提供商
func TestCLIListProviders(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/translator/main.go", "--list-providers", "true")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to run list-providers command")
	
	outputStr := string(output)
	
	// 检查提供商列表
	assert.Contains(t, outputStr, "支持的翻译提供商:")
	assert.Contains(t, outputStr, "openai")
	assert.Contains(t, outputStr, "deepl")
	assert.Contains(t, outputStr, "google")
	assert.Contains(t, outputStr, "deeplx")
	assert.Contains(t, outputStr, "libretranslate")
}

// TestCLIVersion 测试版本信息
func TestCLIVersion(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/translator/main.go", "--version")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to run version command")
	
	outputStr := string(output)
	// Cobra 自动处理 version 标志，输出格式可能不同
	assert.True(t, 
		strings.Contains(outputStr, "version") || strings.Contains(outputStr, "翻译工具"),
		"Expected version info, got: %s", outputStr)
	assert.Contains(t, outputStr, "commit")
	assert.Contains(t, outputStr, "built")
}

// TestCLIWithOldTranslator 测试使用旧翻译器的兼容性
func TestCLIWithOldTranslator(t *testing.T) {
	// 设置环境变量以使用旧翻译器
	os.Setenv("USE_NEW_TRANSLATOR", "false")
	defer os.Unsetenv("USE_NEW_TRANSLATOR")
	
	cmd := exec.Command("go", "run", "../../cmd/translator/main.go", "--help")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to run help with old translator")
	
	// 应该仍然能正常工作
	outputStr := string(output)
	assert.Contains(t, outputStr, "翻译工具")
}

// TestCLIMissingArgs 测试缺少参数的情况
func TestCLIMissingArgs(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/translator/main.go")
	output, err := cmd.CombinedOutput()
	
	// 应该返回错误
	assert.Error(t, err)
	
	outputStr := string(output)
	// 应该包含使用说明
	assert.True(t, 
		strings.Contains(outputStr, "accepts 2 arg(s)") || 
		strings.Contains(outputStr, "缺少输入或输出文件参数"),
		"Expected error about missing arguments, got: %s", outputStr)
}