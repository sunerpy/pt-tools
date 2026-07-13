package cloakdriver

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
