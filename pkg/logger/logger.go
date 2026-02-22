package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogLevel represents the logging level
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

// Logger represents a logger instance
type Logger struct {
	level         LogLevel
	format        string // text or json
	output        io.Writer
	taskOutput    map[string]io.Writer
	taskLogManager *TaskLogManager
	mu            sync.RWMutex
}

var (
	globalLogger *Logger
	once         sync.Once
)

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	once.Do(func() {
		globalLogger = New(InfoLevel, "text", os.Stdout)
	})
	return globalLogger
}

// SetGlobalLogger sets the global logger instance
func SetGlobalLogger(l *Logger) {
	globalLogger = l
}

// New creates a new logger
func New(level LogLevel, format string, output io.Writer) *Logger {
	return &Logger{
		level:      level,
		format:     format,
		output:     output,
		taskOutput: make(map[string]io.Writer),
	}
}

// NewFileLogger creates a logger that writes to a file
func NewFileLogger(level LogLevel, format, logDir string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logFile := filepath.Join(logDir, "cloudsync.log")
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return New(level, format, f), nil
}

// NewRotatingLogger creates a logger with log rotation support
func NewRotatingLogger(level LogLevel, format, logDir string, maxSize int64, maxBackups int) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logFile := filepath.Join(logDir, "cloudsync.log")
	rotator, err := NewRotatingWriter(logFile, maxSize, maxBackups)
	if err != nil {
		return nil, fmt.Errorf("failed to create rotating log writer: %w", err)
	}

	return &Logger{
		level:          level,
		format:         format,
		output:         rotator,
		taskOutput:     make(map[string]io.Writer),
		taskLogManager: NewTaskLogManager(logDir, maxSize, maxBackups),
	}, nil
}

// SetTaskLogManager sets the task log manager
func (l *Logger) SetTaskLogManager(tlm *TaskLogManager) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.taskLogManager = tlm
}

// GetTaskLogManager returns the task log manager
func (l *Logger) GetTaskLogManager() *TaskLogManager {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.taskLogManager
}

// SetLevel sets the logging level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetTaskOutput sets the output writer for a specific task
func (l *Logger) SetTaskOutput(taskName string, w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.taskOutput[taskName] = w
}

// Debug logs a debug message
func (l *Logger) Debug(msg string) {
	l.log(DebugLevel, msg, nil)
}

// Debugf logs a formatted debug message
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(DebugLevel, fmt.Sprintf(format, args...), nil)
}

// Info logs an info message
func (l *Logger) Info(msg string) {
	l.log(InfoLevel, msg, nil)
}

// Infof logs a formatted info message
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(InfoLevel, fmt.Sprintf(format, args...), nil)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string) {
	l.log(WarnLevel, msg, nil)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(WarnLevel, fmt.Sprintf(format, args...), nil)
}

// Error logs an error message
func (l *Logger) Error(msg string) {
	l.log(ErrorLevel, msg, nil)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(ErrorLevel, fmt.Sprintf(format, args...), nil)
}

// TaskDebug logs a debug message for a specific task
func (l *Logger) TaskDebug(taskName, msg string) {
	l.log(DebugLevel, msg, &taskName)
}

// TaskInfo logs an info message for a specific task
func (l *Logger) TaskInfo(taskName, msg string) {
	l.log(InfoLevel, msg, &taskName)
}

// TaskWarn logs a warning message for a specific task
func (l *Logger) TaskWarn(taskName, msg string) {
	l.log(WarnLevel, msg, &taskName)
}

// TaskError logs an error message for a specific task
func (l *Logger) TaskError(taskName, msg string) {
	l.log(ErrorLevel, msg, &taskName)
}

func (l *Logger) log(level LogLevel, msg string, taskName *string) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if level < l.level {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	levelStr := levelToString(level)

	var output io.Writer = l.output
	if taskName != nil {
		// First check for direct task output
		if taskOut, ok := l.taskOutput[*taskName]; ok {
			output = taskOut
		} else if l.taskLogManager != nil {
			// Try to get task log writer
			if writer, err := l.taskLogManager.GetWriter(*taskName); err == nil {
				output = writer
			}
		}
	}

	if l.format == "json" {
		fmt.Fprintf(output, `{"time":"%s","level":"%s","msg":"%s"}`+"\n", timestamp, levelStr, msg)
	} else {
		fmt.Fprintf(output, "[%s] %s: %s\n", timestamp, levelStr, msg)
	}
}

func levelToString(level LogLevel) string {
	switch level {
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

// ParseLevel parses a log level string
func ParseLevel(s string) LogLevel {
	switch s {
	case "debug":
		return DebugLevel
	case "warn", "warning":
		return WarnLevel
	case "error":
		return ErrorLevel
	default:
		return InfoLevel
	}
}
