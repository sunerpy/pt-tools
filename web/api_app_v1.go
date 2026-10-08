package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/version"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// App API v1（路线图 M12）：给手机 App 用的版本化接口，/api/app/v1/*。
//
// 和现有 /api/* 不同，这里认两种主体：有效的 session cookie（网页登录，拥有全部权限范围），或者
// `Authorization: Bearer ptt_…` 的 API 令牌（只有自己的权限范围）。每条路由在路由表里声明要的权限范围：
// 没有主体回 401，权限范围不够回 403。返回的 DTO 只放 App 要的字段，不放 Cookie、API key、passkey、密码、
// RSS 地址、下载链接与通知配置。令牌做的写操作记进操作审计（ChannelType 是 api_token）。

const (
	appPrefix = "/api/app/v1"
	// AppRemoteAPILevel 是 App API 的兼容级别：不兼容的改动才加一，App 遇到不认识的级别时提示升级
	AppRemoteAPILevel = 1
)

// appAuditRecorder 记下一条操作审计（app.AuditService 实现了它）。
type appAuditRecorder interface {
	Record(ctx context.Context, e app.AuditEntry) error
}

// appRoute 是 App API 的一条路由。Path 不含 /api/app/v1 前缀，用 Go 1.22 的路由写法（可以带 {参数}）。
type appRoute struct {
	Method  string
	Path    string
	Scope   string
	Handler http.HandlerFunc
}

// appRoutes 是 App API v1 的路由表。读接口要 app:read，写接口要 app:write。
func (s *Server) appRoutes() []appRoute {
	r := apitoken.ScopeAppRead
	return []appRoute{
		{http.MethodGet, "/meta", r, s.appMeta},
		{http.MethodGet, "/overview", r, s.appOverview},
		{http.MethodGet, "/sites", r, s.appSites},
		{http.MethodGet, "/tasks", r, s.appTasks},
		{http.MethodGet, "/favicon/{site}", r, s.appFavicon},
	}
}

func (s *Server) registerAppV1Routes(mux *http.ServeMux) {
	for _, rt := range s.appRoutes() {
		mux.Handle(rt.Method+" "+appPrefix+rt.Path, s.appHandler(rt))
	}
	// 没有的接口回 JSON 的 404，不落到 SPA 的兜底（那会 302 到登录页）
	mux.HandleFunc(appPrefix+"/", func(w http.ResponseWriter, r *http.Request) {
		appError(w, http.StatusNotFound, "not_found", "没有这个接口")
	})
}

// appError 写 App API 的错误：{"error": 代码, "message": 说明}。
func appError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message})
}

// appJSON 写 App API 的成功回应。
func appJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// appPrincipal 解析请求的主体：有效的 session cookie 优先，其次是 Bearer 令牌。没有主体时返回 nil 与要回的状态码。
func (s *Server) appPrincipal(r *http.Request) (*middleware.Principal, int) {
	if c, err := r.Cookie("session"); err == nil {
		if user, ok := s.sessions.lookup(c.Value); ok {
			return &middleware.Principal{Kind: middleware.KindSession, ID: user}, 0
		}
	}
	plain, ok := middleware.BearerToken(r)
	if !ok || s.tokens == nil {
		return nil, http.StatusUnauthorized
	}
	tok, err := s.tokens.Verify(r.Context(), plain)
	if errors.Is(err, apitoken.ErrUnauthorized) {
		return nil, http.StatusUnauthorized
	}
	if err != nil {
		global.GetSlogger().Warnf("[App API] 校验令牌失败: %v", err)
		return nil, http.StatusServiceUnavailable
	}
	return &middleware.Principal{Kind: middleware.KindAPIToken, ID: strconv.FormatUint(uint64(tok.ID), 10), Name: tok.Name, Scopes: tok.Scopes}, 0
}

// appHandler 包一条路由：解析主体、检查权限范围；非 session 主体的写操作处理完以后记审计。
func (s *Server) appHandler(rt appRoute) http.Handler {
	write := rt.Scope == apitoken.ScopeAppWrite
	command := rt.Method + " " + appPrefix + rt.Path
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, status := s.appPrincipal(r)
		if p == nil {
			if status == http.StatusUnauthorized {
				appError(w, status, "unauthorized", "要登录，或者带上有效的 API 令牌（Authorization: Bearer）")
			} else {
				appError(w, status, "unavailable", "暂时不能校验令牌")
			}
			return
		}
		if !p.Has(rt.Scope) {
			appError(w, http.StatusForbidden, "forbidden", "令牌没有 "+rt.Scope+" 权限")
			return
		}
		r = r.WithContext(middleware.WithPrincipal(r.Context(), p))
		if !write || p.Kind == middleware.KindSession {
			rt.Handler(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		rt.Handler(rec, r)
		s.recordAppWrite(r, p, command, rec.status, time.Since(start))
	})
}

// recordAppWrite 把非 session 主体做的写操作记进操作审计（不改表结构：NotificationConfID 为 0，ChannelType 是主体种类）。
func (s *Server) recordAppWrite(r *http.Request, p *middleware.Principal, command string, status int, took time.Duration) {
	if s.appAudit == nil {
		return
	}
	result := "ok"
	if status >= http.StatusBadRequest {
		result = "error"
	}
	e := app.AuditEntry{
		ChannelType: p.Kind, ChannelUserID: p.ID, Command: command, Result: result, LatencyMs: took.Milliseconds(),
		Args: map[string]any{"path": r.URL.Path, "status": status, "name": p.Name},
	}
	// 请求可能已经被取消：审计照样写
	if err := s.appAudit.Record(context.WithoutCancel(r.Context()), e); err != nil {
		global.GetSlogger().Warnf("[App API] 记审计失败: %v", err)
	}
}

// AppMeta 是 GET /meta 的回应。
type AppMeta struct {
	Name           string        `json:"name"`
	Version        string        `json:"version"`
	RemoteAPILevel int           `json:"remote_api_level"`
	Features       []string      `json:"features"`
	Principal      AppPrincipalV `json:"principal"`
}

// AppPrincipalV 是调用者自己的主体（App 用来判断能不能显示写操作）。
type AppPrincipalV struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name,omitempty"`
	Scopes []string `json:"scopes"`
}

// appFeatures 是这个版本的 App API 提供的功能组。
var appFeatures = []string{"overview", "sites", "torrents", "tasks", "search", "push", "attendance", "brush", "media", "subscriptions"}

func (s *Server) appMeta(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFrom(r.Context())
	scopes := p.Scopes
	if p.Kind == middleware.KindSession {
		scopes = apitoken.Scopes
	}
	appJSON(w, AppMeta{
		Name: "pt-tools", Version: version.GetVersionInfo().Version, RemoteAPILevel: AppRemoteAPILevel, Features: appFeatures,
		Principal: AppPrincipalV{Kind: p.Kind, Name: p.Name, Scopes: append([]string{}, scopes...)},
	})
}
