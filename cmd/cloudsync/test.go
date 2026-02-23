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
	fmt.Printf("Configured Backends: %d\n\n", len(cfg.Storage))

	allSuccess := true
	var totalElapsed time.Duration

	for _, backend := range cfg.Storage {
		fmt.Printf("Backend: %s (%s)\n", backend.Name, backend.Type)
		switch backend.Type {
		case "s3":
			fmt.Printf("  Region: %s\n", backend.S3.Region)
			fmt.Printf("  Bucket: %s\n", backend.S3.Bucket)
			if backend.S3.Endpoint != "" {
				fmt.Printf("  Endpoint: %s\n", backend.S3.Endpoint)
			}
		case "oss":
			fmt.Printf("  Endpoint: %s\n", backend.OSS.Endpoint)
			fmt.Printf("  Bucket: %s\n", backend.OSS.Bucket)
		}
		fmt.Println()

		// Create storage instance
		fmt.Printf("Testing connection...\n")
		store, err := storage.NewStorageFromBackend(backend)
		if err != nil {
			fmt.Printf("  FAILED: %v\n\n", err)
			allSuccess = false
			continue
		}

		// Test connection with timeout
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)

		start := time.Now()
		err = store.Validate(ctx)
		elapsed := time.Since(start)
		totalElapsed += elapsed
		cancel()

		if err != nil {
			fmt.Println("  FAILED")
			fmt.Println()

			// Provide helpful error messages
			switch {
			case isTimeoutError(err):
				fmt.Println("  Error: Connection timeout")
				fmt.Println("    - Check your network connection")
				fmt.Println("    - Verify the endpoint URL is correct")
				fmt.Println("    - Check if a firewall is blocking the connection")
			case isCredentialError(err):
				fmt.Println("  Error: Invalid credentials")
				fmt.Println("    - Check your access key and secret key")
				fmt.Println("    - Verify the credentials have not expired")
				fmt.Println("    - Ensure the credentials have sufficient permissions")
			case isBucketError(err):
				fmt.Println("  Error: Bucket not accessible")
				fmt.Println("    - Verify the bucket name is correct")
				fmt.Println("    - Check if the bucket exists")
				fmt.Println("    - Ensure you have permission to access this bucket")
			default:
				fmt.Printf("  Error: %v\n", err)
			}
			allSuccess = false
		} else {
			fmt.Printf("  SUCCESS (response time: %v)\n", elapsed.Round(time.Millisecond))
		}
		fmt.Println()
	}

	fmt.Println()
	if allSuccess {
		fmt.Printf("All backends tested successfully (total time: %v)\n", totalElapsed.Round(time.Millisecond))
		fmt.Println("Configuration is ready for sync operations.")
	} else {
		return fmt.Errorf("one or more backend tests failed")
	}

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
