// Package v2 provides circuit breaker pattern for fault tolerance
package v2

import (
	"errors"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	// CircuitClosed allows requests to pass through
	CircuitClosed CircuitState = iota
	// CircuitOpen blocks all requests
	CircuitOpen
	// CircuitHalfOpen allows limited requests for testing recovery
	CircuitHalfOpen
)

// String returns the string representation of the circuit state
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

var (
	// ErrCircuitOpen is returned when the circuit is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrTooManyRequests is returned when too many requests in half-open state
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// CircuitBreakerConfig configures the circuit breaker
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of failures before opening the circuit
	FailureThreshold int
	// SuccessThreshold is the number of successes in half-open to close the circuit
	SuccessThreshold int
	// Timeout is how long to wait before transitioning from open to half-open
	Timeout time.Duration
	// MaxHalfOpenRequests is the max concurrent requests in half-open state
	MaxHalfOpenRequests int
}

// DefaultCircuitBreakerConfig returns default configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    5,
		SuccessThreshold:    2,
		Timeout:             30 * time.Second,
		MaxHalfOpenRequests: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mu              sync.RWMutex
	name            string
	config          CircuitBreakerConfig
	state           CircuitState
	failures        int
	successes       int
	lastFailureTime time.Time
	halfOpenCount   int
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(name string, config CircuitBreakerConfig) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 5
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 2
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxHalfOpenRequests <= 0 {
		config.MaxHalfOpenRequests = 1
	}

	return &CircuitBreaker{
		name:   name,
		config: config,
		state:  CircuitClosed,
	}
}

// Execute runs the given function if the circuit allows it
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	err := fn()
	cb.afterRequest(err == nil)
	return err
}

// beforeRequest checks if the request should be allowed
func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return nil

	case CircuitOpen:
		// Check if timeout has passed
		if time.Since(cb.lastFailureTime) > cb.config.Timeout {
			cb.toHalfOpen()
			cb.halfOpenCount++
			return nil
		}
		return ErrCircuitOpen

	case CircuitHalfOpen:
		if cb.halfOpenCount >= cb.config.MaxHalfOpenRequests {
			return ErrTooManyRequests
		}
		cb.halfOpenCount++
		return nil
	}

	return nil
}

// afterRequest updates the circuit state based on the result
func (cb *CircuitBreaker) afterRequest(success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		if success {
			cb.failures = 0
		} else {
			cb.failures++
			cb.lastFailureTime = time.Now()
			if cb.failures >= cb.config.FailureThreshold {
				cb.toOpen()
			}
		}

	case CircuitHalfOpen:
		cb.halfOpenCount--
		if success {
			cb.successes++
			if cb.successes >= cb.config.SuccessThreshold {
				cb.toClosed()
			}
		} else {
			cb.toOpen()
		}
	}
}

// toOpen transitions to open state
func (cb *CircuitBreaker) toOpen() {
	cb.state = CircuitOpen
	cb.lastFailureTime = time.Now()
	cb.successes = 0
	cb.halfOpenCount = 0
}

// toHalfOpen transitions to half-open state
func (cb *CircuitBreaker) toHalfOpen() {
	cb.state = CircuitHalfOpen
	cb.successes = 0
	cb.halfOpenCount = 0
}

// toClosed transitions to closed state
func (cb *CircuitBreaker) toClosed() {
	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenCount = 0
}

// State returns the current circuit state
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Name returns the circuit breaker name
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.toClosed()
}

// Stats returns circuit breaker statistics
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return CircuitBreakerStats{
		Name:            cb.name,
		State:           cb.state.String(),
		Failures:        cb.failures,
		Successes:       cb.successes,
		LastFailureTime: cb.lastFailureTime,
	}
}

// CircuitBreakerStats contains circuit breaker statistics
type CircuitBreakerStats struct {
	Name            string    `json:"name"`
	State           string    `json:"state"`
	Failures        int       `json:"failures"`
	Successes       int       `json:"successes"`
	LastFailureTime time.Time `json:"lastFailureTime,omitempty"`
}

// CircuitBreakerRegistry manages multiple circuit breakers
type CircuitBreakerRegistry struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   CircuitBreakerConfig
}

// NewCircuitBreakerRegistry creates a new registry
func NewCircuitBreakerRegistry(config CircuitBreakerConfig) *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// Get returns or creates a circuit breaker for the given name
func (r *CircuitBreakerRegistry) Get(name string) *CircuitBreaker {
	r.mu.RLock()
	cb, ok := r.breakers[name]
	r.mu.RUnlock()

	if ok {
		return cb
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if existingCB, exists := r.breakers[name]; exists {
		return existingCB
	}

	cb = NewCircuitBreaker(name, r.config)
	r.breakers[name] = cb
	return cb
}

// GetAll returns all circuit breakers
func (r *CircuitBreakerRegistry) GetAll() []*CircuitBreaker {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*CircuitBreaker, 0, len(r.breakers))
	for _, cb := range r.breakers {
		result = append(result, cb)
	}
	return result
}

// Stats returns statistics for all circuit breakers
func (r *CircuitBreakerRegistry) Stats() []CircuitBreakerStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]CircuitBreakerStats, 0, len(r.breakers))
	for _, cb := range r.breakers {
		result = append(result, cb.Stats())
	}
	return result
}

// Reset resets all circuit breakers
func (r *CircuitBreakerRegistry) Reset() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, cb := range r.breakers {
		cb.Reset()
	}
}
