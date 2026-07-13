package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestsHTTPDoerRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "bar", r.Header.Get("X-Foo"))
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	}))
	defer srv.Close()

	doer := NewRequestsHTTPDoer(srv.URL, 5*time.Second)
	defer doer.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, strings.NewReader("ping"))
	require.NoError(t, err)
	req.Header.Set("X-Foo", "bar")

	resp, err := doer.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	buf := make([]byte, 4)
	_, _ = resp.Body.Read(buf)
	assert.Equal(t, "pong", string(buf))
}

func TestRequestsHTTPDoerGetNoBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	doer := NewRequestsHTTPDoer(srv.URL, 5*time.Second)
	defer doer.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := doer.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRequestsHTTPDoerCloseNilSession(t *testing.T) {
	d := &RequestsHTTPDoer{}
	assert.NoError(t, d.Close())
}
