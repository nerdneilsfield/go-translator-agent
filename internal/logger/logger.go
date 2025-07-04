package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TraceLevel 定义 TRACE 日志级别，比 DEBUG 更详细
const TraceLevel zapcore.Level = -2

func NewLogger(debug bool) *zap.Logger {
	return NewLoggerWithPath(debug, false, "")
}

func NewLoggerWithVerbose(debug, verbose bool) *zap.Logger {
	return NewLoggerWithPath(debug, verbose, "")
}

// 你的回调函数类型
type LogCallback func(entry zapcore.Entry, fields []zapcore.Field)

// 自定义 Core，它会调用一个回调函数
type CallbackCore struct {
	zapcore.Core // 嵌入一个已有的 Core，以便委托部分工作
	callback     LogCallback
	callbackLock sync.Mutex // 如果回调不是线程安全的，可能需要锁
}

// NewCallbackCore 创建一个新的 CallbackCore
func NewCallbackCore(underlyingCore zapcore.Core, callback LogCallback) *CallbackCore {
	return &CallbackCore{
		Core:     underlyingCore, // 例如，你可以将 NewTee 创建的 core 传进来
		callback: callback,
	}
}

// With 返回一个新的 CallbackCore，其中包含添加的字段
func (cc *CallbackCore) With(fields []zapcore.Field) zapcore.Core {
	// 创建一个新的 CallbackCore，并传递新的 underlyingCore.With(fields)
	return NewCallbackCore(cc.Core.With(fields), cc.callback)
}

// Write 是核心方法，在这里调用回调
func (cc *CallbackCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	// 调用你的回调函数
	if cc.callback != nil {
		cc.callbackLock.Lock() // 保证回调的线程安全（如果需要）
		cc.callback(entry, fields)
		cc.callbackLock.Unlock()
	}

	// 调用嵌入的 Core 的 Write 方法来实际记录日志 (如果需要)
	// 如果你希望回调完全取代日志记录，可以不调用下面的行
	return cc.Core.Write(entry, fields)
}

// Check 仍然委托给嵌入的 Core
func (cc *CallbackCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	// 如果 Enabled 返回 true，则返回 ce.AddCore(ent, cc)
	// 这样 Write 方法才会被调用
	if cc.Enabled(ent.Level) {
		return ce.AddCore(ent, cc)
	}
	return ce
}

// 自定义 CallerEncoder，同时包含短路径、行号和函数名
func customCallerEncoder(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
	if !caller.Defined {
		enc.AppendString("undefined_caller")
		return
	}

	// caller.PC 已经是 zap.AddCaller() 帮你找到的正确的程序计数器
	fn := runtime.FuncForPC(caller.PC)
	if fn == nil {
		// 如果无法获取函数信息，回退到仅显示文件和行号
		enc.AppendString(caller.TrimmedPath()) // TrimmedPath() 返回 "path/file.go:line"
		return
	}

	// fn.Name() 返回 "pkg/path.FuncName" 或 "pkg/path.(ReceiverType).MethodName"
	fullFuncName := fn.Name()

	// 提取短函数名 (不包含包路径)
	shortFuncName := fullFuncName
	if lastSlash := strings.LastIndex(fullFuncName, "/"); lastSlash >= 0 {
		// 如果函数名是 "pkg/path.(ReceiverType).MethodName"
		// lastSlash 之后是 "(ReceiverType).MethodName" 或者 "FunctionName"
		// 我们需要进一步处理，只取最后的方法名或函数名
		pkgAndFunc := fullFuncName[lastSlash+1:]
		if dot := strings.Index(pkgAndFunc, "."); dot >= 0 && strings.Contains(pkgAndFunc, "(") && strings.HasPrefix(pkgAndFunc, "(") {
			// 可能是匿名函数或者闭包，尝试保持原样或简化
			// 示例：main.main.func1
			// 如果是 "(*pkg.MyType).MyMethod" 这样的，我们可能只想取 "MyMethod"
			// 这部分简化逻辑可能需要根据你的函数命名风格调整
			shortFuncName = pkgAndFunc // 保持 "pkg.FuncName" 或 "(*Receiver).MethodName" 形式
		} else if dot > 0 && !strings.Contains(pkgAndFunc, "(") { // "pkg.FuncName"
			shortFuncName = pkgAndFunc[dot+1:]
		} else {
			shortFuncName = pkgAndFunc // fallback
		}
	}
	// 如果 shortFuncName 仍然是包含括号的接收者方法，例如 "(*MyType).MyMethod"
	// 可以进一步简化
	if strings.HasPrefix(shortFuncName, "(*") && strings.Contains(shortFuncName, ").") {
		shortFuncName = shortFuncName[strings.Index(shortFuncName, ").")+2:]
	} else if strings.Contains(shortFuncName, ".") && !strings.HasPrefix(shortFuncName, "(") { // pkg.Function
		// 可能已经被上面的逻辑处理，或者保持为 pkg.Function
	}

	// 输出格式: file.go:line (FunctionName)
	enc.AppendString(fmt.Sprintf("%s:%d (%s)", filepath.Base(caller.File), caller.Line, shortFuncName))
}

// 示例回调函数
func myCustomLogCallback(entry zapcore.Entry, fields []zapcore.Field) {
	fmt.Printf("Level: %s, Time: %s, Message: %s, Caller: %s\n",
		entry.Level.String(),
		entry.Time.Format("2006-01-02 15:04:05"),
		entry.Message,
		entry.Caller.TrimmedPath(), // 如果 Caller 被正确设置
	)
	//if len(fields) > 0 {
	//	fmt.Println("  Fields:")
	//	for _, f := range fields {
	//		fmt.Printf("    %s: %v (Type: %T)\n", f.Key, f.Interface, f.Type)
	//	}
	//}
	fmt.Println("---")
}

// NewLoggerWithPath 创建一个新的日志记录器
func NewLoggerWithPath(debug, verbose bool, outputPath string) *zap.Logger {
	// 控制台输出配置（彩色）
	consoleConfig := zap.NewDevelopmentEncoderConfig()
	consoleConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	consoleConfig.EncodeCaller = customCallerEncoder

	// 文件输出配置（JSON）
	fileConfig := zap.NewProductionEncoderConfig()
	fileConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// 设置日志级别
	level := zapcore.InfoLevel
	if debug {
		level = zapcore.DebugLevel
	} else if verbose {
		// verbose 模式显示 DEBUG 级别，但只在控制台输出时
		level = zapcore.DebugLevel
	}

	enableFileOutput := false
	if outputPath != "" {
		enableFileOutput = true
	}

	// 创建编码器
	consoleEncoder := zapcore.NewConsoleEncoder(consoleConfig)
	var fileEncoder zapcore.Encoder
	if enableFileOutput {
		fileEncoder = zapcore.NewJSONEncoder(fileConfig)
	}

	// 创建输出目标
	consoleWriter := zapcore.AddSync(os.Stdout)

	var fileWriter zapcore.WriteSyncer
	if enableFileOutput {
		dir := filepath.Dir(outputPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			// 使用一个临时的基本logger来记录这个致命错误，因为我们的完整logger还没建好
			tempLogger, _ := zap.NewProduction()
			tempLogger.Fatal("创建日志输出目录失败", zap.Error(err))
			// panic(err) // Fatal 已经会 os.Exit(1)
		}
		file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
		if err != nil {
			tempLogger, _ := zap.NewProduction()
			tempLogger.Fatal("打开日志输出文件失败", zap.Error(err))
			// panic(err)
		}
		fileWriter = zapcore.AddSync(file)
	}

	// 创建基础的 cores (控制台和文件)
	var baseCores []zapcore.Core

	// 控制台输出级别
	consoleLevel := level

	// 文件输出级别（verbose 模式下文件仍然只记录 INFO 及以上）
	fileLevel := zapcore.InfoLevel
	if debug {
		fileLevel = zapcore.DebugLevel
	}

	baseCores = append(baseCores, zapcore.NewCore(consoleEncoder, consoleWriter, consoleLevel))
	if enableFileOutput {
		baseCores = append(baseCores, zapcore.NewCore(fileEncoder, fileWriter, fileLevel))
	}
	underlyingCore := zapcore.NewTee(baseCores...)

	// 检查是否需要包装CallbackCore（默认禁用以避免重复输出）
	// 如果需要启用debug回调，可以通过环境变量控制
	if os.Getenv("TRANSLATOR_DEBUG_CALLBACK") == "true" {
		callbackEnabledCore := NewCallbackCore(underlyingCore, myCustomLogCallback)
		return zap.New(callbackEnabledCore, zap.AddCaller())
	}

	return zap.New(underlyingCore, zap.AddCaller())
}

// Logger 接口定义了日志记录功能
type Logger interface {
	Trace(msg string, fields ...zapcore.Field) // 新增：详细日志级别
	Debug(msg string, fields ...zapcore.Field)
	Info(msg string, fields ...zapcore.Field)
	Warn(msg string, fields ...zapcore.Field)
	Error(msg string, fields ...zapcore.Field)
	Fatal(msg string, fields ...zapcore.Field)
	With(fields ...zapcore.Field) Logger
}

// ZapLogger 是 Logger 接口的 Zap 实现
type ZapLogger struct {
	logger *zap.Logger
}

// NewZapLogger 创建一个新的 ZapLogger 实例
func NewZapLogger(debug bool) *ZapLogger {
	return &ZapLogger{
		logger: NewLogger(debug),
	}
}

// NewZapLoggerWithVerbose 创建支持 verbose 模式的 ZapLogger 实例
func NewZapLoggerWithVerbose(debug, verbose bool) *ZapLogger {
	return &ZapLogger{
		logger: NewLoggerWithVerbose(debug, verbose),
	}
}

// Trace 记录详细级别的日志
func (l *ZapLogger) Trace(msg string, fields ...zapcore.Field) {
	if ce := l.logger.Check(TraceLevel, msg); ce != nil {
		ce.Write(fields...)
	}
}

// Debug 记录调试级别的日志
func (l *ZapLogger) Debug(msg string, fields ...zapcore.Field) {
	l.logger.Debug(msg, fields...)
}

// Info 记录信息级别的日志
func (l *ZapLogger) Info(msg string, fields ...zapcore.Field) {
	l.logger.Info(msg, fields...)
}

// Warn 记录警告级别的日志
func (l *ZapLogger) Warn(msg string, fields ...zapcore.Field) {
	l.logger.Warn(msg, fields...)
}

// Error 记录错误级别的日志
func (l *ZapLogger) Error(msg string, fields ...zapcore.Field) {
	l.logger.Error(msg, fields...)
}

// Fatal 记录致命级别的日志
func (l *ZapLogger) Fatal(msg string, fields ...zapcore.Field) {
	l.logger.Fatal(msg, fields...)
}

// With 返回带有附加字段的新 Logger
func (l *ZapLogger) With(fields ...zapcore.Field) Logger {
	return &ZapLogger{
		logger: l.logger.With(fields...),
	}
}

// GetZapLogger 返回底层的 zap.Logger
func (l *ZapLogger) GetZapLogger() *zap.Logger {
	return l.logger
}

// DetailedLogConfig 详细日志配置
type DetailedLogConfig struct {
	EnableDetailedLog bool   // 是否启用详细日志
	LogLevel          string // 基础日志级别 (trace/debug/info/warn/error)
	ConsoleLogLevel   string // 控制台日志级别
	NormalLogFile     string // 普通日志文件路径
	DetailedLogFile   string // 详细日志文件路径
	Debug             bool   // 调试模式
	Verbose           bool   // 详细模式
}

// NewDetailedLogger 创建支持详细日志的新Logger
func NewDetailedLogger(config DetailedLogConfig) *ZapLogger {
	logger := NewLoggerWithDetailedConfig(config)
	return &ZapLogger{
		logger: logger,
	}
}

// NewLoggerWithDetailedConfig 根据详细配置创建日志记录器
func NewLoggerWithDetailedConfig(config DetailedLogConfig) *zap.Logger {
	// 解析日志级别
	parseLevel := func(levelStr string) zapcore.Level {
		switch strings.ToLower(levelStr) {
		case "trace":
			return TraceLevel
		case "debug":
			return zapcore.DebugLevel
		case "info":
			return zapcore.InfoLevel
		case "warn", "warning":
			return zapcore.WarnLevel
		case "error":
			return zapcore.ErrorLevel
		default:
			return zapcore.InfoLevel
		}
	}

	// 控制台配置（彩色）
	consoleConfig := zap.NewDevelopmentEncoderConfig()
	consoleConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	consoleConfig.EncodeCaller = customCallerEncoder

	// 文件配置（JSON）
	fileConfig := zap.NewProductionEncoderConfig()
	fileConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// 设置各层级日志级别
	consoleLevel := parseLevel(config.ConsoleLogLevel)
	if config.ConsoleLogLevel == "" {
		consoleLevel = zapcore.InfoLevel // 控制台默认INFO级别
	}

	normalFileLevel := parseLevel(config.LogLevel)
	if config.Debug {
		normalFileLevel = zapcore.DebugLevel
	} else if config.Verbose {
		normalFileLevel = zapcore.DebugLevel
	}

	detailedFileLevel := TraceLevel // 详细日志文件包含所有级别

	// 创建编码器
	consoleEncoder := zapcore.NewConsoleEncoder(consoleConfig)
	fileEncoder := zapcore.NewJSONEncoder(fileConfig)

	// 创建输出目标
	var cores []zapcore.Core

	// 1. 控制台输出（确保不包含 TRACE 级别）
	consoleWriter := zapcore.AddSync(os.Stdout)
	// 如果控制台级别设置为 TRACE，提升到 DEBUG
	actualConsoleLevel := consoleLevel
	if consoleLevel == TraceLevel {
		actualConsoleLevel = zapcore.DebugLevel
	}
	cores = append(cores, zapcore.NewCore(consoleEncoder, consoleWriter, actualConsoleLevel))

	// 2. 普通日志文件
	if config.NormalLogFile != "" {
		if err := ensureDir(config.NormalLogFile); err == nil {
			if normalFile, err := os.OpenFile(config.NormalLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666); err == nil {
				normalWriter := zapcore.AddSync(normalFile)
				cores = append(cores, zapcore.NewCore(fileEncoder, normalWriter, normalFileLevel))
			}
		}
	}

	// 3. 详细日志文件（仅在启用详细日志时）
	if config.EnableDetailedLog && config.DetailedLogFile != "" {
		if err := ensureDir(config.DetailedLogFile); err == nil {
			if detailedFile, err := os.OpenFile(config.DetailedLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666); err == nil {
				detailedWriter := zapcore.AddSync(detailedFile)
				cores = append(cores, zapcore.NewCore(fileEncoder, detailedWriter, detailedFileLevel))
			}
		}
	}

	// 创建组合核心
	core := zapcore.NewTee(cores...)

	// 检查是否需要包装CallbackCore（默认禁用以避免重复输出）
	// 如果需要启用debug回调，可以通过环境变量控制
	if os.Getenv("TRANSLATOR_DEBUG_CALLBACK") == "true" {
		callbackCore := NewCallbackCore(core, myCustomLogCallback)
		return zap.New(callbackCore, zap.AddCaller())
	}

	return zap.New(core, zap.AddCaller())
}

// ensureDir 确保目录存在
func ensureDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return os.MkdirAll(dir, 0o755)
}
