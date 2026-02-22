// Package notify provides notification functionality for backup tasks
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"
	"time"

	"github.com/ximing/cloudsync/pkg/logger"
)

// Level represents the notification level
type Level string

const (
	// LevelAlways always send notification
	LevelAlways Level = "always"
	// LevelOnError only send notification on error
	LevelOnError Level = "on-error"
	// LevelNever never send notification
	LevelNever Level = "never"
)

// Result represents a task execution result for notification
type Result struct {
	TaskName     string
	Status       string
	Duration     time.Duration
	FileCount    int
	SuccessCount int
	FailedCount  int
	SkippedCount int
	BytesTotal   int64
	BytesSuccess int64
	Error        string
	StartTime    time.Time
	EndTime      time.Time
}

// Notifier is the interface for notification implementations
type Notifier interface {
	// Send sends a notification with the given result
	Send(ctx context.Context, result *Result) error
	// ShouldNotify returns true if notification should be sent for the given result
	ShouldNotify(result *Result) bool
}

// Config represents notification configuration
type Config struct {
	Enabled    bool
	APIURL     string
	MsgType    string
	HTMLHeight int
	Level      Level
	Template   string
}

// DefaultConfig returns default notification configuration
func DefaultConfig() Config {
	return Config{
		Enabled:    false,
		MsgType:    "html",
		HTMLHeight: 350,
		Level:      LevelOnError,
		Template:   DefaultTemplate,
	}
}

// DefaultTemplate is the default notification message template
const DefaultTemplate = `<h3>备份任务 {{.TaskName}} {{if eq .Status "success"}}成功{{else}}失败{{end}}</h3>
<p><strong>状态:</strong> {{.Status}}</p>
<p><strong>耗时:</strong> {{.Duration}}</p>
<p><strong>文件数:</strong> {{.SuccessCount}}/{{.FileCount}} 成功{{if gt .FailedCount 0}}, {{.FailedCount}} 失败{{end}}</p>
<p><strong>数据量:</strong> {{formatBytes .BytesSuccess}}</p>
{{if .Error}}<p style="color: red;"><strong>错误:</strong> {{.Error}}</p>{{end}}
<p><small>开始时间: {{.StartTime.Format "2006-01-02 15:04:05"}}</small></p>`

// templateFuncs provides helper functions for templates
var templateFuncs = template.FuncMap{
	"formatBytes": formatBytes,
}

func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// MeoWNotifier implements Notifier for MeoW push service
type MeoWNotifier struct {
	config Config
	client *http.Client
	logger *logger.Logger
}

// NewMeoWNotifier creates a new MeoW notifier
func NewMeoWNotifier(config Config, logger *logger.Logger) *MeoWNotifier {
	return &MeoWNotifier{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// meoWRequest represents the request body for MeoW API
type meoWRequest struct {
	Title string `json:"title"`
	Msg   string `json:"msg"`
	URL   string `json:"url,omitempty"`
}

// ShouldNotify returns true if notification should be sent for the given result
func (n *MeoWNotifier) ShouldNotify(result *Result) bool {
	if !n.config.Enabled {
		return false
	}

	switch n.config.Level {
	case LevelNever:
		return false
	case LevelAlways:
		return true
	case LevelOnError:
		return result.Status != "success"
	default:
		return result.Status != "success"
	}
}

// Send sends a notification to MeoW
func (n *MeoWNotifier) Send(ctx context.Context, result *Result) error {
	if !n.config.Enabled {
		return nil
	}

	// Build request URL
	url := fmt.Sprintf("%s?msgType=%s&htmlHeight=%d",
		n.config.APIURL,
		n.config.MsgType,
		n.config.HTMLHeight,
	)

	// Render message template
	msg, err := n.renderTemplate(result)
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	// Build request body
	reqBody := meoWRequest{
		Title: n.buildTitle(result),
		Msg:   msg,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("meow API returned status %d", resp.StatusCode)
	}

	return nil
}

// buildTitle builds the notification title
func (n *MeoWNotifier) buildTitle(result *Result) string {
	statusText := "成功"
	if result.Status != "success" {
		statusText = "失败"
	}
	return fmt.Sprintf("备份任务 %s %s", result.TaskName, statusText)
}

// renderTemplate renders the notification message template
func (n *MeoWNotifier) renderTemplate(result *Result) (string, error) {
	tmpl, err := template.New("notify").Funcs(templateFuncs).Parse(n.config.Template)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, result); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// NewNotifier creates a notifier based on the configuration
func NewNotifier(config Config, logger *logger.Logger) Notifier {
	return NewMeoWNotifier(config, logger)
}

// ParseLevel parses a level string
func ParseLevel(s string) Level {
	switch s {
	case "always":
		return LevelAlways
	case "on-error", "on_error":
		return LevelOnError
	case "never":
		return LevelNever
	default:
		return LevelOnError
	}
}
