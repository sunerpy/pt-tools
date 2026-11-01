package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 编辑站点以前发出的通知：整站保存按编号原地更新 RSS，之后点「立即下载」，按钮按通知记下的 RSS 编号照样找到这条 RSS，
// 并按编辑后的设置推送。以前保存时 RSS 删掉重建，按钮报「查询 RSS 订阅 #N 失败」。
func TestOnRSSDownload_AfterSiteSave(t *testing.T) {
	r := newButtonRig(t, false)
	r.lookupFn = func() (*v2.TorrentItem, error) {
		return &v2.TorrentItem{ID: "77", DiscountLevel: v2.DiscountNone, FetchedAt: time.Now().UTC()}, nil
	}

	// 整站保存的校验要求站点有地址（这个测试程序里查不到站点定义的默认地址）：先存一个，读出来再原样带回
	require.NoError(t, r.db.DB.Model(&models.SiteSetting{}).Where("name = ?", "springsunday").
		Update("api_url", "https://springsunday.net").Error)
	store := core.NewConfigStore(r.db)
	sc, err := store.GetSiteConf(models.SiteGroup("springsunday"))
	require.NoError(t, err)
	require.Len(t, sc.RSS, 1)
	// 站点用 Passkey 登录：保存时不用加解密 Cookie
	sc.AuthMethod, sc.Passkey = "passkey", "pk"
	sc.RSS[0].DownloadPath = "/data/edited"
	require.NoError(t, store.UpsertSiteWithRSS(models.SiteGroup("springsunday"), sc))

	require.NoError(t, r.actions.OnRSSDownload(context.Background(), r.row.ID, 0))

	require.Len(t, r.dl.adds, 1)
	assert.Equal(t, "/data/edited", r.dl.adds[0].SavePath, "按编辑后的设置推送")
	assert.Equal(t, []string{"bound"}, r.dl.used, "站点绑定的下载器不变")
	ti := r.record(t)
	require.NotNil(t, ti.SourceRSSID)
	assert.Equal(t, r.sub.ID, *ti.SourceRSSID, "所有者是通知里的那条 RSS")
}

// RSS 通知按 RSS 编号去重：整站保存（这里是扩展同步凭据）以后编号不变，下一轮读到同一个还没推送的种子不再通知第二遍。
// 以前保存时 RSS 删掉重建、编号全变，RSS 里还没推送的种子都会按新编号再通知一遍。
func TestRSSNotifier_NoRepeatAfterSiteSave(t *testing.T) {
	db, err := core.OpenTestDB(t)
	require.NoError(t, err)
	require.NoError(t, db.DB.AutoMigrate(&models.RSSNotificationLog{}, &models.NotificationConf{}, &models.NotificationOutbox{}))
	store := core.NewConfigStore(db)
	enabled := true
	// 站点用 Passkey 登录：保存时不用加解密 Cookie
	require.NoError(t, store.UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "passkey", Passkey: "pk", APIUrl: "https://springsunday.net",
		RSS: []models.RSSConfig{{
			Name: "全部通知", URL: "https://springsunday.net/torrentrss.php?rows=10", IntervalMinutes: 10,
			NotifyMode: "all", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 100,
		}},
	}))
	push := &capturePushService{}
	notifier := NewRSSNotifier(db.DB, push)
	// notifyRound 按库里现在的配置跑一轮通知（调度器重载以后就是这份配置）
	notifyRound := func() {
		t.Helper()
		cfg, cfgErr := store.GetSiteConf(models.SiteGroup("springsunday"))
		require.NoError(t, cfgErr)
		require.Len(t, cfg.RSS, 1)
		rss := cfg.RSS[0]
		require.NoError(t, notifier.NotifyNewItem(context.Background(), RSSItemEvent{
			RSS: &rss, FeedItem: newFeedItem(), SiteName: "springsunday", TorrentID: "12345",
		}))
	}

	notifyRound()
	require.Equal(t, 1, push.callCount())

	sc, err := store.GetSiteConf(models.SiteGroup("springsunday"))
	require.NoError(t, err)
	sc.Passkey = "pk2"
	require.NoError(t, store.UpsertSiteWithRSS(models.SiteGroup("springsunday"), sc))

	notifyRound()
	assert.Equal(t, 1, push.callCount(), "同一个种子不再通知第二遍")
}
