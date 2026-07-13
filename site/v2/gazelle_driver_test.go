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

func TestNewGazelleDriver(t *testing.T) {
	config := GazelleDriverConfig{
		BaseURL: "https://example.com",
		APIKey:  "test-api-key",
		Cookie:  "test-cookie",
	}

	driver := NewGazelleDriver(config)

	assert.Equal(t, "https://example.com", driver.BaseURL)
	assert.Equal(t, "test-api-key", driver.APIKey)
	assert.Equal(t, "test-cookie", driver.Cookie)
	assert.NotNil(t, driver.httpClient)
}

func TestGazelleDriver_PrepareSearch(t *testing.T) {
	driver := NewGazelleDriver(GazelleDriverConfig{
		BaseURL: "https://example.com",
		APIKey:  "test-api-key",
	})

	tests := []struct {
		name       string
		query      SearchQuery
		wantAction string
		wantKey    string
		wantVal    string
	}{
		{
			name:       "keyword search",
			query:      SearchQuery{Keyword: "test album"},
			wantAction: "browse",
			wantKey:    "searchstr",
			wantVal:    "test album",
		},
		{
			name:       "free only",
			query:      SearchQuery{FreeOnly: true},
			wantAction: "browse",
			wantKey:    "freetorrent",
			wantVal:    "1",
		},
		{
			name:       "with page",
			query:      SearchQuery{Page: 2},
			wantAction: "browse",
			wantKey:    "page",
			wantVal:    "2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := driver.PrepareSearch(tt.query)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAction, req.Action)
			if tt.wantKey != "" {
				assert.Equal(t, tt.wantVal, req.Params.Get(tt.wantKey))
			}
		})
	}
}

func TestGazelleDriver_Execute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		assert.Equal(t, "test-api-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))

		// Verify action parameter
		assert.Equal(t, "browse", r.URL.Query().Get("action"))

		resp := GazelleResponse{
			Status:   "success",
			Response: json.RawMessage(`{"currentPage":1,"pages":1,"results":[]}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	driver := NewGazelleDriver(GazelleDriverConfig{
		BaseURL: server.URL,
		APIKey:  "test-api-key",
	})

	req := GazelleRequest{
		Action: "browse",
	}

	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "success", res.Status)
}

func TestGazelleDriver_Execute_WithCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify cookie is set
		assert.Contains(t, r.Header.Get("Cookie"), "test-cookie")

		resp := GazelleResponse{
			Status:   "success",
			Response: json.RawMessage(`{}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	driver := NewGazelleDriver(GazelleDriverConfig{
		BaseURL: server.URL,
		Cookie:  "test-cookie",
	})

	req := GazelleRequest{Action: "index"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "success", res.Status)
}

func TestGazelleDriver_Execute_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	driver := NewGazelleDriver(GazelleDriverConfig{
		BaseURL: server.URL,
		APIKey:  "invalid-key",
	})

	req := GazelleRequest{Action: "browse"}
	_, err := driver.Execute(context.Background(), req)
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestGazelleDriver_Execute_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := GazelleResponse{
			Status: "failure",
			Error:  "Invalid action",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	driver := NewGazelleDriver(GazelleDriverConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	})

	req := GazelleRequest{Action: "invalid"}
	_, err := driver.Execute(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}

func TestGazelleDriver_ParseSearch(t *testing.T) {
	driver := NewGazelleDriver(GazelleDriverConfig{
		BaseURL: "https://example.com",
		APIKey:  "test-api-key",
	})

	searchResp := GazelleSearchResponse{
		CurrentPage: 1,
		Pages:       1,
		Results: []GazelleTorrentGroup{
			{
				GroupID:   100,
				GroupName: "Test Album",
				Artist:    "Test Artist",
				Tags:      []string{"rock", "2024"},
				Torrents: []GazelleTorrent{
					{
						TorrentID:   12345,
						Format:      "FLAC",
						Encoding:    "Lossless",
						Size:        524288000, // 500 MB
						Seeders:     50,
						Leechers:    5,
						Snatches:    200,
						IsFreeleech: true,
						Time:        "2024-01-15 10:30:00",
					},
				},
			},
		},
	}

	respBytes, _ := json.Marshal(searchResp)
	res := GazelleResponse{
		Status:     "success",
		Response:   respBytes,
		StatusCode: http.StatusOK,
	}

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, "12345", items[0].ID)
	assert.Contains(t, items[0].Title, "Test Artist")
	assert.Contains(t, items[0].Title, "Test Album")
	assert.Contains(t, items[0].Title, "FLAC")
	assert.Equal(t, int64(524288000), items[0].SizeBytes)
	assert.Equal(t, 50, items[0].Seeders)
	assert.Equal(t, 5, items[0].Leechers)
	assert.Equal(t, 200, items[0].Snatched)
	assert.Equal(t, DiscountFree, items[0].DiscountLevel)
	assert.Contains(t, items[0].Tags, "rock")
}

func TestGazelleDriver_PrepareUserInfo(t *testing.T) {
	driver := NewGazelleDriver(GazelleDriverConfig{
		BaseURL: "https://example.com",
		APIKey:  "test-api-key",
	})

	req, err := driver.PrepareUserInfo()
	require.NoError(t, err)
	assert.Equal(t, "index", req.Action)
}

func TestGazelleDriver_ParseUserInfo(t *testing.T) {
	driver := NewGazelleDriver(GazelleDriverConfig{
		BaseURL: "https://example.com",
		APIKey:  "test-api-key",
	})

	userResp := GazelleUserResponse{
		Username: "testuser",
		ID:       12345,
		Stats: struct {
			Uploaded   int64   `json:"uploaded"`
			Downloaded int64   `json:"downloaded"`
			Ratio      float64 `json:"ratio"`
			Buffer     int64   `json:"buffer"`
			LastAccess string  `json:"LastAccess,omitempty"`
		}{
			Uploaded:   1099511627776, // 1 TB
			Downloaded: 549755813888,  // 512 GB
			Ratio:      2.0,
		},
		Ranks: struct {
			Class string `json:"class"`
		}{Class: "Power User"},
		Personal: struct {
			Bonus float64 `json:"bonus"`
		}{Bonus: 10000.5},
		Community: struct {
			Seeding  int `json:"seeding"`
			Leeching int `json:"leeching"`
		}{Seeding: 50, Leeching: 2},
	}

	respBytes, _ := json.Marshal(userResp)
	res := GazelleResponse{
		Status:     "success",
		Response:   respBytes,
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

func TestGazelleDriver_PrepareDownload(t *testing.T) {
	driver := NewGazelleDriver(GazelleDriverConfig{
		BaseURL: "https://example.com",
		APIKey:  "test-api-key",
	})

	req, err := driver.PrepareDownload("12345")
	require.NoError(t, err)
	assert.Equal(t, "download", req.Action)
	assert.Equal(t, "12345", req.Params.Get("id"))
}

func TestGazelleDriver_GetTorrentDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "torrent", r.URL.Query().Get("action"))
		assert.Equal(t, "12345", r.URL.Query().Get("id"))

		resp := GazelleResponse{
			Status: "success",
			Response: json.RawMessage(`{
				"group": {"name": "Test Album", "artist": "Test Artist"},
				"torrent": {
					"torrentId": 12345,
					"format": "FLAC",
					"encoding": "Lossless",
					"size": 524288000,
					"seeders": 20,
					"leechers": 3,
					"snatches": 66,
					"isFreeleech": true,
					"time": "2024-01-15 10:30:00"
				}
			}`),
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	driver := NewGazelleDriver(GazelleDriverConfig{BaseURL: server.URL, APIKey: "key"})

	item, err := driver.GetTorrentDetail(context.Background(), "", "https://mooko.org/torrents.php?id=12345", "")
	require.NoError(t, err)
	require.NotNil(t, item)

	assert.Equal(t, "12345", item.ID)
	assert.Equal(t, "Test Artist - Test Album [FLAC Lossless]", item.Title)
	assert.Equal(t, int64(524288000), item.SizeBytes)
	assert.Equal(t, 20, item.Seeders)
	assert.Equal(t, 3, item.Leechers)
	assert.Equal(t, 66, item.Snatched)
	assert.Equal(t, DiscountFree, item.DiscountLevel)
	assert.Equal(t, server.URL, item.SourceSite)
	assert.NotZero(t, item.UploadedAt)
}

func TestExtractGazelleTorrentID(t *testing.T) {
	tests := []struct {
		name string
		guid string
		link string
		want string
	}{
		{name: "prefer link id query", guid: "", link: "https://mooko.org/torrents.php?id=12345", want: "12345"},
		{name: "guid as url", guid: "https://mooko.org/torrents.php?torrentid=23456", link: "", want: "23456"},
		{name: "guid numeric", guid: "34567", link: "", want: "34567"},
		{name: "path numeric", guid: "", link: "https://mooko.org/torrent/45678", want: "45678"},
		{name: "invalid", guid: "abc", link: "https://mooko.org/torrents.php?id=abc", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractGazelleTorrentID(tt.guid, tt.link))
		})
	}
}

func TestParseGazelleDiscount(t *testing.T) {
	tests := []struct {
		name           string
		isFreeleech    bool
		isNeutralLeech bool
		isPersonalFL   bool
		expected       DiscountLevel
	}{
		{"freeleech", true, false, false, DiscountFree},
		{"neutral leech", false, true, false, DiscountFree},
		{"personal freeleech", false, false, true, DiscountFree},
		{"no discount", false, false, false, DiscountNone},
		{"freeleech and neutral", true, true, false, DiscountFree},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseGazelleDiscount(tt.isFreeleech, tt.isNeutralLeech, tt.isPersonalFL))
		})
	}
}

func TestSelectGazelleDetailTorrent_SingleTorrent(t *testing.T) {
	detail := gazelleTorrentDetailResponse{Torrent: GazelleTorrent{TorrentID: 7, Size: 100}}
	got := selectGazelleDetailTorrent(detail, 0)
	assert.Equal(t, 7, got.TorrentID)
}

func TestSelectGazelleDetailTorrent_MatchInSlice(t *testing.T) {
	detail := gazelleTorrentDetailResponse{Torrents: []GazelleTorrent{
		{TorrentID: 1}, {TorrentID: 2}, {TorrentID: 3},
	}}
	got := selectGazelleDetailTorrent(detail, 2)
	assert.Equal(t, 2, got.TorrentID)
}

func TestSelectGazelleDetailTorrent_FallbackFirst(t *testing.T) {
	detail := gazelleTorrentDetailResponse{Torrents: []GazelleTorrent{
		{TorrentID: 10}, {TorrentID: 20},
	}}
	got := selectGazelleDetailTorrent(detail, 999) // no match -> first
	assert.Equal(t, 10, got.TorrentID)
}

func TestSelectGazelleDetailTorrent_FallbackTopLevel(t *testing.T) {
	detail := gazelleTorrentDetailResponse{ID: 55, Format: "FLAC", Size: 999}
	got := selectGazelleDetailTorrent(detail, 0)
	assert.Equal(t, 55, got.TorrentID)
	assert.Equal(t, "FLAC", got.Format)
	assert.Equal(t, int64(999), got.Size)
}

func TestGazelleDriver_GetUserInfo_ExecuteError(t *testing.T) {
	d := NewGazelleDriver(GazelleDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "key"})
	_, err := d.GetUserInfo(context.Background())
	require.Error(t, err)
}

func TestGazelleDriver_GetTorrentDetail_Array(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","response":{"group":{"name":"Album","artist":"Artist"},
			"torrents":[{"torrentId":5,"format":"FLAC","encoding":"Lossless","size":2048,"seeders":3,"leechers":1,"snatches":9,"isFreeleech":true,"time":"2024-01-01 10:00:00"}]}}`))
	}))
	defer server.Close()

	d := NewGazelleDriver(GazelleDriverConfig{BaseURL: server.URL, APIKey: "key"})
	item, err := d.GetTorrentDetail(context.Background(), "5", server.URL+"/torrents.php?id=5", "fallback")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "5", item.ID)
	assert.Contains(t, item.Title, "Album")
	assert.Equal(t, DiscountFree, item.DiscountLevel)
	assert.Greater(t, item.UploadedAt, int64(0))
}

// ---------------------------------------------------------------------------
// unit3d_driver.go — Execute error paths, GetUserInfo
// ---------------------------------------------------------------------------

func TestGazelleDriver_GetUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","response":{
			"id":42,"username":"gztester",
			"stats":{"uploaded":10737418240,"downloaded":1073741824,"ratio":10.0,"LastAccess":"2024-06-01 12:00:00"},
			"ranks":{"class":"Elite"},
			"personal":{"bonus":5000},
			"community":{"seeding":15,"leeching":1}
		}}`))
	}))
	defer server.Close()

	d := NewGazelleDriver(GazelleDriverConfig{BaseURL: server.URL, APIKey: "k"})
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "42", info.UserID)
	assert.Equal(t, "gztester", info.Username)
	assert.Equal(t, int64(10737418240), info.Uploaded)
	assert.Equal(t, "Elite", info.Rank)
	assert.Equal(t, 15, info.Seeding)
	assert.Greater(t, info.LastAccess, int64(0))
}

func TestGazelleDriver_ParseDownload(t *testing.T) {
	d := NewGazelleDriver(GazelleDriverConfig{BaseURL: "https://x.com"})
	data, err := d.ParseDownload(GazelleResponse{RawBody: []byte("torrent")})
	require.NoError(t, err)
	assert.Equal(t, []byte("torrent"), data)

	_, err = d.ParseDownload(GazelleResponse{})
	assert.ErrorIs(t, err, ErrParseError)
}
