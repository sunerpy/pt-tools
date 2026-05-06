package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbyConnector_Ping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/System/Info/Public", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PublicInfo{
			ProductName: "Emby Server",
			Version:     "4.9.2",
			Id:          "emby-server-id",
			ServerName:  "My Emby",
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewEmbyConnector(srv.URL, "", srv.Client())
	info, err := c.Ping(ctx)
	require.NoError(t, err)
	require.Equal(t, "Emby Server", info.Product)
	require.Equal(t, "4.9.2", info.Version)
	require.Equal(t, "emby-server-id", info.ServerID)
	require.Equal(t, "My Emby", info.Name)
}

func TestEmbyConnector_Authenticate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/System/Info", r.URL.Path)
		require.Equal(t, "embykey", r.Header.Get("X-Emby-Token"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PublicInfo{
			ProductName: "Emby Server",
			Version:     "4.9.2",
			Id:          "emby-server-id",
			ServerName:  "My Emby",
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewEmbyConnector(srv.URL, "embykey", srv.Client())
	info, err := c.Authenticate(ctx)
	require.NoError(t, err)
	require.Equal(t, "Emby Server", info.Product)
}

func TestEmbyConnector_ListLibraries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/Library/MediaFolders", r.URL.Path)
		require.Equal(t, "embykey", r.Header.Get("X-Emby-Token"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(MediaFoldersResponse{
			Items: []MediaFolder{
				{Id: "x", Name: "Films", CollectionType: "movies", Path: "/data/films"},
				{Id: "y", Name: "Series", CollectionType: "tvshows"},
			},
			TotalRecordCount: 2,
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewEmbyConnector(srv.URL, "embykey", srv.Client())
	libs, err := c.ListLibraries(ctx)
	require.NoError(t, err)
	require.Len(t, libs, 2)
	require.Equal(t, "x", libs[0].ID)
	require.Equal(t, "Films", libs[0].Name)
	require.Equal(t, "movies", libs[0].CollectionType)
	require.Len(t, libs[0].Paths, 1)
	require.Equal(t, "y", libs[1].ID)
	require.Empty(t, libs[1].Paths)
}

func TestEmbyConnector_RefreshLibrary_Specific(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		require.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewEmbyConnector(srv.URL, "embykey", srv.Client())
	err := c.RefreshLibrary(ctx, "libx")
	require.NoError(t, err)
	require.Equal(t, "/Items/libx/Refresh", gotPath)
}

func TestEmbyConnector_RefreshLibrary_Global(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		require.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewEmbyConnector(srv.URL, "embykey", srv.Client())
	err := c.RefreshLibrary(ctx, "")
	require.NoError(t, err)
	require.Equal(t, "/Library/Refresh", gotPath)
}

func TestEmbyConnector_ScanStatus_Running(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]ScheduledTask{
			{Name: "Scan Media Library", Key: "RefreshLibrary", State: "Running", CurrentProgressPercentage: 75.0},
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewEmbyConnector(srv.URL, "embykey", srv.Client())
	status, err := c.ScanStatus(ctx)
	require.NoError(t, err)
	require.True(t, status.Running)
	require.Equal(t, 75.0, status.Percent)
}

func TestEmbyConnector_ScanStatus_Idle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]ScheduledTask{
			{Key: "RefreshLibrary", State: "Idle"},
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewEmbyConnector(srv.URL, "embykey", srv.Client())
	status, err := c.ScanStatus(ctx)
	require.NoError(t, err)
	require.False(t, status.Running)
}

func TestEmbyConnector_Name(t *testing.T) {
	c := NewEmbyConnector("http://localhost:6789", "", nil)
	require.Equal(t, "emby", c.Name())
}
