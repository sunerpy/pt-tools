// Package qbitcompat 是 qB 兼容入口（路线图 M13）：在单独的端口上实现 qBittorrent WebUI API v2 的一个子集，
// 让 MoviePilot、IYUU、autobrr、Sonarr/Radarr 这类只认 qB 的工具把 pt-tools 当成下载器。
//
// 登录用有 qbit:compat 权限的 API 令牌当密码，换一个内存里的 SID。读接口转给绑定的真实下载器；写接口默认只动经兼容入口加的种子，
// 设置里打开完全控制后不限。添加种子一律先拿到种子文件，再经 internal.PushTorrentToDownloader（磁盘空间保护、站点做种容量照常），
// 链接只认能解析成已启用站点种子编号的，由 pt-tools 自己经站点下载，不去请求客户端给的地址；磁力链接一律拒绝。
// 回应里 tracker 地址的 passkey 会被遮住。所有写操作记操作审计（通道 qbit_compat）。
package qbitcompat

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

const (
	// AppVersion、WebAPIVersion 是报给客户端的 qB 版本：客户端按 4.x 的路径（pause/resume）调用，stop/start 也照样接。
	AppVersion    = "v4.6.7"
	WebAPIVersion = "2.9.3"
	// ChannelType 是审计里的通道。
	ChannelType = "qbit_compat"
	// Source 是经兼容入口加的种子记录的来源。
	Source = "qbit_compat"

	maxFormBody = 1 << 20
	maxAddBody  = 32 << 20
)

// TokenStore 是登录与复查要用的令牌库（apitoken.Store 实现了它）。
type TokenStore interface {
	Verify(ctx context.Context, plain string) (apitoken.Token, error)
	Get(ctx context.Context, id uint) (apitoken.Token, error)
}

// AuditRecorder 记一条操作审计（app.AuditService 实现了它）。
type AuditRecorder interface {
	Record(ctx context.Context, e app.AuditEntry) error
}

// Deps 是兼容入口要的服务，cmd/web.go 里接上。
type Deps struct {
	DB     *gorm.DB
	Tokens TokenStore
	Audit  AuditRecorder
	// Instance 按下载器名字取实例（DownloaderManager.GetDownloaderContext）
	Instance func(ctx context.Context, name string) (downloader.Downloader, error)
	// Push 是推送入口（internal.PushTorrentToDownloader）
	Push func(ctx context.Context, req internal.PushTorrentRequest) (*internal.PushTorrentResult, error)
	// Site 按站点 ID 取已启用站点的实例，没有启用时返回 nil（搜索编排器的 GetSite）
	Site func(id string) v2.Site
	// Resolver 按 tracker 与下载地址认站点
	Resolver *v2.TrackerResolver
	Now      func() time.Time
}

// Server 是兼容入口。
type Server struct {
	deps     Deps
	sessions *sessions
	lock     *loginLock
	rid      atomic.Int64
	addr     atomic.Value // 监听地址，Status 用
}

// New 建一个兼容入口。
func New(d Deps) *Server {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Resolver == nil {
		d.Resolver = v2.NewTrackerResolver()
	}
	return &Server{deps: d, sessions: newSessions(), lock: newLoginLock()}
}

// route 是一条接口：方法、处理函数、是否要登录。
type route struct {
	method string
	h      func(w http.ResponseWriter, r *http.Request, c *call)
	public bool
}

// call 是一次已登录的请求：令牌、会话里的用户名与开始处理的时间（审计记耗时）。
type call struct {
	token    apitoken.Token
	username string
	start    time.Time
}

func (s *Server) routes() map[string]route {
	get, post := http.MethodGet, http.MethodPost
	return map[string]route{
		"auth/login":  {method: post, public: true, h: s.login},
		"auth/logout": {method: post, h: s.logout},

		"app/version":         {method: get, h: s.appVersion},
		"app/webapiVersion":   {method: get, h: s.webapiVersion},
		"app/buildInfo":       {method: get, h: s.buildInfo},
		"app/preferences":     {method: get, h: s.preferences},
		"app/defaultSavePath": {method: get, h: s.defaultSavePath},

		"transfer/info": {method: get, h: s.transferInfo},
		"sync/maindata": {method: get, h: s.maindata},

		"torrents/info":       {method: get, h: s.torrentsInfo},
		"torrents/properties": {method: get, h: s.properties},
		"torrents/files":      {method: get, h: s.files},
		"torrents/trackers":   {method: get, h: s.trackers},
		"torrents/categories": {method: get, h: s.categories},
		"torrents/tags":       {method: get, h: s.tags},

		"torrents/add":            {method: post, h: s.add},
		"torrents/pause":          {method: post, h: s.pause},
		"torrents/stop":           {method: post, h: s.pause},
		"torrents/resume":         {method: post, h: s.resume},
		"torrents/start":          {method: post, h: s.resume},
		"torrents/delete":         {method: post, h: s.deleteTorrents},
		"torrents/setCategory":    {method: post, h: s.setCategory},
		"torrents/addTags":        {method: post, h: s.addTags},
		"torrents/removeTags":     {method: post, h: s.removeTags},
		"torrents/createCategory": {method: post, h: s.createCategory},
		"torrents/createTags":     {method: post, h: s.createTags},
	}
}

// Handler 是兼容入口的路由：/api/v2/<组>/<方法>。没登录回 403、方法不对回 405、没有的接口回 404（都是 qB 的行为）。
func (s *Server) Handler() http.Handler {
	routes := s.routes()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := strings.CutPrefix(r.URL.Path, "/api/v2/")
		rt, found := routes[name]
		if !ok || !found {
			text(w, http.StatusNotFound, "Not Found")
			return
		}
		// qB 的 GET 接口也接受 POST（客户端常把查询参数放在表单里）；POST 接口不接受 GET
		if r.Method != rt.method && (rt.method != http.MethodGet || r.Method != http.MethodPost) {
			w.Header().Set("Allow", rt.method)
			text(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		limit := int64(maxFormBody)
		if name == "torrents/add" {
			limit = maxAddBody
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		if rt.public {
			rt.h(w, r, nil)
			return
		}
		start := time.Now()
		c, status := s.authenticate(r)
		if c == nil {
			if status == http.StatusForbidden {
				text(w, status, "Forbidden")
			} else {
				text(w, status, "暂时不能校验令牌")
			}
			return
		}
		c.start = start
		rt.h(w, r, c)
	})
}

// authenticate 按 SID 找会话，再按令牌编号复查令牌：撤销、过期或没有 qbit:compat 时会话作废。
func (s *Server) authenticate(r *http.Request) (*call, int) {
	ck, err := r.Cookie("SID")
	if err != nil {
		return nil, http.StatusForbidden
	}
	now := s.deps.Now()
	sess, ok := s.sessions.get(ck.Value, now)
	if !ok {
		return nil, http.StatusForbidden
	}
	tok, err := s.deps.Tokens.Get(r.Context(), sess.tokenID)
	if errors.Is(err, apitoken.ErrUnauthorized) || (err == nil && !tok.Has(apitoken.ScopeQbitCompat)) {
		s.sessions.drop(ck.Value)
		return nil, http.StatusForbidden
	}
	if err != nil {
		return nil, http.StatusServiceUnavailable
	}
	return &call{token: tok, username: sess.username}, 0
}

// SetAddr 记下监听地址（Status 用）。
func (s *Server) SetAddr(addr string) { s.addr.Store(addr) }

// Addr 是监听地址；没在监听时是空的。
func (s *Server) Addr() string {
	if v, ok := s.addr.Load().(string); ok {
		return v
	}
	return ""
}

// formBool 读布尔参数：qB 不分大小写，qbittorrent-api 发的是 Python 的 True/False。
func formBool(v string) bool { return strings.EqualFold(strings.TrimSpace(v), "true") }

// text 写纯文本回应（qB 的大多数接口回纯文本）。
func text(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// clientIP 取连接的对端地址，不读 X-Forwarded-For（没有反向代理时可以随意伪造）。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
