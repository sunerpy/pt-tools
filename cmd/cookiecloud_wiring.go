package cmd

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/cookiecloud"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/web"
)

// wireCookieCloudWorker 构造 CookieCloud 导入服务并启动定时同步（默认不同步）。
// 密码用 ConfigStore 的 Cookie 密钥加解密；写 Cookie 走与浏览器扩展同步凭据相同的路径。
func wireCookieCloudWorker(mgr *scheduler.Manager, store *core.ConfigStore, registry *v2.SiteRegistry) *scheduler.CookieCloudWorker {
	if mgr == nil || global.GlobalDB == nil || store == nil || registry == nil {
		return nil
	}
	service := cookiecloud.New(cookiecloud.Config{
		DB:     global.GlobalDB.DB,
		Cipher: storeCipher{store: store},
		HTTP:   &http.Client{Timeout: 30 * time.Second},
		Sites:  cookieCloudSites(store, registry),
		Apply:  applySiteCookies(store, mgr),
		Logger: global.GetSlogger(),
	})
	w := scheduler.NewCookieCloudWorker(scheduler.CookieCloudWorkerConfig{Service: service, Logger: global.GetSlogger()})
	mgr.SetCookieCloudWorker(w)
	w.Start()
	global.GetSlogger().Info("CookieCloud 同步后台已启动")
	return w
}

// cookieCloudSites 列出用 Cookie 登录的站点：pt-tools 访问它用的地址（站点设置里改过的地址优先）、是否启用、现在的 Cookie。
func cookieCloudSites(store *core.ConfigStore, registry *v2.SiteRegistry) func(context.Context) ([]cookiecloud.SiteState, error) {
	return func(context.Context) ([]cookiecloud.SiteState, error) {
		configured, err := store.ListSites()
		if err != nil {
			return nil, err
		}
		ids := registry.List()
		sort.Strings(ids)
		out := make([]cookiecloud.SiteState, 0, len(ids))
		for _, id := range ids {
			meta, ok := registry.Get(id)
			if !ok || (meta.AuthMethod != v2.AuthMethodCookie && meta.AuthMethod != v2.AuthMethodCookieAndAPIKey) {
				continue
			}
			sc := configured[models.SiteGroup(id)]
			base := sc.APIUrl
			if base == "" {
				base = meta.DefaultBaseURL
			}
			out = append(out, cookiecloud.SiteState{
				Name: id, DisplayName: meta.Name, BaseURL: base,
				Enabled: sc.Enabled != nil && *sc.Enabled, Cookie: sc.Cookie,
			})
		}
		return out, nil
	}
}

// applySiteCookies 批量写入站点 Cookie 并启用站点（只改 Cookie 与启用，不动 RSS 等设置；一次配置变更事件，调度随之重新加载），
// 之后刷新站点实例、请求登录探测，与浏览器扩展同步凭据相同。返回写不进去的站点；写进去的照常刷新。
func applySiteCookies(store *core.ConfigStore, mgr *scheduler.Manager) func(context.Context, map[string]string) map[string]error {
	return func(ctx context.Context, cookies map[string]string) map[string]error {
		failed := map[string]error{}
		failAll := func(err error) map[string]error {
			for n := range cookies {
				failed[n] = err
			}
			return failed
		}
		if err := ctx.Err(); err != nil {
			return failAll(fmt.Errorf("已超时，没有写入: %w", err))
		}
		batch := make(map[models.SiteGroup]string, len(cookies))
		for n, c := range cookies {
			batch[models.SiteGroup(n)] = c
		}
		bad, err := store.SetSiteCookies(ctx, batch)
		if err != nil {
			return failAll(fmt.Errorf("保存站点 Cookie 失败: %w", err))
		}
		saved := make([]string, 0, len(cookies))
		for n := range cookies {
			if e, ok := bad[models.SiteGroup(n)]; ok {
				failed[n] = e
				continue
			}
			saved = append(saved, n)
		}
		if len(saved) == 0 {
			return failed
		}
		sort.Strings(saved)
		global.GetSlogger().Infof("[CookieCloud] 已写入 %d 个站点的 Cookie: %v", len(saved), saved)
		if err := web.RefreshSiteRegistrations(store); err != nil {
			global.GetSlogger().Warnf("[CookieCloud] 刷新站点注册失败: %v", err)
		} else if mon := mgr.GetLoginReminderMonitor(); mon != nil {
			for _, n := range saved {
				if err := mon.RequestProbe(n); err != nil {
					global.GetSlogger().Warnf("[CookieCloud] 请求登录探测失败: site=%s err=%v", n, err)
				}
			}
		}
		return failed
	}
}
