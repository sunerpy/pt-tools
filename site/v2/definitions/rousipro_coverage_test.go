package definitions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func newTestRousiDriverWithURL(baseURL string) *rousiDriver {
	return newRousiDriver(rousiDriverConfig{
		BaseURL: baseURL,
		Passkey: "FAKE_TEST_PASSKEY_1234",
	})
}

func TestRousiDriver_PrepareSearch(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")

	req, err := d.PrepareSearch(v2.SearchQuery{Keyword: "matrix", Page: 0})
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/torrents", req.Endpoint)
	assert.Equal(t, http.MethodGet, req.Method)
	assert.Equal(t, "matrix", req.Params["keyword"])
	assert.Equal(t, "1", req.Params["page"])
	assert.Equal(t, "100", req.Params["page_size"])

	// page > 0 -> page+1
	req2, err := d.PrepareSearch(v2.SearchQuery{Page: 2})
	require.NoError(t, err)
	assert.Equal(t, "3", req2.Params["page"])
	_, hasKeyword := req2.Params["keyword"]
	assert.False(t, hasKeyword)
}

func TestRousiDriver_PrepareDownload(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")
	req, err := d.PrepareDownload("uuid-123")
	require.NoError(t, err)
	assert.Contains(t, req.Endpoint, "/api/torrent/uuid-123/download/")
	assert.Equal(t, http.MethodGet, req.Method)
}

func TestRousiDriver_ParseDownload(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")
	data, err := d.ParseDownload(rousiResponse{RawBody: []byte("torrentbytes")})
	require.NoError(t, err)
	assert.Equal(t, []byte("torrentbytes"), data)

	_, err = d.ParseDownload(rousiResponse{})
	assert.Error(t, err)
}

func TestExtractUUIDFromLink(t *testing.T) {
	assert.Equal(t, "abc-123", extractUUIDFromLink("https://rousi.pro/torrent/abc-123"))
	assert.Equal(t, "abc-123", extractUUIDFromLink("https://rousi.pro/torrent/abc-123/"))
	assert.Equal(t, "", extractUUIDFromLink(""))
}

func TestRousiDriver_Execute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer FAKE_TEST_PASSKEY_1234", r.Header.Get("Authorization"))
		assert.Equal(t, "matrix", r.URL.Query().Get("keyword"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0,"message":"success","data":{"torrents":[],"total":0}}`))
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	res, err := d.Execute(context.Background(), rousiRequest{
		Endpoint: "/api/v1/torrents",
		Method:   http.MethodGet,
		Params:   map[string]string{"keyword": "matrix"},
	})
	require.NoError(t, err)
	assert.True(t, res.IsSuccess())
}

func TestRousiDriver_Execute_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	_, err := d.Execute(context.Background(), rousiRequest{Endpoint: "/api/v1/profile"})
	assert.ErrorIs(t, err, v2.ErrInvalidCredentials)
}

func TestRousiDriver_Execute_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code":500,"message":"server error"}`))
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	_, err := d.Execute(context.Background(), rousiRequest{Endpoint: "/x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server error")
}

func TestRousiDriver_GetUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/api/v1/profile")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(rousiUserInfoFixtureJSON))
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "9876", info.UserID)
	assert.Equal(t, "testuser", info.Username)
	assert.Equal(t, int64(1099511627776), info.Uploaded)
	assert.InDelta(t, 10.24, info.Ratio, 0.01)
	assert.Equal(t, "Power User", info.LevelName)
	assert.Equal(t, 120, info.SeederCount)
	assert.Equal(t, int64(5497558138880), info.SeederSize)
	assert.Greater(t, info.JoinDate, int64(0))
	assert.Greater(t, info.LastAccess, int64(0))
}

func TestRousiDriver_GetUserInfo_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":1,"message":"denied"}`))
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	_, err := d.GetUserInfo(context.Background())
	assert.Error(t, err)
}

func TestRousiDriver_GetTorrentDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/api/v1/torrents/")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0,"message":"success","data":{
			"uuid":"detail-uuid","title":"Detail Movie","subtitle":"sub",
			"size":1073741824,"seeders":5,"leechers":1,"downloads":10,
			"created_at":"2025-01-15T08:30:00+08:00",
			"promotion":{"type":2,"is_active":true}
		}}`))
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	item, err := d.GetTorrentDetail(context.Background(), "guid", server.URL+"/torrent/detail-uuid", "")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "detail-uuid", item.ID)
	assert.Equal(t, "Detail Movie", item.Title)
	assert.Equal(t, v2.DiscountFree, item.DiscountLevel)
	assert.Greater(t, item.UploadedAt, int64(0))
}

func TestRousiDriver_GetTorrentDetail_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	item, err := d.GetTorrentDetail(context.Background(), "guid", "", "")
	require.NoError(t, err)
	assert.Nil(t, item)
}

func TestRousiDriver_GetTorrentDetail_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":1,"message":"bad"}`))
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	_, err := d.GetTorrentDetail(context.Background(), "guid", "", "")
	assert.Error(t, err)
}

func TestRousiDriver_GetTorrentDetail_GuidFallback(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0,"message":"success","data":{"uuid":"g","title":"t"}}`))
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	_, err := d.GetTorrentDetail(context.Background(), "guid-only", "", "")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(gotPath, "guid-only"))
}
