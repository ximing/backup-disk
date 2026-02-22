// Package scanner provides file system scanning functionality
package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileInfo represents information about a file to be synced
type FileInfo struct {
	Path         string
	RelativePath string
	Size         int64
	ModTime      int64
}

// Config represents scanner configuration
type Config struct {
	RootPath string
	Include  []string
	Exclude  []string
}

// Scanner handles file system scanning
type Scanner struct {
	config Config
}

// New creates a new scanner
func New(config Config) *Scanner {
	return &Scanner{config: config}
}

// Scan scans the source directory and returns files matching include/exclude patterns
func (s *Scanner) Scan() ([]FileInfo, error) {
	var files []FileInfo

	// Convert include patterns to matchers
	includeMatchers := make([]patternMatcher, 0, len(s.config.Include))
	for _, pattern := range s.config.Include {
		includeMatchers = append(includeMatchers, newPatternMatcher(pattern))
	}

	// Convert exclude patterns to matchers
	excludeMatchers := make([]patternMatcher, 0, len(s.config.Exclude))
	for _, pattern := range s.config.Exclude {
		excludeMatchers = append(excludeMatchers, newPatternMatcher(pattern))
	}

	err := filepath.WalkDir(s.config.RootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Get relative path from root
		relPath, err := filepath.Rel(s.config.RootPath, path)
		if err != nil {
			return err
		}

		// Normalize path separator for pattern matching
		relPath = filepath.ToSlash(relPath)

		// Check exclude patterns first
		if s.matchesAny(relPath, excludeMatchers) {
			return nil
		}

		// Check include patterns (if any are specified)
		if len(includeMatchers) > 0 && !s.matchesAny(relPath, includeMatchers) {
			return nil
		}

		// Get file info
		info, err := d.Info()
		if err != nil {
			return err
		}

		files = append(files, FileInfo{
			Path:         path,
			RelativePath: relPath,
			Size:         info.Size(),
			ModTime:      info.ModTime().Unix(),
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan directory: %w", err)
	}

	return files, nil
}

// matchesAny checks if path matches any of the given matchers
func (s *Scanner) matchesAny(path string, matchers []patternMatcher) bool {
	for _, matcher := range matchers {
		if matcher.Match(path) {
			return true
		}
	}
	return false
}

// patternMatcher handles glob pattern matching
type patternMatcher struct {
	pattern string
}

// newPatternMatcher creates a new pattern matcher
func newPatternMatcher(pattern string) patternMatcher {
	// Normalize pattern path separator
	pattern = filepath.ToSlash(pattern)
	return patternMatcher{pattern: pattern}
}

// Match checks if the given path matches the pattern
func (pm patternMatcher) Match(path string) bool {
	// Try direct match
	matched, err := filepath.Match(pm.pattern, path)
	if err == nil && matched {
		return true
	}

	// Try matching against file name only
	matched, err = filepath.Match(pm.pattern, filepath.Base(path))
	if err == nil && matched {
		return true
	}

	// Try prefix match for directory patterns (e.g., "dir/**")
	if strings.HasSuffix(pm.pattern, "/**") {
		dir := strings.TrimSuffix(pm.pattern, "/**")
		if strings.HasPrefix(path, dir+"/") || path == dir {
			return true
		}
	}

	// Try prefix match for patterns like "dir/*"
	if strings.HasSuffix(pm.pattern, "/*") && !strings.HasSuffix(pm.pattern, "/*/*") {
		dir := strings.TrimSuffix(pm.pattern, "/*")
		if strings.HasPrefix(path, dir+"/") {
			return true
		}
	}

	return false
}

// ValidateSource checks if the source path exists and is accessible
func ValidateSource(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("source path does not exist: %s", path)
		}
		return fmt.Errorf("cannot access source path: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("source path is not a directory: %s", path)
	}

	return nil
}

// CountFiles returns the total number of files and total size
func CountFiles(files []FileInfo) (count int, totalSize int64) {
	for _, f := range files {
		count++
		totalSize += f.Size
	}
	return count, totalSize
}
