package cmd

import (
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// wireBrushMonitor 构造并启动刷流监控：站点用 UserInfoService 里注册的共享实例（与搜索、登录探测共用限速器），
// 下载器用调度器管理的实例，推送走 internal.PushTorrentToDownloader（磁盘与站点容量闸门不变）。
// 刷流任务默认关闭，没有开启的任务时监控每分钟只读一次任务表。
func wireBrushMonitor(mgr *scheduler.Manager, svc *v2.UserInfoService) *scheduler.BrushMonitor {
	if mgr == nil || global.GlobalDB == nil {
		return nil
	}
	mon := scheduler.NewBrushMonitor(scheduler.BrushMonitorConfig{
		DB: global.GlobalDB.DB,
		Sites: scheduler.BrushSitesFunc(func(name string) (v2.Site, bool) {
			if svc == nil {
				return nil, false
			}
			return svc.GetSite(name)
		}),
		Downloaders: mgr,
		Logger:      global.GetSlogger(),
	})
	mgr.SetBrushMonitor(mon)
	mon.Start()
	global.GetSlogger().Info("刷流监控已启动")
	return mon
}
