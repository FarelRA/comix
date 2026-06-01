package logger

import (
	"log/slog"
	"os"
	"strings"
)

func Configure(level, format string, verbose bool) {
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	} else {
		switch strings.ToLower(level) {
		case "debug":
			logLevel = slog.LevelDebug
		case "warn":
			logLevel = slog.LevelWarn
		case "error":
			logLevel = slog.LevelError
		}
	}

	opts := &slog.HandlerOptions{Level: logLevel}
	if strings.EqualFold(format, "json") {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, opts)))
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, opts)))
}
