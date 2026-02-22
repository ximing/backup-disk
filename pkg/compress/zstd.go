package compress

import (
	"io"

	"github.com/klauspost/compress/zstd"
)

// ZstdCompressor implements zstd compression
type ZstdCompressor struct {
	config Config
}

// NewZstdCompressor creates a new zstd compressor
func NewZstdCompressor(cfg Config) *ZstdCompressor {
	return &ZstdCompressor{config: cfg}
}

// Compress compresses data using zstd
func (z *ZstdCompressor) Compress(reader io.Reader) (io.Reader, string, error) {
	level := z.config.Level
	if level < 1 {
		level = 3 // zstd default
	}

	// Map 1-9 level to zstd speed levels
	// zstd.SpeedFastest = 1, zstd.SpeedDefault = 3, zstd.SpeedBetterCompression = 6, zstd.SpeedBestCompression = 11
	var zstdLevel zstd.EncoderLevel
	switch {
	case level <= 2:
		zstdLevel = zstd.SpeedFastest
	case level <= 5:
		zstdLevel = zstd.SpeedDefault
	case level <= 8:
		zstdLevel = zstd.SpeedBetterCompression
	default:
		zstdLevel = zstd.SpeedBestCompression
	}

	// Use pipe to stream compression
	pr, pw := io.Pipe()

	go func() {
		encoder, err := zstd.NewWriter(pw, zstd.WithEncoderLevel(zstdLevel))
		if err != nil {
			pw.CloseWithError(err)
			return
		}

		_, err = io.Copy(encoder, reader)
		if closeErr := encoder.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			pw.CloseWithError(err)
		} else {
			pw.Close()
		}
	}()

	return pr, ".zst", nil
}

// ShouldCompress determines if a file should be compressed
func (z *ZstdCompressor) ShouldCompress(path string, size int64) bool {
	return shouldCompress(path, size, z.config.MinSize, z.config.IncludeExtensions, z.config.ExcludeExtensions)
}
