package scheduler

import (
	"fmt"
	"strings"
)

// StandardPresets defines common cron presets and their descriptions
var StandardPresets = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

// IsValidPreset checks if a schedule string is a valid preset
func IsValidPreset(schedule string) bool {
	_, ok := StandardPresets[schedule]
	return ok
}

// ExpandPreset expands a preset to its cron expression equivalent
// Returns the original expression if it's not a preset
func ExpandPreset(schedule string) string {
	if expanded, ok := StandardPresets[schedule]; ok {
		return expanded
	}
	return schedule
}

// GetPresetDescription returns a human-readable description of a preset
func GetPresetDescription(schedule string) string {
	descriptions := map[string]string{
		"@yearly":   "Once a year at midnight on January 1st",
		"@annually": "Once a year at midnight on January 1st",
		"@monthly":  "Once a month at midnight on the first day",
		"@weekly":   "Once a week at midnight on Sunday",
		"@daily":    "Once a day at midnight",
		"@midnight": "Once a day at midnight",
		"@hourly":   "At the beginning of every hour",
	}

	if desc, ok := descriptions[schedule]; ok {
		return desc
	}
	return ""
}

// CronField represents a field in a cron expression
type CronField struct {
	Name     string
	Min      int
	Max      int
	Examples []string
}

// CronFields defines the standard cron fields
var CronFields = []CronField{
	{
		Name:     "minute",
		Min:      0,
		Max:      59,
		Examples: []string{"0", "30", "*/5", "0,15,30,45"},
	},
	{
		Name:     "hour",
		Min:      0,
		Max:      23,
		Examples: []string{"0", "2", "*/6", "9-17"},
	},
	{
		Name:     "day of month",
		Min:      1,
		Max:      31,
		Examples: []string{"*", "1", "15", "L"},
	},
	{
		Name:     "month",
		Min:      1,
		Max:      12,
		Examples: []string{"*", "1", "1-6", "JAN"},
	},
	{
		Name:     "day of week",
		Min:      0,
		Max:      6,
		Examples: []string{"*", "0", "1-5", "MON-FRI"},
	},
}

// DescribeSchedule returns a human-readable description of a schedule
// Note: This is a simplified description generator
func DescribeSchedule(schedule string) string {
	// Handle presets
	if IsValidPreset(schedule) {
		return GetPresetDescription(schedule)
	}

	// Basic description for cron expressions
	parts := strings.Fields(schedule)
	if len(parts) != 5 {
		return "Custom schedule"
	}

	// Simple pattern matching for common schedules
	if parts[0] == "0" && parts[1] == "0" && parts[2] == "*" && parts[3] == "*" && parts[4] == "*" {
		return "Daily at midnight"
	}
	if parts[0] == "0" && parts[1] == "*" && parts[2] == "*" && parts[3] == "*" && parts[4] == "*" {
		return "At the beginning of every hour"
	}
	if parts[0] == "0" && parts[1] == "0" && parts[2] == "*" && parts[3] == "*" && parts[4] == "0" {
		return "Weekly on Sunday at midnight"
	}
	if parts[0] == "0" && parts[1] == "0" && parts[2] == "1" && parts[3] == "*" && parts[4] == "*" {
		return "Monthly on the 1st at midnight"
	}

	// Check for specific time pattern
	if parts[2] == "*" && parts[3] == "*" && parts[4] == "*" {
		return fmt.Sprintf("Daily at %s:%s", parts[1], parts[0])
	}

	return "Custom schedule: " + schedule
}
