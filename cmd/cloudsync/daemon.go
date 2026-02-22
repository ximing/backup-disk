package main

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/ximing/cloudsync/pkg/config"
	"github.com/ximing/cloudsync/pkg/daemon"
	"github.com/ximing/cloudsync/pkg/logger"
)

// NewDaemonCommand creates the daemon command with subcommands
func NewDaemonCommand() *cobra.Command {
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "守护进程管理",
		Long:  `管理 CloudSync 守护进程，支持启动、停止和查看状态。`,
	}

	daemonCmd.AddCommand(NewDaemonStartCommand())
	daemonCmd.AddCommand(NewDaemonStopCommand())
	daemonCmd.AddCommand(NewDaemonStatusCommand())

	return daemonCmd
}

// NewDaemonStartCommand creates the daemon start command
func NewDaemonStartCommand() *cobra.Command {
	var foreground bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "启动守护进程",
		Long: `启动 CloudSync 守护进程，按计划自动执行备份任务。

示例:
  # 后台启动守护进程
  cloudsync daemon start

  # 前台运行模式（用于调试）
  cloudsync daemon start --foreground`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemonStart(foreground)
		},
	}

	cmd.Flags().BoolVar(&foreground, "foreground", false, "前台运行模式，不脱离终端")

	return cmd
}

// NewDaemonStopCommand creates the daemon stop command
func NewDaemonStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "停止守护进程",
		Long: `停止运行中的 CloudSync 守护进程。

示例:
  cloudsync daemon stop`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemonStop()
		},
	}
}

// NewDaemonStatusCommand creates the daemon status command
func NewDaemonStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看守护进程状态",
		Long: `显示守护进程的运行状态和任务计划。

示例:
  cloudsync daemon status`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemonStatus()
		},
	}
}

func runDaemonStart(foreground bool) error {
	// Load configuration
	cfg, err := config.Load(config.GetConfigPath())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Initialize logger
	logLevel := logger.ParseLevel(cfg.Global.LogLevel)
	var logOutput *os.File
	if foreground {
		logOutput = os.Stdout
	} else {
		// Log to file when running as daemon
		logDir := config.GetLogDir()
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("failed to create log directory: %w", err)
		}
		logFile := fmt.Sprintf("%s/daemon.log", logDir)
		var err error
		logOutput, err = os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		defer logOutput.Close()
	}

	log := logger.New(logLevel, cfg.Global.LogFormat, logOutput)
	logger.SetGlobalLogger(log)

	// Create data directory if not exists
	dataDir := config.GetDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create and start daemon
	d, err := daemon.New(cfg, log, dataDir)
	if err != nil {
		return fmt.Errorf("failed to create daemon: %w", err)
	}

	if foreground {
		fmt.Println("Starting daemon in foreground mode...")
		fmt.Println("Press Ctrl+C to stop")
	}

	return d.Start(foreground)
}

func runDaemonStop() error {
	dataDir := config.GetDataDir()
	pidFile := fmt.Sprintf("%s/cloudsync.pid", dataDir)

	pid, err := readPIDFile(pidFile)
	if err != nil {
		return fmt.Errorf("daemon not running or PID file not found")
	}

	if !isProcessRunning(pid) {
		os.Remove(pidFile)
		return fmt.Errorf("daemon not running (stale PID file removed)")
	}

	// Send SIGTERM
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop daemon: %w", err)
	}

	// Wait for process to exit
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if !isProcessRunning(pid) {
			fmt.Println("Daemon stopped successfully")
			return nil
		}
	}

	return fmt.Errorf("timeout waiting for daemon to stop")
}

func runDaemonStatus() error {
	dataDir := config.GetDataDir()
	pidFile := fmt.Sprintf("%s/cloudsync.pid", dataDir)

	pid, err := readPIDFile(pidFile)
	if err != nil {
		fmt.Println("Daemon is not running")
		return nil
	}

	if !isProcessRunning(pid) {
		os.Remove(pidFile)
		fmt.Println("Daemon is not running (stale PID file removed)")
		return nil
	}

	fmt.Println("Daemon Status")
	fmt.Println("=============")
	fmt.Printf("Status:  Running\n")
	fmt.Printf("PID:     %d\n", pid)

	// Load configuration to show task schedules
	cfg, err := config.Load(config.GetConfigPath())
	if err == nil && len(cfg.Tasks) > 0 {
		fmt.Println()
		fmt.Println("Configured Tasks")
		fmt.Println("----------------")
		for _, task := range cfg.Tasks {
			nextRuns, err := getNextRuns(task.Schedule, 1)
			if err == nil && len(nextRuns) > 0 {
				duration := time.Until(nextRuns[0])
				nextRunStr := fmt.Sprintf("in %s", formatDuration(duration))
				fmt.Printf("  %s: %s (next run %s)\n", task.Name, task.Schedule, nextRunStr)
			} else {
				fmt.Printf("  %s: %s\n", task.Name, task.Schedule)
			}
		}
	}

	return nil
}

// Helper functions
func readPIDFile(pidFile string) (int, error) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds, need to send signal 0
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func getNextRuns(schedule string, n int) ([]time.Time, error) {
	// This is a simplified version - ideally we'd use the scheduler package
	// For now, just return empty to avoid import cycle
	return nil, fmt.Errorf("not implemented")
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd%dh", days, hours)
}
