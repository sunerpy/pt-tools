package scheduler

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func newTransferWorkerForTest(t *testing.T, now time.Time) (*TransferWorker, *gorm.DB, *sitelogin.FakeClock) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "w.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.TorrentTransferJob{}, &models.DownloaderPathMap{}, &models.TransferRule{}, &models.TorrentInfo{}))
	src, dst := newSchedFakeDownloader("src"), newSchedFakeDownloader("dst")
	clock := sitelogin.NewFakeClock(now)
	svc := transfer.New(transfer.Config{
		DB: db,
		Downloaders: transfer.DownloadersFunc(func(_ context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error) {
			switch id {
			case 1:
				return src, models.DownloaderSetting{ID: 1, Name: "src"}, nil
			case 2:
				return dst, models.DownloaderSetting{ID: 2, Name: "dst"}, nil
			}
			return nil, models.DownloaderSetting{}, errors.New("下载器未启用")
		}),
		Now: clock.Now,
	})
	return NewTransferWorker(TransferWorkerConfig{Service: svc, DB: db, Clock: clock, Tick: time.Hour}), db, clock
}

// 到期的规则才运行，运行时间与结果写回规则；运行失败也记下原因。
func TestTransferWorker_RunsDueRules(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	w, db, clock := newTransferWorkerForTest(t, now)
	ok := models.TransferRule{Name: "ok", Enabled: true, SourceDownloaderID: 1, TargetDownloaderID: 2, IntervalMin: 60}
	broken := models.TransferRule{Name: "broken", Enabled: true, SourceDownloaderID: 9, TargetDownloaderID: 2, IntervalMin: 60}
	off := models.TransferRule{Name: "off", SourceDownloaderID: 1, TargetDownloaderID: 2}
	for _, r := range []*models.TransferRule{&ok, &broken, &off} {
		require.NoError(t, db.Create(r).Error)
	}
	_, created := w.RunOnce(context.Background())
	assert.Zero(t, created)
	read := func(id uint) models.TransferRule {
		var r models.TransferRule
		require.NoError(t, db.First(&r, id).Error)
		return r
	}
	require.NotNil(t, read(ok.ID).LastRunAt)
	assert.Equal(t, "符合条件 0 个，建了 0 个任务", read(ok.ID).LastResult)
	assert.Contains(t, read(broken.ID).LastResult, "运行失败")
	assert.Nil(t, read(off.ID).LastRunAt, "关闭的规则不跑")

	clock.Advance(30 * time.Minute)
	require.NoError(t, db.Model(&models.TransferRule{}).Where("id = ?", ok.ID).Update("last_result", "x").Error)
	w.RunOnce(context.Background())
	assert.Equal(t, "x", read(ok.ID).LastResult, "没到间隔")
	clock.Advance(31 * time.Minute)
	w.RunOnce(context.Background())
	assert.NotEqual(t, "x", read(ok.ID).LastResult)
}

// Start 之后 Trigger 立即跑一轮；Stop 等循环退出；挂到管理器上随 StopAll 停止。
func TestTransferWorker_Lifecycle(t *testing.T) {
	w, db, _ := newTransferWorkerForTest(t, time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC))
	r := models.TransferRule{Name: "r", Enabled: true, SourceDownloaderID: 1, TargetDownloaderID: 2}
	require.NoError(t, db.Create(&r).Error)
	w.Start()
	w.Start()
	w.Trigger()
	require.Eventually(t, func() bool {
		var got models.TransferRule
		return db.First(&got, r.ID).Error == nil && got.LastRunAt != nil
	}, 5*time.Second, 20*time.Millisecond)
	w.Stop()
	w.Stop()
	assert.NotNil(t, w.Service())

	var nilWorker *TransferWorker
	nilWorker.Start()
	nilWorker.Trigger()
	nilWorker.Stop()
	assert.Nil(t, nilWorker.Service())
	s, c := nilWorker.RunOnce(context.Background())
	assert.Zero(t, s+c)

	mgr := &Manager{}
	mgr.SetTransferWorker(w)
	assert.Same(t, w, mgr.GetTransferWorker())
	other, _, _ := newTransferWorkerForTest(t, time.Now())
	other.Start()
	mgr.SetTransferWorker(other) // 换掉旧的
	mgr.StopAll()
	assert.Nil(t, mgr.GetTransferWorker())
}
