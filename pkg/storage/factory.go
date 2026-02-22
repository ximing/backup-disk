package storage

import (
	"fmt"
	"strings"

	"github.com/ximing/cloudsync/pkg/config"
)

// NewStorage creates a storage instance based on the configuration
func NewStorage(cfg *config.Config) (Storage, error) {
	storageType := strings.ToLower(cfg.Storage.Type)

	switch storageType {
	case "s3":
		return NewS3StorageFromConfig(cfg.Storage.S3)
	case "oss":
		return NewOSSStorageFromConfig(cfg.Storage.OSS)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Storage.Type)
	}
}

// NewS3StorageFromConfig creates S3 storage from config.S3Config
func NewS3StorageFromConfig(cfg config.S3Config) (Storage, error) {
	s3Cfg := S3Config{
		Region:     cfg.Region,
		Bucket:     cfg.Bucket,
		AccessKey:  cfg.AccessKey,
		SecretKey:  cfg.SecretKey,
		Endpoint:   cfg.Endpoint,
		Encryption: cfg.Encryption,
	}

	return NewS3Storage(s3Cfg)
}

// NewOSSStorageFromConfig creates OSS storage from config.OSSConfig
// This is a placeholder that will be implemented in US-003
func NewOSSStorageFromConfig(cfg config.OSSConfig) (Storage, error) {
	return nil, fmt.Errorf("OSS storage not yet implemented (US-003)")
}
