package web

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServe_StaticAndAuthedRoutes(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("admin", hashPassword("adminadmin")))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(addr) }()

	client := &http.Client{
		Timeout:       3 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	waitReady(t, client, "http://"+addr+"/api/ping")

	base := "http://" + addr

	t.Run("static css served", func(t *testing.T) {
		resp, err := client.Get(base + "/static/style.css")
		require.NoError(t, err)
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("static js content-type", func(t *testing.T) {
		resp, err := client.Get(base + "/static/app.js")
		require.NoError(t, err)
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
	})

	t.Run("assets served", func(t *testing.T) {
		resp, err := client.Get(base + "/assets/nonexistent.js")
		require.NoError(t, err)
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
	})

	t.Run("version check authed via login", func(t *testing.T) {
		var jar []*http.Cookie
		loginResp, err := client.Post(base+"/login", "application/json",
			strings.NewReader(`{"username":"admin","password":"adminadmin"}`))
		require.NoError(t, err)
		jar = loginResp.Cookies()
		_, _ = io.Copy(io.Discard, loginResp.Body)
		loginResp.Body.Close()

		req, _ := http.NewRequest(http.MethodGet, base+"/api/version", nil)
		for _, c := range jar {
			req.AddCookie(c)
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	require.NoError(t, srv.Shutdown(context.Background()))
	select {
	case <-errCh:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
}
