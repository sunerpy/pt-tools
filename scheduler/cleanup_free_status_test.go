package scheduler

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// 「免费到期未完成删除」按 RSS 最近一次读到的免费资格判断（docs/design/free-expiry-safety.md B.3.3）：
// 已推送的种子每轮刷新资格，免费延长的等新的结束时间，取消或提前结束的按记下的免费结束时间（free_ended_at）删。
func TestCleanup_FreeExpiredFollowsRefreshedStatus(t *testing.T) {
	cm := newCleanupMonitorWithFake(t, newSchedFakeDownloader("qb1"))
	cfg := baseCfg()
	cfg.CleanupDelFreeExpired = true
	now := time.Now()
	past, future := now.Add(-time.Minute), now.Add(3*time.Hour)
	pushed := true
	rows := []struct {
		hash string
		cols map[string]any
		want bool
		why  string
	}{
		{"hcancelled", map[string]any{"is_free": false, "free_validity": "not_free", "free_level": "NONE", "free_end_time": nil, "free_ended_at": past}, true, "免费取消：按记下的免费结束时间删"},
		{"hextended", map[string]any{"is_free": true, "free_validity": "free_until", "free_end_time": future}, false, "免费延长：等新的结束时间"},
		{"hexpired", map[string]any{"is_free": true, "free_validity": "free_until", "free_end_time": past}, true, "结束时间已过"},
		{"hlegacyhalf", map[string]any{"is_free": false, "free_end_time": past}, false, "旧记录里半价的结束时间不算免费到期"},
	}
	for i, r := range rows {
		ti := models.TorrentInfo{SiteName: "springsunday", TorrentID: fmt.Sprintf("c%d", i), TorrentHash: &r.hash, IsPushed: &pushed}
		require.NoError(t, cm.db.Create(&ti).Error)
		require.NoError(t, cm.db.Model(&models.TorrentInfo{}).Where("id = ?", ti.ID).Updates(r.cols).Error)
	}
	for _, r := range rows {
		tor := seedingTorrent(r.hash, r.hash, r.hash, 1, 0)
		tor.Progress = 0.5
		got := cm.deleteReasons(cfg, tor)
		if r.want {
			assert.Contains(t, got, "免费期过了还没下完", r.why)
		} else {
			assert.NotContains(t, got, "免费期过了还没下完", r.why)
		}
	}
}
