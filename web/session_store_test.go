package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sessionsWith 构造一个预置了会话的 store，供直接拼 Server 的测试使用。
func sessionsWith(sid, username string) *sessionStore {
	s := newSessionStore()
	s.put(sid, username)
	return s
}

func newClockedSessionStore(start time.Time) (*sessionStore, *time.Time) {
	now := start
	s := newSessionStore()
	s.now = func() time.Time { return now }
	return s, &now
}

func TestSessionStore_IdleSessionsExpire(t *testing.T) {
	s, now := newClockedSessionStore(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	s.put("a", "admin")

	*now = now.Add(sessionIdleTimeout - time.Hour)
	user, ok := s.lookup("a")
	require.True(t, ok)
	assert.Equal(t, "admin", user)

	// 上一次访问刷新了 lastSeen，从那之后再过整个空闲期才失效
	*now = now.Add(sessionIdleTimeout + time.Second)
	_, ok = s.lookup("a")
	assert.False(t, ok)
	assert.Equal(t, 0, s.len(), "过期会话在查询时删除")
}

func TestSessionStore_EvictsLeastRecentlyUsedBeyondLimit(t *testing.T) {
	s, now := newClockedSessionStore(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	for i := range sessionMaxCount {
		s.put(fmt.Sprintf("sid-%d", i), "admin")
		*now = now.Add(time.Second)
	}
	s.put("newest", "admin")

	assert.Equal(t, sessionMaxCount, s.len())
	assert.False(t, s.valid("sid-0"), "最久没用的会话被淘汰")
	assert.True(t, s.valid("sid-1"))
	assert.True(t, s.valid("newest"))
}

func TestSessionStore_RemoveAllExcept(t *testing.T) {
	s := newSessionStore()
	s.put("keep", "admin")
	s.put("b", "admin")
	s.put("c", "admin")

	assert.Equal(t, 2, s.removeAllExcept("keep"))
	assert.True(t, s.valid("keep"))
	assert.False(t, s.valid("b"))
	assert.False(t, s.valid("c"))
}

func TestSessionStore_NilIsEmpty(t *testing.T) {
	var s *sessionStore
	assert.False(t, s.valid("x"))
	s.remove("x")
	assert.Equal(t, 0, s.removeAllExcept(""))
}

// 登录、登出和鉴权并发执行时不能出现 map 并发读写（-race 下此前必报 DATA RACE）。
func TestServer_ConcurrentLoginLogoutAndAuth(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("admin", hashPassword("secret")))
	srv.sessions.put("steady", "admin")
	protected := srv.auth(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			for range 3 {
				body, _ := json.Marshal(map[string]string{"username": "admin", "password": "secret"})
				req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				srv.loginHandler(w, req)
				assert.Equal(t, http.StatusOK, w.Code)
				for _, c := range w.Result().Cookies() {
					out := httptest.NewRequest(http.MethodGet, "/logout", nil)
					out.AddCookie(c)
					srv.logoutHandler(httptest.NewRecorder(), out)
				}
			}
		})
	}
	for range 3 {
		wg.Go(func() {
			for range 200 {
				req := httptest.NewRequest(http.MethodGet, "/api/global", nil)
				req.AddCookie(&http.Cookie{Name: "session", Value: "steady"})
				w := httptest.NewRecorder()
				protected(w, req)
				assert.Equal(t, http.StatusOK, w.Code)
			}
		})
	}
	wg.Wait()
}

// 修改密码后，除发起修改的会话外，其他会话立即失效。
func TestAPIPassword_RevokesOtherSessions(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("admin", hashPassword("oldpass")))
	srv.sessions.put("current", "admin")
	srv.sessions.put("stolen", "admin")

	body, _ := json.Marshal(map[string]string{"Username": "admin", "Old": "oldpass", "New": "newpass"})
	req := httptest.NewRequest(http.MethodPost, "/api/password", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "session", Value: "current"})
	w := httptest.NewRecorder()
	srv.apiPassword(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	assert.True(t, srv.sessions.valid("current"))
	assert.False(t, srv.sessions.valid("stolen"))
}

// 空口令或只有空白的口令会让账号再也登录不了，接口直接拒绝，原口令保持不变。
func TestAPIPassword_RejectsBlankNewPassword(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("admin", hashPassword("oldpass")))

	for _, newPw := range []string{"", "   "} {
		body, _ := json.Marshal(map[string]string{"Username": "admin", "Old": "oldpass", "New": newPw})
		w := httptest.NewRecorder()
		srv.apiPassword(w, httptest.NewRequest(http.MethodPost, "/api/password", bytes.NewReader(body)))
		assert.Equal(t, http.StatusBadRequest, w.Code, "new=%q", newPw)
	}
	u, err := srv.store.GetAdmin("admin")
	require.NoError(t, err)
	assert.True(t, verifyPassword(u.PasswordHash, "oldpass"))
}

// 带首尾空白的新口令按去掉空白后的值保存，与登录时的处理一致。
func TestAPIPassword_TrimsLikeLogin(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("admin", hashPassword("oldpass")))

	body, _ := json.Marshal(map[string]string{"Username": "admin", "Old": "oldpass", "New": "  newpass  "})
	w := httptest.NewRecorder()
	srv.apiPassword(w, httptest.NewRequest(http.MethodPost, "/api/password", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code)

	login, _ := json.Marshal(map[string]string{"username": "admin", "password": "  newpass  "})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(login))
	req.Header.Set("Content-Type", "application/json")
	lw := httptest.NewRecorder()
	srv.loginHandler(lw, req)
	assert.Equal(t, http.StatusOK, lw.Code)
}
