package definitions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "rousipro",
		Search:   testRousiProSearch,
		Detail:   testRousiProDetail,
		UserInfo: testRousiProUserInfo,
	})
}

// --- Fixtures ---

const rousiSearchFixtureJSON = `{
    "code": 0,
    "message": "success",
    "data": {
        "torrents": [
            {
                "id": 12345,
                "uuid": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
                "title": "[Test] Example.Movie.2025.BluRay.1080p.DTS-HD.MA.5.1.x264",
                "subtitle": "测试电影 / Test Movie / 2025",
                "size": 42949672960,
                "seeders": 150,
                "leechers": 10,
                "downloads": 500,
                "created_at": "2025-01-15T08:30:00+08:00",
                "uploader": "testuser",
                "category_name": "Movies",
                "promotion": {
                    "type": 2,
                    "is_active": true
                }
            },
            {
                "id": 12346,
                "uuid": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
                "title": "[Test] Another.Show.S01E01.WEB-DL.1080p.H.264.AAC",
                "subtitle": "测试剧集 / Test Show / S01E01",
                "size": 2147483648,
                "seeders": 50,
                "leechers": 5,
                "downloads": 200,
                "created_at": "2025-01-14T20:00:00+08:00",
                "uploader": "anotheruser",
                "category_name": "TV",
                "promotion": null
            }
        ],
        "total": 2
    }
}`

const rousiUserInfoFixtureJSON = `{
    "code": 0,
    "message": "success",
    "data": {
        "id": 9876,
        "username": "testuser",
        "level": 3,
        "level_text": "Power User",
        "uploaded": 1099511627776,
        "downloaded": 107374182400,
        "ratio": 10.24,
        "karma": 50000.5,
        "credits": 12345.67,
        "seeding_karma_per_hour": 2.5,
        "seeding_points_per_hour": 1.8,
        "seeding_time": 8640000,
        "registered_at": "2024-03-15T12:00:00+08:00",
        "last_active_at": "2025-01-15T09:00:00+08:00",
        "seeding_leeching_data": {
            "seeding_count": 120,
            "seeding_size": 5497558138880
        }
    }
}`

// rousiDetailFixtureJSON uses string "uploader" to match rousiTorrent struct.
// The real detail API returns uploader as an object — that's a known issue in rousiTorrent.
const rousiDetailFixtureJSON = `{
    "code": 0,
    "message": "success",
    "data": {
        "anonymous": true,
        "attributes": {
            "douban": "https://movie.douban.com/subject/4830483/",
            "douban_id": "4830483",
            "genre": [
                "动作",
                "科幻"
            ],
            "imdb": "https://www.imdb.com/title/tt1634106/",
            "imdb_id": "tt1634106",
            "region": "大陆",
            "resolution": "4K / 2160p",
            "source": "UHD Blu-ray"
        },
        "created_at": "2026-02-05T22:05:58.532+08:00",
        "description": "◎译　　名　喋血战士 ...",
        "downloads": 10,
        "files": [],
        "group_id": 2654,
        "id": 4343,
        "info_hash": "xxxxx",
        "leechers": 1,
        "media_info": "DISC ... 0.256 kbps",
        "other_versions": null,
        "poster": "https://webp.rousi.pro/uploads/images/06/068b6f6f-4ee8-4cc7-91a6-19a8f33995c1.png",
        "price": 0,
        "promotion": {
            "color": "#2de615ff",
            "down_multiplier": 0,
            "is_active": true,
            "is_global": false,
            "text": "免费",
            "time_type": 2,
            "type": 2,
            "until": "2026-02-10 17:56:01",
            "up_multiplier": 1
        },
        "seeders": 3,
        "size": 52644631227,
        "status": "approved",
        "sticky": false,
        "sticky_until": null,
        "subtitle": "喋血战士|范·迪塞尔 主演|4K UHD原盘无内封中字|HPB原盘补足计划*030",
        "tags": [],
        "title": "Bloodshot 2020 2160p UHD Blu-ray HDR HEVC TrueHD 7.1 Atmos-BeyondHD",
        "type": "movie",
        "updated_at": "2026-02-07T17:56:01.637+08:00",
        "uploaded_by": null,
        "uploader": null,
        "uuid": "f523e931-a139-4734-b658-0f49c2d6bde1",
        "views": 15
    }
}`

// --- Helpers ---

func newTestRousiDriver() *rousiDriver {
	return newRousiDriver(rousiDriverConfig{
		BaseURL: "https://rousi.pro",
		Passkey: "FAKE_TEST_PASSKEY_1234",
	})
}

// --- Suite: Search ---

func testRousiProSearch(t *testing.T) {
	resp := DecodeFixtureJSON[rousiResponse](t, "rousi_search", rousiSearchFixtureJSON)
	driver := newTestRousiDriver()

	items, err := driver.ParseSearch(resp)
	require.NoError(t, err)
	require.Len(t, items, 2)

	free := items[0]
	assert.Equal(t, "[Test] Example.Movie.2025.BluRay.1080p.DTS-HD.MA.5.1.x264", free.Title)
	assert.Equal(t, int64(42949672960), free.SizeBytes)
	assert.Equal(t, 150, free.Seeders)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.Equal(t, "a1b2c3d4-e5f6-7890-abcd-ef1234567890", free.ID)
	assert.Equal(t, "rousipro", free.SourceSite)

	normal := items[1]
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
	assert.Equal(t, "rousipro", normal.SourceSite)

	t.Run("EmptyResult", func(t *testing.T) {
		raw := `{"code":0,"message":"success","data":{"torrents":[],"total":0}}`
		resp := DecodeFixtureJSON[rousiResponse](t, "rousi_empty", raw)
		items, err := driver.ParseSearch(resp)
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("Promotions", func(t *testing.T) {
		tests := []struct {
			promoType int
			expected  v2.DiscountLevel
		}{
			{2, v2.DiscountFree},
			{3, v2.Discount2xUp},
			{4, v2.Discount2xFree},
			{5, v2.DiscountPercent50},
			{6, v2.Discount2x50},
			{7, v2.DiscountPercent30},
			{0, v2.DiscountNone},
		}
		for _, tt := range tests {
			t.Run(fmt.Sprintf("type_%d", tt.promoType), func(t *testing.T) {
				raw := fmt.Sprintf(`{"code":0,"message":"success","data":{"torrents":[{"id":1,"uuid":"uuid-%d","title":"promo","subtitle":"","size":1,"seeders":1,"leechers":0,"downloads":0,"created_at":"2025-01-15T08:30:00+08:00","uploader":"tester","category_name":"Movies","promotion":{"type":%d,"is_active":true}}],"total":1}}`, tt.promoType, tt.promoType)
				resp := DecodeFixtureJSON[rousiResponse](t, "rousi_promo", raw)
				items, err := driver.ParseSearch(resp)
				require.NoError(t, err)
				require.Len(t, items, 1)
				assert.Equal(t, tt.expected, items[0].DiscountLevel)
			})
		}
	})
}

// --- Suite: Detail ---

func testRousiProDetail(t *testing.T) {
	var result struct {
		Code    int          `json:"code"`
		Message string       `json:"message"`
		Data    rousiTorrent `json:"data"`
	}
	RequireNoSecrets(t, "rousi_detail", rousiDetailFixtureJSON)
	require.NoError(t, json.Unmarshal([]byte(rousiDetailFixtureJSON), &result))
	require.Equal(t, 0, result.Code)

	data := result.Data
	assert.Equal(t, "f523e931-a139-4734-b658-0f49c2d6bde1", data.UUID)
	assert.Equal(t, "Bloodshot 2020 2160p UHD Blu-ray HDR HEVC TrueHD 7.1 Atmos-BeyondHD", data.Title)
	assert.Equal(t, "喋血战士|范·迪塞尔 主演|4K UHD原盘无内封中字|HPB原盘补足计划*030", data.Subtitle)
	assert.Equal(t, int64(52644631227), data.Size)
	assert.Equal(t, 3, data.Seeders)
	assert.Equal(t, 1, data.Leechers)
	assert.Equal(t, 10, data.Downloads)

	driver := newTestRousiDriver()
	discount, _ := driver.parsePromotion(data.Promotion)
	assert.Equal(t, v2.DiscountFree, discount, "promotion type 2 should be Free")
}

// --- Suite: UserInfo ---

func testRousiProUserInfo(t *testing.T) {
	resp := DecodeFixtureJSON[rousiResponse](t, "rousi_userinfo", rousiUserInfoFixtureJSON)

	var userData rousiUserData
	require.NoError(t, json.Unmarshal(resp.Data, &userData))

	assert.Equal(t, "testuser", userData.Username)
	assert.Equal(t, int64(1099511627776), userData.Uploaded)
	assert.Equal(t, 10.24, userData.Ratio)
	assert.Equal(t, 50000.5, userData.Karma)
	assert.Equal(t, 3, userData.Level)
	assert.Equal(t, "Power User", userData.LevelText)
	require.NotNil(t, userData.SeedingLeechingData)
	assert.Equal(t, 120, userData.SeedingLeechingData.SeedingCount)
	assert.Equal(t, int64(5497558138880), userData.SeedingLeechingData.SeedingSize)
}

// --- Standalone Tests (edge cases beyond suite scope) ---

func TestRousiPro_CreateDriver_Smoke(t *testing.T) {
	config := v2.SiteConfig{
		ID:      "rousipro",
		Name:    "Rousi Pro",
		BaseURL: "https://rousi.pro",
		Options: json.RawMessage(`{"passkey":"FAKE_TEST_PASSKEY_1234"}`),
	}
	site, err := createRousiDriver(config, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, site)
	assert.Equal(t, "rousipro", site.ID())
	assert.Equal(t, "Rousi Pro", site.Name())
}

func TestRousiPro_CreateDriver_MissingPasskey(t *testing.T) {
	config := v2.SiteConfig{
		ID:      "rousipro",
		Name:    "Rousi Pro",
		BaseURL: "https://rousi.pro",
		Options: json.RawMessage(`{"passkey":""}`),
	}
	site, err := createRousiDriver(config, zap.NewNop())
	require.Error(t, err)
	assert.Nil(t, site)
	assert.ErrorContains(t, err, "Passkey")
}

func TestRousiPro_Fixtures_NoSecrets(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"search", rousiSearchFixtureJSON},
		{"user", rousiUserInfoFixtureJSON},
		{"detail", rousiDetailFixtureJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RequireNoSecrets(t, tt.name, tt.raw)
		})
	}
}

// TestCreateRousiDriver_Errors covers createRousiDriver error/success branches.
func TestCreateRousiDriver_MissingPasskey(t *testing.T) {
	_, err := createRousiDriver(v2.SiteConfig{ID: "rousipro", Name: "RousiPro"}, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Passkey")
}

func TestCreateRousiDriver_BadOptions(t *testing.T) {
	_, err := createRousiDriver(v2.SiteConfig{
		ID:      "rousipro",
		Options: json.RawMessage(`not json`),
	}, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse Rousi options")
}

func TestCreateRousiDriver_Success(t *testing.T) {
	opts, _ := json.Marshal(v2.RousiOptions{Passkey: "FAKE_PK", Cookie: "c=1"})
	site, err := createRousiDriver(v2.SiteConfig{
		ID:      "rousipro",
		Name:    "RousiPro",
		BaseURL: "https://rousi.pro",
		Options: opts,
	}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, site)
	assert.Equal(t, "rousipro", site.ID())
}

// TestCreateRousiDriver_BaseURLFromDefinition exercises the fallback to definition URLs.
func TestCreateRousiDriver_BaseURLFromDefinition(t *testing.T) {
	opts, _ := json.Marshal(v2.RousiOptions{Passkey: "FAKE_PK"})
	site, err := createRousiDriver(v2.SiteConfig{
		ID:      "rousipro",
		Name:    "RousiPro",
		Options: opts,
		// BaseURL intentionally empty -> should use definition URLs
	}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, site)
}

// TestRousiDriver_ParseSearch_Full covers ParseSearch with items + promotion + times.
func TestRousiDriver_ParseSearch_Full(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")

	body := `{"code":0,"message":"success","data":{"torrents":[
		{"id":1,"uuid":"u1","title":"Movie A","subtitle":"sub","size":1024,"seeders":3,"leechers":1,"downloads":9,
		 "created_at":"2025-01-15T08:30:00+08:00","category_name":"Movies",
		 "promotion":{"type":2,"is_active":true,"until":"2025-02-01T00:00:00+08:00"}},
		{"id":2,"uuid":"u2","title":"Movie B","size":2048,"seeders":5,"leechers":0,"downloads":2,
		 "created_at":"2025-01-16 09:00:00"}
	],"total":2}}`

	_ = d
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	d2 := newTestRousiDriverWithURL(server.URL)
	resp, err := d2.Execute(context.Background(), rousiRequest{Endpoint: "/api/v1/torrents"})
	require.NoError(t, err)
	items, err := d2.ParseSearch(resp)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "u1", items[0].ID)
	assert.Equal(t, "Movie A", items[0].Title)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Greater(t, items[0].UploadedAt, int64(0))
	// second item uses the "2006-01-02 15:04:05" CST fallback path
	assert.Greater(t, items[1].UploadedAt, int64(0))
	assert.Equal(t, v2.DiscountNone, items[1].DiscountLevel)
}

func TestRousiDriver_ParseSearch_APIError(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")
	_, err := d.ParseSearch(rousiResponse{Code: 1, Message: "denied"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "denied")
}

func TestRousiDriver_ParseSearch_BadData(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")
	_, err := d.ParseSearch(rousiResponse{Data: json.RawMessage(`not json`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse search data")
}

// TestRousiDriver_ParsePromotion_Types covers the various promotion type mappings.
func TestRousiDriver_ParsePromotion_Types(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")

	cases := []struct {
		promo *rousiPromotion
		want  v2.DiscountLevel
	}{
		{nil, v2.DiscountNone},
		{&rousiPromotion{IsActive: false, Type: 2}, v2.DiscountNone},
		{&rousiPromotion{IsActive: true, Type: 0}, v2.DiscountNone},
		{&rousiPromotion{IsActive: true, Type: 3}, v2.Discount2xUp},
		{&rousiPromotion{IsActive: true, Type: 4}, v2.Discount2xFree},
		{&rousiPromotion{IsActive: true, Type: 5}, v2.DiscountPercent50},
		{&rousiPromotion{IsActive: true, Type: 6}, v2.Discount2x50},
		{&rousiPromotion{IsActive: true, Type: 7}, v2.DiscountPercent30},
		{&rousiPromotion{IsActive: true, Type: 99, DownMultiplier: 0, UpMultiplier: 2}, v2.Discount2xFree},
		{&rousiPromotion{IsActive: true, Type: 99, DownMultiplier: 0, UpMultiplier: 1}, v2.DiscountFree},
		{&rousiPromotion{IsActive: true, Type: 99, DownMultiplier: 1, UpMultiplier: 1}, v2.DiscountNone},
	}
	for _, c := range cases {
		got, _ := d.parsePromotion(c.promo)
		assert.Equal(t, c.want, got)
	}
}

// TestRousiDriver_ParsePromotion_UntilLayouts exercises the different time layouts.
func TestRousiDriver_ParsePromotion_UntilLayouts(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")

	_, end := d.parsePromotion(&rousiPromotion{IsActive: true, Type: 2, Until: "2025-02-01T00:00:00+08:00"})
	assert.False(t, end.IsZero())

	_, end2 := d.parsePromotion(&rousiPromotion{IsActive: true, Type: 2, Until: "2025-02-01 00:00:00"})
	assert.False(t, end2.IsZero())

	_, end3 := d.parsePromotion(&rousiPromotion{IsActive: true, Type: 2, Until: "garbage"})
	assert.True(t, end3.IsZero())
}

// TestRousiDriver_GetUserInfo_Full covers the full user data parse with seeding data + times.
func TestRousiDriver_GetUserInfo_Full(t *testing.T) {
	body := `{"code":0,"message":"success","data":{
		"id":9876,"username":"tester","level":2,"level_text":"Power User",
		"uploaded":1099511627776,"downloaded":107374182,"ratio":10.24,
		"karma":5000.5,"credits":123.4,"seeding_karma_per_hour":1.5,"seeding_points_per_hour":2.5,
		"registered_at":"2020-01-01T00:00:00+08:00","last_active_at":"2024-06-01T12:00:00+08:00",
		"seeding_leeching_data":{"seeding_count":120,"seeding_size":5497558138880}
	}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "9876", info.UserID)
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, int64(1099511627776), info.Uploaded)
	assert.Equal(t, "Power User", info.LevelName)
	assert.Equal(t, 120, info.SeederCount)
	assert.Equal(t, int64(5497558138880), info.SeederSize)
	assert.Greater(t, info.JoinDate, int64(0))
	assert.Greater(t, info.LastAccess, int64(0))
	assert.InDelta(t, 5000.5, info.Bonus, 0.01)
}

// TestRousiDriver_GetUserInfo_AltTimeLayout covers the "-0700" layout fallback.
func TestRousiDriver_GetUserInfo_AltTimeLayout(t *testing.T) {
	body := `{"code":0,"message":"success","data":{
		"id":1,"username":"t","registered_at":"2020-01-01T00:00:00+0800","last_active_at":"2024-06-01T12:00:00+0800"
	}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Greater(t, info.JoinDate, int64(0))
	assert.Greater(t, info.LastAccess, int64(0))
}

func TestRousiDriver_GetUserInfo_BadData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":"not-an-object"}`))
	}))
	defer server.Close()

	d := newTestRousiDriverWithURL(server.URL)
	_, err := d.GetUserInfo(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse user data")
}

func TestRousiDriver_GetUserInfo_ExecuteError(t *testing.T) {
	d := newTestRousiDriverWithURL("http://127.0.0.1:1") // connection refused
	_, err := d.GetUserInfo(context.Background())
	require.Error(t, err)
}

// TestRousiDriver_GetSiteID covers both branches of getSiteID.
func TestRousiDriver_GetSiteID(t *testing.T) {
	d := newTestRousiDriverWithURL("https://rousi.pro")
	assert.Equal(t, "rousipro", d.getSiteID())

	def, ok := v2.GetDefinitionRegistry().Get("rousipro")
	require.True(t, ok)
	d.siteDefinition = def
	assert.Equal(t, def.ID, d.getSiteID())
}

// TestExtractUUIDFromLink_Extra covers additional link shapes.
func TestExtractUUIDFromLink_Extra(t *testing.T) {
	assert.Equal(t, "xyz", extractUUIDFromLink("xyz"))
	assert.Equal(t, "last", extractUUIDFromLink("a/b/c/last"))
}

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
