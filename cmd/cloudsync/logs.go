package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/ximing/cloudsync/pkg/config"
)

// NewLogsCommand creates the logs command
func NewLogsCommand() *cobra.Command {
	var follow bool
	var lines int

	cmd := &cobra.Command{
		Use:   "logs [task-name]",
		Short: "查看日志",
		Long: `查看任务日志或主日志文件。支持实时跟踪和指定行数。

示例:
  # 查看主日志最后100行
  cloudsync logs

  # 查看指定任务的日志
  cloudsync logs my-task

  # 实时跟踪任务日志
  cloudsync logs my-task --follow

  # 查看最后50行
  cloudsync logs my-task --lines 50`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var taskName string
			if len(args) > 0 {
				taskName = args[0]
			}
			return runLogs(taskName, follow, lines)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "实时跟踪日志输出")
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "显示的最后行数")

	return cmd
}

func runLogs(taskName string, follow bool, lines int) error {
	logDir := config.GetLogDir()

	var logPath string
	if taskName == "" {
		// Main log file
		logPath = filepath.Join(logDir, "cloudsync.log")
	} else {
		// Task-specific log file
		logPath = filepath.Join(logDir, "tasks", taskName+".log")
	}

	// Check if file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		if taskName == "" {
			return fmt.Errorf("main log file not found: %s", logPath)
		}
		return fmt.Errorf("log file not found for task: %s", taskName)
	}

	// Open file
	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Read and print last N lines
	if lines > 0 {
		fileLines, err := readLastLines(file, lines)
		if err != nil {
			return fmt.Errorf("failed to read log file: %w", err)
		}
		for _, line := range fileLines {
			fmt.Println(line)
		}
	}

	// Follow mode
	if follow {
		if taskName == "" {
			fmt.Println("\n--- Following main log (Ctrl+C to exit) ---")
		} else {
			fmt.Printf("\n--- Following %s task log (Ctrl+C to exit) ---\n", taskName)
		}

		// Seek to end of file
		file.Seek(0, 2)

		reader := bufio.NewReader(file)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				// Wait a bit and try again
				continue
			}
			fmt.Print(line)
		}
	}

	return nil
}

// readLastLines reads the last n lines from a file using a ring buffer approach
func readLastLines(file *os.File, n int) ([]string, error) {
	if n <= 0 {
		n = 100
	}

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	size := stat.Size()
	if size == 0 {
		return []string{}, nil
	}

	// Read file in chunks from the end
	const chunkSize = 4096
	lines := make([]string, 0, n)
	lineBuf := make([]byte, 0, 256)
	buffer := make([]byte, 0, chunkSize)

	pos := size
	for pos > 0 && len(lines) < n {
		// Calculate chunk to read
		start := pos - chunkSize
		if start < 0 {
			start = 0
		}
		chunkLen := pos - start

		// Read chunk
		chunk := make([]byte, chunkLen)
		_, err := file.ReadAt(chunk, start)
		if err != nil {
			return nil, err
		}

		// Prepend to buffer
		buffer = append(chunk, buffer...)

		// Process buffer for lines
		for i := len(buffer) - 1; i >= 0 && len(lines) < n; i-- {
			if buffer[i] == '\n' {
				if len(lineBuf) > 0 {
					// Reverse lineBuf to get correct order
					for j, k := 0, len(lineBuf)-1; j < k; j, k = j+1, k-1 {
						lineBuf[j], lineBuf[k] = lineBuf[k], lineBuf[j]
					}
					lines = append([]string{string(lineBuf)}, lines...)
					lineBuf = lineBuf[:0]
				}
			} else if buffer[i] != '\r' {
				lineBuf = append(lineBuf, buffer[i])
			}
		}

		buffer = buffer[:0]
		pos = start
	}

	// Handle remaining line buffer
	if len(lineBuf) > 0 && len(lines) < n {
		// Reverse lineBuf
		for j, k := 0, len(lineBuf)-1; j < k; j, k = j+1, k-1 {
			lineBuf[j], lineBuf[k] = lineBuf[k], lineBuf[j]
		}
		lines = append([]string{string(lineBuf)}, lines...)
	}

	return lines, nil
}
