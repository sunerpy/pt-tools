package core

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSentinelErrorsAreDistinct(t *testing.T) {
	sentinels := []error{
		ErrNotFound,
		ErrInvalidID,
		ErrAlreadyExists,
		ErrRateLimited,
		ErrUnauthorized,
		ErrProviderDisabled,
		ErrParseFailed,
		ErrTimeout,
		ErrAllProvidersFailed,
		ErrProviderDown,
		ErrUnsupported,
	}
	for i, a := range sentinels {
		require.NotNil(t, a, "sentinel at index %d must not be nil", i)
		for j, b := range sentinels {
			if i == j {
				continue
			}
			assert.False(t, errors.Is(a, b), "sentinel %q should not match %q", a, b)
		}
	}
}

func TestSentinelErrorsIsMatch(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"not found", ErrNotFound},
		{"invalid id", ErrInvalidID},
		{"already exists", ErrAlreadyExists},
		{"rate limited", ErrRateLimited},
		{"unauthorized", ErrUnauthorized},
		{"provider disabled", ErrProviderDisabled},
		{"parse failed", ErrParseFailed},
		{"timeout", ErrTimeout},
		{"all providers failed", ErrAllProvidersFailed},
		{"provider down", ErrProviderDown},
		{"unsupported", ErrUnsupported},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("wrapper: %w", tc.err)
			assert.True(t, errors.Is(wrapped, tc.err))
			assert.NotEmpty(t, tc.err.Error())
		})
	}
}

func TestWrap_NilError(t *testing.T) {
	got := Wrap(nil, "some message")
	assert.Nil(t, got)
}

func TestWrap_PreservesOriginal(t *testing.T) {
	base := ErrNotFound
	wrapped := Wrap(base, "movie lookup")
	require.Error(t, wrapped)
	assert.True(t, errors.Is(wrapped, ErrNotFound))
	assert.Contains(t, wrapped.Error(), "movie lookup")
	assert.Contains(t, wrapped.Error(), "resource not found")
}

func TestWrap_UnwrapReturnsOriginal(t *testing.T) {
	base := errors.New("raw error")
	wrapped := Wrap(base, "context")
	unwrapped := errors.Unwrap(wrapped)
	assert.Same(t, base, unwrapped)
}

func TestWrap_Chained(t *testing.T) {
	base := ErrTimeout
	first := Wrap(base, "call provider")
	second := Wrap(first, "search movie")
	assert.True(t, errors.Is(second, ErrTimeout))
	assert.Contains(t, second.Error(), "search movie")
	assert.Contains(t, second.Error(), "call provider")
}
