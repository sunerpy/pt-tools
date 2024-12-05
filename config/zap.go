package config

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	logFile = "info.log"
	WorkDir = ".pt-tools"
)

var DefaultZapConfig = Zap{
	Directory:     "logs",
	MaxSize:       10,
	MaxAge:        30,
	MaxBackups:    10,
	Compress:      true,
	Level:         "info",
	Format:        "console",
	ShowLine:      false,
	EncodeLevel:   "CapitalColorLevelEncoder",
	StacktraceKey: "",
	LogInConsole:  true,
}

type Zap struct {
	Directory     string `mapstructure:"directory" json:"directory"  yaml:"directory"`
	MaxSize       int    `mapstructure:"max_size" json:"max_size" yaml:"max_size"`
	MaxAge        int    `mapstructure:"max_age" json:"max_age" yaml:"max_age"`
	MaxBackups    int    `mapstructure:"max_backups" json:"max_backups" yaml:"max_backups"`
	Compress      bool   `mapstructure:"compress" json:"compress" yaml:"compress"`
	Level         string `mapstructure:"level" json:"level" yaml:"level"` // debug  info  warn  error
	Format        string `mapstructure:"format" json:"format" yaml:"format"`
	EncodeLevel   string `mapstructure:"encode_level" json:"encode_level" yaml:"encode_level"`
	StacktraceKey string `mapstructure:"stacktrace_key" json:"stacktrace_key" yaml:"stacktrace_key"`
	LogInConsole  bool   `mapstructure:"log_in_console" json:"log_in_console" yaml:"log_in_console"`
	ShowLine      bool   `mapstructure:"show_line" json:"show_line" yaml:"show_line"`
}

func (z *Zap) ZapEncodeLevel() zapcore.LevelEncoder {
	switch z.EncodeLevel {
	case "LowercaseLevelEncoder":
		return zapcore.LowercaseLevelEncoder
	case "LowercaseColorLevelEncoder":
		return zapcore.LowercaseColorLevelEncoder
	case "CapitalLevelEncoder":
		return zapcore.CapitalLevelEncoder
	case "CapitalColorLevelEncoder":
		return zapcore.CapitalColorLevelEncoder
	default:
		return zapcore.LowercaseLevelEncoder
	}
}

func (z *Zap) InitLogger() (*zap.Logger, error) {
	// 创建日志目录
	homeDir, _ := os.UserHomeDir()
	zapPath := filepath.Join(homeDir, WorkDir, z.Directory)
	if err := os.MkdirAll(zapPath, os.ModePerm); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	// 解析日志级别
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(z.Level)); err != nil {
		return nil, fmt.Errorf("解析日志级别失败: %w", err)
	}
	// 初始化日志核心
	cores := []zapcore.Core{
		z.createFileCore(zapPath, level),
	}
	if z.LogInConsole {
		cores = append(cores, z.createConsoleCore(level))
	}
	// 构造 zap.Logger
	core := zapcore.NewTee(cores...)
	options := z.buildLoggerOptions()
	logger := zap.New(core, options...)
	return logger, nil
}

// 创建文件日志核心
func (z *Zap) createFileCore(logPath string, level zapcore.Level) zapcore.Core {
	encoder := z.createEncoder(z.Format == "json")
	writer := zapcore.AddSync(&lumberjack.Logger{
		Filename:   filepath.Join(logPath, logFile),
		MaxSize:    z.MaxSize,
		MaxBackups: z.MaxBackups,
		MaxAge:     z.MaxAge,
		Compress:   z.Compress,
	})
	enabler := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.InfoLevel && lvl >= level
	})
	return zapcore.NewCore(encoder, writer, enabler)
}

// 创建控制台日志核心
func (z *Zap) createConsoleCore(level zapcore.Level) zapcore.Core {
	encoder := z.createEncoder(false) // 使用控制台编码器
	enabler := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= level
	})
	return zapcore.NewCore(encoder, zapcore.Lock(os.Stdout), enabler)
}

// 构建日志编码器
func (z *Zap) createEncoder(isJSON bool) zapcore.Encoder {
	config := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  z.StacktraceKey,
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    z.ZapEncodeLevel(),
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	if isJSON {
		return zapcore.NewJSONEncoder(config)
	}
	config.EncodeLevel = zapcore.CapitalColorLevelEncoder // 彩色输出适合控制台
	return zapcore.NewConsoleEncoder(config)
}

// 构建 Logger 的选项
func (z *Zap) buildLoggerOptions() []zap.Option {
	options := []zap.Option{}
	if z.ShowLine {
		options = append(options, zap.AddCaller())
	}
	if z.StacktraceKey != "" {
		options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	}
	return options
}
