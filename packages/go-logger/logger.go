// Package go-logger provides a centralized logging and tracing system for the Digital Bank Platform.
// It implements structured logging, correlation tracing, and supports multiple output formats.
package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

const (
	// Log levels
	LevelTrace = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

// Level represents a log level.
type Level int

// Field represents a log field.
type Field struct {
	Key   string      `json:"key"`
	Value any         `json:"value,omitempty"`
}

// LogRecord represents a complete log entry.
type LogRecord struct {
	Timestamp      string                 `json:"timestamp"`
	TraceID        string                 `json:"trace_id"`
	SpanID         string                 `json:"span_id,omitempty"`
	CorrelationID  string                 `json:"correlation_id,omitempty"`
	Service        string                 `json:"service"`
	Version        string                 `json:"version"`
	Level          string                 `json:"level"`
	Msg            string                 `json:"msg"`
	Context        map[string]any         `json:"context,omitempty"`
	Fields        []Field                `json:"fields,omitempty"`
	Source         string                 `json:"source,omitempty"`
	SourceFile     string                 `json:"source_file,omitempty"`
	SourceLine     string                 `json:"source_line,omitempty"`
	Duration       string                 `json:"duration,omitempty"`
	Labels         map[string]any         `json:"labels,omitempty"`
	Environment    string                 `json:"environment,omitempty"`
	Custom         map[string]any         `json:"custom,omitempty"`
}

// Logger wraps logr.Logger with platform-specific features.
type Logger struct {
	internal logr.Logger
	Level    Level
	buffered []byte
	mu       sync.Mutex
}

// ZapLogger wraps zap logger (for production).
type ZapLogger struct {
	internal interface{}
	buffered []byte
	mu       sync.Mutex
}

// JSONFormatter formats logs as JSON.
type JSONFormatter struct {
	Pretty       bool
	IncludeTrace bool
	IncludeSpan  bool
}

// TextFormatter formats logs as plain text.
type TextFormatter struct {
	IncludeTrace bool
	Prefix       string
}

// ConsoleFormatter formats logs for console display.
type ConsoleFormatter struct {
	Colors      bool
	IncludeTrace bool
}

// LogOutput represents a logging output destination.
type LogOutput interface {
	Write(*LogRecord) error
	Close() error
}

// StdoutOutput writes logs to stdout.
type StdoutOutput struct {
	Formatter        Formatter
	FlushInterval    time.Duration
	EnableTimestamp  bool
	EnableTraceID    bool
	EnableLabels     bool
	autoFlush        bool
	flushTimer       *time.Timer
	mu               sync.Mutex
}

// StderrOutput writes logs to stderr.
type StderrOutput struct {
	Formatter     Formatter
	Level         int
	lastErrorTime time.Time
}

// FileOutput writes logs to a file.
type FileOutput struct {
	path       string
	file       *os.File
	maxSize    int64
	maxLines   int
	keepFiles  int
	rotator    *LogRotator
	closed     bool
	mu         sync.Mutex
}

// LogRotator handles log file rotation.
type LogRotator struct {
	interval  time.Duration
	maxSize   int64
	maxFiles  int
	mu       sync.Mutex
	closed   bool
	current  *os.File
	tailSize int64
	files    []string
}

// Formatter represents a log formatter.
type Formatter interface {
	Format(*LogRecord) []byte
}

// JSONFormatter implements the Formatter interface.
type JSONFormatter struct {
	pretty       bool
	includeTrace bool
	includeSpan  bool
	includeTime  bool
}

func (f *JSONFormatter) Format(lr *LogRecord) []byte {
	var record map[string]any
	record["timestamp"] = lr.Timestamp
	if f.includeTime {
		record["time"] = lr.Time
	}
	if f.includeTrace {
		record["trace_id"] = lr.TraceID
		if lr.SpanID != "" {
			record["span_id"] = lr.SpanID
		}
	}
	if f.includeTrace {
		record["correlation_id"] = lr.CorrelationID
	}
	record["service"] = lr.Service
	record["version"] = lr.Version
	record["level"] = lr.Level
	record["msg"] = lr.Msg
	for _, field := range lr.Fields {
		record[field.Key] = field.Value
	}
	if len(lr.Context) > 0 {
		record["context"] = lr.Context
	}
	record["environment"] = lr.Environment

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if f.pretty {
		enc.SetIndent("  ", "  ")
	} else {
		enc.SetIndent("", ",")
	}
	if err := enc.Encode(record); err != nil {
		return []byte(fmt.Sprintf(`{"error": "%v"}`, err))
	}
	return buf.Bytes()
}

// TextFormatter implements the Formatter interface.
type TextFormatter struct {
	prefix  string
	traceID bool
}

func (f *TextFormatter) Format(lr *LogRecord) []byte {
	var sb strings.Builder
	timePrefix := "["
	if !lr.HasTime() {
		timePrefix = ""
	}
	sb.WriteString(timePrefix)
	sb.WriteString(lr.Time)
	sb.WriteString(timePrefix)

	sb.WriteString("[")
	sb.WriteString(lr.Service)
	sb.WriteString("/")
	sb.WriteString(lr.Version)
	sb.WriteString("] ")

	levelText := strings.ToUpper(lr.Level)
	sb.WriteString(levelText)
	sb.WriteString(": ")

	sb.WriteString(lr.Msg)
	sb.WriteString("\n")

	for _, field := range lr.Fields {
		sb.WriteString("  ")
		sb.WriteString(field.Key)
		sb.WriteString("=")
		sb.WriteString(fmt.Sprintf("%v", field.Value))
		sb.WriteString(", ")
	}

	if f.traceID && lr.TraceID != "" {
		sb.WriteString(" [trace_id=%s]")
		sb.WriteString(lr.TraceID)
		sb.WriteString("\n")
	}

	return []byte(sb.String())
}

// ConsoleFormatter implements the Formatter interface.
type ConsoleFormatter struct {
	enableTrace bool
}

func (f *ConsoleFormatter) Format(lr *LogRecord) []byte {
	// Console formatting with colors
	var colors = map[string]string{
		"TRACE": "\033[38;5;228m", // Yellow
		"DEBUG": "\033[38;5;226m", // Green-Yellow
		"INFO":  "\033[38;5;34m",  // Blue
		"WARN":  "\033[38;5;208m", // Orange
		"ERROR": "\033[38;5;210m", // Red
	}

	var reset = "\033[0m"

	level := colors[lr.Level]

	var sb strings.Builder
	sb.WriteString(level)
	sb.WriteString(strings.ToUpper(lr.Level))
	sb.WriteString(reset)
	sb.WriteString(": ")
	sb.WriteString(lr.Msg)
	sb.WriteString("\n")

	for _, field := range lr.Fields {
		sb.WriteString("  ")
		sb.WriteString(field.Key)
		sb.WriteString("=")
		sb.WriteString(fmt.Sprintf("%v", field.Value))
		sb.WriteString(", ")
	}

	if f.enableTrace && lr.TraceID != "" {
		sb.WriteString(" [trace_id=%s]")
		sb.WriteString(lr.TraceID)
		sb.WriteString("\n")
	}

	return []byte(sb.String())
}
