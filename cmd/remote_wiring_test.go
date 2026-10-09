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
	"github.com/sunerpy/pt-tools/internal/remote"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/web"
)

func remoteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	prevDB := global.GlobalDB
	t.Cleanup(func() { global.GlobalDB = prevDB })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.RemoteSetting{}, &models.RemoteDevice{}, &models.NotificationConf{}, &models.MonitorNotificationLog{}))
	global.GlobalDB = &models.TorrentDB{DB: db}
	return db
}

// 配对通知写进监控通知日志：source=remote，标题与正文里有设备名与权限
func TestRemotePairedNotifier(t *testing.T) {
	db := remoteTestDB(t)
	require.NoError(t, db.Create(&models.NotificationConf{Name: "tg", ChannelType: "telegram", Enabled: true}).Error)
	n := scheduler.NewMonitorNotifier(db, scheduler.MonitorSenderFunc(func(context.Context, uint, string, string) error { return nil }), nil, nil)
	notify := remotePairedNotifier(n)
	require.NotNil(t, notify)
	notify(context.Background(), remote.Device{ID: 3, Name: "我的手机", Scopes: remote.ScopesRead})
	var rows []models.MonitorNotificationLog
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "remote", rows[0].Source)
	assert.Equal(t, "3", rows[0].Subject)
	assert.Equal(t, "paired", rows[0].Kind)
	assert.Contains(t, rows[0].PayloadJSON, "新设备已配对")
	assert.Contains(t, rows[0].PayloadJSON, "我的手机")
	assert.Contains(t, rows[0].PayloadJSON, "只读")
	assert.Contains(t, pairedText(remote.Device{Name: "平板", Scopes: remote.ScopesFull}), "完全控制")
	assert.Nil(t, remotePairedNotifier(nil), "没有投递器时不发")
}

// 主机端按库里的设置启动（默认关着），接到 web 上
func TestNewRemoteHost(t *testing.T) {
	remoteTestDB(t)
	store := core.NewConfigStore(global.GlobalDB)
	srv := web.NewServer(store, nil)
	h := newRemoteHost(store, srv, nil)
	require.NotNil(t, h)
	t.Cleanup(h.Close)
	assert.False(t, h.Enabled())
	assert.Nil(t, newRemoteHost(nil, srv, nil))
}
