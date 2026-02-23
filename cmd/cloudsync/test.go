package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/ximing/cloudsync/pkg/config"
	"github.com/ximing/cloudsync/pkg/storage"
)

// NewTestCommand creates the test command
func NewTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "测试存储后端连接",
		Long: `测试 S3 或 OSS 存储后端的连接是否正常。

验证项包括:
  - 凭证是否有效
  - Bucket 是否存在且可访问
  - 网络连接是否正常

示例:
  cloudsync test              # 测试配置文件中指定的存储后端
  cloudsync test --timeout 30 # 设置超时时间为 30 秒`,
		RunE: runTest,
	}

	cmd.Flags().Int("timeout", 10, "连接超时时间(秒)")

	return cmd
}

func runTest(cmd *cobra.Command, args []string) error {
	timeout, _ := cmd.Flags().GetInt("timeout")
	configPath := config.GetConfigPath()

	fmt.Printf("Loading config: %s\n\n", configPath)

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Display storage configuration
	fmt.Printf("Storage Type: %s\n", cfg.Storage.Type)
	switch cfg.Storage.Type {
	case "s3":
		fmt.Printf("  Region: %s\n", cfg.Storage.S3.Region)
		fmt.Printf("  Bucket: %s\n", cfg.Storage.S3.Bucket)
		if cfg.Storage.S3.Endpoint != "" {
			fmt.Printf("  Endpoint: %s\n", cfg.Storage.S3.Endpoint)
		}
	case "oss":
		fmt.Printf("  Endpoint: %s\n", cfg.Storage.OSS.Endpoint)
		fmt.Printf("  Bucket: %s\n", cfg.Storage.OSS.Bucket)
	}
	fmt.Println()

	// Create storage instance
	fmt.Printf("Creating storage client...\n")
	store, err := storage.NewStorage(cfg)
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}

	// Test connection with timeout
	fmt.Printf("Testing connection (timeout: %ds)...\n", timeout)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	start := time.Now()
	if err := store.Validate(ctx); err != nil {
		fmt.Println()
		fmt.Println("Connection test FAILED")
		fmt.Println()

		// Provide helpful error messages
		switch {
		case isTimeoutError(err):
			fmt.Println("Error: Connection timeout")
			fmt.Println("  - Check your network connection")
			fmt.Println("  - Verify the endpoint URL is correct")
			fmt.Println("  - Check if a firewall is blocking the connection")
		case isCredentialError(err):
			fmt.Println("Error: Invalid credentials")
			fmt.Println("  - Check your access key and secret key")
			fmt.Println("  - Verify the credentials have not expired")
			fmt.Println("  - Ensure the credentials have sufficient permissions")
		case isBucketError(err):
			fmt.Println("Error: Bucket not accessible")
			fmt.Println("  - Verify the bucket name is correct")
			fmt.Println("  - Check if the bucket exists")
			fmt.Println("  - Ensure you have permission to access this bucket")
		default:
			fmt.Printf("Error: %v\n", err)
		}
		return fmt.Errorf("connection test failed")
	}

	elapsed := time.Since(start)
	fmt.Println()
	fmt.Println("Connection test SUCCESS!")
	fmt.Printf("  Response time: %v\n", elapsed.Round(time.Millisecond))
	fmt.Println()
	fmt.Println("Configuration is ready for sync operations.")

	return nil
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return containsAny(errStr, []string{"timeout", "deadline exceeded", "context deadline", "i/o timeout", "connection timed out"})
}

func isCredentialError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return containsAny(errStr, []string{"invalid credentials", "invalidaccesskeyid", "signaturedoesnotmatch", "accesskeyidnotfound", "invalid access key"})
}

func isBucketError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return containsAny(errStr, []string{"bucket not found", "nosuchbucket", "bucket not accessible", "not exist"})
}

func containsAny(s string, substrs []string) bool {
	lowerS := ""
	for _, substr := range substrs {
		if lowerS == "" {
			lowerS = toLower(s)
		}
		if contains(lowerS, toLower(substr)) {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	// Simple ASCII lowercase
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsInternal(s, substr))
}

func containsInternal(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
