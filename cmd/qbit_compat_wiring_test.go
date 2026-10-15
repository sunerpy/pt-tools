package cmd

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/web"
)

// 监听地址：参数优先，其次环境变量 PT_QBIT_COMPAT_ADDR，再次网页上存的；都没有时不开
func TestQbitCompatListenAddr(t *testing.T) {
	prev := qbitCompatAddr
	t.Cleanup(func() { qbitCompatAddr = prev })

	qbitCompatAddr = ""
	t.Setenv("PT_QBIT_COMPAT_ADDR", "")
	addr, src := qbitCompatListenAddr("")
	assert.Empty(t, addr)
	assert.Empty(t, src)
	addr, src = qbitCompatListenAddr(" 0.0.0.0:8082 ")
	assert.Equal(t, [2]string{"0.0.0.0:8082", "settings"}, [2]string{addr, src})
	t.Setenv("PT_QBIT_COMPAT_ADDR", " 127.0.0.1:8081 ")
	addr, src = qbitCompatListenAddr("0.0.0.0:8082")
	assert.Equal(t, [2]string{"127.0.0.1:8081", "env"}, [2]string{addr, src})
	qbitCompatAddr = "0.0.0.0:9091"
	addr, src = qbitCompatListenAddr("0.0.0.0:8082")
	assert.Equal(t, [2]string{"0.0.0.0:9091", "flag"}, [2]string{addr, src}, "参数优先")
}

// freeAddr 是一个现在没人用的本机地址（拿到以后马上放掉）。
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func newTestCompat(t *testing.T) (*compatControl, *web.Server) {
	t.Helper()
	global.InitLogger(zap.NewNop())
	srv := web.NewServer(nil, nil)
	c := newQbitCompat(nil, srv, nil, nil, nil, nil)
	t.Cleanup(func() { _ = c.Shutdown(context.Background()) })
	return c, srv
}

func closed(addr string) func() bool {
	return func() bool {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			return true
		}
		_ = conn.Close()
		return false
	}
}

// 开兼容入口：在设的地址上监听、回 qB 的 403（没登录）；地址用不了时记下原因、不开
func TestQbitCompatApply(t *testing.T) {
	c, srv := newTestCompat(t)
	assert.Empty(t, srv.QbitCompatAddr(), "没设地址：不开")
	a1 := freeAddr(t)
	require.NoError(t, c.Apply(a1))
	assert.Equal(t, a1, srv.QbitCompatAddr())
	assert.Equal(t, web.QbitCompatRuntime{Addr: a1, Want: a1, Source: "settings"}, c.Runtime())
	resp, err := http.Get("http://" + a1 + "/api/v2/app/version")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, c.Apply(a1), "地址没变：不动")

	// 换地址：旧的关掉，新的开起来
	a2 := freeAddr(t)
	require.NoError(t, c.Apply(a2))
	assert.Equal(t, a2, c.Runtime().Addr)
	assert.Eventually(t, closed(a1), 5*time.Second, 50*time.Millisecond)

	// 端口被占了：记下原因；腾出来以后再存一次就开
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	b := busy.Addr().String()
	require.Error(t, c.Apply(b))
	rt := c.Runtime()
	assert.Empty(t, rt.Addr)
	assert.Equal(t, b, rt.Want)
	assert.NotEmpty(t, rt.Error)
	assert.Eventually(t, closed(a2), 5*time.Second, 50*time.Millisecond, "换地址时旧的先关掉")
	require.NoError(t, busy.Close())
	require.NoError(t, c.Apply(b))
	assert.Equal(t, b, c.Runtime().Addr)
	assert.Empty(t, c.Runtime().Error)

	// 关掉
	require.NoError(t, c.Apply(""))
	assert.Equal(t, web.QbitCompatRuntime{}, c.Runtime())
	assert.Eventually(t, closed(b), 5*time.Second, 50*time.Millisecond)

	require.Error(t, c.Apply("256.0.0.1:99999"))
	assert.NotEmpty(t, c.Runtime().Error)
}

// 地址由启动参数或环境变量给的：启动时按它开，网页上改了也不换
func TestQbitCompatStartLocked(t *testing.T) {
	prev := qbitCompatAddr
	t.Cleanup(func() { qbitCompatAddr = prev })
	a := freeAddr(t)
	qbitCompatAddr = a
	c, srv := newTestCompat(t)
	c.start()
	assert.Equal(t, a, srv.QbitCompatAddr())
	assert.Equal(t, "flag", c.Runtime().Source)
	require.NoError(t, c.Apply(freeAddr(t)))
	assert.Equal(t, a, c.Runtime().Addr, "启动参数给的地址不换")
	require.NoError(t, c.Apply(""))
	assert.Equal(t, a, c.Runtime().Addr)
}

// 关闭：先让进行中的请求停下（请求的上下文跟着结束），再关监听
func TestQbitCompatShutdownCancelsRequests(t *testing.T) {
	c, srv := newTestCompat(t)
	require.NoError(t, c.Apply(freeAddr(t)))
	ctxDone := make(chan struct{})
	// 换一个会一直等到上下文结束的处理函数，模拟进行中的添加
	c.cur.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(ctxDone)
	})
	go func() { _, _ = http.Get("http://" + srv.QbitCompatAddr() + "/api/v2/torrents/add") }()
	time.Sleep(200 * time.Millisecond) // 等请求进到处理函数里（只是让它先到，不是等结果）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	_ = c.Shutdown(ctx)
	select {
	case <-ctxDone:
	case <-time.After(3 * time.Second):
		t.Fatal("进行中的请求没有被取消")
	}
	assert.Less(t, time.Since(start), 3*time.Second)
}

// 关闭流程先于监听开始（信号处理装好以后、开始监听之前就收到了信号）：之后不再开
func TestQbitCompatListenAfterShutdown(t *testing.T) {
	prev := qbitCompatAddr
	t.Cleanup(func() { qbitCompatAddr = prev })
	qbitCompatAddr = freeAddr(t)
	c, srv := newTestCompat(t)
	require.NoError(t, c.Shutdown(context.Background()))
	c.start()
	assert.Empty(t, srv.QbitCompatAddr(), "已经在关闭：不开")
	qbitCompatAddr = ""
	c.source = ""
	require.Error(t, c.Apply(freeAddr(t)))
	assert.Empty(t, srv.QbitCompatAddr())
}

// 站点要已启用、搜索编排器里有它才给；没有这个站点、没启用、编排器没起来时都是 nil
func TestEnabledSite(t *testing.T) {
	db, err := core.OpenTestDB(t)
	require.NoError(t, err)
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "on", Enabled: true}).Error)
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "off", Enabled: false}).Error)
	site := enabledSite(core.NewConfigStore(db))
	assert.Nil(t, site("missing"))
	assert.Nil(t, site("off"))
	assert.Nil(t, site("on"), "编排器没起来")
}

// 下载器管理器没初始化、下载器不存在时都回错误
func TestCompatInstance(t *testing.T) {
	_, err := compatInstance(&scheduler.Manager{})(context.Background(), "qb")
	require.ErrorContains(t, err, "没有初始化")
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	_, err = compatInstance(mgr)(context.Background(), "missing")
	require.Error(t, err)
}

// 关闭时等不到进行中的请求结束（处理函数不理会取消）：强行断开连接，不一直等
func TestQbitCompatShutdownForcesClose(t *testing.T) {
	c, srv := newTestCompat(t)
	require.NoError(t, c.Apply(freeAddr(t)))
	release, entered := make(chan struct{}), make(chan struct{})
	defer close(release)
	c.cur.Handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	})
	done := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + srv.QbitCompatAddr() + "/api/v2/app/version")
		if err == nil {
			_ = resp.Body.Close()
		}
		done <- err
	}()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, c.Shutdown(ctx), "上下文已经结束：等不到请求结束")
	select {
	case err := <-done:
		require.Error(t, err, "连接被强行断开")
	case <-time.After(5 * time.Second):
		t.Fatal("连接没有被断开")
	}
}
