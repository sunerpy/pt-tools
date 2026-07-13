// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func itemXMLWithCategory(guid, title, enclosure, category string) string {
	return fmt.Sprintf(`<item><title>%s</title><guid>%s</guid><category>%s</category>`+
		`<enclosure url="%s" type="application/x-bittorrent"/></item>`, title, guid, category, enclosure)
}

func itemXMLLinkOnly(guid, title, link string) string {
	return fmt.Sprintf(`<item><title>%s</title><guid>%s</guid><link>%s</link></item>`, title, guid, link)
}

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
