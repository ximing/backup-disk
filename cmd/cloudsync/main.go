package main

import (
	"fmt"
	"os"

	"github.com/ximing/cloudsync/pkg/config"
	"github.com/ximing/cloudsync/pkg/logger"
)

func main() {
	if err := Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// Execute runs the root command
func Execute() error {
	// Initialize root command
	rootCmd := NewRootCommand()
	return rootCmd.Execute()
}

// ConfigDir returns the configuration directory path
func ConfigDir() string {
	return config.GetConfigDir()
}

// Logger returns the global logger instance
func Logger() *logger.Logger {
	return logger.GetLogger()
}
