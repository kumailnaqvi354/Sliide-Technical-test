package main

import (
	"io"
	"log/slog"
	"strings"
)

const serviceName = "articles-api"

// newLogger returns a JSON logger at the given level ("debug", "info", "warn"
// or "error", case-insensitive). JSON because log pipelines can index and
// filter it by field. An empty level means info. An unknown one also falls
// back to info rather than failing startup, and is reported in the second
// return value so the caller can log it.
func newLogger(w io.Writer, level string) (logger *slog.Logger, unknownLevel bool) {
	var parsed slog.Level
	if level != "" {
		if err := parsed.UnmarshalText([]byte(strings.TrimSpace(level))); err != nil {
			parsed, unknownLevel = slog.LevelInfo, true
		}
	}

	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: parsed})

	return slog.New(handler).With("service", serviceName), unknownLevel
}
