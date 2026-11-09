package logging

import (
	"log/slog"
	"os"
	"strings"
)

func Configure(level string) {
	var l slog.Level
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		l = slog.LevelDebug
	case "WARN", "WARNING":
		l = slog.LevelWarn
	case "ERROR":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})
	slog.SetDefault(slog.New(handler))
}
