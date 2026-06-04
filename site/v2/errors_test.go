package v2

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSentinelErrorsIs(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		target  error
		wantErr bool
	}{
		{
			name:    "direct ErrSessionExpired",
			err:     ErrSessionExpired,
			target:  ErrSessionExpired,
			wantErr: true,
		},
		{
			name:    "wrapped ErrSessionExpired",
			err:     fmt.Errorf("probe failed: %w", ErrSessionExpired),
			target:  ErrSessionExpired,
			wantErr: true,
		},
		{
			name:    "deeply wrapped ErrSessionExpired",
			err:     fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", ErrSessionExpired)),
			target:  ErrSessionExpired,
			wantErr: true,
		},
		{
			name:    "direct ErrCircuitOpen",
			err:     ErrCircuitOpen,
			target:  ErrCircuitOpen,
			wantErr: true,
		},
		{
			name:    "wrapped ErrCircuitOpen",
			err:     fmt.Errorf("rate limit hit: %w", ErrCircuitOpen),
			target:  ErrCircuitOpen,
			wantErr: true,
		},
		{
			name:    "wrong sentinel",
			err:     ErrSessionExpired,
			target:  ErrCircuitOpen,
			wantErr: false,
		},
		{
			name:    "nil error",
			err:     nil,
			target:  ErrSessionExpired,
			wantErr: false,
		},
		{
			name:    "generic error",
			err:     errors.New("some other error"),
			target:  ErrSessionExpired,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := errors.Is(tt.err, tt.target)
			assert.Equal(t, tt.wantErr, result)
		})
	}
}

func TestSentinelErrorMessages(t *testing.T) {
	assert.Equal(t, "session expired or cookie invalid", ErrSessionExpired.Error())
	assert.Equal(t, "circuit breaker open", ErrCircuitOpen.Error())
}
