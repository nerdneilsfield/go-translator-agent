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

func NewLogger(debug bool) *zap.Logger {
	return NewLoggerWithPath(debug, "")
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

// NewLogger 创建一个新的日志记录器
func NewLoggerWithPath(debug bool, outputPath string) *zap.Logger {
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
		if err := os.MkdirAll(dir, 0755); err != nil {
			// 使用一个临时的基本logger来记录这个致命错误，因为我们的完整logger还没建好
			tempLogger, _ := zap.NewProduction()
			tempLogger.Fatal("创建日志输出目录失败", zap.Error(err))
			// panic(err) // Fatal 已经会 os.Exit(1)
		}
		file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			tempLogger, _ := zap.NewProduction()
			tempLogger.Fatal("打开日志输出文件失败", zap.Error(err))
			// panic(err)
		}
		fileWriter = zapcore.AddSync(file)
	}

	// 创建基础的 cores (控制台和文件)
	var baseCores []zapcore.Core
	baseCores = append(baseCores, zapcore.NewCore(consoleEncoder, consoleWriter, level))
	if enableFileOutput {
		baseCores = append(baseCores, zapcore.NewCore(fileEncoder, fileWriter, level))
	}
	underlyingCore := zapcore.NewTee(baseCores...)

	// 用 CallbackCore 包装 underlyingCore
	callbackEnabledCore := NewCallbackCore(underlyingCore, myCustomLogCallback)

	// 创建 logger
	// zap.AddCaller() 会在 CallbackCore 外部添加调用者信息，
	// 如果你希望回调函数也能收到已经包含调用者信息的 Entry，这是可以的。
	// 如果你想在 CallbackCore 内部控制调用者信息，可能会更复杂。
	// zap.AddCallerSkip(skip) 也可以在这里调整，以确保回调收到的 Entry.Caller 是你期望的。
	// 如果你的 CallbackCore.Write 也调用了 cc.Core.Write，那么 AddCaller 的效果会作用于最终输出。
	return zap.New(callbackEnabledCore, zap.AddCaller() /*, zap.AddStacktrace(zapcore.ErrorLevel) */)
}

// Logger 接口定义了日志记录功能
type Logger interface {
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
