package v2

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestURLFailoverManager_ExecuteWithFailover tests the failover behavior
func TestURLFailoverManager_ExecuteWithFailover(t *testing.T) {
	t.Run("success on first URL", func(t *testing.T) {
		config := URLFailoverConfig{
			BaseURLs:   []string{"http://url1", "http://url2"},
			RetryDelay: 10 * time.Millisecond,
			MaxRetries: 0,
			Timeout:    5 * time.Second,
		}
		manager := NewURLFailoverManager(config, nil)

		var calledURL string
		err := manager.ExecuteWithFailover(context.Background(), func(baseURL string) error {
			calledURL = baseURL
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, "http://url1", calledURL)
	})

	t.Run("failover to second URL", func(t *testing.T) {
		config := URLFailoverConfig{
			BaseURLs:   []string{"http://url1", "http://url2", "http://url3"},
			RetryDelay: 10 * time.Millisecond,
			MaxRetries: 0,
			Timeout:    5 * time.Second,
		}
		manager := NewURLFailoverManager(config, nil)

		callCount := 0
		var lastURL string
		err := manager.ExecuteWithFailover(context.Background(), func(baseURL string) error {
			callCount++
			lastURL = baseURL
			if baseURL == "http://url1" {
				return errors.New("url1 failed")
			}
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 2, callCount)
		assert.Equal(t, "http://url2", lastURL)
	})

	t.Run("all URLs fail", func(t *testing.T) {
		config := URLFailoverConfig{
			BaseURLs:   []string{"http://url1", "http://url2"},
			RetryDelay: 10 * time.Millisecond,
			MaxRetries: 0,
			Timeout:    5 * time.Second,
		}
		manager := NewURLFailoverManager(config, nil)

		callCount := 0
		err := manager.ExecuteWithFailover(context.Background(), func(baseURL string) error {
			callCount++
			return errors.New("failed")
		})

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrAllURLsFailed))
		assert.Equal(t, 2, callCount)
	})

	t.Run("no URLs configured", func(t *testing.T) {
		config := URLFailoverConfig{
			BaseURLs: []string{},
		}
		manager := NewURLFailoverManager(config, nil)

		err := manager.ExecuteWithFailover(context.Background(), func(baseURL string) error {
			return nil
		})

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoURLsConfigured))
	})

	t.Run("context cancellation", func(t *testing.T) {
		config := URLFailoverConfig{
			BaseURLs:   []string{"http://url1", "http://url2"},
			RetryDelay: 100 * time.Millisecond,
			MaxRetries: 2,
			Timeout:    5 * time.Second,
		}
		manager := NewURLFailoverManager(config, nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := manager.ExecuteWithFailover(ctx, func(baseURL string) error {
			return errors.New("should not reach here")
		})

		assert.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("retry on same URL", func(t *testing.T) {
		config := URLFailoverConfig{
			BaseURLs:   []string{"http://url1"},
			RetryDelay: 10 * time.Millisecond,
			MaxRetries: 2,
			Timeout:    5 * time.Second,
		}
		manager := NewURLFailoverManager(config, nil)

		callCount := 0
		err := manager.ExecuteWithFailover(context.Background(), func(baseURL string) error {
			callCount++
			if callCount < 3 {
				return errors.New("temporary error")
			}
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 3, callCount) // 1 initial + 2 retries
	})
}

// TestFailoverHTTPClient tests the HTTP client with failover
func TestFailoverHTTPClient(t *testing.T) {
	t.Run("successful GET request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}))
		defer server.Close()

		config := DefaultFailoverConfig([]string{server.URL})
		client := NewFailoverHTTPClient(config)

		resp, err := client.Get(context.Background(), "/test", nil)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "success", string(resp.Body))
	})

	t.Run("failover on server error", func(t *testing.T) {
		var callCount int32
		server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&callCount, 1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server1.Close()

		server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&callCount, 1)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}))
		defer server2.Close()

		config := DefaultFailoverConfig([]string{server1.URL, server2.URL})
		config.MaxRetries = 0 // No retries for this test - failover only
		client := NewFailoverHTTPClient(config)

		resp, err := client.Get(context.Background(), "/test", nil)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, int32(2), atomic.LoadInt32(&callCount))
	})

	t.Run("POST request with body", func(t *testing.T) {
		var receivedBody string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			buf := make([]byte, 1024)
			n, _ := r.Body.Read(buf)
			receivedBody = string(buf[:n])
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := DefaultFailoverConfig([]string{server.URL})
		client := NewFailoverHTTPClient(config)

		resp, err := client.Post(context.Background(), "/test", []byte("test body"), map[string]string{
			"Content-Type": "text/plain",
		})
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "test body", receivedBody)
	})

	t.Run("custom headers", func(t *testing.T) {
		var receivedHeader string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeader = r.Header.Get("X-Custom-Header")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := DefaultFailoverConfig([]string{server.URL})
		client := NewFailoverHTTPClient(config)

		resp, err := client.Get(context.Background(), "/test", map[string]string{
			"X-Custom-Header": "custom-value",
		})
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "custom-value", receivedHeader)
	})
}

// TestSiteURLRegistry tests the site URL registry
func TestSiteURLRegistry(t *testing.T) {
	t.Run("register and get URLs", func(t *testing.T) {
		registry := NewSiteURLRegistry(nil)
		registry.RegisterURLs(SiteNameMTeam, []string{"http://url1", "http://url2"})

		urls := registry.GetURLs(SiteNameMTeam)
		assert.Equal(t, []string{"http://url1", "http://url2"}, urls)
	})

	t.Run("get URLs for unregistered site", func(t *testing.T) {
		registry := NewSiteURLRegistry(nil)
		urls := registry.GetURLs(SiteNameMTeam)
		assert.Nil(t, urls)
	})

	t.Run("has site", func(t *testing.T) {
		registry := NewSiteURLRegistry(nil)
		registry.RegisterURLs(SiteNameHDSky, []string{"http://hdsky.me"})

		assert.True(t, registry.HasSite(SiteNameHDSky))
		assert.False(t, registry.HasSite(SiteNameMTeam))
	})

	t.Run("list sites", func(t *testing.T) {
		registry := NewSiteURLRegistry(nil)
		registry.RegisterURLs(SiteNameMTeam, []string{"http://url1"})
		registry.RegisterURLs(SiteNameHDSky, []string{"http://url2"})

		sites := registry.ListSites()
		assert.Len(t, sites, 2)
		assert.Contains(t, sites, SiteNameMTeam)
		assert.Contains(t, sites, SiteNameHDSky)
	})

	t.Run("get failover client", func(t *testing.T) {
		registry := NewSiteURLRegistry(nil)
		registry.RegisterURLs(SiteNameMTeam, []string{"http://url1", "http://url2"})

		client, err := registry.GetFailoverClient(SiteNameMTeam)
		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "http://url1", client.GetCurrentBaseURL())
	})

	t.Run("get failover client for unregistered site", func(t *testing.T) {
		registry := NewSiteURLRegistry(nil)

		client, err := registry.GetFailoverClient(SiteNameMTeam)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoURLsConfigured))
		assert.Nil(t, client)
	})
}

// TestDefaultSiteURLs tests the default site URL configuration
func TestDefaultSiteURLs(t *testing.T) {
	t.Run("M-Team has multiple URLs", func(t *testing.T) {
		urls := DefaultSiteURLs[SiteNameMTeam]
		assert.Len(t, urls, 3)
		assert.Contains(t, urls, "https://api.m-team.cc")
		assert.Contains(t, urls, "https://kp.m-team.cc")
		assert.Contains(t, urls, "https://pt.m-team.cc")
	})

	t.Run("HDSky has URL", func(t *testing.T) {
		urls := DefaultSiteURLs[SiteNameHDSky]
		assert.Len(t, urls, 1)
		assert.Equal(t, "https://hdsky.me", urls[0])
	})

	t.Run("SpringSunday has URL", func(t *testing.T) {
		urls := DefaultSiteURLs[SiteNameSpringSunday]
		assert.Len(t, urls, 1)
		assert.Equal(t, "https://springsunday.net", urls[0])
	})
}

// TestSiteKindMap tests the site kind mapping
func TestSiteKindMap(t *testing.T) {
	t.Run("M-Team is MTorrent", func(t *testing.T) {
		assert.Equal(t, SiteMTorrent, SiteKindMap[SiteNameMTeam])
	})

	t.Run("HDSky is NexusPHP", func(t *testing.T) {
		assert.Equal(t, SiteNexusPHP, SiteKindMap[SiteNameHDSky])
	})

	t.Run("SpringSunday is NexusPHP", func(t *testing.T) {
		assert.Equal(t, SiteNexusPHP, SiteKindMap[SiteNameSpringSunday])
	})

	t.Run("GetSiteKind returns correct kind", func(t *testing.T) {
		assert.Equal(t, SiteMTorrent, GetSiteKind(SiteNameMTeam))
		assert.Equal(t, SiteNexusPHP, GetSiteKind(SiteNameHDSky))
	})

	t.Run("GetSiteKind defaults to NexusPHP", func(t *testing.T) {
		assert.Equal(t, SiteNexusPHP, GetSiteKind(SiteName("unknown")))
	})
}

// TestIsRetryableError tests the error classification
func TestIsRetryableError(t *testing.T) {
	t.Run("nil error is not retryable", func(t *testing.T) {
		assert.False(t, isRetryableError(nil))
	})

	t.Run("context canceled is not retryable", func(t *testing.T) {
		assert.False(t, isRetryableError(context.Canceled))
	})

	t.Run("context deadline exceeded is not retryable", func(t *testing.T) {
		assert.False(t, isRetryableError(context.DeadlineExceeded))
	})

	t.Run("invalid credentials is not retryable", func(t *testing.T) {
		assert.False(t, isRetryableError(ErrInvalidCredentials))
	})

	t.Run("rate limited is not retryable", func(t *testing.T) {
		assert.False(t, isRetryableError(ErrRateLimited))
	})

	t.Run("generic error is retryable", func(t *testing.T) {
		assert.True(t, isRetryableError(errors.New("network error")))
	})
}

func TestURLFailoverManager_GetAllURLs(t *testing.T) {
	cfg := URLFailoverConfig{BaseURLs: []string{"http://a", "http://b"}, Timeout: time.Second}
	m := NewURLFailoverManager(cfg, nil)
	assert.Equal(t, []string{"http://a", "http://b"}, m.GetAllURLs())
}

func TestFailover_WithLogger(t *testing.T) {
	logger := zap.NewNop()
	cfg := URLFailoverConfig{BaseURLs: []string{"http://a"}, Timeout: time.Second}
	c := NewFailoverHTTPClient(cfg, WithLogger(logger))
	assert.Same(t, logger, c.logger)
}

func TestFailoverHTTPClient_Do(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api", r.URL.Path)
		assert.Equal(t, "custom", r.Header.Get("X-Test"))
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("get-ok"))
		}
	}))
	defer server.Close()

	cfg := DefaultFailoverConfig([]string{server.URL})
	c := NewFailoverHTTPClient(cfg)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		resp, err := c.Do(context.Background(), method, "/api", []byte("body"), map[string]string{"X-Test": "custom"})
		require.NoError(t, err, "method %s", method)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}
}

func TestFailoverHTTPClient_Do_UnsupportedMethod(t *testing.T) {
	cfg := DefaultFailoverConfig([]string{"http://127.0.0.1:1"})
	cfg.MaxRetries = 0
	c := NewFailoverHTTPClient(cfg)
	_, err := c.Do(context.Background(), "BREW", "/x", nil, nil)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// nexusphp_driver.go — Execute / login & 2FA detection / ParseDownload live /
// FetchSeedingStatus / filterNames / buildCurlCommand / isHexString
// ---------------------------------------------------------------------------

func TestURLFailoverManager_GetCurrentURL(t *testing.T) {
	m := NewURLFailoverManager(URLFailoverConfig{BaseURLs: []string{"http://a", "http://b"}}, nil)
	assert.Equal(t, "http://a", m.GetCurrentURL())

	empty := NewURLFailoverManager(URLFailoverConfig{}, nil)
	assert.Equal(t, "", empty.GetCurrentURL())
}

func TestFailoverHTTPClient_GetCurrentBaseURL(t *testing.T) {
	c := NewFailoverHTTPClient(URLFailoverConfig{BaseURLs: []string{"http://z"}, Timeout: time.Second})
	assert.Equal(t, "http://z", c.GetCurrentBaseURL())
}

// ---------------------------------------------------------------------------
// http_client.go — RequestsClient doWithRetry max exceeded
// ---------------------------------------------------------------------------
