package cloakdriver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testToken = "test-token-abc"

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func TestManagerClient_LaunchProfile_Happy(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("missing/wrong Authorization header: %q", got)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/profiles/site-x/launch" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ProfileLaunchResult{
			ProfileID: "site-x",
			CdpURL:    "ws://localhost:9222/devtools/browser/abc",
			VncURL:    "ws://localhost:6080/vnc",
			StartedAt: time.Now().UTC(),
		})
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	lr, err := c.LaunchProfile(context.Background(), "site-x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lr == nil || lr.CdpURL == "" {
		t.Fatalf("expected non-empty CdpURL, got %+v", lr)
	}
	if lr.ProfileID != "site-x" {
		t.Fatalf("expected ProfileID site-x, got %q", lr.ProfileID)
	}
}

func TestManagerClient_LaunchProfile_AuthFail(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, "wrong-token", 5*time.Second)
	_, err := c.LaunchProfile(context.Background(), "site-x")
	if !errors.Is(err, ErrManagerAuthFailed) {
		t.Fatalf("expected ErrManagerAuthFailed, got %v", err)
	}
}

func TestManagerClient_LaunchProfile_NotFound(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	_, err := c.LaunchProfile(context.Background(), "missing")
	if !errors.Is(err, ErrManagerNotFound) {
		t.Fatalf("expected ErrManagerNotFound, got %v", err)
	}
}

func TestManagerClient_LaunchProfile_ServerError(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	_, err := c.LaunchProfile(context.Background(), "site-x")
	if !errors.Is(err, ErrManagerServerError) {
		t.Fatalf("expected ErrManagerServerError, got %v", err)
	}
}

func TestManagerClient_LaunchProfile_DNSFail(t *testing.T) {
	c := NewManagerClient("http://bogus-no-such-host-pt-tools.invalid:1234", testToken, 3*time.Second)
	_, err := c.LaunchProfile(context.Background(), "site-x")
	if !errors.Is(err, ErrManagerDNSFailed) {
		t.Fatalf("expected ErrManagerDNSFailed, got %v", err)
	}
}

func TestManagerClient_LaunchProfile_ConnRefused(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := ts.URL
	ts.Close()

	c := NewManagerClient(addr, testToken, 3*time.Second)
	_, err := c.LaunchProfile(context.Background(), "site-x")
	if !errors.Is(err, ErrManagerConnRefused) {
		t.Fatalf("expected ErrManagerConnRefused, got %v", err)
	}
}

func TestManagerClient_LaunchProfile_Timeout(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 200*time.Millisecond)
	_, err := c.LaunchProfile(context.Background(), "site-x")
	if !errors.Is(err, ErrManagerTimeout) {
		t.Fatalf("expected ErrManagerTimeout, got %v", err)
	}
}

func TestManagerClient_ManagerStatus_Happy(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	if err := c.ManagerStatus(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManagerClient_ManagerStatus_AuthFail(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer wrong" {
			t.Errorf("expected wrong token in header, got %q", got)
		}
		w.WriteHeader(http.StatusUnauthorized)
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, "wrong", 5*time.Second)
	err := c.ManagerStatus(context.Background())
	if !errors.Is(err, ErrManagerAuthFailed) {
		t.Fatalf("expected ErrManagerAuthFailed, got %v", err)
	}
}

func TestManagerClient_DeleteProfile_Idempotent(t *testing.T) {
	var callCount int32
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	if err := c.DeleteProfile(context.Background(), "site-x"); err != nil {
		t.Fatalf("first DELETE: unexpected error: %v", err)
	}
	if err := c.DeleteProfile(context.Background(), "site-x"); err != nil {
		t.Fatalf("second DELETE (idempotent): unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&callCount); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}

func TestManagerClient_GetProfileStatus_Happy(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/profiles/site-x/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ProfileStatus{
			ProfileID: "site-x",
			Running:   true,
			CdpURL:    "ws://localhost:9222/devtools/browser/abc",
		})
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	ps, err := c.GetProfileStatus(context.Background(), "site-x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ps.Running || ps.CdpURL == "" {
		t.Fatalf("expected running + non-empty CdpURL, got %+v", ps)
	}
}

func TestManagerClient_StopProfile_Happy(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/profiles/site-x/stop" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	if err := c.StopProfile(context.Background(), "site-x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManagerClientManagerStatusFullHappy(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/status", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"0.0.4"}`))
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	info, err := c.ManagerStatusFull(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ok", info.Status)
	assert.Equal(t, "0.0.4", info.Version)
}

func TestManagerClientManagerStatusFullBadJSON(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not json`))
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	_, err := c.ManagerStatusFull(context.Background())
	assert.ErrorIs(t, err, ErrManagerProtocolError)
}

func TestManagerClientManagerStatusFullServerError(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	_, err := c.ManagerStatusFull(context.Background())
	assert.ErrorIs(t, err, ErrManagerServerError)
}

func TestManagerClientLaunchProfileBadJSON(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{broken`))
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	_, err := c.LaunchProfile(context.Background(), "x")
	assert.ErrorIs(t, err, ErrManagerProtocolError)
}

func TestManagerClientGetProfileStatusBadJSON(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{broken`))
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	_, err := c.GetProfileStatus(context.Background(), "x")
	assert.ErrorIs(t, err, ErrManagerProtocolError)
}

func TestManagerClientGetProfileStatusNotFound(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	_, err := c.GetProfileStatus(context.Background(), "x")
	assert.ErrorIs(t, err, ErrManagerNotFound)
}

func TestManagerClientStopProfileServerError(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	err := c.StopProfile(context.Background(), "x")
	assert.ErrorIs(t, err, ErrManagerServerError)
}

func TestManagerClientDeleteProfileServerError(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	err := c.DeleteProfile(context.Background(), "x")
	assert.ErrorIs(t, err, ErrManagerServerError)
}

func TestManagerClientDefaultTimeoutApplied(t *testing.T) {
	c := NewManagerClient("http://example.test", "", 0).(*httpManagerClient)
	assert.Equal(t, defaultManagerTimeout, c.httpClient.Timeout)
}

func TestManagerClientBaseURLTrimmed(t *testing.T) {
	c := NewManagerClient("http://example.test/", "", time.Second).(*httpManagerClient)
	assert.Equal(t, "http://example.test", c.baseURL)
}

func TestManagerClient4xxProtocolError(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	defer ts.Close()

	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	err := c.ManagerStatus(context.Background())
	assert.ErrorIs(t, err, ErrManagerProtocolError)
}

func TestManagerClientContextCanceledMapsToProtocolOrTimeout(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := NewManagerClient(ts.URL, testToken, 5*time.Second)
	err := c.ManagerStatus(ctx)
	require.Error(t, err)
}

func TestClassifyHTTPErrorNilResp(t *testing.T) {
	assert.Nil(t, classifyHTTPError(nil, nil))
}

func TestClassifyHTTPErrorDeadline(t *testing.T) {
	err := classifyHTTPError(nil, context.DeadlineExceeded)
	assert.ErrorIs(t, err, ErrManagerTimeout)
}

func TestCloakSLoggerNotNil(t *testing.T) {
	assert.NotNil(t, sLogger())
}

func closedSession(t *testing.T) *CDPSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	allocCtx, allocCancel := context.WithCancel(ctx)
	taskCtx, taskCancel := context.WithCancel(allocCtx)
	s := &CDPSession{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		taskCtx:     taskCtx,
		taskCancel:  taskCancel,
	}
	s.Close()
	cancel()
	return s
}

func TestCDPGetCookiesOnClosedSessionErrors(t *testing.T) {
	s := closedSession(t)
	_, err := s.GetCookies(s.TaskContext(), "https://example.com/")
	assert.Error(t, err)
}

func TestCDPLoadPageAndExtractOnClosedSessionErrors(t *testing.T) {
	s := closedSession(t)
	called := false
	err := s.LoadPageAndExtract(s.TaskContext(), "https://example.com/", "body", func(string) error {
		called = true
		return nil
	})
	assert.Error(t, err)
	assert.False(t, called, "extract must not run when navigation fails")
}

func TestCDPInjectCookiesNilEntriesOnClosedSession(t *testing.T) {
	s := closedSession(t)
	err := s.InjectCookies(s.TaskContext(), "https://example.com/", []*http.Cookie{nil, {Name: "a", Value: "b"}})
	assert.Error(t, err)
}

func TestManagerClientNewRequestErrorPaths(t *testing.T) {
	c := NewManagerClient("http://exa\x7fmple", "", time.Second)
	ctx := context.Background()

	_, err := c.LaunchProfile(ctx, "x")
	assert.ErrorIs(t, err, ErrManagerProtocolError)

	_, err = c.GetProfileStatus(ctx, "x")
	assert.ErrorIs(t, err, ErrManagerProtocolError)

	assert.ErrorIs(t, c.StopProfile(ctx, "x"), ErrManagerProtocolError)
	assert.ErrorIs(t, c.DeleteProfile(ctx, "x"), ErrManagerProtocolError)
	assert.ErrorIs(t, c.ManagerStatus(ctx), ErrManagerProtocolError)

	_, err = c.ManagerStatusFull(ctx)
	assert.ErrorIs(t, err, ErrManagerProtocolError)
}

func TestNewCDPSessionEmptyURLFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	sess, err := NewCDPSession(ctx, "")
	if err == nil {
		sess.Close()
		t.Fatal("expected error for empty cdp url")
	}
	assert.Error(t, err)
}
