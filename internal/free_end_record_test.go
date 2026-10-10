// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// subWithAnyRule 建一个 RSS 订阅并关联一条不要求免费、匹配一切的下载规则（过滤规则通道会下载半价种子）。
func subWithAnyRule(t *testing.T, db *models.TorrentDB) models.RSSSubscription {
	t.Helper()
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "cookie", Cookie: "c=1",
		RSS: []models.RSSConfig{{Name: "sub", URL: "http://placeholder", IntervalMinutes: 1, Tag: "movie"}},
	}))
	var sub models.RSSSubscription
	require.NoError(t, db.DB.First(&sub).Error)
	rule := models.FilterRule{Name: "any", Pattern: ".*", PatternType: "regex", MatchField: "both", Enabled: true, Priority: 100, Purpose: "download"}
	require.NoError(t, db.DB.Create(&rule).Error)
	require.NoError(t, db.DB.Model(&models.FilterRule{}).Where("id = ?", rule.ID).Update("require_free", false).Error)
	require.NoError(t, models.NewRSSFilterAssociationDB(db.DB).SetFilterRulesForRSS(sub.ID, []uint{rule.ID}))
	return sub
}

// 半价、2X 这类优惠也有结束时间：只有免费的才记成免费到期，不然「删除免费到期未完成的种子」会删掉没下完的半价种子。
func TestFetchUnified_FreeEndOnlyForFree(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	sub := subWithAnyRule(t, db)
	end := time.Now().Add(3 * time.Hour)

	for _, tc := range []struct {
		guid     string
		level    v2.DiscountLevel
		wantFree bool
	}{
		{"uhalf", v2.DiscountPercent50, false},
		{"u2xup", v2.Discount2xUp, false},
		{"ufree", v2.DiscountFree, true},
		{"u2xfree", v2.Discount2xFree, true},
	} {
		srv := feedServerUnified(t, rssBody(itemXML(tc.guid, "T "+tc.guid, "http://x/"+tc.guid+".torrent")))
		site := &unifiedFake{
			enabled: true, writeFile: true,
			detail: &v2.TorrentItem{ID: tc.guid, Title: "T " + tc.guid, DiscountLevel: tc.level, DiscountEndTime: end, SizeBytes: 2048},
		}
		require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
			models.RSSConfig{ID: sub.ID, Name: sub.Name, URL: srv.URL, Tag: "movie", PauseOnFreeEnd: true}))

		ti, err := db.GetTorrentBySiteAndID("springsunday", tc.guid)
		require.NoError(t, err)
		require.NotNil(t, ti, tc.guid)
		assert.Equal(t, string(tc.level), ti.FreeLevel, tc.guid)
		assert.Equal(t, tc.wantFree, ti.IsFree, tc.guid)
		if tc.wantFree {
			require.NotNil(t, ti.FreeEndTime, tc.guid)
			assert.WithinDuration(t, end, *ti.FreeEndTime, time.Second, tc.guid)
		} else {
			assert.Nil(t, ti.FreeEndTime, "%s: a non-free discount must not be recorded as the free end", tc.guid)
		}
	}
}

func TestFetchLegacy_FreeEndOnlyForFree(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	sub := subWithAnyRule(t, db)

	for _, tc := range []struct {
		guid     string
		discount models.DiscountType
		wantFree bool
	}{
		{"lhalf", models.DISCOUNT_FIFTY, false},
		{"l2x", models.DISCOUNT_TWO_X, false},
		{"lfree", models.DISCOUNT_FREE, true},
	} {
		srv := feedServerUnified(t, rssBody(itemXML(tc.guid, "L "+tc.guid, "http://x/"+tc.guid+".torrent")))
		site := &legacyPTStub{enabled: true, discount: tc.discount, sizeMB: 1, writeFile: true}
		require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
			models.RSSConfig{ID: sub.ID, Name: sub.Name, URL: srv.URL, Tag: "movie", PauseOnFreeEnd: true}))

		ti, err := db.GetTorrentBySiteAndID("springsunday", tc.guid)
		require.NoError(t, err)
		require.NotNil(t, ti, tc.guid)
		assert.Equal(t, tc.wantFree, ti.IsFree, tc.guid)
		if tc.wantFree {
			assert.NotNil(t, ti.FreeEndTime, tc.guid)
		} else {
			assert.Nil(t, ti.FreeEndTime, "%s: a non-free discount must not be recorded as the free end", tc.guid)
		}
	}
}
