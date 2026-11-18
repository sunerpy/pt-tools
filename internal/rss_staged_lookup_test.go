package internal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// 暂存文件按文件名（<站点>-<种子编号>.torrent）对应记录并核对哈希（docs/design/free-expiry-safety.md B.3.1）。

func diskProtectSettings(base string) models.SettingsGlobal {
	return models.SettingsGlobal{
		DownloadDir: base, DefaultIntervalMinutes: 10, DefaultEnabled: true, MaxRetry: 3,
		CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 10,
	}
}

// 同一个哈希还有一条别的入口留下、编号不同、不要求免费的记录（库里排在前面）：暂存文件按文件名对应 RSS 的严格记录，
// 磁盘拒绝 → 优惠结束 → 空间恢复后不加入下载器，也不接管那条记录。
func TestRSSFreeSafety_SameHashLooseRecordNotUsed(t *testing.T) {
	r := newRSSRig(t)
	require.NoError(t, core.NewConfigStore(r.db).SaveGlobalSettings(diskProtectSettings(r.base)))
	hash, err := qbit.ComputeTorrentHash(roundTorrentBytes("springsunday-s1"))
	require.NoError(t, err)
	loose := models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "manual-1", TorrentHash: &hash, Title: "manual",
		IsDownloaded: true, DownloadSource: "manual_push",
	}
	require.NoError(t, r.db.DB.Create(&loose).Error)

	feed := newFeedServer(t, "s1")
	rss := r.addRSS(feed, models.RSSConfig{Name: "free", Tag: "movie", IntervalMinutes: 10})[0]
	r.dls["qb"].setFree(5 * gb)
	r.site.set("s1", freeUntil(2*time.Hour))
	r.round(rss)
	strict := r.record("s1")
	require.True(t, strict.RequireFree)
	require.NotNil(t, strict.TorrentHash)
	require.Equal(t, hash, *strict.TorrentHash, "两条记录的哈希相同")
	require.Greater(t, strict.ID, loose.ID, "不要求免费的那条在库里排在前面")
	require.Empty(t, r.dls["qb"].addCalls())
	require.True(t, r.staged(rss, "s1"))

	feed.setItems()
	r.site.set("s1", v2.TorrentItem{DiscountLevel: v2.DiscountFree, DiscountEndTime: time.Now().Add(-time.Minute)})
	r.dls["qb"].setFree(1 << 50)
	r.round(rss)

	assert.Empty(t, r.dls["qb"].addCalls(), "优惠结束后不能按另一条不要求免费的记录加入")
	assert.True(t, r.record("s1").IsExpired)
	assert.False(t, r.staged(rss, "s1"))
	other := r.record("manual-1")
	assert.Nil(t, other.SourceRSSID, "别的入口的记录不被 RSS 接管")
	assert.False(t, pushedOf(other))
}

// 暂存文件的哈希和文件名对应的记录对不上（站点更新过种子文件等）：不推这个文件、删掉它，下一轮重新下载后照常推送。
func TestRSSFreeSafety_StagedHashMismatchRedownloads(t *testing.T) {
	r := newRSSRig(t)
	require.NoError(t, core.NewConfigStore(r.db).SaveGlobalSettings(diskProtectSettings(r.base)))
	feed := newFeedServer(t, "x1")
	rss := r.addRSS(feed, models.RSSConfig{Name: "free", Tag: "movie", IntervalMinutes: 10})[0]
	r.dls["qb"].setFree(5 * gb)
	r.site.set("x1", freeUntil(48*time.Hour))
	r.round(rss)
	require.True(t, r.staged(rss, "x1"))
	require.NoError(t, r.db.DB.Model(&models.TorrentInfo{}).Where("torrent_id = ?", "x1").
		Update("torrent_hash", "0123456789abcdef0123456789abcdef01234567").Error)

	r.dls["qb"].setFree(1 << 50)
	r.push(WithRSSAttempt(context.Background(), time.Now()), rss)
	assert.Empty(t, r.dls["qb"].addCalls(), "文件和记录对不上时不推")
	assert.False(t, r.staged(rss, "x1"), "删掉对不上的暂存文件")

	r.round(rss)
	assert.Len(t, r.dls["qb"].addCalls(), 1, "重新下载后照常推送")
	assert.True(t, pushedOf(r.record("x1")))
}
