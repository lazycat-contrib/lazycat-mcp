package zlog

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Logger struct {
	zerolog.Logger
}

type LogConfig struct {
	LogLevel    string
	LogDir      string
	LogFileName string
	MaxSize     int
	MaxBackups  int
	MaxAge      int
}

// NewLogger 创建一个新的日志记录器，支持终端和文件输出
func NewLogger(config LogConfig) *Logger {
	// 设置日志级别
	var level zerolog.Level
	switch config.LogLevel {
	case "debug":
		level = zerolog.DebugLevel
	case "info":
		level = zerolog.InfoLevel
	case "warn":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	case "fatal":
		level = zerolog.FatalLevel
	default:
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// 创建日志目录
	if config.LogDir != "" {
		err := os.MkdirAll(config.LogDir, os.ModePerm)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create log directory")
		}
	}

	// 准备文件输出
	var writers []io.Writer

	// 控制台输出（带颜色）
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}
	writers = append(writers, consoleWriter)

	// 文件输出
	if config.LogFileName != "" {
		logPath := filepath.Join(config.LogDir, config.LogFileName)

		// 使用Lumberjack进行日志切割
		fileWriter := &lumberjack.Logger{
			Filename:   logPath,
			MaxSize:    config.MaxSize,    // 最大文件大小(MB)
			MaxBackups: config.MaxBackups, // 保留旧文件的最大个数
			MaxAge:     config.MaxAge,     // 保留旧文件的最大天数
			Compress:   true,              // 是否压缩/归档旧文件
		}
		writers = append(writers, fileWriter)
	}

	// 创建多输出写入器
	multiWriter := io.MultiWriter(writers...)

	// 创建日志记录器
	logger := zerolog.New(multiWriter).
		With().
		Timestamp().
		Caller().
		Logger()

	return &Logger{logger}
}

// Debug 封装Debug级别日志
func (l *Logger) Debug() *zerolog.Event {
	return l.Logger.Debug()
}

// Info 封装Info级别日志
func (l *Logger) Info() *zerolog.Event {
	return l.Logger.Info()
}

// Warn 封装Warn级别日志
func (l *Logger) Warn() *zerolog.Event {
	return l.Logger.Warn()
}

// Error 封装Error级别日志
func (l *Logger) Error() *zerolog.Event {
	return l.Logger.Error()
}

// Fatal 封装Fatal级别日志
func (l *Logger) Fatal() *zerolog.Event {
	return l.Logger.Fatal()
}
