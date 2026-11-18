package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// #542：站点设置里填了「H&R 上传量达标」时，种子下完、累计上传达到体积的这么多倍也算 H&R 达标。
func TestHRUploadMet(t *testing.T) {
	const size = 10 * gib
	cases := []struct {
		name     string
		ratio    float64
		size     int64
		uploaded int64
		progress float64
		want     bool
	}{
		{name: "没填比例", size: size, uploaded: 100 * gib, progress: 1},
		{name: "体积未知", ratio: 0.5, uploaded: 100 * gib, progress: 1},
		{name: "没下完", ratio: 0.5, size: size, uploaded: 100 * gib, progress: 0.99},
		{name: "刚好达标", ratio: 0.5, size: size, uploaded: 5 * gib, progress: 1, want: true},
		{name: "差一点", ratio: 0.5, size: size, uploaded: 5*gib - 1, progress: 1},
		{name: "超过", ratio: 0.9, size: size, uploaded: 12 * gib, progress: 1, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, hrUploadMet(tc.ratio, tc.size, tc.uploaded, tc.progress))
		})
	}
}

// hrGuard：做种时长没满时，填了上传量要求、体积已知、下完了、上传够了就不保护；保护原因写上上传量。
func TestHRGuard_UploadRatio(t *testing.T) {
	const size = 10 * gib
	both := hrInfo{HasHR: true, HRSeedTimeH: 72, UploadRatio: 0.9, SizeBytes: size}
	ratioOnly := hrInfo{HasHR: true, UploadRatio: 0.9, SizeBytes: size}
	cases := []struct {
		name string
		info hrInfo
		tor  downloader.Torrent
		want string // 空串是不保护
	}{
		{name: "时长不够但上传够", info: both, tor: downloader.Torrent{Progress: 1, SeedingTime: 10 * 3600, TotalUploaded: 9*gib + gib/2}},
		{
			name: "两样都不够", info: both, tor: downloader.Torrent{Progress: 1, SeedingTime: 10 * 3600, TotalUploaded: 3 * gib},
			want: "H&R：要求做种 72 小时或上传 0.9 倍体积，已做 10.0 小时、已传 0.3 倍",
		},
		{name: "做满时长", info: both, tor: downloader.Torrent{Progress: 1, SeedingTime: 72 * 3600}},
		{name: "时长未知但上传够", info: ratioOnly, tor: downloader.Torrent{Progress: 1, TotalUploaded: 10 * gib}},
		{
			name: "时长未知、上传不够", info: ratioOnly, tor: downloader.Torrent{Progress: 1, SeedingTime: 1 << 30, TotalUploaded: 3 * gib},
			want: "H&R：站点没有给出做种时长，要求上传 0.9 倍体积，已传 0.3 倍",
		},
		{
			name: "时长未知且没填比例", info: hrInfo{HasHR: true, SizeBytes: size}, tor: downloader.Torrent{Progress: 1, TotalUploaded: 100 * gib},
			want: "H&R：站点没有给出做种要求，一直保护（可以在站点设置里填「H&R 做种要求」）",
		},
		{
			name: "记录里没有体积：用下载器给的", info: hrInfo{HasHR: true, HRSeedTimeH: 72, UploadRatio: 0.9},
			tor: downloader.Torrent{Progress: 1, TotalSize: size, SeedingTime: 3600, TotalUploaded: 10 * gib},
		},
		{
			name: "体积都不知道：只按时长", info: hrInfo{HasHR: true, HRSeedTimeH: 72, UploadRatio: 0.9},
			tor:  downloader.Torrent{Progress: 1, SeedingTime: 3600, TotalUploaded: 100 * gib},
			want: "H&R：要求做种 72 小时，已做 1.0 小时",
		},
		{
			name: "没下完但上传够：仍保护", info: both, tor: downloader.Torrent{Progress: 0.5, TotalUploaded: 20 * gib},
			want: "H&R：要求做种 72 小时或上传 0.9 倍体积，已做 0.0 小时、已传 2 倍（还没下完，下完才按上传量算）",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tor := tc.tor
			tor.InfoHash = "H1"
			p := hrGuard(tor, map[string]hrInfo{"h1": tc.info})
			assert.Equal(t, tc.want, p.Text)
			if tc.want == "" {
				assert.Empty(t, p.Rule)
			} else {
				assert.Equal(t, GuardHR, p.Rule)
			}
		})
	}
}

// loadHRInfo 读出站点设置里的上传量要求与记录里的体积；同一个哈希有几条记录时取最严：
// 有一条的站点没填上传量要求就不按上传量放行，都填了取最大的倍数，体积取最大的。
func TestLoadHRInfo_UploadRatio(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	for _, s := range []map[string]any{
		{"name": "ra", "auth_method": "cookie", "enabled": true, "hr_upload_ratio": 0.9},
		{"name": "rb", "auth_method": "cookie", "enabled": true, "hr_upload_ratio": 1.5, "hr_seed_time_h": 24},
		{"name": "none", "auth_method": "cookie", "enabled": true, "hr_seed_time_h": 24},
	} {
		require.NoError(t, cm.db.Table("site_settings").Create(s).Error)
	}
	pushed := true
	add := func(site, id, hash string, size int64) {
		require.NoError(t, cm.db.Create(&models.TorrentInfo{
			SiteName: site, TorrentID: id, TorrentHash: &hash, IsPushed: &pushed, HasHR: true, TorrentSize: size,
		}).Error)
	}
	add("ra", "1", "honly", 10*gib) // 站点只填了上传量要求
	add("ra", "2", "hboth", 0)      // 两条的站点都填了：取最大的倍数，体积取有的那条
	add("rb", "3", "hboth", 8*gib)
	add("ra", "4", "hmixed", 10*gib) // 一条的站点没填：不按上传量放行
	add("none", "5", "hmixed", 10*gib)

	m, err := loadHRInfo(cm.db)
	require.NoError(t, err)
	assert.Equal(t, hrInfo{HasHR: true, UploadRatio: 0.9, SizeBytes: 10 * gib}, m["honly"])
	assert.Equal(t, hrInfo{HasHR: true, UploadRatio: 1.5, SizeBytes: 8 * gib}, m["hboth"], "ra 没有做种时长：合并后时长未知")
	assert.Equal(t, hrInfo{HasHR: true, SizeBytes: 10 * gib}, m["hmixed"])
}

// #542：自动删种按站点设置里的上传量要求判断：做种时长没满、下完以后上传够了的照常按规则删，上传不够的照样保护。
func TestCleanup_HRUploadRatioFromSiteSetting(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	require.NoError(t, cm.db.Table("site_settings").Create(map[string]any{
		"name": "hdsky", "auth_method": "cookie", "enabled": true, "hr_seed_time_h": 72, "hr_upload_ratio": 0.9,
	}).Error)
	cfg := baseCfg()
	cfg.CleanupProtectHR = true
	pushed := true
	for _, id := range []string{"enough", "short"} {
		hash := "h" + id
		require.NoError(t, cm.db.Create(&models.TorrentInfo{
			SiteName: "hdsky", TorrentID: id, TorrentHash: &hash, IsPushed: &pushed, HasHR: true, TorrentSize: 10 * gib,
		}).Error)
	}
	// seedingTorrent 不填进度：做种的种子都下完了
	enough := seedingTorrent("enough", "henough", "Enough", 10, 5)
	enough.Progress, enough.TotalUploaded = 1, 10*gib
	short := seedingTorrent("short", "hshort", "Short", 10, 5)
	short.Progress, short.TotalUploaded = 1, 3*gib

	protected, candidates := cm.splitProtected(cfg, []downloader.Torrent{enough, short})
	require.Len(t, protected, 1)
	assert.Equal(t, "short", protected[0].ID, "做种 10 小时、上传 0.3 倍：保护")
	require.Len(t, candidates, 1)
	assert.Equal(t, "enough", candidates[0].ID, "上传够了：照常按规则删")
}

// #542：站点设置里只填了「H&R 上传量达标」（没有做种时长）时，不排除 H&R 的刷流任务收体积已知的 H&R 种子，体积未知的不收。
func TestBrushMonitor_HRUploadRatioFromSiteSetting(t *testing.T) {
	r := newBrushRig(t)
	require.NoError(t, r.db.DB.Table("site_settings").Create(map[string]any{
		"name": "hdsky", "auth_method": "cookie", "enabled": true, "hr_upload_ratio": 0.9,
	}).Error)
	task := r.task(t, func(bt *models.BrushTask) { bt.ExcludeHR = false })
	r.addItem(t, "h1", func(i *v2.TorrentItem) { i.HasHR = true })
	r.addItem(t, "h2", func(i *v2.TorrentItem) { i.HasHR = true; i.SizeBytes = 0 })

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Eligible)
	assert.Equal(t, 1, res.Added)
	require.Len(t, res.Rejected, 1)
	assert.Equal(t, brushRejectHRNoSize, res.Rejected[0].Reason)
	var bt models.BrushTorrent
	require.NoError(t, r.db.DB.Where("task_id = ?", task.ID).First(&bt).Error)
	assert.True(t, bt.HasHR)
	assert.Equal(t, 0, bt.HRSeedTimeH, "做种时长照旧记（未知是 0）")
	assert.Equal(t, int64(4*gib), bt.SizeBytes)
}

// #542：刷流删种按删除时的站点设置判上传量：做种时长没满、下完以后上传够了的照常按规则删，上传不够的留着。
func TestBrushMonitor_RemovesHRByUploadRatio(t *testing.T) {
	r := newBrushRig(t)
	require.NoError(t, r.db.DB.Table("site_settings").Create(map[string]any{
		"name": "hdsky", "auth_method": "cookie", "enabled": true, "hr_seed_time_h": 72, "hr_upload_ratio": 0.9,
	}).Error)
	task := r.task(t, func(bt *models.BrushTask) { bt.RemoveRatio = 1; bt.Enabled = false })
	tag := models.BrushTaskTag(task.ID)
	repo := models.NewBrushRepository(r.db.DB)
	for _, hash := range []string{"enough", "short"} {
		require.NoError(t, repo.RecordAdded(&models.BrushTorrent{
			TaskID: task.ID, InfoHash: hash, SiteName: "hdsky", TorrentID: hash, Title: "T-" + hash,
			AddedAt: r.clock.Now().Add(-time.Hour), State: models.BrushTorrentActive, DownloaderID: 1,
			HasHR: true, HRSeedTimeH: 72, SizeBytes: 10 * gib,
		}, "2026-10-06"))
	}
	r.dl.torrents = []downloader.Torrent{
		{ID: "id-enough", InfoHash: "enough", Tags: tag, Progress: 1, SeedingTime: 3600, Ratio: 1.5, TotalUploaded: 15 * gib, TotalDownloaded: 10 * gib},
		{ID: "id-short", InfoHash: "short", Tags: tag, Progress: 1, SeedingTime: 3600, Ratio: 1.2, TotalUploaded: 8 * gib, TotalDownloaded: 6 * gib},
	}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"id-enough"}, r.dl.removedSingle, "上传 1.5 倍体积：按分享率删；上传 0.8 倍：H&R 没做满，不删")
	assert.Equal(t, 1, res.Removed)
}
