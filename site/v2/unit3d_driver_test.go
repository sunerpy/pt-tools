package v2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUnit3DDriver(t *testing.T) {
	config := Unit3DDriverConfig{
		BaseURL: "https://example.com",
		APIKey:  "test-api-key",
	}

	driver := NewUnit3DDriver(config)

	assert.Equal(t, "https://example.com", driver.BaseURL)
	assert.Equal(t, "test-api-key", driver.APIKey)
	assert.NotNil(t, driver.httpClient)
}

func TestUnit3DDriver_PrepareSearch(t *testing.T) {
	driver := NewUnit3DDriver(Unit3DDriverConfig{
		BaseURL: "https://example.com",
		APIKey:  "test-api-key",
	})

	tests := []struct {
		name    string
		query   SearchQuery
		wantKey string
		wantVal string
	}{
		{
			name:    "keyword search",
			query:   SearchQuery{Keyword: "test movie"},
			wantKey: "name",
			wantVal: "test movie",
		},
		{
			name:    "free only",
			query:   SearchQuery{FreeOnly: true},
			wantKey: "freeleech",
			wantVal: "1",
		},
		{
			name:    "with page",
			query:   SearchQuery{Page: 2},
			wantKey: "page",
			wantVal: "2",
		},
		{
			name:    "with page size",
			query:   SearchQuery{PageSize: 50},
			wantKey: "perPage",
			wantVal: "50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := driver.PrepareSearch(tt.query)
			require.NoError(t, err)
			assert.Equal(t, "/api/torrents/filter", req.Endpoint)
			assert.Equal(t, "GET", req.Method)
			if tt.wantKey != "" {
				assert.Equal(t, tt.wantVal, req.Params.Get(tt.wantKey))
			}
		})
	}
}

func TestUnit3DDriver_Execute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))

		resp := Unit3DResponse{
			Data: json.RawMessage(`[]`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	driver := NewUnit3DDriver(Unit3DDriverConfig{
		BaseURL: server.URL,
		APIKey:  "test-api-key",
	})

	req := Unit3DRequest{
		Endpoint: "/api/torrents/filter",
		Method:   "GET",
	}

	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestUnit3DDriver_Execute_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	driver := NewUnit3DDriver(Unit3DDriverConfig{
		BaseURL: server.URL,
		APIKey:  "invalid-key",
	})

	req := Unit3DRequest{Endpoint: "/api/test", Method: "GET"}
	_, err := driver.Execute(context.Background(), req)
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestUnit3DDriver_ParseSearch(t *testing.T) {
	driver := NewUnit3DDriver(Unit3DDriverConfig{
		BaseURL: "https://example.com",
		APIKey:  "test-api-key",
	})

	torrents := []Unit3DTorrent{
		{
			ID:             12345,
			Name:           "Test Movie 2024",
			InfoHash:       "abc123def456",
			Size:           1073741824, // 1 GB
			Seeders:        100,
			Leechers:       10,
			TimesCompleted: 500,
			Category: struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			}{ID: 1, Name: "Movies"},
			Freeleech:    "100",
			DoubleUpload: true,
			CreatedAt:    "2024-01-15T10:30:00Z",
		},
	}

	dataBytes, _ := json.Marshal(torrents)
	res := Unit3DResponse{
		Data:       dataBytes,
		StatusCode: http.StatusOK,
	}

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, "12345", items[0].ID)
	assert.Equal(t, "Test Movie 2024", items[0].Title)
	assert.Equal(t, "abc123def456", items[0].InfoHash)
	assert.Equal(t, int64(1073741824), items[0].SizeBytes)
	assert.Equal(t, 100, items[0].Seeders)
	assert.Equal(t, 10, items[0].Leechers)
	assert.Equal(t, 500, items[0].Snatched)
	assert.Equal(t, "Movies", items[0].Category)
	assert.Equal(t, Discount2xFree, items[0].DiscountLevel) // 100% free + double upload
}

func TestUnit3DDriver_PrepareUserInfo(t *testing.T) {
	driver := NewUnit3DDriver(Unit3DDriverConfig{
		BaseURL: "https://example.com",
		APIKey:  "test-api-key",
	})

	req, err := driver.PrepareUserInfo()
	require.NoError(t, err)
	assert.Equal(t, "/api/user", req.Endpoint)
	assert.Equal(t, "GET", req.Method)
}

func TestUnit3DDriver_ParseUserInfo(t *testing.T) {
	driver := NewUnit3DDriver(Unit3DDriverConfig{
		BaseURL: "https://example.com",
		APIKey:  "test-api-key",
	})

	profile := Unit3DUserProfile{
		ID:         12345,
		Username:   "testuser",
		Uploaded:   1099511627776, // 1 TB
		Downloaded: 549755813888,  // 512 GB
		Ratio:      2.0,
		Seedbonus:  10000.5,
		Seeding:    50,
		Leeching:   2,
		Group: struct {
			Name string `json:"name"`
		}{Name: "Power User"},
		CreatedAt: "2020-01-01T00:00:00Z",
	}

	dataBytes, _ := json.Marshal(profile)
	res := Unit3DResponse{
		Data:       dataBytes,
		StatusCode: http.StatusOK,
	}

	info, err := driver.ParseUserInfo(res)
	require.NoError(t, err)

	assert.Equal(t, "12345", info.UserID)
	assert.Equal(t, "testuser", info.Username)
	assert.Equal(t, int64(1099511627776), info.Uploaded)
	assert.Equal(t, int64(549755813888), info.Downloaded)
	assert.Equal(t, 2.0, info.Ratio)
	assert.Equal(t, 10000.5, info.Bonus)
	assert.Equal(t, 50, info.Seeding)
	assert.Equal(t, 2, info.Leeching)
	assert.Equal(t, "Power User", info.Rank)
}

func TestUnit3DDriver_PrepareDownload(t *testing.T) {
	driver := NewUnit3DDriver(Unit3DDriverConfig{
		BaseURL: "https://example.com",
		APIKey:  "test-api-key",
	})

	req, err := driver.PrepareDownload("12345")
	require.NoError(t, err)
	assert.Equal(t, "/api/torrents/12345/download", req.Endpoint)
	assert.Equal(t, "GET", req.Method)
}

func TestParseUnit3DDiscount(t *testing.T) {
	tests := []struct {
		freeleech    string
		doubleUpload bool
		expected     DiscountLevel
	}{
		{"100", false, DiscountFree},
		{"100", true, Discount2xFree},
		{"1", false, DiscountFree},
		{"true", false, DiscountFree},
		{"50", false, DiscountPercent50},
		{"50", true, Discount2x50},
		{"25", false, DiscountPercent30},
		{"30", false, DiscountPercent30},
		{"75", false, DiscountPercent70},
		{"70", false, DiscountPercent70},
		{"0", true, Discount2xUp},
		{"0", false, DiscountNone},
		{"", false, DiscountNone},
	}

	for _, tt := range tests {
		name := tt.freeleech
		if tt.doubleUpload {
			name += "_2x"
		}
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseUnit3DDiscount(tt.freeleech, tt.doubleUpload))
		})
	}
}

func TestUnit3DDriver_Execute_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := d.Execute(context.Background(), Unit3DRequest{Endpoint: "/api/user"})
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestUnit3DDriver_Execute_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := d.Execute(context.Background(), Unit3DRequest{Endpoint: "/api/user"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 502")
}

func TestUnit3DDriver_Execute_BadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := d.Execute(context.Background(), Unit3DRequest{Endpoint: "/api/user"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse JSON")
}

func TestUnit3DDriver_GetUserInfo_Error(t *testing.T) {
	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "k"})
	_, err := d.GetUserInfo(context.Background())
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// base_site.go — DownloadWithHash, GetDetailFetcher
// ---------------------------------------------------------------------------

func TestUnit3DDriver_ParseSearch_Full(t *testing.T) {
	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: "https://u.example", APIKey: "k"})
	data := []byte(`[{"id":5,"name":"Film","info_hash":"abc","size":1024,"seeders":10,"leechers":2,
		"times_completed":50,"category":{"name":"Movies"},"freeleech":"100","double_upload":true,
		"created_at":"2024-06-01T12:00:00Z","freeleech_ends":"2024-07-01T12:00:00Z","download_link":"https://dl"}]`)
	items, err := d.ParseSearch(Unit3DResponse{Data: data})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "5", items[0].ID)
	assert.Equal(t, "Film", items[0].Title)
	assert.Equal(t, Discount2xFree, items[0].DiscountLevel)
	assert.Greater(t, items[0].UploadedAt, int64(0))
	assert.False(t, items[0].DiscountEndTime.IsZero())
}

func TestUnit3DDriver_ParseSearch_BadJSON(t *testing.T) {
	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: "https://u.example", APIKey: "k"})
	_, err := d.ParseSearch(Unit3DResponse{Data: []byte("notjson")})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// driver_registry.go — CreateSiteFromDefinition custom driver + no factory
// ---------------------------------------------------------------------------

func TestUnit3DDriver_GetUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{
			"id":7,"username":"u3duser","uploaded":10737418240,"downloaded":1073741824,
			"ratio":10.0,"seedbonus":5000,"seeding":20,"leeching":2,
			"group":{"name":"Uploader"},"created_at":"2020-01-01T00:00:00Z",
			"last_login":"2024-06-01T12:00:00Z","last_action":"2024-06-02T09:00:00Z"
		}}`))
	}))
	defer server.Close()

	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: server.URL, APIKey: "k"})
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "7", info.UserID)
	assert.Equal(t, "u3duser", info.Username)
	assert.Equal(t, "Uploader", info.Rank)
	assert.Greater(t, info.JoinDate, int64(0))
	assert.Greater(t, info.LastLogin, int64(0))
	assert.Greater(t, info.LastAccess, int64(0))
}

func TestUnit3DDriver_ParseDownload(t *testing.T) {
	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: "https://x.com", APIKey: "k"})
	data, err := d.ParseDownload(Unit3DResponse{RawBody: []byte("torrent")})
	require.NoError(t, err)
	assert.Equal(t, []byte("torrent"), data)

	_, err = d.ParseDownload(Unit3DResponse{})
	assert.ErrorIs(t, err, ErrParseError)
}

func TestParseUnit3DTimestamp(t *testing.T) {
	assert.Equal(t, int64(0), parseUnit3DTimestamp(""))
	assert.Equal(t, int64(0), parseUnit3DTimestamp("garbage"))
	assert.Greater(t, parseUnit3DTimestamp("2024-06-01T12:00:00Z"), int64(0))
	assert.Greater(t, parseUnit3DTimestamp("2024-06-01 12:00:00"), int64(0))
}
