package internal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// #727：站点定义里没有 H&R 做种时长时，RSS 记录按站点设置里填的「H&R 做种要求」记（自动删种按它保护）。
func TestRSS_HRSeedTimeFromSiteSetting(t *testing.T) {
	r := newRSSRig(t)
	feed := newFeedServer(t, "r1")
	rss := r.addRSS(feed, models.RSSConfig{Name: "free", Tag: "movie", IntervalMinutes: 10})[0]
	require.NoError(t, r.db.DB.Table("site_settings").Where("name = ?", "springsunday").Update("hr_seed_time_h", 48).Error)
	item := freeUntil(48 * time.Hour)
	item.HasHR = true
	r.site.set("r1", item)
	r.round(rss)

	ti := r.record("r1")
	assert.True(t, ti.HasHR)
	assert.Equal(t, 48, ti.HRSeedTimeH)
}
