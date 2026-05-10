// Package core defines domain models and interfaces for the scraper subsystem.
package core

import (
	"errors"
	"fmt"
)

// Sentinel errors for scraper operations.
var (
	// ErrNotFound is returned when a resource is not found.
	ErrNotFound = errors.New("resource not found")

	// ErrInvalidID is returned when an ID is invalid or malformed.
	ErrInvalidID = errors.New("invalid ID format")

	// ErrAlreadyExists is returned when trying to register a duplicate name.
	ErrAlreadyExists = errors.New("already exists")

	// ErrRateLimited is returned when a provider rate limit is hit.
	ErrRateLimited = errors.New("provider rate limit exceeded")

	// ErrUnauthorized is returned when authentication fails.
	ErrUnauthorized = errors.New("unauthorized: invalid credentials")

	// ErrProviderDisabled is returned when a provider is disabled or unavailable.
	ErrProviderDisabled = errors.New("provider is disabled or unavailable")

	// ErrParseFailed is returned when parsing provider response fails.
	ErrParseFailed = errors.New("failed to parse provider response")

	// ErrTimeout is returned when a provider request times out.
	ErrTimeout = errors.New("provider request timeout")

	// ErrAllProvidersFailed is returned when all providers fail to fulfill a request.
	ErrAllProvidersFailed = errors.New("all providers failed to complete request")

	// ErrProviderDown is returned when the provider is unreachable or returning 5xx.
	ErrProviderDown = errors.New("provider is down")

	// ErrUnsupported is returned when a requested feature or server type is not supported.
	ErrUnsupported = errors.New("unsupported")

	// ErrPermanent marks an error as non-retryable. Wrap or errors.Is with this
	// sentinel when the failure is deterministic and will repeat identically —
	// e.g. "no provider registered", "provider credentials missing", "invalid
	// media path". PersistentQueue.persistentTask.Run checks this to skip the
	// retry loop and mark the task failed immediately, avoiding the misleading
	// "retrying" UI state for config-level failures.
	ErrPermanent = errors.New("permanent error (non-retryable)")
)

// IsPermanent reports whether err indicates a deterministic, non-retryable
// failure. ErrUnauthorized / ErrNotFound / ErrInvalidID / ErrUnsupported /
// ErrPermanent and any error wrapping them are treated as permanent.
func IsPermanent(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrPermanent),
		errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrNotFound),
		errors.Is(err, ErrInvalidID),
		errors.Is(err, ErrUnsupported):
		return true
	}
	return false
}

// Wrap wraps an error with an additional message using fmt.Errorf.
// It preserves the original error for errors.Is/As checking.
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}
