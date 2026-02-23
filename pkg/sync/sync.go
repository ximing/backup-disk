// Package sync provides task synchronization functionality
package sync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ximing/cloudsync/pkg/compress"
	"github.com/ximing/cloudsync/pkg/config"
	"github.com/ximing/cloudsync/pkg/logger"
	"github.com/ximing/cloudsync/pkg/retry"
	"github.com/ximing/cloudsync/pkg/scanner"
	"github.com/ximing/cloudsync/pkg/storage"
)

// Result represents the result of a sync operation
type Result struct {
	TaskName     string
	StartTime    time.Time
	EndTime      time.Time
	Success      bool
	Error        error
	FilesTotal   int
	FilesSuccess int
	FilesFailed  int
	FilesSkipped int
	BytesTotal   int64
	BytesSuccess int64
	FailedFiles  []string
}

// Duration returns the duration of the sync operation
func (r *Result) Duration() time.Duration {
	return r.EndTime.Sub(r.StartTime)
}

// Stats holds runtime statistics for a sync operation
type Stats struct {
	filesTotal   int32
	filesSuccess int32
	filesFailed  int32
	bytesTotal   int64
	bytesSuccess int64
	mu           sync.RWMutex
	failedFiles  []string
}

// IncrementFilesTotal increments the total files counter
func (s *Stats) IncrementFilesTotal() {
	atomic.AddInt32(&s.filesTotal, 1)
}

// IncrementFilesSuccess increments the success files counter
func (s *Stats) IncrementFilesSuccess() {
	atomic.AddInt32(&s.filesSuccess, 1)
}

// IncrementFilesFailed increments the failed files counter and records the file
func (s *Stats) IncrementFilesFailed(path string) {
	atomic.AddInt32(&s.filesFailed, 1)
	s.mu.Lock()
	s.failedFiles = append(s.failedFiles, path)
	s.mu.Unlock()
}

// AddBytesTotal adds to the total bytes counter
func (s *Stats) AddBytesTotal(n int64) {
	atomic.AddInt64(&s.bytesTotal, n)
}

// AddBytesSuccess adds to the success bytes counter
func (s *Stats) AddBytesSuccess(n int64) {
	atomic.AddInt64(&s.bytesSuccess, n)
}

// GetFailedFiles returns the list of failed files
func (s *Stats) GetFailedFiles() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, len(s.failedFiles))
	copy(result, s.failedFiles)
	return result
}

// Options represents sync options
type Options struct {
	DryRun      bool
	DateFormat  string
	Compression compress.Config
	MaxRetries  int // Max retries for file uploads (default: 3)
}

// Executor handles task execution
type Executor struct {
	storages []storage.Storage
	logger   *logger.Logger
}

// NewExecutor creates a new sync executor with a single storage
func NewExecutor(s storage.Storage, log *logger.Logger) *Executor {
	return &Executor{
		storages: []storage.Storage{s},
		logger:   log,
	}
}

// NewExecutorWithStorages creates a new sync executor with multiple storages
func NewExecutorWithStorages(storages []storage.Storage, logger *logger.Logger) *Executor {
	return &Executor{
		storages: storages,
		logger:   logger,
	}
}

// Execute executes a sync task
func (e *Executor) Execute(ctx context.Context, task config.TaskConfig, opts Options) (*Result, error) {
	startTime := time.Now()
	result := &Result{
		TaskName:  task.Name,
		StartTime: startTime,
	}

	e.logger.TaskInfo(task.Name, fmt.Sprintf("Starting sync task: %s", task.Name))

	// Validate source path
	if err := scanner.ValidateSource(task.Source.Path); err != nil {
		result.Success = false
		result.Error = err
		result.EndTime = time.Now()
		return result, err
	}

	// Scan files
	e.logger.TaskInfo(task.Name, fmt.Sprintf("Scanning source directory: %s", task.Source.Path))
	scannerCfg := scanner.Config{
		RootPath: task.Source.Path,
		Include:  task.Source.Include,
		Exclude:  task.Source.Exclude,
	}
	s := scanner.New(scannerCfg)
	files, err := s.Scan()
	if err != nil {
		result.Success = false
		result.Error = err
		result.EndTime = time.Now()
		return result, err
	}

	fileCount, totalSize := scanner.CountFiles(files)
	e.logger.TaskInfo(task.Name, fmt.Sprintf("Found %d files, total size: %s", fileCount, FormatBytes(totalSize)))

	// Generate target prefix with date
	dateStr := time.Now().Format(getDateFormat(task.Target.DateFormat))
	targetPrefix := filepath.Join(task.Target.Prefix, dateStr)
	targetPrefix = filepath.ToSlash(targetPrefix)

	e.logger.TaskInfo(task.Name, fmt.Sprintf("Target prefix: %s", targetPrefix))

	if opts.DryRun {
		e.logger.TaskInfo(task.Name, "DRY RUN MODE - No files will be uploaded")
	}

	// Process files based on compression mode
	stats := &Stats{}

	if opts.Compression.Enabled && opts.Compression.Mode == compress.ModeArchive {
		// Archive mode: pack all files into a single compressed archive
		err := e.executeArchiveMode(ctx, task, files, targetPrefix, opts, stats)
		if err != nil {
			result.Success = false
			result.Error = err
			result.EndTime = time.Now()
			return result, err
		}
	} else {
		// File mode: upload files individually (possibly compressed)
		err := e.executeFileMode(ctx, task, files, targetPrefix, opts, stats)
		if err != nil {
			result.Success = false
			result.Error = err
			result.EndTime = time.Now()
			return result, err
		}
	}

	// Populate result
	result.EndTime = time.Now()

	// Populate result
	result.EndTime = time.Now()
	result.Success = atomic.LoadInt32(&stats.filesFailed) == 0
	result.FilesTotal = int(atomic.LoadInt32(&stats.filesTotal))
	result.FilesSuccess = int(atomic.LoadInt32(&stats.filesSuccess))
	result.FilesFailed = int(atomic.LoadInt32(&stats.filesFailed))
	result.BytesTotal = atomic.LoadInt64(&stats.bytesTotal)
	result.BytesSuccess = atomic.LoadInt64(&stats.bytesSuccess)
	result.FailedFiles = stats.GetFailedFiles()

	if result.Success {
		e.logger.TaskInfo(task.Name, fmt.Sprintf("Sync completed successfully: %d files, %s in %v",
			result.FilesSuccess, FormatBytes(result.BytesSuccess), result.Duration()))
	} else {
		e.logger.TaskWarn(task.Name, fmt.Sprintf("Sync completed with errors: %d failed, %d succeeded",
			result.FilesFailed, result.FilesSuccess))
	}

	return result, nil
}

// executeArchiveMode packs all files into a compressed archive and uploads it
func (e *Executor) executeArchiveMode(ctx context.Context, task config.TaskConfig, files []scanner.FileInfo, targetPrefix string, opts Options, stats *Stats) error {
	if len(files) == 0 {
		e.logger.TaskInfo(task.Name, "No files to archive")
		return nil
	}

	e.logger.TaskInfo(task.Name, fmt.Sprintf("Creating %s archive with %d files...", opts.Compression.Type, len(files)))

	// Create archive compressor
	archiveCompressor := compress.NewArchiveCompressor(opts.Compression)

	// Create the archive
	archivePath, archiveSize, err := archiveCompressor.CreateArchive(files, task.Source.Path, task.Name)
	if err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}
	defer archiveCompressor.Cleanup(archivePath)

	e.logger.TaskInfo(task.Name, fmt.Sprintf("Archive created: %s (%s)", archivePath, FormatBytes(archiveSize)))

	// Build remote path for archive
	archiveExt := archiveCompressor.GetArchiveExtension()
	archiveName := opts.Compression.ArchiveName
	if archiveName == "" {
		archiveName = fmt.Sprintf("%s_%s", task.Name, time.Now().Format("20060102_150405"))
	}
	remotePath := filepath.Join(targetPrefix, archiveName+archiveExt)
	remotePath = filepath.ToSlash(remotePath)

	e.logger.TaskInfo(task.Name, fmt.Sprintf("Uploading archive to: %s", remotePath))

	// Upload the archive with retry
	retryConfig := retry.Config{
		MaxRetries: opts.MaxRetries,
		BaseDelay:  1 * time.Second,
		Multiplier: 2,
	}
	if retryConfig.MaxRetries <= 0 {
		retryConfig.MaxRetries = 3
	}

	// Upload to all storage backends
	uploadErr := e.uploadToAllStorages(ctx, retryConfig, func(s storage.Storage) error {
		return s.Upload(ctx, archivePath, remotePath)
	})

	if uploadErr != nil {
		// Record all files as failed
		for _, f := range files {
			stats.IncrementFilesFailed(f.RelativePath)
		}
		return fmt.Errorf("failed to upload archive: %w", uploadErr)
	}

	// Record success for all files
	for _, f := range files {
		stats.IncrementFilesTotal()
		stats.AddBytesTotal(f.Size)
		stats.IncrementFilesSuccess()
		stats.AddBytesSuccess(f.Size)
	}

	e.logger.TaskInfo(task.Name, fmt.Sprintf("Archive uploaded successfully: %s (%s)", remotePath, FormatBytes(archiveSize)))
	return nil
}

// executeFileMode uploads files individually with optional per-file compression
func (e *Executor) executeFileMode(ctx context.Context, task config.TaskConfig, files []scanner.FileInfo, targetPrefix string, opts Options, stats *Stats) error {
	// Create compressor
	compressor, err := compress.NewCompressor(opts.Compression)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5) // Limit concurrent uploads

	for _, file := range files {
		stats.IncrementFilesTotal()
		stats.AddBytesTotal(file.Size)

		if opts.DryRun {
			e.logger.TaskInfo(task.Name, fmt.Sprintf("[DRY RUN] Would upload: %s", file.RelativePath))
			continue
		}

		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore

		go func(f scanner.FileInfo) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore

			err := e.uploadFile(ctx, task.Name, f, targetPrefix, compressor, opts.MaxRetries)
			if err != nil {
				e.logger.TaskError(task.Name, fmt.Sprintf("Failed to upload %s: %v", f.RelativePath, err))
				stats.IncrementFilesFailed(f.RelativePath)
			} else {
				stats.IncrementFilesSuccess()
				stats.AddBytesSuccess(f.Size)
			}
		}(file)
	}

	wg.Wait()
	return nil
}

// uploadFile uploads a single file with retry logic
func (e *Executor) uploadFile(ctx context.Context, taskName string, file scanner.FileInfo, targetPrefix string, compressor compress.Compressor, maxRetries int) error {
	// Determine if we should compress
	shouldCompress := compressor.ShouldCompress(file.Path, file.Size)

	// Build remote path
	remotePath := filepath.Join(targetPrefix, file.RelativePath)
	if shouldCompress {
		_, ext, _ := compressor.Compress(nil) // Get extension only
		remotePath += ext
	}
	remotePath = filepath.ToSlash(remotePath)

	e.logger.TaskDebug(taskName, fmt.Sprintf("Uploading: %s -> %s", file.RelativePath, remotePath))

	// Prepare upload with retry logic
	retryConfig := retry.Config{
		MaxRetries: maxRetries,
		BaseDelay:  1 * time.Second,
		Multiplier: 2,
	}

	if retryConfig.MaxRetries <= 0 {
		retryConfig.MaxRetries = 3 // Default
	}

	return retry.Retry(ctx, retryConfig, retry.IsRecoverableError, func() error {
		return e.doUpload(ctx, file, remotePath, compressor, shouldCompress)
	})
}

// doUpload performs the actual file upload
func (e *Executor) doUpload(ctx context.Context, file scanner.FileInfo, remotePath string, compressor compress.Compressor, shouldCompress bool) error {
	// Open file
	f, err := os.Open(file.Path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	var reader io.Reader = f

	// Apply compression if needed
	if shouldCompress {
		compressedReader, _, err := compressor.Compress(f)
		if err != nil {
			return fmt.Errorf("failed to compress file: %w", err)
		}
		reader = compressedReader
	}

	// Create a temp file for upload (since storage interface expects local path)
	uploadPath := file.Path
	if shouldCompress {
		tempFile, err := os.CreateTemp("", "cloudsync-*")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		defer os.Remove(tempFile.Name())
		defer tempFile.Close()

		_, err = io.Copy(tempFile, reader)
		if err != nil {
			return fmt.Errorf("failed to write compressed data: %w", err)
		}
		tempFile.Close()

		uploadPath = tempFile.Name()
	}

	// Upload file to all storage backends
	for i, s := range e.storages {
		if err := s.Upload(ctx, uploadPath, remotePath); err != nil {
			return fmt.Errorf("failed to upload to storage %d: %w", i, err)
		}
	}

	return nil
}

// uploadToAllStorages uploads to all storage backends with retry
func (e *Executor) uploadToAllStorages(ctx context.Context, retryConfig retry.Config, uploadFn func(storage.Storage) error) error {
	for i, s := range e.storages {
		err := retry.Retry(ctx, retryConfig, retry.IsRecoverableError, func() error {
			return uploadFn(s)
		})
		if err != nil {
			return fmt.Errorf("storage %d: %w", i, err)
		}
	}
	return nil
}

// getDateFormat returns the date format string, using default if empty
// Supports both Go time layout (2006/01/02/150405) and standard placeholders (YYYY/MM/DD/HHmmss)
func getDateFormat(format string) string {
	if format == "" {
		return "2006/01/02/150405"
	}

	// Convert standard date placeholders to Go time layout
	replacements := map[string]string{
		"YYYY": "2006",
		"YY":   "06",
		"MM":   "01",
		"DD":   "02",
		"HH":   "15",
		"mm":   "04",
		"ss":   "05",
	}

	result := format
	for old, new := range replacements {
		result = strings.ReplaceAll(result, old, new)
	}

	return result
}

// FormatBytes formats bytes to human-readable string
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// PrintResult prints the sync result
func PrintResult(result *Result) {
	fmt.Println()
	fmt.Println("=" + strings.Repeat("=", 50))
	fmt.Println("Sync Report")
	fmt.Println("=" + strings.Repeat("=", 50))
	fmt.Printf("Task:        %s\n", result.TaskName)
	fmt.Printf("Status:      %s\n", map[bool]string{true: "SUCCESS", false: "FAILED"}[result.Success])
	fmt.Printf("Duration:    %v\n", result.Duration())
	fmt.Printf("Files:       %d total, %d success, %d failed, %d skipped\n",
		result.FilesTotal, result.FilesSuccess, result.FilesFailed, result.FilesSkipped)
	fmt.Printf("Bytes:       %s total, %s uploaded\n",
		FormatBytes(result.BytesTotal), FormatBytes(result.BytesSuccess))

	if len(result.FailedFiles) > 0 {
		fmt.Println()
		fmt.Println("Failed files:")
		for _, f := range result.FailedFiles {
			fmt.Printf("  - %s\n", f)
		}
	}
	fmt.Println("=" + strings.Repeat("=", 50))
}
