// MIT License
// Copyright (c) 2025 pt-tools

// Unit coverage for internal helper functions that don't need a live tracker:
// buildSkipReason, calcHRSeedTimeForTorrent, extractTorrentRef, the enter.go
// registration setters, and the site-capacity DB lookup.

package internal

import (
	"context"
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestBuildSkipReason(t *testing.T) {
	cases := []struct {
		name    string
		isFree  bool
		canFin  bool
		byFilt  bool
		wantSub string
	}{
		{"not free", false, true, true, "非免费"},
		{"cannot finish", true, false, true, "免费期内无法完成"},
		{"filter no match", true, true, false, "未匹配过滤规则"},
		{"all ok -> unknown", true, true, true, "未知原因"},
		{"multi", false, true, false, "非免费, 未匹配过滤规则"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildSkipReason(c.isFree, c.canFin, c.byFilt)
			assert.Equal(t, c.wantSub, got)
		})
	}
}

func TestCalcHRSeedTimeForTorrent(t *testing.T) {
	// nil def -> fallback.
	assert.Equal(t, 72, calcHRSeedTimeForTorrent(nil, 72, 1<<30))

	// def with no rules -> fallback.
	defFlat := &v2.SiteDefinition{HREnabled: true, HRSeedTimeHours: 48}
	assert.Equal(t, 24, calcHRSeedTimeForTorrent(defFlat, 24, 1<<30))

	// def with size-tiered rules -> picks matching tier.
	defRules := &v2.SiteDefinition{
		HREnabled:       true,
		HRSeedTimeHours: 10,
		HRSeedTimeRules: []v2.HRSeedTimeRule{
			{MinSizeGB: 0, MaxSizeGB: 50, SeedTimeH: 100},
			{MinSizeGB: 50, MaxSizeGB: 0, SeedTimeH: 200},
		},
	}
	// 10 GiB -> tier 1 (100h)
	assert.Equal(t, 100, calcHRSeedTimeForTorrent(defRules, 10, 10*(1<<30)))
	// 80 GiB -> tier 2 (200h)
	assert.Equal(t, 200, calcHRSeedTimeForTorrent(defRules, 10, 80*(1<<30)))
}

func TestExtractTorrentRef(t *testing.T) {
	cases := []struct {
		name     string
		item     *gofeed.Item
		wantSite string
		wantID   string
	}{
		{"nil item", nil, "", ""},
		{"empty link", &gofeed.Item{Link: ""}, "", ""},
		{"id query", &gofeed.Item{Link: "https://pt.example.com/details.php?id=1234"}, "pt.example.com", "1234"},
		{"numeric path", &gofeed.Item{Link: "https://pt.example.com/torrents/5678"}, "pt.example.com", "5678"},
		{"no numeric", &gofeed.Item{Link: "https://pt.example.com/browse"}, "pt.example.com", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			site, id := extractTorrentRef(c.item)
			assert.Equal(t, c.wantSite, site)
			assert.Equal(t, c.wantID, id)
		})
	}
}

func TestScheduleTorrentForMonitoring(t *testing.T) {
	// No scheduler registered -> no-op, no panic.
	ScheduleTorrentForMonitoring(models.TorrentInfo{TorrentID: "x"})

	var got string
	RegisterTorrentScheduler(func(ti models.TorrentInfo) { got = ti.TorrentID })
	t.Cleanup(func() { RegisterTorrentScheduler(nil) })

	ScheduleTorrentForMonitoring(models.TorrentInfo{TorrentID: "sched-1"})
	assert.Equal(t, "sched-1", got)
}

type stubRSSNotifier struct{}

func (stubRSSNotifier) NotifyNewItem(_ context.Context, _ RSSItemNotice) error { return nil }

func (stubRSSNotifier) NotifyFilteredItem(_ context.Context, _ RSSFilteredNotice) error { return nil }

func TestSetAndGetRSSNotifier(t *testing.T) {
	// initially unset within a fresh binary is not guaranteed; set then read.
	SetRSSNotifier(nil)
	assert.Nil(t, getRSSNotifier())

	n := stubRSSNotifier{}
	SetRSSNotifier(n)
	t.Cleanup(func() { SetRSSNotifier(nil) })
	assert.NotNil(t, getRSSNotifier())
}

func TestSiteSeedingCapacityGB(t *testing.T) {
	// empty site name -> 0
	assert.Equal(t, float64(0), siteSeedingCapacityGB(""))

	// DB nil -> 0
	global.GlobalDB = nil
	assert.Equal(t, float64(0), siteSeedingCapacityGB("mteam"))

	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	// unknown site -> 0
	assert.Equal(t, float64(0), siteSeedingCapacityGB("nosuchsite"))

	// known site with capacity
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "mteam", SeedingCapacityGB: 500}).Error)
	assert.Equal(t, float64(500), siteSeedingCapacityGB("mteam"))
}
