package v2

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitState_String(t *testing.T) {
	assert.Equal(t, "closed", CircuitClosed.String())
	assert.Equal(t, "open", CircuitOpen.String())
	assert.Equal(t, "half-open", CircuitHalfOpen.String())
	assert.Equal(t, "unknown", CircuitState(99).String())
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	assert.Equal(t, 5, cfg.FailureThreshold)
	assert.Equal(t, 2, cfg.SuccessThreshold)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, 1, cfg.MaxHalfOpenRequests)
}

func TestNewCircuitBreaker_Defaults(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{})
	assert.Equal(t, "test", cb.Name())
	assert.Equal(t, CircuitClosed, cb.State())
	assert.Equal(t, 5, cb.config.FailureThreshold)
	assert.Equal(t, 2, cb.config.SuccessThreshold)
	assert.Equal(t, 30*time.Second, cb.config.Timeout)
	assert.Equal(t, 1, cb.config.MaxHalfOpenRequests)
}

func TestCircuitBreaker_SuccessKeepsClosed(t *testing.T) {
	cb := NewCircuitBreaker("s", CircuitBreakerConfig{FailureThreshold: 3})
	for i := 0; i < 5; i++ {
		err := cb.Execute(func() error { return nil })
		require.NoError(t, err)
	}
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cb := NewCircuitBreaker("f", CircuitBreakerConfig{FailureThreshold: 3, Timeout: time.Hour})
	testErr := errors.New("fail")

	for i := 0; i < 3; i++ {
		err := cb.Execute(func() error { return testErr })
		assert.ErrorIs(t, err, testErr)
	}
	assert.Equal(t, CircuitOpen, cb.State())

	// While open, requests are rejected.
	err := cb.Execute(func() error { return nil })
	assert.ErrorIs(t, err, ErrCircuitOpen)

	stats := cb.Stats()
	assert.Equal(t, "open", stats.State)
	assert.Equal(t, 3, stats.Failures)
	assert.Equal(t, "f", stats.Name)
}

func TestCircuitBreaker_HalfOpenRecoversToClosed(t *testing.T) {
	cb := NewCircuitBreaker("r", CircuitBreakerConfig{
		FailureThreshold: 2, SuccessThreshold: 2, Timeout: 5 * time.Millisecond,
	})
	testErr := errors.New("fail")
	// Open the circuit.
	cb.Execute(func() error { return testErr })
	cb.Execute(func() error { return testErr })
	require.Equal(t, CircuitOpen, cb.State())

	time.Sleep(10 * time.Millisecond) // let timeout elapse

	// First request transitions to half-open and succeeds.
	err := cb.Execute(func() error { return nil })
	require.NoError(t, err)
	// Second success closes the circuit.
	err = cb.Execute(func() error { return nil })
	require.NoError(t, err)
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker("ro", CircuitBreakerConfig{
		FailureThreshold: 1, SuccessThreshold: 2, Timeout: 5 * time.Millisecond,
	})
	testErr := errors.New("fail")
	cb.Execute(func() error { return testErr })
	require.Equal(t, CircuitOpen, cb.State())

	time.Sleep(10 * time.Millisecond)
	// Half-open request fails -> back to open.
	err := cb.Execute(func() error { return testErr })
	assert.ErrorIs(t, err, testErr)
	assert.Equal(t, CircuitOpen, cb.State())
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker("rst", CircuitBreakerConfig{FailureThreshold: 1, Timeout: time.Hour})
	cb.Execute(func() error { return errors.New("x") })
	require.Equal(t, CircuitOpen, cb.State())
	cb.Reset()
	assert.Equal(t, CircuitClosed, cb.State())
	assert.Equal(t, 0, cb.Stats().Failures)
}

func TestCircuitBreakerRegistry(t *testing.T) {
	reg := NewCircuitBreakerRegistry(CircuitBreakerConfig{FailureThreshold: 2, Timeout: time.Hour})

	cb1 := reg.Get("site-a")
	cb2 := reg.Get("site-a")
	assert.Same(t, cb1, cb2)

	cb3 := reg.Get("site-b")
	assert.NotSame(t, cb1, cb3)

	all := reg.GetAll()
	assert.Len(t, all, 2)

	// Trip site-a
	cb1.Execute(func() error { return errors.New("e") })
	cb1.Execute(func() error { return errors.New("e") })
	assert.Equal(t, CircuitOpen, cb1.State())

	stats := reg.Stats()
	assert.Len(t, stats, 2)

	reg.Reset()
	assert.Equal(t, CircuitClosed, cb1.State())
}
