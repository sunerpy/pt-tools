package v2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHDDolbyDriver_Defaults(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://www.hddolby.com/"})
	assert.Equal(t, "https://www.hddolby.com", d.BaseURL)
	assert.Equal(t, "https://api.hddolby.com", d.APIURL)
	assert.Equal(t, "pt-tools/1.0", d.userAgent)
	assert.NotNil(t, d.httpClient)

	// explicit APIURL + userAgent
	d2 := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com", APIURL: "https://api.x.com/", UserAgent: "custom"})
	assert.Equal(t, "https://api.x.com", d2.APIURL)
	assert.Equal(t, "custom", d2.userAgent)
}

func TestHDDolbyDriver_PrepareSearch(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://hddolby.com"})
	req, err := d.PrepareSearch(SearchQuery{Keyword: "matrix"})
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/torrent/search", req.Endpoint)
	assert.Equal(t, http.MethodPost, req.Method)
	body := req.Body.(HDDolbySearchRequest)
	assert.Equal(t, "matrix", body.Keyword)
	assert.Equal(t, 100, body.PageSize)
	assert.Equal(t, 1, body.Visible)
}

func TestHDDolbyDriver_ParseSearch(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://hddolby.com"})
	searchJSON := `{"data":[{
		"id":123,"name":"Test.Movie.2024","small_descr":"sub",
		"category":402,"size":10737418240,"seeders":50,"leechers":5,
		"times_completed":100,"added":"2024-01-15 10:30:00",
		"promotion_time_type":1,"promotion_until":"2024-02-15 10:30:00",
		"downhash":"abc123","hr":1
	}],"total":1}`
	res := HDDolbyResponse{Data: json.RawMessage(searchJSON)}
	items, err := d.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "123", items[0].ID)
	assert.Equal(t, "Test.Movie.2024", items[0].Title)
	assert.Equal(t, "sub", items[0].Subtitle)
	assert.Equal(t, int64(10737418240), items[0].SizeBytes)
	assert.Equal(t, 50, items[0].Seeders)
	assert.Equal(t, DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, "Movies/HD", items[0].Category)
	assert.True(t, items[0].HasHR)
	assert.False(t, items[0].DiscountEndTime.IsZero())
	assert.Contains(t, items[0].DownloadURL, "downhash=abc123")
	assert.Greater(t, items[0].UploadedAt, int64(0))
}

func TestHDDolbyDriver_ParseSearch_ArrayFallback(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://hddolby.com"})
	arrayJSON := `[{"id":9,"name":"Bare","category":999,"tags":"gf"}]`
	res := HDDolbyResponse{Data: json.RawMessage(arrayJSON)}
	items, err := d.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "9", items[0].ID)
	assert.Equal(t, Discount2xFree, items[0].DiscountLevel)
	assert.Equal(t, "999", items[0].Category) // unknown category -> string ID
}

func TestHDDolbyDriver_ParseSearch_Invalid(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://hddolby.com"})
	_, err := d.ParseSearch(HDDolbyResponse{Data: json.RawMessage(`not json`)})
	assert.Error(t, err)
}

func TestHDDolbyDriver_ParseDiscount(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	assert.Equal(t, Discount2xFree, d.parseDiscount(0, "gf"))
	assert.Equal(t, DiscountFree, d.parseDiscount(0, "f"))
	assert.Equal(t, Discount2xUp, d.parseDiscount(0, "g"))
	assert.Equal(t, DiscountFree, d.parseDiscount(1, ""))
	assert.Equal(t, Discount2xUp, d.parseDiscount(2, ""))
	assert.Equal(t, Discount2xFree, d.parseDiscount(3, ""))
	assert.Equal(t, DiscountPercent50, d.parseDiscount(4, ""))
	assert.Equal(t, Discount2x50, d.parseDiscount(5, ""))
	assert.Equal(t, DiscountPercent30, d.parseDiscount(6, ""))
	assert.Equal(t, DiscountNone, d.parseDiscount(0, ""))
}

func TestHDDolbyDriver_ParseDiscountEndTime(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	assert.True(t, d.parseDiscountEndTime("").IsZero())
	assert.True(t, d.parseDiscountEndTime("0000-00-00 00:00:00").IsZero())
	assert.True(t, d.parseDiscountEndTime("invalid").IsZero())
	assert.False(t, d.parseDiscountEndTime("2024-02-15 10:30:00").IsZero())
}

func TestHDDolbyDriver_GetCategoryName(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	assert.Equal(t, "Movies/SD", d.getCategoryName(401))
	assert.Equal(t, "TV/HD", d.getCategoryName(408))
	assert.Equal(t, "Other", d.getCategoryName(418))
	assert.Equal(t, "12345", d.getCategoryName(12345))
}

func TestHDDolbyDriver_PrepareUserInfo(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	req, err := d.PrepareUserInfo()
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/user/data", req.Endpoint)
	assert.Equal(t, http.MethodGet, req.Method)
}

func TestHDDolbyDriver_ParseUserInfo(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://hddolby.com"})
	userJSON := `[{
		"id":"777","username":"tester","added":"2020-01-01 00:00:00",
		"last_access":"2024-06-01 12:00:00","class":"5",
		"uploaded":"10737418240","downloaded":"1073741824",
		"seedbonus":"50000","sebonus":"1234.5","unread_messages":"3"
	}]`
	info, err := d.ParseUserInfo(HDDolbyResponse{Data: json.RawMessage(userJSON)})
	require.NoError(t, err)
	assert.Equal(t, "hddolby", info.Site)
	assert.Equal(t, "777", info.UserID)
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, int64(10737418240), info.Uploaded)
	assert.Equal(t, int64(1073741824), info.Downloaded)
	assert.InDelta(t, 10.0, info.Ratio, 0.001)
	assert.InDelta(t, 50000, info.Bonus, 0.001)
	assert.InDelta(t, 1234.5, info.SeedingBonus, 0.001)
	assert.Equal(t, 3, info.UnreadMessageCount)
	assert.Greater(t, info.JoinDate, int64(0))
	assert.Greater(t, info.LastAccess, int64(0))
	assert.Equal(t, "5", info.LevelName) // no siteDef -> classID unchanged
}

func TestHDDolbyDriver_ParseUserInfo_Errors(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	_, err := d.ParseUserInfo(HDDolbyResponse{Data: json.RawMessage(`bad`)})
	assert.Error(t, err)
	_, err = d.ParseUserInfo(HDDolbyResponse{Data: json.RawMessage(`[]`)})
	assert.Error(t, err)
}

func TestHDDolbyDriver_GetLevelName(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	// no siteDef
	assert.Equal(t, "3", d.getLevelName("3"))

	d.SetSiteDefinition(&SiteDefinition{
		ID: "hddolby",
		LevelRequirements: []SiteLevelRequirement{
			{ID: 5, Name: "Elite User"},
		},
	})
	assert.Equal(t, "Elite User", d.getLevelName("5"))
	assert.Equal(t, "99", d.getLevelName("99"))   // not found
	assert.Equal(t, "abc", d.getLevelName("abc")) // non-numeric
}

func TestHDDolbyDriver_GetSiteID(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	assert.Equal(t, "hddolby", d.getSiteID())
	d.SetSiteDefinition(&SiteDefinition{ID: "custom"})
	assert.Equal(t, "custom", d.getSiteID())
}

func TestHDDolbyDriver_ParseDownload(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	data, err := d.ParseDownload(HDDolbyResponse{RawBody: []byte("torrentdata")})
	require.NoError(t, err)
	assert.Equal(t, []byte("torrentdata"), data)

	_, err = d.ParseDownload(HDDolbyResponse{})
	assert.Error(t, err)
}

func TestHDDolbyDriver_PrepareDownload(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	req, err := d.PrepareDownload("42")
	require.NoError(t, err)
	assert.Contains(t, req.Endpoint, "id=42")
}

func TestHDDolbyDriver_torrentToItem(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	item := d.torrentToItem(HDDolbyTorrent{
		ID: 5, Name: "T", SmallDescr: "d", Size: 100, Seeders: 3,
		PromotionTimeType: 1, PromotionUntil: "2024-02-15 10:30:00", HR: 1,
	})
	assert.Equal(t, "5", item.ID)
	assert.Equal(t, DiscountFree, item.DiscountLevel)
	assert.True(t, item.HasHR)
	assert.False(t, item.DiscountEndTime.IsZero())
}

func TestHDDolbyDriver_Execute_HTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.Header.Get("x-api-key"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":200,"data":[{"id":1,"name":"X"}]}`))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIURL: server.URL, APIKey: "test-key"})
	res, err := d.Execute(context.Background(), HDDolbyRequest{Endpoint: "/api/v1/torrent/search", Method: http.MethodPost, Body: map[string]any{"keyword": ""}})
	require.NoError(t, err)
	assert.Equal(t, 200, res.Status)
}

func TestHDDolbyDriver_Execute_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"code":"FORBIDDEN","message":"denied"}}`))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIURL: server.URL, APIKey: "k"})
	_, err := d.Execute(context.Background(), HDDolbyRequest{Endpoint: "/x", Method: http.MethodGet})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FORBIDDEN")
}

func TestHDDolbyDriver_Execute_Cloudflare(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`<html>Just a moment...</html>`))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIURL: server.URL, APIKey: "k"})
	_, err := d.Execute(context.Background(), HDDolbyRequest{Endpoint: "/x", Method: http.MethodGet})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cloudflare")
}

func TestHDDolbyDriver_Search_HTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":200,"data":{"data":[{"id":1,"name":"Movie","category":401}],"total":1}}`))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIURL: server.URL, APIKey: "k"})
	items, err := d.Search(context.Background(), SearchQuery{Keyword: "movie"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "1", items[0].ID)
}

func TestHDDolbyDriver_GetUserInfo_HTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/user/data"):
			w.Write([]byte(`{"status":200,"data":[{"id":"1","username":"u","class":"1","uploaded":"1000","downloaded":"500"}]}`))
		case strings.Contains(r.URL.Path, "/user/peers"):
			w.Write([]byte(`{"status":200,"data":[{"id":1,"size":2048,"seeders":1,"leechers":0}]}`))
		default:
			w.Write([]byte(`{"status":200,"data":[]}`))
		}
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIURL: server.URL, APIKey: "k"})
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "u", info.Username)
	assert.Equal(t, int64(1000), info.Uploaded)
	assert.Equal(t, 1, info.SeederCount)
	assert.Equal(t, int64(2048), info.SeederSize)
}

func TestHDDolbyDriver_GetBonusPerHour(t *testing.T) {
	// No cookie -> 0
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	bph, err := d.GetBonusPerHour(context.Background())
	require.NoError(t, err)
	assert.Equal(t, float64(0), bph)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<table><tr><td>合计</td><td colspan="5">-</td><td>460</td><td>25.98 / 25.98</td></tr></table>`))
	}))
	defer server.Close()

	d2 := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIURL: server.URL, Cookie: "uid=1"})
	bph2, err := d2.GetBonusPerHour(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 25.98, bph2, 0.01)
}

func TestHDDolbyDriver_GetTorrentDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":200,"data":{"data":[{"id":42,"name":"Found","promotion_time_type":1,"size":500}],"total":1}}`))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIURL: server.URL, APIKey: "k"})

	item, err := d.GetTorrentDetail(context.Background(), "42", "", "Found")
	require.NoError(t, err)
	assert.Equal(t, "42", item.ID)
	assert.Equal(t, "Found", item.Title)
	assert.Equal(t, DiscountFree, item.DiscountLevel)

	// Not found -> returns default item, no error
	item2, err := d.GetTorrentDetail(context.Background(), "999", "", "")
	require.NoError(t, err)
	assert.Equal(t, "999", item2.ID)
	assert.Equal(t, DiscountNone, item2.DiscountLevel)
}

func TestHDDolbyDriver_GetTorrentDetail_InvalidID(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	_, err := d.GetTorrentDetail(context.Background(), "", "", "")
	assert.Error(t, err)
	_, err = d.GetTorrentDetail(context.Background(), "notanumber", "", "")
	assert.Error(t, err)
}

func TestHDDolbyDriver_Download_HTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("d4:infod6:lengthi1eee"))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIURL: server.URL, APIKey: "k"})
	data, err := d.DownloadWithHash(context.Background(), "1", "hash123")
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	data2, err := d.Download(context.Background(), "1")
	require.NoError(t, err)
	assert.NotEmpty(t, data2)
}

func TestHDDolbyDriver_InvalidateDetailCache(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	d.detailCache = []HDDolbyTorrent{{ID: 1}}
	d.invalidateDetailCache()
	assert.Nil(t, d.detailCache)
}
