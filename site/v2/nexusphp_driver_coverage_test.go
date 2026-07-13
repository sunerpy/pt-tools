package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	require.NoError(t, err)
	return doc
}

func TestTruncateStr(t *testing.T) {
	assert.Equal(t, "abc", truncateStr("abc", 10))
	assert.Equal(t, "ab...", truncateStr("abcdef", 2))
}

func TestNexusPHPDriver_GetSiteDefinition(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	assert.Nil(t, d.GetSiteDefinition())
	def := &SiteDefinition{ID: "x"}
	d.SetSiteDefinition(def)
	assert.Equal(t, def, d.GetSiteDefinition())
}

func TestNexusPHPDriver_getSiteID(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://hdsky.me"})
	assert.Equal(t, "hdsky", d.getSiteID())
	d.SetSiteDefinition(&SiteDefinition{ID: "custom"})
	assert.Equal(t, "custom", d.getSiteID())
}

func TestExtractUserID(t *testing.T) {
	assert.Equal(t, "12345", extractUserID("userdetails.php?id=12345"))
	assert.Equal(t, "", extractUserID("no id here"))
}

func TestExtractTorrentIDFromURL(t *testing.T) {
	assert.Equal(t, "42", extractTorrentIDFromURL("https://x.com/details.php?id=42"))
	assert.Equal(t, "99", extractTorrentIDFromURL("https://x.com/torrents/99/name"))
	assert.Equal(t, "", extractTorrentIDFromURL("https://x.com/nothing"))
	assert.Equal(t, "", extractTorrentIDFromURL("://bad"))
}

func TestExtractSiteIDFromURL(t *testing.T) {
	assert.Equal(t, "hdsky", extractSiteIDFromURL("https://hdsky.me"))
	assert.Equal(t, "mteam", extractSiteIDFromURL("https://api.m-team.cc"))
	assert.Equal(t, "", extractSiteIDFromURL("://bad"))
	assert.Equal(t, "localhost", extractSiteIDFromURL("http://localhost:8080"))
}

func TestExtractNumber(t *testing.T) {
	assert.Equal(t, "123456", extractNumber("123,456 (详情)"))
	assert.Equal(t, "3.14", extractNumber("pi is 3.14"))
	assert.Equal(t, "", extractNumber("no digits"))
}

func TestExtractValueFromTransfer(t *testing.T) {
	text := "上传量: 1.5 TB 下载量: 500 GB 分享率: 3.5"
	assert.Equal(t, "3.5", extractValueFromTransfer(text, "分享率", "Ratio"))
	assert.Equal(t, "", extractValueFromTransfer(text, "NotThere"))
}

func TestExtractSizeFromTransfer(t *testing.T) {
	text := "上传量: 1.5 TB 下载量: 500 GB"
	up := extractSizeFromTransfer(text, "上传量", "Uploaded")
	assert.Greater(t, up, int64(0))
	assert.Equal(t, int64(0), extractSizeFromTransfer(text, "NotThere"))
}

func TestFindTextByLabel(t *testing.T) {
	doc := mustDoc(t, `<table><tr><td>上传量</td><td>1.5 TB</td></tr></table>`)
	assert.Equal(t, "1.5 TB", findTextByLabel(doc, "上传量"))
	assert.Equal(t, "", findTextByLabel(doc, "不存在"))
}

func TestFindInfoBlockValue(t *testing.T) {
	doc := mustDoc(t, `<div id="info_block">魔力值: 12345 | 上传量: 1.5 TB</div>`)
	assert.Equal(t, "12345", findInfoBlockValue(doc, "魔力值"))
	assert.Equal(t, "", findInfoBlockValue(doc, "不存在"))

	empty := mustDoc(t, `<div>nothing</div>`)
	assert.Equal(t, "", findInfoBlockValue(empty, "魔力值"))
}

func TestNexusPHPDriver_PrepareUserDetails(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	req, err := d.PrepareUserDetails("777")
	require.NoError(t, err)
	assert.Equal(t, "/userdetails.php", req.Path)
	assert.Equal(t, "777", req.Params.Get("id"))
}

func TestNexusPHPDriver_ParseUserInfo(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})

	// nil doc -> parse error
	_, err := d.ParseUserInfo(NexusPHPResponse{})
	assert.ErrorIs(t, err, ErrParseError)

	html := `<html><body>
	<div id="info_block">
		<a class="User_Name" href="userdetails.php?id=888">tester</a>
		上传量: 1.5 TB 下载量: 500 GB 分享率: 3.0 魔力值: 12345 等级: Power User
	</div>
	</body></html>`
	doc := mustDoc(t, html)
	info, err := d.ParseUserInfo(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, "888", info.UserID)
	assert.Greater(t, info.Uploaded, int64(0))
}

func TestNexusPHPDriver_ParseUserDetails(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})

	_, err := d.ParseUserDetails(NexusPHPResponse{})
	assert.ErrorIs(t, err, ErrParseError)

	html := `<html><body>
	<table>
		<tr><td class="rowhead">用户名</td><td class="rowfollow">tester</td></tr>
		<tr><td class="rowhead">上传量</td><td class="rowfollow">1.5 TB</td></tr>
		<tr><td class="rowhead">下载量</td><td class="rowfollow">500 GB</td></tr>
		<tr><td class="rowhead">分享率</td><td class="rowfollow">3.0</td></tr>
		<tr><td class="rowhead">魔力值</td><td class="rowfollow">12,345 (详情)</td></tr>
		<tr><td class="rowhead">等级</td><td class="rowfollow">Power User</td></tr>
		<tr><td class="rowhead">上次访问</td><td class="rowfollow">2024-06-01 12:00:00</td></tr>
		<tr><td class="rowhead">上次登录</td><td class="rowfollow">2024-06-02 09:00:00</td></tr>
	</table>
	</body></html>`
	doc := mustDoc(t, html)
	info, err := d.ParseUserDetails(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Equal(t, "tester", info.Username)
	assert.Greater(t, info.Uploaded, int64(0))
	assert.Greater(t, info.Downloaded, int64(0))
	assert.InDelta(t, 3.0, info.Ratio, 0.01)
	assert.InDelta(t, 12345, info.Bonus, 0.5)
	assert.Equal(t, "Power User", info.Rank)
	assert.Greater(t, info.LastAccess, int64(0))
	assert.Greater(t, info.LastLogin, int64(0))
}

func TestNexusPHPDriver_ParseUserDetails_TransferRow(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body>
	<table>
		<tr><td class="rowhead">传输</td><td class="rowfollow">上传量: 2.0 TB 下载量: 1.0 TB 分享率: 2.0</td></tr>
		<tr><td class="rowhead">做种积分</td><td class="rowfollow">42</td></tr>
	</table>
	</body></html>`
	doc := mustDoc(t, html)
	info, err := d.ParseUserDetails(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Greater(t, info.Uploaded, int64(0))
	assert.Greater(t, info.Downloaded, int64(0))
	assert.InDelta(t, 2.0, info.Ratio, 0.01)
	assert.Equal(t, 42, info.Seeding)
}

func TestNexusPHPDriver_getUserInfoLegacy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.Path, "userdetails") {
			w.Write([]byte(`<html><body><table>
				<tr><td class="rowhead">用户名</td><td class="rowfollow">detailname</td></tr>
				<tr><td class="rowhead">上传量</td><td class="rowfollow">2.0 TB</td></tr>
				<tr><td class="rowhead">下载量</td><td class="rowfollow">1.0 TB</td></tr>
				<tr><td class="rowhead">分享率</td><td class="rowfollow">2.0</td></tr>
			</table></body></html>`))
			return
		}
		w.Write([]byte(`<html><body>
			<div id="info_block"><a class="User_Name" href="userdetails.php?id=888">tester</a></div>
		</body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "detailname", info.Username)
	assert.Greater(t, info.Uploaded, int64(0))
}

func TestNexusPHPDriver_getUserInfoWithDefinition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch {
		case strings.Contains(r.URL.Path, "userdetails"):
			w.Write([]byte(`<html><body><table>
				<tr><td class="rowhead">上传量</td><td class="rowfollow" id="up">91970600</td></tr>
			</table></body></html>`))
		case strings.Contains(r.URL.Path, "getusertorrentlistajax"):
			w.Write([]byte(`<html><body><b>10</b>条记录，共计<b>2.5 TB</b><table></table></body></html>`))
		default:
			w.Write([]byte(`<html><body><a id="uid" href="userdetails.php?id=888">tester</a></body></html>`))
		}
	}))
	defer server.Close()

	def := &SiteDefinition{
		ID: "testsite",
		UserInfo: &UserInfoConfig{
			Process: []UserInfoProcess{
				{
					RequestConfig: RequestConfig{URL: "/index.php", ResponseType: "document"},
					Fields:        []string{"id", "name"},
				},
				{
					RequestConfig: RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
					Assertion:     map[string]string{"id": "params.id"},
					Fields:        []string{"uploaded"},
				},
			},
			Selectors: map[string]FieldSelector{
				"id":       {Selector: []string{"#uid"}, Attr: "href", Filters: []Filter{{Name: "querystring", Args: []any{"id"}}}},
				"name":     {Selector: []string{"#uid"}},
				"uploaded": {Selector: []string{"#up"}},
			},
		},
	}

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	d.SetSiteDefinition(def)

	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "888", info.UserID)
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, int64(91970600), info.Uploaded)
	// seeding status fetched via fallback since no seedingSize selector
	assert.Greater(t, info.SeederSize, int64(0))
}

func TestNexusPHPDriver_setUserInfoField(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	info := &UserInfo{}

	d.setUserInfoField(info, "id", "5")
	assert.Equal(t, "5", info.UserID)
	d.setUserInfoField(info, "name", "bob")
	assert.Equal(t, "bob", info.Username)
	d.setUserInfoField(info, "uploaded", "1.5 GB")
	assert.Greater(t, info.Uploaded, int64(0))
	d.setUserInfoField(info, "downloaded", "500 MB")
	assert.Greater(t, info.Downloaded, int64(0))
	d.setUserInfoField(info, "ratio", "3.0")
	assert.InDelta(t, 3.0, info.Ratio, 0.01)
	d.setUserInfoField(info, "bonus", "12345")
	assert.InDelta(t, 12345, info.Bonus, 0.5)
	d.setUserInfoField(info, "levelName", "PU")
	assert.Equal(t, "PU", info.LevelName)
	d.setUserInfoField(info, "seedingBonus", "100")
	assert.InDelta(t, 100, info.SeedingBonus, 0.5)
	d.setUserInfoField(info, "bonusPerHour", "5.5")
	assert.InDelta(t, 5.5, info.BonusPerHour, 0.01)
	d.setUserInfoField(info, "joinTime", "1600000000")
	assert.Equal(t, int64(1600000000), info.JoinDate)
	d.setUserInfoField(info, "lastAccessAt", "1700000000")
	assert.Equal(t, int64(1700000000), info.LastAccess)
	d.setUserInfoField(info, "lastLoginAt", "1710000000")
	assert.Equal(t, int64(1710000000), info.LastLogin)
	d.setUserInfoField(info, "messageCount", "3")
	assert.Equal(t, 3, info.UnreadMessageCount)
	d.setUserInfoField(info, "hnrUnsatisfied", "2")
	assert.Equal(t, 2, info.HnRUnsatisfied)
	d.setUserInfoField(info, "hnrPreWarning", "1")
	assert.Equal(t, 1, info.HnRPreWarning)
	d.setUserInfoField(info, "seeding", "10")
	assert.Equal(t, 10, info.Seeding)
	d.setUserInfoField(info, "leeching", "1")
	assert.Equal(t, 1, info.Leeching)
	d.setUserInfoField(info, "uploads", "7")
	assert.Equal(t, 7, info.Uploads)
	d.setUserInfoField(info, "trueUploaded", "1 GB")
	assert.Greater(t, info.TrueUploaded, int64(0))
	d.setUserInfoField(info, "trueDownloaded", "1 GB")
	assert.Greater(t, info.TrueDownloaded, int64(0))
	d.setUserInfoField(info, "seederSize", "1 GB")
	assert.Greater(t, info.SeederSize, int64(0))
	d.setUserInfoField(info, "leecherSize", "1 GB")
	assert.Greater(t, info.LeecherSize, int64(0))
}

func TestNexusPHPDriver_FetchSeedingStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><b>94</b>条记录，共计<b>2.756 TB</b><table></table></body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	seeding, size, err := d.FetchSeedingStatus(context.Background(), "888")
	require.NoError(t, err)
	assert.Equal(t, 94, seeding)
	assert.Greater(t, size, int64(0))
}

func TestNexusPHPDriver_FetchSeedingStatus_NoTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body>no table here</body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	seeding, size, err := d.FetchSeedingStatus(context.Background(), "888")
	require.NoError(t, err)
	assert.Equal(t, 0, seeding)
	assert.Equal(t, int64(0), size)
}

func TestNexusPHPDriver_GetTorrentDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body>
			<input name="torrent_name" value="My.Movie.2024">
			<input name="detail_torrent_id" value="42">
			<h1><font class="free">免费</font><span title="2026-01-20 15:30:00">2天</span></h1>
			<td class="rowhead">基本信息</td><td>大小：16.87 GB</td>
		</body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	item, err := d.GetTorrentDetail(context.Background(), "42", "", "")
	require.NoError(t, err)
	assert.Equal(t, "42", item.ID)
	assert.Equal(t, "My.Movie.2024", item.Title)
}

func TestNexusPHPDriver_GetTorrentDetail_IDFromLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "99", r.URL.Query().Get("id"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><input name="torrent_name" value="X"></body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	_, err := d.GetTorrentDetail(context.Background(), "", server.URL+"/details.php?id=99", "")
	require.NoError(t, err)
}

func TestNexusPHPDriver_GetTorrentDetail_NoID(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	_, err := d.GetTorrentDetail(context.Background(), "", "", "")
	assert.Error(t, err)
}

func TestNexusPHPDriver_ParseDownload(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})

	// raw body path (no document)
	data, err := d.ParseDownload(NexusPHPResponse{RawBody: []byte("torrentbytes")})
	require.NoError(t, err)
	assert.Equal(t, []byte("torrentbytes"), data)

	// nil doc + empty body -> error
	_, err = d.ParseDownload(NexusPHPResponse{})
	assert.ErrorIs(t, err, ErrParseError)
}
