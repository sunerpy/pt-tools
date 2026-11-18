package cmd

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestWireBrushMonitor(t *testing.T) {
	global.InitLogger(zap.NewNop())
	prevDB := global.GlobalDB
	t.Cleanup(func() { global.GlobalDB = prevDB })

	global.GlobalDB = nil
	// 每个 Manager 都有一个订阅 ConfigChanged 的协程，会读 global.GlobalDB：用完必须 StopAll，否则后面的测试改写它时竞态
	noDB := scheduler.NewManager()
	t.Cleanup(noDB.StopAll)
	assert.Nil(t, wireBrushMonitor(noDB, nil), "没有数据库时不接线")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.BrushTask{}, &models.DownloaderSetting{}))
	global.GlobalDB = &models.TorrentDB{DB: db}
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)

	mon := wireBrushMonitor(mgr, nil)
	require.NotNil(t, mon)
	assert.Same(t, mon, mgr.GetBrushMonitor())

	// 下载器按 ID 取：不存在、未启用都报错
	_, _, err = mgr.BrushDownloader(42)
	require.ErrorContains(t, err, "不存在")
	ds := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: false}
	require.NoError(t, db.Create(&ds).Error)
	require.NoError(t, db.Model(&ds).Update("enabled", false).Error)
	_, name, err := mgr.BrushDownloader(ds.ID)
	require.ErrorContains(t, err, "未启用")
	assert.Equal(t, "qb", name)
}

func TestBrushSites(t *testing.T) {
	_, ok := brushSites(nil).BrushSite("hdsky")
	assert.False(t, ok, "用户数据服务没起来时取不到站点")
	svc := v2.NewUserInfoService(v2.UserInfoServiceConfig{Repo: v2.NewInMemoryUserInfoRepo(), Logger: zap.NewNop()})
	_, ok = brushSites(svc).BrushSite("hdsky")
	assert.False(t, ok, "没注册的站点取不到")
}
