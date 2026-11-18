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
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

func TestWireTransferWorker(t *testing.T) {
	global.InitLogger(zap.NewNop())
	prevDB := global.GlobalDB
	t.Cleanup(func() { global.GlobalDB = prevDB })

	global.GlobalDB = nil
	noDB := scheduler.NewManager()
	t.Cleanup(noDB.StopAll)
	assert.Nil(t, wireTransferWorker(noDB, nil), "没有数据库时不接线")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.TorrentTransferJob{}, &models.TransferRule{}, &models.DownloaderSetting{}))
	global.GlobalDB = &models.TorrentDB{DB: db}
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)

	w := wireTransferWorker(mgr, nil)
	require.NotNil(t, w)
	assert.Same(t, w, mgr.GetTransferWorker())
	require.NotNil(t, w.Service())

	// 下载器按 ID 取：不存在、未启用都报错
	_, _, err = mgr.TransferDownloader(context.Background(), 42)
	require.ErrorContains(t, err, "不存在")
	ds := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: false}
	require.NoError(t, db.Create(&ds).Error)
	_, got, err := mgr.TransferDownloader(context.Background(), ds.ID)
	require.ErrorContains(t, err, "未启用")
	assert.Equal(t, "qb", got.Name)
}
