package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ximing/cloudsync/pkg/compress"
	"github.com/ximing/cloudsync/pkg/config"
	"github.com/ximing/cloudsync/pkg/logger"
	"github.com/ximing/cloudsync/pkg/storage"
	"github.com/ximing/cloudsync/pkg/sync"

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

	// Initialize logger
	logLevel := logger.ParseLevel(cfg.Global.LogLevel)
	log := logger.New(logLevel, cfg.Global.LogFormat, os.Stdout)
	logger.SetGlobalLogger(log)

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
	executor := sync.NewExecutor(store, log)
	var results []*sync.Result

	for _, task := range tasks {
		// Build compression config from task
		compressionConfig := compress.Config{
			Enabled:           task.Compression.Enabled,
			Type:              task.Compression.Type,
			Level:             task.Compression.Level,
			MinSize:           task.Compression.MinSize,
			IncludeExtensions: task.Compression.IncludeExtensions,
			ExcludeExtensions: task.Compression.ExcludeExtensions,
		}

		opts := sync.Options{
			DryRun:      dryRun,
			DateFormat:  task.Target.DateFormat,
			Compression: compressionConfig,
		}

		result, err := executor.Execute(ctx, task, opts)
		if err != nil {
			log.Errorf("Task %s failed: %v", task.Name, err)
		}
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
			status, r.TaskName, r.FilesSuccess, sync.FormatBytes(r.BytesSuccess), r.Duration())
	}

	fmt.Println("========================================")

	// Print detailed results for each task
	for _, r := range results {
		sync.PrintResult(r)
	}

	if !allSuccess {
		return fmt.Errorf("one or more tasks failed")
	}

	return nil
}
