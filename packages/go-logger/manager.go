// Package logger provides logging and tracing utilities for the Digital Bank Platform.
//
// Core Components:
//   - Logger: Base logging interface for structured logging.
//   - LoggerManager: Creates and manages multiple Logger instances with different formats.
//   - Level: Log levels (Trace, Debug, Info, Warn, Error).
//   - LogRecord: Complete log entry with metadata.
//   - Formatter: Formats log records for output.
//
// Usage:
//   Basic:
//     lm := logger.NewManager("myapp", "1.0.0")
//     log := lm.GetLogger("main")
//     log.Info("Application started")
//
//   With custom levels:
//     log.SetLevel(logger.LevelDebug)
//     log.Debug("Debug message")
//
//   Multiple outputs:
//     stdout := lm.GetOutput(stdoutOutput{level: logger.LevelInfo})
//     file   := lm.GetOutput(fileOutput{level: logger.LevelWarn})
//
//   With trace context:
//     ctx, ctxLogger := log.Trace(ctx, "span_id", "trace_id")
//     log.WithTraceID(ctx.TraceID).Info("Traced message")
package logger

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LogRecord represents a complete log entry with all metadata.
type LogRecord struct {
	Timestamp  string
	Time      string
	TraceID    string
	SpanID     string
	CorrelationID string
	Service    string
	Version    string
	Environment string
	Level      string
	Msg        string
	Source     string
	SourceFile string
	SourceLine string
	Duration   string
	Labels     map[string]any
	Custom     map[string]any
	Fields     []Field
}

// Field represents a key-value log field.
type Field struct {
	Key   string
	Value any
}

// OutputLevel controls what level messages are written to an output.
type OutputLevel struct {
	Min    int // Minimum level to log
	Max    int // Maximum level to log (0 means no max limit)
	Level  string
	LevelID int
}

// Formatter formats log records for output.
type Formatter interface {
	Format(*LogRecord) []byte
}

// TextFormatter formats logs as plain text.
type TextFormatter struct {
	Prefix       string
	IncludeTrace bool
	PrefixColor  string
	Separator    string
}

// JSONFormatter formats logs as JSON.
type JSONFormatter struct {
	Compact      bool
	IncludeTrace bool
	IncludeSpan  bool
	Timestamp    bool
}

// ConsoleFormatter formats logs for terminal/console with colors.
type ConsoleFormatter struct {
	IncludeTrace bool
	Custom       map[string]string
	Prefix       string
}

// LogOutput is the interface for logging output destinations.
type LogOutput interface {
	// Write logs a record to the output
	Write(*LogRecord) error
	// Flush any buffered logs
	Flush() error
	// Close the output
	Close() error
}

// StdoutOutput writes logs to stdout.
type StdoutOutput struct {
	Formatter Formatter
	level     *OutputLevel
	mu        sync.Mutex
	buffered  []byte
	bufferSize int
	autoFlush bool
	flushTimer *time.Timer
}

// FileOutput writes logs to a file with rotation support.
type FileOutput struct {
	path string
	level *OutputLevel
	Formatter
	maxSize int64  // Rotate when file exceeds this size
	maxFiles int // Keep this many old files (0 = no limit)
	rotationInterval time.Duration
}

// StderrOutput writes logs to stderr.
type StderrOutput struct {
	Formatter Formatter
	level     *OutputLevel
	mu        sync.Mutex
}

// Logger represents a logging instance with scope and configuration.
type Logger struct {
	scope string
	name  string
	metadata map[string]string
	level *OutputLevel
	mu     sync.RWMutex

	nameMu sync.RWMutex
	name   string

	outputs []LogOutput
	muOutputs sync.RWMutex

	traceID string
	traceMu sync.RWMutex

	formattedLevels map[string]int
}

// Manager is the central logger manager.
type Manager struct {
	name       string
	version    string
	environment string
	service    string
	defaultOutput LogOutput

	locked bool
	mu     sync.RWMutex

	initialized bool
}

// Option configures a logger.
type Option func(*Logger)

// WithLevel sets the log level.
func WithLevel(level string, levelID int) Option {
	return func(l *Logger) {
		l.level = &OutputLevel{
			Min:    levelID,
			Max:    levelID + 5, // Allow 5 levels above
			Level:  level,
			LevelID: levelID,
		}
	}
}

// WithMetadata sets logger metadata.
func WithMetadata(key, value string) Option {
	return func(l *Logger) {
		if l.metadata == nil {
			l.metadata = make(map[string]string)
		}
		l.metadata[key] = value
	}
}

// WithOutput adds an output destination.
func WithOutput(output LogOutput) Option {
	return func(l *Logger) {
		if l.outputs == nil {
			l.outputs = make([]LogOutput, 0)
		}
		l.outputs = append(l.outputs, output)
	}
}

// WithTrace enables trace ID support.
func WithTrace() Option {
	return func(l *Logger) {
		l.formattedLevels = make(map[string]int)
		l.formattedLevels["TRACE"] = 0
		l.formattedLevels["DEBUG"] = 1
		l.formattedLevels["INFO"] = 2
		l.formattedLevels["WARN"] = 3
		l.formattedLevels["ERROR"] = 4
	}
}

// WithName sets the logger name.
func WithName(name string) Option {
	return func(l *Logger) {
		l.nameMu.Lock()
		defer l.nameMu.Unlock()
		l.name = name
	}
}

// WithContext creates a new logger with context values.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	return &Logger{
		scope:        l.scope,
		name:         l.name,
		metadata:     cloneMap(l.metadata),
		level:        l.level,
		outputs:      cloneSlice(l.outputs),
		formattedLevels: cloneMap(l.formattedLevels),
	}
}

// WithScope creates a child logger with a new scope.
func (l *Logger) WithScope(scope string) *Logger {
	return &Logger{
		scope:        scope,
		name:         l.name,
		metadata:     cloneMap(l.metadata),
		level:        l.level,
		outputs:      cloneSlice(l.outputs),
		formattedLevels: cloneMap(l.formattedLevels),
	}
}

// WithName creates a child logger with a new name.
func (l *Logger) WithName(name string) *Logger {
	return &Logger{
		scope:        l.scope,
		name:         name,
		metadata:     cloneMap(l.metadata),
		level:        l.level,
		outputs:      cloneSlice(l.outputs),
		formattedLevels: cloneMap(l.formattedLevels),
	}
}

// WithTraceID creates a logger with a trace ID.
func (l *Logger) WithTraceID(traceID string) *Logger {
	return &Logger{
		scope:         l.scope,
		name:          l.name,
		metadata:      cloneMap(l.metadata),
		level:         l.level,
		outputs:       cloneSlice(l.outputs),
		formattedLevels: cloneMap(l.formattedLevels),
		traceID:       traceID,
	}
}

// WithSpanID creates a logger with a span ID.
func (l *Logger) WithSpanID(spanID string) *Logger {
	return &Logger{
		scope:        l.scope,
		name:         l.name,
		metadata:     cloneMap(l.metadata),
		level:        l.level,
		outputs:      cloneSlice(l.outputs),
		formattedLevels: cloneMap(l.formattedLevels),
	}
}

// Enabled returns true if logging at the given level is enabled.
func (l *Logger) Enabled(level *OutputLevel) bool {
	if l.level == nil {
		return true
	}
	if l.level.Min > level.LevelID {
		return false
	}
	return level.LevelID <= l.level.Max || l.level.Max == 0
}

// Log formats and writes a log record.
func (l *Logger) Log(level *OutputLevel, traceID, spanID string, levelText string, msg string, fields ...Field) {
	now := time.Now()
	rec := &LogRecord{
		Timestamp:  now.Format(time.RFC3339),
		Time:      now.Format(time.Kitchen),
		TraceID:    traceID,
		SpanID:     spanID,
		Service:    l.getService(),
		Version:    l.getVersion(),
		Environment: l.getEnvironment(),
		Level:      levelText,
		Msg:        msg,
	}

	// Add metadata
	for k, v := range l.metadata {
		rec.Fields = append(rec.Fields, Field{Key: k, Value: v})
	}

	// Add level info
	rec.Fields = append(rec.Fields, Field{Key: "level", Value: levelText})
	rec.Fields = append(rec.Fields, Field{Key: "level_id", Value: level.LevelID})

	// Add fields
	rec.Fields = append(rec.Fields, fields...)

	// Add trace info if available
	if l.traceID != "" {
		rec.Fields = append(rec.Fields, Field{Key: "trace_id", Value: l.traceID})
	}

	// Write to all outputs
	l.writeRecord(rec)
}

// Info logs an info-level message.
func (l *Logger) Info(msg string, fields ...Field) {
	l.log(5, "", 0, "INFO", msg, fields...)
}

// Warn logs a warning-level message.
func (l *Logger) Warn(msg string, fields ...Field) {
	l.log(5, "", 0, "WARN", msg, fields...)
}

// Error logs an error-level message.
func (l *Logger) Error(err error, msg string, fields ...Field) {
	l.log(5, "", 0, "ERROR", msg+": "+err.Error(), fields...)
}

// Debug logs a debug-level message.
func (l *Logger) Debug(msg string, fields ...Field) {
	l.log(1, "", 0, "DEBUG", msg, fields...)
}

// Trace logs a trace-level message.
func (l *Logger) Trace(msg string, fields ...Field) {
	l.log(0, "", 0, "TRACE", msg, fields...)
}

// Logf formats and logs a message with format specifier.
func (l *Logger) Logf(level *OutputLevel, traceID, spanID string, format string, args ...any) {
	l.log(level, traceID, spanID, "", fmt.Sprintf(format, args...))
}

// Infof formats and logs an info-level message.
func (l *Logger) Infof(format string, args ...any) {
	l.log(5, "", 0, "INFO", fmt.Sprintf(format, args...))
}

// Warnf formats and logs a warning-level message.
func (l *Logger) Warnf(format string, args ...any) {
	l.log(5, "", 0, "WARN", fmt.Sprintf(format, args...))
}

// Errorf formats and logs an error-level message.
func (l *Logger) Errorf(format string, args ...any) {
	l.log(5, "", 0, "ERROR", fmt.Sprintf(format, args...))
}

// Debugf formats and logs a debug-level message.
func (l *Logger) Debugf(format string, args ...any) {
	l.log(1, "", 0, "DEBUG", fmt.Sprintf(format, args...))
}

// Tracef formats and logs a trace-level message.
func (l *Logger) Tracef(format string, args ...any) {
	l.log(0, "", 0, "TRACE", fmt.Sprintf(format, args...))
}

// GetLevel returns the current log level.
func (l *Logger) GetLevel() string {
	if l.level == nil {
		return "INFO"
	}
	return l.level.Level
}

// SetLevel sets the log level.
func (l *Logger) SetLevel(level string) {
	switch level {
	case "TRACE":
		l.level = &OutputLevel{Min: 0, Max: 5, Level: "TRACE", LevelID: 0}
	case "DEBUG":
		l.level = &OutputLevel{Min: 1, Max: 5, Level: "DEBUG", LevelID: 1}
	case "INFO":
		l.level = &OutputLevel{Min: 2, Max: 5, Level: "INFO", LevelID: 2}
	case "WARN":
		l.level = &OutputLevel{Min: 3, Max: 5, Level: "WARN", LevelID: 3}
	case "ERROR":
		l.level = &OutputLevel{Min: 5, Max: 5, Level: "ERROR", LevelID: 5}
	default:
		l.level = &OutputLevel{Min: 2, Max: 5, Level: "INFO", LevelID: 2}
	}
}

// GetOutput returns the specified output by name.
func (l *Logger) GetOutput(name string) (*StdoutOutput, error) {
	l.muOutputs.RLock()
	defer l.muOutputs.RUnlock()

	for _, o := range l.outputs {
		if sw, ok := o.(*StdoutOutput); ok && sw != nil {
			return sw, nil
		}
	}
	return nil, fmt.Errorf("output %q not found", name)
}

// GetOutputByLevel returns the first output that logs at the given level.
func (l *Logger) GetOutputByLevel(level string) LogOutput {
	l.muOutputs.RLock()
	defer l.muOutputs.RUnlock()

	for _, o := range l.outputs {
		if o != nil {
			if o.GetLevel() == level {
				return o
			}
		}
	}
	return l.defaultOutput
}

// GetOutputByMinLevel returns the first output that logs at or above the given level.
func (l *Logger) GetOutputByMinLevel(minLevel int) LogOutput {
	l.muOutputs.RLock()
	defer l.muOutputs.RUnlock()

	for _, o := range l.outputs {
		if o != nil {
			if o.GetMinLevel() <= minLevel {
				return o
			}
		}
	}
	return l.defaultOutput
}

// Clone creates a shallow copy of the logger.
func (l *Logger) Clone() *Logger {
	return &Logger{
		scope:     l.scope,
		name:      l.name,
		metadata:  cloneMap(l.metadata),
		level:     cloneOutputLevel(l.level),
		outputs:   cloneSlice(l.outputs),
		formattedLevels: cloneMap(l.formattedLevels),
	}
}

// Shutdown closes all outputs.
func (l *Logger) Shutdown() error {
	l.muOutputs.RLock()
	defer l.muOutputs.RUnlock()

	var lastErr error
	for _, o := range l.outputs {
		if o != nil {
			if err := o.Close(); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// getService returns the service name.
func (l *Logger) getService() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.metadata["service"]
}

// getVersion returns the version.
func (l *Logger) getVersion() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.metadata["version"]
}

// getEnvironment returns the environment.
func (l *Logger) getEnvironment() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.metadata["environment"]
}

func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

func cloneSlice(s []LogOutput) []LogOutput {
	if s == nil {
		return nil
	}
	c := make([]LogOutput, len(s))
	copy(c, s)
	return c
}

func cloneOutputLevel(o *OutputLevel) *OutputLevel {
	if o == nil {
		return nil
	}
	return &OutputLevel{
		Min:    o.Min,
		Max:    o.Max,
		Level:  o.Level,
		LevelID: o.LevelID,
	}
}

// NewManager creates a new logger manager.
func NewManager(name, version, environment, service string) *Manager {
	lm := &Manager{
		name:       name,
		version:    version,
		environment: environment,
		service:    service,
		defaultOutput: &StdoutOutput{
			Formatter:     &TextFormatter{Prefix: "[main]", IncludeTrace: false},
			EnableTimestamp: true,
			EnableTraceID:  true,
		},
	}
	lm.initialized = true
	return lm
}

// GetLogger creates a new logger with the given scope.
func (lm *Manager) GetLogger(scope string) *Logger {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if !lm.initialized {
		panic("logger manager not initialized")
	}
	return lm.getOrCreateLogger(scope)
}

// GetLoggerByName creates a logger with the given name.
func (lm *Manager) GetLoggerByName(name string) *Logger {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if !lm.initialized {
		panic("logger manager not initialized")
	}
	l := lm.getOrCreateLogger(name)
	l.name = name
	return l
}

func (lm *Manager) getOrCreateLogger(name string) *Logger {
	// Try to get existing logger
	return &Logger{
		scope:     name,
		name:      name,
		metadata: map[string]string{
			"service":  lm.service,
			"version":  lm.version,
			"environment": lm.environment,
		},
		level:     &OutputLevel{Min: 2, Max: 5, Level: "INFO", LevelID: 2},
		outputs:   []LogOutput{lm.defaultOutput},
		formattedLevels: make(map[string]int),
	}
}

// RegisterOutput registers a new output with the manager.
func (lm *Manager) RegisterOutput(output LogOutput, level string, levelID int) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if !lm.initialized {
		panic("logger manager not initialized")
	}
	output.(*StdoutOutput).level = &OutputLevel{
		Min:    levelID,
		Max:    levelID + 5,
		Level:  level,
		LevelID: levelID,
	}
	lm.defaultOutput = output
}

// SetDefaultOutput sets the default output for new loggers.
func (lm *Manager) SetDefaultOutput(output LogOutput) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if !lm.initialized {
		panic("logger manager not initialized")
	}
	lm.defaultOutput = output
}

// IsGlobal returns true if the logger is a global logger.
func (l *Logger) IsGlobal() bool {
	return l.scope == ""
}

// Scope returns the logger's scope.
func (l *Logger) Scope() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.scope
}

// Name returns the logger's name.
func (l *Logger) Name() string {
	l.nameMu.RLock()
	defer l.nameMu.RUnlock()
	return l.name
}

// Outputs returns all outputs.
func (l *Logger) Outputs() []LogOutput {
	l.muOutputs.RLock()
	defer l.muOutputs.RUnlock()
	return l.outputs
}

func (o *StdoutOutput) Close() error {
	if o.flushTimer != nil {
		o.flushTimer.Stop()
	}
	return nil
}

func (o *StdoutOutput) Flush() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.flushTimer != nil {
		o.flushTimer.Stop()
	}
	if len(o.buffered) > 0 {
		_, err := os.Stdout.Write(o.buffered)
		o.buffered = nil
		return err
	}
	return nil
}

func (o *StdoutOutput) GetLevel() string {
	if o.level == nil {
		return "INFO"
	}
	return o.level.Level
}

func (o *StdoutOutput) GetMinLevel() int {
	if o.level == nil {
		return 2
	}
	return o.level.Min
}

func (o *StdoutOutput) Write(record *LogRecord) error {
	if o.level == nil || !o.level.Enabled(record.Level) {
		return nil
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	var formatted []byte
	if o.formatter != nil {
		formatted = o.formatter.Format(record)
	}

	if o.autoFlush {
		_, err := os.Stdout.Write(formatted)
		if err != nil {
			return err
		}
	} else {
		o.buffered = append(o.buffered, formatted...)
		if len(o.buffered) > o.bufferSize {
			err := o.Flush()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (o *StdoutOutput) SetAutoFlush(autoFlush bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.autoFlush = autoFlush
	if autoFlush {
		if o.flushTimer == nil {
			o.flushTimer = time.NewTicker(500 * time.Millisecond)
		}
		go o.flushLoop()
	}
}

func (o *StdoutOutput) flushLoop() {
	for {
		select {
		case <-o.flushTimer.C:
			o.Flush()
		}
	}
}

func (o *StdoutOutput) SetFormatter(formatter Formatter) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.formatter = formatter
}

type FileOutput struct {
	path       string
	level      *OutputLevel
	formatter  Formatter
	maxSize    int64
	maxFiles   int
	rotationInterval time.Duration
	closed     bool
	mu        sync.Mutex
}

func (o *FileOutput) Close() error {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil
	}
	o.closed = true
	if o.fw != nil {
		o.fw.Close()
	}
	o.mu.Unlock()
	return nil
}

func (o *FileOutput) Flush() error {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil
	}
	o.mu.Unlock()

	o.mu.Lock()
	defer o.mu.Unlock()

	var err error
	if o.fw != nil {
		_, err = o.fw.Write(o.buffer)
		o.buffer = nil
	}
	return err
}

func (o *FileOutput) GetLevel() string {
	if o.level == nil {
		return "INFO"
	}
	return o.level.Level
}

func (o *FileOutput) GetMinLevel() int {
	if o.level == nil {
		return 2
	}
	return o.level.Min
}

func (o *FileOutput) Write(record *LogRecord) error {
	if o.level == nil || !o.level.Enabled(record.Level) {
		return nil
	}

	if o.closed {
		return fmt.Errorf("file output is closed")
	}

	if o.fw == nil {
		o.mu.Lock()
		if o.fw == nil {
			f, err := os.Create(o.path)
			if err != nil {
				o.mu.Unlock()
				return err
			}
			o.fw = f
			o.mu.Unlock()
		}
	}

	var formatted []byte
	if o.formatter != nil {
		formatted = o.formatter.Format(record)
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if o.maxSize > 0 {
		o.mu.Lock()
		info, err := o.fw.Stat()
		if err != nil {
			o.mu.Unlock()
			o.mu.Unlock()
			return err
		}
		fileSize := info.Size()
		if fileSize > o.maxSize {
			o.mu.Unlock()
			if err := o.rotate(); err != nil {
				o.mu.Lock()
				o.mu.Unlock()
				return err
			}
			o.mu.Lock()
		}
		o.mu.Unlock()
	}

	if o.maxSize > 0 {
		o.mu.Lock()
		info, err := o.fw.Stat()
		if err != nil {
			o.mu.Unlock()
			o.mu.Unlock()
			return err
		}
		fileSize := info.Size()
		if fileSize+int64(len(formatted)) > o.maxSize {
			o.mu.Unlock()
			if err := o.rotate(); err != nil {
				o.mu.Lock()
				o.mu.Unlock()
				return err
			}
			o.mu.Lock()
		}
		o.mu.Unlock()
	}

	if _, err := o.fw.Write(formatted); err != nil {
		o.mu.Unlock()
		o.mu.Unlock()
		return err
	}
	o.mu.Unlock()

	return nil
}
