// Package servarr 是 Servarr 兼容入口（媒体中心 M10）：在主端口的 /servarr/radarr/api/v3/* 与 /servarr/sonarr/api/v3/*
// 上实现 Radarr v3 与 Sonarr v3 API 的一个子集，让 Seerr 把 pt-tools 当成 Radarr、Sonarr 用：Seerr 里的请求变成 pt-tools 的
// 电影订阅与追剧，订阅的状态与库里的情况回给 Seerr。
//
// 只实现 Seerr 固定 commit dca1af6a（seerr-team/seerr，2026-11-01）用到的接口与字段，契约见 docs/guide/servarr.md 与测试。
//
// Key 放在查询参数 apikey 或请求头 X-Api-Key 里，是有 servarr 权限的 API 令牌；不认会话。写操作（POST、PUT、DELETE，
// 以及 RefreshMonitoredDownloads 以外的命令）记操作审计（通道 servarr）。删除只删订阅，不删任何文件或种子。
package servarr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/fsutil"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

const (
	// Prefix 是兼容入口在主端口上的路径前缀。
	Prefix = "/servarr/"
	// ChannelType 是审计里的通道。
	ChannelType = "servarr"

	radarrBase = "/servarr/radarr"
	sonarrBase = "/servarr/sonarr"
	apiPath    = "/api/v3"

	// RadarrVersion、SonarrVersion 是报给 Seerr 的版本：Sonarr 的主版本大于 3 时 Seerr 不再请求语言档案。
	RadarrVersion = "5.0.0.0"
	SonarrVersion = "4.0.0.0"

	// defaultProfileID 是「默认档案」（订阅的 profile_id = 0，用订阅设置里的默认档案）
	defaultProfileID = 999999
	// autoRootID、autoRootPath 是「按类型自动选库」（订阅的 library_id = 0）
	autoRootID   = 999999
	autoRootPath = "auto"

	maxBody = 1 << 20

	// 同一个来源 1 分钟内 Key 错 10 次，之后 5 分钟直接回 429
	failWindow = time.Minute
	failMax    = 10
	failBan    = 5 * time.Minute
	maxFailIPs = 10000
)

// listBudget 是列出全部剧集时等 TMDB 的总时间（Seerr 的请求 10 秒超时）：超出的剧这次不列，下次再列（TMDB 的结果有缓存）。
// 是变量只为测试能改短。
var listBudget = 7 * time.Second

// TokenStore 校验 API 令牌（apitoken.Store 实现了它）。
type TokenStore interface {
	Verify(ctx context.Context, plain string) (apitoken.Token, error)
}

// AuditRecorder 记一条操作审计（app.AuditService 实现了它）。
type AuditRecorder interface {
	Record(ctx context.Context, e app.AuditEntry) error
}

// Subscriptions 是兼容入口要的订阅服务（subscribe.Service 实现了它）。
type Subscriptions interface {
	SubscriptionsOf(ctx context.Context, kind string, tmdbID int) ([]models.MediaSubscription, error)
	SubscriptionRow(ctx context.Context, id uint) (models.MediaSubscription, error)
	CreateSubscription(ctx context.Context, in subscribe.SubscriptionInput, source string) (*models.MediaSubscription, error)
	SetMonitored(ctx context.Context, id uint, on, searchNow bool) (*models.MediaSubscription, error)
	SetProfile(ctx context.Context, id, profileID uint) (*models.MediaSubscription, error)
	DeleteSubscription(ctx context.Context, id uint) error
	SearchSoon(ctx context.Context, id uint) error
	Profiles(ctx context.Context) ([]subscribe.Profile, error)
	Episodes(ctx context.Context, id uint) ([]subscribe.EpisodeState, error)
	AiredEpisodes(ctx context.Context, row *models.MediaSubscription) int
	MovieLibrary(ctx context.Context, tmdbID int) (subscribe.LibraryMovie, error)
	LibrarySeasons(ctx context.Context, tmdbID int) (map[int]subscribe.LibrarySeason, error)
}

// Deps 是兼容入口要的服务，cmd 里接上。
type Deps struct {
	// DB 存 Servarr 的标签（servarr_tags、servarr_tag_links）
	DB     *gorm.DB
	Tokens TokenStore
	Audit  AuditRecorder
	Subs   Subscriptions
	// Libraries 是全部媒体库（organize.Service.Libraries）
	Libraries func(ctx context.Context) ([]models.MediaLibrary, error)
	// TMDB 取按当前设置建的 TMDB 客户端（recognize.Service.TMDB）
	TMDB func(ctx context.Context) (*tmdb.Client, error)
	// DiskUsage 读目录所在磁盘的空间；为空时用 fsutil.DiskUsage
	DiskUsage func(path string) (fsutil.Usage, error)
	Now       func() time.Time
}

// Server 是兼容入口。
type Server struct {
	deps    Deps
	fails   *failLock
	cmdID   atomic.Int64
	started time.Time
	radarr  http.Handler
	sonarr  http.Handler
}

// New 建一个兼容入口。
func New(d Deps) *Server {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.DiskUsage == nil {
		d.DiskUsage = fsutil.DiskUsage
	}
	s := &Server{deps: d, fails: &failLock{m: map[string]*failEntry{}}, started: d.Now()}
	s.radarr, s.sonarr = s.routes(arrRadarr), s.routes(arrSonarr)
	return s
}

// arr 是两种兼容的应用。
type arr struct {
	name    string // Radarr、Sonarr
	base    string // /servarr/radarr
	version string
	kind    string // 订阅与媒体库的类型
}

var (
	arrRadarr = arr{name: "Radarr", base: radarrBase, version: RadarrVersion, kind: models.MediaKindMovie}
	arrSonarr = arr{name: "Sonarr", base: sonarrBase, version: SonarrVersion, kind: models.MediaKindTV}
)

// handlerFunc 是一条接口：已经校验过 Key。
type handlerFunc func(w http.ResponseWriter, r *http.Request, c *call)

// routes 是一种应用的路由（Go 1.22 的写法，带方法）。没有的接口回 404，方法不对回 405。
func (s *Server) routes(a arr) http.Handler {
	mux := http.NewServeMux()
	p := a.base + apiPath
	add := func(pattern string, h handlerFunc) {
		method, path, _ := strings.Cut(pattern, " ")
		mux.Handle(method+" "+p+path, s.wrap(a, h))
	}
	add("GET /system/status", s.systemStatus)
	add("GET /qualityProfile", s.qualityProfiles)
	add("GET /qualityprofile", s.qualityProfiles)
	add("GET /rootfolder", s.rootFolders)
	add("GET /rootFolder", s.rootFolders)
	add("GET /tag", s.listTags)
	add("POST /tag", s.createTag)
	add("PUT /tag/{id}", s.renameTag)
	add("POST /command", s.command)
	add("GET /queue", s.queue)
	if a.kind == models.MediaKindMovie {
		add("GET /movie", s.listMovies)
		add("GET /movie/lookup", s.lookupMovie)
		add("GET /movie/{id}", s.getMovie)
		add("POST /movie", s.addMovie)
		add("PUT /movie", s.updateMovie)
		add("PUT /movie/{id}", s.updateMovie)
		add("DELETE /movie/{id}", s.deleteMovie)
	} else {
		add("GET /series", s.listSeries)
		add("GET /series/lookup", s.lookupSeries)
		add("GET /series/{id}", s.getSeries)
		add("POST /series", s.addSeries)
		add("PUT /series", s.updateSeries)
		add("PUT /series/{id}", s.updateSeries)
		add("DELETE /series/{id}", s.deleteSeries)
		add("GET /episode", s.listEpisodes)
		add("PUT /episode/monitor", s.monitorEpisodes)
		add("GET /languageprofile", s.languageProfiles)
		add("GET /languageProfile", s.languageProfiles)
	}
	return mux
}

// Handler 是兼容入口的路由，挂在主端口的 Prefix 下。
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, radarrBase+"/"):
			s.radarr.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, sonarrBase+"/"):
			s.sonarr.ServeHTTP(w, r)
		default:
			writeError(w, http.StatusNotFound, "没有这个接口：Radarr 的地址前缀是 "+radarrBase+"，Sonarr 的是 "+sonarrBase)
		}
	})
}

// call 是一次校验过 Key 的请求：令牌、应用与审计要记的内容。
type call struct {
	token apitoken.Token
	arr   arr
	start time.Time
	// args 是审计里记的参数；skipAudit 为真时这次写请求不记审计（RefreshMonitoredDownloads 每分钟一次）
	args      map[string]any
	skipAudit bool
}

// statusWriter 记下回应的状态码（审计用）。
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// wrap 包一条接口：校验 Key 与权限（失败都回 401）、限制请求体，写请求按状态码记审计。
func (s *Server) wrap(a arr, h handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ip := clientIP(r)
		now := s.deps.Now()
		if s.fails.locked(ip, now) {
			writeError(w, http.StatusTooManyRequests, "API Key 错的次数太多，过几分钟再试")
			return
		}
		plain := apiKey(r)
		if plain == "" || s.deps.Tokens == nil {
			s.fails.fail(ip, now)
			writeError(w, http.StatusUnauthorized, "要带上 API Key（查询参数 apikey 或请求头 X-Api-Key），值是有 servarr 权限的 API 令牌")
			return
		}
		tok, err := s.deps.Tokens.Verify(r.Context(), plain)
		if errors.Is(err, apitoken.ErrUnauthorized) {
			s.fails.fail(ip, now)
			writeError(w, http.StatusUnauthorized, "API Key 不对，或者令牌已经撤销、过期")
			return
		}
		if err != nil {
			global.GetSlogger().Warnf("[Servarr] 校验令牌失败: %v", err)
			writeError(w, http.StatusServiceUnavailable, "暂时不能校验 API Key")
			return
		}
		write := r.Method != http.MethodGet
		c := &call{token: tok, arr: a, start: start, args: map[string]any{}}
		if !tok.Has(apitoken.ScopeServarr) {
			writeError(w, http.StatusUnauthorized, "这个令牌没有 servarr 权限")
			if write {
				s.record(r, c, "denied:scope")
			}
			return
		}
		s.fails.reset(ip)
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		if !write {
			h(w, r, c)
			return
		}
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		h(sw, r, c)
		if c.skipAudit {
			return
		}
		result := "success"
		if sw.status >= http.StatusBadRequest {
			result = "error:http_" + strconv.Itoa(sw.status)
		}
		s.record(r, c, result)
	})
}

// record 记一条审计：用户是令牌编号，命令是方法与路径（不含查询参数，Key 不进审计）。
func (s *Server) record(r *http.Request, c *call, result string) {
	if s.deps.Audit == nil {
		return
	}
	args := map[string]any{"name": c.token.Name}
	maps.Copy(args, c.args)
	e := app.AuditEntry{
		ChannelType: ChannelType, ChannelUserID: strconv.FormatUint(uint64(c.token.ID), 10),
		Command: r.Method + " " + r.URL.Path, Result: result, Args: args, LatencyMs: time.Since(c.start).Milliseconds(),
	}
	if err := s.deps.Audit.Record(context.WithoutCancel(r.Context()), e); err != nil {
		global.GetSlogger().Warnf("[Servarr] 记审计失败: %v", err)
	}
}

// apiKey 取请求里的 Key：查询参数 apikey（Seerr 放在这里），其次请求头 X-Api-Key。
func apiKey(r *http.Request) string {
	if k := strings.TrimSpace(r.URL.Query().Get("apikey")); k != "" {
		return k
	}
	return strings.TrimSpace(r.Header.Get("X-Api-Key"))
}

// clientIP 取连接的对端地址，不读 X-Forwarded-For（没有反向代理时可以随意伪造）。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// failLock 是 Key 错误的限流：同一来源 failWindow 内错 failMax 次，之后 failBan 内直接拒绝（不再查令牌）。
type failLock struct {
	mu sync.Mutex
	m  map[string]*failEntry
}

type failEntry struct {
	fails       int
	windowStart time.Time
	lockedUntil time.Time
}

func (l *failLock) locked(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.m[ip]
	return ok && now.Before(e.lockedUntil)
}

func (l *failLock) fail(ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.m[ip]
	if !ok && len(l.m) >= maxFailIPs {
		for k, x := range l.m {
			if now.Sub(x.windowStart) > failWindow && !now.Before(x.lockedUntil) {
				delete(l.m, k)
			}
		}
		// 还是满的：淘汰最早开始计数的那个，表不会无限长
		for len(l.m) >= maxFailIPs {
			var oldest string
			var at time.Time
			for k, x := range l.m {
				if oldest == "" || x.windowStart.Before(at) {
					oldest, at = k, x.windowStart
				}
			}
			delete(l.m, oldest)
		}
	}
	if !ok || now.Sub(e.windowStart) > failWindow {
		e = &failEntry{windowStart: now}
		l.m[ip] = e
	}
	e.fails++
	if e.fails >= failMax {
		e.lockedUntil = now.Add(failBan)
		e.fails, e.windowStart = 0, now
	}
}

func (l *failLock) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, ip)
}

// writeJSON 写 JSON 回应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError 写错误回应：{"message": "..."}（Radarr、Sonarr 的写法）。
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}

// decode 读 JSON 请求体（不认识的字段忽略：Seerr 会把整个对象发回来）。
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "请求体超过 1 MiB")
		} else {
			writeError(w, http.StatusBadRequest, "请求体不是正确的 JSON")
		}
		return false
	}
	return true
}

// pathID 读路径里的编号（正整数）。
func pathID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "没有这个编号")
		return 0, false
	}
	return id, true
}

// badRequest 是要回 400 的输入错误（原因写给 Seerr 的用户看）。
type badRequest struct{ msg string }

func (e badRequest) Error() string { return e.msg }

func errBadRequest(msg string) error { return badRequest{msg: msg} }

// writeSubError 按错误写回应：不对的输入 400，没有 404，其他 500（原因写日志）。
func writeSubError(w http.ResponseWriter, err error, what string) {
	var bad badRequest
	switch {
	case errors.As(err, &bad):
		writeError(w, http.StatusBadRequest, bad.msg)
	case errors.Is(err, subscribe.ErrInvalid):
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), subscribe.ErrInvalid.Error()+": "))
	case errors.Is(err, subscribe.ErrNotFound), errors.Is(err, tmdb.ErrNotFound):
		writeError(w, http.StatusNotFound, "没有这个"+what)
	case errors.Is(err, tmdb.ErrNoKey):
		writeError(w, http.StatusServiceUnavailable, "pt-tools 还没有设置 TMDB API Key")
	default:
		global.GetSlogger().Warnf("[Servarr] %s失败: %v", what, err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("处理%s失败", what))
	}
}

// timeText 是 Servarr 写时间的格式；零值写成 Radarr 的默认值。
func timeText(t time.Time) string {
	if t.IsZero() {
		return "0001-01-01T00:00:00Z"
	}
	return t.UTC().Format(time.RFC3339)
}
