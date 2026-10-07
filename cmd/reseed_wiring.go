package cmd

import (
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/iyuu"
	"github.com/sunerpy/pt-tools/internal/reseed"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// storeCipher 用 ConfigStore 的密钥加解密 IYUU token（与站点 Cookie 同一把 AES 密钥）。
type storeCipher struct{ store *core.ConfigStore }

func (c storeCipher) Encrypt(plain string) (string, error) { return c.store.EncryptCookie(plain) }
func (c storeCipher) Decrypt(text string) (string, error)  { return c.store.DecryptCookie(text) }

// wireReseedWorker 构造并启动 IYUU 辅种后台：加入、校验、回滚复用转移做种的任务（要先接好转移做种后台），
// 从站点下载种子用 UserInfoService 里的共享站点实例（与搜索、登录探测共用限速器）。辅种默认关闭。
func wireReseedWorker(mgr *scheduler.Manager, svc *v2.UserInfoService, store *core.ConfigStore) *scheduler.ReseedWorker {
	if mgr == nil || global.GlobalDB == nil || store == nil {
		return nil
	}
	tw := mgr.GetTransferWorker()
	if tw == nil || tw.Service() == nil {
		global.GetSlogger().Warn("IYUU 辅种跳过初始化：转移做种后台没有启动")
		return nil
	}
	sites := brushSites(svc)
	service := reseed.New(reseed.Config{
		DB:          global.GlobalDB.DB,
		Cipher:      storeCipher{store: store},
		Transfer:    tw.Service(),
		Downloaders: mgr,
		Sites:       sites.BrushSite,
		SiteIDs: func() []string {
			if svc == nil {
				return nil
			}
			return svc.ListSites()
		},
		NewClient: func(token string) *iyuu.Client {
			c := iyuu.New(token)
			if u := qaIYUUBaseURL(); u != "" {
				c.BaseURL = u
			}
			return c
		},
		Logger: global.GetSlogger(),
	})
	w := scheduler.NewReseedWorker(scheduler.ReseedWorkerConfig{Service: service, Transfer: tw, Logger: global.GetSlogger()})
	mgr.SetReseedWorker(w)
	w.Start()
	global.GetSlogger().Info("IYUU 辅种后台已启动")
	return w
}
