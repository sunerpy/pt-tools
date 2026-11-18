package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/server"
	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/models"
)

// 媒体服务器的索引、继续观看、Webhook 与待处理问题（媒体中心 M6）。Webhook 的接收地址不认会话，凭每台服务器的令牌；
// 其余只认会话（s.auth）。

// registerMediaServerRoutes 挂上媒体服务器的索引、继续观看、Webhook 与待处理问题的接口。
func (s *Server) registerMediaServerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/media/servers/{id}/sync", s.auth(s.apiMediaServerSync))
	mux.HandleFunc("POST /api/media/servers/users", s.auth(s.apiMediaServerUsers))
	mux.HandleFunc("POST /api/media/servers/{id}/webhook", s.auth(s.apiMediaServerWebhookCreate))
	mux.HandleFunc("DELETE /api/media/servers/{id}/webhook", s.auth(s.apiMediaServerWebhookDelete))
	// 媒体服务器发来的 Webhook：不认会话，令牌在地址里（日志里打码，见 logMiddleware）
	mux.HandleFunc("POST /api/media/servers/{id}/webhook/{token}", s.apiMediaServerWebhook)
	mux.HandleFunc("GET /api/media/resume", s.auth(s.apiMediaResume))
	mux.HandleFunc("GET /api/media/issues", s.auth(s.apiMediaIssues))
	mux.HandleFunc("POST /api/media/issues/{id}/dismiss", s.auth(s.apiMediaIssueDismiss))
}

// webhookPathRe 是 Webhook 的接收地址：日志里把令牌换成 ***。
var webhookPathRe = regexp.MustCompile(`^(/api/media/servers/[^/]+/webhook/)[^/]+$`)

// logPath 是写进日志的路径：Webhook 的令牌打码。
func logPath(p string) string {
	if !strings.Contains(p, "/webhook/") {
		return p
	}
	return webhookPathRe.ReplaceAllString(p, "${1}***")
}

// apiMediaServerSync handles POST /api/media/servers/{id}/sync：马上同步这台服务器的索引，等同步做完（最多 10 分钟）。
func (s *Server) apiMediaServerSync(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := svc.SyncServer(r.Context(), id)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, res)
}

// apiMediaServerUsers handles POST /api/media/servers/users：列出服务器的用户（请求同测试连接）。
func (s *Server) apiMediaServerUsers(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	var in organize.ServerTestInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	users, err := svc.ServerUsers(ctx, in)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, users)
}

// webhookTokenView 是生成的 Webhook 令牌：只在这一次回；Path 是接收地址的路径（前面加上媒体服务器能访问到的 pt-tools 地址）。
type webhookTokenView struct {
	Token string `json:"token"`
	Path  string `json:"path"`
}

// apiMediaServerWebhookCreate handles POST /api/media/servers/{id}/webhook：生成（或重新生成）Webhook 令牌。
func (s *Server) apiMediaServerWebhookCreate(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	token, err := svc.CreateWebhookToken(r.Context(), id)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, webhookTokenView{Token: token, Path: fmt.Sprintf("/api/media/servers/%d/webhook/%s", id, token)})
}

// apiMediaServerWebhookDelete handles DELETE /api/media/servers/{id}/webhook：关掉 Webhook。
func (s *Server) apiMediaServerWebhookDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteWebhookToken(r.Context(), id); err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// apiMediaServerWebhook handles POST /api/media/servers/{id}/webhook/{token}：媒体服务器发来的 Webhook。不认会话；服务器不存在与
// 令牌不对都回 404；每台每分钟最多 60 次（429）；请求体最多 1 MiB（413）；看不懂的请求体回 400；收下了回 204。
func (s *Server) apiMediaServerWebhook(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, server.WebhookMaxBody)
	err = svc.HandleWebhook(r.Context(), uint(id), r.PathValue("token"), r.Header.Get("Content-Type"), r.Body)
	var tooBig *http.MaxBytesError
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, organize.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, organize.ErrRateLimited):
		http.Error(w, err.Error(), http.StatusTooManyRequests)
	case errors.As(err, &tooBig):
		http.Error(w, "请求体超过 1 MiB", http.StatusRequestEntityTooLarge)
	case errors.Is(err, server.ErrBadWebhook):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "处理 Webhook 失败", http.StatusInternalServerError)
	}
}

// apiMediaResume handles GET /api/media/resume：各台服务器的继续观看。
func (s *Server) apiMediaResume(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	groups, err := svc.Resume(r.Context())
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"servers": groups})
}

// apiMediaIssues handles GET /api/media/issues?status=&kind=&media_type=&tmdb_id=&limit=：待处理问题，最新的在前。
func (s *Server) apiMediaIssues(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := serverindex.IssueFilter{Status: q.Get("status"), Kind: q.Get("kind"), MediaType: q.Get("media_type")}
	switch f.Status {
	case "", models.MediaIssueOpen, models.MediaIssueResolved, models.MediaIssueDismissed:
	default:
		http.Error(w, "status 要选 open、resolved 或 dismissed", http.StatusBadRequest)
		return
	}
	f.TMDBID, _ = strconv.Atoi(q.Get("tmdb_id"))
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	list, err := svc.Issues(r.Context(), f)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": list})
}

// apiMediaIssueDismiss handles POST /api/media/issues/{id}/dismiss：忽略一个还开着的问题。
func (s *Server) apiMediaIssueDismiss(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	row, err := svc.DismissIssue(r.Context(), id)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, row)
}
