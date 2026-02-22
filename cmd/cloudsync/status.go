package main

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/ximing/cloudsync/pkg/config"
	"github.com/ximing/cloudsync/pkg/scheduler"
	"github.com/ximing/cloudsync/pkg/state"
)

// NewStatusCommand creates the status command
func NewStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "显示任务状态",
		Long: `显示所有同步任务的状态信息，包括最后执行状态、下次计划时间等。

示例:
  # 显示所有任务状态
  cloudsync status`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}
}

func runStatus() error {
	// Load configuration
	cfg, err := config.Load(config.GetConfigPath())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Open state database
	stateStore, err := state.New(state.GetDBPath())
	if err != nil {
		return fmt.Errorf("failed to open state database: %w", err)
	}
	defer stateStore.Close()

	// Check daemon status
	daemonRunning := isDaemonRunning()

	// Print daemon status
	fmt.Println("CloudSync Status")
	fmt.Println("================")
	if daemonRunning {
		fmt.Println("Daemon:    Running")
		pid, _ := readPIDFile(config.GetDataDir() + "/cloudsync.pid")
		fmt.Printf("PID:       %d\n", pid)
	} else {
		fmt.Println("Daemon:    Stopped")
	}
	fmt.Println()

	// Print task status
	fmt.Println("Task Status")
	fmt.Println("-----------")

	for _, task := range cfg.Tasks {
		fmt.Printf("\n  Task: %s\n", task.Name)
		fmt.Printf("    Schedule: %s", task.Schedule)

		// Get next run time
		nextRun, err := getNextRunTime(task.Schedule)
		if err == nil && nextRun != nil {
			duration := time.Until(*nextRun)
			fmt.Printf(" (next run in %s)", formatDuration(duration))
		}
		fmt.Println()

		// Get task status from database
		taskStatus, err := stateStore.GetTaskStatus(task.Name)
		if err == nil && taskStatus.LastRun != nil {
			fmt.Printf("    Last Run: %s", taskStatus.LastRun.Format("2006-01-02 15:04:05"))
			if taskStatus.LastStatus != nil {
				statusStr := string(*taskStatus.LastStatus)
				statusIcon := getStatusIcon(*taskStatus.LastStatus)
				fmt.Printf(" [%s %s]", statusIcon, statusStr)
			}
			fmt.Println()
			fmt.Printf("    History:  %d runs (%d success, %d failed)\n",
				taskStatus.TotalRuns, taskStatus.SuccessRuns, taskStatus.FailedRuns)
		} else {
			fmt.Println("    Last Run: Never")
		}
	}

	// Show running executions
	running, err := stateStore.GetRunningExecutions()
	if err == nil && len(running) > 0 {
		fmt.Println("\nRunning Tasks")
		fmt.Println("-------------")
		for _, exec := range running {
			duration := time.Since(exec.StartTime)
			fmt.Printf("  %s: running for %s\n", exec.TaskName, formatDuration(duration))
		}
	}

	return nil
}

func isDaemonRunning() bool {
	dataDir := config.GetDataDir()
	pidFile := dataDir + "/cloudsync.pid"

	pid, err := readPIDFile(pidFile)
	if err != nil {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, need to send signal 0
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func getNextRunTime(schedule string) (*time.Time, error) {
	nextRuns, err := scheduler.GetNextRuns(schedule, 1)
	if err != nil {
		return nil, err
	}
	if len(nextRuns) == 0 {
		return nil, fmt.Errorf("no next run time available")
	}
	return &nextRuns[0], nil
}

func getStatusIcon(status state.ExecutionStatus) string {
	switch status {
	case state.StatusSuccess:
		return "✓"
	case state.StatusFailed:
		return "✗"
	case state.StatusRunning:
		return "▶"
	case state.StatusCanceled:
		return "○"
	default:
		return "?"
	}
}
