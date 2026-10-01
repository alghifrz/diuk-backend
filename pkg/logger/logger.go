// Package logger provides the application logger built on log/slog.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a structured logger that writes to stdout. Production
// environments get JSON output so logs stay machine readable, while other
// environments get a more readable text format. Every record carries a
// timestamp and a level (INFO, WARN, ERROR) by default.
func New(env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: levelFor(env)}

	var handler slog.Handler
	if strings.EqualFold(env, "production") {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

func levelFor(env string) slog.Level {
	if strings.EqualFold(env, "production") {
		return slog.LevelInfo
	}
	return slog.LevelDebug
}
