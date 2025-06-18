package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/formatfix"
	"github.com/nerdneilsfield/go-translator-agent/internal/formatfix/loader"
	"github.com/nerdneilsfield/go-translator-agent/internal/formatter"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/internal/translator"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	// 命令行标志变量
	cfgFile                    string
	sourceLang                 string
	targetLang                 string
	country                    string
	stepSet                    string
	formatType                 string
	useCache                   bool
	cacheDir                   string
	debugMode                  bool
	verboseMode                bool // 显示详细日志
	dryRun                     bool // 预演模式，只显示将要执行的操作
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
	showConfig   bool     // 显示当前配置

	// 翻译后处理相关标志
	enablePostProcessing      bool   // 启用翻译后处理
	glossaryPath              string // 词汇表文件路径
	contentProtection         bool   // 内容保护
	terminologyConsistency    bool   // 术语一致性检查
	mixedLanguageSpacing      bool   // 中英文混排空格优化
	machineTranslationCleanup bool   // 机器翻译痕迹清理

	// 格式修复相关标志
	enableFormatFix      bool // 启用格式修复
	formatFixInteractive bool // 交互式格式修复
	noPreTranslationFix  bool // 禁用翻译前修复
	noPostTranslationFix bool // 禁用翻译后修复
	noExternalTools      bool // 禁用外部工具
	listFormatFixers     bool // 列出格式修复器
	checkFormatOnly      bool // 仅检查格式，不修复
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
  - libretranslate: LibreTranslate (开源)
  - ollama: Ollama 本地大语言模型`,
		Version: fmt.Sprintf("%s (commit %s, built %s)", version, commit, buildDate),
		Args: func(cmd *cobra.Command, args []string) error {
			// 对于特殊的标志命令，不需要参数
			if showVersion || listModels || listFormats || listStepSets || listCache || listProviders() || listFormatFixers || showConfig {
				return nil
			}
			// 格式检查模式或预演模式需要至少一个参数
			if checkFormatOnly || dryRun {
				if len(args) < 1 {
					return fmt.Errorf("check-format-only or dry-run mode requires at least 1 file argument")
				}
				return nil
			}
			// 其他情况需要两个参数：输入文件和输出文件
			if len(args) != 2 {
				return fmt.Errorf("accepts 2 arg(s), received %d", len(args))
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			// 初始化临时日志（用于加载配置）
			tempLog := logger.NewLoggerWithVerbose(debugMode, verboseMode)
			defer func() {
				_ = tempLog.Sync()
			}()

			if showVersion {
				fmt.Printf("翻译工具 %s (commit %s, built %s)\n", version, commit, buildDate)
				return
			}

			// 列出可用的提供商
			if listProviders() {
				fmt.Println("支持的翻译提供商:")
				providers := []string{"openai", "deepl", "google", "deeplx", "libretranslate"}
				for _, p := range providers {
					fmt.Printf("  - %s\n", p)
				}
				return
			}

			// 处理其他列表命令
			if listModels || listStepSets || listFormats || listCache || listFormatFixers || showConfig {
				handleListCommands(cmd, args, tempLog)
				return
			}

			// 处理格式检查模式
			if checkFormatOnly {
				handleFormatCheckOnly(cmd, args, tempLog)
				return
			}

			// 处理预演模式
			if dryRun {
				handleDryRun(cmd, args, tempLog)
				return
			}

			// 获取输入和输出文件路径
			if len(args) < 2 {
				tempLog.Error("缺少输入或输出文件参数")
				fmt.Println("使用方法: translator [flags] input_file output_file")
				os.Exit(1)
			}

			inputPath := args[0]
			outputPath := args[1]

			// 创建格式化管理器
			formatterManager := formatter.NewManager(tempLog)

			if formatOnly {
				// 仅格式化文件
				_, err := formatterManager.FormatFile(inputPath, inputPath, nil)
				if err != nil {
					tempLog.Error("格式化文件失败", zap.Error(err))
					os.Exit(1)
				}
				tempLog.Info("格式化完成", zap.String("文件", inputPath))
				return
			}

			// 在翻译之前先格式化文件
			_, err := formatterManager.FormatFile(inputPath, inputPath, nil)
			if err != nil {
				tempLog.Error("文件格式化失败，无法继续翻译",
					zap.String("文件", inputPath),
					zap.Error(err))
				os.Exit(1)
			}
			tempLog.Info("文件格式化完成", zap.String("文件", inputPath))

			// 加载配置
			cfg, err := config.LoadConfig(cfgFile)
			if err != nil {
				tempLog.Error("加载配置失败", zap.Error(err))
				os.Exit(1)
			}

			// 根据配置创建详细日志
			detailedLogConfig := logger.DetailedLogConfig{
				EnableDetailedLog: cfg.EnableDetailedLog,
				LogLevel:          cfg.LogLevel,
				ConsoleLogLevel:   cfg.ConsoleLogLevel,
				NormalLogFile:     cfg.NormalLogFile,
				DetailedLogFile:   cfg.DetailedLogFile,
				Debug:             cfg.Debug || debugMode,
				Verbose:           cfg.Verbose || verboseMode,
			}

			loggerWrapper := logger.NewDetailedLogger(detailedLogConfig)
			log := loggerWrapper.GetZapLogger()
			defer func() {
				_ = log.Sync()
			}()

			// 处理预定义翻译（暂时不使用）
			if predefinedTranslationsPath != "" {
				_, err = config.LoadPredefinedTranslations(predefinedTranslationsPath)
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
				if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
					log.Error("创建缓存目录失败", zap.Error(err))
					os.Exit(1)
				}
			}

			// 使用 Translation Coordinator 进行翻译
			log.Info("使用 Translation Coordinator")

			// 使用与 stats 命令一致的路径
			progressPath := cfg.CacheDir
			if progressPath == "" {
				progressPath = "/tmp/.translator-progress"
			}

			coordinator, err := translator.NewTranslationCoordinator(cfg, log, progressPath)
			if err != nil {
				log.Error("创建 Translation Coordinator 失败", zap.Error(err))
				os.Exit(1)
			}

			// 如果启用流式输出，设置相关配置
			if streamOutput {
				log.Info("流式输出已启用")
				// TODO: 实现流式输出支持
			}

			// 直接使用 coordinator 翻译文件
			ctx := cmd.Context()
			result, err := coordinator.TranslateFile(ctx, inputPath, outputPath)
			if err != nil {
				log.Error("翻译文件失败", zap.Error(err))
				os.Exit(1)
			}

			// 显示翻译结果
			log.Info("翻译完成",
				zap.String("输入文件", result.InputFile),
				zap.String("输出文件", result.OutputFile),
				// zap.String("源语言", result.SourceLanguage),
				// zap.String("目标语言", result.TargetLanguage),
				zap.Int("总节点", result.TotalNodes),
				zap.Int("完成节点", result.CompletedNodes),
				zap.Int("失败节点", result.FailedNodes),
				zap.Float64("进度", result.Progress),
				zap.Duration("耗时", result.Duration),
			)
		},
	}

	// 添加全局标志
	addGlobalFlags(rootCmd)

	// 添加新的标志
	rootCmd.PersistentFlags().StringVar(&provider, "provider", "", "指定翻译提供商 (openai, deepl, google, deeplx, libretranslate)")
	rootCmd.PersistentFlags().BoolVar(&streamOutput, "stream", false, "启用流式输出 (实时显示翻译进度)")
	rootCmd.PersistentFlags().StringSliceVar(&providers, "list-providers", nil, "列出支持的翻译提供商")
	rootCmd.PersistentFlags().BoolVar(&showConfig, "show-config", false, "显示当前配置信息")

	// 添加子命令
	rootCmd.AddCommand(NewStatsCommand())
	rootCmd.AddCommand(NewFormatCommand())

	return rootCmd
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

	if listFormatFixers {
		handleListFormatFixers(log)
		return
	}

	if showConfig {
		handleShowConfig(cmd, cfg, log)
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

		// 显示步骤集
		if cfg.StepSets != nil && len(cfg.StepSets) > 0 {
			fmt.Println("\n步骤集配置:")
			for _, ss := range cfg.StepSets {
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
		// 创建格式化管理器来获取支持的格式
		formatterManager := formatter.NewManager(log)
		formatMap := formatterManager.ListAvailableFormatters()

		if len(formatMap) == 0 {
			// 如果没有注册的格式化器，显示默认支持的格式
			formats := []string{"markdown", "text", "html", "epub"}
			for _, format := range formats {
				fmt.Printf("  - %s\n", format)
			}
		} else {
			for format, formatters := range formatMap {
				fmt.Printf("  - %s\n", format)
				for _, formatter := range formatters {
					fmt.Printf("    * %s\n", formatter)
				}
			}
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
	if cmd.Flags().Changed("refresh-cache") {
		cfg.RefreshCache = forceCacheRefresh
	}
	if cmd.Flags().Changed("debug") {
		cfg.Debug = debugMode
	}
	if cmd.Flags().Changed("verbose") {
		cfg.Verbose = verboseMode
	}
	if cmd.Flags().Changed("no-post-process") {
		cfg.PostProcessMarkdown = !noPostProcess
	}

	// 格式修复相关配置更新
	if cmd.Flags().Changed("format-fix") {
		cfg.EnableFormatFix = enableFormatFix
	}
	if cmd.Flags().Changed("format-fix-interactive") {
		cfg.FormatFixInteractive = formatFixInteractive
	}
	if cmd.Flags().Changed("no-pre-fix") {
		cfg.PreTranslationFix = !noPreTranslationFix
	}
	if cmd.Flags().Changed("no-post-fix") {
		cfg.PostTranslationFix = !noPostTranslationFix
	}
	if cmd.Flags().Changed("no-external-tools") {
		cfg.UseExternalTools = !noExternalTools
	}

	// 翻译后处理相关配置更新
	if cmd.Flags().Changed("enable-post-processing") {
		cfg.EnablePostProcessing = enablePostProcessing
	}
	if cmd.Flags().Changed("glossary") {
		cfg.GlossaryPath = glossaryPath
	}
	if cmd.Flags().Changed("content-protection") {
		cfg.ContentProtection = contentProtection
	}
	if cmd.Flags().Changed("terminology-consistency") {
		cfg.TerminologyConsistency = terminologyConsistency
	}
	if cmd.Flags().Changed("mixed-language-spacing") {
		cfg.MixedLanguageSpacing = mixedLanguageSpacing
	}
	if cmd.Flags().Changed("mt-cleanup") {
		cfg.MachineTranslationCleanup = machineTranslationCleanup
	}
}

// updateConfigForProvider 根据指定的提供商更新配置
func updateConfigForProvider(cfg *config.Config, provider string) {
	// 创建一个临时的步骤集来使用指定的提供商
	providerStepSet := fmt.Sprintf("provider_%s", provider)

	// 根据提供商类型设置默认模型
	modelName := getDefaultModelForProvider(provider)

	// 创建新的步骤集
	cfg.StepSets[providerStepSet] = config.StepSetConfigV2{
		ID:          providerStepSet,
		Name:        fmt.Sprintf("使用 %s 提供商", provider),
		Description: fmt.Sprintf("使用 %s 提供商进行翻译", provider),
		Steps: []config.StepConfigV2{
			{
				Name:            "initial_translation",
				Provider:        provider,
				ModelName:       modelName,
				Temperature:     0.5,
				MaxTokens:       4096,
				AdditionalNotes: "Translate accurately while maintaining meaning and tone.",
			},
			{
				Name:            "reflection",
				Provider:        provider,
				ModelName:       modelName,
				Temperature:     0.3,
				MaxTokens:       2048,
				AdditionalNotes: "Review the translation and identify any issues.",
			},
			{
				Name:            "improvement",
				Provider:        provider,
				ModelName:       modelName,
				Temperature:     0.5,
				MaxTokens:       4096,
				AdditionalNotes: "Improve the translation based on feedback.",
			},
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
	rootCmd.PersistentFlags().BoolVarP(&verboseMode, "verbose", "v", false, "显示详细日志（包括翻译片段）")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "预演模式，只显示将要执行的操作，不实际进行翻译")
	rootCmd.PersistentFlags().BoolVar(&showVersion, "version", false, "显示版本信息")
	rootCmd.PersistentFlags().BoolVar(&listModels, "list-models", false, "列出支持的模型")
	rootCmd.PersistentFlags().BoolVar(&listFormats, "list-formats", false, "列出支持的文件格式")
	rootCmd.PersistentFlags().BoolVar(&listStepSets, "list-step-sets", false, "列出可用的步骤集")
	rootCmd.PersistentFlags().BoolVar(&listCache, "list-cache", false, "列出缓存文件")
	rootCmd.PersistentFlags().BoolVar(&formatOnly, "format-only", false, "仅格式化文件，不进行翻译")
	rootCmd.PersistentFlags().BoolVar(&noPostProcess, "no-post-process", false, "禁用翻译后的Markdown后处理")
	rootCmd.PersistentFlags().StringVar(&predefinedTranslationsPath, "predefined-translations", "", "预定义的翻译文件路径")

	// 格式修复相关标志
	rootCmd.PersistentFlags().BoolVar(&enableFormatFix, "format-fix", true, "启用格式修复")
	rootCmd.PersistentFlags().BoolVar(&formatFixInteractive, "format-fix-interactive", false, "启用交互式格式修复")
	rootCmd.PersistentFlags().BoolVar(&noPreTranslationFix, "no-pre-fix", false, "禁用翻译前格式修复")
	rootCmd.PersistentFlags().BoolVar(&noPostTranslationFix, "no-post-fix", false, "禁用翻译后格式修复")
	rootCmd.PersistentFlags().BoolVar(&noExternalTools, "no-external-tools", false, "禁用外部工具（如markdownlint、prettier）")
	rootCmd.PersistentFlags().BoolVar(&listFormatFixers, "list-format-fixers", false, "列出可用的格式修复器")
	rootCmd.PersistentFlags().BoolVar(&checkFormatOnly, "check-format-only", false, "仅检查格式问题，不进行修复或翻译")

	// 翻译后处理相关标志
	rootCmd.PersistentFlags().BoolVar(&enablePostProcessing, "enable-post-processing", false, "启用翻译后处理")
	rootCmd.PersistentFlags().StringVar(&glossaryPath, "glossary", "", "词汇表文件路径")
	rootCmd.PersistentFlags().BoolVar(&contentProtection, "content-protection", true, "启用内容保护（URL、代码等）")
	rootCmd.PersistentFlags().BoolVar(&terminologyConsistency, "terminology-consistency", true, "启用术语一致性检查")
	rootCmd.PersistentFlags().BoolVar(&mixedLanguageSpacing, "mixed-language-spacing", true, "启用中英文混排空格优化")
	rootCmd.PersistentFlags().BoolVar(&machineTranslationCleanup, "mt-cleanup", true, "启用机器翻译痕迹清理")
}

// handleListFormatFixers 处理列出格式修复器命令
func handleListFormatFixers(log *zap.Logger) {
	registry, err := loader.CreateRegistry(log)
	if err != nil {
		log.Error("failed to create format fix registry", zap.Error(err))
		fmt.Println("错误：无法创建格式修复器注册中心")
		os.Exit(1)
	}

	fmt.Println("可用的格式修复器:")
	stats := registry.GetStats()

	if fixerInfo, ok := stats["fixer_info"].(map[string][]string); ok {
		for name, formats := range fixerInfo {
			fmt.Printf("  - %s\n", name)
			fmt.Printf("    支持格式: %s\n", strings.Join(formats, ", "))
		}
	}

	fmt.Printf("\n支持的格式总览: %s\n", strings.Join(registry.GetSupportedFormats(), ", "))

	// 检查外部工具可用性
	fmt.Println("\n外部工具可用性:")
	toolManager := formatfix.NewDefaultToolManager(log)
	tools := []string{"markdownlint", "prettier", "htmlhint"}
	for _, tool := range tools {
		status := "❌ 不可用"
		if toolManager.IsToolAvailable(tool) {
			if version, err := toolManager.GetToolVersion(tool); err == nil {
				status = fmt.Sprintf("✅ 可用 (%s)", version)
			} else {
				status = "✅ 可用"
			}
		}
		fmt.Printf("  - %s: %s\n", tool, status)
	}
}

// handleFormatCheckOnly 处理仅检查格式命令
func handleFormatCheckOnly(cmd *cobra.Command, args []string, log *zap.Logger) {
	// 加载配置
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		cfg = config.NewDefaultConfig()
	}

	// 更新配置
	updateConfigFromFlags(cmd, cfg)

	// 创建格式修复器注册中心
	registry, err := loader.CreateRegistry(log)
	if err != nil {
		log.Error("failed to create format fix registry", zap.Error(err))
		fmt.Println("错误：无法创建格式修复器注册中心")
		os.Exit(1)
	}

	// 检查每个输入文件
	for _, filePath := range args {
		fmt.Printf("\n检查文件: %s\n", filePath)
		fmt.Println(strings.Repeat("=", 50))

		// 读取文件
		content, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("错误：无法读取文件 %s: %v\n", filePath, err)
			continue
		}

		// 检测文件格式
		format := detectFileFormat(filePath)
		fmt.Printf("检测到格式: %s\n", format)

		// 检查是否支持此格式
		if !registry.IsFormatSupported(format) {
			fmt.Printf("警告：不支持的格式 %s\n", format)
			continue
		}

		// 获取修复器并检查问题
		fixer, err := registry.GetFixerForFormat(format)
		if err != nil {
			fmt.Printf("错误：无法获取格式修复器: %v\n", err)
			continue
		}

		issues, err := fixer.CheckIssues(content)
		if err != nil {
			fmt.Printf("错误：检查格式问题失败: %v\n", err)
			continue
		}

		if len(issues) == 0 {
			fmt.Println("✅ 未发现格式问题")
		} else {
			fmt.Printf("发现 %d 个格式问题:\n", len(issues))

			// 按严重性分组
			severityGroups := make(map[formatfix.Severity][]*formatfix.FixIssue)
			for _, issue := range issues {
				severityGroups[issue.Severity] = append(severityGroups[issue.Severity], issue)
			}

			// 按严重性显示
			for _, severity := range []formatfix.Severity{formatfix.SeverityCritical, formatfix.SeverityError, formatfix.SeverityWarning, formatfix.SeverityInfo} {
				if issues := severityGroups[severity]; len(issues) > 0 {
					fmt.Printf("\n%s (%d个):\n", getSeverityIcon(severity), len(issues))
					for _, issue := range issues {
						fmt.Printf("  行%d列%d: [%s] %s\n", issue.Line, issue.Column, issue.Type, issue.Message)
						if issue.Suggestion != "" {
							fmt.Printf("    建议: %s\n", issue.Suggestion)
						}
					}
				}
			}
		}
	}
}

// detectFileFormat 检测文件格式（从coordinator.go复制）
func detectFileFormat(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".md", ".markdown":
		return "markdown"
	case ".txt":
		return "text"
	case ".html", ".htm":
		return "html"
	case ".epub":
		return "epub"
	case ".tex":
		return "latex"
	default:
		return "text"
	}
}

// getSeverityIcon 获取严重性图标
func getSeverityIcon(severity formatfix.Severity) string {
	switch severity {
	case formatfix.SeverityCritical:
		return "🔴 严重"
	case formatfix.SeverityError:
		return "🟠 错误"
	case formatfix.SeverityWarning:
		return "🟡 警告"
	case formatfix.SeverityInfo:
		return "🔵 信息"
	default:
		return "⚪ 未知"
	}
}

// handleShowConfig 显示当前配置信息
func handleShowConfig(cmd *cobra.Command, cfg *config.Config, log *zap.Logger) {
	// 先应用命令行参数覆盖
	updateConfigFromFlags(cmd, cfg)

	fmt.Println("🔧 当前配置信息")
	fmt.Println(strings.Repeat("=", 60))

	// 基本翻译配置
	fmt.Println("\n📋 基本翻译配置:")
	fmt.Printf("  源语言: %s\n", cfg.SourceLang)
	fmt.Printf("  目标语言: %s\n", cfg.TargetLang)
	fmt.Printf("  国家/地区: %s\n", cfg.Country)
	fmt.Printf("  活动步骤集: %s\n", cfg.ActiveStepSet)

	// 显示命令行参数覆盖
	overrides := []string{}
	if cmd.Flags().Changed("source") {
		overrides = append(overrides, fmt.Sprintf("源语言: %s", sourceLang))
	}
	if cmd.Flags().Changed("target") {
		overrides = append(overrides, fmt.Sprintf("目标语言: %s", targetLang))
	}
	if cmd.Flags().Changed("country") {
		overrides = append(overrides, fmt.Sprintf("国家/地区: %s", country))
	}
	if cmd.Flags().Changed("step-set") {
		overrides = append(overrides, fmt.Sprintf("步骤集: %s", stepSet))
	}

	if len(overrides) > 0 {
		fmt.Println("\n🔄 命令行参数覆盖:")
		for _, override := range overrides {
			fmt.Printf("  ✓ %s\n", override)
		}
	}

	// 缓存配置
	fmt.Println("\n💾 缓存配置:")
	fmt.Printf("  启用缓存: %t\n", cfg.UseCache)
	fmt.Printf("  缓存目录: %s\n", cfg.CacheDir)

	// 格式修复配置
	fmt.Println("\n🛠️  格式修复配置:")
	fmt.Printf("  启用格式修复: %t\n", cfg.EnableFormatFix)
	fmt.Printf("  交互式修复: %t\n", cfg.FormatFixInteractive)
	fmt.Printf("  翻译前修复: %t\n", cfg.PreTranslationFix)
	fmt.Printf("  翻译后修复: %t\n", cfg.PostTranslationFix)
	fmt.Printf("  使用外部工具: %t\n", cfg.UseExternalTools)

	// 翻译后处理配置
	fmt.Println("\n🔧 翻译后处理配置:")
	fmt.Printf("  启用后处理: %t\n", cfg.EnablePostProcessing)
	if cfg.GlossaryPath != "" {
		fmt.Printf("  词汇表路径: %s\n", cfg.GlossaryPath)
	} else {
		fmt.Printf("  词汇表路径: 未设置\n")
	}
	fmt.Printf("  内容保护: %t\n", cfg.ContentProtection)
	fmt.Printf("  术语一致性: %t\n", cfg.TerminologyConsistency)
	fmt.Printf("  中英文空格优化: %t\n", cfg.MixedLanguageSpacing)
	fmt.Printf("  机器翻译清理: %t\n", cfg.MachineTranslationCleanup)

	// 性能配置
	fmt.Println("\n⚡ 性能配置:")
	fmt.Printf("  块大小: %d\n", cfg.ChunkSize)
	fmt.Printf("  并行度: %d\n", cfg.Concurrency)
	fmt.Printf("  重试次数: %d\n", cfg.RetryAttempts)
	fmt.Printf("  请求超时: %d 秒\n", cfg.RequestTimeout)

	// 模型配置
	fmt.Println("\n🤖 模型配置:")
	if len(cfg.ModelConfigs) > 0 {
		for name, model := range cfg.ModelConfigs {
			fmt.Printf("  - %s (%s)\n", name, model.APIType)
			fmt.Printf("    Base URL: %s\n", model.BaseURL)
			fmt.Printf("    最大输出令牌: %d\n", model.MaxOutputTokens)
			fmt.Printf("    温度: %.2f\n", model.Temperature)
		}
	} else {
		fmt.Println("  未配置任何模型")
	}

	// 步骤集配置
	fmt.Println("\n📝 活动步骤集详情:")
	activeStepSetID := cfg.ActiveStepSet

	// 检查新格式步骤集
	if stepSet, exists := cfg.StepSets[activeStepSetID]; exists {
		fmt.Printf("  名称: %s\n", stepSet.Name)
		fmt.Printf("  描述: %s\n", stepSet.Description)
		fmt.Printf("  快速模式阈值: %d 字符\n", stepSet.FastModeThreshold)
		fmt.Printf("  步骤数量: %d\n", len(stepSet.Steps))

		for i, step := range stepSet.Steps {
			fmt.Printf("    步骤 %d - %s:\n", i+1, step.Name)
			fmt.Printf("      提供商: %s\n", step.Provider)
			fmt.Printf("      模型: %s\n", step.ModelName)
			fmt.Printf("      温度: %.2f\n", step.Temperature)
			if step.MaxTokens > 0 {
				fmt.Printf("      最大令牌: %d\n", step.MaxTokens)
			}
		}
	} else if stepSet, exists := cfg.StepSets[activeStepSetID]; exists {
		// 检查旧格式步骤集
		fmt.Printf("  名称: %s (传统格式)\n", stepSet.Name)
		fmt.Printf("  描述: %s\n", stepSet.Description)
		fmt.Printf("  快速模式阈值: %d 字符\n", stepSet.FastModeThreshold)
		fmt.Printf("  初始翻译模型: %s\n", stepSet.InitialTranslation.ModelName)
		fmt.Printf("  反思模型: %s\n", stepSet.Reflection.ModelName)
		fmt.Printf("  改进模型: %s\n", stepSet.Improvement.ModelName)
	} else {
		fmt.Printf("  ⚠️  警告: 未找到活动步骤集 '%s'\n", activeStepSetID)
	}

	// 其他配置
	fmt.Println("\n🔧 其他配置:")
	fmt.Printf("  调试模式: %t\n", cfg.Debug)
	fmt.Printf("  保留数学公式: %t\n", cfg.PreserveMath)
	fmt.Printf("  翻译图表标题: %t\n", cfg.TranslateFigureCaptions)
	fmt.Printf("  修复数学公式: %t\n", cfg.FixMathFormulas)
	fmt.Printf("  修复表格格式: %t\n", cfg.FixTableFormat)
}

// handleDryRun 处理预演模式
func handleDryRun(cmd *cobra.Command, args []string, log *zap.Logger) {
	inputFile := args[0]

	// 加载配置
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		log.Error("加载配置失败", zap.Error(err))
		// 使用默认配置作为回退
		cfg = config.NewDefaultConfig()
	}

	// 应用命令行覆盖
	updateConfigFromFlags(cmd, cfg)

	fmt.Println("🎭 预演模式 - 显示将要执行的操作")
	fmt.Println("============================================================")

	// 显示输入文件信息
	fmt.Printf("📄 输入文件: %s\n", inputFile)

	// 检查文件是否存在
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		fmt.Printf("❌ 错误: 输入文件不存在\n")
		return
	}

	// 获取文件信息
	fileInfo, err := os.Stat(inputFile)
	if err != nil {
		fmt.Printf("❌ 错误: 无法获取文件信息: %v\n", err)
		return
	}

	fmt.Printf("📏 文件大小: %d 字节\n", fileInfo.Size())

	// 显示输出文件
	outputFile := ""
	if len(args) > 1 {
		outputFile = args[1]
	} else {
		// 生成默认输出文件名
		outputFile = generateDefaultOutputFile(inputFile)
	}
	fmt.Printf("📝 输出文件: %s\n", outputFile)

	// 显示翻译配置
	fmt.Printf("\n🔧 翻译配置:\n")
	fmt.Printf("  源语言: %s\n", cfg.SourceLang)
	fmt.Printf("  目标语言: %s\n", cfg.TargetLang)
	fmt.Printf("  国家/地区: %s\n", cfg.Country)
	fmt.Printf("  活动步骤集: %s\n", cfg.ActiveStepSet)

	// 显示步骤集详情
	if stepSet, exists := cfg.StepSets[cfg.ActiveStepSet]; exists {
		fmt.Printf("\n📋 步骤集详情: %s\n", stepSet.Name)
		fmt.Printf("  描述: %s\n", stepSet.Description)
		fmt.Printf("  步骤数量: %d\n", len(stepSet.Steps))

		for i, step := range stepSet.Steps {
			fmt.Printf("    步骤 %d - %s:\n", i+1, step.Name)
			fmt.Printf("      提供商: %s\n", step.Provider)
			fmt.Printf("      模型: %s\n", step.ModelName)
			fmt.Printf("      温度: %.2f\n", step.Temperature)
			if step.MaxTokens > 0 {
				fmt.Printf("      最大令牌: %d\n", step.MaxTokens)
			}
		}
	} else {
		fmt.Printf("⚠️ 警告: 步骤集 '%s' 未找到\n", cfg.ActiveStepSet)
	}

	// 显示处理配置
	fmt.Printf("\n⚡ 处理配置:\n")
	fmt.Printf("  并行度: %d\n", cfg.Concurrency)
	fmt.Printf("  块大小: %d 字符\n", cfg.ChunkSize)
	fmt.Printf("  使用缓存: %t\n", cfg.UseCache)
	if cfg.UseCache {
		fmt.Printf("  缓存目录: %s\n", cfg.CacheDir)
	}

	// 显示格式修复配置
	if cfg.EnableFormatFix {
		fmt.Printf("\n🔧 格式修复:\n")
		fmt.Printf("  翻译前修复: %t\n", cfg.PreTranslationFix)
		fmt.Printf("  翻译后修复: %t\n", cfg.PostTranslationFix)
		fmt.Printf("  交互式修复: %t\n", cfg.FormatFixInteractive)
	}

	fmt.Printf("\n✅ 预演完成 - 使用相同参数但不加 --dry-run 来执行实际翻译\n")
}

// generateDefaultOutputFile 生成默认输出文件名
func generateDefaultOutputFile(inputFile string) string {
	ext := filepath.Ext(inputFile)
	base := strings.TrimSuffix(inputFile, ext)
	return base + "_translated" + ext
}
