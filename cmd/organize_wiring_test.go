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
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func organizeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	prevDB := global.GlobalDB
	t.Cleanup(func() { global.GlobalDB = prevDB })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.SettingsGlobal{}, &models.DownloaderSetting{}, &models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{},
		&models.MediaCache{}, &models.MediaLibrary{}, &models.MediaPathMap{}, &models.MediaServer{}, &models.MediaOrganizeSetting{},
		&models.MediaTransferHistory{}, &models.MonitorNotificationLog{}, &models.NotificationConf{},
	))
	global.GlobalDB = &models.TorrentDB{DB: db}
	return db
}

// 整理入库服务：没有识别服务时不建；媒体服务器的 Token 用 ConfigStore 的密钥加密保存。
func TestNewOrganizeService(t *testing.T) {
	db := organizeTestDB(t)
	store := core.NewConfigStore(global.GlobalDB)
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	assert.Nil(t, newOrganizeService(store, mgr, nil, nil))
	assert.Nil(t, newOrganizeService(nil, mgr, newMediaService(store), nil))

	svc := newOrganizeService(store, mgr, newMediaService(store), nil)
	require.NotNil(t, svc)
	t.Cleanup(svc.Stop)
	tok := "embykey"
	_, err := svc.SaveServer(context.Background(), 0, organize.ServerInput{Name: "Emby", Kind: "emby", URL: "http://127.0.0.1:8096", Token: &tok})
	require.NoError(t, err)
	var row models.MediaServer
	require.NoError(t, db.First(&row).Error)
	assert.NotEmpty(t, row.TokenEncrypted)
	assert.NotContains(t, row.TokenEncrypted, tok)
	assert.Empty(t, qaTMDBImageURL(), "正式构建里 TMDB 图片地址不能改")
}

func TestOrganizeDownloaders(t *testing.T) {
	db := organizeTestDB(t)
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	fake := &organizeFakeDL{}
	dm := mgr.GetDownloaderManager()
	dm.RegisterFactory(downloader.DownloaderQBittorrent, func(_ downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		return fake, nil
	})
	require.NoError(t, dm.RegisterConfig("qb", downloader.NewGenericConfig(downloader.DownloaderQBittorrent, "http://qb", "u", "p", true), true))
	on := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://qb", Enabled: true}
	off := models.DownloaderSetting{Name: "tr", Type: "transmission", URL: "http://tr"}
	require.NoError(t, db.Create(&on).Error)
	require.NoError(t, db.Create(&off).Error)
	require.NoError(t, db.Model(&models.DownloaderSetting{}).Where("id = ?", off.ID).Update("enabled", false).Error)

	d := organizeDownloaders{mgr: mgr}
	ctx := context.Background()
	dl, s, err := d.Get(ctx, on.ID)
	require.NoError(t, err)
	assert.Equal(t, "qb", s.Name)
	assert.Same(t, fake, dl)
	_, s, err = d.ByName(ctx, "qb")
	require.NoError(t, err)
	assert.Equal(t, on.ID, s.ID)
	_, _, err = d.Get(ctx, off.ID)
	require.ErrorContains(t, err, "没有启用")
	_, _, err = d.Get(ctx, 99)
	require.ErrorIs(t, err, errOrganizeNoDownloader)
	_, _, err = d.ByName(ctx, "nope")
	require.ErrorIs(t, err, errOrganizeNoDownloader)
	list, err := d.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1, "只列启用的")
}

func TestOrganizeNotifier(t *testing.T) {
	db := organizeTestDB(t)
	assert.Nil(t, organizeNotifier(nil), "没有投递器时不发")
	n := scheduler.NewMonitorNotifier(db, scheduler.MonitorSenderFunc(func(context.Context, uint, string, string) error { return nil }), nil, nil)
	conf := models.NotificationConf{Name: "tg", ChannelType: "telegram", Enabled: true}
	require.NoError(t, db.Create(&conf).Error)
	notify := organizeNotifier(n)
	require.NotNil(t, notify)
	require.NoError(t, notify(context.Background(), organize.Notice{Key: "abc:1", Title: "入库：奥本海默 (2023)", Text: "电影", ChannelIDs: []uint{conf.ID}}))
	require.NoError(t, notify(context.Background(), organize.Notice{Key: "abc:1", Title: "入库：奥本海默 (2023)", Text: "电影", ChannelIDs: []uint{conf.ID}}))
	var rows []models.MonitorNotificationLog
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1, "同一批只写一行")
	assert.Equal(t, "media", rows[0].Source)
	assert.Equal(t, "organized", rows[0].Kind)
	assert.Equal(t, conf.ID, rows[0].NotificationConfID)
	assert.Equal(t, "abc", rows[0].Subject)
	assert.Equal(t, "abc:1", rows[0].EventKey)
}

type organizeFakeDL struct {
	downloader.Downloader
}

func (*organizeFakeDL) Authenticate() error { return nil }
func (*organizeFakeDL) IsHealthy() bool     { return true }
func (*organizeFakeDL) GetName() string     { return "qb" }
func (*organizeFakeDL) Close() error        { return nil }
