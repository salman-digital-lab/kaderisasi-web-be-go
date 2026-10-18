package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestConfiguredLogFiltering(t *testing.T) {
	for _, level := range []string{"trace", "debug", "info", "warn", "error", "fatal", "silent"} {
		t.Run(level, func(t *testing.T) {
			var out bytes.Buffer
			logger, err := NewLogger(&out, level)
			if err != nil {
				t.Fatal(err)
			}
			logger.Info("request completed")
			logger.Error("request failed")
			if got, want := strings.Contains(out.String(), "request completed"), level == "trace" || level == "debug" || level == "info"; got != want {
				t.Fatalf("info emitted = %v, want %v", got, want)
			}
			if got, want := strings.Contains(out.String(), "request failed"), level != "fatal" && level != "silent"; got != want {
				t.Fatalf("error emitted = %v, want %v", got, want)
			}
		})
	}
	if _, err := NewLogger(&bytes.Buffer{}, "unknown"); err == nil {
		t.Fatal("unknown log level must fail startup")
	}
}

func TestLoggerUsesTerminalTextOutsideProduction(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	var development bytes.Buffer
	logger, err := NewLogger(&development, "info")
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("request completed", "status", 200)
	if got := development.String(); !strings.Contains(got, "level=INFO") || strings.Contains(got, `"level"`) {
		t.Fatalf("development log = %q, want readable terminal text", got)
	}

	var production bytes.Buffer
	t.Setenv("NODE_ENV", "production")
	logger, err = NewLogger(&production, "info")
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("request completed", "status", 200)
	if got := production.String(); !strings.Contains(got, `"level":"INFO"`) {
		t.Fatalf("production log = %q, want JSON", got)
	}
}
