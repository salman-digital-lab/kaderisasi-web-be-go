package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// NewLogger preserves the configured Pino level names, including trace, fatal
// and silent. Development and test processes use slog's readable text format,
// while production keeps newline-delimited JSON for log aggregation.
func NewLogger(output io.Writer, name string) (*slog.Logger, error) {
	levels := map[string]slog.Level{"trace": -8, "debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError, "fatal": 12, "silent": 100}
	level, ok := levels[name]
	if !ok {
		return nil, fmt.Errorf("invalid LOG_LEVEL %q", name)
	}
	options := &slog.HandlerOptions{Level: level}
	if os.Getenv("NODE_ENV") == "production" {
		return slog.New(slog.NewJSONHandler(output, options)), nil
	}
	return slog.New(slog.NewTextHandler(output, options)), nil
}
