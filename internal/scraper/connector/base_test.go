package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func newTestClient(t *testing.T, h http.HandlerFunc) (*baseClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &baseClient{BaseURL: srv.URL, APIKey: "testkey", Client: srv.Client()}, srv
}

func TestBaseClient_DoAuthHeader(t *testing.T) {
	var gotToken, gotAccept string
	bc, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Emby-Token")
		gotAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{}`)
	})

	var out struct{}
	err := bc.do(context.Background(), http.MethodGet, "/test", nil, &out)
	require.NoError(t, err)
	require.Equal(t, "testkey", gotToken)
	require.Equal(t, "application/json", gotAccept)
}

func TestBaseClient_DoNoAuthWhenEmpty(t *testing.T) {
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Emby-Token")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{}`)
	}))
	defer srv.Close()

	bc := &baseClient{BaseURL: srv.URL, Client: srv.Client()}
	var out struct{}
	err := bc.do(context.Background(), http.MethodGet, "/test", nil, &out)
	require.NoError(t, err)
	require.Equal(t, "", gotToken)
}

func TestBaseClient_Do404(t *testing.T) {
	bc, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	err := bc.do(context.Background(), http.MethodGet, "/x", nil, nil)
	require.ErrorIs(t, err, core.ErrNotFound)
}

func TestBaseClient_Do401(t *testing.T) {
	bc, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	err := bc.do(context.Background(), http.MethodGet, "/x", nil, nil)
	require.ErrorIs(t, err, core.ErrUnauthorized)
}

func TestBaseClient_Do403(t *testing.T) {
	bc, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	err := bc.do(context.Background(), http.MethodGet, "/x", nil, nil)
	require.ErrorIs(t, err, core.ErrUnauthorized)
}

func TestBaseClient_Do500(t *testing.T) {
	bc, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	err := bc.do(context.Background(), http.MethodGet, "/x", nil, nil)
	require.ErrorIs(t, err, core.ErrProviderDown)
}

func TestBaseClient_Do502(t *testing.T) {
	bc, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	err := bc.do(context.Background(), http.MethodGet, "/x", nil, nil)
	require.ErrorIs(t, err, core.ErrProviderDown)
}

func TestBaseClient_DoOtherStatus(t *testing.T) {
	bc, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	err := bc.do(context.Background(), http.MethodGet, "/x", nil, nil)
	require.Error(t, err)
	require.NotErrorIs(t, err, core.ErrNotFound)
	require.NotErrorIs(t, err, core.ErrUnauthorized)
	require.NotErrorIs(t, err, core.ErrProviderDown)
}

func TestBaseClient_Do204NoBody(t *testing.T) {
	bc, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	type payload struct {
		Name string `json:"name"`
	}
	out := payload{Name: "keep"}
	err := bc.do(context.Background(), http.MethodGet, "/x", nil, &out)
	require.NoError(t, err)
	require.Equal(t, "keep", out.Name)
}

func TestBaseClient_DoNilOut(t *testing.T) {
	bc, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"foo":"bar"}`)
	})
	err := bc.do(context.Background(), http.MethodGet, "/x", nil, nil)
	require.NoError(t, err)
}

func TestBaseClient_DoPostBody(t *testing.T) {
	type reqBody struct {
		Name string `json:"name"`
	}
	var gotCT string
	var gotBody reqBody
	bc, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &gotBody)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"ok":true}`)
	})

	err := bc.do(context.Background(), http.MethodPost, "/create", reqBody{Name: "hello"}, nil)
	require.NoError(t, err)
	require.Equal(t, "application/json", gotCT)
	require.Equal(t, "hello", gotBody.Name)
}

func TestBaseClient_DoDecodeError(t *testing.T) {
	bc, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `not-json`)
	})
	var out map[string]any
	err := bc.do(context.Background(), http.MethodGet, "/x", nil, &out)
	require.Error(t, err)
}

func TestBaseClient_DoTrimsTrailingSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{}`)
	}))
	defer srv.Close()

	bc := &baseClient{BaseURL: srv.URL + "/", Client: srv.Client()}
	err := bc.do(context.Background(), http.MethodGet, "/System/Info/Public", nil, nil)
	require.NoError(t, err)
	require.Equal(t, "/System/Info/Public", gotPath)
}

func TestBaseClient_DoContextCancel(t *testing.T) {
	bc, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := bc.do(ctx, http.MethodGet, "/x", nil, nil)
	require.Error(t, err)
}

func TestDetectServerType_Jellyfin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/System/Info/Public", r.URL.Path)
		_ = json.NewEncoder(w).Encode(PublicInfo{ProductName: "Jellyfin Server", Version: "10.11.8"})
	}))
	defer srv.Close()

	kind, err := DetectServerType(context.Background(), srv.URL, srv.Client())
	require.NoError(t, err)
	require.Equal(t, ServerTypeJellyfin, kind)
}

func TestDetectServerType_Emby(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PublicInfo{ProductName: "Emby Server", Version: "4.8"})
	}))
	defer srv.Close()

	kind, err := DetectServerType(context.Background(), srv.URL, srv.Client())
	require.NoError(t, err)
	require.Equal(t, ServerTypeEmby, kind)
}

func TestDetectServerType_CaseInsensitive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PublicInfo{ProductName: "JELLYFIN SERVER"})
	}))
	defer srv.Close()

	kind, err := DetectServerType(context.Background(), srv.URL, srv.Client())
	require.NoError(t, err)
	require.Equal(t, ServerTypeJellyfin, kind)
}

func TestDetectServerType_Unknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PublicInfo{ProductName: "Plex Media Server"})
	}))
	defer srv.Close()

	_, err := DetectServerType(context.Background(), srv.URL, srv.Client())
	require.Error(t, err)
	require.ErrorIs(t, err, core.ErrUnsupported)
}

func TestDetectServerType_NetworkError(t *testing.T) {
	_, err := DetectServerType(context.Background(), "http://127.0.0.1:1", nil)
	require.Error(t, err)
}

func TestDetectServerType_NilClientDefaults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PublicInfo{ProductName: "Jellyfin Server"})
	}))
	defer srv.Close()

	kind, err := DetectServerType(context.Background(), srv.URL, nil)
	require.NoError(t, err)
	require.Equal(t, ServerTypeJellyfin, kind)
}
