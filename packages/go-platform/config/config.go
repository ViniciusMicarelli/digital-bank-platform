// Package config reads service configuration from environment variables.
package config

import (
	"log/slog"
	"os"
	"strconv"
	"time"
)

// String returns the value of key, or def when unset or empty.
func String(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Duration parses key as a time.Duration ("5s", "1m"), or returns def.
func Duration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		slog.Warn("config: invalid duration, using default", "key", key, "value", v)
	}
	return def
}

// Int parses key as an int, or returns def.
func Int(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		slog.Warn("config: invalid int, using default", "key", key, "value", v)
	}
	return def
}

// Logger returns a JSON logger tagged with the service name. LOG_LEVEL accepts
// debug, info, warn or error.
func Logger(service string) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(String("LOG_LEVEL", "info"))); err != nil {
		level = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(h).With("service", service)
}
