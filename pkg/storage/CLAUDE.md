# Storage Package

This package provides storage backend abstractions and implementations for CloudSync.

## Storage Interface

All storage backends must implement the `Storage` interface defined in `interface.go`:

```go
type Storage interface {
    Upload(ctx context.Context, localPath string, remotePath string) error
    List(ctx context.Context, prefix string) ([]ObjectInfo, error)
    Delete(ctx context.Context, remotePath string) error
    Validate(ctx context.Context) error
}
```

## Creating New Storage Backends

1. Create a new file (e.g., `oss.go` for Alibaba OSS)
2. Define a config struct for backend-specific settings
3. Implement a constructor function (e.g., `NewOSSStorage`)
4. Add a factory function in `factory.go` to create the storage from config
5. Return `*StorageError` for domain-specific errors

## Error Handling

Use `StorageError` for all storage-related errors:

```go
return &StorageError{Message: "bucket not found", Cause: err}
```

Common error types:
- `ErrNotFound` - Object not found
- `ErrInvalidCredentials` - Invalid credentials
- `ErrPermissionDenied` - Access denied

## AWS SDK v2 Error Handling

For S3 storage, errors implement the `smithyError` interface:

```go
type smithyError interface {
    Error() string
    ErrorCode() string
}
```

Common AWS error codes:
- `NoSuchKey`, `NotFound` - Object not found
- `InvalidAccessKeyId`, `SignatureDoesNotMatch` - Invalid credentials
- `AccessDenied`, `Forbidden` - Permission denied
- `NoSuchBucket` - Bucket not found

## Aliyun OSS SDK Error Handling

For OSS storage, errors are detected by checking error message strings:

```go
switch {
case strings.Contains(lowerErr, "nosuchkey"):
    return &StorageError{Message: "not found", Cause: err}
case strings.Contains(lowerErr, "invalidaccesskeyid"):
    return &StorageError{Message: "invalid credentials", Cause: err}
}
```

Note: OSS SDK uses different API patterns than AWS SDK:
- Validation uses `client.ListBuckets()` and `bucket.ListObjects(oss.MaxKeys(1))`
- No `GetBucketInfo` or `GetBucketACL` methods available on Bucket type
- Pagination uses `Marker` and `NextMarker` instead of continuation tokens

## Factory Pattern

Use `NewStorage(config)` from `factory.go` to create storage instances:

```go
storage, err := storage.NewStorage(cfg)
if err != nil {
    return err
}
```

## Configuration

Storage configurations are defined in `pkg/config/config.go`:
- `S3Config` for AWS S3 settings
- `OSSConfig` for Aliyun OSS settings

Environment variables are expanded in the config package before being passed to storage constructors.

## Validation

Always call `Validate()` before using a storage instance to verify credentials and connectivity.
