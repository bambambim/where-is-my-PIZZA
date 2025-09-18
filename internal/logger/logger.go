package logger

import (
	"encoding/json"
	"io"
	"os"
	"runtime/debug"
	"sync"
	"time"
)

// Level represents the severity level for a log entry.
type Level string

const (
	LevelInfo  Level = "INFO"
	LevelDebug Level = "DEBUG"
	LevelError Level = "ERROR"
)

// ErrorObject contains structured information for error logs.
type ErrorObject struct {
	Msg   string `json:"msg"`
	Stack string `json:"stack"`
}

// LogEntry defines the structure for a single log entry.
type LogEntry struct {
	Timestamp string       `json:"timestamp"`
	Level     Level        `json:"level"`
	Service   string       `json:"service"`
	Action    string       `json:"action"`
	Message   string       `json:"message"`
	Hostname  string       `json:"hostname"`
	RequestID string       `json:"request_id,omitempty"`
	Error     *ErrorObject `json:"error,omitempty"`
}

// Logger is a structured JSON logger. It is safe for concurrent use.
type Logger struct {
	out      io.Writer
	mu       sync.Mutex
	service  string
	hostname string
}

// NewLogger creates and initializes a new Logger instance.
// It takes the service name as an argument and writes logs to os.Stdout.
func NewLogger(serviceName string) *Logger {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown" // Fallback hostname
	}

	return &Logger{
		out:      os.Stdout,
		service:  serviceName,
		hostname: hostname,
	}
}

// print marshals the LogEntry to JSON and writes it to the output.
func (l *Logger) print(level Level, action, message, requestID string, errData *ErrorObject) {
	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level,
		Service:   l.service,
		Action:    action,
		Message:   message,
		Hostname:  l.hostname,
		RequestID: requestID,
		Error:     errData,
	}

	// Lock the mutex to ensure that log entries are not garbled in concurrent scenarios.
	l.mu.Lock()
	defer l.mu.Unlock()

	// Use a JSON encoder to write directly to the output stream.
	if err := json.NewEncoder(l.out).Encode(entry); err != nil {
		// As a fallback, print the error to stderr if JSON encoding fails.
		// This should be a rare event.
		os.Stderr.WriteString("logger: failed to marshal log entry: " + err.Error())
	}
}

// Info logs a message at the INFO level.
// requestID is an optional correlation ID for tracing.
func (l *Logger) Info(action, message string, requestID ...string) {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	}
	l.print(LevelInfo, action, message, rid, nil)
}

// Debug logs a message at the DEBUG level.
// requestID is an optional correlation ID for tracing.
func (l *Logger) Debug(action, message string, requestID ...string) {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	}
	l.print(LevelDebug, action, message, rid, nil)
}

// Error logs an error with a full stack trace.
// err is the error to be logged.
// requestID is an optional correlation ID for tracing.
func (l *Logger) Error(err error, action, message string, requestID ...string) {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	}

	errObj := &ErrorObject{
		Msg:   err.Error(),
		Stack: string(debug.Stack()),
	}

	l.print(LevelError, action, message, rid, errObj)
}
