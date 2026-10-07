package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// 媒体识别接口：TMDB 设置、识别预览、手动纠正、识别词。全部只认 session（s.auth）。
// API Key 与代理地址只写不读（代理地址里的密码换成 ***）。

// mediaAPITimeout 是一次识别（可能要搜好几次 TMDB）的时限。
const mediaAPITimeout = 45 * time.Second

// SetMediaService 接上媒体识别服务（cmd 里建好后调用）。
func (s *Server) SetMediaService(svc *recognize.Service) { s.media = svc }

func (s *Server) registerMediaRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/media/settings", s.auth(s.apiMediaSettingsGet))
	mux.HandleFunc("PUT /api/media/settings", s.auth(s.apiMediaSettingsPut))
	mux.HandleFunc("POST /api/media/settings/test", s.auth(s.apiMediaSettingsTest))
	mux.HandleFunc("POST /api/media/recognize", s.auth(s.apiMediaRecognize))
	mux.HandleFunc("GET /api/media/overrides", s.auth(s.apiMediaOverrides))
	mux.HandleFunc("POST /api/media/overrides", s.auth(s.apiMediaOverrideSave))
	mux.HandleFunc("DELETE /api/media/overrides/{id}", s.auth(s.apiMediaOverrideDelete))
	mux.HandleFunc("GET /api/media/words", s.auth(s.apiMediaWords))
	mux.HandleFunc("POST /api/media/words", s.auth(s.apiMediaWordSave))
	mux.HandleFunc("PUT /api/media/words/{id}", s.auth(s.apiMediaWordSave))
	mux.HandleFunc("DELETE /api/media/words/{id}", s.auth(s.apiMediaWordDelete))
}

func (s *Server) mediaService(w http.ResponseWriter) (*recognize.Service, bool) {
	if s != nil && s.media != nil {
		return s.media, true
	}
	http.Error(w, "媒体识别服务没有启动", http.StatusServiceUnavailable)
	return nil, false
}

func writeMediaError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, recognize.ErrInvalid), errors.Is(err, tmdb.ErrNoKey), errors.Is(err, tmdb.ErrUnauthorized):
		status = http.StatusBadRequest
	case errors.Is(err, recognize.ErrNotFound), errors.Is(err, tmdb.ErrNotFound):
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

func pathID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, "编号无效", http.StatusBadRequest)
		return 0, false
	}
	return uint(id), true
}

// apiMediaSettingsGet handles GET /api/media/settings
func (s *Server) apiMediaSettingsGet(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.mediaService(w)
	if !ok {
		return
	}
	got, err := svc.Settings(r.Context())
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiMediaSettingsPut handles PUT /api/media/settings
func (s *Server) apiMediaSettingsPut(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.mediaService(w)
	if !ok {
		return
	}
	var in recognize.SettingsInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SaveSettings(r.Context(), in)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiMediaSettingsTest handles POST /api/media/settings/test
func (s *Server) apiMediaSettingsTest(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.mediaService(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), mediaAPITimeout)
	defer cancel()
	if err := svc.TestTMDB(ctx); err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// apiMediaRecognize handles POST /api/media/recognize
func (s *Server) apiMediaRecognize(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.mediaService(w)
	if !ok {
		return
	}
	var in recognize.Input
	if !decodeStrictBody(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), mediaAPITimeout)
	defer cancel()
	got, err := svc.Recognize(ctx, in)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiMediaOverrides handles GET /api/media/overrides
func (s *Server) apiMediaOverrides(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.mediaService(w)
	if !ok {
		return
	}
	list, err := svc.Overrides(r.Context())
	if err != nil {
		writeMediaError(w, err)
		return
	}
	if list == nil {
		list = []models.MediaOverride{}
	}
	writeJSON(w, list)
}

// apiMediaOverrideSave handles POST /api/media/overrides
func (s *Server) apiMediaOverrideSave(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.mediaService(w)
	if !ok {
		return
	}
	var in recognize.OverrideInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), mediaAPITimeout)
	defer cancel()
	got, err := svc.SetOverride(ctx, in)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiMediaOverrideDelete handles DELETE /api/media/overrides/{id}
func (s *Server) apiMediaOverrideDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.mediaService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteOverride(r.Context(), id); err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// apiMediaWords handles GET /api/media/words
func (s *Server) apiMediaWords(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.mediaService(w)
	if !ok {
		return
	}
	list, err := svc.Words(r.Context())
	if err != nil {
		writeMediaError(w, err)
		return
	}
	if list == nil {
		list = []models.MediaWordRule{}
	}
	writeJSON(w, list)
}

type mediaWordBody struct {
	Kind        string `json:"kind"`
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
	Offset      int    `json:"offset"`
	IsRegex     bool   `json:"is_regex"`
	Enabled     bool   `json:"enabled"`
	Note        string `json:"note"`
}

// apiMediaWordSave handles POST /api/media/words and PUT /api/media/words/{id}
func (s *Server) apiMediaWordSave(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.mediaService(w)
	if !ok {
		return
	}
	var id uint
	if r.Method == http.MethodPut {
		if id, ok = pathID(w, r); !ok {
			return
		}
	}
	var in mediaWordBody
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SaveWord(r.Context(), models.MediaWordRule{
		ID: id, Kind: in.Kind, Pattern: in.Pattern, Replacement: in.Replacement, Offset: in.Offset,
		IsRegex: in.IsRegex, Enabled: in.Enabled, Note: in.Note,
	})
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiMediaWordDelete handles DELETE /api/media/words/{id}
func (s *Server) apiMediaWordDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.mediaService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteWord(r.Context(), id); err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
