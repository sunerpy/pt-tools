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
)

// Wrap wraps an error with an additional message using fmt.Errorf.
// It preserves the original error for errors.Is/As checking.
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}
