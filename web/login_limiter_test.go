package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func postLogin(srv *Server, remote, user, pass string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remote
	w := httptest.NewRecorder()
	srv.loginHandler(w, req)
	return w
}

// 同一 IP 连续失败到上限后暂停登录（连正确口令也拒绝，且不再做口令校验），其他 IP 不受影响。
func TestLoginHandler_LocksOutAfterRepeatedFailures(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("admin", hashPassword("secret")))
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	srv.logins.now = func() time.Time { return now }

	for i := range loginMaxFailures {
		w := postLogin(srv, "203.0.113.7:5000", "admin", "wrong")
		require.Equal(t, http.StatusUnauthorized, w.Code, "attempt %d", i+1)
	}

	w := postLogin(srv, "203.0.113.7:5001", "admin", "secret")
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "15 分钟")

	other := postLogin(srv, "198.51.100.2:6000", "admin", "secret")
	assert.Equal(t, http.StatusOK, other.Code, "其他 IP 不受影响")

	now = now.Add(loginLockout + time.Second)
	w = postLogin(srv, "203.0.113.7:5002", "admin", "secret")
	assert.Equal(t, http.StatusOK, w.Code, "暂停期过后恢复")
}

// 不存在的用户名同样计入失败次数，避免借此无限试探。
func TestLoginHandler_UnknownUserCountsAsFailure(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("admin", hashPassword("secret")))
	for range loginMaxFailures {
		postLogin(srv, "203.0.113.9:1", "ghost", "x")
	}
	assert.Equal(t, http.StatusTooManyRequests, postLogin(srv, "203.0.113.9:1", "admin", "secret").Code)
}

func TestLoginHandler_SuccessResetsFailures(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("admin", hashPassword("secret")))
	for range loginMaxFailures - 1 {
		postLogin(srv, "203.0.113.10:1", "admin", "wrong")
	}
	require.Equal(t, http.StatusOK, postLogin(srv, "203.0.113.10:1", "admin", "secret").Code)
	for range loginMaxFailures - 1 {
		postLogin(srv, "203.0.113.10:1", "admin", "wrong")
	}
	assert.Equal(t, http.StatusOK, postLogin(srv, "203.0.113.10:1", "admin", "secret").Code,
		"成功登录后失败次数清零")
}

func TestLoginHandler_RejectsOversizedBody(t *testing.T) {
	srv := setupServer(t)
	body := `{"username":"admin","password":"` + strings.Repeat("a", loginMaxBodyBytes) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.loginHandler(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLoginLimiter_AcquireVerifyHonoursContext(t *testing.T) {
	l := newLoginLimiter()
	var releases []func()
	for range loginVerifyConcurrency {
		release, err := l.acquireVerify(context.Background())
		require.NoError(t, err)
		releases = append(releases, release)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := l.acquireVerify(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded, "名额占满时按请求 ctx 放弃等待")

	releases[0]()
	release, err := l.acquireVerify(context.Background())
	require.NoError(t, err)
	release()
	for _, r := range releases[1:] {
		r()
	}
}

func TestLoginLimiter_BoundsTrackedIPs(t *testing.T) {
	l := newLoginLimiter()
	for i := range loginMaxTrackedIPs + 10 {
		l.fail(fmt.Sprintf("10.%d.%d.%d", i>>16&255, i>>8&255, i&255))
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	assert.LessOrEqual(t, len(l.failures), loginMaxTrackedIPs)
}

func TestNilLoginLimiterIsNoop(t *testing.T) {
	var l *loginLimiter
	_, blocked := l.blocked("1.2.3.4")
	assert.False(t, blocked)
	l.fail("1.2.3.4")
	l.succeed("1.2.3.4")
	release, err := l.acquireVerify(context.Background())
	require.NoError(t, err)
	release()
}

// /api/qbit 的 GET 不回传下载器密码；POST 空密码沿用已保存的密码。
func TestAPIQbit_DoesNotExposePassword(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	require.NoError(t, srv.store.SaveQbitSettings(models.QbitSettings{
		Enabled: true, URL: "http://127.0.0.1:8080", User: "admin", Password: "qb-secret",
	}))

	w := httptest.NewRecorder()
	srv.apiQbit(w, httptest.NewRequest(http.MethodGet, "/api/qbit", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "qb-secret")
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, true, got["has_password"])
	assert.Equal(t, "admin", got["user"])

	body, _ := json.Marshal(map[string]any{"enabled": true, "url": "http://127.0.0.1:9090", "user": "admin", "password": ""})
	pw := httptest.NewRecorder()
	srv.apiQbit(pw, httptest.NewRequest(http.MethodPost, "/api/qbit", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, pw.Code, pw.Body.String())

	cur, err := srv.store.GetQbitSettings()
	require.NoError(t, err)
	assert.Equal(t, "qb-secret", cur.Password, "空密码表示沿用旧密码")
	assert.Equal(t, "http://127.0.0.1:9090", cur.URL)
}
