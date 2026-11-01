package scheduler

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
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

	got := cm.Preview(t.Context(), cfg)
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
	assert.Contains(t, cm.Preview(t.Context(), cfg)[0].Error, "连不上")
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
	assert.Equal(t, []string{"做种超过 10 小时", "做种 5 小时分享率还不到 0.5"}, cm.deleteReasons(cfg, slow, "qb1"))
	fast := seedingTorrent("b", "hb", "Fast", 20, 2)
	fast.IsCompleted = true
	assert.Empty(t, cm.deleteReasons(cfg, fast, "qb1"), "「且」时有一个没对上就不删")

	cfg.CleanupConditionMode = "or"
	cfg.CleanupMaxInactiveH = 12
	idle := seedingTorrent("c", "hc", "Idle", 20, 2)
	idle.UploadSpeed = 0
	assert.Contains(t, cm.deleteReasons(cfg, idle, "qb1"), "做种中，最近 12 小时没有上传或下载")
}

// 预览列出受保护、这一轮不删的种子与原因（命中的第一条保护规则），写明下载器里一共几个种子，
// 以及刷流任务的、竞争度监控暂停着的各几个（常规规则不管它们）：用户能看出为什么没删。
func TestCleanupPreviewProtectedReasons(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	cfg := baseCfg()
	cfg.CleanupScope = "tag"
	cfg.CleanupScopeTags = "pt"
	cfg.CleanupMinRetainH = 24
	cfg.CleanupProtectTags = "keep"
	cfg.CleanupMaxSeedTimeH = 1
	pushed := true
	record := func(hash string, info models.TorrentInfo) {
		info.SiteName, info.TorrentID, info.TorrentHash, info.IsPushed = "qa", hash, &hash, &pushed
		require.NoError(t, cm.db.Create(&info).Error)
	}
	tagged := func(id, name, tags string) downloader.Torrent {
		tor := seedingTorrent(id, "h"+id, name, 30, 0.1)
		tor.Tags = tags
		return tor
	}
	young := tagged("young", "Young", "pt")
	young.DateAdded = time.Now().Add(-4 * time.Hour).Unix()
	record("hhr", models.TorrentInfo{HasHR: true, HRSeedTimeH: 72})
	hr := tagged("hr", "HR", "pt")
	hr.SeedingTime = 10 * 3600
	record("hunk", models.TorrentInfo{HasHR: true})
	unknown := tagged("unk", "Unknown", "pt")
	kept := tagged("kept", "Kept", "pt,keep")
	downloading := tagged("dl", "Downloading", "pt")
	downloading.State = downloader.TorrentDownloading
	brush := tagged("brush", "Brush", "pt,"+models.BrushTagAll)
	record("hpeer", models.TorrentInfo{IsPausedBySystem: true, PauseReason: PauseReasonPeerRatio})
	peer := tagged("peer", "Peer", "pt")
	del := tagged("del", "Delete", "pt")
	outside := seedingTorrent("out", "hout", "Outside", 30, 0.1)
	fake.torrents = []downloader.Torrent{young, hr, unknown, kept, downloading, brush, peer, del, outside}

	r := cm.Preview(t.Context(), cfg)[0]
	assert.Equal(t, 9, r.Total, "下载器里一共几个（管理范围外的也算）")
	assert.Equal(t, 8, r.Managed)
	assert.Equal(t, 5, r.Protected)
	assert.Equal(t, 1, r.Brush, "刷流任务的种子由刷流规则管")
	assert.Equal(t, 1, r.PeerPaused, "竞争度监控暂停着的不删")
	why := map[string]string{}
	for _, it := range r.ProtectedItems {
		why[it.Name] = strings.Join(it.Reasons, "；")
	}
	assert.Equal(t, "加进下载器不到 24 小时（最短保种时间），还差 20.0 小时", why["Young"])
	assert.Equal(t, "H&R：要求做种 72 小时，已做 10.0 小时", why["HR"])
	assert.Equal(t, "H&R：站点没有给出做种要求，一直保护（可以在站点设置里填「H&R 做种要求」）", why["Unknown"])
	assert.Equal(t, "带保护标签「keep」", why["Kept"])
	assert.Equal(t, "正在下载或校验（打开了「保护下载中的种子」）", why["Downloading"])
	require.Len(t, r.Items, 1)
	assert.Equal(t, "Delete", r.Items[0].Name)
}

// 连不上的下载器的结果也带空的列表（不是 null）：页面按列表算个数。
func TestCleanupPreviewUnreachableHasEmptyLists(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	fake.healthy = false
	r := cm.Preview(t.Context(), baseCfg())[0]
	assert.NotEmpty(t, r.Error)
	assert.NotNil(t, r.Items)
	assert.NotNil(t, r.ProtectedItems)
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

	r := cm.Preview(t.Context(), cfg)[0]
	assert.True(t, r.Emergency)
	assert.InDelta(t, 10, r.FreeGB, 0.01)
	require.Len(t, r.Items, 1)
	assert.Contains(t, r.Items[0].Reasons[0], "紧急清理")

	cfg.CleanupRemoveData = false
	r = cm.Preview(t.Context(), cfg)[0]
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
