package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 预览：按规则算出会删哪些与为什么，不删；真的删一轮以后记下结果，换监控也留着
func TestCleanupPreviewAndHistory(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	cm.hist = &CleanupHistory{}

	cfg := baseCfg()
	cfg.CleanupScope = "all"
	cfg.CleanupMaxSeedTimeH = 72
	cfg.CleanupMinRatio = 3
	cfg.CleanupProtectDL = false
	cfg.CleanupProtectHR = false
	old := seedingTorrent("del1", "HASHDELETE", "OldSeed", 100, 1.0)
	both := seedingTorrent("del2", "hashboth", "Both", 100, 5)
	keep := seedingTorrent("keep1", "hashkeep", "Young", 10, 1.0)
	fake.torrents = []downloader.Torrent{old, both, keep}

	got := cm.Preview(cfg)
	require.Len(t, got, 1)
	r := got[0]
	assert.Equal(t, "qb1", r.Downloader)
	assert.Equal(t, 3, r.Managed)
	assert.Equal(t, 3, r.Candidates)
	assert.Zero(t, r.Deleted)
	require.Len(t, r.Items, 2)
	assert.Equal(t, "OldSeed", r.Items[0].Name)
	assert.Equal(t, "hashdelete", r.Items[0].InfoHash)
	assert.Equal(t, []string{"做种超过 72 小时"}, r.Items[0].Reasons)
	assert.Equal(t, []string{"做种超过 72 小时", "分享率达到 3"}, r.Items[1].Reasons)
	assert.Empty(t, fake.removedBatch, "预览不删")
	last, _ := cm.hist.Snapshot()
	assert.Nil(t, last, "预览不算一轮")

	cm.runOnce(cfg)
	require.Len(t, fake.removedBatch, 1)
	last, _ = cm.hist.Snapshot()
	require.NotNil(t, last)
	require.Len(t, last.Results, 1)
	assert.Equal(t, 2, last.Results[0].Deleted)
	assert.Len(t, last.Results[0].Items, 2)

	// 连不上的下载器：写明没检查
	fake.healthy = false
	cm.runOnce(cfg)
	last, _ = cm.hist.Snapshot()
	assert.Contains(t, last.Results[0].Error, "连不上")
	assert.Contains(t, cm.Preview(cfg)[0].Error, "连不上")
}

// 条件之间是「且」时，设了的都对上才删，理由写全；免费期过了还没下完的直接删
func TestDeleteReasonsModes(t *testing.T) {
	cm := newCleanupMonitorWithFake(t, newSchedFakeDownloader("qb1"))
	cfg := baseCfg()
	cfg.CleanupMaxSeedTimeH = 10
	cfg.CleanupSlowSeedTimeH = 5
	cfg.CleanupSlowMaxRatio = 0.5
	cfg.CleanupConditionMode = "and"
	slow := seedingTorrent("a", "ha", "Slow", 20, 0.1)
	slow.IsCompleted = true
	assert.Equal(t, []string{"做种超过 10 小时", "做种 5 小时分享率还不到 0.5"}, cm.deleteReasons(cfg, slow))
	fast := seedingTorrent("b", "hb", "Fast", 20, 2)
	fast.IsCompleted = true
	assert.Empty(t, cm.deleteReasons(cfg, fast), "「且」时有一个没对上就不删")

	cfg.CleanupConditionMode = "or"
	cfg.CleanupMaxInactiveH = 12
	idle := seedingTorrent("c", "hc", "Idle", 20, 2)
	idle.UploadSpeed = 0
	assert.Contains(t, cm.deleteReasons(cfg, idle), "做种超过 12 小时、现在没有上传")
}

// 剩余空间低于下限：紧急清理多挑的写明原因；没打开连数据一起删时写明腾不出空间
func TestCleanupPreviewEmergency(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	cfg := baseCfg()
	cfg.CleanupScope = "all"
	cfg.CleanupProtectHR = false
	cfg.CleanupProtectDL = false
	cfg.CleanupDiskProtect = true
	cfg.CleanupMinDiskSpaceGB = 100
	cfg.CleanupRemoveData = true
	fake.diskInfo = downloader.DiskInfo{FreeSpace: 10 * oneGB}
	big := seedingTorrent("big", "hbig", "Big", 10, 1.0)
	big.TotalSize = 200 * oneGB
	fake.torrents = []downloader.Torrent{big}

	r := cm.Preview(cfg)[0]
	assert.True(t, r.Emergency)
	assert.InDelta(t, 10, r.FreeGB, 0.01)
	require.Len(t, r.Items, 1)
	assert.Contains(t, r.Items[0].Reasons[0], "紧急清理")

	cfg.CleanupRemoveData = false
	r = cm.Preview(cfg)[0]
	assert.Empty(t, r.Items)
	assert.Contains(t, r.Notes[0], "腾不出空间")
}

// 下一轮的时间：开着时记下，关掉时清空
func TestCleanupHistoryNext(t *testing.T) {
	h := &CleanupHistory{}
	at := time.Now().Add(time.Hour)
	h.setNext(at)
	_, next := h.Snapshot()
	assert.Equal(t, at, next)
	h.setNext(time.Time{})
	_, next = h.Snapshot()
	assert.True(t, next.IsZero())
	var nilHist *CleanupHistory
	nilHist.record(CleanupRun{})
	nilHist.setNext(at)
	run, next := nilHist.Snapshot()
	assert.Nil(t, run)
	assert.True(t, next.IsZero())
}
