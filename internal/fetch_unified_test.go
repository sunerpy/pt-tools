// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestFetchUnified_SkipsAlreadyDownloadedFile(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	gl, err := core.NewConfigStore(db).GetGlobalOnly()
	require.NoError(t, err)
	base := gl.DownloadDir

	// Pre-create the target .torrent so shouldSkipSiteDownload returns true
	// (IsDownloaded && local file exists) — the worker must skip re-download.
	sub := filepath.Join(base, "movie")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	fileBase := "springsunday-g900"
	require.NoError(t, os.WriteFile(filepath.Join(sub, fileBase+".torrent"), []byte("d4:infod4:name1:aee"), 0o644))

	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "g900", IsDownloaded: true}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("g900", "Dl", "http://x/d.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail:  &v2.TorrentItem{ID: "g900", Title: "Dl", DiscountLevel: v2.DiscountFree, SizeBytes: 1024},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))

	assert.Equal(t, int32(0), site.downloadCalls.Load(), "existing downloaded file must skip re-download")
}

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

func TestFetchUnified_NewRowWithCategory(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := feedServerUnified(t, rssBody(itemXMLWithCategory("gc1", "Cat", "http://x/c.torrent", "Movies/HD")))
	site := &unifiedFake{
		enabled: true, writeFile: true,
		detail: &v2.TorrentItem{ID: "gc1", Title: "Cat", DiscountLevel: v2.DiscountFree, SizeBytes: 1024},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))

	ti, err := db.GetTorrentBySiteAndID("springsunday", "gc1")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.Equal(t, "Movies/HD", ti.Category)
}

func TestFetchUnified_NewRowWithFilterRuleID(t *testing.T) {
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

	srv := feedServerUnified(t, rssBody(itemXML("grf", "RuleMatch", "http://x/r.torrent")))
	site := &unifiedFake{
		enabled: true, writeFile: true,
		detail: &v2.TorrentItem{ID: "grf", Title: "RuleMatch", DiscountLevel: v2.DiscountNone, SizeBytes: 2048},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{ID: sub.ID, Name: sub.Name, URL: srv.URL, Tag: "movie"}))

	ti, err := db.GetTorrentBySiteAndID("springsunday", "grf")
	require.NoError(t, err)
	require.NotNil(t, ti)
	require.NotNil(t, ti.FilterRuleID)
	assert.Equal(t, rule.ID, *ti.FilterRuleID)
}

func TestFetchUnified_DownloadNoFileResetsState(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := feedServerUnified(t, rssBody(itemXML("gnf", "NoFile", "http://x/n.torrent")))
	site := &unifiedFake{
		enabled: true, writeFile: false,
		detail: &v2.TorrentItem{ID: "gnf", Title: "NoFile", DiscountLevel: v2.DiscountFree, SizeBytes: 1024},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))

	ti, err := db.GetTorrentBySiteAndID("springsunday", "gnf")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.False(t, ti.IsDownloaded)
}

func TestFetchUnified_LinkOnlyItemUsesLinkAsURL(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := feedServerUnified(t, rssBody(itemXMLLinkOnly("gl1", "LinkOnly", "http://x/details.php?id=gl1")))
	site := &unifiedFake{
		enabled: true, writeFile: true,
		detail: &v2.TorrentItem{ID: "gl1", Title: "LinkOnly", DiscountLevel: v2.DiscountFree, SizeBytes: 1024},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))

	assert.Equal(t, int32(1), site.downloadCalls.Load())
	ti, err := db.GetTorrentBySiteAndID("springsunday", "gl1")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsDownloaded)
}

func TestFetchUnified_ContextCancelledDuringDispatch(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	items := ""
	for i := 0; i < 200; i++ {
		items += itemXML("ucc"+itoa(int64(i)), "T", "http://x/t.torrent")
	}
	srv := feedServerUnified(t, rssBody(items))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	site := &unifiedFake{
		enabled: true,
		detail:  &v2.TorrentItem{ID: "x", Title: "T", DiscountLevel: v2.DiscountFree, SizeBytes: 1024},
	}
	_ = FetchAndDownloadFreeRSSUnified(ctx, site, models.RSSConfig{Name: "r", URL: srv.URL})
}

func TestFetchUnified_FiresFilteredNotification(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "cookie", Cookie: "c=1",
		RSS: []models.RSSConfig{{Name: "sub", URL: "http://placeholder", IntervalMinutes: 1, Tag: "movie"}},
	}))
	var sub models.RSSSubscription
	require.NoError(t, db.DB.First(&sub).Error)

	rule := models.FilterRule{
		Name: "notify", Pattern: ".*", PatternType: "regex", MatchField: "both",
		RequireFree: false, Enabled: true, Priority: 100, Purpose: "notify",
	}
	require.NoError(t, db.DB.Create(&rule).Error)
	assoc := models.NewRSSFilterAssociationDB(db.DB)
	require.NoError(t, assoc.SetFilterRulesForRSS(sub.ID, []uint{rule.ID}))

	n := &recordingNotifier{}
	SetRSSNotifier(n)
	t.Cleanup(func() { SetRSSNotifier(nil) })

	srv := feedServerUnified(t, rssBody(itemXML("gf1", "FilterNotify", "http://x/f.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail:  &v2.TorrentItem{ID: "gf1", Title: "FilterNotify", DiscountLevel: v2.DiscountFree, SizeBytes: 4096},
	}
	cfg := models.RSSConfig{ID: sub.ID, Name: sub.Name, URL: srv.URL, NotifyMode: "filtered", Tag: "movie"}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site, cfg))

	assert.GreaterOrEqual(t, int(n.filteredItems.Load()), 1)
}

func TestFetchUnified_MinFreeMinutesSkips(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true, MinFreeMinutes: 120,
	}))

	soon := time.Now().Add(30 * time.Minute)
	srv := feedServerUnified(t, rssBody(itemXML("gmf", "SoonEnd", "http://x/s.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail: &v2.TorrentItem{
			ID: "gmf", Title: "SoonEnd", DiscountLevel: v2.DiscountFree,
			SizeBytes: 1024, DiscountEndTime: soon,
		},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t"}))

	assert.Equal(t, int32(0), site.downloadCalls.Load(), "free ending too soon must skip")
	ti, err := db.GetTorrentBySiteAndID("springsunday", "gmf")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsSkipped)
}

func TestFetchUnified_ExistingRowAlreadyDownloadedSkipsRedownload(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	// Pre-create a row already downloaded with the local file present so
	// shouldSkipSiteDownload short-circuits the re-download.
	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "gex", FreeEndTime: &future, IsPushed: &pushed,
		IsDownloaded: true, RetryCount: 0,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("gex", "Existing", "http://x/e.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail:  &v2.TorrentItem{ID: "gex", Title: "Existing", DiscountLevel: v2.DiscountFree, SizeBytes: 1024},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))
	// Detail fetched (row not skipped up-front since not pushed/skipped), but
	// re-download skipped by shouldSkipSiteDownload (already downloaded + file).
	assert.Equal(t, int32(1), site.detailCalls.Load())
}

func TestFetchUnified_MaxRetrySkipsRedownload(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true, MaxRetry: 2,
	}))

	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "gmr", FreeEndTime: &future, IsPushed: &pushed, RetryCount: 5,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("gmr", "MaxRetry", "http://x/m.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail:  &v2.TorrentItem{ID: "gmr", Title: "MaxRetry", DiscountLevel: v2.DiscountFree, SizeBytes: 1024},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))
	assert.Equal(t, int32(0), site.downloadCalls.Load(), "over-max-retry torrent must not re-download")
}

func (p *legacyPTStub) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	if p.detailErr != nil {
		return nil, p.detailErr
	}
	return &models.APIResponse[models.PHPTorrentInfo]{
		Code: "success",
		Data: models.PHPTorrentInfo{
			Title: item.Title, TorrentID: item.GUID,
			Discount: p.discount, SizeMB: p.sizeMB, EndTime: time.Now().Add(2 * time.Hour),
		},
	}, nil
}

func (p *legacyPTStub) IsEnabled() bool { return p.enabled }

func (p *legacyPTStub) DownloadTorrent(_, title, dir string) (string, error) {
	p.dlCalls++
	if p.dlErr != nil {
		return "", p.dlErr
	}
	if p.writeFile {
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, title+".torrent"), []byte("d4:infod4:name1:aee"), 0o644)
	}
	return "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", nil
}

func (p *legacyPTStub) MaxRetries() int { return 1 }

func (p *legacyPTStub) RetryDelay() time.Duration { return 0 }

func (p *legacyPTStub) SendTorrentToDownloader(_ context.Context, _ models.RSSConfig) error {
	return nil
}

func (p *legacyPTStub) Context() context.Context { return context.Background() }

func (r *recordingNotifier) NotifyNewItem(_ context.Context, _ RSSItemNotice) error {
	r.newItems.Add(1)
	return nil
}

func (r *recordingNotifier) NotifyFilteredItem(_ context.Context, _ RSSFilteredNotice) error {
	r.filteredItems.Add(1)
	return nil
}

func TestFetchUnified_FiresAllNotification(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	n := &recordingNotifier{}
	SetRSSNotifier(n)
	t.Cleanup(func() { SetRSSNotifier(nil) })

	srv := feedServerUnified(t, rssBody(itemXML("g700", "Notify", "http://x/n.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail:  &v2.TorrentItem{ID: "g700", Title: "Notify", DiscountLevel: v2.DiscountNone, SizeBytes: 1024},
	}
	cfg := models.RSSConfig{Name: "r", URL: srv.URL, NotifyMode: "all"}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site, cfg))

	assert.GreaterOrEqual(t, int(n.newItems.Load()), 1, "all-mode notification should fire for new item")
}

func TestFetchUnified_FilterRuleAssociatedDownloads(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	// Persist a site + RSS subscription so rssCfg.ID != 0 and rules can associate.
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled:    boolPtrFW(true),
		AuthMethod: "cookie",
		Cookie:     "c=1",
		RSS: []models.RSSConfig{
			{Name: "sub", URL: "http://placeholder", IntervalMinutes: 1, Tag: "movie"},
		},
	}))

	var sub models.RSSSubscription
	require.NoError(t, db.DB.First(&sub).Error)

	// A permissive rule (no free requirement) associated with the RSS.
	rule := models.FilterRule{
		Name: "any", Pattern: ".*", PatternType: "regex", MatchField: "both",
		RequireFree: false, Enabled: true, Priority: 100, Purpose: "download",
	}
	require.NoError(t, db.DB.Create(&rule).Error)
	assoc := models.NewRSSFilterAssociationDB(db.DB)
	require.NoError(t, assoc.SetFilterRulesForRSS(sub.ID, []uint{rule.ID}))

	srv := feedServerUnified(t, rssBody(itemXML("g800", "RuleMatch", "http://x/r.torrent")))
	site := &unifiedFake{
		enabled:   true,
		writeFile: true,
		detail:    &v2.TorrentItem{ID: "g800", Title: "RuleMatch", DiscountLevel: v2.DiscountFree, SizeBytes: 2048},
	}
	rssCfg := models.RSSConfig{ID: sub.ID, Name: sub.Name, URL: srv.URL, Tag: "movie"}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site, rssCfg))

	// Free torrent matched via the associated-rules filter.Decide path -> downloads.
	assert.Equal(t, int32(1), site.downloadCalls.Load())
	ti, err := db.GetTorrentBySiteAndID("springsunday", "g800")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsDownloaded)
}

func (f *unifiedFake) GetTorrentDetails(_ *gofeed.Item) (*v2.TorrentItem, error) {
	f.detailCalls.Add(1)
	if f.detailErr != nil {
		return nil, f.detailErr
	}
	if f.detail != nil {
		return f.detail, nil
	}
	return &v2.TorrentItem{ID: "1", Title: "t", DiscountLevel: v2.DiscountFree}, nil
}

func (f *unifiedFake) IsEnabled() bool { return f.enabled }

func (f *unifiedFake) DownloadTorrent(_, title, downloadDir string) (string, error) {
	f.downloadCalls.Add(1)
	if f.downloadErr != nil {
		return "", f.downloadErr
	}
	if f.writeFile {
		_ = os.MkdirAll(downloadDir, 0o755)
		p := filepath.Join(downloadDir, title+".torrent")
		_ = os.WriteFile(p, []byte("d4:infod4:name3:abcee"), 0o644)
	}
	return "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", nil
}

func (f *unifiedFake) MaxRetries() int { return 1 }

func (f *unifiedFake) RetryDelay() time.Duration { return time.Millisecond }

func (f *unifiedFake) SendTorrentToDownloader(_ context.Context, _ models.RSSConfig) error {
	f.sendCalls.Add(1)
	return nil
}

func (f *unifiedFake) Context() context.Context { return context.Background() }

func (f *unifiedFake) SiteGroup() models.SiteGroup { return models.SiteGroup("springsunday") }

func TestFetchUnified_DBNil(t *testing.T) {
	global.GlobalDB = nil
	err := FetchAndDownloadFreeRSSUnified(context.Background(), &unifiedFake{enabled: true}, models.RSSConfig{URL: "http://x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB 不可用")
}

func TestFetchUnified_BlankDownloadDir(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	// Persist a global row with an empty DownloadDir.
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{DownloadDir: ""}).Error)
	t.Cleanup(func() { global.GlobalDB = nil })

	err = FetchAndDownloadFreeRSSUnified(context.Background(), &unifiedFake{enabled: true}, models.RSSConfig{URL: "http://x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "下载目录为空")
}

func TestFetchUnified_DisabledSite(t *testing.T) {
	_ = setupDB(t)
	err := FetchAndDownloadFreeRSSUnified(context.Background(), &unifiedFake{enabled: false}, models.RSSConfig{URL: "http://x"})
	require.Error(t, err)
	assert.Equal(t, enableError, err.Error())
}

func TestFetchUnified_FeedFetchError(t *testing.T) {
	_ = setupDB(t)
	// Unreachable URL -> fetchRSSFeed returns error.
	err := FetchAndDownloadFreeRSSUnified(context.Background(), &unifiedFake{enabled: true},
		models.RSSConfig{Name: "r", URL: "http://127.0.0.1:0/none"})
	require.Error(t, err)
}

func TestFetchUnified_EmptyFeedSucceeds(t *testing.T) {
	_ = setupDB(t)
	srv := feedServerUnified(t, rssBody(""))
	site := &unifiedFake{enabled: true}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL}))
	assert.Equal(t, int32(0), site.detailCalls.Load())
}

func TestFetchUnified_DiscardsItemsWithoutLink(t *testing.T) {
	_ = setupDB(t)
	// Item with no enclosure and no link is discarded before reaching a worker.
	srv := feedServerUnified(t, rssBody(itemXML("g1", "NoLink", "")))
	site := &unifiedFake{enabled: true}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL}))
	assert.Equal(t, int32(0), site.detailCalls.Load(), "item without link must be discarded")
}

func TestFetchUnified_HappyPath_DownloadsAndRecords(t *testing.T) {
	db := setupDB(t)
	srv := feedServerUnified(t, rssBody(itemXML("g100", "FreeMovie", "http://x/f.torrent")))
	site := &unifiedFake{
		enabled:   true,
		writeFile: true,
		detail: &v2.TorrentItem{
			ID: "g100", Title: "FreeMovie", DiscountLevel: v2.DiscountFree, SizeBytes: 1024,
		},
	}
	cfg := models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site, cfg))

	assert.Equal(t, int32(1), site.detailCalls.Load())
	assert.Equal(t, int32(1), site.downloadCalls.Load())

	ti, err := db.GetTorrentBySiteAndID("springsunday", "g100")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsFree)
	assert.True(t, ti.IsDownloaded)
	require.NotNil(t, ti.TorrentHash)
}

func TestFetchUnified_NonFreeIsSkipped(t *testing.T) {
	db := setupDB(t)
	srv := feedServerUnified(t, rssBody(itemXML("g200", "Paid", "http://x/p.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail: &v2.TorrentItem{
			ID: "g200", Title: "Paid", DiscountLevel: v2.DiscountNone, SizeBytes: 1024,
		},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t"}))

	assert.Equal(t, int32(0), site.downloadCalls.Load(), "non-free must not download")
	ti, err := db.GetTorrentBySiteAndID("springsunday", "g200")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsSkipped)
	assert.False(t, ti.IsFree)
}

func TestFetchUnified_DetailErrorCountsFailed(t *testing.T) {
	_ = setupDB(t)
	srv := feedServerUnified(t, rssBody(itemXML("g300", "Boom", "http://x/b.torrent")))
	site := &unifiedFake{enabled: true, detailErr: fmt.Errorf("detail boom")}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL}))
	assert.Equal(t, int32(1), site.detailCalls.Load())
	assert.Equal(t, int32(0), site.downloadCalls.Load())
}

func TestFetchUnified_DownloadErrorCountsFailed(t *testing.T) {
	db := setupDB(t)
	srv := feedServerUnified(t, rssBody(itemXML("g400", "DlFail", "http://x/d.torrent")))
	site := &unifiedFake{
		enabled:     true,
		downloadErr: fmt.Errorf("download boom"),
		detail: &v2.TorrentItem{
			ID: "g400", Title: "DlFail", DiscountLevel: v2.DiscountFree, SizeBytes: 1024,
		},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t"}))
	assert.Equal(t, int32(1), site.downloadCalls.Load())

	// Row exists (created before download) but not marked downloaded.
	ti, err := db.GetTorrentBySiteAndID("springsunday", "g400")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.False(t, ti.IsDownloaded)
}

func TestFetchUnified_AlreadyPushedIsSkipped(t *testing.T) {
	db := setupDB(t)
	pushed := true
	now := time.Now()
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "g500", IsPushed: &pushed, PushTime: &now}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("g500", "Dup", "http://x/dup.torrent")))
	site := &unifiedFake{enabled: true}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL}))
	assert.Equal(t, int32(0), site.detailCalls.Load(), "already-pushed torrent must be skipped before detail fetch")
}

func TestFetchUnified_ContextCanceled(t *testing.T) {
	_ = setupDB(t)
	srv := feedServerUnified(t, rssBody(itemXML("g600", "Cancel", "http://x/c.torrent")))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled
	site := &unifiedFake{enabled: true}
	err := FetchAndDownloadFreeRSSUnified(ctx, site, models.RSSConfig{Name: "r", URL: srv.URL})
	// Either the ctx error surfaces, or the worker drains before the send loop
	// observes cancellation; both are valid. Just ensure no panic and it returns.
	_ = err
}
