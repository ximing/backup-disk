package compress

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ximing/cloudsync/pkg/scanner"
)

// ArchiveCompressor creates compressed archives
type ArchiveCompressor struct {
	cfg Config
}

// NewArchiveCompressor creates a new archive compressor
func NewArchiveCompressor(cfg Config) *ArchiveCompressor {
	return &ArchiveCompressor{cfg: cfg}
}

// CreateArchive creates a compressed archive from the given files
// Returns the path to the created archive and its size
func (a *ArchiveCompressor) CreateArchive(files []scanner.FileInfo, basePath string, taskName string) (string, int64, error) {
	// Determine archive name
	archiveName := a.cfg.ArchiveName
	if archiveName == "" {
		archiveName = fmt.Sprintf("%s_%s", taskName, time.Now().Format("20060102_150405"))
	}

	// Determine extension based on compression type
	extension := ".tar.gz"
	if a.cfg.Type == "zstd" || a.cfg.Type == "zst" {
		extension = ".tar.zst"
	}
	archiveName += extension

	// Create temp file for archive
	tempFile, err := os.CreateTemp("", "cloudsync-archive-*"+extension)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	// Build the archive
	var writeErr error
	var bytesWritten int64

	switch a.cfg.Type {
	case "gzip":
		bytesWritten, writeErr = a.createTarGzip(tempFile, files, basePath)
	case "zstd", "zst":
		bytesWritten, writeErr = a.createTarZstd(tempFile, files, basePath)
	default:
		writeErr = fmt.Errorf("unsupported compression type for archive: %s", a.cfg.Type)
	}

	if writeErr != nil {
		os.Remove(tempFile.Name())
		return "", 0, writeErr
	}

	// Sync to ensure data is written
	if err := tempFile.Sync(); err != nil {
		os.Remove(tempFile.Name())
		return "", 0, fmt.Errorf("failed to sync archive: %w", err)
	}

	return tempFile.Name(), bytesWritten, nil
}

// createTarGzip creates a gzip-compressed tar archive
func (a *ArchiveCompressor) createTarGzip(w io.Writer, files []scanner.FileInfo, basePath string) (int64, error) {
	// Set compression level
	level := a.cfg.Level
	if level < 1 || level > 9 {
		level = gzip.DefaultCompression
	}

	gzipWriter, err := gzip.NewWriterLevel(w, level)
	if err != nil {
		return 0, fmt.Errorf("failed to create gzip writer: %w", err)
	}
	defer gzipWriter.Close()

	return a.createTarArchive(gzipWriter, files, basePath)
}

// createTarZstd creates a zstd-compressed tar archive
func (a *ArchiveCompressor) createTarZstd(w io.Writer, files []scanner.FileInfo, basePath string) (int64, error) {
	level := a.cfg.Level
	if level < 1 || level > 9 {
		level = 3 // Default zstd level
	}

	// Map 1-9 to zstd speed levels
	var speed zstd.EncoderLevel
	switch {
	case level <= 2:
		speed = zstd.SpeedFastest
	case level <= 5:
		speed = zstd.SpeedDefault
	case level <= 7:
		speed = zstd.SpeedBetterCompression
	default:
		speed = zstd.SpeedBestCompression
	}

	zstdWriter, err := zstd.NewWriter(w, zstd.WithEncoderLevel(speed))
	if err != nil {
		return 0, fmt.Errorf("failed to create zstd writer: %w", err)
	}
	defer zstdWriter.Close()

	return a.createTarArchive(zstdWriter, files, basePath)
}

// createTarArchive creates a tar archive from files
func (a *ArchiveCompressor) createTarArchive(w io.Writer, files []scanner.FileInfo, basePath string) (int64, error) {
	tarWriter := tar.NewWriter(w)
	defer tarWriter.Close()

	var totalBytes int64

	for _, file := range files {
		// Open source file
		srcFile, err := os.Open(file.Path)
		if err != nil {
			return totalBytes, fmt.Errorf("failed to open file %s: %w", file.Path, err)
		}

		// Get file info for mode
		stat, err := srcFile.Stat()
		if err != nil {
			srcFile.Close()
			return totalBytes, fmt.Errorf("failed to stat file %s: %w", file.Path, err)
		}

		// Create tar header
		header := &tar.Header{
			Name:    file.RelativePath,
			Size:    file.Size,
			Mode:    int64(stat.Mode() & 0777),
			ModTime: time.Unix(file.ModTime, 0),
		}

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			srcFile.Close()
			return totalBytes, fmt.Errorf("failed to write tar header for %s: %w", file.Path, err)
		}

		// Write file content
		n, err := io.Copy(tarWriter, srcFile)
		srcFile.Close()
		if err != nil {
			return totalBytes, fmt.Errorf("failed to write file %s to archive: %w", file.Path, err)
		}

		totalBytes += n
	}

	// Close tar writer to flush data
	if err := tarWriter.Close(); err != nil {
		return totalBytes, fmt.Errorf("failed to close tar writer: %w", err)
	}

	return totalBytes, nil
}

// GetArchiveExtension returns the file extension for the archive
func (a *ArchiveCompressor) GetArchiveExtension() string {
	switch a.cfg.Type {
	case "zstd", "zst":
		return ".tar.zst"
	default:
		return ".tar.gz"
	}
}

// Cleanup removes the temporary archive file
func (a *ArchiveCompressor) Cleanup(archivePath string) {
	if archivePath != "" {
		os.Remove(archivePath)
	}
}
