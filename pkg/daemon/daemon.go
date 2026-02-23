// Package daemon provides daemon mode functionality for running scheduled tasks
package daemon

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	gosync "sync"
	"syscall"
	"time"

	"github.com/ximing/cloudsync/pkg/compress"
	"github.com/ximing/cloudsync/pkg/config"
	"github.com/ximing/cloudsync/pkg/logger"
	"github.com/ximing/cloudsync/pkg/notify"
	"github.com/ximing/cloudsync/pkg/scheduler"
	"github.com/ximing/cloudsync/pkg/state"
	"github.com/ximing/cloudsync/pkg/storage"
	syncpkg "github.com/ximing/cloudsync/pkg/sync"
)

const (
	pidFileName = "cloudsync.pid"
)

// Daemon manages the daemon lifecycle
type Daemon struct {
	cfg        *config.Config
	scheduler  *scheduler.TaskScheduler
	storages   []storage.Storage
	logger     *logger.Logger
	stateStore *state.Store
	pidFile    string
	dataDir    string
	mu         gosync.RWMutex
	running    bool
	stopChan   chan struct{}
	reloadChan chan struct{}
	wg         gosync.WaitGroup
	pipeFD     int // Pipe file descriptor for signaling parent (child only)
}

// Status represents the daemon status
type Status struct {
	Running    bool
	PID        int
	StartTime  time.Time
	Tasks      []TaskStatus
}

// TaskStatus represents the status of a scheduled task
type TaskStatus struct {
	Name        string
	Schedule    string
	NextRun     time.Time
	LastRun     *time.Time
	LastError   error
}

// New creates a new daemon instance
func New(cfg *config.Config, log *logger.Logger, dataDir string) (*Daemon, error) {
	var storages []storage.Storage
	var err error

	if cfg != nil {
		// Create storage backends for all configured storages
		storages, err = storage.NewStoragesFromBackends(cfg.Storage)
		if err != nil {
			return nil, fmt.Errorf("failed to create storage: %w", err)
		}
	}

	pidFile := filepath.Join(dataDir, pidFileName)

	// Initialize state store
	stateStore, err := state.New(state.GetDBPath())
	if err != nil {
		return nil, fmt.Errorf("failed to open state database: %w", err)
	}

	d := &Daemon{
		cfg:        cfg,
		storages:   storages,
		logger:     log,
		stateStore: stateStore,
		pidFile:    pidFile,
		dataDir:    dataDir,
		stopChan:   make(chan struct{}),
		reloadChan: make(chan struct{}),
	}

	// Create task scheduler with handler
	d.scheduler = scheduler.NewTaskScheduler(d.handleTask)

	return d, nil
}

// writeStartupError writes error to temp file for parent to read
func (d *Daemon) writeStartupError(msg string) {
	if errFile := os.Getenv("CLOUDSYNC_STARTUP_ERR_FILE"); errFile != "" {
		os.WriteFile(errFile, []byte(msg), 0644)
	}
}

// readStartupError reads startup error from temp file
func readStartupError(errFilePath string) string {
	if errFilePath == "" {
		return ""
	}
	data, err := os.ReadFile(errFilePath)
	if err != nil {
		return ""
	}
	return string(data)
}

// reinitializeLogger reinitializes the logger for the child process after daemonize
func (d *Daemon) reinitializeLogger() error {
	if d.cfg == nil {
		return nil
	}

	logDir := filepath.Join(d.dataDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	logFile := filepath.Join(logDir, "daemon.log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	logLevel := logger.ParseLevel(d.cfg.Global.LogLevel)
	newLogger := logger.New(logLevel, d.cfg.Global.LogFormat, f)
	d.logger = newLogger
	logger.SetGlobalLogger(newLogger)

	return nil
}

// Start starts the daemon
func (d *Daemon) Start(foreground bool) error {
	// Check if already running
	if existingPID, err := d.readPIDFile(); err == nil && existingPID > 0 {
		if isProcessRunning(existingPID) {
			return fmt.Errorf("daemon already running with PID %d", existingPID)
		}
		// Stale PID file, remove it
		os.Remove(d.pidFile)
	}

	// If not foreground mode, daemonize
	if !foreground {
		// Create a temp error file to capture startup errors from child process
		errFile, err := os.CreateTemp("", "cloudsync-startup-*.err")
		if err != nil {
			return fmt.Errorf("failed to create temp error file: %w", err)
		}
		errFilePath := errFile.Name()
		errFile.Close()
		defer os.Remove(errFilePath)

		// Pass error file path to child via environment
		os.Setenv("CLOUDSYNC_STARTUP_ERR_FILE", errFilePath)

		if err := d.daemonize(); err != nil {
			return fmt.Errorf("failed to daemonize: %w", err)
		}

		// In child process: reinitialize logger and capture startup errors
		if err := d.reinitializeLogger(); err != nil {
			// Write error to temp file so parent can read it
			os.WriteFile(errFilePath, []byte(err.Error()), 0644)
			return fmt.Errorf("failed to reinitialize logger: %w", err)
		}

			// Clear the env var in child
		os.Unsetenv("CLOUDSYNC_STARTUP_ERR_FILE")

		// In child process: defer to catch any startup errors
		var startupErr error
		defer func() {
			if startupErr != nil {
				os.WriteFile(errFilePath, []byte(startupErr.Error()), 0644)
			}
			if r := recover(); r != nil {
				os.WriteFile(errFilePath, []byte(fmt.Sprintf("panic: %v", r)), 0644)
				panic(r)
			}
		}()
	}

	// Write PID file
	if err := d.writePIDFile(); err != nil {
		if !foreground {
			d.writeStartupError(err.Error())
		}
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Validate all storage connections
	ctx := context.Background()
	for i, s := range d.storages {
		if err := s.Validate(ctx); err != nil {
			if !foreground {
				d.writeStartupError(fmt.Sprintf("storage %d validation failed: %v", i, err))
			}
			return fmt.Errorf("storage %d validation failed: %w", i, err)
		}
	}

	d.mu.Lock()
	d.running = true
	d.mu.Unlock()

	// Register all tasks
	if err := d.registerTasks(); err != nil {
		if !foreground {
			d.writeStartupError(fmt.Sprintf("failed to register tasks: %v", err))
		}
		return fmt.Errorf("failed to register tasks: %w", err)
	}

	// Print startup message
	d.logger.Infof("Daemon started (PID: %d)", os.Getpid())
	d.printNextRuns()

	// Start scheduler
	d.scheduler.Start()

	// Signal parent that startup was successful
	if d.pipeFD > 0 {
		d.signalParentSuccess()
	}

	// Setup signal handling
	d.setupSignalHandling()

	// Wait for stop signal
	<-d.stopChan

	// Graceful shutdown
	d.shutdown()

	return nil
}

// Stop stops the daemon
func (d *Daemon) Stop() error {
	pid, err := d.readPIDFile()
	if err != nil {
		return fmt.Errorf("daemon not running or PID file not found")
	}

	if !isProcessRunning(pid) {
		// Clean up stale PID file
		os.Remove(d.pidFile)
		return fmt.Errorf("daemon not running (stale PID file removed)")
	}

	// Send SIGTERM to the daemon process
	if err := sendSignal(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM to PID %d: %w", pid, err)
	}

	// Wait for process to exit (with timeout)
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if !isProcessRunning(pid) {
			return nil
		}
	}

	return fmt.Errorf("timeout waiting for daemon to stop")
}

// Status returns the daemon status
func (d *Daemon) Status() (*Status, error) {
	pid, err := d.readPIDFile()
	if err != nil || pid == 0 {
		return &Status{Running: false}, nil
	}

	if !isProcessRunning(pid) {
		// Clean up stale PID file
		os.Remove(d.pidFile)
		return &Status{Running: false}, nil
	}

	// Build task status
	var tasks []TaskStatus
	for _, task := range d.scheduler.GetAllTasks() {
		ts := TaskStatus{
			Name:     task.Config.Name,
			Schedule: task.Config.Schedule,
			NextRun:  task.NextRun(),
		}
		tasks = append(tasks, ts)
	}

	return &Status{
		Running: true,
		PID:     pid,
		Tasks:   tasks,
	}, nil
}

// registerTasks registers all configured tasks with the scheduler
func (d *Daemon) registerTasks() error {
	for _, taskCfg := range d.cfg.Tasks {
		// Convert config.TaskConfig to scheduler.TaskConfig
		schTask := scheduler.TaskConfig{
			Name:     taskCfg.Name,
			Schedule: taskCfg.Schedule,
			Source: scheduler.SourceConfig{
				Path:    taskCfg.Source.Path,
				Include: taskCfg.Source.Include,
				Exclude: taskCfg.Source.Exclude,
			},
			Target: scheduler.TargetConfig{
				Prefix:     taskCfg.Target.Prefix,
				DateFormat: taskCfg.Target.DateFormat,
			},
			Compression: scheduler.CompressionConfig{
				Enabled:           taskCfg.Compression.Enabled,
				Type:              taskCfg.Compression.Type,
				Mode:              taskCfg.Compression.Mode,
				Level:             taskCfg.Compression.Level,
				MinSize:           taskCfg.Compression.MinSize,
				IncludeExtensions: taskCfg.Compression.IncludeExtensions,
				ExcludeExtensions: taskCfg.Compression.ExcludeExtensions,
				ArchiveName:       taskCfg.Compression.ArchiveName,
			},
		}

		if taskCfg.Retention != nil {
			schTask.Retention = &scheduler.RetentionPolicy{
				MaxDays:     taskCfg.Retention.MaxDays,
				MaxVersions: taskCfg.Retention.MaxVersions,
			}
		}

		if taskCfg.Notify != nil {
			schTask.Notify = &scheduler.NotifySettings{
				Enabled: taskCfg.Notify.Enabled,
			}
		}

		if err := d.scheduler.AddTask(schTask); err != nil {
			return fmt.Errorf("failed to add task %s: %w", taskCfg.Name, err)
		}
		d.logger.Debugf("Registered task: %s (schedule: %s)", taskCfg.Name, taskCfg.Schedule)
	}

	return nil
}

// handleTask is the callback function executed when a task is triggered
func (d *Daemon) handleTask(ctx context.Context, task scheduler.TaskConfig) error {
	d.logger.TaskInfo(task.Name, "Task triggered by scheduler")

	// Find the original config.TaskConfig
	var taskCfg *config.TaskConfig
	for i := range d.cfg.Tasks {
		if d.cfg.Tasks[i].Name == task.Name {
			taskCfg = &d.cfg.Tasks[i]
			break
		}
	}
	if taskCfg == nil {
		return fmt.Errorf("task config not found: %s", task.Name)
	}

	// Record execution start
	var execID int64
	if d.stateStore != nil {
		id, err := d.stateStore.StartExecution(task.Name)
		if err != nil {
			d.logger.Warnf("Failed to record execution start: %v", err)
		} else {
			execID = id
		}
	}

	// Build compression config
	compressionConfig := compress.Config{
		Enabled:           task.Compression.Enabled,
		Type:              task.Compression.Type,
		Mode:              compress.Mode(task.Compression.Mode),
		Level:             task.Compression.Level,
		MinSize:           task.Compression.MinSize,
		IncludeExtensions: task.Compression.IncludeExtensions,
		ExcludeExtensions: task.Compression.ExcludeExtensions,
		ArchiveName:       task.Compression.ArchiveName,
	}

	// Get storage backends for this task
	taskBackends := d.cfg.GetBackendsForTask(*taskCfg)
	if len(taskBackends) == 0 {
		return fmt.Errorf("no storage backends available for task: %s", task.Name)
	}

	// Create storage instances for this task's backends
	var taskStorages []storage.Storage
	for _, backend := range taskBackends {
		s, err := storage.NewStorageFromBackend(backend)
		if err != nil {
			return fmt.Errorf("failed to create storage for backend '%s': %w", backend.Name, err)
		}
		taskStorages = append(taskStorages, s)
	}

	// Execute the sync
	executor := syncpkg.NewExecutorWithStorages(taskStorages, d.logger)
	opts := syncpkg.Options{
		DryRun:      false,
		DateFormat:  task.Target.DateFormat,
		Compression: compressionConfig,
		MaxRetries:  d.cfg.Global.MaxRetries,
	}

	result, err := executor.Execute(ctx, *taskCfg, opts)

	// Record execution completion
	if execID > 0 && d.stateStore != nil {
		var status state.ExecutionStatus
		var errorMsg string

		if err != nil {
			status = state.StatusFailed
			errorMsg = err.Error()
		} else if result.Success {
			status = state.StatusSuccess
		} else {
			status = state.StatusFailed
			if len(result.FailedFiles) > 0 {
				errorMsg = fmt.Sprintf("%d files failed", len(result.FailedFiles))
			}
		}

		recordErr := d.stateStore.CompleteExecution(
			execID,
			status,
			result.FilesTotal,
			result.FilesSuccess,
			result.FilesFailed,
			result.FilesSkipped,
			result.BytesTotal,
			result.BytesSuccess,
			errorMsg,
		)
		if recordErr != nil {
			d.logger.Warnf("Failed to record execution completion: %v", recordErr)
		}
	}

	if err != nil {
		d.logger.TaskError(task.Name, fmt.Sprintf("Task execution failed: %v", err))
		return err
	}

	if result.Success {
		d.logger.TaskInfo(task.Name, "Task completed successfully")
	} else {
		d.logger.TaskWarn(task.Name, fmt.Sprintf("Task completed with %d failures", result.FilesFailed))
	}

	// Send notification
	d.sendNotification(ctx, taskCfg, result, err)

	return nil
}

// sendNotification sends a notification for task completion
func (d *Daemon) sendNotification(ctx context.Context, taskCfg *config.TaskConfig, result *syncpkg.Result, execErr error) {
	// Build notification config
	notifyConfig := notify.BuildConfig(d.cfg.Notify, taskCfg.Notify, taskCfg.Name)
	if !notifyConfig.Enabled {
		return
	}

	// Create notifier
	notifier := notify.NewNotifier(notifyConfig, d.logger)

	// Build result
	status := "success"
	if !result.Success {
		status = "failed"
	}

	errorMsg := ""
	if execErr != nil {
		errorMsg = execErr.Error()
	} else if len(result.FailedFiles) > 0 {
		errorMsg = fmt.Sprintf("%d files failed", len(result.FailedFiles))
	}

	notifyResult := &notify.Result{
		TaskName:     taskCfg.Name,
		Status:       status,
		Duration:     result.Duration(),
		FileCount:    result.FilesTotal,
		SuccessCount: result.FilesSuccess,
		FailedCount:  result.FilesFailed,
		SkippedCount: result.FilesSkipped,
		BytesTotal:   result.BytesTotal,
		BytesSuccess: result.BytesSuccess,
		Error:        errorMsg,
		StartTime:    result.StartTime,
		EndTime:      result.EndTime,
	}

	// Check if we should notify
	if !notifier.ShouldNotify(notifyResult) {
		return
	}

	// Send notification
	if err := notifier.Send(ctx, notifyResult); err != nil {
		d.logger.Warnf("Failed to send notification: %v", err)
	} else {
		d.logger.Debugf("Notification sent for task %s", taskCfg.Name)
	}
}

// setupSignalHandling sets up signal handlers for graceful shutdown
func (d *Daemon) setupSignalHandling() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		for sig := range sigChan {
			switch sig {
			case syscall.SIGTERM, syscall.SIGINT:
				d.logger.Infof("Received %s signal, shutting down gracefully...", sig)
				close(d.stopChan)
				return
			case syscall.SIGHUP:
				d.logger.Info("Received SIGHUP signal, reloading configuration...")
				d.reload()
			}
		}
	}()
}

// reload reloads the configuration
func (d *Daemon) reload() {
	// Load new configuration
	cfg, err := config.Load(config.GetConfigPath())
	if err != nil {
		d.logger.Errorf("Failed to reload configuration: %v", err)
		return
	}

	if err := cfg.Validate(); err != nil {
		d.logger.Errorf("Invalid configuration: %v", err)
		return
	}

	// Update configuration and storage
	d.mu.Lock()
	d.cfg = cfg

	// Create new storage backends with new config
	newStorages, err := storage.NewStoragesFromBackends(cfg.Storage)
	if err != nil {
		d.mu.Unlock()
		d.logger.Errorf("Failed to create new storages: %v", err)
		return
	}
	d.storages = newStorages
	d.mu.Unlock()

	// Restart scheduler with new tasks
	d.scheduler.Stop()
	d.scheduler = scheduler.NewTaskScheduler(d.handleTask)

	if err := d.registerTasks(); err != nil {
		d.logger.Errorf("Failed to register tasks after reload: %v", err)
		return
	}

	d.scheduler.Start()
	d.logger.Info("Configuration reloaded successfully")
	d.printNextRuns()
}

// shutdown performs graceful shutdown
func (d *Daemon) shutdown() {
	d.mu.Lock()
	d.running = false
	d.mu.Unlock()

	d.logger.Info("Shutting down daemon...")

	// Stop scheduler (this waits for current jobs to complete)
	d.scheduler.Stop()

	// Wait for any ongoing tasks
	d.wg.Wait()

	// Close state store
	if d.stateStore != nil {
		d.stateStore.Close()
	}

	// Remove PID file
	os.Remove(d.pidFile)

	d.logger.Info("Daemon stopped")
}

// writePIDFile writes the current process PID to the PID file
func (d *Daemon) writePIDFile() error {
	pid := os.Getpid()
	data := strconv.Itoa(pid)
	return os.WriteFile(d.pidFile, []byte(data), 0644)
}

// readPIDFile reads the PID from the PID file
func (d *Daemon) readPIDFile() (int, error) {
	data, err := os.ReadFile(d.pidFile)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}

// isProcessRunning checks if a process with the given PID is running
func isProcessRunning(pid int) bool {
	// Use platform-specific signal check
	err := signalProcessCheck(pid)
	return err == nil
}

// daemonize forks the process to run as a daemon
// Platform-specific implementation is in daemon_*.go files
func (d *Daemon) daemonize() error {
	return d.daemonizeImpl()
}

// printNextRuns prints the next scheduled run times for all tasks
func (d *Daemon) printNextRuns() {
	d.logger.Info("Scheduled tasks:")
	for _, task := range d.scheduler.GetAllTasks() {
		nextRun := task.NextRun()
		duration := time.Until(nextRun)
		d.logger.Infof("  - %s: next run in %s (at %s)",
			task.Config.Name,
			scheduler.FormatDuration(duration),
			nextRun.Format("2006-01-02 15:04:05"))
	}
}

// IsRunning returns true if the daemon is currently running
func (d *Daemon) IsRunning() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.running
}
