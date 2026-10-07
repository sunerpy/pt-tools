package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// 订阅接口：订阅设置、质量档案、订阅、豆瓣想看来源、探索。全部只认 session（s.auth），请求体严格解析。

// subscribeSearchTimeout 是「立即搜索」的时限（要在各站点搜索、下载种子文件并推送）。
const subscribeSearchTimeout = 3 * time.Minute

// SetSubscribeService 接上订阅服务（cmd 里建好后调用）。
func (s *Server) SetSubscribeService(svc *subscribe.Service) { s.subscriber = svc }

func (s *Server) registerSubscribeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/media/subscribe/settings", s.auth(s.apiSubscribeSettingsGet))
	mux.HandleFunc("PUT /api/media/subscribe/settings", s.auth(s.apiSubscribeSettingsPut))
	mux.HandleFunc("GET /api/media/quality-profiles", s.auth(s.apiQualityProfiles))
	mux.HandleFunc("POST /api/media/quality-profiles", s.auth(s.apiQualityProfileSave))
	mux.HandleFunc("PUT /api/media/quality-profiles/{id}", s.auth(s.apiQualityProfileSave))
	mux.HandleFunc("DELETE /api/media/quality-profiles/{id}", s.auth(s.apiQualityProfileDelete))
	mux.HandleFunc("GET /api/media/subscriptions", s.auth(s.apiSubscriptions))
	mux.HandleFunc("POST /api/media/subscriptions", s.auth(s.apiSubscriptionCreate))
	mux.HandleFunc("GET /api/media/subscriptions/{id}", s.auth(s.apiSubscriptionGet))
	mux.HandleFunc("PUT /api/media/subscriptions/{id}", s.auth(s.apiSubscriptionUpdate))
	mux.HandleFunc("DELETE /api/media/subscriptions/{id}", s.auth(s.apiSubscriptionDelete))
	mux.HandleFunc("POST /api/media/subscriptions/{id}/status", s.auth(s.apiSubscriptionStatus))
	mux.HandleFunc("POST /api/media/subscriptions/{id}/search", s.auth(s.apiSubscriptionSearch))
	mux.HandleFunc("GET /api/media/douban-sources", s.auth(s.apiDoubanSources))
	mux.HandleFunc("POST /api/media/douban-sources", s.auth(s.apiDoubanSourceSave))
	mux.HandleFunc("PUT /api/media/douban-sources/{id}", s.auth(s.apiDoubanSourceSave))
	mux.HandleFunc("DELETE /api/media/douban-sources/{id}", s.auth(s.apiDoubanSourceDelete))
	mux.HandleFunc("POST /api/media/douban-sources/{id}/fetch", s.auth(s.apiDoubanSourceFetch))
	mux.HandleFunc("GET /api/media/douban-sources/{id}/items", s.auth(s.apiDoubanSourceItems))
	mux.HandleFunc("GET /api/media/explore", s.auth(s.apiMediaExplore))
}

func (s *Server) subscribeService(w http.ResponseWriter) (*subscribe.Service, bool) {
	if s != nil && s.subscriber != nil {
		return s.subscriber, true
	}
	http.Error(w, "订阅服务没有启动", http.StatusServiceUnavailable)
	return nil, false
}

func writeSubscribeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, subscribe.ErrInvalid), errors.Is(err, tmdb.ErrNoKey), errors.Is(err, tmdb.ErrUnauthorized):
		status = http.StatusBadRequest
	case errors.Is(err, subscribe.ErrNotFound), errors.Is(err, tmdb.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, tmdb.ErrRateLimited):
		status = http.StatusTooManyRequests
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	case errors.Is(err, tmdb.ErrUnavailable):
		status = http.StatusBadGateway
	}
	http.Error(w, err.Error(), status)
}

// apiSubscribeSettingsGet handles GET /api/media/subscribe/settings
func (s *Server) apiSubscribeSettingsGet(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	got, err := svc.Settings(r.Context())
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscribeSettingsPut handles PUT /api/media/subscribe/settings
func (s *Server) apiSubscribeSettingsPut(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	var in subscribe.Settings
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SaveSettings(r.Context(), in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiQualityProfiles handles GET /api/media/quality-profiles：档案列表，带上分辨率、来源、编码的可选值。
func (s *Server) apiQualityProfiles(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	list, err := svc.Profiles(r.Context())
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{
		"items": list, "resolutions": subscribe.Resolutions, "sources": subscribe.Sources, "codecs": subscribe.Codecs,
	})
}

// apiQualityProfileSave handles POST /api/media/quality-profiles and PUT /api/media/quality-profiles/{id}
func (s *Server) apiQualityProfileSave(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := optionalID(w, r)
	if !ok {
		return
	}
	var in subscribe.ProfileInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SaveProfile(r.Context(), id, in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiQualityProfileDelete handles DELETE /api/media/quality-profiles/{id}
func (s *Server) apiQualityProfileDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteProfile(r.Context(), id); err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// apiSubscriptions handles GET /api/media/subscriptions?status=&q=
func (s *Server) apiSubscriptions(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	list, err := svc.Subscriptions(r.Context(), subscribe.SubscriptionQuery{Status: q.Get("status"), Keyword: q.Get("q")})
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, list)
}

// apiSubscriptionCreate handles POST /api/media/subscriptions
func (s *Server) apiSubscriptionCreate(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	var in subscribe.SubscriptionInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.CreateSubscription(r.Context(), in, models.MediaSubFromManual)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscriptionGet handles GET /api/media/subscriptions/{id}
func (s *Server) apiSubscriptionGet(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	got, err := svc.Subscription(r.Context(), id)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscriptionUpdate handles PUT /api/media/subscriptions/{id}
func (s *Server) apiSubscriptionUpdate(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in subscribe.SubscriptionInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.UpdateSubscription(r.Context(), id, in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscriptionDelete handles DELETE /api/media/subscriptions/{id}
func (s *Server) apiSubscriptionDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteSubscription(r.Context(), id); err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// apiSubscriptionStatus handles POST /api/media/subscriptions/{id}/status {status: active|paused}
func (s *Server) apiSubscriptionStatus(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SetStatus(r.Context(), id, in.Status)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiSubscriptionSearch handles POST /api/media/subscriptions/{id}/search：立即搜索一次，回结果说明。
func (s *Server) apiSubscriptionSearch(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), subscribeSearchTimeout)
	defer cancel()
	msg, err := svc.SearchNow(ctx, id)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"message": msg})
}

// apiDoubanSources handles GET /api/media/douban-sources
func (s *Server) apiDoubanSources(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	list, err := svc.DoubanSources(r.Context())
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, list)
}

// apiDoubanSourceSave handles POST /api/media/douban-sources and PUT /api/media/douban-sources/{id}
func (s *Server) apiDoubanSourceSave(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := optionalID(w, r)
	if !ok {
		return
	}
	var in subscribe.DoubanSourceInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SaveDoubanSource(r.Context(), id, in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiDoubanSourceDelete handles DELETE /api/media/douban-sources/{id}
func (s *Server) apiDoubanSourceDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteDoubanSource(r.Context(), id); err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// apiDoubanSourceFetch handles POST /api/media/douban-sources/{id}/fetch：立即拉一次，回新建的订阅数。
func (s *Server) apiDoubanSourceFetch(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), subscribeSearchTimeout)
	defer cancel()
	n, err := svc.FetchDouban(ctx, id)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"created": n})
}

// apiDoubanSourceItems handles GET /api/media/douban-sources/{id}/items
func (s *Server) apiDoubanSourceItems(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, err := svc.DoubanItems(r.Context(), id)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, list)
}

// apiMediaExplore handles GET /api/media/explore?kind=movie|tv&list=trending|popular|search&page=&q=
func (s *Server) apiMediaExplore(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	page := 1
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			http.Error(w, "page 要是正整数", http.StatusBadRequest)
			return
		}
		page = n
	}
	got, err := svc.Explore(r.Context(), q.Get("kind"), q.Get("list"), page, q.Get("q"))
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}
