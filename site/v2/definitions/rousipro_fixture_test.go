package definitions

import (
	"encoding/json"
	"fmt"
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
