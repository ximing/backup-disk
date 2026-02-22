// Package retry provides retry functionality with exponential backoff
package retry

import (
	"context"
	"fmt"
	"time"
)

// ErrorType represents the type of error for retry decisions
type ErrorType int

const (
	// ErrorTypeRecoverable indicates a recoverable error that can be retried
	ErrorTypeRecoverable ErrorType = iota
	// ErrorTypeNonRecoverable indicates a non-recoverable error that should not be retried
	ErrorTypeNonRecoverable
)

// Config represents retry configuration
type Config struct {
	MaxRetries  int           // Maximum number of retries (default: 3)
	BaseDelay   time.Duration // Base delay for exponential backoff (default: 1s)
	MaxDelay    time.Duration // Maximum delay between retries (default: 30s)
	Multiplier  float64       // Exponential backoff multiplier (default: 2)
}

// DefaultConfig returns default retry configuration
func DefaultConfig() Config {
	return Config{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
		Multiplier: 2,
	}
}

// ErrorClassifier is a function that classifies errors as recoverable or non-recoverable
type ErrorClassifier func(error) ErrorType

// RetryableFunc is a function that can be retried
type RetryableFunc func() error

// Retry executes the given function with retry logic
func Retry(ctx context.Context, config Config, classifier ErrorClassifier, fn RetryableFunc) error {
	if config.MaxRetries <= 0 {
		config.MaxRetries = DefaultConfig().MaxRetries
	}
	if config.BaseDelay <= 0 {
		config.BaseDelay = DefaultConfig().BaseDelay
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = DefaultConfig().MaxDelay
	}
	if config.Multiplier <= 1 {
		config.Multiplier = DefaultConfig().Multiplier
	}

	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Execute the function
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if this is the last attempt
		if attempt >= config.MaxRetries {
			break
		}

		// Classify the error
		errType := classifier(err)
		if errType == ErrorTypeNonRecoverable {
			return fmt.Errorf("non-recoverable error: %w", err)
		}

		// Calculate delay for next attempt
		delay := calculateDelay(config, attempt)

		// Wait before retry
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return fmt.Errorf("max retries exceeded (%d): %w", config.MaxRetries, lastErr)
}

// calculateDelay calculates the delay for the given attempt using exponential backoff
func calculateDelay(config Config, attempt int) time.Duration {
	// Calculate exponential delay: base * (multiplier ^ attempt)
	delay := float64(config.BaseDelay) * pow(config.Multiplier, float64(attempt))

	// Cap at max delay
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	return time.Duration(delay)
}

// pow calculates x raised to the power of y
func pow(x, y float64) float64 {
	result := 1.0
	for i := 0; i < int(y); i++ {
		result *= x
	}
	return result
}

// RetryWithResult executes the given function with retry logic and returns a result
func RetryWithResult[T any](ctx context.Context, config Config, classifier ErrorClassifier, fn func() (T, error)) (T, error) {
	var zero T

	if config.MaxRetries <= 0 {
		config.MaxRetries = DefaultConfig().MaxRetries
	}
	if config.BaseDelay <= 0 {
		config.BaseDelay = DefaultConfig().BaseDelay
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = DefaultConfig().MaxDelay
	}
	if config.Multiplier <= 1 {
		config.Multiplier = DefaultConfig().Multiplier
	}

	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Execute the function
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if this is the last attempt
		if attempt >= config.MaxRetries {
			break
		}

		// Classify the error
		errType := classifier(err)
		if errType == ErrorTypeNonRecoverable {
			return zero, fmt.Errorf("non-recoverable error: %w", err)
		}

		// Calculate delay for next attempt
		delay := calculateDelay(config, attempt)

		// Wait before retry
		select {
		case <-ctx.Done():
			return zero, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return zero, fmt.Errorf("max retries exceeded (%d): %w", config.MaxRetries, lastErr)
}

// IsRecoverableError checks if an error is recoverable (default implementation)
func IsRecoverableError(err error) ErrorType {
	if err == nil {
		return ErrorTypeRecoverable
	}

	errStr := err.Error()

	// Non-recoverable errors (authentication, permission, configuration issues)
	nonRecoverablePatterns := []string{
		"authentication failed",
		"access denied",
		"invalid credentials",
		"signature does not match",
		"invalid access key",
		"forbidden",
		"unauthorized",
		"invalid bucket name",
		"no such bucket",
		"malformed",
		"invalid parameter",
	}

	for _, pattern := range nonRecoverablePatterns {
		if containsIgnoreCase(errStr, pattern) {
			return ErrorTypeNonRecoverable
		}
	}

	// Recoverable errors (network, rate limiting, temporary issues)
	recoverablePatterns := []string{
		"timeout",
		"deadline exceeded",
		"connection refused",
		"connection reset",
		"no such host",
		"temporary",
		"rate limit",
		"throttled",
		"slow down",
		"service unavailable",
		"internal server error",
		"bad gateway",
		"gateway timeout",
		"network",
		"i/o timeout",
		"context deadline",
	}

	for _, pattern := range recoverablePatterns {
		if containsIgnoreCase(errStr, pattern) {
			return ErrorTypeRecoverable
		}
	}

	// Default to recoverable for unknown errors
	return ErrorTypeRecoverable
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && containsFold(s, substr)
}

// containsFold is a case-insensitive contains check
func containsFold(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalFold(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

// equalFold checks if two strings are equal (case-insensitive)
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if toLower(a[i]) != toLower(b[i]) {
			return false
		}
	}
	return true
}

// toLower converts a byte to lowercase
func toLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
