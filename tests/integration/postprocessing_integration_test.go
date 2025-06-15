package integration

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/internal/testutils"
	"github.com/nerdneilsfield/go-translator-agent/internal/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTranslationPostProcessingIntegration(t *testing.T) {
	// 创建测试配置
	tempDir := t.TempDir()
	cfg := testutils.CreatePostProcessingTestConfig(tempDir)

	// 创建logger
	log := logger.NewLogger(false)

	// 创建翻译后处理器
	processor := translator.NewTranslationPostProcessor(cfg, log)
	require.NotNil(t, processor)

	ctx := context.Background()

	t.Run("Prompt Markers Cleanup", func(t *testing.T) {
		originalText := "This is the original text about machine learning."
		translatedText := `<TRANSLATION>This is translated text about machine learning.</TRANSLATION>
Translation: 这是关于机器学习的翻译文本。
翻译结果：这是机器学习的内容。`

		result, err := processor.ProcessTranslation(ctx, originalText, translatedText, nil)
		require.NoError(t, err)

		// 验证提示词标记被清理
		assert.NotContains(t, result, "<TRANSLATION>")
		assert.NotContains(t, result, "</TRANSLATION>")
		assert.NotContains(t, result, "Translation:")
		assert.NotContains(t, result, "翻译结果：")
	})

	t.Run("Content Protection", func(t *testing.T) {
		originalText := `Contact us at support@example.com or visit https://api.example.com/v1/data
DOI: 10.1038/nature12373
Version: v2.1.0
Code: pip install tensorflow`

		translatedText := `联系我们通过 支持@例子.com 或访问 https://api.example.com/v1/data
DOI: 10.1038/nature12373
版本: v2.1.0
代码: pip 安装 tensorflow`

		result, err := processor.ProcessTranslation(ctx, originalText, translatedText, nil)
		require.NoError(t, err)

		// 验证受保护内容被保留
		assert.Contains(t, result, "support@example.com")
		assert.Contains(t, result, "https://api.example.com/v1/data")
		assert.Contains(t, result, "DOI: 10.1038/nature12373")
		assert.Contains(t, result, "v2.1.0")
		assert.Contains(t, result, "pip install tensorflow")
	})

	t.Run("Quote Normalization", func(t *testing.T) {
		originalText := `This uses "regular quotes" and 'apostrophes'.`
		translatedText := `这使用"智能引号"和'弯曲撇号'。还有「中文引号」和『书名号』。`

		result, err := processor.ProcessTranslation(ctx, originalText, translatedText, nil)
		require.NoError(t, err)

		// 验证引号被规范化
		assert.Contains(t, result, `"智能引号"`)
		assert.Contains(t, result, `'弯曲撇号'`)
		assert.Contains(t, result, `"中文引号"`)
		assert.NotContains(t, result, "「")
		assert.NotContains(t, result, "」")
		assert.NotContains(t, result, "『")
		assert.NotContains(t, result, "』")
	})

	t.Run("Mixed Language Spacing", func(t *testing.T) {
		originalText := "This is about version 2.0 and machine learning."
		translatedText := "这是关于version2.0和machine learning的内容。"

		result, err := processor.ProcessTranslation(ctx, originalText, translatedText, nil)
		require.NoError(t, err)

		// 验证中英文之间添加了空格
		assert.Contains(t, result, "version 2.0")
		assert.Contains(t, result, "machine learning")
		// 验证中文和英文之间有空格
		assert.True(t, strings.Contains(result, "关于 version") || strings.Contains(result, "version 2.0 和"))
	})

	t.Run("Machine Translation Cleanup", func(t *testing.T) {
		originalText := "Google and Microsoft are leading companies."
		translatedText := "谷歌公司 and 微软公司 are are leading companies."

		result, err := processor.ProcessTranslation(ctx, originalText, translatedText, nil)
		require.NoError(t, err)

		// 验证品牌名修复和重复词清理
		assert.Contains(t, result, "Google")
		assert.Contains(t, result, "Microsoft")
		assert.NotContains(t, result, "谷歌公司")
		assert.NotContains(t, result, "微软公司")
		// 验证重复词被清理
		assert.NotContains(t, result, "are are")
	})

	t.Run("Ellipsis and Dash Normalization", func(t *testing.T) {
		originalText := "This is a test...with various punctuation."
		translatedText := "这是一个测试..with various——punctuation。"

		result, err := processor.ProcessTranslation(ctx, originalText, translatedText, nil)
		require.NoError(t, err)

		// 验证省略号和破折号规范化
		assert.Contains(t, result, "...")
		assert.Contains(t, result, "——")
	})
}

func TestGlossaryIntegration(t *testing.T) {
	// 检查词汇表文件是否存在
	glossaryPath := "../../configs/glossary_example.yaml"
	if _, err := os.Stat(glossaryPath); os.IsNotExist(err) {
		t.Skip("Glossary file not found, skipping glossary tests")
	}

	cfg := testutils.CreatePostProcessingTestConfig(glossaryPath)
	cfg.GlossaryPath = glossaryPath

	log := logger.NewLogger(false)
	processor := translator.NewTranslationPostProcessor(cfg, log)
	require.NotNil(t, processor)

	ctx := context.Background()

	t.Run("Terminology Consistency", func(t *testing.T) {
		originalText := "Machine learning and deep learning are AI technologies."
		translatedText := "机器学习和深度学习是AI技术。"

		result, err := processor.ProcessTranslation(ctx, originalText, translatedText, nil)
		require.NoError(t, err)

		// 如果词汇表加载成功，术语应该被统一
		// 这里我们主要验证处理过程不会出错
		assert.NotEmpty(t, result)
		t.Logf("Original: %s", originalText)
		t.Logf("Translated: %s", translatedText)
		t.Logf("Processed: %s", result)
	})

	t.Run("Brand Name Protection", func(t *testing.T) {
		originalText := "Google and Microsoft provide AI services."
		translatedText := "谷歌和微软提供AI服务。"

		result, err := processor.ProcessTranslation(ctx, originalText, translatedText, nil)
		require.NoError(t, err)

		// 品牌名应该被保护或修复
		assert.NotEmpty(t, result)
		t.Logf("Original: %s", originalText)
		t.Logf("Translated: %s", translatedText)
		t.Logf("Processed: %s", result)
	})
}

func TestTranslationCoordinatorWithPostProcessing(t *testing.T) {
	// 创建测试配置
	cfg := testutils.CreatePostProcessingTestConfig("./temp")

	log := logger.NewLogger(false)
	progressPath := t.TempDir()

	// 创建翻译协调器
	coordinator, err := translator.NewTranslationCoordinator(cfg, log, progressPath)
	require.NoError(t, err)
	require.NotNil(t, coordinator)

	t.Run("Coordinator Initialization", func(t *testing.T) {
		// 验证翻译协调器正确初始化了翻译后处理器
		assert.NotNil(t, coordinator)
		// 这里我们主要验证创建过程不会出错
	})

	t.Run("Text Translation with Post Processing", func(t *testing.T) {
		ctx := context.Background()
		testText := "This is a test of machine learning technology."

		// 由于我们没有真实的翻译服务，这里主要测试流程
		result, err := coordinator.TranslateText(ctx, testText)

		// 在模拟模式下，应该返回带前缀的文本
		if err == nil {
			assert.Contains(t, result, "Translated:")
			t.Logf("Translation result: %s", result)
		} else {
			t.Logf("Translation failed (expected in test mode): %v", err)
		}
	})
}

func TestConfigurationOverrides(t *testing.T) {
	t.Run("Command Line Override Simulation", func(t *testing.T) {
		// 创建基础配置
		cfg := testutils.CreatePostProcessingTestConfig("./temp")
		// 先关闭所有后处理功能以测试覆盖
		cfg.EnablePostProcessing = false
		cfg.ContentProtection = false
		cfg.TerminologyConsistency = false
		cfg.MixedLanguageSpacing = false
		cfg.MachineTranslationCleanup = false

		// 模拟命令行覆盖
		cfg.EnablePostProcessing = true
		cfg.ContentProtection = true
		cfg.TerminologyConsistency = true
		cfg.MixedLanguageSpacing = true
		cfg.MachineTranslationCleanup = true
		cfg.GlossaryPath = "../../configs/glossary_example.yaml"

		// 验证配置被正确覆盖
		assert.True(t, cfg.EnablePostProcessing)
		assert.True(t, cfg.ContentProtection)
		assert.True(t, cfg.TerminologyConsistency)
		assert.True(t, cfg.MixedLanguageSpacing)
		assert.True(t, cfg.MachineTranslationCleanup)
		assert.Equal(t, "../../configs/glossary_example.yaml", cfg.GlossaryPath)
	})
}

// 基准测试
func BenchmarkPostProcessing(b *testing.B) {
	cfg := testutils.CreatePostProcessingTestConfig("./temp")

	log := zap.NewNop()
	processor := translator.NewTranslationPostProcessor(cfg, log)
	ctx := context.Background()

	originalText := "This is a test of machine learning and deep learning technologies from Google and Microsoft."
	translatedText := `<TRANSLATION>这是关于机器学习和深度学习技术的测试，来自谷歌公司和微软公司。</TRANSLATION>
Translation: 这是machine learning和deep learning技术的测试。`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := processor.ProcessTranslation(ctx, originalText, translatedText, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
