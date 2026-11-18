package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/freewatch"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

type freeWatchSite struct{ v2.Site }

func (freeWatchSite) GetDetailFetcher() v2.TorrentDetailFetcher {
	return struct{ v2.TorrentDetailFetcher }{}
}

// 后台：启动时先跑一轮，Trigger 马上再跑一轮；Stop 等正在跑的一轮结束；没有服务时什么都不做。
func TestFreeWatchWorker_Lifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.FreeWatch{}, &models.DownloaderSetting{}))
	dl := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://qb", Enabled: true}
	require.NoError(t, db.Create(&dl).Error)
	reads := make(chan string, 8)
	svc := freewatch.New(freewatch.Config{
		DB:   db,
		Site: func(string) v2.Site { return freeWatchSite{} },
		Fresh: func(_ context.Context, _ v2.Site, _, id string, _ time.Time) *v2.PromotionSnapshot {
			reads <- id
			return nil
		},
	})
	_, err = svc.Create(context.Background(), freewatch.CreateInput{Site: "hdtime", TorrentID: "1", DownloaderID: dl.ID})
	require.NoError(t, err)

	w := NewFreeWatchWorker(svc, time.Hour)
	assert.Same(t, svc, w.Service())
	w.Start()
	w.Start()
	select {
	case id := <-reads:
		assert.Equal(t, "1", id, "启动时先跑一轮")
	case <-time.After(10 * time.Second):
		t.Fatal("启动以后没有检查")
	}
	_, err = svc.Create(context.Background(), freewatch.CreateInput{Site: "hdtime", TorrentID: "2", DownloaderID: dl.ID})
	require.NoError(t, err)
	w.Trigger()
	w.Trigger()
	select {
	case id := <-reads:
		assert.Equal(t, "2", id, "Trigger 马上跑一轮")
	case <-time.After(10 * time.Second):
		t.Fatal("Trigger 以后没有检查")
	}
	w.Stop()
	w.Stop()

	var nilWorker *FreeWatchWorker
	nilWorker.Start()
	nilWorker.Stop()
	nilWorker.Trigger()
	assert.Nil(t, nilWorker.Service())
	assert.Zero(t, nilWorker.RunOnce(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Zero(t, w.RunOnce(ctx), "上下文取消了不跑")
	assert.Equal(t, freeWatchTick, NewFreeWatchWorker(svc, 0).tick)

	mgr := NewManager()
	mgr.SetFreeWatchWorker(w)
	other := NewFreeWatchWorker(svc, time.Hour)
	other.Start()
	mgr.SetFreeWatchWorker(other)
	assert.Same(t, other, mgr.GetFreeWatchWorker())
	mgr.StopAll()
	assert.Nil(t, mgr.GetFreeWatchWorker(), "StopAll 停掉并摘下")
}
