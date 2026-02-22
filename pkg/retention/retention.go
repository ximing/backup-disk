// Package retention provides backup retention policy management and cleanup
package retention

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ximing/cloudsync/pkg/storage"
)

// Policy represents a retention policy for backups
type Policy struct {
	MaxDays    int // Maximum number of days to keep backups
	MaxVersions int // Maximum number of backup versions to keep
}

// BackupInfo represents information about a backup
type BackupInfo struct {
	Path        string
	Time        time.Time
	Size        int64
	ObjectCount int
}

// CleanupResult represents the result of a cleanup operation
type CleanupResult struct {
	DeletedBackups []BackupInfo
	TotalSize      int64
	DryRun         bool
}

// Manager handles retention policy enforcement
type Manager struct {
	storage storage.Storage
	policy  Policy
}

// NewManager creates a new retention manager
func NewManager(store storage.Storage, policy Policy) *Manager {
	return &Manager{
		storage: store,
		policy:  policy,
	}
}

// ListBackups lists all backups under the given prefix
// It groups objects by date directory pattern: {prefix}/{YYYY}/{MM}/{DD}/{HHmmss}/
func (m *Manager) ListBackups(ctx context.Context, prefix string) ([]BackupInfo, error) {
	// List all objects under the prefix
	objects, err := m.storage.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	// Group objects by backup directory
	backupMap := make(map[string]*BackupInfo)

	for _, obj := range objects {
		// Parse the path to extract backup time
		backupTime, backupPath := parseBackupPath(obj.Key, prefix)
		if backupTime.IsZero() {
			// Not a valid backup path, skip
			continue
		}

		info, exists := backupMap[backupPath]
		if !exists {
			info = &BackupInfo{
				Path: backupPath,
				Time: backupTime,
			}
			backupMap[backupPath] = info
		}

		info.Size += obj.Size
		info.ObjectCount++
	}

	// Convert map to slice
	backups := make([]BackupInfo, 0, len(backupMap))
	for _, info := range backupMap {
		backups = append(backups, *info)
	}

	// Sort by time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Time.After(backups[j].Time)
	})

	return backups, nil
}

// parseBackupPath extracts the backup time from a path
// Expected format: {prefix}/{YYYY}/{MM}/{DD}/{HHmmss}/filename
func parseBackupPath(objectKey, prefix string) (time.Time, string) {
	// Remove prefix from the path
	relativePath := strings.TrimPrefix(objectKey, prefix)
	relativePath = strings.TrimPrefix(relativePath, "/")

	// Split the path into components
	parts := strings.Split(relativePath, "/")
	if len(parts) < 4 {
		return time.Time{}, ""
	}

	// Parse year, month, day
	year, err := strconv.Atoi(parts[0])
	if err != nil || year < 2000 || year > 3000 {
		return time.Time{}, ""
	}

	month, err := strconv.Atoi(parts[1])
	if err != nil || month < 1 || month > 12 {
		return time.Time{}, ""
	}

	day, err := strconv.Atoi(parts[2])
	if err != nil || day < 1 || day > 31 {
		return time.Time{}, ""
	}

	// Parse time component (HHmmss)
	if len(parts) < 4 {
		return time.Time{}, ""
	}

	timeStr := parts[3]
	if len(timeStr) != 6 {
		return time.Time{}, ""
	}

	hour, err := strconv.Atoi(timeStr[0:2])
	if err != nil || hour < 0 || hour > 23 {
		return time.Time{}, ""
	}

	minute, err := strconv.Atoi(timeStr[2:4])
	if err != nil || minute < 0 || minute > 59 {
		return time.Time{}, ""
	}

	second, err := strconv.Atoi(timeStr[4:6])
	if err != nil || second < 0 || second > 59 {
		return time.Time{}, ""
	}

	// Construct the backup directory path
	backupPath := path.Join(prefix, parts[0], parts[1], parts[2], parts[3])

	// Create time object
	backupTime := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)

	return backupTime, backupPath
}

// Cleanup performs cleanup based on retention policy
// Returns a list of deleted backups and total freed space
func (m *Manager) Cleanup(ctx context.Context, prefix string, dryRun bool) (*CleanupResult, error) {
	backups, err := m.ListBackups(ctx, prefix)
	if err != nil {
		return nil, err
	}

	if len(backups) == 0 {
		return &CleanupResult{
			DeletedBackups: []BackupInfo{},
			TotalSize:      0,
			DryRun:         dryRun,
		}, nil
	}

	// Determine which backups to delete
	toDelete := m.selectBackupsForDeletion(backups)

	result := &CleanupResult{
		DeletedBackups: toDelete,
		TotalSize:      0,
		DryRun:         dryRun,
	}

	for _, backup := range toDelete {
		result.TotalSize += backup.Size

		if !dryRun {
			// Delete all objects under this backup path
			if err := m.deleteBackup(ctx, backup.Path); err != nil {
				return result, fmt.Errorf("failed to delete backup %s: %w", backup.Path, err)
			}
		}
	}

	return result, nil
}

// selectBackupsForDeletion selects backups to delete based on retention policy
func (m *Manager) selectBackupsForDeletion(backups []BackupInfo) []BackupInfo {
	if len(backups) == 0 {
		return nil
	}

	// Create a set of backups to delete
	deleteSet := make(map[int]bool)

	// Apply max_days policy
	if m.policy.MaxDays > 0 {
		cutoffTime := time.Now().UTC().AddDate(0, 0, -m.policy.MaxDays)
		for i, backup := range backups {
			if backup.Time.Before(cutoffTime) {
				deleteSet[i] = true
			}
		}
	}

	// Apply max_versions policy
	if m.policy.MaxVersions > 0 {
		// Count non-deleted backups
		validVersions := 0
		for i := 0; i < len(backups); i++ {
			if !deleteSet[i] {
				validVersions++
				if validVersions > m.policy.MaxVersions {
					deleteSet[i] = true
				}
			}
		}
	}

	// Build result list
	var toDelete []BackupInfo
	for i := range backups {
		if deleteSet[i] {
			toDelete = append(toDelete, backups[i])
		}
	}

	return toDelete
}

// deleteBackup deletes all objects under a backup path
func (m *Manager) deleteBackup(ctx context.Context, backupPath string) error {
	// List all objects under the backup path
	objects, err := m.storage.List(ctx, backupPath)
	if err != nil {
		return fmt.Errorf("failed to list objects in backup: %w", err)
	}

	// Delete each object
	for _, obj := range objects {
		if err := m.storage.Delete(ctx, obj.Key); err != nil {
			return fmt.Errorf("failed to delete object %s: %w", obj.Key, err)
		}
	}

	return nil
}

// GetPolicy returns the current retention policy
func (m *Manager) GetPolicy() Policy {
	return m.policy
}

// UpdatePolicy updates the retention policy
func (m *Manager) UpdatePolicy(policy Policy) {
	m.policy = policy
}
