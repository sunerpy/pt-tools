package cmd

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
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

// qB 兼容入口（路线图 M13）：在单独的端口上开，和 Web 端口分开。地址按 --qbit-compat-addr、环境变量 PT_QBIT_COMPAT_ADDR、
// 网页上存的（「qB 兼容入口」页）的顺序取，都没有时不开；网页上改了地址时马上换监听。

var qbitCompatAddr string

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

// compatInstance 是兼容入口取下载器实例用的：从下载器管理器按名字取。
func compatInstance(mgr *scheduler.Manager) func(ctx context.Context, name string) (downloader.Downloader, error) {
	return func(ctx context.Context, name string) (downloader.Downloader, error) {
		dm := mgr.GetDownloaderManager()
		if dm == nil {
			return nil, errors.New("下载器管理器没有初始化")
		}
		return dm.GetDownloaderContext(ctx, name)
	}
}

// compatServer 是兼容入口的一次监听：一个 http.Server。关闭时先取消进行中请求的上下文（添加种子的循环、下载器请求跟着停），
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

// compatControl 管兼容入口的监听：启动时按启动参数、环境变量或网页上存的地址开（前两个优先）；网页上改了地址时换监听
// （地址由启动参数或环境变量给的时候不换）。每次监听用一个新的 http.Server，兼容入口本身（会话、登录锁）不变。
type compatControl struct {
	qc *qbitcompat.Server
	// stopTimeout 是换地址时最多等旧监听上的请求收尾多久（测试里改短）
	stopTimeout time.Duration

	mu     sync.Mutex
	cur    *compatServer
	addr   string // 正在监听的地址
	want   string // 要监听的地址
	source string // want 从哪来：flag、env、settings
	err    error  // want 监听失败的原因
	closed bool
	// gen 每次 Apply 加一：等旧监听收尾（在锁外）的时候又改了设置，就不再按这一次的地址开
	gen uint64
	// draining 是换地址时还在收尾的旧监听（Shutdown 也等它们）
	draining sync.WaitGroup
}

// qbitCompatListenAddr 是兼容入口的监听地址与它从哪来：参数优先，其次环境变量，再次网页上存的；都没有时不开。
func qbitCompatListenAddr(settingsAddr string) (string, string) {
	if a := strings.TrimSpace(qbitCompatAddr); a != "" {
		return a, "flag"
	}
	if a := strings.TrimSpace(os.Getenv("PT_QBIT_COMPAT_ADDR")); a != "" {
		return a, "env"
	}
	if a := strings.TrimSpace(settingsAddr); a != "" {
		return a, "settings"
	}
	return "", ""
}

// newQbitCompat 建兼容入口与它的监听管理，交给 Web 服务（设置页用），还不监听（见 start）。
func newQbitCompat(db *gorm.DB, srv *web.Server, mgr *scheduler.Manager, store *core.ConfigStore, tokens *apitoken.Store, audit app.AuditService) *compatControl {
	qc := qbitcompat.New(qbitcompat.Deps{
		DB: db, Tokens: tokens, Audit: audit, Instance: compatInstance(mgr),
		Push: internal.PushTorrentToDownloader,
		Site: enabledSite(store),
	})
	c := &compatControl{qc: qc, stopTimeout: 10 * time.Second}
	var saved string
	if db != nil {
		row, err := qbitcompat.LoadSettings(context.Background(), db)
		if err != nil {
			global.GetSlogger().Warnf("读取 qB 兼容入口设置失败，按没设监听地址处理: %v", err)
		}
		saved = row.ListenAddr
	}
	c.want, c.source = qbitCompatListenAddr(saved)
	srv.SetQbitCompat(qc)
	srv.SetQbitCompatControl(c)
	return c
}

// start 按启动时定下的地址开始监听（信号处理装好以后调用）。监听失败只记日志，不影响 Web 服务。
func (c *compatControl) start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.want != "" && !c.closed {
		_ = c.listenLocked(c.want)
	}
}

// Runtime 是现在的监听情况。
func (c *compatControl) Runtime() web.QbitCompatRuntime {
	c.mu.Lock()
	defer c.mu.Unlock()
	rt := web.QbitCompatRuntime{Addr: c.addr, Want: c.want, Source: c.source}
	if c.err != nil {
		rt.Error = c.err.Error()
	}
	return rt
}

// Apply 按网页上设的地址重新监听（空串是关掉）：地址没变、正在监听时不动；地址由启动参数或环境变量给的时候不动。
// 旧的监听先从锁里摘下来，在锁外等它收尾（最多 stopTimeout），再开新的：等的时候 Runtime、Shutdown 不被挡住；
// 这期间退出了或者又改了设置，就不再开。
func (c *compatControl) Apply(addr string) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("pt-tools 正在退出")
	}
	if c.source == "flag" || c.source == "env" || (addr == c.want && (addr == "" || c.cur != nil)) {
		c.mu.Unlock()
		return nil
	}
	old := c.cur
	c.cur, c.addr = nil, ""
	c.want, c.source, c.err = addr, "", nil
	if addr != "" {
		c.source = "settings"
	}
	c.gen++
	gen := c.gen
	if old != nil {
		c.draining.Add(1)
	}
	c.mu.Unlock()
	if old != nil {
		c.qc.SetAddr("")
		c.drain(old)
	}
	if addr == "" {
		global.GetSlogger().Info("qB 兼容入口已按设置关闭")
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.closed:
		return errors.New("pt-tools 正在退出")
	case c.gen != gen:
		// 等旧监听收尾的时候设置又改了：按新的那次开
		return nil
	}
	return c.listenLocked(addr)
}

// drain 等换下来的旧监听收尾（最多 stopTimeout，等不到就强行断开）。
func (c *compatControl) drain(old *compatServer) {
	defer c.draining.Done()
	ctx, cancel := context.WithTimeout(context.Background(), c.stopTimeout)
	defer cancel()
	if err := old.Shutdown(ctx); err != nil {
		global.GetSlogger().Warnf("qB 兼容入口换监听时没等到旧监听上的请求收尾: %v", err)
	}
}

// Shutdown 停止监听（退出时）：先标记关闭（之后的 Apply 不再开），再在锁外关掉现在的监听，并等换地址时还在收尾的旧监听，
// 都以 ctx 为限。
func (c *compatControl) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	c.closed = true
	cur := c.cur
	c.cur, c.addr = nil, ""
	c.mu.Unlock()
	c.qc.SetAddr("")
	var err error
	if cur != nil {
		err = cur.Shutdown(ctx)
	}
	done := make(chan struct{})
	go func() {
		c.draining.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		if err == nil {
			err = ctx.Err()
		}
	}
	return err
}

func (c *compatControl) listenLocked(addr string) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		c.err = err
		global.GetSlogger().Errorf("qB 兼容入口监听 %s 失败，没有开启: %v", addr, err)
		return err
	}
	base, cancel := context.WithCancel(context.Background())
	hs := &compatServer{cancel: cancel, Server: &http.Server{
		Handler: c.qc.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Minute, IdleTimeout: 2 * time.Minute,
		BaseContext: func(net.Listener) context.Context { return base },
	}}
	c.cur, c.addr, c.err = hs, ln.Addr().String(), nil
	c.qc.SetAddr(c.addr)
	go func() {
		if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			global.GetSlogger().Errorf("qB 兼容入口停止: %v", err)
		}
	}()
	global.GetSlogger().Infof("qB 兼容入口启动于 %s", c.addr)
	return nil
}
