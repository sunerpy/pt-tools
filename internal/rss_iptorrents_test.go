package internal

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// iptorrentsFake 是站点分组为 iptorrents 的 unifiedFake：RSS 按 IPTorrents 的站点定义认条目的编号。
type iptorrentsFake struct{ *unifiedFake }

func (iptorrentsFake) SiteGroup() models.SiteGroup { return models.SiteGroup("iptorrents") }

// iptRSSItem 是 2026-11-21 经用户浏览器取到的 IPTorrents RSS 条目的结构（t.rss，Feed Type 选 Download link）：
// guid 是详情页地址（isPermaLink="true"），link 与附件是带 torrent_pass 的下载地址。编号、标题、体积是示例值。
func iptRSSItem(id, name string) string {
	dl := fmt.Sprintf("https://iptorrents.com/download.php/%s/%s.torrent?torrent_pass=FAKE_TEST_PASS", id, name)
	return fmt.Sprintf(`<item><title>%s</title><link>%s</link><guid isPermaLink="true">https://iptorrents.com/t/%s</guid>`+
		`<pubDate>Sat, 21 Nov 2026 11:23:00 +0000</pubDate><description>19.25 GB; Movie/4K (S:10 L:7)</description>`+
		`<enclosure url="%s" length="20669530112" type="application/x-bittorrent" /></item>`, name, dl, id, dl)
}

// IPTorrents 的 guid 是详情页地址：原样当种子编号时，暂存文件名（<站点>-<编号>.torrent）里的「: / .」被去掉，
// 记录与暂存文件对不上，下好的种子被当成孤立文件删掉、永远推不出去。按下载链接里的种子编号记。
func TestFetchUnified_IPTorrentsGUIDBecomesTorrentID(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := feedServerUnified(t, rssBody(iptRSSItem("9900123", "Fixture.Show.S01.2160p-FIXTURE")))
	site := iptorrentsFake{&unifiedFake{
		enabled: true, writeFile: true,
		detail: &v2.TorrentItem{ID: "9900123", Title: "Fixture.Show.S01.2160p-FIXTURE", DiscountLevel: v2.DiscountFree, SizeBytes: 1024},
	}}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))

	ti, err := db.GetTorrentBySiteAndID("iptorrents", "9900123")
	require.NoError(t, err)
	require.NotNil(t, ti, "记录按种子编号存，不按 guid 里的详情页地址")
	assert.True(t, ti.IsDownloaded, "暂存文件与记录对得上")
	assert.Equal(t, "9900123", ti.DetailID)
	assert.True(t, ti.HasHR, "整站 H&R：详情里没标也按 H&R 记")
	assert.Equal(t, 336, ti.HRSeedTimeH)
	gl, err := core.NewConfigStore(db).GetGlobalOnly()
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(gl.DownloadDir, "movie", "iptorrents-9900123.torrent"))

	var urlKeyed int64
	require.NoError(t, db.DB.Model(&models.TorrentInfo{}).Where("torrent_id LIKE ?", "%://%").Count(&urlKeyed).Error)
	assert.Zero(t, urlKeyed, "种子编号里不能是网址")
}

// 早先的 IPTorrents RSS 没有 guid（Sonarr 的 IPTorrents 测试样本）：每条都按下载链接里的种子编号记，
// 不能都挤在编号为空的同一条记录上（那样只有第一条会下载）。
func TestFetchUnified_IPTorrentsItemsWithoutGUID(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	body := rssBody(`<item><title>Fixture A</title><link>https://iptorrents.com/download.php/9900124/Fixture.A.torrent?torrent_pass=FAKE_TEST_PASS</link>` +
		`<pubDate>Mon, 12 May 2014 19:06:34 +0000</pubDate><description>556 MB; TV/x264</description></item>` +
		`<item><title>Fixture B</title><link>https://iptorrents.com/download.php/9900125/Fixture.B.torrent?torrent_pass=FAKE_TEST_PASS</link>` +
		`<pubDate>Mon, 12 May 2014 19:06:25 +0000</pubDate><description>Category: TV/x264 Size: 1.37 GB </description></item>`)
	srv := feedServerUnified(t, body)
	site := iptorrentsFake{&unifiedFake{
		enabled: true, writeFile: true,
		detail: &v2.TorrentItem{ID: "9900124", Title: "Fixture A", DiscountLevel: v2.DiscountFree, SizeBytes: 1024},
	}}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))

	for _, id := range []string{"9900124", "9900125"} {
		ti, err := db.GetTorrentBySiteAndID("iptorrents", id)
		require.NoError(t, err)
		require.NotNil(t, ti, id)
		assert.True(t, ti.IsDownloaded, id)
	}
	assert.Equal(t, int32(2), site.downloadCalls.Load(), "两条都下载")
}

func TestRSSItemGUID(t *testing.T) {
	ipt, ok := v2.GetDefinitionRegistry().Get("iptorrents")
	require.True(t, ok)
	nexus, ok := v2.GetDefinitionRegistry().Get("springsunday")
	require.True(t, ok)
	const dl = "https://iptorrents.com/download.php/123/Some.Release-GRP.torrent?torrent_pass=FAKE_TEST_PASS"

	cases := []struct {
		name string
		def  *v2.SiteDefinition
		item *gofeed.Item
		want string
	}{
		{"详情页地址换成下载链接里的编号", ipt, &gofeed.Item{GUID: "https://iptorrents.com/t/123", Link: dl}, "123"},
		{"只有附件也认", ipt, &gofeed.Item{GUID: "https://iptorrents.com/t/123", Enclosures: []*gofeed.Enclosure{nil, {URL: dl}}}, "123"},
		{"没有 guid 时按下载链接", ipt, &gofeed.Item{Link: dl}, "123"},
		{"已经是编号的不动", ipt, &gofeed.Item{GUID: "456", Link: dl}, "456"},
		{"链接不是下载地址（Web link）时照旧", ipt, &gofeed.Item{GUID: "https://iptorrents.com/t/123", Link: "https://iptorrents.com/t/123"}, "https://iptorrents.com/t/123"},
		{"别的域名上的链接不认", ipt, &gofeed.Item{GUID: "https://iptorrents.com/t/123", Link: "https://elsewhere.invalid/download.php/123/x.torrent"}, "https://iptorrents.com/t/123"},
		{"别的站点照旧", nexus, &gofeed.Item{GUID: "https://springsunday.net/details.php?id=1", Link: "https://springsunday.net/download.php?id=1"}, "https://springsunday.net/details.php?id=1"},
		{"没有站点定义照旧", nil, &gofeed.Item{GUID: "https://iptorrents.com/t/123", Link: dl}, "https://iptorrents.com/t/123"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, rssItemGUID(c.def, c.item), c.name)
	}
	assert.Empty(t, rssItemGUID(ipt, nil))
}
