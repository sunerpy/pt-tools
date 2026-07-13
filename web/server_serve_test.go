package web

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServe_FullLifecycle(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(addr)
	}()

	client := &http.Client{
		Timeout: 3 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	waitReady(t, client, "http://"+addr+"/api/ping")

	t.Run("ping is public", func(t *testing.T) {
		resp, err := client.Get("http://" + addr + "/api/ping")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("protected api returns 401", func(t *testing.T) {
		resp, err := client.Get("http://" + addr + "/api/global")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("root redirects to login without session", func(t *testing.T) {
		resp, err := client.Get("http://" + addr + "/")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusFound, resp.StatusCode)
	})

	t.Run("login page served", func(t *testing.T) {
		resp, err := client.Get("http://" + addr + "/login")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("cors preflight from extension", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, "http://"+addr+"/api/ping", nil)
		req.Header.Set("Origin", "chrome-extension://abc")
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
		assert.Equal(t, "chrome-extension://abc", resp.Header.Get("Access-Control-Allow-Origin"))
	})

	require.NoError(t, srv.Shutdown(context.Background()))
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

func waitReady(t *testing.T, client *http.Client, url string) {
	t.Helper()
	require.Eventually(t, func() bool {
		resp, err := client.Get(url)
		if err != nil {
			return false
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 3*time.Second, 20*time.Millisecond)
}
