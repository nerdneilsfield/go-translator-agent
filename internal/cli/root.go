package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/adapter"
	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/pkg/formats"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	// 原有的标志（从 root_old.go）
	cfgFile                    string
	sourceLang                 string
	targetLang                 string
	country                    string
	stepSet                    string
	formatType                 string
	useCache                   bool
	cacheDir                   string
	debugMode                  bool
	showVersion                bool
	listModels                 bool
	listFormats                bool
	listStepSets               bool
	forceCacheRefresh          bool
	listCache                  bool
	formatOnly                 bool
	noPostProcess              bool
	predefinedTranslationsPath string
	
	// 新增的标志
	provider     string   // 指定翻译提供商
	streamOutput bool     // 启用流式输出
	providers    []string // 可用的提供商列表
)

// NewRootCommand 创建根命令（默认使用新适配器）
func NewRootCommand(version, commit, buildDate string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "translator [flags] input_file output_file",
		Short: "翻译工具是一个高质量、灵活的多语言翻译系统",
		Long: `翻译工具是一个高质量、灵活的多语言翻译系统，采用三步翻译流程来确保翻译质量。
该工具支持多种文件格式，可以为不同翻译阶段配置不同的语言模型，并提供完善的缓存机制以提高效率。

支持的翻译提供商:
  - openai: OpenAI GPT 模型
  - deepl: DeepL 专业翻译
  - google: Google Translate
  - deeplx: DeepLX (免费 DeepL 替代)
  - libretranslate: LibreTranslate (开源)`,
		Version: fmt.Sprintf("%s (commit %s, built %s)", version, commit, buildDate),
		Args: func(cmd *cobra.Command, args []string) error {
			// 对于特殊的标志命令，不需要参数
			if showVersion || listModels || listFormats || listStepSets || listCache || listProviders() {
				return nil
			}
			// 其他情况需要两个参数：输入文件和输出文件
			if len(args) != 2 {
				return fmt.Errorf("accepts 2 arg(s), received %d", len(args))
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			// 初始化日志
			log := logger.NewLogger(debugMode)
			defer func() {
				_ = log.Sync()
			}()

			if showVersion {
				fmt.Printf("翻译工具 %s (commit %s, built %s)\n", version, commit, buildDate)
				return
			}

			// 列出可用的提供商
			if listProviders() {
				fmt.Println("支持的翻译提供商:")
				factory := adapter.NewProviderFactory(nil)
				for _, p := range factory.GetAvailableProviders() {
					fmt.Printf("  - %s\n", p)
				}
				return
			}

			// 处理其他列表命令
			if listModels || listStepSets || listFormats || listCache {
				handleListCommands(cmd, args, log)
				return
			}

			// 获取输入和输出文件路径
			if len(args) < 2 {
				log.Error("缺少输入或输出文件参数")
				fmt.Println("使用方法: translator [flags] input_file output_file")
				os.Exit(1)
			}

			inputPath := args[0]
			outputPath := args[1]

			if formatOnly {
				if err := formats.FormatFile(inputPath, log); err != nil {
					log.Error("格式化文件失败", zap.Error(err))
					os.Exit(1)
				}
				log.Info("格式化完成", zap.String("文件", inputPath))
				return
			}

			// 在翻译之前先格式化文件
			if err := formats.FormatFile(inputPath, log); err != nil {
				log.Error("文件格式化失败，无法继续翻译",
					zap.String("文件", inputPath),
					zap.Error(err))
				os.Exit(1)
			}
			log.Info("文件格式化完成", zap.String("文件", inputPath))

			// 加载配置
			cfg, err := config.LoadConfig(cfgFile)
			if err != nil {
				log.Error("加载配置失败", zap.Error(err))
				os.Exit(1)
			}

			// 处理预定义翻译
			predefinedTranslations := &config.PredefinedTranslation{}
			if predefinedTranslationsPath != "" {
				predefinedTranslations, err = config.LoadPredefinedTranslations(predefinedTranslationsPath)
				if err != nil {
					log.Error("加载预定义翻译失败", zap.Error(err))
					os.Exit(1)
				}
			}

			// 使用命令行参数覆盖配置
			updateConfigFromFlags(cmd, cfg)

			// 如果指定了提供商，更新配置
			if provider != "" {
				log.Info("使用指定的翻译提供商", zap.String("provider", provider))
				// 可以在这里设置特定的步骤集或模型配置来使用指定的提供商
				updateConfigForProvider(cfg, provider)
			}

			// 创建缓存目录（如果不存在）
			if cfg.UseCache {
				if err := os.MkdirAll(cfg.CacheDir, 0755); err != nil {
					log.Error("创建缓存目录失败", zap.Error(err))
					os.Exit(1)
				}
			}

			// 使用适配器创建翻译器
			var t translator.Translator
			if useAdapter() {
				log.Info("使用新的翻译适配器")
				// 使用默认适配器工厂
				t, err = adapter.CreateDefaultTranslatorAdapter(cfg)
				if err != nil {
					log.Error("创建翻译适配器失败", zap.Error(err))
					os.Exit(1)
				}
			} else {
				// 向后兼容：使用旧的翻译器
				translatorOptions := []translator.Option{}
				if forceCacheRefresh {
					log.Info("强制刷新缓存已启用")
					translatorOptions = append(translatorOptions, translator.WithForceCacheRefresh())
				}
				translatorOptions = append(translatorOptions, translator.WithNewProgressBar())
				
				t, err = translator.New(cfg, translatorOptions...)
				if err != nil {
					log.Error("创建翻译器失败", zap.Error(err))
					os.Exit(1)
				}
			}

			// 初始化翻译器
			t.InitTranslator()
			defer t.Finish()

			// 获取文件处理器
			var processor formats.Processor
			if formatType != "" {
				// 使用指定格式
				processor, err = formats.NewProcessor(t, formatType, predefinedTranslations, nil)
			} else {
				// 根据文件扩展名自动检测格式
				processor, err = formats.ProcessorFromFilePath(t, inputPath, predefinedTranslations, nil)
			}

			if err != nil {
				log.Error("创建文件处理器失败", zap.Error(err))
				os.Exit(1)
			}

			// 如果启用流式输出，设置相关配置
			if streamOutput {
				log.Info("流式输出已启用")
				// TODO: 实现流式输出支持
			}

			if err := processor.TranslateFile(inputPath, outputPath); err != nil {
				log.Error("翻译文件失败", zap.Error(err))
				os.Exit(1)
			}

			log.Info("翻译完成",
				zap.String("输入文件", inputPath),
				zap.String("输出文件", outputPath),
				zap.String("源语言", cfg.SourceLang),
				zap.String("目标语言", cfg.TargetLang),
				zap.String("步骤集", cfg.ActiveStepSet),
			)
		},
	}

	// 添加全局标志
	addGlobalFlags(rootCmd)

	// 添加新的标志
	rootCmd.PersistentFlags().StringVar(&provider, "provider", "", "指定翻译提供商 (openai, deepl, google, deeplx, libretranslate)")
	rootCmd.PersistentFlags().BoolVar(&streamOutput, "stream", false, "启用流式输出 (实时显示翻译进度)")
	rootCmd.PersistentFlags().StringSliceVar(&providers, "list-providers", nil, "列出支持的翻译提供商")

	return rootCmd
}

// useAdapter 判断是否使用新的适配器
func useAdapter() bool {
	// 可以通过环境变量控制
	if os.Getenv("USE_NEW_TRANSLATOR") == "false" {
		return false
	}
	// 默认使用新的适配器
	return true
}

// listProviders 检查是否需要列出提供商
func listProviders() bool {
	return len(providers) > 0 || os.Getenv("LIST_PROVIDERS") == "true"
}

// handleListCommands 处理各种列表命令
func handleListCommands(cmd *cobra.Command, args []string, log *zap.Logger) {
	// 加载配置以获取信息
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		// 使用默认配置
		cfg = config.NewDefaultConfig()
	}

	if listCache {
		// 显示缓存目录中的文件
		files, err := os.ReadDir(cfg.CacheDir)
		if err != nil {
			log.Error("读取缓存目录失败", zap.Error(err))
			os.Exit(1)
		}
		fmt.Printf("缓存目录 (%s) 中的文件:\n", cfg.CacheDir)
		for _, file := range files {
			info, err := file.Info()
			if err != nil {
				continue
			}
			fmt.Printf("  - %s (大小: %d 字节)\n", file.Name(), info.Size())
		}
		return
	}

	if listModels {
		fmt.Println("支持的模型:")
		for _, model := range cfg.ModelConfigs {
			fmt.Printf("  - %s (%s)\n", model.Name, model.APIType)
		}
		return
	}

	if listStepSets {
		fmt.Println("可用的步骤集:")
		
		// 显示新格式的步骤集
		if cfg.StepSetsV2 != nil && len(cfg.StepSetsV2) > 0 {
			fmt.Println("\n新格式步骤集（支持多提供商）:")
			for _, ss := range cfg.StepSetsV2 {
				fmt.Printf("  - %s: %s\n", ss.ID, ss.Description)
				for i, step := range ss.Steps {
					fmt.Printf("      步骤 %d: %s (提供商: %s, 模型: %s)\n", 
						i+1, step.Name, step.Provider, step.ModelName)
				}
			}
		}
		
		// 显示旧格式的步骤集
		if len(cfg.StepSets) > 0 {
			fmt.Println("\n传统步骤集:")
			for _, ss := range cfg.StepSets {
				fmt.Printf("  - %s: %s\n", ss.ID, ss.Description)
			}
		}
		return
	}

	if listFormats {
		fmt.Println("支持的文件格式:")
		for _, format := range formats.RegisteredFormats() {
			fmt.Printf("  - %s\n", format)
		}
		return
	}
}

// updateConfigFromFlags 使用命令行参数更新配置
func updateConfigFromFlags(cmd *cobra.Command, cfg *config.Config) {
	if cmd.Flags().Changed("source") {
		cfg.SourceLang = sourceLang
	}
	if cmd.Flags().Changed("target") {
		cfg.TargetLang = targetLang
	}
	if cmd.Flags().Changed("country") {
		cfg.Country = country
	}
	if cmd.Flags().Changed("step-set") {
		cfg.ActiveStepSet = stepSet
	}
	if cmd.Flags().Changed("cache") {
		cfg.UseCache = useCache
	}
	if cmd.Flags().Changed("cache-dir") {
		cfg.CacheDir = cacheDir
	}
	if cmd.Flags().Changed("debug") {
		cfg.Debug = debugMode
	}
	if cmd.Flags().Changed("no-post-process") {
		cfg.PostProcessMarkdown = !noPostProcess
	}
}

// updateConfigForProvider 根据指定的提供商更新配置
func updateConfigForProvider(cfg *config.Config, provider string) {
	// 创建一个临时的步骤集来使用指定的提供商
	providerStepSet := fmt.Sprintf("provider_%s", provider)
	
	// 根据提供商类型设置默认模型
	modelName := getDefaultModelForProvider(provider)
	
	// 创建新的步骤集
	cfg.StepSets[providerStepSet] = config.StepSetConfig{
		ID:          providerStepSet,
		Name:        fmt.Sprintf("使用 %s 提供商", provider),
		Description: fmt.Sprintf("使用 %s 提供商进行翻译", provider),
		InitialTranslation: config.StepConfig{
			Name:        "初始翻译",
			ModelName:   modelName,
			Temperature: 0.5,
		},
		Reflection: config.StepConfig{
			Name:        "反思",
			ModelName:   modelName,
			Temperature: 0.3,
		},
		Improvement: config.StepConfig{
			Name:        "改进",
			ModelName:   modelName,
			Temperature: 0.5,
		},
		FastModeThreshold: 300,
	}
	
	// 设置为活动步骤集
	cfg.ActiveStepSet = providerStepSet
}

// getDefaultModelForProvider 获取提供商的默认模型
func getDefaultModelForProvider(provider string) string {
	switch strings.ToLower(provider) {
	case "openai":
		return "gpt-3.5-turbo"
	case "deepl":
		return "deepl"
	case "google":
		return "google-translate"
	case "deeplx":
		return "deeplx"
	case "libretranslate":
		return "libretranslate"
	default:
		return "gpt-3.5-turbo"
	}
}

// addGlobalFlags 添加全局标志
func addGlobalFlags(rootCmd *cobra.Command) {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径")
	rootCmd.PersistentFlags().StringVar(&sourceLang, "source", "", "源语言")
	rootCmd.PersistentFlags().StringVar(&targetLang, "target", "", "目标语言")
	rootCmd.PersistentFlags().StringVar(&country, "country", "", "目标语言国家/地区")
	rootCmd.PersistentFlags().StringVar(&stepSet, "step-set", "", "使用的步骤集")
	rootCmd.PersistentFlags().StringVar(&formatType, "format", "", "文件格式")
	rootCmd.PersistentFlags().BoolVar(&useCache, "cache", true, "是否使用缓存")
	rootCmd.PersistentFlags().StringVar(&cacheDir, "cache-dir", "", "缓存目录路径")
	rootCmd.PersistentFlags().BoolVar(&forceCacheRefresh, "refresh-cache", false, "强制刷新缓存")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "启用调试模式")
	rootCmd.PersistentFlags().BoolVar(&showVersion, "version", false, "显示版本信息")
	rootCmd.PersistentFlags().BoolVar(&listModels, "list-models", false, "列出支持的模型")
	rootCmd.PersistentFlags().BoolVar(&listFormats, "list-formats", false, "列出支持的文件格式")
	rootCmd.PersistentFlags().BoolVar(&listStepSets, "list-step-sets", false, "列出可用的步骤集")
	rootCmd.PersistentFlags().BoolVar(&listCache, "list-cache", false, "列出缓存文件")
	rootCmd.PersistentFlags().BoolVar(&formatOnly, "format-only", false, "仅格式化文件，不进行翻译")
	rootCmd.PersistentFlags().BoolVar(&noPostProcess, "no-post-process", false, "禁用翻译后的Markdown后处理")
	rootCmd.PersistentFlags().StringVar(&predefinedTranslationsPath, "predefined-translations", "", "预定义的翻译文件路径")
}