package ags

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/getangry/ags/pkg/middleware"
)

// LogLevel represents different logging levels
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger interface for custom logging implementations
type Logger interface {
	WithFields(fields map[string]interface{}) Logger
	WithContext(ctx context.Context) Logger
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	GetLevel() LogLevel
	SetLevel(level LogLevel)
}

// DefaultLogger provides a basic implementation of the Logger interface
type DefaultLogger struct {
	level  LogLevel
	fields map[string]interface{}
	ctx    context.Context
}

// NewDefaultLogger creates a new default logger with specified log level
func NewDefaultLogger(level LogLevel) *DefaultLogger {
	return &DefaultLogger{
		level:  level,
		fields: make(map[string]interface{}),
		ctx:    context.Background(),
	}
}

func (l *DefaultLogger) log(level LogLevel, msg string, fields ...interface{}) {
	if level < l.level {
		return
	}

	timestamp := time.Now().Format(time.RFC3339)
	logMsg := fmt.Sprintf("[%s] %s %s", level.String(), timestamp, msg)

	// Add context fields if available
	if l.ctx != nil {
		if reqId := middleware.GetReqID(l.ctx); reqId != "" {
			logMsg += fmt.Sprintf(" request_id=%s", reqId)
		}
	}

	// Add default fields
	for k, v := range l.fields {
		logMsg += fmt.Sprintf(" %s=%v", k, v)
	}

	// Add additional fields
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			logMsg += fmt.Sprintf(" %v=%v", fields[i], fields[i+1])
		}
	}

	log.Println(logMsg)
}

func (l *DefaultLogger) WithFields(fields map[string]interface{}) Logger {
	newLogger := &DefaultLogger{
		level:  l.level,
		fields: make(map[string]interface{}),
		ctx:    l.ctx,
	}

	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}

	// Add new fields
	for k, v := range fields {
		newLogger.fields[k] = v
	}

	return newLogger
}

func (l *DefaultLogger) WithContext(ctx context.Context) Logger {
	return &DefaultLogger{
		level:  l.level,
		fields: l.fields,
		ctx:    ctx,
	}
}

func (l *DefaultLogger) Debug(msg string, fields ...interface{}) { l.log(DebugLevel, msg, fields...) }
func (l *DefaultLogger) Info(msg string, fields ...interface{})  { l.log(InfoLevel, msg, fields...) }
func (l *DefaultLogger) Warn(msg string, fields ...interface{})  { l.log(WarnLevel, msg, fields...) }
func (l *DefaultLogger) Error(msg string, fields ...interface{}) { l.log(ErrorLevel, msg, fields...) }
func (l *DefaultLogger) GetLevel() LogLevel                      { return l.level }
func (l *DefaultLogger) SetLevel(level LogLevel)                 { l.level = level }
