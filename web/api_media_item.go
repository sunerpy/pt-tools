package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
)

// 条目详情与订阅日历（媒体中心 M3）：一部电影或剧集的资料、库里与做种的情况、订阅、关联的种子与推荐；剧集一季的每一集；
// 一段日子里订阅播出的集与上映的电影。删除一部片（M7）：预览与执行。只认会话。

// mediaItemTimeout 是条目详情与日历的时限：要问 TMDB（有缓存）与下载器。
const mediaItemTimeout = 60 * time.Second

// mediaDeleteTimeout 是删除一部片的时限：一台台下载器现读种子列表、删种子，再删库里的文件。
const mediaDeleteTimeout = 120 * time.Second

func (s *Server) registerMediaItemRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/media/items/{type}/{tmdb_id}", s.auth(s.apiMediaItem))
	mux.HandleFunc("GET /api/media/items/tv/{tmdb_id}/seasons/{n}", s.auth(s.apiMediaItemSeason))
	mux.HandleFunc("POST /api/media/items/delete/preview", s.auth(s.apiMediaItemDeletePreview))
	mux.HandleFunc("POST /api/media/items/delete", s.auth(s.apiMediaItemDelete))
	mux.HandleFunc("GET /api/media/calendar", s.auth(s.apiMediaCalendar))
}

// apiMediaItemDeletePreview handles POST /api/media/items/delete/preview {media_type, tmdb_id, season, episodes}：
// 删除的预览（库里的文件、关联的种子与能不能删、订阅、问题，见 subscribe.DeletePreview）。
func (s *Server) apiMediaItemDeletePreview(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	var in subscribe.DeleteSelector
	if !decodeStrictBody(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), mediaItemTimeout)
	defer cancel()
	got, err := svc.PreviewDelete(ctx, in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiMediaItemDelete handles POST /api/media/items/delete：按勾选删除（见 subscribe.DeleteRequest），每一步单独回结果。
func (s *Server) apiMediaItemDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	var in subscribe.DeleteRequest
	if !decodeStrictBody(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), mediaDeleteTimeout)
	defer cancel()
	got, err := svc.DeleteItem(ctx, in)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// pathInt 读路径里的非负整数，不对时回 400。
func pathInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	n, err := strconv.Atoi(r.PathValue(name))
	if err != nil || n < 0 {
		http.Error(w, name+" 无效", http.StatusBadRequest)
		return 0, false
	}
	return n, true
}

// apiMediaItem handles GET /api/media/items/{type}/{tmdb_id}：条目详情（见 subscribe.ItemDetail）。
func (s *Server) apiMediaItem(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathInt(w, r, "tmdb_id")
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), mediaItemTimeout)
	defer cancel()
	got, err := svc.Item(ctx, r.PathValue("type"), id)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiMediaItemSeason handles GET /api/media/items/tv/{tmdb_id}/seasons/{n}：剧集一季的每一集（见 subscribe.SeasonDetail）。
func (s *Server) apiMediaItemSeason(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	id, ok := pathInt(w, r, "tmdb_id")
	if !ok {
		return
	}
	n, ok := pathInt(w, r, "n")
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), mediaItemTimeout)
	defer cancel()
	got, err := svc.SeasonItem(ctx, id, n)
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiMediaCalendar handles GET /api/media/calendar?from=&to=：订阅日历（日期 YYYY-MM-DD，含两头，最多 62 天；
// 不写时从今天起 7 天）。
func (s *Server) apiMediaCalendar(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.subscribeService(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), mediaItemTimeout)
	defer cancel()
	q := r.URL.Query()
	got, err := svc.Calendar(ctx, q.Get("from"), q.Get("to"))
	if err != nil {
		writeSubscribeError(w, err)
		return
	}
	writeJSON(w, got)
}
