package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJellyfinConnector_Ping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/System/Info/Public", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PublicInfo{
			ProductName: "Jellyfin Server",
			Version:     "10.11.8",
			Id:          "server-id",
			ServerName:  "My Jellyfin",
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewJellyfinConnector(srv.URL, "", srv.Client())
	info, err := c.Ping(ctx)
	require.NoError(t, err)
	require.Equal(t, "Jellyfin Server", info.Product)
	require.Equal(t, "10.11.8", info.Version)
	require.Equal(t, "server-id", info.ServerID)
	require.Equal(t, "My Jellyfin", info.Name)
}

func TestJellyfinConnector_Authenticate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/System/Info", r.URL.Path)
		require.Equal(t, "testkey", r.Header.Get("X-Emby-Token"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PublicInfo{
			ProductName: "Jellyfin Server",
			Version:     "10.11.8",
			Id:          "server-id",
			ServerName:  "My Jellyfin",
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewJellyfinConnector(srv.URL, "testkey", srv.Client())
	info, err := c.Authenticate(ctx)
	require.NoError(t, err)
	require.Equal(t, "Jellyfin Server", info.Product)
	require.Equal(t, "10.11.8", info.Version)
}

func TestJellyfinConnector_ListLibraries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/Library/MediaFolders", r.URL.Path)
		require.Equal(t, "testkey", r.Header.Get("X-Emby-Token"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(MediaFoldersResponse{
			Items: []MediaFolder{
				{Id: "a", Name: "Movies", CollectionType: "movies", Path: "/media/movies"},
				{Id: "b", Name: "TV Shows", CollectionType: "tvshows"},
			},
			TotalRecordCount: 2,
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewJellyfinConnector(srv.URL, "testkey", srv.Client())
	libs, err := c.ListLibraries(ctx)
	require.NoError(t, err)
	require.Len(t, libs, 2)
	require.Equal(t, "a", libs[0].ID)
	require.Equal(t, "Movies", libs[0].Name)
	require.Equal(t, "movies", libs[0].CollectionType)
	require.Len(t, libs[0].Paths, 1)
	require.Equal(t, "/media/movies", libs[0].Paths[0])
	require.Equal(t, "b", libs[1].ID)
	require.Equal(t, "TV Shows", libs[1].Name)
	require.Empty(t, libs[1].Paths)
}

func TestJellyfinConnector_RefreshLibrary_Specific(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		require.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewJellyfinConnector(srv.URL, "testkey", srv.Client())
	err := c.RefreshLibrary(ctx, "abc123")
	require.NoError(t, err)
	require.Equal(t, "/Items/abc123/Refresh", gotPath)
	require.Contains(t, gotQuery, "MetadataRefreshMode=Default")
	require.Contains(t, gotQuery, "Recursive=true")
}

func TestJellyfinConnector_RefreshLibrary_Global(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		require.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewJellyfinConnector(srv.URL, "testkey", srv.Client())
	err := c.RefreshLibrary(ctx, "")
	require.NoError(t, err)
	require.Equal(t, "/Library/Refresh", gotPath)
}

func TestJellyfinConnector_ScanStatus_Running(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/ScheduledTasks", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]ScheduledTask{
			{Name: "Refresh Library", Key: "RefreshLibrary", State: "Running", CurrentProgressPercentage: 42.5},
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewJellyfinConnector(srv.URL, "testkey", srv.Client())
	status, err := c.ScanStatus(ctx)
	require.NoError(t, err)
	require.True(t, status.Running)
	require.Equal(t, 42.5, status.Percent)
	require.Equal(t, "Refresh Library", status.TaskName)
}

func TestJellyfinConnector_ScanStatus_Idle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]ScheduledTask{
			{Key: "RefreshLibrary", State: "Idle"},
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewJellyfinConnector(srv.URL, "", srv.Client())
	status, err := c.ScanStatus(ctx)
	require.NoError(t, err)
	require.False(t, status.Running)
	require.Equal(t, 0.0, status.Percent)
}

func TestJellyfinConnector_ScanStatus_NoRefreshTask(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]ScheduledTask{
			{Key: "SomeOtherTask", State: "Idle"},
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewJellyfinConnector(srv.URL, "", srv.Client())
	status, err := c.ScanStatus(ctx)
	require.NoError(t, err)
	require.False(t, status.Running)
}

func TestJellyfinConnector_Name(t *testing.T) {
	c := NewJellyfinConnector("http://localhost:8096", "", nil)
	require.Equal(t, "jellyfin", c.Name())
}

func TestJellyfinConnector_Authenticate_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewJellyfinConnector(srv.URL, "invalid", srv.Client())
	_, err := c.Authenticate(ctx)
	require.Error(t, err)
}
