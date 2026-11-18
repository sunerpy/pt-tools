package definitions

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "hddolby",
		Search:   testHDDolbySearch,
		Detail:   testHDDolbyDetail,
		UserInfo: testHDDolbyUserInfo,
	})
}

// --- Fixtures ---

// 字段照 2026-11-02 www.uhddolby.com 的 /api/v1/torrent/search 返回，数值、名字与 downhash 都是示例。
// 优惠看 sp_state（NexusPHP 的促销编号），时限看 promotion_time_type；tags 是官方、中字这类标签，不是优惠。
const hddolbySearchFixtureJSON = `{"status":0,"data":[
{"id":900101,"sp_state":2,"promotion_time_type":2,"promotion_until":"2026-11-04 14:36:32","category":402,"medium":10,"codec":1,
 "standard":5,"team":10,"audiocodec":14,"leechers":3,"seeders":12,"name":"Example.Show.S01E01.2026.2160p.WEB-DL.H265-DBTV",
 "small_descr":"示例剧集 第1集","times_completed":40,"size":9734472868,"added":"2026-11-02 14:36:32","url":0,"hr":1,
 "tmdb_type":"tv","tmdb_id":1,"imdb_id":null,"tags":"gf;zz;lz","info_hash":"0123456789ABCDEF0123456789ABCDEF01234567","downhash":"example-downhash-1"},
{"id":900102,"sp_state":1,"promotion_time_type":0,"promotion_until":"0000-00-00 00:00:00","category":401,"leechers":0,"seeders":5,
 "name":"Example.Movie.2026.1080p.BluRay.x264","small_descr":"示例电影","times_completed":2,"size":27609205714,
 "added":"2026-11-02 13:29:13","hr":0,"tags":"gf;sf","downhash":"example-downhash-2"},
{"id":900103,"sp_state":4,"promotion_time_type":0,"promotion_until":"0000-00-00 00:00:00","category":404,"leechers":9,"seeders":1,
 "name":"Example.Doc.2026.2160p.UHD.BluRay","small_descr":"","times_completed":0,"size":64496595623,
 "added":"2026-11-02 12:27:28","hr":0,"tags":"zz;db","downhash":"example-downhash-3"}
]}`

const hddolbyUserInfoFixtureJSON = `{"status":0,"data":[{"id":"9876","username":"TestDolbyUser","added":"2024-03-15 12:00:00",` +
	`"last_access":"2026-11-02 09:00:00","class":"7","uploaded":"1099511627776","downloaded":"107374182400",` +
	`"seedbonus":"50000.5","sebonus":"12345.67","unread_messages":"3"}]}`

const hddolbyPeersFixtureJSON = `{"status":0,"data":[{"id":900101,"size":9734472868,"seeders":12,"leechers":3},` +
	`{"id":900102,"size":27609205714,"seeders":5,"leechers":0}]}`

// --- Helpers ---

func getHDDolbyDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("hddolby")
	require.True(t, ok, "hddolby definition not found")
	return def
}

// newHDDolbyFixtureServer 模拟站点的 API：只回答带了 API Key、User-Agent 不是浏览器的请求（站点对浏览器回「Browser access is blocked!」）
func newHDDolbyFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("x-api-key") != "FAKE_TEST_KEY_12345" {
			_, _ = w.Write([]byte(`{"status":30001,"error":{"message":"Illegal api key","details":""}}`))
			return
		}
		assert.NotContains(t, r.Header.Get("User-Agent"), "Mozilla")
		switch r.URL.Path {
		case "/api/v1/torrent/search":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			assert.NoError(t, json.Unmarshal(raw, &body))
			assert.EqualValues(t, 0, body["page_number"], "page_number 是跳过的条数")
			_, _ = w.Write([]byte(hddolbySearchFixtureJSON))
		case "/api/v1/user/data":
			_, _ = w.Write([]byte(hddolbyUserInfoFixtureJSON))
		case "/api/v1/user/peers":
			_, _ = w.Write([]byte(hddolbyPeersFixtureJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func newTestHDDolbyDriver(def *v2.SiteDefinition, baseURL string) *v2.HDDolbyDriver {
	driver := v2.NewHDDolbyDriver(v2.HDDolbyDriverConfig{
		BaseURL: baseURL,
		APIKey:  "FAKE_TEST_KEY_12345",
	})
	driver.SetSiteDefinition(def)
	return driver
}

// --- Suite: Search ---

func testHDDolbySearch(t *testing.T) {
	def := getHDDolbyDef(t)
	RequireNoSecrets(t, "hddolby_search", hddolbySearchFixtureJSON)
	driver := newTestHDDolbyDriver(def, newHDDolbyFixtureServer(t).URL)

	items, err := driver.Search(context.Background(), v2.SearchQuery{Keyword: "Example"})
	require.NoError(t, err)
	require.Len(t, items, 3)
	now := time.Date(2026, 11, 2, 15, 0, 0, 0, v2.CSTLocation)

	free := items[0]
	assert.Equal(t, "900101", free.ID)
	assert.Equal(t, "Example.Show.S01E01.2026.2160p.WEB-DL.H265-DBTV", free.Title)
	assert.Equal(t, "示例剧集 第1集", free.Subtitle)
	assert.Equal(t, int64(9734472868), free.SizeBytes)
	assert.Equal(t, 12, free.Seeders)
	assert.Equal(t, 3, free.Leechers)
	assert.Equal(t, 40, free.Snatched)
	assert.Equal(t, "电视剧", free.Category)
	assert.Equal(t, []string{"官方", "中字", "连载"}, free.Tags)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.True(t, free.DiscountEndTime.Equal(time.Date(2026, 11, 4, 14, 36, 32, 0, v2.CSTLocation)))
	assert.Equal(t, v2.FreeUntil, free.Promotion(now).Validity)
	assert.True(t, free.HasHR)

	normal := items[1]
	assert.Equal(t, "900102", normal.ID)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel, "官方、首发是标签，不是优惠")
	assert.True(t, normal.DiscountEndTime.IsZero())
	assert.False(t, normal.HasHR)
	assert.Equal(t, "电影", normal.Category)

	// 跟站点默认的 2X 免费：发布以后 24 小时结束
	twoXFree := items[2]
	assert.Equal(t, v2.Discount2xFree, twoXFree.DiscountLevel)
	assert.True(t, twoXFree.DiscountEndTime.Equal(time.Date(2026, 11, 3, 12, 27, 28, 0, v2.CSTLocation)))
	assert.Equal(t, v2.FreeUntil, twoXFree.Promotion(now).Validity)
}

// --- Suite: Detail ---

// 详情走 API：种子详情从最近一次搜索结果里取（站点没有按 ID 查一个种子的接口）
func testHDDolbyDetail(t *testing.T) {
	def := getHDDolbyDef(t)
	driver := newTestHDDolbyDriver(def, newHDDolbyFixtureServer(t).URL)

	info, err := driver.GetTorrentDetail(context.Background(), "900101", "", "")
	require.NoError(t, err)
	assert.Equal(t, "900101", info.ID)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.True(t, info.DiscountEndTime.Equal(time.Date(2026, 11, 4, 14, 36, 32, 0, v2.CSTLocation)))
	assert.True(t, info.HasHR)
	assert.Equal(t, int64(9734472868), info.SizeBytes)
	assert.Contains(t, info.DownloadURL, "downhash=example-downhash-1")

	_, err = driver.GetTorrentDetail(context.Background(), "1", "", "")
	require.ErrorIs(t, err, v2.ErrTorrentDetailUnavailable)
}

// --- Suite: UserInfo ---

func testHDDolbyUserInfo(t *testing.T) {
	def := getHDDolbyDef(t)
	RequireNoSecrets(t, "hddolby_userinfo", hddolbyUserInfoFixtureJSON)
	driver := newTestHDDolbyDriver(def, newHDDolbyFixtureServer(t).URL)

	// 只配 API Key：用户数据、做种都有，时魔要 Cookie
	info, err := driver.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "9876", info.UserID)
	assert.Equal(t, "TestDolbyUser", info.Username)
	assert.Equal(t, int64(1099511627776), info.Uploaded)
	assert.Equal(t, int64(107374182400), info.Downloaded)
	assert.InDelta(t, 10.24, info.Ratio, 0.0001)
	assert.InDelta(t, 50000.5, info.Bonus, 0.0001)
	assert.InDelta(t, 12345.67, info.SeedingBonus, 0.0001)
	assert.Equal(t, "Extreme User", info.LevelName)
	assert.Equal(t, 3, info.UnreadMessageCount)
	assert.Equal(t, 2, info.SeederCount)
	assert.Equal(t, int64(9734472868+27609205714), info.SeederSize)
	assert.Equal(t, time.Date(2024, 3, 15, 12, 0, 0, 0, v2.CSTLocation).Unix(), info.JoinDate)
	assert.Equal(t, time.Date(2026, 11, 2, 9, 0, 0, 0, v2.CSTLocation).Unix(), info.LastAccess)
	assert.Zero(t, info.BonusPerHour)

	// RSS key 过期：站点回 30001，算凭据无效
	expired := v2.NewHDDolbyDriver(v2.HDDolbyDriverConfig{BaseURL: newHDDolbyFixtureServer(t).URL, APIKey: "FAKE_EXPIRED_KEY"})
	expired.SetSiteDefinition(def)
	_, err = expired.GetUserInfo(context.Background())
	require.ErrorIs(t, err, v2.ErrInvalidCredentials)
}

// --- Standalone Tests (edge cases beyond suite scope) ---

func TestHDDolby_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":   hddolbySearchFixtureJSON,
		"userinfo": hddolbyUserInfoFixtureJSON,
		"peers":    hddolbyPeersFixtureJSON,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}

// 定义：新域名、旧域名都认（下载器里的旧 tracker 还是 t.hddolby.com），签到要 Cookie，H&R 48 小时
func TestHDDolby_Definition(t *testing.T) {
	def := getHDDolbyDef(t)
	assert.Equal(t, []string{"https://www.uhddolby.com/"}, def.URLs)
	assert.Contains(t, def.LegacyURLs, "https://www.hddolby.com/")
	assert.Equal(t, v2.AuthMethodCookieAndAPIKey, def.AuthMethod)
	assert.NotNil(t, def.Attendance)
	assert.Equal(t, 48, def.HRSeedTimeHours)
	assert.Equal(t, 50, def.RateWindowLimit)

	r := v2.NewTrackerResolver()
	for _, tracker := range []string{
		"https://t.hddolby.com/announce.php?passkey=x", // 下载器里旧种子的 tracker（下载器助手替换以前）
		"https://www.uhddolby.com/announce.php?passkey=x",
	} {
		got, ok := r.Resolve(tracker)
		assert.True(t, ok, tracker)
		assert.Equal(t, "hddolby", got, tracker)
	}
	// 旧域名的下载地址照样认作这个站点（以前存下的 RSS 链接、推送记录）
	id, ok := v2.ParseDownloadURL(def, "https://www.hddolby.com/download.php?id=9&downhash=x")
	assert.True(t, ok)
	assert.Equal(t, "9", id)
}
