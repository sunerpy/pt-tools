package cmd

import (
	"context"
	"errors"

	chatopscmds "github.com/sunerpy/pt-tools/internal/chatops/commands"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

// attendanceCommands 让 ChatOps 的 /signin 调用每日签到监控。监控在 ChatOps 引导之后才接线，
// 所以每次调用时再从管理器取。
type attendanceCommands struct {
	mgr *scheduler.Manager
}

func (a attendanceCommands) monitor() (*scheduler.AttendanceMonitor, error) {
	if a.mgr == nil {
		return nil, errors.New("签到服务未启动")
	}
	mon := a.mgr.GetAttendanceMonitor()
	if mon == nil {
		return nil, errors.New("签到服务未启动")
	}
	return mon, nil
}

func (a attendanceCommands) SignIn(ctx context.Context, site string) (chatopscmds.AttendanceOutcome, error) {
	mon, err := a.monitor()
	if err != nil {
		return chatopscmds.AttendanceOutcome{Site: site}, err
	}
	row, err := mon.SignNow(ctx, site)
	if err != nil {
		return chatopscmds.AttendanceOutcome{Site: site}, err
	}
	return attendanceOutcome(*row), nil
}

func (a attendanceCommands) SignInAll(ctx context.Context) ([]chatopscmds.AttendanceOutcome, error) {
	mon, err := a.monitor()
	if err != nil {
		return nil, err
	}
	rows, err := mon.SignAll(ctx)
	out := make([]chatopscmds.AttendanceOutcome, 0, len(rows))
	for _, row := range rows {
		out = append(out, attendanceOutcome(row))
	}
	return out, err
}

func attendanceOutcome(row models.SiteAttendanceLog) chatopscmds.AttendanceOutcome {
	return chatopscmds.AttendanceOutcome{Site: row.SiteName, Status: row.Status, Message: row.Message, Error: row.LastError}
}
