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
		SiteID:   "hddolby",
		Search:   testHDDolbySearch,
		Detail:   testHDDolbyDetail,
		UserInfo: testHDDolbyUserInfo,
	})
}

// --- Fixtures ---

const hddolbySearchFixtureJSON = `{"data":[{"id":12345,"name":"Test.Movie.2025.BluRay.1080p","small_descr":"测试电影","category":402,"size":45097156608,"seeders":100,"leechers":5,"times_completed":300,"added":"2025-01-15 08:30:00","promotion_time_type":0,"promotion_until":"2025-02-15 12:00:00","tags":"f","downhash":"abc123","hr":1},{"id":12346,"name":"Another.Show.S01E01.WEB-DL","small_descr":"测试剧集","category":408,"size":2147483648,"seeders":50,"leechers":3,"times_completed":200,"added":"2025-01-14 20:00:00","promotion_time_type":0,"promotion_until":"0000-00-00 00:00:00","tags":"","downhash":"def456","hr":0}],"total":2}`

const hddolbyUserInfoFixtureJSON = `[{"id":"9876","username":"TestDolbyUser","added":"2024-03-15 12:00:00","last_access":"2025-01-15 09:00:00","class":"7","uploaded":"1099511627776","downloaded":"107374182400","seedbonus":"50000.5","sebonus":"12345.67","unread_messages":"3"}]`

const hddolbyDetailFixtureHTML = `<html><body>
<h1>
  <input name="torrent_name" type="hidden" value="[Test] Example.Movie.2025.BluRay.1080p" />
  <input name="detail_torrent_id" type="hidden" value="54321" />
  <font class="free">免费</font>
</h1>
<table>
  <tr>
    <td class="rowhead">基本信息</td>
    <td class="rowfollow">大小：12.5 GB | 类型：电影</td>
  </tr>
</table>
</body></html>`

// --- Helpers ---

func getHDDolbyDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("hddolby")
	require.True(t, ok, "hddolby definition not found")
	return def
}

func newTestHDDolbyDriver(def *v2.SiteDefinition) *v2.HDDolbyDriver {
	driver := v2.NewHDDolbyDriver(v2.HDDolbyDriverConfig{
		BaseURL: "https://www.hddolby.com",
		APIKey:  "FAKE_TEST_KEY_12345",
	})
	driver.SetSiteDefinition(def)
	return driver
}

// --- Suite: Search ---

func testHDDolbySearch(t *testing.T) {
	def := getHDDolbyDef(t)
	RequireNoSecrets(t, "hddolby_search", hddolbySearchFixtureJSON)

	resp := v2.HDDolbyResponse{
		Data: json.RawMessage(hddolbySearchFixtureJSON),
	}
	driver := newTestHDDolbyDriver(def)

	items, err := driver.ParseSearch(resp)
	require.NoError(t, err)
	require.Len(t, items, 2)

	free := items[0]
	assert.Equal(t, "Test.Movie.2025.BluRay.1080p", free.Title)
	assert.Equal(t, strconv.Itoa(12345), free.ID)
	assert.Equal(t, int64(45097156608), free.SizeBytes)
	assert.Equal(t, 100, free.Seeders)
	assert.Equal(t, 5, free.Leechers)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.True(t, free.HasHR)
	assert.False(t, free.DiscountEndTime.IsZero())
	assert.Equal(t, 2025, free.DiscountEndTime.Year())
	assert.Equal(t, 2, int(free.DiscountEndTime.Month()))
	assert.Equal(t, 15, free.DiscountEndTime.Day())

	normal := items[1]
	assert.Equal(t, strconv.Itoa(12346), normal.ID)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
	assert.False(t, normal.HasHR)
	assert.True(t, normal.DiscountEndTime.IsZero())
}

// --- Suite: Detail ---

func testHDDolbyDetail(t *testing.T) {
	def := getHDDolbyDef(t)
	RequireNoSecrets(t, "hddolby_detail", hddolbyDetailFixtureHTML)

	doc := FixtureDoc(t, "hddolby_detail", hddolbyDetailFixtureHTML)
	parser := v2.NewNexusPHPParserFromDefinition(def)
	info := parser.ParseAll(doc.Selection)

	assert.Equal(t, "54321", info.TorrentID)
	assert.Equal(t, "[Test] Example.Movie.2025.BluRay.1080p", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 12.5*1024, info.SizeMB, 0.1)
}

// --- Suite: UserInfo ---

func testHDDolbyUserInfo(t *testing.T) {
	def := getHDDolbyDef(t)
	RequireNoSecrets(t, "hddolby_userinfo", hddolbyUserInfoFixtureJSON)

	resp := v2.HDDolbyResponse{
		Data: json.RawMessage(hddolbyUserInfoFixtureJSON),
	}
	driver := newTestHDDolbyDriver(def)

	info, err := driver.ParseUserInfo(resp)
	require.NoError(t, err)

	assert.Equal(t, "9876", info.UserID)
	assert.Equal(t, "TestDolbyUser", info.Username)
	assert.Equal(t, int64(1099511627776), info.Uploaded)
	assert.Equal(t, int64(107374182400), info.Downloaded)
	assert.InDelta(t, 10.24, info.Ratio, 0.0001)
	assert.Equal(t, 50000.5, info.Bonus)
	assert.Equal(t, 12345.67, info.SeedingBonus)
	assert.Equal(t, "Extreme User", info.LevelName)
	assert.Equal(t, 3, info.UnreadMessageCount)
	assert.NotZero(t, info.JoinDate)
	assert.NotZero(t, info.LastAccess)
}

// --- Standalone Tests (edge cases beyond suite scope) ---

func TestHDDolby_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":   hddolbySearchFixtureJSON,
		"detail":   hddolbyDetailFixtureHTML,
		"userinfo": hddolbyUserInfoFixtureJSON,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
