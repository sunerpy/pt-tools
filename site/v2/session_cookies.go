package v2

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"weak"
)

// 会续期的登录会话（见 SiteDefinition.SessionCookies）。BYR 登录后是 auth_token（JWT，15 分钟）、refresh_token、session_id：
// auth_token 过期后，站点在下一个请求里用 refresh_token 换发一对新的，refresh_token 也换（2026-10-29 实测）。
// pt-tools 以前一直发存着的那组、不理 Set-Cookie：过期后第一个请求续上了，之后还拿旧的 refresh_token，就掉线了。
// 现在这类站点的请求一个接一个发（两个请求不会拿同一个旧令牌各续一次），响应里换了这些 Cookie 就用新值接着请求，
// 并交给 SessionCookieSink 存回站点设置，重启以后接着用。
//
// 一个站点在进程里同时有好几个实例：用户信息与搜索共用的那个、登录探测和 RSS 取详情时每次新建的。各管各的令牌的话，
// 一个实例续了期，别的实例手里的 refresh_token 就作废了（站点只认最新一个时），pt-tools 自己把自己挤下线。
// 所以会话按 Cookie 共用：新实例拿到的 Cookie 是某个会话用过的（多半是从设置里读的、最近存回去的那组），就接进那个会话，
// 排同一个队、用同一组令牌；谁都没用过的（扩展刚推来的、验证时的候选）另起一个会话，不影响在用的那个。

// SessionCookieSink 存续期后换发的那组 Cookie：chain 是这个会话用过的 Cookie（旧的在前，最后一个就是 renewed）。
// 站点设置里存着的是 chain 里的一个时换成 renewed；已经是别的（扩展推来了新的、用户改了）时不要覆盖，返回 stored=false。
// cmd 里接到 ConfigStore。
type SessionCookieSink func(siteID string, chain []string, renewed string) (stored bool, err error)

var sessionCookieSink atomic.Pointer[SessionCookieSink]

// SetSessionCookieSink 设置存续期 Cookie 的方法；nil 表示不存（只在这个进程里接着用新值）。
func SetSessionCookieSink(fn SessionCookieSink) {
	if fn == nil {
		sessionCookieSink.Store(nil)
		return
	}
	sessionCookieSink.Store(&fn)
}

// SessionCookieHolder 是有续期会话的站点或驱动：CurrentCookie 是现在用的那组（续期过就是换发后的）；没有续期会话时 ok 为假。
type SessionCookieHolder interface {
	CurrentCookie() (cookie string, ok bool)
}

// CurrentSessionCookie 取站点现在用的那组 Cookie（续期过就是换发后的）；站点没有续期会话时 ok 为假。
// 验证扩展推来的 Cookie 时要用它：验证那次请求要是续了期，原来那组的 refresh_token 已经用掉了，要存换发后的。
func CurrentSessionCookie(site Site) (string, bool) {
	if h, ok := site.(SessionCookieHolder); ok {
		return h.CurrentCookie()
	}
	return "", false
}

// sessionHistoryLimit 是一个会话记住的 Cookie 个数：auth_token 15 分钟换一次，64 个约 16 小时，
// 足够认出设置里存着的那组（每次续期都存回去）。更早的忘掉，再拿来建实例就另起一个会话。
const sessionHistoryLimit = 64

// sessionCookies 是一个续期会话：一次只发一个请求；cookie 是现在用的那组，history 是用过的（旧的在前，最后一个就是 cookie）。
// 同一站点、拿着其中任一组 Cookie 建的实例共用它（见 joinSessionCookies）。
type sessionCookies struct {
	mu      sync.Mutex
	siteID  string
	names   []string
	cookie  string
	history []string
}

// sessionPool 按「站点 + Cookie」找会话。存弱引用：会话只由站点实例持有，实例都没了就回收，条目随后清掉。
var sessionPool = struct {
	mu sync.Mutex
	m  map[string]weak.Pointer[sessionCookies]
}{m: map[string]weak.Pointer[sessionCookies]{}}

// sessionKey 不拿 Cookie 原文当键：键里只有站点与 Cookie 的摘要。
func sessionKey(siteID, cookie string) string {
	sum := sha256.Sum256([]byte(cookie))
	return siteID + "\x00" + hex.EncodeToString(sum[:16])
}

// joinSessionCookies 返回拿着 cookie 的实例该用的会话：还在用的会话里有一个用过这组 Cookie 就接进去，否则新建一个。
func joinSessionCookies(siteID string, names []string, cookie string) *sessionCookies {
	key := sessionKey(siteID, cookie)
	sessionPool.mu.Lock()
	defer sessionPool.mu.Unlock()
	if s := sessionPool.m[key].Value(); s != nil {
		return s
	}
	s := &sessionCookies{siteID: siteID, names: slices.Clone(names), cookie: cookie, history: []string{cookie}}
	sessionPool.m[key] = weak.Make(s)
	pruneSessionPoolLocked()
	return s
}

// pruneSessionPoolLocked 清掉会话已经回收的条目（调用方持 sessionPool.mu）；条目不多时不扫。
func pruneSessionPoolLocked() {
	if len(sessionPool.m) < 256 {
		return
	}
	for k, wp := range sessionPool.m {
		if wp.Value() == nil {
			delete(sessionPool.m, k)
		}
	}
}

// remember 记下会话换上的新 Cookie（调用方持 s.mu）：之后拿这组 Cookie 建的实例接进这个会话；超过上限时忘掉最旧的。
func (s *sessionCookies) remember(cookie string) {
	s.history = append(s.history, cookie)
	var forget []string
	if over := len(s.history) - sessionHistoryLimit; over > 0 {
		forget = slices.Clone(s.history[:over])
		s.history = slices.Clone(s.history[over:])
	}
	sessionPool.mu.Lock()
	defer sessionPool.mu.Unlock()
	sessionPool.m[sessionKey(s.siteID, cookie)] = weak.Make(s)
	for _, old := range forget {
		if k := sessionKey(s.siteID, old); sessionPool.m[k].Value() == s {
			delete(sessionPool.m, k)
		}
	}
	pruneSessionPoolLocked()
}

func (s *sessionCookies) current() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cookie
}

// send 用现在的 Cookie 发一个请求；响应里换了会话 Cookie 时换上新值并存回站点设置。整个过程持锁，同一会话的请求排队。
func (s *sessionCookies) send(do func(cookie string) (*HTTPResponse, error)) (*HTTPResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	resp, err := do(s.cookie)
	if resp == nil {
		return resp, err
	}
	renewed, changed := mergeSessionCookies(s.cookie, resp.Headers, s.names, time.Now())
	if !changed {
		return resp, err
	}
	s.cookie = renewed
	s.remember(renewed)
	// 存没存上由 sink 自己记日志；没存上时这个进程里照样接着用新的
	if sink := sessionCookieSink.Load(); sink != nil {
		_, _ = (*sink)(s.siteID, slices.Clone(s.history), renewed)
	}
	return resp, err
}

// mergeSessionCookies 把响应 Set-Cookie 里 names 列着的、有值且没过期的 Cookie 换进 cookie 串（没有的接在后面），
// 返回新串与有没有变。删 Cookie 的（值为空、Max-Age<0、Expires 已过）不照做：续期失败时站点会清登录 Cookie，
// 存着的那组留着，等扩展推来新的。
func mergeSessionCookies(cookie string, header http.Header, names []string, now time.Time) (string, bool) {
	renewed := map[string]string{}
	var order []string
	for _, line := range header.Values("Set-Cookie") {
		c, err := http.ParseSetCookie(line)
		if err != nil || !slices.Contains(names, c.Name) || c.Value == "" || c.MaxAge < 0 ||
			(!c.Expires.IsZero() && !c.Expires.After(now)) {
			continue
		}
		if _, seen := renewed[c.Name]; !seen {
			order = append(order, c.Name)
		}
		renewed[c.Name] = c.Value
	}
	if len(renewed) == 0 {
		return cookie, false
	}
	parts := strings.Split(cookie, ";")
	applied := map[string]bool{}
	changed := false
	out := make([]string, 0, len(parts)+len(renewed))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		name, value, _ := strings.Cut(p, "=")
		name = strings.TrimSpace(name)
		if v, ok := renewed[name]; ok && !applied[name] {
			applied[name] = true
			if v != value {
				changed = true
			}
			p = name + "=" + v
		}
		out = append(out, p)
	}
	for _, name := range order {
		if !applied[name] {
			out = append(out, name+"="+renewed[name])
			changed = true
		}
	}
	if !changed {
		return cookie, false
	}
	return strings.Join(out, "; "), true
}
