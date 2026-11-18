package cmd

import (
	"context"
	"strconv"
	"time"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/freewatch"
	"github.com/sunerpy/pt-tools/scheduler"
)

// freeWatchNotifySource 是等免费的通知来源（监控通知日志的 source，种类是 pushed、expired、failed）。
const freeWatchNotifySource = "free_watch"

// wireFreeWatchWorker 构造并启动等免费后台：读优惠与下种子用搜索编排器里启用着的站点（同网页推送与 qB 兼容入口）；
// 推送走 internal.PushTorrentToDownloader（免费门禁、磁盘与站点容量闸门不变）；通知写进监控通知日志，发给所有启用的通道。
func wireFreeWatchWorker(mgr *scheduler.Manager, store *core.ConfigStore, notifier *scheduler.MonitorNotifier) *scheduler.FreeWatchWorker {
	if mgr == nil || global.GlobalDB == nil || global.GlobalDB.DB == nil {
		global.GetSlogger().Warn("等免费跳过初始化：数据库未就绪")
		return nil
	}
	cfg := freewatch.Config{DB: global.GlobalDB.DB, Notify: freeWatchNotifier(notifier), Logger: global.GetSlogger()}
	if store != nil {
		cfg.Site = enabledSite(store)
	}
	svc := freewatch.New(cfg)
	w := scheduler.NewFreeWatchWorker(svc, 0)
	mgr.SetFreeWatchWorker(w)
	w.Start()
	global.GetSlogger().Info("等免费后台已启动")
	return w
}

// freeWatchNotifier 把等免费的通知写进监控通知日志（同一条记录的同一种通知只发一次）。
func freeWatchNotifier(n *scheduler.MonitorNotifier) func(ctx context.Context, m freewatch.Message) {
	if !n.Enabled() {
		return nil
	}
	return func(ctx context.Context, m freewatch.Message) {
		id := strconv.FormatUint(uint64(m.Watch.ID), 10)
		title, text, ev := freewatch.Notice(m, time.Now())
		if _, err := n.Enqueue(ctx, scheduler.MonitorNotifyEntry{
			Source: freeWatchNotifySource, Subject: id, Kind: m.Kind, EventKey: id, Title: title, Text: text, Event: ev,
		}); err != nil {
			global.GetSlogger().Warnf("[等免费] 写入通知失败: %v", err)
		}
	}
}
