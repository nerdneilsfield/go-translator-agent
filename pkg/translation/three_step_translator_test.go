package translation

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockProvider 模拟提供者
type MockProvider struct {
	name         string
	translations map[string]string
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) Translate(ctx context.Context, req *Request) (*Response, error) {
	// 简单地返回预设的翻译或模拟响应
	if translation, ok := m.translations[req.Text]; ok {
		return &Response{
			Text: translation,
			Usage: Usage{
				InputTokens:  len(req.Text),
				OutputTokens: len(translation),
			},
		}, nil
	}

	// 默认行为：如果是反思步骤，返回无问题
	if strings.Contains(req.Text, "review a source text") {
		return &Response{
			Text: "No issues found. The translation is perfect.",
			Usage: Usage{
				InputTokens:  100,
				OutputTokens: 10,
			},
		}, nil
	}

	// 默认行为：返回一个简单的翻译
	return &Response{
		Text: "Translated: " + req.Text,
		Usage: Usage{
			InputTokens:  len(req.Text),
			OutputTokens: len(req.Text) + 12,
		},
	}, nil
}

func TestThreeStepTranslator(t *testing.T) {
	// 创建配置
	config := &Config{
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
		ChunkSize:      1000,
		MaxConcurrency: 3,
		Metadata: map[string]interface{}{
			"country": "China",
		},
	}

	// 创建模拟提供者
	providers := map[string]Provider{
		"openai": &MockProvider{
			name:         "openai",
			translations: map[string]string{
				// 预设一些翻译响应
			},
		},
	}

	// 创建步骤集配置
	stepSet := &StepSetConfig{
		Initial: StepConfig{
			Provider:    "openai",
			Model:       "gpt-4",
			Temperature: 0.3,
			MaxTokens:   4096,
		},
		Reflection: StepConfig{
			Provider:    "openai",
			Model:       "gpt-4",
			Temperature: 0.1,
			MaxTokens:   2048,
		},
		Improvement: StepConfig{
			Provider:    "openai",
			Model:       "gpt-4",
			Temperature: 0.3,
			MaxTokens:   4096,
		},
	}

	// 创建翻译器
	translator := NewThreeStepTranslator(config, providers, stepSet)

	t.Run("Basic Translation", func(t *testing.T) {
		ctx := context.Background()
		text := "Hello world"

		result, err := translator.TranslateText(ctx, text)
		require.NoError(t, err)
		assert.NotEmpty(t, result)
		assert.Contains(t, result, "Translated:")
	})

	t.Run("Translation with Protection Blocks", func(t *testing.T) {
		ctx := context.Background()
		text := "This is a formula: $E = mc^2$ and some code: `print('hello')`"

		result, err := translator.TranslateText(ctx, text)
		require.NoError(t, err)

		// 验证保护的内容被还原
		assert.Contains(t, result, "$E = mc^2$")
		assert.Contains(t, result, "`print('hello')`")
	})

	t.Run("Fast Mode", func(t *testing.T) {
		// 启用快速模式
		config.Metadata["fast_mode"] = true
		config.Metadata["fast_mode_threshold"] = 1000

		translator := NewThreeStepTranslator(config, providers, stepSet)

		ctx := context.Background()
		text := "Short text"

		result, err := translator.TranslateText(ctx, text)
		require.NoError(t, err)
		assert.NotEmpty(t, result)
	})
}

func TestProtectionIntegration(t *testing.T) {
	// 测试保护块在整个翻译流程中的工作
	config := &Config{
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
		Metadata: map[string]interface{}{
			"preserve_patterns": []string{"@@PRESERVE_"},
		},
	}

	// 创建一个会保留保护块标记的模拟提供者
	preservingProvider := &MockProvider{
		name:         "preserving",
		translations: map[string]string{},
	}

	// 重写 Translate 方法以保留保护块
	preservingProvider.translations["mock"] = "mock" // 占位

	providers := map[string]Provider{
		"preserving": preservingProvider,
	}

	stepSet := &StepSetConfig{
		Initial: StepConfig{
			Provider: "preserving",
			Model:    "test",
		},
		Reflection: StepConfig{
			Provider: "preserving",
			Model:    "test",
		},
		Improvement: StepConfig{
			Provider: "preserving",
			Model:    "test",
		},
	}

	translator := NewThreeStepTranslator(config, providers, stepSet)

	t.Run("Code Block Protection", func(t *testing.T) {
		ctx := context.Background()
		text := `Here is some code:
'''go
func main() {
    fmt.Println("Hello")
}
'''
And more text.`

		result, err := translator.TranslateText(ctx, text)
		require.NoError(t, err)

		// 验证代码块被保护和还原
		assert.Contains(t, result, "func main()")
		assert.Contains(t, result, "fmt.Println")
	})

	t.Run("LaTeX Protection", func(t *testing.T) {
		ctx := context.Background()
		text := `The equation $\alpha + \beta = \gamma$ is important.
Also see: $$\int_0^1 x^2 dx = \frac{1}{3}$$`

		result, err := translator.TranslateText(ctx, text)
		require.NoError(t, err)

		// 验证 LaTeX 公式被保护
		assert.Contains(t, result, `$\alpha + \beta = \gamma$`)
		assert.Contains(t, result, `$$\int_0^1 x^2 dx = \frac{1}{3}$$`)
	})

	t.Run("URL Protection", func(t *testing.T) {
		ctx := context.Background()
		text := "Visit https://example.com/path?query=value for more info."

		result, err := translator.TranslateText(ctx, text)
		require.NoError(t, err)

		// 验证 URL 被保护
		assert.Contains(t, result, "https://example.com/path?query=value")
	})

	t.Run("Citation Protection", func(t *testing.T) {
		ctx := context.Background()
		text := "As shown in previous studies [1,2,3] and recent work [10-15]."

		result, err := translator.TranslateText(ctx, text)
		require.NoError(t, err)

		// 验证引用被保护
		assert.Contains(t, result, "[1,2,3]")
		assert.Contains(t, result, "[10-15]")
	})
}

func TestPromptBuilderIntegration(t *testing.T) {
	config := &Config{
		SourceLanguage: "English",
		TargetLanguage: "Chinese",
		Metadata: map[string]interface{}{
			"country": "China",
		},
	}

	providers := map[string]Provider{
		"test": &MockProvider{name: "test"},
	}

	stepSet := &StepSetConfig{
		Initial:     StepConfig{Provider: "test", Model: "test"},
		Reflection:  StepConfig{Provider: "test", Model: "test"},
		Improvement: StepConfig{Provider: "test", Model: "test"},
	}

	translator := NewThreeStepTranslator(config, providers, stepSet)

	// 获取提示词构建器
	pb := translator.GetPromptBuilder()
	assert.NotNil(t, pb)

	// 验证提示词包含保护块说明
	prompt := pb.BuildInitialTranslationPrompt("test text")
	assert.Contains(t, prompt, "@@PRESERVE_")
	assert.Contains(t, prompt, "IMPORTANT: Preserve Markers")
}
