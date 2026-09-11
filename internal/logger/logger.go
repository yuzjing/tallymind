package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"

	"tallymind/internal/config"
)

func InitLogger(cfg config.LogConfig) func() {
	var level slog.Level

	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo // 默认 info
	}

	var writers []io.Writer
	var closer io.Closer

	if cfg.ToStdout {
		writers = append(writers, os.Stdout)
	}

	if cfg.File.Enabled && cfg.File.Dir != "" {
		fileLogger := &lumberjack.Logger{
			Filename:   filepath.Join(cfg.File.Dir, "tallymind.log"),
			MaxSize:    cfg.File.MaxSize,    // MB
			MaxBackups: cfg.File.MaxBackups, // 个数
			MaxAge:     cfg.File.MaxAge,     // 天数
			Compress:   cfg.File.Compress,   // 是否压缩
		}
		writers = append(writers, fileLogger)
		closer = fileLogger
	}

	if len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}

	multiwriter := io.MultiWriter(writers...)

	logger := slog.New(slog.NewTextHandler(multiwriter, &slog.HandlerOptions{
		Level: level,
	}))

	slog.SetDefault(logger) // 设置全局日志记录器

	return func() {
		if closer != nil {
			closer.Close()
		}
	}

}
