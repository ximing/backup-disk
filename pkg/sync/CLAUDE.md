# Sync Package

This package provides task synchronization functionality for CloudSync CLI.

## Key Components

### Executor
The `Executor` struct orchestrates the entire sync process:
- Validates source paths
- Scans files using `pkg/scanner`
- Compresses files using `pkg/compress`
- Uploads to cloud storage using `pkg/storage`

### Usage

```go
store, _ := storage.NewStorage(cfg)
log := logger.GetLogger()
executor := sync.NewExecutor(store, log)

result, err := executor.Execute(ctx, taskConfig, sync.Options{
    DryRun:      false,
    DateFormat:  "YYYY/MM/DD/HHmmss",
    Compression: compress.Config{...},
})
```

### Result Structure

The `Result` struct contains:
- `TaskName`: Name of the executed task
- `StartTime`/`EndTime`: Execution timestamps
- `Success`: Whether all files were synced
- `FilesTotal/Success/Failed`: File counts
- `BytesTotal/Success`: Byte counts
- `FailedFiles`: List of paths that failed to sync

## Concurrency

Uploads are limited to 5 concurrent operations using a semaphore to prevent overwhelming the storage backend or local system.

## Date Format

The target path format supports standard date placeholders:
- `YYYY` - Year (4 digits)     - `MM` - Month (01-12)       - `DD` - Day (01-31)
- `YY`   - Year (2 digits)     - `HH` - Hour (00-23)        - `mm` - Minute (00-59)
- `ss`   - Second (00-59)

- Default: `YYYY/MM/DD/HHmmss` (produces paths like `prefix/2024/02/23/143022/`)
- Examples:
  - `YYYY-MM-DD` → `2024-03-15`
  - `YYYYMMDD_HHmmss` → `20240315_143022`

## Dry Run Mode

When `Options.DryRun` is true, the executor will:
- Scan files normally
- Log what would be uploaded
- Skip actual upload operations
- Return results showing what would happen
