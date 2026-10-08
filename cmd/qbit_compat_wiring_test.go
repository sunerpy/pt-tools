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

// 监听地址：参数优先，其次环境变量 PT_QBIT_COMPAT_ADDR；都没有时不开
func TestQbitCompatListenAddr(t *testing.T) {
	prev := qbitCompatAddr
	t.Cleanup(func() { qbitCompatAddr = prev })

	qbitCompatAddr = ""
	t.Setenv("PT_QBIT_COMPAT_ADDR", "")
	assert.Empty(t, qbitCompatListenAddr())
	t.Setenv("PT_QBIT_COMPAT_ADDR", " 127.0.0.1:8081 ")
	assert.Equal(t, "127.0.0.1:8081", qbitCompatListenAddr())
	qbitCompatAddr = "0.0.0.0:9091"
	assert.Equal(t, "0.0.0.0:9091", qbitCompatListenAddr(), "参数优先")
}

// 开兼容入口：在给的地址上监听、回 qB 的 403（没登录）；地址不对时只记日志、不开
func TestStartQbitCompat(t *testing.T) {
	global.InitLogger(zap.NewNop())
	srv := web.NewServer(nil, nil)
	hs := startQbitCompat("127.0.0.1:0", nil, srv, nil, nil, nil, nil)
	require.NotNil(t, hs)
	t.Cleanup(func() { _ = hs.Shutdown(context.Background()) })

	addr := srv.QbitCompatAddr()
	require.NotEmpty(t, addr, "监听地址记在兼容入口上，设置页从这里读")
	resp, err := http.Get("http://" + addr + "/api/v2/app/version")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	assert.Nil(t, startQbitCompat("256.0.0.1:99999", nil, web.NewServer(nil, nil), nil, nil, nil, nil))
}

// 关闭：先让进行中的请求停下（请求的上下文跟着结束），再关监听
func TestQbitCompatShutdownCancelsRequests(t *testing.T) {
	global.InitLogger(zap.NewNop())
	srv := web.NewServer(nil, nil)
	hs := startQbitCompat("127.0.0.1:0", nil, srv, nil, nil, nil, nil)
	require.NotNil(t, hs)
	ctxDone := make(chan struct{})
	// 换一个会一直等到上下文结束的处理函数，模拟进行中的添加
	hs.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(ctxDone)
	})
	go func() { _, _ = http.Get("http://" + srv.QbitCompatAddr() + "/api/v2/torrents/add") }()
	time.Sleep(200 * time.Millisecond) // 等请求进到处理函数里（只是让它先到，不是等结果）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	_ = hs.Shutdown(ctx)
	select {
	case <-ctxDone:
	case <-time.After(3 * time.Second):
		t.Fatal("进行中的请求没有被取消")
	}
	assert.Less(t, time.Since(start), 3*time.Second)
}

// 关闭流程先于监听开始（信号处理装好以后、开始监听之前就收到了信号）：listen 照样不报错，但不接请求，端口跟着关掉
func TestQbitCompatListenAfterShutdown(t *testing.T) {
	global.InitLogger(zap.NewNop())
	srv := web.NewServer(nil, nil)
	c := newQbitCompat(nil, srv, nil, nil, nil, nil)
	require.NoError(t, c.Shutdown(context.Background()))
	require.True(t, c.listen("127.0.0.1:0"))
	addr := srv.QbitCompatAddr()
	require.NotEmpty(t, addr)
	assert.Eventually(t, func() bool {
		conn, derr := net.DialTimeout("tcp", addr, time.Second)
		if derr != nil {
			return true
		}
		_ = conn.Close()
		return false
	}, 5*time.Second, 50*time.Millisecond, "已经在关闭：不接请求")
}

// 站点要已启用、搜索编排器里有它才给；没有这个站点、没启用、编排器没起来时都是 nil
func TestEnabledSite(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
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
	global.InitLogger(zap.NewNop())
	srv := web.NewServer(nil, nil)
	hs := startQbitCompat("127.0.0.1:0", nil, srv, nil, nil, nil, nil)
	require.NotNil(t, hs)
	release, entered := make(chan struct{}), make(chan struct{})
	defer close(release)
	hs.Handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
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
	require.Error(t, hs.Shutdown(ctx), "上下文已经结束：等不到请求结束")
	select {
	case err := <-done:
		require.Error(t, err, "连接被强行断开")
	case <-time.After(5 * time.Second):
		t.Fatal("连接没有被断开")
	}
}
