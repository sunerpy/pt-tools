package cmd

import (
	"context"
	"testing"
	"time"

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

func TestWireReseedWorker(t *testing.T) {
	global.InitLogger(zap.NewNop())
	prevDB := global.GlobalDB
	t.Cleanup(func() { global.GlobalDB = prevDB })

	global.GlobalDB = nil
	noDB := scheduler.NewManager()
	t.Cleanup(noDB.StopAll)
	assert.Nil(t, wireReseedWorker(noDB, nil, nil, nil), "没有数据库时不接线")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// 内存库每个连接各是一份：辅种后台一启动就读库（把上次没跑完的轮次记为被打断），只留一个连接才看得到建好的表
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.TorrentTransferJob{}, &models.TransferRule{}, &models.DownloaderSetting{}, &models.ReseedSetting{}, &models.ReseedRecord{}))
	global.GlobalDB = &models.TorrentDB{DB: db}
	store := core.NewConfigStore(global.GlobalDB)
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	assert.Nil(t, wireReseedWorker(mgr, nil, store, nil), "转移做种后台没起来时不接线")

	ctx, today := context.Background(), time.Now().Format(time.DateOnly)
	assert.Empty(t, reseedReportLine(nil)(ctx, today, "今日"))
	assert.Empty(t, reseedReportLine(mgr)(ctx, today, "今日"), "辅种后台还没接线")

	require.NotNil(t, wireTransferWorker(mgr, nil))
	w := wireReseedWorker(mgr, nil, store, nil)
	require.NotNil(t, w)
	assert.Same(t, w, mgr.GetReseedWorker())
	assert.NotNil(t, w.Service())

	// 战报里的辅种一行：设置里没开时没有；读不到统计时不写；有数据时写
	line := reseedReportLine(mgr)
	assert.Empty(t, line(ctx, today, "今日"), "没开")
	require.NoError(t, db.Create(&models.ReseedSetting{ID: 1, DailyReport: true}).Error)
	assert.Empty(t, line(ctx, today, "今日"), "统计的表没有：不写这一行")
	require.NoError(t, db.AutoMigrate(&models.ReseedAttempt{}, &models.ReseedDailySkip{}, &models.ReseedIYUUDaily{}))
	require.NoError(t, db.Create(&models.ReseedAttempt{StartedAt: time.Now(), SiteName: "hdsky", InfoHash: "h", Outcome: models.ReseedAttemptFailed}).Error)
	assert.Equal(t, "🌱 今日辅种：建成 0 · 成功 0 · 失败 1 · 站点已删除 0", line(ctx, today, "今日"))

	t.Setenv("PT_TOOLS_SECRET_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	c := storeCipher{store: store}
	enc, err := c.Encrypt("tok")
	require.NoError(t, err)
	assert.NotEqual(t, "tok", enc)
	plain, err := c.Decrypt(enc)
	require.NoError(t, err)
	assert.Equal(t, "tok", plain)
}
