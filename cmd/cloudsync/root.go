package main

import (
	"github.com/spf13/cobra"
)

// NewRootCommand creates the root command
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "cloudsync",
		Short: "本地磁盘到 S3/OSS 同步工具",
		Long: `CloudSync CLI - 支持定时调度、压缩策略、保留策略和 MeoW 推送通知的本地磁盘到 S3/OSS 同步工具。

支持的存储后端:
  - AWS S3
  - 阿里云 OSS

主要功能:
  - 多任务配置与管理
  - Cron 定时调度
  - gzip/zstd 压缩
  - 保留策略自动清理
  - MeoW 推送通知`,
		SilenceUsage: true,
	}

	// Add subcommands
	rootCmd.AddCommand(NewInitCommand())
	rootCmd.AddCommand(NewValidateCommand())
	rootCmd.AddCommand(NewSyncCommand())
	rootCmd.AddCommand(NewDaemonCommand())
	rootCmd.AddCommand(NewStatusCommand())
	rootCmd.AddCommand(NewLogsCommand())
	rootCmd.AddCommand(NewHistoryCommand())

	return rootCmd
}
