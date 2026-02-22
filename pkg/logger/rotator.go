package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultMaxSize is the default maximum size of a log file (100MB)
	DefaultMaxSize int64 = 100 * 1024 * 1024
	// DefaultMaxBackups is the default maximum number of backup files to keep
	DefaultMaxBackups = 5
)

// RotatingWriter is a writer that rotates log files when they reach a max size
type RotatingWriter struct {
	filename    string
	maxSize     int64
	maxBackups  int
	currentFile *os.File
	currentSize int64
	mu          sync.Mutex
}

// NewRotatingWriter creates a new rotating file writer
func NewRotatingWriter(filename string, maxSize int64, maxBackups int) (*RotatingWriter, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	if maxBackups <= 0 {
		maxBackups = DefaultMaxBackups
	}

	// Ensure directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	rw := &RotatingWriter{
		filename:   filename,
		maxSize:    maxSize,
		maxBackups: maxBackups,
	}

	// Open or create the log file
	if err := rw.open(); err != nil {
		return nil, err
	}

	return rw, nil
}

// open opens the current log file
func (rw *RotatingWriter) open() error {
	info, err := os.Stat(rw.filename)
	if err == nil {
		rw.currentSize = info.Size()
	} else {
		rw.currentSize = 0
	}

	file, err := os.OpenFile(rw.filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	rw.currentFile = file
	return nil
}

// Write implements io.Writer
func (rw *RotatingWriter) Write(p []byte) (n int, err error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	// Check if we need to rotate
	if rw.currentSize+int64(len(p)) > rw.maxSize {
		if err := rw.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = rw.currentFile.Write(p)
	rw.currentSize += int64(n)
	return n, err
}

// rotate rotates the log file
func (rw *RotatingWriter) rotate() error {
	// Close current file
	if rw.currentFile != nil {
		rw.currentFile.Close()
	}

	// Rotate existing backups
	for i := rw.maxBackups - 1; i >= 0; i-- {
		oldPath := rw.backupName(i)
		newPath := rw.backupName(i + 1)

		if _, err := os.Stat(oldPath); err == nil {
			if i == rw.maxBackups-1 {
				// Delete oldest backup
				os.Remove(oldPath)
			} else {
				os.Rename(oldPath, newPath)
			}
		}
	}

	// Rename current file to .1
	if _, err := os.Stat(rw.filename); err == nil {
		if err := os.Rename(rw.filename, rw.backupName(0)); err != nil {
			return fmt.Errorf("failed to rotate log file: %w", err)
		}
	}

	// Open new file
	rw.currentSize = 0
	return rw.open()
}

// backupName returns the backup file name for the given index
func (rw *RotatingWriter) backupName(index int) string {
	if index == 0 {
		return rw.filename + ".1"
	}
	return fmt.Sprintf("%s.%d", rw.filename, index)
}

// Close closes the rotating writer
func (rw *RotatingWriter) Close() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.currentFile != nil {
		return rw.currentFile.Close()
	}
	return nil
}

// TaskLogManager manages task-specific log files
type TaskLogManager struct {
	logDir     string
	maxSize    int64
	maxBackups int
	writers    map[string]*RotatingWriter
	mu         sync.RWMutex
}

// NewTaskLogManager creates a new task log manager
func NewTaskLogManager(logDir string, maxSize int64, maxBackups int) *TaskLogManager {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	if maxBackups <= 0 {
		maxBackups = DefaultMaxBackups
	}

	return &TaskLogManager{
		logDir:     logDir,
		maxSize:    maxSize,
		maxBackups: maxBackups,
		writers:    make(map[string]*RotatingWriter),
	}
}

// GetWriter returns a writer for the specified task
func (tm *TaskLogManager) GetWriter(taskName string) (*RotatingWriter, error) {
	tm.mu.RLock()
	writer, exists := tm.writers[taskName]
	tm.mu.RUnlock()

	if exists {
		return writer, nil
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Double-check after acquiring write lock
	if writer, exists := tm.writers[taskName]; exists {
		return writer, nil
	}

	// Create task logs directory
	taskLogDir := filepath.Join(tm.logDir, "tasks")
	if err := os.MkdirAll(taskLogDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create task log directory: %w", err)
	}

	// Create new rotating writer for this task
	filename := filepath.Join(taskLogDir, taskName+".log")
	writer, err := NewRotatingWriter(filename, tm.maxSize, tm.maxBackups)
	if err != nil {
		return nil, err
	}

	tm.writers[taskName] = writer
	return writer, nil
}

// Close closes all task log writers
func (tm *TaskLogManager) Close() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	var lastErr error
	for _, writer := range tm.writers {
		if err := writer.Close(); err != nil {
			lastErr = err
		}
	}

	tm.writers = make(map[string]*RotatingWriter)
	return lastErr
}

// GetTaskLogPath returns the path to a task's log file
func (tm *TaskLogManager) GetTaskLogPath(taskName string) string {
	return filepath.Join(tm.logDir, "tasks", taskName+".log")
}

// ListTaskLogs returns a list of all task log files
func (tm *TaskLogManager) ListTaskLogs() ([]string, error) {
	taskLogDir := filepath.Join(tm.logDir, "tasks")

	entries, err := os.ReadDir(taskLogDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read task log directory: %w", err)
	}

	var logs []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".log") {
			logs = append(logs, strings.TrimSuffix(name, ".log"))
		}
	}

	sort.Strings(logs)
	return logs, nil
}

// ReadTaskLog reads the content of a task log file
func (tm *TaskLogManager) ReadTaskLog(taskName string, lines int) ([]string, error) {
	logPath := tm.GetTaskLogPath(taskName)

	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	return tailFile(file, lines)
}

// tailFile reads the last n lines from a file
func tailFile(file *os.File, n int) ([]string, error) {
	if n <= 0 {
		n = 100
	}

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	size := stat.Size()
	if size == 0 {
		return []string{}, nil
	}

	// Read file in chunks from the end
	const chunkSize = 4096
	buffer := make([]byte, 0, chunkSize)
	lines := make([]string, 0, n)
	lineBuf := make([]byte, 0, 256)

	pos := size
	for pos > 0 && len(lines) < n {
		// Calculate chunk to read
		start := pos - chunkSize
		if start < 0 {
			start = 0
		}
		chunkLen := pos - start

		// Read chunk
		chunk := make([]byte, chunkLen)
		_, err := file.ReadAt(chunk, start)
		if err != nil {
			return nil, err
		}

		// Prepend to buffer
		buffer = append(chunk, buffer...)

		// Process buffer for lines
		for i := len(buffer) - 1; i >= 0 && len(lines) < n; i-- {
			if buffer[i] == '\n' {
				if len(lineBuf) > 0 {
					// Reverse lineBuf to get correct order
					for j, k := 0, len(lineBuf)-1; j < k; j, k = j+1, k-1 {
						lineBuf[j], lineBuf[k] = lineBuf[k], lineBuf[j]
					}
					lines = append([]string{string(lineBuf)}, lines...)
					lineBuf = lineBuf[:0]
				}
			} else {
				lineBuf = append(lineBuf, buffer[i])
			}
		}

		buffer = buffer[:0]
		pos = start
	}

	// Handle remaining line buffer
	if len(lineBuf) > 0 && len(lines) < n {
		// Reverse lineBuf
		for j, k := 0, len(lineBuf)-1; j < k; j, k = j+1, k-1 {
			lineBuf[j], lineBuf[k] = lineBuf[k], lineBuf[j]
		}
		lines = append([]string{string(lineBuf)}, lines...)
	}

	return lines, nil
}

// FollowTaskLog follows a task log file for new lines
func (tm *TaskLogManager) FollowTaskLog(taskName string, lines int, callback func(string)) error {
	logPath := tm.GetTaskLogPath(taskName)

	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("log file not found for task: %s", taskName)
		}
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Seek to end initially
	_, err = file.Seek(0, 2)
	if err != nil {
		return err
	}

	// Read new lines as they appear
	buf := make([]byte, 4096)
	var remainder []byte

	for {
		n, err := file.Read(buf)
		if n > 0 {
			data := append(remainder, buf[:n]...)
			start := 0

			for i := 0; i < len(data); i++ {
				if data[i] == '\n' {
					line := string(data[start:i])
					callback(line)
					start = i + 1
				}
			}

			remainder = data[start:]
		}

		if err != nil {
			// Wait a bit before trying again
			time.Sleep(100 * time.Millisecond)
		}
	}
}
