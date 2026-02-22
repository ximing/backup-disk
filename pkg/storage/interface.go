// Package storage provides storage backend interfaces and implementations
package storage

import (
	"context"
	"io"
	"time"
)

// ObjectInfo represents information about a stored object
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
}

// Storage defines the interface for storage backends
type Storage interface {
	// Upload uploads a file to storage
	// localPath: path to the local file
	// remotePath: destination path in storage
	Upload(ctx context.Context, localPath string, remotePath string) error

	// List lists objects with the given prefix
	// Returns a list of ObjectInfo for all matching objects
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)

	// Delete removes an object from storage
	Delete(ctx context.Context, remotePath string) error

	// Validate checks if the storage connection is valid
	// Typically implemented by listing buckets or making a test request
	Validate(ctx context.Context) error
}

// UploadOptions contains options for upload operations
type UploadOptions struct {
	ContentType string
	Metadata    map[string]string
}

// ReaderUpload uploads data from a reader to storage
// This is a helper that can be implemented by backends
func ReaderUpload(ctx context.Context, storage Storage, reader io.Reader, size int64, remotePath string) error {
	// Default implementation: write to temp file then upload
	// Specific backends may override this for more efficient streaming
	return ErrNotImplemented
}

// ErrNotImplemented indicates a feature is not implemented
var ErrNotImplemented = &StorageError{Message: "not implemented"}

// ErrNotFound indicates an object was not found
var ErrNotFound = &StorageError{Message: "not found"}

// ErrInvalidCredentials indicates invalid credentials
var ErrInvalidCredentials = &StorageError{Message: "invalid credentials"}

// ErrPermissionDenied indicates permission denied
var ErrPermissionDenied = &StorageError{Message: "permission denied"}

// StorageError represents a storage-related error
type StorageError struct {
	Message string
	Cause   error
}

func (e *StorageError) Error() string {
	if e.Cause != nil {
		return "storage: " + e.Message + ": " + e.Cause.Error()
	}
	return "storage: " + e.Message
}

func (e *StorageError) Unwrap() error {
	return e.Cause
}
