package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
)

// 只改「自动启动」一列：其余全局设置原样保留，改了才发布配置变更。
func TestSetAutoStart_OnlyTouchesAutoStart(t *testing.T) {
	db := newTempDB(t)
	s := NewConfigStore(db)
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir: "/data/downloads", DefaultIntervalMinutes: 15, TorrentSizeGB: 80,
		MinFreeMinutes: 45, RetainHours: 12, MaxRetry: 5, CleanupEnabled: true,
	}).Error)

	_, ch, cancel := events.Subscribe(4)
	defer cancel()
	require.NoError(t, s.SetAutoStart(true))

	select {
	case e := <-ch:
		assert.Equal(t, events.ConfigChanged, e.Type)
		assert.Equal(t, "global", e.Source)
	case <-time.After(time.Second):
		t.Fatal("打开自动启动应发布 ConfigChanged")
	}

	gs, err := s.GetGlobalSettings()
	require.NoError(t, err)
	assert.True(t, gs.AutoStart)
	assert.Equal(t, "/data/downloads", gs.DownloadDir)
	assert.EqualValues(t, 15, gs.DefaultIntervalMinutes)
	assert.Equal(t, 80, gs.TorrentSizeGB)
	assert.Equal(t, 45, gs.MinFreeMinutes)
	assert.Equal(t, 12, gs.RetainHours)
	assert.Equal(t, 5, gs.MaxRetry)
	assert.True(t, gs.CleanupEnabled)

	// 值没变：不写库，也不发布
	require.NoError(t, s.SetAutoStart(true))
	select {
	case e := <-ch:
		t.Fatalf("值没变时不应发布配置变更: %+v", e)
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, s.SetAutoStart(false))
	gs, err = s.GetGlobalSettings()
	require.NoError(t, err)
	assert.False(t, gs.AutoStart, "关掉也要写进去（布尔零值）")
}

func TestSetAutoStart_NoGlobalRow(t *testing.T) {
	s := NewConfigStore(newTempDB(t))
	assert.Error(t, s.SetAutoStart(true))
}

// 只数已启用站点的订阅，不解密 Cookie（密钥缺失时状态查询照样能用）。
func TestEnabledRSS_OnlyEnabledSites(t *testing.T) {
	db := newTempDB(t)
	s := NewConfigStore(db)
	on := models.SiteSetting{Name: "hdsky", Enabled: true, CookieEncrypted: "not-decryptable"}
	off := models.SiteSetting{Name: "mteam", Enabled: false}
	require.NoError(t, db.DB.Create(&on).Error)
	require.NoError(t, db.DB.Create(&off).Error)
	require.NoError(t, db.DB.Create(&models.RSSSubscription{SiteID: on.ID, Name: "a", URL: "https://hdsky.me/rss/a", IntervalMinutes: 10}).Error)
	require.NoError(t, db.DB.Create(&models.RSSSubscription{SiteID: on.ID, Name: "b", URL: "https://hdsky.me/rss/b", IntervalMinutes: 10, IsExample: true}).Error)
	require.NoError(t, db.DB.Create(&models.RSSSubscription{SiteID: off.ID, Name: "c", URL: "https://m-team.cc/rss", IntervalMinutes: 10}).Error)

	rss, err := s.EnabledRSS()
	require.NoError(t, err)
	names := make([]string, 0, len(rss))
	for _, r := range rss {
		names = append(names, r.Name)
	}
	assert.ElementsMatch(t, []string{"a", "b"}, names, "停用站点的订阅不算；示例订阅留给调用方按 ShouldSkip 剔除")
	for _, r := range rss {
		if r.Name == "b" {
			assert.True(t, r.IsExample)
		}
	}

	require.NoError(t, db.DB.Model(&models.SiteSetting{}).Where("id = ?", on.ID).Update("enabled", false).Error)
	rss, err = s.EnabledRSS()
	require.NoError(t, err)
	assert.Empty(t, rss)
}
