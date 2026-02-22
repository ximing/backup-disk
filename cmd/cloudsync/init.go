package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/ximing/cloudsync/pkg/config"
)

// NewInitCommand creates the init command
func NewInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "初始化 CloudSync 配置",
		Long: `初始化 CloudSync 配置文件和目录结构。

该命令会:
  1. 创建 ~/.cloudsync/ 目录
  2. 创建日志目录 ~/.cloudsync/logs/
  3. 创建数据目录 ~/.cloudsync/data/
  4. 生成默认配置文件 ~/.cloudsync/config.yaml`,
		RunE: runInit,
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	configDir := config.GetConfigDir()

	// Create directories
	dirs := []string{
		configDir,
		filepath.Join(configDir, "logs"),
		filepath.Join(configDir, "logs", "tasks"),
		filepath.Join(configDir, "data"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		fmt.Printf("Created directory: %s\n", dir)
	}

	// Check if config already exists
	configPath := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config file already exists: %s\n", configPath)
		fmt.Println("Use 'cloudsync validate' to check the configuration.")
		return nil
	}

	// Write default config
	if err := os.WriteFile(configPath, []byte(config.DefaultConfig), 0644); err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	fmt.Printf("Created config file: %s\n", configPath)
	fmt.Println("\n请编辑配置文件，设置您的存储后端和同步任务。")
	fmt.Println("然后运行 'cloudsync validate' 验证配置。")

	return nil
}
