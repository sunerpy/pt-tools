package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

type adapterAttendSite struct{}

func (adapterAttendSite) SupportsAttendance() bool { return true }

func (adapterAttendSite) Attend(context.Context) (v2.AttendResult, error) {
	return v2.AttendResult{Status: v2.AttendAlready, Message: "您今天已经签到过了"}, nil
}

// M1c：/signin 经适配器调用签到监控；监控没有接线时说明原因。
func TestAttendanceCommands(t *testing.T) {
	_, err := attendanceCommands{}.SignIn(context.Background(), "hdtime")
	assert.ErrorContains(t, err, "未启动")
	idle := scheduler.NewManager()
	// 不停掉的话，它的配置事件协程会在后续测试发布 ConfigChanged 时 reload、读 GlobalDB。
	idle.StopAll()
	_, err = attendanceCommands{mgr: idle}.SignInAll(context.Background())
	assert.ErrorContains(t, err, "未启动")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteAttendanceLog{}))
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdtime", Enabled: true, AuthMethod: "cookie", Cookie: "c", AttendanceEnabled: true}).Error)

	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	mgr.SetAttendanceMonitor(scheduler.NewAttendanceMonitor(scheduler.AttendanceMonitorConfig{
		DB: db,
		Sites: scheduler.AttendanceSitesFunc(func(string) (v2.AttendanceCapable, bool) {
			return adapterAttendSite{}, true
		}),
		Location: time.UTC,
	}))
	cmds := attendanceCommands{mgr: mgr}

	one, err := cmds.SignIn(context.Background(), "hdtime")
	require.NoError(t, err)
	assert.Equal(t, "hdtime", one.Site)
	assert.Equal(t, models.AttendanceAlready, one.Status)
	assert.Contains(t, one.Message, "已经签到")

	_, err = cmds.SignIn(context.Background(), "no-such-site")
	assert.ErrorContains(t, err, "站点不存在")

	all, err := cmds.SignInAll(context.Background())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, models.AttendanceAlready, all[0].Status)
}
