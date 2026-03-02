package definitions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "mooko",
		Search:   testMooKoSearch,
		Detail:   testMooKoDetail,
		UserInfo: testMooKoUserInfo,
	})
}

const mookoSearchFixture = `{
    "status": "success",
    "response": {
        "currentPage": 1,
        "pages": 1,
        "results": [
            {
                "groupId": 4830,
                "groupName": "美丽心灵",
                "artist": "",
                "tags": ["剧情", "悬疑", "传记"],
                "torrents": [
                    {
                        "torrentId": 5001,
                        "media": "Blu-ray",
                        "format": "FLAC",
                        "encoding": "Lossless",
                        "remastered": false,
                        "fileCount": 5,
                        "time": "2025-01-15 08:30:00",
                        "size": 45097156608,
                        "snatches": 150,
                        "seeders": 42,
                        "leechers": 3,
                        "isFreeleech": true,
                        "isNeutralLeech": false,
                        "isPersonalFreeleech": false,
                        "canUseToken": true
                    },
                    {
                        "torrentId": 5002,
                        "media": "WEB",
                        "format": "AAC",
                        "encoding": "320",
                        "remastered": false,
                        "fileCount": 1,
                        "time": "2025-01-14 20:00:00",
                        "size": 2147483648,
                        "snatches": 50,
                        "seeders": 10,
                        "leechers": 1,
                        "isFreeleech": false,
                        "isNeutralLeech": false,
                        "isPersonalFreeleech": false,
                        "canUseToken": true
                    }
                ]
            }
        ]
    }
}`

const mookoUserInfoFixture = `{
    "status": "success",
    "response": {
        "username": "TestUser",
        "id": 1908,
        "stats": {
            "uploaded": 56925179188162,
            "downloaded": 15020725171164,
            "ratio": 3.79,
            "buffer": 41904454016998
        },
        "ranks": {
            "class": "Elite"
        },
        "personal": {
            "bonus": 8287681.5
        },
        "community": {
            "seeding": 49,
            "leeching": 0
        }
    }
}`

func testMooKoSearch(t *testing.T) {
	def, ok := v2.GetDefinitionRegistry().Get("mooko")
	require.True(t, ok)
	_ = def

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mookoSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewGazelleDriver(v2.GazelleDriverConfig{
		BaseURL: server.URL,
		Cookie:  "test_cookie=1",
	})

	req, err := driver.PrepareSearch(v2.SearchQuery{Keyword: "test"})
	require.NoError(t, err)

	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	free := items[0]
	assert.Equal(t, "5001", free.ID)
	assert.Contains(t, free.Title, "美丽心灵")
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.Equal(t, int64(45097156608), free.SizeBytes)
	assert.Equal(t, 42, free.Seeders)
	assert.Equal(t, 3, free.Leechers)

	normal := items[1]
	assert.Equal(t, "5002", normal.ID)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
}

func testMooKoDetail(t *testing.T) {
	def, ok := v2.GetDefinitionRegistry().Get("mooko")
	require.True(t, ok)
	_ = def

	driver := v2.NewGazelleDriver(v2.GazelleDriverConfig{
		BaseURL: "https://mooko.org",
		Cookie:  "test_cookie=1",
	})

	req, err := driver.PrepareDownload("5001")
	require.NoError(t, err)
	assert.Equal(t, "download", req.Action)
	assert.Equal(t, "5001", req.Params.Get("id"))
}

func testMooKoUserInfo(t *testing.T) {
	def, ok := v2.GetDefinitionRegistry().Get("mooko")
	require.True(t, ok)
	_ = def

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mookoUserInfoFixture))
	}))
	defer server.Close()

	driver := v2.NewGazelleDriver(v2.GazelleDriverConfig{
		BaseURL: server.URL,
		Cookie:  "test_cookie=1",
	})

	info, err := driver.GetUserInfo(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "1908", info.UserID)
	assert.Equal(t, "TestUser", info.Username)
	assert.Equal(t, int64(56925179188162), info.Uploaded)
	assert.Equal(t, int64(15020725171164), info.Downloaded)
	assert.InDelta(t, 3.79, info.Ratio, 0.01)
	assert.InDelta(t, 8287681.5, info.Bonus, 0.1)
	assert.Equal(t, 49, info.Seeding)
	assert.Equal(t, 0, info.Leeching)
	assert.Equal(t, "Elite", info.Rank)
}

func TestMooKo_Fixtures_NoSecrets(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "search", raw: mookoSearchFixture},
		{name: "user", raw: mookoUserInfoFixture},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RequireNoSecrets(t, tt.name, tt.raw)
		})
	}
}
