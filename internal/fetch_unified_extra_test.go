// MIT License
// Copyright (c) 2025 pt-tools

// Coverage for downloadWorkerUnified's notifier + filter-rule branches:
// RSS "all" notification fired on new item, and a filter-rule-associated RSS
// driving decision via filter.Decide (associated rules path).

package internal

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

type recordingNotifier struct {
	newItems      atomic.Int32
	filteredItems atomic.Int32
}

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

func boolPtrFW(b bool) *bool { return &b }
