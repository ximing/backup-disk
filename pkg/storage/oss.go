package storage

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OSSConfig holds OSS-specific configuration
type OSSConfig struct {
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	AccessKeySecret string
}

// OSSStorage implements Storage interface for Aliyun OSS
type OSSStorage struct {
	client *oss.Client
	bucket *oss.Bucket
	config OSSConfig
}

// NewOSSStorage creates a new OSS storage instance
func NewOSSStorage(cfg OSSConfig) (*OSSStorage, error) {
	// Create OSS client
	client, err := oss.New(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("failed to create OSS client: %w", err)
	}

	// Get bucket instance
	bucket, err := client.Bucket(cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket: %w", err)
	}

	return &OSSStorage{
		client: client,
		bucket: bucket,
		config: cfg,
	}, nil
}

// Upload uploads a file to OSS
func (s *OSSStorage) Upload(ctx context.Context, localPath string, remotePath string) error {
	// Check if file exists and is not a directory
	stat, err := os.Stat(localPath)
	if err != nil {
		return &StorageError{Message: "failed to stat local file", Cause: err}
	}
	if stat.IsDir() {
		return &StorageError{Message: "cannot upload directory directly"}
	}

	// Upload file
	err = s.bucket.PutObjectFromFile(remotePath, localPath)
	if err != nil {
		return s.convertError(err, "failed to upload file")
	}

	return nil
}

// List lists objects with the given prefix
func (s *OSSStorage) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var objects []ObjectInfo
	marker := ""

	for {
		// List objects with marker for pagination
		result, err := s.bucket.ListObjects(oss.Prefix(prefix), oss.Marker(marker))
		if err != nil {
			return nil, s.convertError(err, "failed to list objects")
		}

		for _, obj := range result.Objects {
			objects = append(objects, ObjectInfo{
				Key:          obj.Key,
				Size:         obj.Size,
				LastModified: obj.LastModified,
				ETag:         obj.ETag,
			})
		}

		// Check if there are more objects
		if !result.IsTruncated {
			break
		}
		marker = result.NextMarker
	}

	return objects, nil
}

// Delete removes an object from OSS
func (s *OSSStorage) Delete(ctx context.Context, remotePath string) error {
	err := s.bucket.DeleteObject(remotePath)
	if err != nil {
		return s.convertError(err, "failed to delete object")
	}

	return nil
}

// Validate checks if the OSS credentials are valid
func (s *OSSStorage) Validate(ctx context.Context) error {
	// Try to list buckets to verify credentials
	_, err := s.client.ListBuckets()
	if err != nil {
		return s.convertError(err, "failed to validate credentials")
	}

	// Check if bucket exists by trying to list objects (with max 1 result)
	_, err = s.bucket.ListObjects(oss.MaxKeys(1))
	if err != nil {
		return s.convertError(err, "bucket not accessible")
	}

	return nil
}

// convertError converts OSS SDK errors to StorageError
func (s *OSSStorage) convertError(err error, message string) error {
	if err == nil {
		return nil
	}

	errStr := err.Error()
	lowerErr := strings.ToLower(errStr)

	// Check for specific OSS error patterns
	switch {
	case strings.Contains(lowerErr, "nosuchkey") || strings.Contains(lowerErr, "not found"):
		return &StorageError{Message: "not found", Cause: err}
	case strings.Contains(lowerErr, "invalidaccesskeyid") ||
		strings.Contains(lowerErr, "signaturedoesnotmatch") ||
		strings.Contains(lowerErr, "accesskeyidnotfound"):
		return &StorageError{Message: "invalid credentials", Cause: err}
	case strings.Contains(lowerErr, "accessdenied") || strings.Contains(lowerErr, "forbidden"):
		return &StorageError{Message: "permission denied", Cause: err}
	case strings.Contains(lowerErr, "nosuchbucket"):
		return &StorageError{Message: "bucket not found", Cause: err}
	}

	return &StorageError{Message: message, Cause: err}
}
