package qbitcompat

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
)

const (
	sessionTTL = time.Hour
	maxSession = 256

	loginWindow   = 15 * time.Minute
	loginMaxFails = 5
	loginBan      = 15 * time.Minute
	maxLockIPs    = 10000
)

// session 是登录以后的会话：令牌编号（每次请求复查）与客户端填的用户名（只记审计）。
type session struct {
	tokenID  uint
	username string
	seen     time.Time
}

// sessions 是内存里的 SID 表：滑动 1 小时过期，最多 256 个，超出时淘汰最久没用的。重启以后客户端重新登录。
type sessions struct {
	mu sync.Mutex
	m  map[string]*session
}

func newSessions() *sessions { return &sessions{m: map[string]*session{}} }

func (s *sessions) put(sid string, sess *session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.m {
		if sess.seen.Sub(v.seen) > sessionTTL {
			delete(s.m, k)
		}
	}
	for len(s.m) >= maxSession {
		var oldest string
		var at time.Time
		for k, v := range s.m {
			if oldest == "" || v.seen.Before(at) {
				oldest, at = k, v.seen
			}
		}
		delete(s.m, oldest)
	}
	s.m[sid] = sess
}

func (s *sessions) get(sid string, now time.Time) (session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[sid]
	if !ok {
		return session{}, false
	}
	if now.Sub(v.seen) > sessionTTL {
		delete(s.m, sid)
		return session{}, false
	}
	v.seen = now
	return *v, true
}

func (s *sessions) drop(sid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, sid)
}

// loginLock 是登录失败的锁定：同一 IP 15 分钟内失败 5 次，锁 15 分钟。
type loginLock struct {
	mu sync.Mutex
	m  map[string]*lockEntry
}

type lockEntry struct {
	fails       int
	windowStart time.Time
	lockedUntil time.Time
}

func newLoginLock() *loginLock { return &loginLock{m: map[string]*lockEntry{}} }

func (l *loginLock) locked(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.m[ip]
	return ok && now.Before(e.lockedUntil)
}

func (l *loginLock) fail(ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.m) >= maxLockIPs {
		for k, e := range l.m {
			if now.Sub(e.windowStart) > loginWindow && !now.Before(e.lockedUntil) {
				delete(l.m, k)
			}
		}
	}
	e, ok := l.m[ip]
	if !ok || now.Sub(e.windowStart) > loginWindow {
		e = &lockEntry{windowStart: now}
		l.m[ip] = e
	}
	e.fails++
	if e.fails >= loginMaxFails {
		e.lockedUntil = now.Add(loginBan)
		e.fails, e.windowStart = 0, now
	}
}

func (l *loginLock) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, ip)
}

func newSID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// login 是 POST /api/v2/auth/login：password 是有 qbit:compat 的 API 令牌，username 随意。
// 成功回 Ok. 与 SID；失败回 200 Fails.；锁定时回 403（都是 qB 的行为）。
func (s *Server) login(w http.ResponseWriter, r *http.Request, _ *call) {
	ip := clientIP(r)
	now := s.deps.Now()
	if s.lock.locked(ip, now) {
		text(w, http.StatusForbidden, "Your IP address has been banned for too many failed login attempts.")
		return
	}
	if err := r.ParseForm(); err != nil {
		text(w, http.StatusBadRequest, "Bad Request")
		return
	}
	username := truncate(strings.TrimSpace(r.PostForm.Get("username")), 64)
	tok, err := s.deps.Tokens.Verify(r.Context(), strings.TrimSpace(r.PostForm.Get("password")))
	if err != nil && !errors.Is(err, apitoken.ErrUnauthorized) {
		global.GetSlogger().Warnf("[qB 兼容] 校验令牌失败: %v", err)
		text(w, http.StatusServiceUnavailable, "暂时不能校验令牌")
		return
	}
	if err != nil || !tok.Has(apitoken.ScopeQbitCompat) {
		s.lock.fail(ip, now)
		reason := "denied:bad_credentials"
		if err == nil {
			reason = "denied:scope"
		}
		s.record(r, app.AuditEntry{
			ChannelUserID: "ip:" + ip, Command: "POST /api/v2/auth/login", Result: reason,
			Args: map[string]any{"username": username},
		})
		text(w, http.StatusOK, "Fails.")
		return
	}
	sid, err := newSID()
	if err != nil {
		text(w, http.StatusInternalServerError, "生成会话失败")
		return
	}
	s.lock.reset(ip)
	s.sessions.put(sid, &session{tokenID: tok.ID, username: username, seen: now})
	http.SetCookie(w, &http.Cookie{Name: "SID", Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	text(w, http.StatusOK, "Ok.")
}

// logout 是 POST /api/v2/auth/logout：删掉这个会话。
func (s *Server) logout(w http.ResponseWriter, r *http.Request, _ *call) {
	if ck, err := r.Cookie("SID"); err == nil {
		s.sessions.drop(ck.Value)
	}
	text(w, http.StatusOK, "")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}
