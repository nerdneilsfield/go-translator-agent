package cli

import (
	"fmt"
	"os"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/pkg/formats"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
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
)

// NewRootCommand 创建根命令
func NewRootCommand(version, commit, buildDate string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "translator [flags] input_file output_file",
		Short: "翻译工具是一个高质量、灵活的多语言翻译系统",
		Long: `翻译工具是一个高质量、灵活的多语言翻译系统，采用三步翻译流程来确保翻译质量。
该工具支持多种文件格式，可以为不同翻译阶段配置不同的语言模型，并提供完善的缓存机制以提高效率。`,
		Version: fmt.Sprintf("%s (commit %s, built %s)", version, commit, buildDate),
		Args: func(cmd *cobra.Command, args []string) error {
			// 对于特殊的标志命令，不需要参数
			if showVersion || listModels || listFormats || listStepSets || listCache {
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

			// 获取输入和输出文件路径
			inputPath := args[0]
			outputPath := args[1]

			if formatOnly {
				if err := formats.FormatFile(inputPath); err != nil {
					log.Error("格式化文件失败", zap.Error(err))
					os.Exit(1)
				}
				log.Info("格式化完成", zap.String("文件", inputPath))
				return
			}

			// 在翻译之前先格式化文件
			if err := formats.FormatFile(inputPath); err != nil {
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

			predefinedTranslations := &config.PredefinedTranslation{}

			if predefinedTranslationsPath != "" {
				predefinedTranslations, err = config.LoadPredefinedTranslations(predefinedTranslationsPath)
				if err != nil {
					log.Error("加载预定义翻译失败", zap.Error(err))
					os.Exit(1)
				}
			}

			// 使用命令行参数覆盖配置
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
				for _, ss := range cfg.StepSets {
					fmt.Printf("  - %s: %s\n", ss.ID, ss.Description)
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

			// 确保有足够的参数
			if len(args) < 2 {
				log.Error("缺少输入或输出文件参数")
				fmt.Println("使用方法: translator [flags] input_file output_file")
				os.Exit(1)
			}

			// 创建缓存目录（如果不存在）
			if cfg.UseCache {
				if err := os.MkdirAll(cfg.CacheDir, 0755); err != nil {
					log.Error("创建缓存目录失败", zap.Error(err))
					os.Exit(1)
				}
			}

			// 创建翻译器
			translatorOptions := []translator.Option{}

			// 如果强制刷新缓存，添加相应选项
			if forceCacheRefresh {
				log.Info("强制刷新缓存已启用")
				translatorOptions = append(translatorOptions, translator.WithForceCacheRefresh())
			}

			// 使用新的进度条系统
			// 创建一个带有进度条的翻译器选项
			translatorOptions = append(translatorOptions, translator.WithNewProgressBar())

			t, err := translator.New(cfg, translatorOptions...)
			if err != nil {
				log.Error("创建翻译器失败", zap.Error(err))
				os.Exit(1)
			}

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

	return rootCmd
}
