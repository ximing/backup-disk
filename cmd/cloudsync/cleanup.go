package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ximing/cloudsync/pkg/config"
	"github.com/ximing/cloudsync/pkg/retention"
	"github.com/ximing/cloudsync/pkg/storage"
)

// NewCleanupCommand creates the cleanup command
func NewCleanupCommand() *cobra.Command {
	var dryRun bool
	var force bool

	cmd := &cobra.Command{
		Use:   "cleanup [task-name]",
		Short: "清理过期备份",
		Long: `根据保留策略清理云存储中的过期备份。

示例:
  # 清理指定任务的过期备份
  cloudsync cleanup my-task

  # 清理所有任务的过期备份
  cloudsync cleanup --all

  # 预览将被删除的文件（不实际删除）
  cloudsync cleanup my-task --dry-run

  # 跳过确认提示直接清理
  cloudsync cleanup my-task --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			allFlag, _ := cmd.Flags().GetBool("all")

			// Validate arguments
			if !allFlag && len(args) == 0 {
				return fmt.Errorf("task name is required (or use --all flag)")
			}

			var taskName string
			if len(args) > 0 {
				taskName = args[0]
			}

			return runCleanup(taskName, allFlag, dryRun, force)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "预览模式，不实际删除文件")
	cmd.Flags().BoolVar(&force, "force", false, "跳过确认提示，直接执行清理")
	cmd.Flags().Bool("all", false, "清理所有任务")

	return cmd
}

func runCleanup(taskName string, allTasks bool, dryRun bool, force bool) error {
	// Load configuration
	cfg, err := config.Load(config.GetConfigPath())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

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

	// Determine which tasks to clean up
	var tasks []config.TaskConfig
	if allTasks {
		tasks = cfg.Tasks
	} else {
		// Find specific task
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

	// First pass: preview what will be deleted
	type cleanupPreview struct {
		taskName string
		prefix   string
		policy   retention.Policy
		result   *retention.CleanupResult
	}

	var previews []cleanupPreview
	var totalDirs int
	var totalSize int64

	fmt.Println("扫描备份中...")
	fmt.Println()

	for _, task := range tasks {
		// Determine retention policy (task-specific or global)
		policy := getRetentionPolicy(task, cfg.Retention)

		// Skip if no policy is configured
		if policy.MaxDays == 0 && policy.MaxVersions == 0 {
			fmt.Printf("任务 %s: 未配置保留策略，跳过\n", task.Name)
			continue
		}

		// Create retention manager
		manager := retention.NewManager(store, policy)

		// Preview cleanup
		result, err := manager.Cleanup(ctx, task.Target.Prefix, true)
		if err != nil {
			fmt.Printf("任务 %s: 扫描失败: %v\n", task.Name, err)
			continue
		}

		if len(result.DeletedBackups) > 0 {
			previews = append(previews, cleanupPreview{
				taskName: task.Name,
				prefix:   task.Target.Prefix,
				policy:   policy,
				result:   result,
			})
			totalDirs += len(result.DeletedBackups)
			totalSize += result.TotalSize
		}
	}

	// Show preview
	if len(previews) == 0 {
		fmt.Println("没有发现需要清理的过期备份。")
		return nil
	}

	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println("清理预览")
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println()

	for _, preview := range previews {
		fmt.Printf("任务: %s\n", preview.taskName)
		fmt.Printf("  前缀: %s\n", preview.prefix)
		fmt.Printf("  保留策略: ")
		if preview.policy.MaxDays > 0 {
			fmt.Printf("max_days=%d天 ", preview.policy.MaxDays)
		}
		if preview.policy.MaxVersions > 0 {
			fmt.Printf("max_versions=%d ", preview.policy.MaxVersions)
		}
		fmt.Println()
		fmt.Printf("  待清理备份数: %d\n", len(preview.result.DeletedBackups))
		fmt.Printf("  可释放空间: %s\n", formatBytes(preview.result.TotalSize))

		if dryRun {
			fmt.Println("  待删除目录:")
			for _, backup := range preview.result.DeletedBackups {
				fmt.Printf("    - %s (%s, %d 个文件)\n",
					backup.Path,
					formatBytes(backup.Size),
					backup.ObjectCount)
			}
		}
		fmt.Println()
	}

	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Printf("总计: %d 个备份目录, %s 可释放\n", totalDirs, formatBytes(totalSize))
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println()

	// If dry run, we're done
	if dryRun {
		fmt.Println("这是预览模式 (--dry-run)，没有文件被实际删除。")
		fmt.Println("要执行清理，请去掉 --dry-run 标志。")
		return nil
	}

	// Confirm deletion
	if !force {
		fmt.Print("确认删除以上备份吗？此操作不可恢复 [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("已取消清理。")
			return nil
		}
		fmt.Println()
	}

	// Execute cleanup
	fmt.Println("开始清理...")
	fmt.Println()

	var totalDeleted int
	var totalFreed int64

	for _, preview := range previews {
		manager := retention.NewManager(store, preview.policy)
		result, err := manager.Cleanup(ctx, preview.prefix, false)
		if err != nil {
			fmt.Printf("任务 %s: 清理失败: %v\n", preview.taskName, err)
			continue
		}

		fmt.Printf("任务 %s: 已删除 %d 个备份, 释放 %s\n",
			preview.taskName,
			len(result.DeletedBackups),
			formatBytes(result.TotalSize))

		totalDeleted += len(result.DeletedBackups)
		totalFreed += result.TotalSize
	}

	fmt.Println()
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println("清理完成")
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Printf("删除备份数: %d\n", totalDeleted)
	fmt.Printf("释放空间: %s\n", formatBytes(totalFreed))
	fmt.Println("=" + strings.Repeat("=", 60))

	return nil
}

// getRetentionPolicy returns the effective retention policy for a task
func getRetentionPolicy(task config.TaskConfig, globalRetention config.RetentionConfig) retention.Policy {
	policy := retention.Policy{}

	// Check task-specific retention first
	if task.Retention != nil {
		policy.MaxDays = task.Retention.MaxDays
		policy.MaxVersions = task.Retention.MaxVersions
	}

	// Fall back to global retention if not set
	if policy.MaxDays == 0 && globalRetention.MaxDays > 0 {
		policy.MaxDays = globalRetention.MaxDays
	}
	if policy.MaxVersions == 0 && globalRetention.MaxVersions > 0 {
		policy.MaxVersions = globalRetention.MaxVersions
	}

	return policy
}

