// Package compress provides compression interfaces and implementations
package compress

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// Compressor defines the interface for compression implementations
type Compressor interface {
	// Compress compresses data from the provided reader
	// Returns the compressed reader, file extension (e.g., ".gz", ".zst"), and any error
	Compress(reader io.Reader) (io.Reader, string, error)

	// ShouldCompress determines if a file should be compressed based on filters
	// path: file path
	// size: file size in bytes
	ShouldCompress(path string, size int64) bool
}

// Config holds compression configuration
type Config struct {
	Enabled           bool
	Type              string   // "gzip" or "zstd"
	Level             int      // 1-9
	MinSize           int64    // minimum file size in bytes
	IncludeExtensions []string // only compress these extensions (empty = all)
	ExcludeExtensions []string // never compress these extensions
}

// NewCompressor creates a new compressor based on configuration
func NewCompressor(cfg Config) (Compressor, error) {
	if !cfg.Enabled {
		return &NoopCompressor{}, nil
	}

	// Normalize compression level
	level := cfg.Level
	if level < 1 {
		level = 1
	}
	if level > 9 {
		level = 9
	}

	switch strings.ToLower(cfg.Type) {
	case "gzip":
		return NewGzipCompressor(cfg), nil
	case "zstd":
		return NewZstdCompressor(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported compression type: %s (must be 'gzip' or 'zstd')", cfg.Type)
	}
}

// NoopCompressor is a pass-through compressor that doesn't compress
type NoopCompressor struct{}

// Compress returns the reader unchanged with empty extension
func (n *NoopCompressor) Compress(reader io.Reader) (io.Reader, string, error) {
	return reader, "", nil
}

// ShouldCompress always returns true (no filtering for noop)
func (n *NoopCompressor) ShouldCompress(path string, size int64) bool {
	return true
}

// shouldCompress determines if a file should be compressed based on filters
func shouldCompress(path string, size int64, minSize int64, includeExts, excludeExts []string) bool {
	// Check minimum size
	if minSize > 0 && size < minSize {
		return false
	}

	ext := strings.ToLower(filepath.Ext(path))

	// Check exclude list first
	for _, exclude := range excludeExts {
		if ext == strings.ToLower(exclude) {
			return false
		}
	}

	// If include list is specified, file must match one of them
	if len(includeExts) > 0 {
		for _, include := range includeExts {
			if ext == strings.ToLower(include) {
				return true
			}
		}
		return false
	}

	return true
}

// CompressionError represents a compression-related error
type CompressionError struct {
	Message string
	Cause   error
}

func (e *CompressionError) Error() string {
	if e.Cause != nil {
		return "compress: " + e.Message + ": " + e.Cause.Error()
	}
	return "compress: " + e.Message
}

func (e *CompressionError) Unwrap() error {
	return e.Cause
}

// ErrInvalidLevel indicates an invalid compression level
var ErrInvalidLevel = &CompressionError{Message: "invalid compression level"}

// ErrUnsupportedType indicates an unsupported compression type
var ErrUnsupportedType = &CompressionError{Message: "unsupported compression type"}
