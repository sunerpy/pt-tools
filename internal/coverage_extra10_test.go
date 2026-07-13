// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestFetchAndDownloadFreeRSS_FilterRuleAssociated(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "cookie", Cookie: "c=1",
		RSS: []models.RSSConfig{{Name: "sub", URL: "http://placeholder", IntervalMinutes: 1, Tag: "movie"}},
	}))
	var sub models.RSSSubscription
	require.NoError(t, db.DB.First(&sub).Error)

	rule := models.FilterRule{Name: "any", Pattern: ".*", PatternType: "regex", MatchField: "both", RequireFree: false, Enabled: true, Priority: 100, Purpose: "download"}
	require.NoError(t, db.DB.Create(&rule).Error)
	require.NoError(t, db.DB.Model(&models.FilterRule{}).Where("id = ?", rule.ID).Update("require_free", false).Error)
	assoc := models.NewRSSFilterAssociationDB(db.DB)
	require.NoError(t, assoc.SetFilterRulesForRSS(sub.ID, []uint{rule.ID}))

	srv := feedServerUnified(t, rssBody(itemXML("lgf1", "RuleMatch", "http://x/r.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1, writeFile: true}
	rssCfg := models.RSSConfig{ID: sub.ID, Name: sub.Name, URL: srv.URL, Tag: "movie"}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site, rssCfg))

	assert.Equal(t, 1, site.dlCalls)
	ti, err := db.GetTorrentBySiteAndID("springsunday", "lgf1")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsDownloaded)
}

func TestFetchAndDownloadFreeRSS_AlreadyPushedSkips(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	pushed := true
	now := time.Now()
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "lgp", IsPushed: &pushed, PushTime: &now}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("lgp", "Dup", "http://x/d.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL}))
	assert.Equal(t, 0, site.dlCalls, "already-pushed torrent must be skipped")
}

func TestFetchAndDownloadFreeRSS_MinFreeMinutesSkips(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true,
		DownloadLimitEnabled: true, DownloadSpeedLimit: 10, MinFreeMinutes: 120,
	}))

	srv := feedServerUnified(t, rssBody(itemXML("lgmf", "SoonEnd", "http://x/s.torrent")))
	site := &legacyMinFreeStub{}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t"}))
	ti, err := db.GetTorrentBySiteAndID("springsunday", "lgmf")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsSkipped)
}

func TestFetchAndDownloadFreeRSS_MaxRetrySkips(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true, MaxRetry: 2,
	}))

	pushed := false
	future := time.Now().Add(time.Hour)
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "lgmr", FreeEndTime: &future, IsPushed: &pushed, RetryCount: 5,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("lgmr", "MaxRetry", "http://x/m.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))
	assert.Equal(t, 0, site.dlCalls, "over-max-retry torrent must not re-download")
}

// legacyMinFreeStub returns a free torrent whose EndTime is close enough that
// CanbeFinished + MinFreeMinutes forces a skip.
type legacyMinFreeStub struct{ legacyPTStub }

func (p *legacyMinFreeStub) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	return &models.APIResponse[models.PHPTorrentInfo]{
		Code: "success",
		Data: models.PHPTorrentInfo{
			Title: item.Title, TorrentID: item.GUID,
			Discount: models.DISCOUNT_FREE, SizeMB: 1, EndTime: time.Now().Add(30 * time.Minute),
		},
	}, nil
}
func (p *legacyMinFreeStub) IsEnabled() bool { return true }
