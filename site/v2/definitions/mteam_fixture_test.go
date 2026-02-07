package definitions

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "mteam",
		Search:   testMTeamSearch,
		Detail:   testMTeamDetail,
		UserInfo: testMTeamUserInfo,
	})
}

// --- Fixtures ---

const mteamSearchFixtureJSON = `{
    "data": [
        {
            "id": "12345",
            "name": "Test.Movie.2024.BluRay.1080p",
            "smallDescr": "测试电影",
            "size": "45097156608",
            "createdDate": "2025-01-15 10:30:00",
            "status": {
                "seeders": 100,
                "leechers": 5,
                "timesCompleted": 300,
                "discount": "FREE",
                "discountEndTime": "2026-03-01 12:00:00"
            },
            "category": "402"
        },
        {
            "id": "12346",
            "name": "Another.Show.S01E01.WEB-DL",
            "smallDescr": "测试剧集",
            "size": "2147483648",
            "createdDate": "2025-01-14 20:00:00",
            "status": {
                "seeders": 50,
                "leechers": 3,
                "timesCompleted": 200,
                "discount": "NORMAL"
            },
            "category": "402"
        }
    ],
    "total": 2
}`

const mteamUserInfoFixtureJSON = `{
    "id": "9876",
    "username": "TestMTeamUser",
    "createdDate": "2024-03-15 12:00:00",
    "role": "3",
    "memberCount": {
        "uploaded": "1099511627776",
        "downloaded": "107374182400",
        "bonus": "50000.5",
        "shareRate": "10.24"
    },
    "memberStatus": {
        "lastBrowse": "2025-01-15 09:00:00",
        "vip": false
    }
}`

const mteamDetailFixtureJSON = `{
    "id": "54321",
    "name": "哈利波特与魔法石 4K UHD BluRay",
    "smallDescr": "Harry Potter 4K Remux",
    "size": "85899345920",
    "createdDate": "2025-12-01 08:00:00",
    "status": {
        "seeders": 50,
        "leechers": 2,
        "timesCompleted": 150,
        "discount": "FREE",
        "discountEndTime": "2026-02-28 23:59:59"
    },
    "category": "421"
}`

// --- Helpers ---

func getMTeamDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("mteam")
	require.True(t, ok, "mteam definition not found")
	return def
}

func newTestMTorrentDriver(def *v2.SiteDefinition) *v2.MTorrentDriver {
	driver := v2.NewMTorrentDriver(v2.MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "FAKE_TEST_API_KEY_12345",
	})
	driver.SetSiteDefinition(def)
	return driver
}

// --- Suite: Search ---

func testMTeamSearch(t *testing.T) {
	RequireNoSecrets(t, "mteam_search", mteamSearchFixtureJSON)
	def := getMTeamDef(t)
	resp := v2.MTorrentResponse{
		Code:    "0",
		Message: "success",
		Data:    json.RawMessage(mteamSearchFixtureJSON),
	}

	driver := newTestMTorrentDriver(def)
	items, err := driver.ParseSearch(resp)
	require.NoError(t, err)
	require.Len(t, items, 2)

	free := items[0]
	assert.Equal(t, "12345", free.ID)
	assert.Equal(t, "Test.Movie.2024.BluRay.1080p", free.Title)
	assert.Equal(t, "测试电影", free.Subtitle)
	assert.Equal(t, int64(45097156608), free.SizeBytes)
	assert.Equal(t, 100, free.Seeders)
	assert.Equal(t, 5, free.Leechers)
	assert.Equal(t, 300, free.Snatched)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.Equal(t, "影剧/综艺/HD", free.Category)
	assert.Equal(t, driver.BaseURL, free.SourceSite)
	assert.Equal(t, 2026, free.DiscountEndTime.Year())
	assert.Equal(t, 3, int(free.DiscountEndTime.Month()))
	assert.Equal(t, 1, free.DiscountEndTime.Day())

	normal := items[1]
	assert.Equal(t, "12346", normal.ID)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
	assert.Equal(t, "影剧/综艺/HD", normal.Category)
	assert.Equal(t, driver.BaseURL, normal.SourceSite)

	t.Run("EmptyResult", func(t *testing.T) {
		raw := `{"data":[],"total":0}`
		RequireNoSecrets(t, "mteam_empty", raw)
		resp := v2.MTorrentResponse{
			Code:    "0",
			Message: "success",
			Data:    json.RawMessage(raw),
		}
		items, err := driver.ParseSearch(resp)
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("PromotionOverride", func(t *testing.T) {
		raw := `{
    "data": [
        {
            "id": "99999",
            "name": "Promo.Movie.2025.WEB-DL",
            "smallDescr": "促销测试",
            "size": "1073741824",
            "createdDate": "2025-01-10 11:00:00",
            "status": {
                "seeders": 10,
                "leechers": 1,
                "timesCompleted": 20,
                "discount": "NORMAL",
                "promotionRule": {
                    "discount": "PERCENT_50",
                    "startTime": "2024-01-01 00:00:00",
                    "endTime": "2030-01-01 00:00:00"
                }
            },
            "category": "401"
        }
    ],
    "total": 1
}`
		RequireNoSecrets(t, "mteam_promo", raw)
		resp := v2.MTorrentResponse{
			Code:    "0",
			Message: "success",
			Data:    json.RawMessage(raw),
		}
		items, err := driver.ParseSearch(resp)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, v2.DiscountPercent50, items[0].DiscountLevel)
		assert.Equal(t, 2030, items[0].DiscountEndTime.Year())
	})
}

// --- Suite: Detail ---

func testMTeamDetail(t *testing.T) {
	RequireNoSecrets(t, "mteam_detail", mteamDetailFixtureJSON)

	resp := v2.MTorrentResponse{
		Code:    "0",
		Message: "SUCCESS",
		Data:    json.RawMessage(mteamDetailFixtureJSON),
	}

	var detail v2.MTorrentTorrent
	require.NoError(t, json.Unmarshal(resp.Data, &detail))

	assert.Equal(t, "54321", detail.ID)
	assert.Equal(t, "哈利波特与魔法石 4K UHD BluRay", detail.Name)
	assert.Equal(t, "Harry Potter 4K Remux", detail.SmallDescr)
	assert.Equal(t, 50, detail.Status.Seeders.Int())
	assert.Equal(t, "FREE", detail.Status.Discount)
	assert.Equal(t, "2026-02-28 23:59:59", detail.Status.DiscountEndTime)
	assert.Equal(t, "421", detail.Category)

	sizeBytes, err := strconv.ParseInt(detail.Size, 10, 64)
	require.NoError(t, err)
	assert.Equal(t, int64(85899345920), sizeBytes)
}

// --- Suite: UserInfo ---

func testMTeamUserInfo(t *testing.T) {
	RequireNoSecrets(t, "mteam_userinfo", mteamUserInfoFixtureJSON)
	def := getMTeamDef(t)
	resp := v2.MTorrentResponse{
		Code:    "0",
		Message: "success",
		Data:    json.RawMessage(mteamUserInfoFixtureJSON),
	}

	driver := newTestMTorrentDriver(def)
	info, err := driver.ParseUserInfo(resp)
	require.NoError(t, err)

	assert.Equal(t, "9876", info.UserID)
	assert.Equal(t, "TestMTeamUser", info.Username)
	assert.Equal(t, int64(1099511627776), info.Uploaded)
	assert.Equal(t, int64(107374182400), info.Downloaded)
	assert.Equal(t, 10.24, info.Ratio)
	assert.Equal(t, 50000.5, info.Bonus)
	assert.Equal(t, "Elite User", info.Rank)
	assert.Equal(t, "Elite User", info.LevelName)
	assert.Equal(t, 3, info.LevelID)
	assert.NotZero(t, info.LastUpdate)

	joinTime, err := v2.ParseTimeInCST("2006-01-02 15:04:05", "2024-03-15 12:00:00")
	require.NoError(t, err)
	accessTime, err := v2.ParseTimeInCST("2006-01-02 15:04:05", "2025-01-15 09:00:00")
	require.NoError(t, err)
	assert.Equal(t, joinTime.Unix(), info.JoinDate)
	assert.Equal(t, accessTime.Unix(), info.LastAccess)
}

// --- Standalone Tests (edge cases beyond suite scope) ---

func TestMTeam_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search": mteamSearchFixtureJSON,
		"user":   mteamUserInfoFixtureJSON,
		"detail": mteamDetailFixtureJSON,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
