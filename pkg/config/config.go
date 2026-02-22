package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

// Config represents the main configuration structure
type Config struct {
	Global    GlobalConfig    `yaml:"global"`
	Storage   StorageConfig   `yaml:"storage"`
	Tasks     []TaskConfig    `yaml:"tasks"`
	Retention RetentionConfig `yaml:"retention"`
	Notify    NotifyConfig    `yaml:"notify"`
}

// GlobalConfig represents global settings
type GlobalConfig struct {
	LogLevel  string `yaml:"log_level"`
	LogFormat string `yaml:"log_format"`
}

// StorageConfig represents storage backend configuration
type StorageConfig struct {
	Type string    `yaml:"type"` // s3 or oss
	S3   S3Config  `yaml:"s3"`
	OSS  OSSConfig `yaml:"oss"`
}

// S3Config represents AWS S3 configuration
type S3Config struct {
	Region    string `yaml:"region"`
	Bucket    string `yaml:"bucket"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	Endpoint  string `yaml:"endpoint,omitempty"` // For S3-compatible services
	Encryption string `yaml:"encryption,omitempty"`
}

// OSSConfig represents Aliyun OSS configuration
type OSSConfig struct {
	Endpoint        string `yaml:"endpoint"`
	Bucket          string `yaml:"bucket"`
	AccessKeyID     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
}

// TaskConfig represents a sync task configuration
type TaskConfig struct {
	Name        string            `yaml:"name"`
	Schedule    string            `yaml:"schedule"`
	Source      SourceConfig      `yaml:"source"`
	Target      TargetConfig      `yaml:"target"`
	Compression CompressionConfig `yaml:"compression,omitempty"`
	Retention   *RetentionPolicy  `yaml:"retention,omitempty"`
	Notify      *NotifySettings   `yaml:"notify,omitempty"`
}

// SourceConfig represents source file configuration
type SourceConfig struct {
	Path            string   `yaml:"path"`
	Include         []string `yaml:"include,omitempty"`
	Exclude         []string `yaml:"exclude,omitempty"`
}

// TargetConfig represents target storage configuration
type TargetConfig struct {
	Prefix      string `yaml:"prefix"`
	DateFormat  string `yaml:"date_format,omitempty"`
}

// CompressionConfig represents compression settings
type CompressionConfig struct {
	Enabled            bool     `yaml:"enabled"`
	Type               string   `yaml:"type"` // gzip or zstd
	Level              int      `yaml:"level"`
	MinSize            int64    `yaml:"min_size,omitempty"`
	IncludeExtensions  []string `yaml:"include_extensions,omitempty"`
	ExcludeExtensions  []string `yaml:"exclude_extensions,omitempty"`
}

// RetentionConfig represents global retention settings
type RetentionConfig struct {
	MaxDays    int `yaml:"max_days"`
	MaxVersions int `yaml:"max_versions"`
}

// RetentionPolicy represents task-specific retention settings
type RetentionPolicy struct {
	MaxDays     int `yaml:"max_days,omitempty"`
	MaxVersions int `yaml:"max_versions,omitempty"`
}

// NotifyConfig represents global notification settings
type NotifyConfig struct {
	Enabled  bool   `yaml:"enabled"`
	APIURL   string `yaml:"api_url,omitempty"`
	MsgType  string `yaml:"msg_type,omitempty"`
	HTMLHeight int `yaml:"html_height,omitempty"`
}

// NotifySettings represents task-specific notification settings
type NotifySettings struct {
	Enabled string `yaml:"enabled,omitempty"` // always, on-error, never
}

// GetConfigDir returns the configuration directory path
func GetConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cloudsync"
	}
	return filepath.Join(home, ".cloudsync")
}

// GetConfigPath returns the full path to the config file
func GetConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.yaml")
}

// Load loads configuration from the specified file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate storage configuration
	if err := c.validateStorage(); err != nil {
		return err
	}

	// Validate tasks
	if len(c.Tasks) == 0 {
		return fmt.Errorf("at least one task must be configured")
	}

	taskNames := make(map[string]bool)
	for i, task := range c.Tasks {
		if err := c.validateTask(task, i); err != nil {
			return err
		}
		if taskNames[task.Name] {
			return fmt.Errorf("duplicate task name: %s", task.Name)
		}
		taskNames[task.Name] = true
	}

	return nil
}

func (c *Config) validateStorage() error {
	switch strings.ToLower(c.Storage.Type) {
	case "s3":
		if c.Storage.S3.Bucket == "" {
			return fmt.Errorf("S3 bucket is required")
		}
		if c.Storage.S3.Region == "" {
			return fmt.Errorf("S3 region is required")
		}
	case "oss":
		if c.Storage.OSS.Bucket == "" {
			return fmt.Errorf("OSS bucket is required")
		}
		if c.Storage.OSS.Endpoint == "" {
			return fmt.Errorf("OSS endpoint is required")
		}
	default:
		return fmt.Errorf("unsupported storage type: %s (must be 's3' or 'oss')", c.Storage.Type)
	}
	return nil
}

func (c *Config) validateTask(task TaskConfig, index int) error {
	if task.Name == "" {
		return fmt.Errorf("task %d: name is required", index)
	}
	if task.Schedule == "" {
		return fmt.Errorf("task %s: schedule is required", task.Name)
	}
	if task.Source.Path == "" {
		return fmt.Errorf("task %s: source.path is required", task.Name)
	}
	if task.Target.Prefix == "" {
		return fmt.Errorf("task %s: target.prefix is required", task.Name)
	}

	// Validate schedule expression
	if err := validateSchedule(task.Schedule); err != nil {
		return fmt.Errorf("task %s: invalid schedule: %w", task.Name, err)
	}

	// Validate date format if provided
	if task.Target.DateFormat != "" {
		// The format will be validated at runtime
		// We just check it's not empty here
	}

	return nil
}

// validateSchedule validates a cron schedule expression
func validateSchedule(schedule string) error {
	parser := cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)
	_, err := parser.Parse(schedule)
	if err != nil {
		return fmt.Errorf("invalid schedule: %w", err)
	}
	return nil
}
