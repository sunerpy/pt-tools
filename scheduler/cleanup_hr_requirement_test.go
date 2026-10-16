package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// #727：有 H&R、要求的做种时长未知（站点定义没有、站点设置也没填）的种子，自动删种保护、不删（宁可不删，免得被站点记 H&R）。
func TestCleanup_HRUnknownRequirementProtected(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupProtectHR = true
	hash := "hrunknown"
	pushed := true
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "u1", TorrentHash: &hash, IsPushed: &pushed, HasHR: true, HRSeedTimeH: 0,
	}).Error)
	tor := seedingTorrent("u1", hash, "Unknown", 100, 5)

	protected, candidates := cm.splitProtected(cfg, []downloader.Torrent{tor})
	assert.Len(t, protected, 1, "要求未知：保护")
	assert.Empty(t, candidates)
}

// #727：站点设置里填了「H&R 做种要求」：没做满保护，做满了照常按规则删。
func TestCleanup_HRRequirementFromSiteSetting(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	require.NoError(t, cm.db.Table("site_settings").Create(map[string]any{
		"name": "hdsky", "auth_method": "cookie", "enabled": true, "hr_seed_time_h": 72,
	}).Error)
	cfg := baseCfg()
	cfg.CleanupProtectHR = true
	pushed := true
	for _, id := range []string{"young", "done"} {
		hash := "h" + id
		require.NoError(t, cm.db.Create(&models.TorrentInfo{
			SiteName: "hdsky", TorrentID: id, TorrentHash: &hash, IsPushed: &pushed, HasHR: true,
		}).Error)
	}
	young := seedingTorrent("young", "hyoung", "Young", 10, 5)
	done := seedingTorrent("done", "hdone", "Done", 80, 5)

	protected, candidates := cm.splitProtected(cfg, []downloader.Torrent{young, done})
	require.Len(t, protected, 1)
	assert.Equal(t, "young", protected[0].ID, "做种 10 小时，没满 72 小时：保护")
	require.Len(t, candidates, 1)
	assert.Equal(t, "done", candidates[0].ID, "做满了：照常按规则删")
}

// 同一个哈希有几条有 H&R 的记录（跨站辅种等）：取最严的——有一条要求未知就按未知保护，否则取最长的要求。
func TestCleanup_HRMergesStrictestPerHash(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupProtectHR = true
	pushed := true
	add := func(site, id, hash string, req int) {
		require.NoError(t, cm.db.Create(&models.TorrentInfo{
			SiteName: site, TorrentID: id, TorrentHash: &hash, IsPushed: &pushed, HasHR: true, HRSeedTimeH: req,
		}).Error)
	}
	add("a", "1", "hunknown", 0) // 要求未知的先写
	add("b", "2", "hunknown", 24)
	add("a", "3", "hlong", 720)
	add("b", "4", "hlong", 24)
	unknown := seedingTorrent("u", "hunknown", "U", 30, 5)
	long := seedingTorrent("l", "hlong", "L", 30, 5)

	protected, candidates := cm.splitProtected(cfg, []downloader.Torrent{unknown, long})
	assert.Len(t, protected, 2, "做种 30 小时：一条要求未知、一条要求 720 小时，都没满")
	assert.Empty(t, candidates)
}

// 记录归档以后 H&R 保护照样在：归档表里的 H&R 记录也算。
func TestCleanup_HRFromArchivedRecord(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupProtectHR = true
	pushed := true
	hash := "harchived"
	require.NoError(t, cm.db.Create(&models.TorrentInfoArchive{
		SiteName: "a", TorrentID: "9", TorrentHash: &hash, IsPushed: &pushed, HasHR: true, HRSeedTimeH: 720,
	}).Error)
	tor := seedingTorrent("x", hash, "X", 400, 5)

	protected, _ := cm.splitProtected(cfg, []downloader.Torrent{tor})
	assert.Len(t, protected, 1, "归档的记录要求 720 小时，做种 400 小时：保护")
}
