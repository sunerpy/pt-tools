package cmd

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/remote"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/version"
	"github.com/sunerpy/pt-tools/web"
)

// newRemoteHost 构造远程访问的主机端（路线图 M15）并按库里的设置启动（默认关着）：
// 主机密钥用 ConfigStore 的密钥加解密（和站点 Cookie 同一把），设备会话里的请求只交给 App API，
// 配对结果记操作审计，「新设备已配对」写进监控通知日志（source=remote），由现有的投递器发出。
func newRemoteHost(store *core.ConfigStore, srv *web.Server, notifier *scheduler.MonitorNotifier) *remote.Host {
	if global.GlobalDB == nil || global.GlobalDB.DB == nil || store == nil || srv == nil {
		global.GetSlogger().Warn("远程访问跳过初始化：数据库没有就绪")
		return nil
	}
	h := remote.New(remote.Config{
		Store:      remote.NewStore(global.GlobalDB.DB, storeCipher{store: store}),
		Dispatcher: srv.RemoteDispatcher(),
		OnPaired:   remotePairedNotifier(notifier),
		Audit:      srv.RemoteAudit,
		Logger:     global.GetSlogger(),
		Version:    version.GetVersionInfo().Version,
	})
	if err := h.Start(context.Background()); err != nil {
		global.GetSlogger().Warnf("远程访问没有启动: %v", err)
	}
	srv.SetRemoteHost(h)
	return h
}

// remotePairedNotifier 把「新设备已配对」写进监控通知日志，发给所有启用的通道；不直接调用 Push、也不自建 Router。
func remotePairedNotifier(n *scheduler.MonitorNotifier) func(context.Context, remote.Device) {
	if !n.Enabled() {
		return nil
	}
	return func(ctx context.Context, d remote.Device) {
		id := strconv.FormatUint(uint64(d.ID), 10)
		_, err := n.Enqueue(ctx, scheduler.MonitorNotifyEntry{
			Source: "remote", Subject: id, Kind: "paired", EventKey: id,
			Title: "新设备已配对", Text: pairedText(d),
		})
		if err != nil {
			global.GetSlogger().Warnf("[远程访问] 写配对通知失败: %v", err)
		}
	}
}

// pairedText 是配对通知的正文。
func pairedText(d remote.Device) string {
	perm := "只读"
	if slices.Contains(d.Scopes, apitoken.ScopeAppWrite) {
		perm = "完全控制"
	}
	return fmt.Sprintf("设备「%s」已经配对，权限：%s。如果不是你本人操作，请在「系统 → 远程访问」撤销这台设备。", d.Name, perm)
}
