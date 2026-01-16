package config

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/sunerpy/pt-tools/models"
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

// AtomicLogLevel 全局动态日志级别，用于运行时调整
var AtomicLogLevel zap.AtomicLevel

func init() {
	AtomicLogLevel = zap.NewAtomicLevelAt(zapcore.InfoLevel)
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

	// 解析初始日志级别并设置到 AtomicLogLevel
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(z.Level)); err != nil {
		return nil, fmt.Errorf("解析日志级别失败: %w", err)
	}
	AtomicLogLevel.SetLevel(level)

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

	allWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   filepath.Join(zapPath, "all.log"),
		MaxSize:    z.MaxSize,
		MaxBackups: z.MaxBackups,
		MaxAge:     z.MaxAge,
		Compress:   z.Compress,
	})
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

	allPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= AtomicLogLevel.Level()
	})
	debugPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl == zapcore.DebugLevel && lvl >= AtomicLogLevel.Level()
	})
	highPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel && lvl >= AtomicLogLevel.Level()
	})
	lowPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl > zapcore.DebugLevel && lvl < zapcore.ErrorLevel && lvl >= AtomicLogLevel.Level()
	})

	cores := []zapcore.Core{
		zapcore.NewCore(fileEncoder, allWriter, allPriority),
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
		// 控制台也使用动态日志级别
		consoleCore := zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stdout), AtomicLogLevel)
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

// SetLogLevel 动态设置日志级别
func SetLogLevel(level string) error {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		return fmt.Errorf("无效的日志级别: %s", level)
	}
	AtomicLogLevel.SetLevel(zapLevel)
	return nil
}

// GetLogLevel 获取当前日志级别
func GetLogLevel() string {
	return AtomicLogLevel.Level().String()
}
