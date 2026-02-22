package compress

import (
	"compress/gzip"
	"io"
)

// GzipCompressor implements gzip compression
type GzipCompressor struct {
	config Config
}

// NewGzipCompressor creates a new gzip compressor
func NewGzipCompressor(cfg Config) *GzipCompressor {
	return &GzipCompressor{config: cfg}
}

// Compress compresses data using gzip
func (g *GzipCompressor) Compress(reader io.Reader) (io.Reader, string, error) {
	level := g.config.Level
	if level < 1 {
		level = gzip.DefaultCompression
	}
	if level > 9 {
		level = gzip.BestCompression
	}

	// Use pipe to stream compression
	pr, pw := io.Pipe()

	go func() {
		var gw *gzip.Writer
		var err error

		if level == gzip.DefaultCompression {
			gw = gzip.NewWriter(pw)
		} else {
			gw, err = gzip.NewWriterLevel(pw, level)
			if err != nil {
				pw.CloseWithError(err)
				return
			}
		}

		_, err = io.Copy(gw, reader)
		if closeErr := gw.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			pw.CloseWithError(err)
		} else {
			pw.Close()
		}
	}()

	return pr, ".gz", nil
}

// ShouldCompress determines if a file should be compressed
func (g *GzipCompressor) ShouldCompress(path string, size int64) bool {
	return shouldCompress(path, size, g.config.MinSize, g.config.IncludeExtensions, g.config.ExcludeExtensions)
}
