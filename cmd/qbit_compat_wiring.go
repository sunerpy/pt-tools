package cmd

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/web"
	"github.com/sunerpy/pt-tools/web/qbitcompat"
)

// qB 兼容入口（路线图 M13）：给了 --qbit-compat-addr 或环境变量 PT_QBIT_COMPAT_ADDR 时才在那个地址上开，和 Web 端口分开。

var qbitCompatAddr string

// qbitCompatListenAddr 是兼容入口的监听地址：参数优先，其次环境变量；都没给时不开。
func qbitCompatListenAddr() string {
	if a := strings.TrimSpace(qbitCompatAddr); a != "" {
		return a
	}
	return strings.TrimSpace(os.Getenv("PT_QBIT_COMPAT_ADDR"))
}

// enabledSite 是兼容入口经站点下载种子用的：站点要已启用，再从搜索编排器取实例。
func enabledSite(store *core.ConfigStore) func(id string) v2.Site {
	return func(id string) v2.Site {
		sites, err := store.ListSites()
		if err != nil {
			return nil
		}
		sc, ok := sites[models.SiteGroup(id)]
		if !ok || sc.Enabled == nil || !*sc.Enabled {
			return nil
		}
		orch := web.GetSearchOrchestrator()
		if orch == nil {
			return nil
		}
		return orch.GetSite(id)
	}
}

// compatServer 是兼容入口的 HTTP 服务。关闭时先取消进行中请求的上下文（添加种子的循环、下载器请求跟着停），
// 再等它们退出；等不到就强行断开连接，后面关下载器时不再有请求在用它们。
type compatServer struct {
	*http.Server
	cancel context.CancelFunc
}

func (c *compatServer) Shutdown(ctx context.Context) error {
	c.cancel()
	err := c.Server.Shutdown(ctx)
	if err != nil {
		_ = c.Close()
	}
	return err
}

// startQbitCompat 开兼容入口。监听失败只记日志，不影响 Web 服务；返回的 server 交给关闭流程（没开时是 nil）。
func startQbitCompat(addr string, db *gorm.DB, srv *web.Server, mgr *scheduler.Manager, store *core.ConfigStore, tokens *apitoken.Store, audit app.AuditService) *compatServer {
	qc := qbitcompat.New(qbitcompat.Deps{
		DB: db, Tokens: tokens, Audit: audit,
		Instance: func(ctx context.Context, name string) (downloader.Downloader, error) {
			dm := mgr.GetDownloaderManager()
			if dm == nil {
				return nil, errors.New("下载器管理器没有初始化")
			}
			return dm.GetDownloaderContext(ctx, name)
		},
		Push: internal.PushTorrentToDownloader,
		Site: enabledSite(store),
	})
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		global.GetSlogger().Errorf("qB 兼容入口监听 %s 失败，没有开启: %v", addr, err)
		return nil
	}
	qc.SetAddr(ln.Addr().String())
	srv.SetQbitCompat(qc)
	base, cancel := context.WithCancel(context.Background())
	hs := &compatServer{cancel: cancel, Server: &http.Server{
		Handler: qc.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Minute, IdleTimeout: 2 * time.Minute,
		BaseContext: func(net.Listener) context.Context { return base },
	}}
	go func() {
		if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			global.GetSlogger().Errorf("qB 兼容入口停止: %v", err)
		}
	}()
	global.GetSlogger().Infof("qB 兼容入口启动于 %s", ln.Addr())
	return hs
}
