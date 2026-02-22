package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ximing/cloudsync/pkg/state"
)

// NewHistoryCommand creates the history command
func NewHistoryCommand() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "history [task-name]",
		Short: "查看执行历史",
		Long: `查看任务执行历史记录，从 state.db 数据库查询。

示例:
  # 查看所有任务的历史记录
  cloudsync history

  # 查看指定任务的历史
  cloudsync history my-task

  # 限制显示条数
  cloudsync history --limit 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var taskName string
			if len(args) > 0 {
				taskName = args[0]
			}
			return runHistory(taskName, limit)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 10, "显示的历史记录条数")

	return cmd
}

func runHistory(taskName string, limit int) error {
	// Open state database
	stateStore, err := state.New(state.GetDBPath())
	if err != nil {
		return fmt.Errorf("failed to open state database: %w", err)
	}
	defer stateStore.Close()

	var records []*state.ExecutionRecord

	if taskName == "" {
		// Get all task history
		records, err = stateStore.GetAllHistory(limit)
		if err != nil {
			return fmt.Errorf("failed to get history: %w", err)
		}
		fmt.Println("Execution History")
		fmt.Println("=================")
	} else {
		// Get specific task history
		records, err = stateStore.GetTaskHistory(taskName, limit)
		if err != nil {
			return fmt.Errorf("failed to get task history: %w", err)
		}
		fmt.Printf("Execution History for Task: %s\n", taskName)
		fmt.Println(strings.Repeat("=", 40+len(taskName)))
	}

	if len(records) == 0 {
		fmt.Println("\nNo execution records found.")
		return nil
	}

	// Print header
	fmt.Printf("\n%-4s %-20s %-15s %-12s %-10s %-12s %-12s\n",
		"ID", "Time", "Task", "Status", "Duration", "Files", "Bytes")
	fmt.Println(strings.Repeat("-", 90))

	// Print records
	for _, rec := range records {
		duration := "-"
		if rec.Duration != nil {
			duration = formatDurationMS(*rec.Duration)
		}

		statusIcon := getHistoryStatusIcon(rec.Status)

		files := fmt.Sprintf("%d/%d", rec.FilesSuccess, rec.FilesTotal)
		if rec.FilesFailed > 0 {
			files = fmt.Sprintf("%s (+%d failed)", files, rec.FilesFailed)
		}

		bytesStr := formatBytes(rec.BytesSuccess)

		fmt.Printf("%-4d %-20s %-15s %s %-10s %-12s %-12s\n",
			rec.ID,
			rec.StartTime.Format("2006-01-02 15:04:05"),
			truncateString(rec.TaskName, 15),
			statusIcon,
			duration,
			files,
			bytesStr,
		)

		// Print error if present
		if rec.Error != "" {
			fmt.Printf("     Error: %s\n", truncateString(rec.Error, 70))
		}
	}

	return nil
}

func getHistoryStatusIcon(status state.ExecutionStatus) string {
	switch status {
	case state.StatusSuccess:
		return "✓ success    "
	case state.StatusFailed:
		return "✗ failed     "
	case state.StatusRunning:
		return "▶ running    "
	case state.StatusCanceled:
		return "○ canceled   "
	default:
		return "? unknown    "
	}
}

func formatDurationMS(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	seconds := ms / 1000
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	seconds = seconds % 60
	if minutes < 60 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	hours := minutes / 60
	minutes = minutes % 60
	return fmt.Sprintf("%dh%dm", hours, minutes)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
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
