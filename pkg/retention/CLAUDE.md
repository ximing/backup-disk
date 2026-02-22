# Retention Package

This package provides retention policy management and cleanup functionality for CloudSync backups.

## Overview

The retention package manages backup lifecycle by enforcing retention policies based on:
- **MaxDays**: Maximum number of days to keep backups
- **MaxVersions**: Maximum number of backup versions to keep

## Usage

### Creating a Retention Manager

```go
policy := retention.Policy{
    MaxDays:     30,  // Keep backups for 30 days
    MaxVersions: 10,  // Keep maximum 10 versions
}

manager := retention.NewManager(storage, policy)
```

### Listing Backups

```go
backups, err := manager.ListBackups(ctx, "backups/mydata")
if err != nil {
    return err
}

for _, backup := range backups {
    fmt.Printf("Backup: %s, Time: %s, Size: %d, Objects: %d\n",
        backup.Path, backup.Time, backup.Size, backup.ObjectCount)
}
```

### Running Cleanup

```go
// Dry-run to preview what will be deleted
result, err := manager.Cleanup(ctx, "backups/mydata", true)
if err != nil {
    return err
}
fmt.Printf("Would delete %d backups, freeing %d bytes\n",
    len(result.DeletedBackups), result.TotalSize)

// Actual cleanup
result, err = manager.Cleanup(ctx, "backups/mydata", false)
```

## Backup Directory Structure

Backups are organized by date in the following structure:

```
{prefix}/{YYYY}/{MM}/{DD}/{HHmmss}/filename
```

Example:
```
backups/mydata/2024/03/15/143022/file1.txt
backups/mydata/2024/03/15/143022/file2.txt
```

The retention manager parses paths to extract backup timestamps and groups objects by their backup directory.

## Policy Evaluation

Policies are evaluated in the following order:

1. **MaxDays**: Backups older than the specified number of days are marked for deletion
2. **MaxVersions**: After removing expired backups, if the count still exceeds max_versions, the oldest backups are deleted

Both policies are applied together - a backup is deleted if it violates either policy.

## CleanupResult

The `Cleanup()` method returns a `CleanupResult` containing:
- `DeletedBackups`: List of backups that were (or would be) deleted
- `TotalSize`: Total bytes freed
- `DryRun`: Whether this was a dry-run operation

## Testing

The package includes comprehensive tests. Run with:

```go
go test ./pkg/retention/... -v
```

## Important Notes

- Backup times are parsed from directory paths, not object modification times
- All times are handled in UTC
- The retention manager uses the storage interface's `List` and `Delete` methods
- Cleanup deletes all objects under a backup directory (recursive deletion)
