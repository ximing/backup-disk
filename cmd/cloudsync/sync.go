package main

import (
	"context"
	"fmt"

	"github.com/ximing/cloudsync/pkg/compress"
	"github.com/ximing/cloudsync/pkg/config"
	"github.com/ximing/cloudsync/pkg/logger"
	"github.com/ximing/cloudsync/pkg/notify"
	"github.com/ximing/cloudsync/pkg/state"
	"github.com/ximing/cloudsync/pkg/storage"
	syncpkg "github.com/ximing/cloudsync/pkg/sync"

	"github.com/spf13/cobra"
)

// NewSyncCommand creates the sync command
func NewSyncCommand() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "sync [task-name]",
		Short: "手动执行同步任务",
		Long: `手动触发一个或多个同步任务。

示例:
  # 同步指定任务
  cloudsync sync my-task

  # 同步所有任务
  cloudsync sync --all

  # 预览模式（不实际上传）
  cloudsync sync my-task --dry-run`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(cmd, args, dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "预览模式，不实际上传文件")
	cmd.Flags().Bool("all", false, "执行所有任务")

	return cmd
}

func runSync(cmd *cobra.Command, args []string, dryRun bool) error {
	// Load configuration
	cfg, err := config.Load(config.GetConfigPath())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Determine which tasks to run
	allFlag, _ := cmd.Flags().GetBool("all")
	var tasks []config.TaskConfig

	if allFlag {
		tasks = cfg.Tasks
	} else {
		if len(args) == 0 {
			return fmt.Errorf("task name is required (or use --all flag)")
		}
		taskName := args[0]
		found := false
		for _, task := range cfg.Tasks {
			if task.Name == taskName {
				tasks = append(tasks, task)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("task not found: %s", taskName)
		}
	}

	// Initialize logger with rotation
	logDir := config.GetLogDir()
	logLevel := logger.ParseLevel(cfg.Global.LogLevel)
	log, err := logger.NewRotatingLogger(logLevel, cfg.Global.LogFormat, logDir, 100*1024*1024, 5)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	logger.SetGlobalLogger(log)

	// Initialize state database
	stateStore, err := state.New(state.GetDBPath())
	if err != nil {
		return fmt.Errorf("failed to open state database: %w", err)
	}
	defer stateStore.Close()

	// Initialize storage
	store, err := storage.NewStorage(cfg)
	if err != nil {
		return fmt.Errorf("failed to create storage: %w", err)
	}

	// Validate storage connection
	ctx := context.Background()
	if err := store.Validate(ctx); err != nil {
		return fmt.Errorf("storage validation failed: %w", err)
	}

	// Execute tasks
	executor := syncpkg.NewExecutor(store, log)
	var results []*syncpkg.Result

	for _, task := range tasks {
		result := executeTaskWithState(ctx, executor, stateStore, log, cfg, task, dryRun)
		results = append(results, result)
	}

	// Print summary
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("Sync Summary")
	fmt.Println("========================================")

	allSuccess := true
	for _, r := range results {
		status := "✓"
		if !r.Success {
			status = "✗"
			allSuccess = false
		}
		fmt.Printf("%s %s: %d files, %s, %v\n",
			status, r.TaskName, r.FilesSuccess, syncpkg.FormatBytes(r.BytesSuccess), r.Duration())
	}

	fmt.Println("========================================")

	// Print detailed results for each task
	for _, r := range results {
		syncpkg.PrintResult(r)
	}

	if !allSuccess {
		return fmt.Errorf("one or more tasks failed")
	}

	return nil
}

// executeTaskWithState executes a task and records its state
func executeTaskWithState(ctx context.Context, executor *syncpkg.Executor, stateStore *state.Store, log *logger.Logger, cfg *config.Config, task config.TaskConfig, dryRun bool) *syncpkg.Result {
	// Start execution recording
	execID, err := stateStore.StartExecution(task.Name)
	if err != nil {
		log.Warnf("Failed to record execution start: %v", err)
	}

	// Build compression config from task
	compressionConfig := compress.Config{
		Enabled:           task.Compression.Enabled,
		Type:              task.Compression.Type,
		Level:             task.Compression.Level,
		MinSize:           task.Compression.MinSize,
		IncludeExtensions: task.Compression.IncludeExtensions,
		ExcludeExtensions: task.Compression.ExcludeExtensions,
	}

	opts := syncpkg.Options{
		DryRun:      dryRun,
		DateFormat:  task.Target.DateFormat,
		Compression: compressionConfig,
	}

	// Execute the task
	result, err := executor.Execute(ctx, task, opts)

	// Complete execution recording
	if execID > 0 {
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

		recordErr := stateStore.CompleteExecution(
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
			log.Warnf("Failed to record execution completion: %v", recordErr)
		}
	}

	// Send notification
	if !dryRun {
		sendNotification(ctx, cfg, task, result, err, log)
	}

	return result
}

// sendNotification sends a notification for task completion
func sendNotification(ctx context.Context, cfg *config.Config, task config.TaskConfig, result *syncpkg.Result, execErr error, log *logger.Logger) {
	// Build notification config
	notifyConfig := notify.BuildConfig(cfg.Notify, task.Notify, task.Name)
	if !notifyConfig.Enabled {
		return
	}

	// Create notifier
	notifier := notify.NewNotifier(notifyConfig, log)

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
		TaskName:     task.Name,
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
		log.Warnf("Failed to send notification: %v", err)
	} else {
		log.Debugf("Notification sent for task %s", task.Name)
	}
}
