package cmd

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
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
