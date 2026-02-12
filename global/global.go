package global

import (
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/sunerpy/pt-tools/models"
)

var (
	GlobalLogger *zap.Logger
	GlobalDB     *models.TorrentDB
	once         sync.Once
	// logLevel 用于动态调整日志级别
	logLevel atomic.Value
)

// LogLevel 日志级别类型
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// Deprecated: GlobalCfg removed; use ConfigStore.Load() when needed
func InitLogger(logger *zap.Logger) {
	once.Do(func() {
		GlobalLogger = logger
		logLevel.Store(LogLevelInfo) // 默认 info 级别
	})
}

func GetLogger() *zap.Logger {
	return GlobalLogger
}

func GetSlogger() *zap.SugaredLogger {
	return GlobalLogger.Sugar()
}

func GetSloggerSafe() *zap.SugaredLogger {
	if GlobalLogger == nil {
		return nil
	}
	return GlobalLogger.Sugar()
}

// GetLogLevel 获取当前日志级别
func GetLogLevel() LogLevel {
	if v := logLevel.Load(); v != nil {
		return v.(LogLevel)
	}
	return LogLevelInfo
}

// SetLogLevel 设置日志级别
func SetLogLevel(level LogLevel) {
	logLevel.Store(level)
}

// IsDebugEnabled 检查是否启用了 debug 日志
func IsDebugEnabled() bool {
	return GetLogLevel() == LogLevelDebug
}

// ParseLogLevel 解析日志级别字符串
func ParseLogLevel(level string) (LogLevel, bool) {
	switch level {
	case "debug":
		return LogLevelDebug, true
	case "info":
		return LogLevelInfo, true
	case "warn":
		return LogLevelWarn, true
	case "error":
		return LogLevelError, true
	default:
		return LogLevelInfo, false
	}
}

// ToZapLevel 转换为 zap 日志级别
func (l LogLevel) ToZapLevel() zapcore.Level {
	switch l {
	case LogLevelDebug:
		return zapcore.DebugLevel
	case LogLevelInfo:
		return zapcore.InfoLevel
	case LogLevelWarn:
		return zapcore.WarnLevel
	case LogLevelError:
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
