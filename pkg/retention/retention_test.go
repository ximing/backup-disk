package retention

import (
	"context"
	"testing"
	"time"

	"github.com/ximing/cloudsync/pkg/storage"
)

// mockStorage is a mock implementation of storage.Storage for testing
type mockStorage struct {
	objects []storage.ObjectInfo
	deleted []string
}

func (m *mockStorage) Upload(ctx context.Context, localPath string, remotePath string) error {
	return nil
}

func (m *mockStorage) List(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	var result []storage.ObjectInfo
	for _, obj := range m.objects {
		if len(obj.Key) >= len(prefix) && obj.Key[:len(prefix)] == prefix {
			result = append(result, obj)
		}
	}
	return result, nil
}

func (m *mockStorage) Delete(ctx context.Context, remotePath string) error {
	m.deleted = append(m.deleted, remotePath)
	return nil
}

func (m *mockStorage) Validate(ctx context.Context) error {
	return nil
}

func TestParseBackupPath(t *testing.T) {
	tests := []struct {
		name       string
		objectKey  string
		prefix     string
		wantTime   time.Time
		wantPath   string
	}{
		{
			name:      "valid path with prefix",
			objectKey: "backups/mydata/2024/03/15/143022/file.txt",
			prefix:    "backups/mydata",
			wantTime:  time.Date(2024, 3, 15, 14, 30, 22, 0, time.UTC),
			wantPath:  "backups/mydata/2024/03/15/143022",
		},
		{
			name:      "valid path with trailing slash prefix",
			objectKey: "backups/data/2023/12/25/080000/backup.tar.gz",
			prefix:    "backups/data/",
			wantTime:  time.Date(2023, 12, 25, 8, 0, 0, 0, time.UTC),
			wantPath:  "backups/data/2023/12/25/080000",
		},
		{
			name:      "invalid path - too few components",
			objectKey: "backups/2024/03/file.txt",
			prefix:    "backups",
			wantTime:  time.Time{},
			wantPath:  "",
		},
		{
			name:      "invalid path - invalid year",
			objectKey: "backups/mydata/abc/03/15/143022/file.txt",
			prefix:    "backups/mydata",
			wantTime:  time.Time{},
			wantPath:  "",
		},
		{
			name:      "invalid path - invalid month",
			objectKey: "backups/mydata/2024/13/15/143022/file.txt",
			prefix:    "backups/mydata",
			wantTime:  time.Time{},
			wantPath:  "",
		},
		{
			name:      "invalid path - invalid day",
			objectKey: "backups/mydata/2024/03/32/143022/file.txt",
			prefix:    "backups/mydata",
			wantTime:  time.Time{},
			wantPath:  "",
		},
		{
			name:      "invalid path - invalid time format",
			objectKey: "backups/mydata/2024/03/15/99/file.txt",
			prefix:    "backups/mydata",
			wantTime:  time.Time{},
			wantPath:  "",
		},
		{
			name:      "invalid path - invalid hour",
			objectKey: "backups/mydata/2024/03/15/253022/file.txt",
			prefix:    "backups/mydata",
			wantTime:  time.Time{},
			wantPath:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTime, gotPath := parseBackupPath(tt.objectKey, tt.prefix)
			if !gotTime.Equal(tt.wantTime) {
				t.Errorf("parseBackupPath() gotTime = %v, want %v", gotTime, tt.wantTime)
			}
			if gotPath != tt.wantPath {
				t.Errorf("parseBackupPath() gotPath = %v, want %v", gotPath, tt.wantPath)
			}
		})
	}
}

func TestListBackups(t *testing.T) {
	mock := &mockStorage{
		objects: []storage.ObjectInfo{
			{Key: "backups/mydata/2024/03/15/143022/file1.txt", Size: 100, LastModified: time.Date(2024, 3, 15, 14, 30, 22, 0, time.UTC)},
			{Key: "backups/mydata/2024/03/15/143022/file2.txt", Size: 200, LastModified: time.Date(2024, 3, 15, 14, 30, 22, 0, time.UTC)},
			{Key: "backups/mydata/2024/03/16/080000/file1.txt", Size: 150, LastModified: time.Date(2024, 3, 16, 8, 0, 0, 0, time.UTC)},
			{Key: "backups/mydata/2024/03/16/080000/file2.txt", Size: 250, LastModified: time.Date(2024, 3, 16, 8, 0, 0, 0, time.UTC)},
			{Key: "backups/mydata/2024/03/17/120000/file1.txt", Size: 300, LastModified: time.Date(2024, 3, 17, 12, 0, 0, 0, time.UTC)},
			// Invalid paths that should be ignored
			{Key: "backups/mydata/README.txt", Size: 50, LastModified: time.Now()},
		},
	}

	manager := NewManager(mock, Policy{})
	backups, err := manager.ListBackups(context.Background(), "backups/mydata")
	if err != nil {
		t.Fatalf("ListBackups() error = %v", err)
	}

	if len(backups) != 3 {
		t.Errorf("ListBackups() returned %d backups, want 3", len(backups))
	}

	// Check that backups are sorted by time (newest first)
	if backups[0].Time != time.Date(2024, 3, 17, 12, 0, 0, 0, time.UTC) {
		t.Errorf("First backup should be newest, got %v", backups[0].Time)
	}

	// Check object counts and sizes
	if backups[0].ObjectCount != 1 || backups[0].Size != 300 {
		t.Errorf("Newest backup: ObjectCount=%d, Size=%d, want 1, 300", backups[0].ObjectCount, backups[0].Size)
	}

	if backups[1].ObjectCount != 2 || backups[1].Size != 400 {
		t.Errorf("Second backup: ObjectCount=%d, Size=%d, want 2, 400", backups[1].ObjectCount, backups[1].Size)
	}
}

func TestSelectBackupsForDeletion_MaxDays(t *testing.T) {
	now := time.Now().UTC()

	backups := []BackupInfo{
		{Path: "backup1", Time: now.AddDate(0, 0, -1)},  // 1 day ago
		{Path: "backup2", Time: now.AddDate(0, 0, -5)},  // 5 days ago
		{Path: "backup3", Time: now.AddDate(0, 0, -10)}, // 10 days ago
		{Path: "backup4", Time: now.AddDate(0, 0, -15)}, // 15 days ago
	}

	manager := NewManager(nil, Policy{MaxDays: 7})
	toDelete := manager.selectBackupsForDeletion(backups)

	if len(toDelete) != 2 {
		t.Errorf("selectBackupsForDeletion() returned %d backups, want 2", len(toDelete))
	}

	// Should delete backups older than 7 days (10 and 15 days ago)
	if toDelete[0].Path != "backup3" || toDelete[1].Path != "backup4" {
		t.Errorf("Wrong backups selected for deletion: %v", toDelete)
	}
}

func TestSelectBackupsForDeletion_MaxVersions(t *testing.T) {
	now := time.Now().UTC()

	backups := []BackupInfo{
		{Path: "backup1", Time: now.AddDate(0, 0, -1)},
		{Path: "backup2", Time: now.AddDate(0, 0, -2)},
		{Path: "backup3", Time: now.AddDate(0, 0, -3)},
		{Path: "backup4", Time: now.AddDate(0, 0, -4)},
		{Path: "backup5", Time: now.AddDate(0, 0, -5)},
	}

	manager := NewManager(nil, Policy{MaxVersions: 3})
	toDelete := manager.selectBackupsForDeletion(backups)

	if len(toDelete) != 2 {
		t.Errorf("selectBackupsForDeletion() returned %d backups, want 2", len(toDelete))
	}

	// Should delete oldest backups (backup4 and backup5)
	if toDelete[0].Path != "backup4" || toDelete[1].Path != "backup5" {
		t.Errorf("Wrong backups selected for deletion: %v", toDelete)
	}
}

func TestSelectBackupsForDeletion_Combined(t *testing.T) {
	now := time.Now().UTC()

	backups := []BackupInfo{
		{Path: "backup1", Time: now.AddDate(0, 0, -1)},  // 1 day ago - keep
		{Path: "backup2", Time: now.AddDate(0, 0, -3)},  // 3 days ago - keep
		{Path: "backup3", Time: now.AddDate(0, 0, -5)},  // 5 days ago - delete (exceeds max_versions)
		{Path: "backup4", Time: now.AddDate(0, 0, -10)}, // 10 days ago - delete (exceeds max_days)
	}

	manager := NewManager(nil, Policy{MaxDays: 7, MaxVersions: 2})
	toDelete := manager.selectBackupsForDeletion(backups)

	if len(toDelete) != 2 {
		t.Errorf("selectBackupsForDeletion() returned %d backups, want 2", len(toDelete))
	}

	// Should delete backup3 (exceeds max_versions) and backup4 (exceeds max_days)
	if toDelete[0].Path != "backup3" || toDelete[1].Path != "backup4" {
		t.Errorf("Wrong backups selected for deletion: %v", toDelete)
	}
}

func TestCleanup_DryRun(t *testing.T) {
	now := time.Now().UTC()

	mock := &mockStorage{
		objects: []storage.ObjectInfo{
			{Key: "backups/mydata/" + now.AddDate(0, 0, -1).Format("2006/01/02/150405") + "/file.txt", Size: 100},
			{Key: "backups/mydata/" + now.AddDate(0, 0, -10).Format("2006/01/02/150405") + "/file.txt", Size: 200},
		},
	}

	manager := NewManager(mock, Policy{MaxDays: 7})
	result, err := manager.Cleanup(context.Background(), "backups/mydata", true)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	if !result.DryRun {
		t.Error("Cleanup() DryRun should be true")
	}

	if len(result.DeletedBackups) != 1 {
		t.Errorf("Cleanup() returned %d deletions, want 1", len(result.DeletedBackups))
	}

	if result.TotalSize != 200 {
		t.Errorf("Cleanup() TotalSize = %d, want 200", result.TotalSize)
	}

	// Verify no objects were actually deleted
	if len(mock.deleted) != 0 {
		t.Errorf("Cleanup() deleted %d objects in dry-run mode", len(mock.deleted))
	}
}

func TestCleanup_ActualDeletion(t *testing.T) {
	now := time.Now().UTC()
	oldDate := now.AddDate(0, 0, -10).Format("2006/01/02/150405")

	mock := &mockStorage{
		objects: []storage.ObjectInfo{
			{Key: "backups/mydata/" + now.AddDate(0, 0, -1).Format("2006/01/02/150405") + "/file.txt", Size: 100},
			{Key: "backups/mydata/" + oldDate + "/file1.txt", Size: 200},
			{Key: "backups/mydata/" + oldDate + "/file2.txt", Size: 300},
		},
	}

	manager := NewManager(mock, Policy{MaxDays: 7})
	result, err := manager.Cleanup(context.Background(), "backups/mydata", false)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	if result.DryRun {
		t.Error("Cleanup() DryRun should be false")
	}

	// Verify objects were actually deleted
	if len(mock.deleted) != 2 {
		t.Errorf("Cleanup() deleted %d objects, want 2", len(mock.deleted))
	}
}
