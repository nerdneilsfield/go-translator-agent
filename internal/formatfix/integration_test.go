package formatfix_test

import (
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/formatfix/loader"
	"go.uber.org/zap"
)

func TestFormatFixIntegration(t *testing.T) {
	logger := zap.NewNop()

	// 创建格式修复器注册中心
	registry, err := loader.CreateRegistry(logger)
	if err != nil {
		t.Fatalf("Failed to create format fix registry: %v", err)
	}

	// 测试支持的格式
	supportedFormats := registry.GetSupportedFormats()
	expectedFormats := []string{"markdown", "md", "text", "txt"}

	for _, expected := range expectedFormats {
		found := false
		for _, supported := range supportedFormats {
			if supported == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected format %s not found in supported formats: %v", expected, supportedFormats)
		}
	}

	// 测试 Markdown 修复器
	t.Run("MarkdownFixer", func(t *testing.T) {
		testMarkdownFix(t, registry)
	})

	// 测试 Text 修复器
	t.Run("TextFixer", func(t *testing.T) {
		testTextFix(t, registry)
	})
}

func testMarkdownFix(t *testing.T, registry interface{}) {
	// 这里只是一个占位测试，因为我们没有实际的修复器实例
	// 在实际项目中，你需要创建真实的修复器来测试

	// 测试数据：有格式问题的 Markdown
	badMarkdown := `#Missing space after hash
- Missing space after dash
	Tab character here
  # Wrong heading level
  
  
  
Multiple blank lines above`

	// 预期修复后的结果
	expectedFixed := `# Missing space after hash
- Missing space after dash
    Tab character here
# Wrong heading level

Multiple blank lines above`

	_ = badMarkdown
	_ = expectedFixed

	// TODO: 实现实际的测试逻辑
	t.Log("Markdown fixer test placeholder - registry available")
}

func testTextFix(t *testing.T, registry interface{}) {
	// 测试数据：有格式问题的文本
	badText := `Line with trailing whitespace   
	Line with tab character
Line without final newline`

	// 预期修复后的结果
	expectedFixed := `Line with trailing whitespace
    Line with tab character
Line without final newline
`

	_ = badText
	_ = expectedFixed

	// TODO: 实现实际的测试逻辑
	t.Log("Text fixer test placeholder - registry available")
}

func TestSilentRegistry(t *testing.T) {
	logger := zap.NewNop()

	// 测试静默修复注册中心
	registry, err := loader.CreateSilentRegistry(logger)
	if err != nil {
		t.Fatalf("Failed to create silent registry: %v", err)
	}

	if registry == nil {
		t.Fatal("Silent registry should not be nil")
	}

	// 验证支持的格式
	formats := registry.GetSupportedFormats()
	if len(formats) == 0 {
		t.Error("Silent registry should support some formats")
	}

	t.Logf("Silent registry supports formats: %v", formats)
}

func TestFormatDetection(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"test.md", "markdown"},
		{"test.markdown", "markdown"},
		{"test.txt", "text"},
		{"test.html", "html"},
		{"test.epub", "epub"},
		{"test.tex", "latex"},
		{"test.unknown", "text"}, // 默认为文本
	}

	for _, test := range tests {
		// 这里我们无法直接测试 detectFileFormat 方法，
		// 因为它是 TranslationCoordinator 的私有方法
		// 在实际项目中，你可能需要将其公开或者创建一个工具函数

		t.Logf("Would detect %s as %s format", test.filename, test.expected)
	}
}

func TestRegistryStats(t *testing.T) {
	logger := zap.NewNop()

	registry, err := loader.CreateRegistry(logger)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	stats := registry.GetStats()

	// 验证统计信息
	if totalFixers, ok := stats["total_fixers"].(int); !ok || totalFixers == 0 {
		t.Error("Expected at least one registered fixer")
	}

	if fixers, ok := stats["registered_fixers"].([]string); !ok || len(fixers) == 0 {
		t.Error("Expected registered fixers list")
	}

	if formats, ok := stats["supported_formats"].([]string); !ok || len(formats) == 0 {
		t.Error("Expected supported formats list")
	}

	t.Logf("Registry stats: %+v", stats)
}

func BenchmarkFormatFixCreation(b *testing.B) {
	logger := zap.NewNop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry, err := loader.CreateRegistry(logger)
		if err != nil {
			b.Fatal(err)
		}
		_ = registry
	}
}

func BenchmarkFormatFixExecution(b *testing.B) {
	logger := zap.NewNop()
	registry, err := loader.CreateSilentRegistry(logger)
	if err != nil {
		b.Fatal(err)
	}

	testContent := []byte(`# Test Content
This is a test document with some issues:
- Missing spaces after bullets
#Missing space after hash
	Tab characters here
`)

	fixer, err := registry.GetFixerForFormat("markdown")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := fixer.AutoFix(testContent)
		if err != nil {
			b.Fatal(err)
		}
	}
}
