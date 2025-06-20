package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/nerdneilsfield/go-translator-agent/pkg/providers"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/deepl"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/deeplx"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/google"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/libretranslate"
	"github.com/nerdneilsfield/go-translator-agent/pkg/providers/openai"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

func main() {
	// 命令行参数
	var (
		provider     = flag.String("provider", "openai", "Translation provider to use (openai, google, deepl, deeplx, libretranslate)")
		text         = flag.String("text", "Hello, world! This is a test.", "Text to translate")
		sourceLang   = flag.String("source", "English", "Source language")
		targetLang   = flag.String("target", "Chinese", "Target language")
		showCaps     = flag.Bool("caps", false, "Show provider capabilities")
		useThreeStep = flag.Bool("three-step", false, "Use three-step translation (OpenAI only)")
	)
	flag.Parse()

	// 初始化提供商
	var translationProvider translation.TranslationProvider
	var err error

	switch *provider {
	case "openai":
		translationProvider, err = initOpenAI()
	case "google":
		translationProvider, err = initGoogle()
	case "deepl":
		translationProvider, err = initDeepL()
	case "deeplx":
		translationProvider, err = initDeepLX()
	case "libretranslate":
		translationProvider, err = initLibreTranslate()
	default:
		log.Fatalf("Unknown provider: %s", *provider)
	}

	if err != nil {
		log.Fatalf("Failed to initialize provider: %v", err)
	}

	// 显示能力信息
	if *showCaps {
		showCapabilities(translationProvider)
		return
	}

	// 执行翻译
	if *useThreeStep && *provider == "openai" {
		// 三步翻译示例
		performThreeStepTranslation(*text, *sourceLang, *targetLang)
	} else {
		// 单步翻译
		performSingleTranslation(translationProvider, *text, *sourceLang, *targetLang)
	}
}

// initOpenAI 初始化OpenAI提供商
func initOpenAI() (translation.TranslationProvider, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}

	// 使用新的官方 SDK 版本
	config := openai.DefaultConfigV2()
	config.APIKey = apiKey
	config.Model = "gpt-3.5-turbo"

	return openai.NewV2(config), nil
}

// initGoogle 初始化Google Translate提供商
func initGoogle() (translation.TranslationProvider, error) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GOOGLE_API_KEY environment variable not set")
	}

	config := google.DefaultConfig()
	config.APIKey = apiKey

	return google.New(config), nil
}

// initDeepL 初始化DeepL提供商
func initDeepL() (translation.TranslationProvider, error) {
	apiKey := os.Getenv("DEEPL_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("DEEPL_API_KEY environment variable not set")
	}

	config := deepl.DefaultConfig()
	config.APIKey = apiKey

	// 检查是否使用免费API
	if os.Getenv("DEEPL_FREE_API") == "true" {
		config.UseFreeAPI = true
	}

	return deepl.New(config), nil
}

// initDeepLX 初始化DeepLX提供商
func initDeepLX() (translation.TranslationProvider, error) {
	config := deeplx.DefaultConfig()

	// 可选：设置自定义端点
	if endpoint := os.Getenv("DEEPLX_ENDPOINT"); endpoint != "" {
		config.APIEndpoint = endpoint
	}

	// 可选：设置访问令牌
	if token := os.Getenv("DEEPLX_TOKEN"); token != "" {
		config.AccessToken = token
	}

	return deeplx.New(config), nil
}

// initLibreTranslate 初始化LibreTranslate提供商
func initLibreTranslate() (translation.TranslationProvider, error) {
	config := libretranslate.DefaultConfig()

	// 可选：设置自定义端点
	if endpoint := os.Getenv("LIBRETRANSLATE_ENDPOINT"); endpoint != "" {
		config.APIEndpoint = endpoint
	}

	// 可选：设置API密钥
	if apiKey := os.Getenv("LIBRETRANSLATE_API_KEY"); apiKey != "" {
		config.APIKey = apiKey
		config.RequiresAPIKey = true
	}

	return libretranslate.New(config), nil
}

// performSingleTranslation 执行单步翻译
func performSingleTranslation(provider translation.TranslationProvider, text, sourceLang, targetLang string) {
	ctx := context.Background()

	fmt.Printf("=== %s Translation ===\n", provider.GetName())
	fmt.Printf("Source Language: %s\n", sourceLang)
	fmt.Printf("Target Language: %s\n", targetLang)
	fmt.Printf("Original Text: %s\n\n", text)

	// 创建翻译请求
	req := &translation.ProviderRequest{
		Text:           text,
		SourceLanguage: sourceLang,
		TargetLanguage: targetLang,
	}

	// 执行翻译
	resp, err := provider.Translate(ctx, req)
	if err != nil {
		log.Fatalf("Translation failed: %v", err)
	}

	// 显示结果
	fmt.Printf("Translated Text: %s\n", resp.Text)
	if resp.TokensIn > 0 || resp.TokensOut > 0 {
		fmt.Printf("Tokens: %d in, %d out\n", resp.TokensIn, resp.TokensOut)
	}
	if len(resp.Metadata) > 0 {
		fmt.Println("Metadata:")
		for k, v := range resp.Metadata {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}
}

// performThreeStepTranslation 执行三步翻译（仅限OpenAI）
func performThreeStepTranslation(text, sourceLang, targetLang string) {
	ctx := context.Background()

	fmt.Println("=== Three-Step Translation with OpenAI ===")
	fmt.Printf("Source Language: %s\n", sourceLang)
	fmt.Printf("Target Language: %s\n", targetLang)
	fmt.Printf("Original Text: %s\n\n", text)

	// 获取API密钥
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable not set")
	}

	// 创建OpenAI provider
	config := openai.DefaultConfigV2()
	config.APIKey = apiKey
	provider := openai.NewV2(config)

	// 创建翻译配置
	translationConfig := &translation.Config{
		SourceLanguage: sourceLang,
		TargetLanguage: targetLang,
		ChunkSize:      1000,
		MaxConcurrency: 1,
		Steps: []translation.StepConfig{
			{
				Name:        "initial_translation",
				Provider:    "openai",
				Model:       "gpt-3.5-turbo",
				Temperature: 0.3,
				MaxTokens:   2048,
				AdditionalNotes: "You are a professional translator. Maintain the original meaning, tone, and style as much as possible.",
				IsLLM:      true,
			},
			{
				Name:        "reflection",
				Provider:    "openai",
				Model:       "gpt-3.5-turbo",
				Temperature: 0.1,
				MaxTokens:   1024,
				AdditionalNotes: "You are a translation quality reviewer. Identify any issues with accuracy, fluency, cultural appropriateness, or style.",
				IsLLM:      true,
			},
			{
				Name:        "improvement",
				Provider:    "openai",
				Model:       "gpt-3.5-turbo",
				Temperature: 0.3,
				MaxTokens:   2048,
				AdditionalNotes: "You are a professional translator focusing on quality improvement. Provide an improved translation that addresses the feedback.",
				IsLLM:      true,
			},
		},
	}

	// 创建翻译服务
	translator, err := translation.New(translationConfig,
		translation.WithSingleProvider("openai", provider),
	)
	if err != nil {
		log.Fatalf("Failed to create translator: %v", err)
	}

	// 执行翻译
	resp, err := translator.TranslateText(ctx, text)
	if err != nil {
		log.Fatalf("Translation failed: %v", err)
	}

	fmt.Printf("Final Translation: %s\n", resp)
}

// showCapabilities 显示提供商能力
func showCapabilities(provider translation.TranslationProvider) {
	// 尝试转换为providers.Provider以获取更多信息
	if p, ok := provider.(providers.Provider); ok {
		caps := p.GetCapabilities()

		fmt.Printf("=== %s Capabilities ===\n", provider.GetName())
		fmt.Printf("Requires API Key: %v\n", caps.RequiresAPIKey)
		fmt.Printf("Max Text Length: %d\n", caps.MaxTextLength)
		fmt.Printf("Supports Batch: %v\n", caps.SupportsBatch)
		fmt.Printf("Supports Formatting: %v\n", caps.SupportsFormatting)
		fmt.Printf("Supports Multi-Step: %v\n", provider.SupportsSteps())

		if caps.RateLimit != nil {
			fmt.Println("\nRate Limits:")
			if caps.RateLimit.RequestsPerMinute > 0 {
				fmt.Printf("  Requests per minute: %d\n", caps.RateLimit.RequestsPerMinute)
			}
			if caps.RateLimit.CharactersPerDay > 0 {
				fmt.Printf("  Characters per day: %d\n", caps.RateLimit.CharactersPerDay)
			}
		}

		fmt.Printf("\nSupported Languages (%d):\n", len(caps.SupportedLanguages))
		for i, lang := range caps.SupportedLanguages {
			fmt.Printf("  %s (%s)", lang.Code, lang.Name)
			if (i+1)%3 == 0 {
				fmt.Println()
			} else if i < len(caps.SupportedLanguages)-1 {
				fmt.Print(", ")
			}
		}
		fmt.Println()

		// 健康检查
		fmt.Print("\nHealth Check: ")
		if err := p.HealthCheck(context.Background()); err != nil {
			fmt.Printf("FAILED - %v\n", err)
		} else {
			fmt.Println("OK")
		}
	} else {
		fmt.Printf("%s does not implement full Provider interface\n", provider.GetName())
	}
}
