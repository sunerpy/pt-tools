package config

import (
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
	zapPath := filepath.Join(homeDir, WorkDir, z.Directory)
	if err := os.MkdirAll(zapPath, os.ModePerm); err != nil {
		return nil, err
	}
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(z.Level)); err != nil {
		return nil, err
	}
	encoderConfig := zapcore.EncoderConfig{
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
	var encoder zapcore.Encoder
	if z.Format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}
	infoWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   filepath.Join(zapPath, logFile),
		MaxSize:    z.MaxSize,
		MaxBackups: z.MaxBackups,
		MaxAge:     z.MaxAge,
		Compress:   z.Compress,
	})
	infoPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.InfoLevel && lvl >= level
	})
	cores := []zapcore.Core{
		zapcore.NewCore(encoder, infoWriter, infoPriority),
	}
	if z.LogInConsole {
		consoleEncoderConfig := zapcore.EncoderConfig{
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
		consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)
		consoleCore := zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stdout), zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= level
		}))
		cores = append(cores, consoleCore)
	}
	core := zapcore.NewTee(cores...)
	options := []zap.Option{}
	if z.ShowLine {
		options = append(options, zap.AddCaller())
	}
	if z.StacktraceKey != "" {
		options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	}
	logger := zap.New(core, options...)
	return logger, nil
}
