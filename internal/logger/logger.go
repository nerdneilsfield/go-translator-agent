package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger 创建一个新的日志记录器
func NewLogger(debug bool) *zap.Logger {
	config := zap.NewProductionConfig()

	if debug {
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	} else {
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	config.DisableStacktrace = true

	logger, err := config.Build()
	if err != nil {
		panic("初始化日志系统失败: " + err.Error())
	}

	return logger
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
