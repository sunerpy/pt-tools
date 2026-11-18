package cmd

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

func TestWireDeadTorrentMonitor(t *testing.T) {
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SettingsGlobal{}, &models.DownloaderSetting{}, &models.NotificationConf{}))
	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prevDB })

	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	wireDeadTorrentMonitor(mgr, nil, nil)
	assert.Nil(t, mgr.GetDeadTorrentMonitor(), "没有配置时不接线")

	wireDeadTorrentMonitor(mgr, core.NewConfigStore(global.GlobalDB), nil)
	require.NotNil(t, mgr.GetDeadTorrentMonitor())

	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "qb-off", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: false}).Error)
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "qb-missing", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}).Error)
	dls, errs, err := deadTorrentDownloaders(mgr)(context.Background())
	require.NoError(t, err)
	assert.Empty(t, dls)
	require.Len(t, errs, 1, "已启用但下载器管理器里没有的那台")
	assert.Contains(t, errs[0].Error(), "qb-missing")

	require.NoError(t, db.Migrator().DropTable(&models.DownloaderSetting{}))
	_, errs, err = deadTorrentDownloaders(mgr)(context.Background())
	assert.Empty(t, errs, "读不到列表时不算哪台连不上")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "读取下载器失败")
}
