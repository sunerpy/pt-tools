package internal

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 共享推送里的门禁与先暂停加入（docs/design/free-expiry-safety.md 附录 B.2.5–B.2.7、B.2.15）。

// stepClock 是每读一次就前进 step 的时钟（模拟等锁、读空间花掉的时间）。
type stepClock struct {
	mu   sync.Mutex
	now  time.Time
	step time.Duration
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.now
	c.now = c.now.Add(c.step)
	return t
}

func pushRig(t *testing.T, names ...string) *rssRig {
	t.Helper()
	r := newRSSRig(t, names...)
	t.Cleanup(SwapPushDownloaderFactory(func(s models.DownloaderSetting) (downloader.Downloader, error) { return r.dls[s.Name], nil }))
	return r
}

func untilSnap(verified, end time.Time) *v2.PromotionSnapshot {
	return &v2.PromotionSnapshot{Level: v2.DiscountFree, Validity: v2.FreeUntil, EndsAt: &end, VerifiedAt: verified}
}

// 磁盘检查以后等待跨过了免费边界：归还预留、拒绝，下载器 Add 调用为 0。
func TestPush_RecheckAfterGatesRejectsAndReleases(t *testing.T) {
	r := pushRig(t)
	require.NoError(t, core.NewConfigStore(r.db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: r.base, DefaultIntervalMinutes: 10, CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 10,
	}))
	start := time.Now()
	// 第一次判定时剩 6 分钟（> 5 分钟余量），每读一次时钟前进 2 分钟：等到真正加入前只剩 4 分钟
	clock := &stepClock{now: start, step: 2 * time.Minute}
	end := start.Add(6 * time.Minute)
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "rc1", TorrentData: roundTorrentBytes("rc1"), DownloaderID: r.dlID("qb"),
		Policy:   AdmissionPolicy{RequireFree: true, SafetyMargin: 5 * time.Minute, SourceKind: SourceKindManual, AttemptStart: start, Clock: clock.Now},
		Snapshot: untilSnap(start, end),
	})
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.Equal(t, PushStatusRejectedExpired, res.Status)
	assert.Empty(t, r.dls["qb"].addCalls(), "跨过边界以后不加入")
	assert.Zero(t, GetDiskBudget().Reserved(), "归还了磁盘预留")
}

// 承诺到期暂停、结束时间可信：先暂停加入、登记监控，记账成功后才开始；记账失败时保持暂停并如实返回。
func TestPush_PausePromisedBookkeeping(t *testing.T) {
	t.Run("记账成功后开始", func(t *testing.T) {
		r := pushRig(t)
		var scheduled []string
		RegisterTorrentScheduler(func(ti models.TorrentInfo) { scheduled = append(scheduled, ti.TorrentID) })
		t.Cleanup(func() { RegisterTorrentScheduler(nil) })
		now := time.Now()
		res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
			SiteID: "springsunday", TorrentID: "pp1", TorrentData: roundTorrentBytes("pp1"), DownloaderID: r.dlID("qb"),
			Policy:   AdmissionPolicy{PauseOnFreeEnd: true, SourceKind: SourceKindManual, AttemptStart: now},
			Snapshot: untilSnap(now, now.Add(3*time.Hour)),
		})
		require.NoError(t, err)
		assert.Equal(t, PushStatusPushed, res.Status)
		adds := r.dls["qb"].addCalls()
		require.Len(t, adds, 1)
		assert.True(t, adds[0].AddAtPaused)
		assert.Len(t, r.dls["qb"].resumeCalls(), 1)
		assert.Equal(t, []string{"pp1"}, scheduled)
		ti := r.record("pp1")
		assert.Nil(t, ti.PushPendingAt, "记账成功清掉推送标记")
		require.NotNil(t, ti.PausePlannedAt)
	})

	t.Run("记账失败保持暂停", func(t *testing.T) {
		r := pushRig(t)
		failBookkeeping(t, r.db.DB)
		now := time.Now()
		res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
			SiteID: "springsunday", TorrentID: "pp2", TorrentData: roundTorrentBytes("pp2"), DownloaderID: r.dlID("qb"),
			Policy:   AdmissionPolicy{PauseOnFreeEnd: true, SourceKind: SourceKindManual, AttemptStart: now},
			Snapshot: untilSnap(now, now.Add(3*time.Hour)),
		})
		require.NoError(t, err)
		assert.True(t, res.Success, "已经加进下载器，不能报推送失败")
		assert.Equal(t, PushStatusMonitorDegraded, res.Status)
		assert.Contains(t, res.Message, "保持暂停")
		assert.Empty(t, r.dls["qb"].resumeCalls(), "记账没写成就不开始")
		assert.NotNil(t, r.record("pp2").PushPendingAt, "推送标记留着，对账按哈希补记账")
	})
}

// failBookkeeping 让加入后的记账 UPDATE 失败（写任务编号的那一条）。
func failBookkeeping(t *testing.T, db *gorm.DB) {
	t.Helper()
	const name = "test:fail_bookkeeping"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if cols, ok := tx.Statement.Dest.(map[string]any); ok {
			if _, has := cols["downloader_task_id"]; has {
				_ = tx.AddError(errors.New("disk I/O error"))
			}
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(name) })
}

// 同一个种子正在另一个下载器里受到期监控：不再推到第二个下载器（按哈希也认得出）。
func TestPush_RejectsSecondMonitoredPlacement(t *testing.T) {
	r := pushRig(t, "qb-a", "qb-b")
	idA, idB := r.dlID("qb-a"), r.dlID("qb-b")
	data := roundTorrentBytes("mp1")
	now := time.Now()
	_, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "guid-mp1", TorrentData: data, DownloaderID: idA,
		Policy:   AdmissionPolicy{PauseOnFreeEnd: true, SourceKind: SourceKindRSS, AttemptStart: now},
		Snapshot: untilSnap(now, now.Add(3*time.Hour)),
	})
	require.NoError(t, err)
	require.Len(t, r.dls["qb-a"].addCalls(), 1)

	// 通知按钮按详情编号推同一个种子到另一个下载器
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "77", TorrentData: data, DownloaderID: idB,
		Policy: AdmissionPolicy{SourceKind: SourceKindRSSNotify},
	})
	require.NoError(t, err)
	assert.Equal(t, PushStatusRejectedPlacement, res.Status)
	assert.Empty(t, r.dls["qb-b"].addCalls())
	ti := r.record("guid-mp1")
	require.NotNil(t, ti.DownloaderID)
	assert.Equal(t, idA, *ti.DownloaderID, "原来的下载器与任务编号不被覆盖")
}

// RSS 暂存推送发现下载器里已有：是上次暂停加入、记账没写成的（记录上留着推送标记），补记账后按设置开始；
// 下载器里原本就有的只记账，不改它的运行状态。
func TestPush_AdoptExisting(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pending bool
		resumes int
	}{
		{"上次加入、记账没写成", true, 1},
		{"下载器里原本就有", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := pushRig(t)
			dlID := r.dlID("qb")
			data := roundTorrentBytes("ad-" + tc.name)
			res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
				SiteID: "springsunday", TorrentID: "ad", TorrentData: data, DownloaderID: dlID,
			})
			require.NoError(t, err)
			require.True(t, res.Success)
			cols := map[string]any{"is_pushed": false, "downloader_task_id": ""}
			if tc.pending {
				cols["push_pending_at"] = time.Now().Add(-time.Minute)
			}
			require.NoError(t, global.GlobalDB.DB.Model(&models.TorrentInfo{}).Where("torrent_id = ?", "ad").Updates(cols).Error)

			now := time.Now()
			info := &DownloaderInfo{ID: dlID, Name: "qb", AutoStart: true}
			res, err = pushTorrent(context.Background(), pushTarget{ID: info.ID, Name: info.Name, AutoStart: true},
				func() (downloader.Downloader, error) { return r.dls["qb"], nil },
				PushTorrentRequest{
					SiteID: "springsunday", TorrentID: "ad", TorrentData: data, DownloaderID: dlID,
					Policy:        AdmissionPolicy{PauseOnFreeEnd: true, SourceKind: SourceKindRSS, AttemptStart: now},
					Snapshot:      untilSnap(now, now.Add(3*time.Hour)),
					AdoptExisting: true, KeepRecord: true,
				})
			require.NoError(t, err)
			assert.Equal(t, PushStatusDuplicate, res.Status)
			assert.Len(t, r.dls["qb"].addCalls(), 1, "没有再加一次")
			assert.Len(t, r.dls["qb"].resumeCalls(), tc.resumes)
			ti := r.record("ad")
			assert.True(t, pushedOf(ti))
			assert.Nil(t, ti.PushPendingAt)
			assert.True(t, ti.PauseOnFreeEnd)
		})
	}
}

// 预检查没查到、加入时下载器才说已经有了：RSS 暂存推送照样记账、登记监控，不改那个已有任务的运行状态。
func TestPush_AddTimeDuplicateIsAdopted(t *testing.T) {
	r := pushRig(t)
	dlID := r.dlID("qb")
	data := roundTorrentBytes("dup-at-add")
	h := hashOfBytes(t, data)
	r.dls["qb"].dupOnAdd = map[string]bool{h: true}
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{SiteName: "springsunday", TorrentID: "dup", TorrentHash: &h}).Error)
	var scheduled []string
	RegisterTorrentScheduler(func(ti models.TorrentInfo) { scheduled = append(scheduled, ti.TorrentID) })
	t.Cleanup(func() { RegisterTorrentScheduler(nil) })

	now := time.Now()
	res, err := pushTorrent(context.Background(), pushTarget{ID: dlID, Name: "qb", AutoStart: true},
		func() (downloader.Downloader, error) { return r.dls["qb"], nil },
		PushTorrentRequest{
			SiteID: "springsunday", TorrentID: "dup", TorrentData: data, DownloaderID: dlID,
			Policy:        AdmissionPolicy{PauseOnFreeEnd: true, SourceKind: SourceKindRSS, AttemptStart: now},
			Snapshot:      untilSnap(now, now.Add(3*time.Hour)),
			AdoptExisting: true, KeepRecord: true,
		})
	require.NoError(t, err)
	assert.Equal(t, PushStatusDuplicate, res.Status)
	ti := r.record("dup")
	assert.True(t, pushedOf(ti), "记账了")
	assert.Equal(t, h, ti.DownloaderTaskID)
	assert.Nil(t, ti.PushPendingAt)
	assert.Equal(t, []string{"dup"}, scheduled, "登记了到期监控")
	assert.Empty(t, r.dls["qb"].resumeCalls(), "不是这次加入的，不改它的运行状态")
}

// blockingDL 在加入时停住，直到放行（用来让两次推送重叠）。
type blockingDL struct {
	*fakeDL
	entered chan struct{}
	release chan struct{}
}

func (b *blockingDL) AddTorrentFileEx(data []byte, opt downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
	close(b.entered)
	<-b.release
	return b.fakeDL.AddTorrentFileEx(data, opt)
}

// 同一个种子同时推到两个下载器（都承诺到期暂停）：第二次等第一次记完账再判，被拒绝，不会两边各加一份。
func TestPush_ConcurrentPlacementsSerialized(t *testing.T) {
	r := newRSSRig(t, "qb-a", "qb-b")
	idA, idB := r.dlID("qb-a"), r.dlID("qb-b")
	blockA := &blockingDL{fakeDL: r.dls["qb-a"], entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(SwapPushDownloaderFactory(func(s models.DownloaderSetting) (downloader.Downloader, error) {
		if s.Name == "qb-a" {
			return blockA, nil
		}
		return r.dls[s.Name], nil
	}))
	data := roundTorrentBytes("race1")
	now := time.Now()
	push := func(dlID uint, id string) (*PushTorrentResult, error) {
		return PushTorrentToDownloader(context.Background(), PushTorrentRequest{
			SiteID: "springsunday", TorrentID: id, TorrentData: data, DownloaderID: dlID,
			Policy:   AdmissionPolicy{PauseOnFreeEnd: true, SourceKind: SourceKindManual, AttemptStart: now},
			Snapshot: untilSnap(now, now.Add(3*time.Hour)),
		})
	}
	doneA := make(chan error, 1)
	go func() { _, err := push(idA, "race1"); doneA <- err }()
	<-blockA.entered // A 已经过了检查、正在加入

	type out struct {
		res *PushTorrentResult
		err error
	}
	doneB := make(chan out, 1)
	go func() { res, err := push(idB, "race1"); doneB <- out{res, err} }()
	select {
	case o := <-doneB:
		t.Fatalf("B 没有等 A 记完账就判定了: %+v %v", o.res, o.err)
	case <-time.After(100 * time.Millisecond): // B 在等同一个种子的锁
	}
	close(blockA.release)
	require.NoError(t, <-doneA)
	o := <-doneB
	require.NoError(t, o.err)
	assert.Equal(t, PushStatusRejectedPlacement, o.res.Status)
	assert.Empty(t, r.dls["qb-b"].addCalls(), "第二个下载器没有收到")
	ti := r.record("race1")
	require.NotNil(t, ti.DownloaderID)
	assert.Equal(t, idA, *ti.DownloaderID)
}
