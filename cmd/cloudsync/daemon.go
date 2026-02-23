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

默认在前台运行，建议使用 systemd 或其他进程管理器来管理守护进程。

示例:
  # 前台启动守护进程（默认）
  cloudsync daemon start

  # 前台运行模式（用于调试，与默认行为相同）
  cloudsync daemon start --foreground

  # 使用 systemd 管理守护进程（推荐用于 Linux）
  # 1. 复制 scripts/cloudsync.service 到 /etc/systemd/system/
  # 2. sudo systemctl daemon-reload
  # 3. sudo systemctl enable cloudsync
  # 4. sudo systemctl start cloudsync`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemonStart(foreground)
		},
	}

	cmd.Flags().BoolVar(&foreground, "foreground", true, "前台运行模式（默认开启，保留用于兼容性）")

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

	// Initialize logger - always log to stdout in foreground mode
	logLevel := logger.ParseLevel(cfg.Global.LogLevel)
	logOutput := os.Stdout

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

	fmt.Println("Starting daemon in foreground mode...")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()
	fmt.Println("Tip: Use systemd or another process manager to run the daemon in background.")
	fmt.Println("     See scripts/cloudsync.service for an example systemd service file.")
	fmt.Println()

	return d.Start(true)
}

func runDaemonStop() error {
	dataDir := config.GetDataDir()
	pidFile := fmt.Sprintf("%s/cloudsync.pid", dataDir)

	pid, err := readPIDFile(pidFile)
	if err != nil {
		return fmt.Errorf("daemon not running or PID file not found")
	}

	if !daemon.IsProcessRunning(pid) {
		os.Remove(pidFile)
		return fmt.Errorf("daemon not running (stale PID file removed)")
	}

	// Send SIGTERM
	if err := daemon.SendSignal(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop daemon: %w", err)
	}

	// Wait for process to exit
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if !daemon.IsProcessRunning(pid) {
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

	if !daemon.IsProcessRunning(pid) {
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
