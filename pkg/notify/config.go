package notify

import (
	"github.com/ximing/cloudsync/pkg/config"
)

// BuildConfig builds notify.Config from the main config and task settings
func BuildConfig(globalConfig config.NotifyConfig, taskNotify *config.NotifySettings, taskName string) Config {
	cfg := DefaultConfig()

	// Apply global settings
	if globalConfig.Enabled {
		cfg.Enabled = true
	}
	if globalConfig.APIURL != "" {
		cfg.APIURL = globalConfig.APIURL
	}
	if globalConfig.MsgType != "" {
		cfg.MsgType = globalConfig.MsgType
	}
	if globalConfig.HTMLHeight > 0 {
		cfg.HTMLHeight = globalConfig.HTMLHeight
	}

	// Apply task-specific settings
	if taskNotify != nil && taskNotify.Enabled != "" {
		cfg.Level = ParseLevel(taskNotify.Enabled)
	}

	// If task-specific setting disables notification, respect it
	if taskNotify != nil && taskNotify.Enabled == "never" {
		cfg.Enabled = false
	}

	return cfg
}
