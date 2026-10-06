package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func newDailyReportStore(t *testing.T) *ConfigStore {
	t.Helper()
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.DB.AutoMigrate(&models.NotificationConf{}))
	store := NewConfigStore(db)
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10}))
	return store
}

func TestDailyReportSettings_DefaultsAndRoundTrip(t *testing.T) {
	store := newDailyReportStore(t)
	got, err := store.DailyReportSettings()
	require.NoError(t, err)
	assert.Equal(t, DailyReportSettings{Enabled: false, Time: DefaultDailyReportTime, ChannelIDs: []uint{}}, got)

	conf := models.NotificationConf{ChannelType: "webhook", Name: "w", Enabled: true}
	require.NoError(t, store.db.DB.Create(&conf).Error)
	require.NoError(t, store.SaveDailyReportSettings(DailyReportSettings{Enabled: true, Time: "21:30", ChannelIDs: []uint{conf.ID}}))

	got, err = store.DailyReportSettings()
	require.NoError(t, err)
	assert.Equal(t, DailyReportSettings{Enabled: true, Time: "21:30", ChannelIDs: []uint{conf.ID}}, got)

	// 保存全局设置不会改动战报的几列
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 15}))
	got, err = store.DailyReportSettings()
	require.NoError(t, err)
	assert.True(t, got.Enabled)
}

// 开启战报至少要选一个通道：不沿用「空列表即全部通道」。时间格式与通道是否存在也要校验。
func TestSaveDailyReportSettings_Validation(t *testing.T) {
	store := newDailyReportStore(t)
	assert.ErrorIs(t, store.SaveDailyReportSettings(DailyReportSettings{Enabled: true, Time: "22:00"}), ErrDailyReportNoChannel)
	require.Error(t, store.SaveDailyReportSettings(DailyReportSettings{Time: "25:00"}))
	require.Error(t, store.SaveDailyReportSettings(DailyReportSettings{Time: "9:00"}))
	require.Error(t, store.SaveDailyReportSettings(DailyReportSettings{Enabled: true, Time: "22:00", ChannelIDs: []uint{999}}),
		"不存在的通道")
	require.NoError(t, store.SaveDailyReportSettings(DailyReportSettings{Enabled: false, Time: "07:05"}), "关闭时可以不选通道")
}

// 偏移首次用到时分配并持久化，之后固定不变。
func TestDailyReportOffset_AssignedOnceAndPersisted(t *testing.T) {
	store := newDailyReportStore(t)
	calls := 0
	rnd := func(n int64) int64 {
		calls++
		return 41 // → 42 秒
	}
	first, err := store.DailyReportOffset(rnd)
	require.NoError(t, err)
	assert.EqualValues(t, 42, first.Seconds())
	second, err := store.DailyReportOffset(func(int64) int64 { return 7 })
	require.NoError(t, err)
	assert.Equal(t, first, second, "分配之后固定不变")
	assert.Equal(t, 1, calls)
}

func TestConfigStore_EnabledSiteNames(t *testing.T) {
	store := newDailyReportStore(t)
	on, off := true, false
	_, err := store.UpsertSite("HDSky", models.SiteConfig{Enabled: &on, AuthMethod: "cookie", Cookie: "c"})
	require.NoError(t, err)
	_, err = store.UpsertSite("pterclub", models.SiteConfig{Enabled: &off, AuthMethod: "cookie", Cookie: "c"})
	require.NoError(t, err)

	got, err := store.EnabledSiteNames()
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"hdsky": true}, got)

	// Cookie 解不开（密钥换了或丢了）时 ListSites 会失败，只看启用状态的这里不受影响
	require.NoError(t, store.db.DB.Model(&models.SiteSetting{}).Where("name = ?", "HDSky").
		Updates(map[string]any{"cookie": "", "cookie_encrypted": "not-a-ciphertext"}).Error)
	_, listErr := store.ListSites()
	require.Error(t, listErr)
	got, err = store.EnabledSiteNames()
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"hdsky": true}, got)
}

// 全局设置行还不存在：读回默认值、偏移为 0、保存时提示先初始化；表坏了时如实报错。
func TestDailyReportSettings_EdgeCases(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.DB.AutoMigrate(&models.NotificationConf{}))
	store := NewConfigStore(db)

	got, err := store.DailyReportSettings()
	require.NoError(t, err)
	assert.Equal(t, DefaultDailyReportTime, got.Time)
	off, err := store.DailyReportOffset(func(int64) int64 { return 1 })
	require.NoError(t, err)
	assert.Zero(t, off)
	err = store.SaveDailyReportSettings(DailyReportSettings{Time: "22:00"})
	require.ErrorContains(t, err, "尚未初始化")

	// 通道列表重复的 ID 只算一次；通道 JSON 坏了时按空列表读
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10}))
	conf := models.NotificationConf{ChannelType: "webhook", Name: "w", Enabled: true}
	require.NoError(t, db.DB.Create(&conf).Error)
	require.NoError(t, store.SaveDailyReportSettings(DailyReportSettings{Enabled: true, Time: "22:00", ChannelIDs: []uint{conf.ID, conf.ID}}))
	got, err = store.DailyReportSettings()
	require.NoError(t, err)
	assert.Equal(t, []uint{conf.ID}, got.ChannelIDs)
	require.NoError(t, db.DB.Model(&models.SettingsGlobal{}).Where("1 = 1").Update("daily_report_channel_ids", "{bad").Error)
	got, err = store.DailyReportSettings()
	require.NoError(t, err)
	assert.Empty(t, got.ChannelIDs)
	assert.Empty(t, parseChannelIDs("  "))

	require.NoError(t, db.DB.Migrator().DropTable(&models.NotificationConf{}))
	require.ErrorContains(t, store.SaveDailyReportSettings(DailyReportSettings{Time: "22:00", ChannelIDs: []uint{1}}), "检查通知通道失败")

	require.NoError(t, db.DB.Migrator().DropTable(&models.SettingsGlobal{}))
	_, err = store.DailyReportSettings()
	require.ErrorContains(t, err, "读取每日战报设置失败")
	_, err = store.DailyReportOffset(func(int64) int64 { return 1 })
	require.ErrorContains(t, err, "读取战报偏移失败")
	require.ErrorContains(t, store.SaveDailyReportSettings(DailyReportSettings{Time: "22:00"}), "读取全局设置失败")
}
