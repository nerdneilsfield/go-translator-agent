package translator_tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/stretchr/testify/assert"
)

// 测试预设配置的加载
func TestLoadPredefinedTranslations(t *testing.T) {
	// 创建临时测试文件
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_predefined.toml")

	// 写入测试内容
	testContent := `
source_lang = "English"
target_lang = "Chinese"

[translations]
"Test" = "测试"
"Hello" = "你好"
"World" = "世界"
`
	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	assert.NoError(t, err)

	// 加载预设配置
	predefined, err := config.LoadPredefinedTranslations(testFile)
	assert.NoError(t, err)
	assert.NotNil(t, predefined)

	// 验证配置内容
	assert.Equal(t, "English", predefined.SourceLang)
	assert.Equal(t, "Chinese", predefined.TargetLang)
	assert.Equal(t, 3, len(predefined.Translations))
	assert.Equal(t, "测试", predefined.Translations["Test"])
	assert.Equal(t, "你好", predefined.Translations["Hello"])
	assert.Equal(t, "世界", predefined.Translations["World"])
}

// 测试预设配置加载失败的情况
func TestLoadPredefinedTranslationsFailure(t *testing.T) {
	// 测试文件不存在的情况
	_, err := config.LoadPredefinedTranslations("non_existent_file.toml")
	assert.Error(t, err)

	// 创建临时测试文件，但内容格式错误
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "invalid_predefined.toml")

	// 写入无效的TOML内容
	invalidContent := `
source_lang = "English"
target_lang = "Chinese"
[invalid section
`
	err = os.WriteFile(testFile, []byte(invalidContent), 0o644)
	assert.NoError(t, err)

	// 尝试加载无效的预设配置
	_, err = config.LoadPredefinedTranslations(testFile)
	assert.Error(t, err)
}

// 测试预设配置的应用
func TestApplyPredefinedTranslations(t *testing.T) {
	// 创建预设配置
	predefined := &config.PredefinedTranslation{
		SourceLang: "English",
		TargetLang: "Chinese",
		Translations: map[string]string{
			"Test":  "测试",
			"Hello": "你好",
			"World": "世界",
		},
	}

	// 测试用例
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "单个词替换",
			input:    "Test",
			expected: "测试",
		},
		{
			name:     "多个词替换",
			input:    "Hello World",
			expected: "你好 世界",
		},
		{
			name:     "部分词替换",
			input:    "Hello everyone, this is a Test",
			expected: "你好 everyone, this is a 测试",
		},
		{
			name:     "无需替换",
			input:    "No replacement needed",
			expected: "No replacement needed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 应用预设配置
			result := applyPredefinedTranslations(tc.input, predefined)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// 简单的预设配置应用函数，模拟实际代码中的行为
func applyPredefinedTranslations(text string, predefined *config.PredefinedTranslation) string {
	if predefined == nil || len(predefined.Translations) == 0 {
		return text
	}

	result := text
	for source, target := range predefined.Translations {
		// 简单的字符串替换，实际代码中可能会更复杂
		// 这里只是为了测试预设配置的基本功能
		result = replaceWholeWord(result, source, target)
	}

	return result
}

// 简单的全词替换函数
func replaceWholeWord(text, oldWord, newWord string) string {
	// 这是一个非常简化的实现，实际代码中应该使用正则表达式或更复杂的逻辑
	// 来确保只替换完整的单词，而不是单词的一部分

	// 在实际代码中，这个函数应该考虑单词边界、标点符号等因素
	// 这里只是为了测试预设配置的基本功能

	// 简单替换，不考虑单词边界
	// 在实际应用中，这里应该使用更复杂的逻辑
	for i := 0; i < len(text)-len(oldWord)+1; i++ {
		if text[i:i+len(oldWord)] == oldWord {
			// 检查是否是完整单词（前后是空格或标点符号）
			isWordStart := i == 0 || text[i-1] == ' ' || text[i-1] == ',' || text[i-1] == '.'
			isWordEnd := i+len(oldWord) == len(text) || text[i+len(oldWord)] == ' ' || text[i+len(oldWord)] == ',' || text[i+len(oldWord)] == '.'

			if isWordStart && isWordEnd {
				text = text[:i] + newWord + text[i+len(oldWord):]
				i += len(newWord) - 1 // 调整索引，避免替换刚刚插入的内容
			}
		}
	}

	return text
}
