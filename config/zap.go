package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sunerpy/pt-tools/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var DefaultZapConfig = Zap{
	Directory:     "logs",
	MaxSize:       10,
	MaxAge:        30,
	MaxBackups:    10,
	Compress:      true,
	Level:         "info",
	Format:        "json",
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
	homeDir, _ := os.UserHomeDir()
	zapPath := filepath.Join(homeDir, models.WorkDir, z.Directory)
	if err := os.MkdirAll(zapPath, os.ModePerm); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(z.Level)); err != nil {
		return nil, fmt.Errorf("解析日志级别失败: %w", err)
	}
	encCfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  z.StacktraceKey,
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	fileEncoder := zapcore.NewJSONEncoder(encCfg)
	debugWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   filepath.Join(zapPath, "debug.log"),
		MaxSize:    z.MaxSize,
		MaxBackups: z.MaxBackups,
		MaxAge:     z.MaxAge,
		Compress:   z.Compress,
	})
	infoWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   filepath.Join(zapPath, "info.log"),
		MaxSize:    z.MaxSize,
		MaxBackups: z.MaxBackups,
		MaxAge:     z.MaxAge,
		Compress:   z.Compress,
	})
	errorWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   filepath.Join(zapPath, "error.log"),
		MaxSize:    z.MaxSize,
		MaxBackups: z.MaxBackups,
		MaxAge:     z.MaxAge,
		Compress:   z.Compress,
	})
	debugPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl <= zapcore.DebugLevel && lvl >= level
	})
	highPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel && lvl >= level
	})
	lowPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl > zapcore.DebugLevel && lvl < zapcore.ErrorLevel && lvl >= level
	})
	cores := []zapcore.Core{
		zapcore.NewCore(fileEncoder, debugWriter, debugPriority),
		zapcore.NewCore(fileEncoder, infoWriter, lowPriority),
		zapcore.NewCore(fileEncoder, errorWriter, highPriority),
	}
	if z.LogInConsole {
		consoleCfg := zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  z.StacktraceKey,
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalColorLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}
		consoleEncoder := zapcore.NewConsoleEncoder(consoleCfg)
		consoleCore := zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stdout), zap.LevelEnablerFunc(func(lvl zapcore.Level) bool { return lvl >= level }))
		cores = append(cores, consoleCore)
	}
	core := zapcore.NewTee(cores...)
	options := []zap.Option{}
	podName := os.Getenv("POD_NAME")
	if podName != "" {
		options = append(options, zap.Fields(zap.String("pod", podName)))
	}
	if z.ShowLine {
		options = append(options, zap.AddCaller())
	}
	if z.StacktraceKey != "" {
		options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	}
	logger := zap.New(core, options...)
	return logger, nil
}

// 保留编码器级别选择方法
