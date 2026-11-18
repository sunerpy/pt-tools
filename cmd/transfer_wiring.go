package cmd

import (
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// wireTransferWorker 构造并启动转移做种后台：下载器用调度器管理的实例，从站点重新下载种子用 UserInfoService
// 里的共享站点实例（与搜索、登录探测共用限速器），推送走 internal.PushTorrentToDownloader（站点容量闸门不变）。
// 没有任务和开启的规则时，后台每 15 秒只读一次任务表和规则表。
func wireTransferWorker(mgr *scheduler.Manager, svc *v2.UserInfoService) *scheduler.TransferWorker {
	if mgr == nil || global.GlobalDB == nil {
		return nil
	}
	sites := brushSites(svc)
	service := transfer.New(transfer.Config{
		DB:          global.GlobalDB.DB,
		Downloaders: mgr,
		Sites:       sites.BrushSite,
		Logger:      global.GetSlogger(),
	})
	w := scheduler.NewTransferWorker(scheduler.TransferWorkerConfig{
		Service: service,
		DB:      global.GlobalDB.DB,
		Logger:  global.GetSlogger(),
	})
	mgr.SetTransferWorker(w)
	w.Start()
	global.GetSlogger().Info("转移做种后台已启动")
	return w
}
