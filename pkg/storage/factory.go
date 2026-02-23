package storage

import (
	"fmt"
	"strings"

	"github.com/ximing/cloudsync/pkg/config"
)

// NewStorage creates a storage instance based on the configuration (deprecated, use NewStorageFromBackend)
func NewStorage(cfg *config.Config) (Storage, error) {
	if len(cfg.Storage) == 0 {
		return nil, fmt.Errorf("no storage backend configured")
	}
	return NewStorageFromBackend(cfg.Storage[0])
}

// NewStorageFromBackend creates a storage instance from a StorageBackend config
func NewStorageFromBackend(backend config.StorageBackend) (Storage, error) {
	storageType := strings.ToLower(backend.Type)

	switch storageType {
	case "s3":
		return NewS3StorageFromConfig(backend.S3)
	case "oss":
		return NewOSSStorageFromConfig(backend.OSS)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", backend.Type)
	}
}

// NewStoragesFromBackends creates storage instances from multiple backends
func NewStoragesFromBackends(backends []config.StorageBackend) ([]Storage, error) {
	var storages []Storage
	for _, backend := range backends {
		s, err := NewStorageFromBackend(backend)
		if err != nil {
			return nil, fmt.Errorf("failed to create storage '%s': %w", backend.Name, err)
		}
		storages = append(storages, s)
	}
	return storages, nil
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
func NewOSSStorageFromConfig(cfg config.OSSConfig) (Storage, error) {
	ossCfg := OSSConfig{
		Endpoint:        cfg.Endpoint,
		Bucket:          cfg.Bucket,
		AccessKeyID:     cfg.AccessKeyID,
		AccessKeySecret: cfg.AccessKeySecret,
	}

	return NewOSSStorage(ossCfg)
}
