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
    DateFormat:  "2006/01/02/150405",
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

The target path format follows Go's time layout conventions:
- Default: `2006/01/02/150405` (produces paths like `prefix/2024/02/23/143022/`)
- Custom formats can be specified per task

## Dry Run Mode

When `Options.DryRun` is true, the executor will:
- Scan files normally
- Log what would be uploaded
- Skip actual upload operations
- Return results showing what would happen
