package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestDeadTorrentScanSettings_DefaultsAndRoundTrip(t *testing.T) {
	store := newDailyReportStore(t)
	got, err := store.DeadTorrentScanSettings()
	require.NoError(t, err)
	assert.Equal(t, DeadTorrentScanSettings{Enabled: false, IntervalHours: DefaultDeadTorrentScanIntervalH, ChannelIDs: []uint{}}, got)

	conf := models.NotificationConf{ChannelType: "webhook", Name: "w", Enabled: true}
	require.NoError(t, store.db.DB.Create(&conf).Error)
	require.NoError(t, store.SaveDeadTorrentScanSettings(DeadTorrentScanSettings{Enabled: true, IntervalHours: 12, ChannelIDs: []uint{conf.ID, conf.ID}}))
	got, err = store.DeadTorrentScanSettings()
	require.NoError(t, err)
	assert.Equal(t, DeadTorrentScanSettings{Enabled: true, IntervalHours: 12, ChannelIDs: []uint{conf.ID}}, got)

	// 保存全局设置不改这几列
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 15}))
	got, err = store.DeadTorrentScanSettings()
	require.NoError(t, err)
	assert.True(t, got.Enabled)

	// 关闭时可以不选通道
	require.NoError(t, store.SaveDeadTorrentScanSettings(DeadTorrentScanSettings{Enabled: false, IntervalHours: 24}))
}

func TestSaveDeadTorrentScanSettings_Validation(t *testing.T) {
	store := newDailyReportStore(t)
	assert.ErrorIs(t, store.SaveDeadTorrentScanSettings(DeadTorrentScanSettings{Enabled: true, IntervalHours: 24}), ErrDeadTorrentScanNoChannel)
	assert.ErrorIs(t, store.SaveDeadTorrentScanSettings(DeadTorrentScanSettings{IntervalHours: 5}), ErrDeadTorrentScanInvalid)
	assert.ErrorIs(t, store.SaveDeadTorrentScanSettings(DeadTorrentScanSettings{IntervalHours: 169}), ErrDeadTorrentScanInvalid)
	assert.ErrorIs(t, store.SaveDeadTorrentScanSettings(DeadTorrentScanSettings{Enabled: true, IntervalHours: 24, ChannelIDs: []uint{999}}), ErrDeadTorrentScanInvalid)
	off := models.NotificationConf{ChannelType: "webhook", Name: "off", Enabled: true}
	require.NoError(t, store.db.DB.Create(&off).Error)
	require.NoError(t, store.db.DB.Model(&off).Update("enabled", false).Error)
	assert.ErrorIs(t, store.SaveDeadTorrentScanSettings(DeadTorrentScanSettings{Enabled: true, IntervalHours: 24, ChannelIDs: []uint{off.ID}}), ErrDeadTorrentScanInvalid,
		"选中的通道都停用了")
	require.NoError(t, store.SaveDeadTorrentScanSettings(DeadTorrentScanSettings{Enabled: false, IntervalHours: 24, ChannelIDs: []uint{off.ID}}), "关闭时可以留着停用的通道")

	empty := NewConfigStore(func() *models.TorrentDB {
		db, err := NewTempDBDir(t.TempDir())
		require.NoError(t, err)
		return db
	}())
	got, err := empty.DeadTorrentScanSettings()
	require.NoError(t, err)
	assert.Equal(t, DefaultDeadTorrentScanIntervalH, got.IntervalHours, "全局设置行还不存在时返回默认值")
	assert.Error(t, empty.SaveDeadTorrentScanSettings(DeadTorrentScanSettings{IntervalHours: 24}), "全局设置还没初始化")
}
