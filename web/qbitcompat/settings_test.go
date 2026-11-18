package qbitcompat

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/apitoken"
)

// 监听地址：空串是不开；「地址:端口」，地址是 IP、localhost 或留空
func TestCleanListenAddr(t *testing.T) {
	for in, want := range map[string]string{
		"":                  "",
		" 0.0.0.0:8081 ":    "0.0.0.0:8081",
		":8081":             ":8081",
		"[::]:8081":         "[::]:8081",
		"localhost:9000":    "localhost:9000",
		"192.168.1.10:8081": "192.168.1.10:8081",
	} {
		got, err := CleanListenAddr(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"8081", "0.0.0.0", "0.0.0.0:0", "0.0.0.0:65536", "0.0.0.0:http", "nas.local:8081"} {
		_, err := CleanListenAddr(bad)
		require.ErrorIs(t, err, ErrInvalid, bad)
	}
}

// 存设置：带了监听地址时存下（检查格式），不带时存着的不变
func TestSaveListenAddr(t *testing.T) {
	e := newEnv(t)
	addr := " 0.0.0.0:8081 "
	row, err := SaveSettings(context.Background(), e.db, Settings{ListenAddr: &addr})
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:8081", row.ListenAddr)
	row, err = SaveSettings(context.Background(), e.db, Settings{FullControl: true})
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:8081", row.ListenAddr)
	assert.True(t, row.FullControl)
	bad := "8081"
	_, err = SaveSettings(context.Background(), e.db, Settings{ListenAddr: &bad})
	require.ErrorIs(t, err, ErrInvalid)
	empty := ""
	row, err = SaveSettings(context.Background(), e.db, Settings{ListenAddr: &empty})
	require.NoError(t, err)
	assert.Empty(t, row.ListenAddr)
}

// 最近一次成功、失败的登录与登录着的会话数；失败太多次被锁时记成 locked
func TestLoginRecords(t *testing.T) {
	e := newEnv(t)
	ok, fail := e.srv.Logins()
	assert.Nil(t, ok)
	assert.Nil(t, fail)
	e.login(e.token(apitoken.ScopeQbitCompat))
	ok, _ = e.srv.Logins()
	require.NotNil(t, ok)
	assert.Equal(t, "ok", ok.Result)
	assert.Equal(t, "moviepilot", ok.Username)
	assert.NotZero(t, ok.TokenID)
	assert.Equal(t, 1, e.srv.Sessions())

	for range loginMaxFails {
		e.do(http.MethodPost, "/api/v2/auth/login", url.Values{"username": {"x"}, "password": {"ptt_bad"}}, nil)
	}
	_, fail = e.srv.Logins()
	require.NotNil(t, fail)
	assert.Equal(t, "bad_credentials", fail.Result)
	w := e.do(http.MethodPost, "/api/v2/auth/login", url.Values{"password": {"ptt_bad"}}, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	_, fail = e.srv.Logins()
	assert.Equal(t, "locked", fail.Result)

	e.now = e.now.Add(2 * time.Hour)
	assert.Zero(t, e.srv.Sessions(), "1 小时没用的会话不算")
}
