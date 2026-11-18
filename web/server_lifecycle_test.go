package web

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 收到关闭信号时 Serve 可能还没起来：原来 Shutdown 看到 httpServer 为 nil 就直接返回，
// 随后 Serve 照常监听，进程再也不会退出。现在 Shutdown 先记下「正在关闭」，之后的 Serve 不再监听。
func TestServer_ShutdownBeforeServeDoesNotListen(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	require.NoError(t, srv.Shutdown(context.Background()))

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve("127.0.0.1:0") }()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		_ = srv.Shutdown(context.Background())
		<-errCh
		t.Fatal("Shutdown 之后调用的 Serve 仍在监听")
	}
}

// Serve 与 Shutdown 并发时不能有数据竞争（-race 下原来会报 httpServer 字段的读写竞争），
// 并且 Shutdown 之后 Serve 一定返回。
func TestServer_ShutdownConcurrentWithServe(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve("127.0.0.1:0") }()
	require.NoError(t, srv.Shutdown(context.Background()))

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown 之后 Serve 没有返回")
	}
}
