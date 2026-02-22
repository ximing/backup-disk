package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ximing/cloudsync/pkg/config"
)

// NewValidateCommand creates the validate command
func NewValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "验证配置文件",
		Long: `验证 CloudSync 配置文件的语法和基本逻辑。

检查项包括:
  - YAML 语法是否正确
  - 必需的配置字段是否存在
  - 存储后端配置是否有效
  - 任务配置是否有效`,
		RunE: runValidate,
	}
}

func runValidate(cmd *cobra.Command, args []string) error {
	configPath := config.GetConfigPath()

	fmt.Printf("Validating config: %s\n\n", configPath)

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	fmt.Println("Configuration is valid!")
	fmt.Printf("\nLoaded %d task(s):\n", len(cfg.Tasks))
	for _, task := range cfg.Tasks {
		fmt.Printf("  - %s: %s -> %s\n", task.Name, task.Source.Path, task.Target.Prefix)
	}

	return nil
}
