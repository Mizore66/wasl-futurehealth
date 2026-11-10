package main

import (
	"context"
	"io"
	"log/slog"
	"os"
)

func writeStructuredLogTo(output io.Writer, level, message string, args ...any) {
	severity := slog.LevelInfo
	if level == "warn" {
		severity = slog.LevelWarn
	} else if level == "error" {
		severity = slog.LevelError
	}
	logger := slog.New(slog.NewJSONHandler(output, nil))
	logger.Log(context.Background(), severity, message, args...)
}

func writeStructuredLog(level, message string, args ...any) {
	writeStructuredLogTo(os.Stderr, level, message, args...)
}
