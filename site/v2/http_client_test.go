package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPClientPool(t *testing.T) {
	pool := NewHTTPClientPool(DefaultHTTPClientConfig(), nil)
	assert.NotNil(t, pool)
}

func TestHTTPClientPool_GetSession(t *testing.T) {
	pool := NewHTTPClientPool(DefaultHTTPClientConfig(), nil)

	session1 := pool.GetSession("site1")
	assert.NotNil(t, session1)

	session2 := pool.GetSession("site1")
	assert.Same(t, session1, session2)

	session3 := pool.GetSession("site2")
	assert.NotSame(t, session1, session3)
}

func TestHTTPClientPool_GetSession_Concurrent(t *testing.T) {
	pool := NewHTTPClientPool(DefaultHTTPClientConfig(), nil)

	var wg sync.WaitGroup
	sessions := make([]interface{}, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sessions[idx] = pool.GetSession("site1")
		}(i)
	}

	wg.Wait()

	for i := 1; i < 10; i++ {
		assert.Equal(t, sessions[0], sessions[i])
	}
}

func TestHTTPClientPool_CloseClient(t *testing.T) {
	pool := NewHTTPClientPool(DefaultHTTPClientConfig(), nil)

	session1 := pool.GetSession("site1")
	assert.NotNil(t, session1)

	pool.CloseClient("site1")

	pool.mu.RLock()
	_, exists := pool.sessions["site1"]
	pool.mu.RUnlock()
	assert.False(t, exists)

	session2 := pool.GetSession("site1")
	assert.NotNil(t, session2)
}

func TestHTTPClientPool_CloseAll(t *testing.T) {
	pool := NewHTTPClientPool(DefaultHTTPClientConfig(), nil)

	pool.GetSession("site1")
	pool.GetSession("site2")
	pool.GetSession("site3")

	pool.CloseAll()

	pool.mu.RLock()
	assert.Empty(t, pool.sessions)
	pool.mu.RUnlock()
}

func TestDefaultHTTPClientConfig(t *testing.T) {
	config := DefaultHTTPClientConfig()

	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 100, config.MaxIdleConns)
	assert.Equal(t, 10, config.MaxIdleConnsPerHost)
	assert.Equal(t, 90*time.Second, config.IdleConnTimeout)
	assert.False(t, config.DisableKeepAlives)
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 1*time.Second, config.InitialBackoff)
	assert.Equal(t, 30*time.Second, config.MaxBackoff)
	assert.Equal(t, 2.0, config.BackoffMultiplier)
	assert.True(t, config.Jitter)
	assert.Contains(t, config.RetryableStatusCodes, http.StatusTooManyRequests)
}

func TestRetryableHTTPClient_Do_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	client := NewRetryableHTTPClient(
		&http.Client{Timeout: 5 * time.Second},
		DefaultRetryConfig(),
		nil,
	)

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := client.Do(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestRetryableHTTPClient_Do_RetryOnError(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultRetryConfig()
	config.InitialBackoff = 10 * time.Millisecond
	config.MaxBackoff = 50 * time.Millisecond

	client := NewRetryableHTTPClient(
		&http.Client{Timeout: 5 * time.Second},
		config,
		nil,
	)

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := client.Do(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
	resp.Body.Close()
}

func TestRetryableHTTPClient_Do_MaxRetriesExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	config := DefaultRetryConfig()
	config.MaxRetries = 2
	config.InitialBackoff = 10 * time.Millisecond

	client := NewRetryableHTTPClient(
		&http.Client{Timeout: 5 * time.Second},
		config,
		nil,
	)

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := client.Do(req)

	// Should return the last response even on max retries
	assert.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		resp.Body.Close()
	}
}

func TestRetryableHTTPClient_Do_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	config := DefaultRetryConfig()
	config.InitialBackoff = 1 * time.Second // Long backoff

	client := NewRetryableHTTPClient(
		&http.Client{Timeout: 5 * time.Second},
		config,
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)

	// Cancel after first attempt
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := client.Do(req)
	assert.Error(t, err)
}

func TestRetryableHTTPClient_calculateBackoff(t *testing.T) {
	config := RetryConfig{
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        10 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            false,
	}

	client := NewRetryableHTTPClient(nil, config, nil)

	// First retry: 1s
	assert.Equal(t, 1*time.Second, client.calculateBackoff(1))
	// Second retry: 2s
	assert.Equal(t, 2*time.Second, client.calculateBackoff(2))
	// Third retry: 4s
	assert.Equal(t, 4*time.Second, client.calculateBackoff(3))
	// Fourth retry: 8s
	assert.Equal(t, 8*time.Second, client.calculateBackoff(4))
	// Fifth retry: capped at 10s
	assert.Equal(t, 10*time.Second, client.calculateBackoff(5))
}

func TestRetryableHTTPClient_calculateBackoff_WithJitter(t *testing.T) {
	config := RetryConfig{
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        10 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            true,
	}

	client := NewRetryableHTTPClient(nil, config, nil)

	// With jitter, backoff should be between base and base + 25%
	backoff := client.calculateBackoff(1)
	assert.GreaterOrEqual(t, backoff, 1*time.Second)
	assert.LessOrEqual(t, backoff, 1250*time.Millisecond)
}

func TestNewSessionManager(t *testing.T) {
	manager := NewSessionManager(nil)
	assert.NotNil(t, manager)
}

func TestSessionManager_SetAndGetSession(t *testing.T) {
	manager := NewSessionManager(nil)

	session := &Session{
		Cookie: "test-cookie",
		APIKey: "test-key",
	}

	manager.SetSession("site1", session)

	retrieved, ok := manager.GetSession("site1")
	assert.True(t, ok)
	assert.Equal(t, "test-cookie", retrieved.Cookie)
	assert.Equal(t, "test-key", retrieved.APIKey)
	assert.Equal(t, "site1", retrieved.SiteID)
}

func TestSessionManager_GetSession_NotFound(t *testing.T) {
	manager := NewSessionManager(nil)

	_, ok := manager.GetSession("nonexistent")
	assert.False(t, ok)
}

func TestSessionManager_UpdateSessionID(t *testing.T) {
	manager := NewSessionManager(nil)

	session := &Session{Cookie: "test"}
	manager.SetSession("site1", session)

	manager.UpdateSessionID("site1", "new-session-id")

	retrieved, _ := manager.GetSession("site1")
	assert.Equal(t, "new-session-id", retrieved.SessionID)
}

func TestSessionManager_InvalidateSession(t *testing.T) {
	manager := NewSessionManager(nil)

	session := &Session{Cookie: "test"}
	manager.SetSession("site1", session)

	assert.True(t, manager.IsSessionValid("site1"))

	manager.InvalidateSession("site1")

	assert.False(t, manager.IsSessionValid("site1"))
}

func TestSessionManager_RemoveSession(t *testing.T) {
	manager := NewSessionManager(nil)

	session := &Session{Cookie: "test"}
	manager.SetSession("site1", session)

	manager.RemoveSession("site1")

	_, ok := manager.GetSession("site1")
	assert.False(t, ok)
}

func TestSessionManager_IsSessionValid(t *testing.T) {
	manager := NewSessionManager(nil)

	// No session
	assert.False(t, manager.IsSessionValid("site1"))

	// Valid session with cookie
	manager.SetSession("site1", &Session{Cookie: "test"})
	assert.True(t, manager.IsSessionValid("site1"))

	// Valid session with API key
	manager.SetSession("site2", &Session{APIKey: "test"})
	assert.True(t, manager.IsSessionValid("site2"))

	// Valid session with session ID
	manager.SetSession("site3", &Session{SessionID: "test"})
	assert.True(t, manager.IsSessionValid("site3"))

	// Empty session
	manager.SetSession("site4", &Session{})
	assert.False(t, manager.IsSessionValid("site4"))

	// Expired session
	manager.SetSession("site5", &Session{
		Cookie:    "test",
		ExpiresAt: time.Now().Add(-time.Hour),
	})
	assert.False(t, manager.IsSessionValid("site5"))
}

func TestSessionManager_LoginCount(t *testing.T) {
	manager := NewSessionManager(nil)

	session := &Session{Cookie: "test"}
	manager.SetSession("site1", session)

	assert.Equal(t, 1, manager.IncrementLoginCount("site1"))
	assert.Equal(t, 2, manager.IncrementLoginCount("site1"))
	assert.Equal(t, 3, manager.IncrementLoginCount("site1"))

	manager.ResetLoginCount("site1")

	retrieved, _ := manager.GetSession("site1")
	assert.Equal(t, 0, retrieved.LoginCount)
}

func TestHandleTransmissionSession(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"X-Transmission-Session-Id": []string{"test-session-id"},
		},
	}

	sessionID := HandleTransmissionSession(resp)
	assert.Equal(t, "test-session-id", sessionID)
}

func TestHandleTransmissionSession_NoHeader(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{},
	}

	sessionID := HandleTransmissionSession(resp)
	assert.Empty(t, sessionID)
}

func TestNewRequestsClient(t *testing.T) {
	client := NewRequestsClient(DefaultHTTPClientConfig(), DefaultRetryConfig(), nil)
	assert.NotNil(t, client)
	defer client.Close()
}

func TestRequestsClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	client := NewRequestsClient(DefaultHTTPClientConfig(), DefaultRetryConfig(), nil)
	defer client.Close()

	resp, err := client.Get(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	assert.Equal(t, "success", resp.Text())
	assert.True(t, resp.IsSuccess())
	assert.False(t, resp.IsError())
}

func TestRequestsClient_Post(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("created"))
	}))
	defer server.Close()

	client := NewRequestsClient(DefaultHTTPClientConfig(), DefaultRetryConfig(), nil)
	defer client.Close()

	resp, err := client.Post(context.Background(), server.URL, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode())
	assert.Equal(t, "created", resp.Text())
}

func TestRequestsClient_RetryOnError(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	config := DefaultRetryConfig()
	config.InitialBackoff = 10 * time.Millisecond
	config.MaxBackoff = 50 * time.Millisecond

	client := NewRequestsClient(DefaultHTTPClientConfig(), config, nil)
	defer client.Close()

	resp, err := client.Get(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
}

func TestRequestsResponse_Methods(t *testing.T) {
	resp := &RequestsResponse{
		statusCode: http.StatusOK,
		body:       []byte("test body"),
		headers:    http.Header{"Content-Type": []string{"text/plain"}},
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode())
	assert.Equal(t, []byte("test body"), resp.Bytes())
	assert.Equal(t, "test body", resp.Text())
	assert.Equal(t, "text/plain", resp.Headers().Get("Content-Type"))
	assert.True(t, resp.IsSuccess())
	assert.False(t, resp.IsError())

	// Test error response
	errResp := &RequestsResponse{statusCode: http.StatusInternalServerError}
	assert.False(t, errResp.IsSuccess())
	assert.True(t, errResp.IsError())
}
