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
	storage    storage.Storage
	logger     *logger.Logger
	stateStore *state.Store
	pidFile    string
	dataDir    string
	mu         gosync.RWMutex
	running    bool
	stopChan   chan struct{}
	reloadChan chan struct{}
	wg         gosync.WaitGroup
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
	var store storage.Storage
	var err error

	if cfg != nil {
		// Create storage backend
		store, err = storage.NewStorage(cfg)
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
		storage:    store,
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
		if err := d.daemonize(); err != nil {
			return fmt.Errorf("failed to daemonize: %w", err)
		}
	}

	// Write PID file
	if err := d.writePIDFile(); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Validate storage connection
	ctx := context.Background()
	if err := d.storage.Validate(ctx); err != nil {
		return fmt.Errorf("storage validation failed: %w", err)
	}

	d.mu.Lock()
	d.running = true
	d.mu.Unlock()

	// Register all tasks
	if err := d.registerTasks(); err != nil {
		return fmt.Errorf("failed to register tasks: %w", err)
	}

	// Print startup message
	d.logger.Infof("Daemon started (PID: %d)", os.Getpid())
	d.printNextRuns()

	// Start scheduler
	d.scheduler.Start()

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
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
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
				Level:             taskCfg.Compression.Level,
				MinSize:           taskCfg.Compression.MinSize,
				IncludeExtensions: taskCfg.Compression.IncludeExtensions,
				ExcludeExtensions: taskCfg.Compression.ExcludeExtensions,
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
		Level:             task.Compression.Level,
		MinSize:           task.Compression.MinSize,
		IncludeExtensions: task.Compression.IncludeExtensions,
		ExcludeExtensions: task.Compression.ExcludeExtensions,
	}

	// Execute the sync
	executor := syncpkg.NewExecutor(d.storage, d.logger)
	opts := syncpkg.Options{
		DryRun:      false,
		DateFormat:  task.Target.DateFormat,
		Compression: compressionConfig,
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

	return nil
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

	// Create new storage backend with new config
	newStorage, err := storage.NewStorage(cfg)
	if err != nil {
		d.mu.Unlock()
		d.logger.Errorf("Failed to create new storage: %v", err)
		return
	}
	d.storage = newStorage
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
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix systems, FindProcess always succeeds, so we need to send signal 0
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// daemonize forks the process to run as a daemon
func (d *Daemon) daemonize() error {
	// Fork the process
	pid, _, errno := syscall.Syscall(syscall.SYS_FORK, 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("fork failed: %v", errno)
	}

	if pid > 0 {
		// Parent process - exit
		os.Exit(0)
	}

	// Child process continues as daemon

	// Create new session
	_, err := syscall.Setsid()
	if err != nil {
		return fmt.Errorf("setsid failed: %w", err)
	}

	// Change working directory to root
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("chdir failed: %w", err)
	}

	// Redirect standard file descriptors to /dev/null
	devNull, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open /dev/null: %w", err)
	}
	defer devNull.Close()

	syscall.Dup2(int(devNull.Fd()), int(os.Stdin.Fd()))
	syscall.Dup2(int(devNull.Fd()), int(os.Stdout.Fd()))
	syscall.Dup2(int(devNull.Fd()), int(os.Stderr.Fd()))

	return nil
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
