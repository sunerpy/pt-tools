package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// M1c：签到开关只写这一列；保存站点配置（整行读出再写回）不会把它清掉。
func TestSetSiteAttendanceEnabled_SurvivesSiteConfigSave(t *testing.T) {
	db, err := OpenTestDB(t)
	require.NoError(t, err)
	s := NewConfigStore(db)
	enabled := true
	require.NoError(t, s.UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "c=1", APIUrl: "http://api",
	}))
	require.NoError(t, s.SetSiteAttendanceEnabled("springsunday", true))

	require.NoError(t, s.UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", APIUrl: "http://api",
	}))
	var row models.SiteSetting
	require.NoError(t, db.DB.Where("name = ?", "springsunday").First(&row).Error)
	assert.True(t, row.AttendanceEnabled, "saving the site config keeps the attendance switch")

	require.NoError(t, s.SetSiteAttendanceEnabled("springsunday", false))
	require.NoError(t, db.DB.Where("name = ?", "springsunday").First(&row).Error)
	assert.False(t, row.AttendanceEnabled)

	assert.Error(t, s.SetSiteAttendanceEnabled("no-such-site", true))
}

// M1c：签到时间窗默认 08:00–10:00；只接受 HH:MM 且开始早于结束；保存全局设置不会改动它。
func TestAttendanceWindow_DefaultsAndValidation(t *testing.T) {
	db, err := OpenTestDB(t)
	require.NoError(t, err)
	s := NewConfigStore(db)

	start, end, err := s.AttendanceWindow()
	require.NoError(t, err)
	assert.Equal(t, "08:00", start)
	assert.Equal(t, "10:00", end)

	require.Error(t, s.SaveAttendanceWindow("07:30", "09:00"), "global settings are not initialised yet")
	require.NoError(t, s.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10}))

	start, end, err = s.AttendanceWindow()
	require.NoError(t, err)
	assert.Equal(t, "08:00", start, "a fresh settings row falls back to the default window")
	assert.Equal(t, "10:00", end)

	require.NoError(t, s.SaveAttendanceWindow("07:30", "09:00"))
	for _, bad := range [][2]string{{"9:00", "10:00"}, {"10:00", "09:00"}, {"10:00", "10:00"}, {"24:00", "23:00"}, {"", "10:00"}} {
		assert.Error(t, s.SaveAttendanceWindow(bad[0], bad[1]), "%v", bad)
	}

	require.NoError(t, s.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10}))
	start, end, err = s.AttendanceWindow()
	require.NoError(t, err)
	assert.Equal(t, "07:30", start, "saving the global settings keeps the window")
	assert.Equal(t, "09:00", end)
}

func TestAttendanceSettings_StoredValuesAndErrors(t *testing.T) {
	db, err := OpenTestDB(t)
	require.NoError(t, err)
	s := NewConfigStore(db)
	require.NoError(t, s.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10}))

	// 库里的值被改坏时按默认值处理
	require.NoError(t, db.DB.Model(&models.SettingsGlobal{}).Where("1 = 1").Updates(map[string]any{"attendance_window_start": "25:00"}).Error)
	start, end, err := s.AttendanceWindow()
	require.NoError(t, err)
	assert.Equal(t, DefaultAttendanceWindowStart, start)
	assert.Equal(t, DefaultAttendanceWindowEnd, end)

	require.NoError(t, db.DB.Migrator().DropTable(&models.SettingsGlobal{}))
	_, _, err = s.AttendanceWindow()
	assert.Error(t, err)
	assert.Error(t, s.SaveAttendanceWindow("07:00", "08:00"))

	require.NoError(t, db.DB.Migrator().DropTable(&models.SiteSetting{}))
	assert.Error(t, s.SetSiteAttendanceEnabled("hdtime", true))
}

// AI 签到：默认关闭；只写这两列，保存全局设置不会把它们清掉；模型名不能有空白。
func TestAttendanceAI_SaveAndKeep(t *testing.T) {
	db, err := OpenTestDB(t)
	require.NoError(t, err)
	s := NewConfigStore(db)

	on, model, err := s.AttendanceAI()
	require.NoError(t, err)
	assert.False(t, on)
	assert.Empty(t, model)
	assert.Error(t, s.SaveAttendanceAI(true, "gemini-flash"), "还没有全局设置行")

	require.NoError(t, db.DB.Create(&models.SettingsGlobal{DownloadDir: "downloads"}).Error)
	require.NoError(t, s.SaveAttendanceAI(true, "  gemini-3.5-flash-lite  "))
	on, model, err = s.AttendanceAI()
	require.NoError(t, err)
	assert.True(t, on)
	assert.Equal(t, "gemini-3.5-flash-lite", model)

	gs, err := s.GetGlobalSettings()
	require.NoError(t, err)
	require.NoError(t, s.SaveGlobalSettings(gs))
	on, model, err = s.AttendanceAI()
	require.NoError(t, err)
	assert.True(t, on, "保存全局设置不改 AI 签到")
	assert.Equal(t, "gemini-3.5-flash-lite", model)

	assert.Error(t, s.SaveAttendanceAI(true, "bad model"))
	require.NoError(t, s.SaveAttendanceAI(false, ""))
	on, model, err = s.AttendanceAI()
	require.NoError(t, err)
	assert.False(t, on)
	assert.Empty(t, model)
}
