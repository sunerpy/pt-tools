package cmd

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/freewatch"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

func TestWireFreeWatchWorker(t *testing.T) {
	global.InitLogger(zap.NewNop())
	prevDB := global.GlobalDB
	t.Cleanup(func() { global.GlobalDB = prevDB })

	global.GlobalDB = nil
	noDB := scheduler.NewManager()
	t.Cleanup(noDB.StopAll)
	assert.Nil(t, wireFreeWatchWorker(noDB, nil, nil), "没有数据库时不接线")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.FreeWatch{}, &models.DownloaderSetting{}, &models.MonitorNotificationLog{}, &models.NotificationConf{}))
	global.GlobalDB = &models.TorrentDB{DB: db}
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)

	w := wireFreeWatchWorker(mgr, nil, nil)
	require.NotNil(t, w)
	assert.Same(t, w, mgr.GetFreeWatchWorker())
	require.NotNil(t, w.Service())
	assert.False(t, w.Service().Supports("hdtime"), "没有配置时没有能等免费的站点")

	// 通知：没有通道服务时不发；有时写进监控通知日志，来源 free_watch，同一条只写一次
	assert.Nil(t, freeWatchNotifier(nil))
	require.NoError(t, db.Create(&models.NotificationConf{ChannelType: "telegram", Name: "tg", Enabled: true}).Error)
	sender := scheduler.MonitorSenderFunc(func(context.Context, uint, scheduler.MonitorMessage) error { return nil })
	notify := freeWatchNotifier(scheduler.NewMonitorNotifier(db, sender, nil, nil))
	require.NotNil(t, notify)
	m := freewatch.Message{Kind: freewatch.KindPushed, Watch: models.FreeWatch{ID: 7, SiteName: "hdtime", TorrentID: "1", Title: "T"}, Downloader: "qb"}
	notify(context.Background(), m)
	notify(context.Background(), m)
	var rows []models.MonitorNotificationLog
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, []string{"free_watch", "7", "pushed", "7"}, []string{rows[0].Source, rows[0].Subject, rows[0].Kind, rows[0].EventKey})
	assert.Contains(t, rows[0].PayloadJSON, "等到免费了，已推送")
	assert.Contains(t, rows[0].PayloadJSON, "torrent.free_watch.pushed")
}
