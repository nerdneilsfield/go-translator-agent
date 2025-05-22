package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"path/filepath"
)

func NewLogger(debug bool) *zap.Logger {
	return NewLoggerWithPath(debug, "")
}

// NewLogger 创建一个新的日志记录器
func NewLoggerWithPath(debug bool, output_path string) *zap.Logger {
	// 控制台输出配置（彩色）
	consoleConfig := zap.NewDevelopmentEncoderConfig()
	consoleConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	consoleConfig.EncodeCaller = zapcore.ShortCallerEncoder

	// 文件输出配置（JSON）
	fileConfig := zap.NewProductionEncoderConfig()
	fileConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// 设置日志级别
	level := zapcore.InfoLevel
	if debug {
		level = zapcore.DebugLevel
	}

	enableFileOutput := false
	if output_path != "" {
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

	// 如果要输出到文件，可以添加文件输出
	var fileWriter zapcore.WriteSyncer
	if enableFileOutput {
		// 如果文件所在目录不存在，创建它
		dir := filepath.Dir(output_path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			zap.L().Fatal("创建日志输出目录失败", zap.Error(err))
			panic(err)
		}
		file, err := os.OpenFile(output_path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			zap.L().Fatal("打开日志输出文件失败", zap.Error(err))
			panic(err)
		}
		fileWriter = zapcore.AddSync(file)
	}

	// 创建 core，使用 Tee 同时输出到多个目标
	var cores []zapcore.Core
	cores = append(cores, zapcore.NewCore(consoleEncoder, consoleWriter, level))
	if enableFileOutput {
		cores = append(cores, zapcore.NewCore(fileEncoder, fileWriter, level))
	}
	core := zapcore.NewTee(
		cores...,
	)

	return zap.New(core, zap.AddCaller())
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
