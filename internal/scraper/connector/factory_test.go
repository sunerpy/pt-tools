package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func TestNewConnector_AutoDetectJellyfin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/System/Info/Public", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PublicInfo{
			ProductName: "Jellyfin Server",
			Version:     "10.11.8",
			Id:          "jf-id",
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c, err := NewConnector(ctx, "auto", srv.URL, "", srv.Client())
	require.NoError(t, err)
	jc, isJellyfin := c.(*JellyfinConnector)
	require.True(t, isJellyfin)
	require.Equal(t, "jellyfin", jc.Name())
}

func TestNewConnector_AutoDetectEmby(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/System/Info/Public", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PublicInfo{
			ProductName: "Emby Server",
			Version:     "4.9.2",
			Id:          "emby-id",
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c, err := NewConnector(ctx, "auto", srv.URL, "", srv.Client())
	require.NoError(t, err)
	ec, isEmby := c.(*EmbyConnector)
	require.True(t, isEmby)
	require.Equal(t, "emby", ec.Name())
}

func TestNewConnector_ExplicitJellyfin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	c, err := NewConnector(ctx, "jellyfin", srv.URL, "key", srv.Client())
	require.NoError(t, err)
	jc, isJellyfin := c.(*JellyfinConnector)
	require.True(t, isJellyfin)
	require.NotNil(t, jc)
	require.Equal(t, "jellyfin", jc.Name())
}

func TestNewConnector_ExplicitEmby(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	c, err := NewConnector(ctx, "emby", srv.URL, "key", srv.Client())
	require.NoError(t, err)
	ec, isEmby := c.(*EmbyConnector)
	require.True(t, isEmby)
	require.NotNil(t, ec)
	require.Equal(t, "emby", ec.Name())
}

func TestNewConnector_EmptyProductDefaultsToAuto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PublicInfo{
			ProductName: "Jellyfin Server",
			Version:     "10.11.8",
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c, err := NewConnector(ctx, "", srv.URL, "", srv.Client())
	require.NoError(t, err)
	jc, ok := c.(*JellyfinConnector)
	require.True(t, ok)
	require.Equal(t, "jellyfin", jc.Name())
}

func TestNewConnector_UnknownProduct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	_, err := NewConnector(ctx, "plex", srv.URL, "", srv.Client())
	require.Error(t, err)
	require.ErrorIs(t, err, core.ErrUnsupported)
}

func TestNewConnector_AutoDetectNetworkError(t *testing.T) {
	ctx := context.Background()
	_, err := NewConnector(ctx, "auto", "http://invalid-hostname-never-exists-12345.local", "", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "auto-detect")
}

func TestNewConnector_UnknownAutoDetect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PublicInfo{
			ProductName: "UnknownServer",
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	_, err := NewConnector(ctx, "auto", srv.URL, "", srv.Client())
	require.Error(t, err)
	require.ErrorIs(t, err, core.ErrUnsupported)
}

func TestNewConnector_DefaultClientNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PublicInfo{
			ProductName: "Jellyfin Server",
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c, err := NewConnector(ctx, "jellyfin", srv.URL, "key", nil)
	require.NoError(t, err)
	jc, ok := c.(*JellyfinConnector)
	require.True(t, ok)
	require.Equal(t, "jellyfin", jc.Name())
}
