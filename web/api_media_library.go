package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/server"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
)

// 整理入库接口：整理设置、媒体库、路径映射、媒体服务器、整理预览与执行、整理历史。全部只认 session（s.auth）。
// 媒体服务器的 Token 只写不读。

// organizeAPITimeout 是一次整理预览或手动整理的时限（要识别、可能要等队列里前面的种子）。
const organizeAPITimeout = 60 * time.Second

// SetOrganizeService 接上整理入库服务（cmd 里建好后调用）。
func (s *Server) SetOrganizeService(svc *organize.Service) { s.organizer = svc }

func (s *Server) registerOrganizeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/media/organize/settings", s.auth(s.apiOrganizeSettingsGet))
	mux.HandleFunc("PUT /api/media/organize/settings", s.auth(s.apiOrganizeSettingsPut))
	mux.HandleFunc("POST /api/media/organize/preview", s.auth(s.apiOrganizePreview))
	mux.HandleFunc("POST /api/media/organize", s.auth(s.apiOrganizeRun))
	mux.HandleFunc("GET /api/media/libraries", s.auth(s.apiLibraries))
	mux.HandleFunc("POST /api/media/libraries", s.auth(s.apiLibrarySave))
	mux.HandleFunc("PUT /api/media/libraries/{id}", s.auth(s.apiLibrarySave))
	mux.HandleFunc("DELETE /api/media/libraries/{id}", s.auth(s.apiLibraryDelete))
	mux.HandleFunc("POST /api/media/libraries/check", s.auth(s.apiLibraryCheck))
	mux.HandleFunc("POST /api/media/libraries/template-preview", s.auth(s.apiTemplatePreview))
	mux.HandleFunc("GET /api/media/path-maps", s.auth(s.apiPathMaps))
	mux.HandleFunc("POST /api/media/path-maps", s.auth(s.apiPathMapSave))
	mux.HandleFunc("PUT /api/media/path-maps/{id}", s.auth(s.apiPathMapSave))
	mux.HandleFunc("DELETE /api/media/path-maps/{id}", s.auth(s.apiPathMapDelete))
	mux.HandleFunc("GET /api/media/servers", s.auth(s.apiMediaServers))
	mux.HandleFunc("POST /api/media/servers", s.auth(s.apiMediaServerSave))
	mux.HandleFunc("PUT /api/media/servers/{id}", s.auth(s.apiMediaServerSave))
	mux.HandleFunc("DELETE /api/media/servers/{id}", s.auth(s.apiMediaServerDelete))
	mux.HandleFunc("POST /api/media/servers/test", s.auth(s.apiMediaServerTest))
	mux.HandleFunc("GET /api/media/history", s.auth(s.apiMediaHistory))
	mux.HandleFunc("POST /api/media/history/{id}/retry", s.auth(s.apiMediaHistoryRetry))
	mux.HandleFunc("DELETE /api/media/history/{id}", s.auth(s.apiMediaHistoryDelete))
}

func (s *Server) organizeService(w http.ResponseWriter) (*organize.Service, bool) {
	if s != nil && s.organizer != nil {
		return s.organizer, true
	}
	http.Error(w, "整理入库服务没有启动", http.StatusServiceUnavailable)
	return nil, false
}

func writeOrganizeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, organize.ErrInvalid), errors.Is(err, server.ErrBadConfig), errors.Is(err, server.ErrUnauthorized),
		errors.Is(err, tmdb.ErrNoKey), errors.Is(err, tmdb.ErrUnauthorized):
		status = http.StatusBadRequest
	case errors.Is(err, organize.ErrNotFound), errors.Is(err, tmdb.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, tmdb.ErrRateLimited):
		status = http.StatusTooManyRequests
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	case errors.Is(err, server.ErrUnavailable), errors.Is(err, server.ErrRedirect), errors.Is(err, tmdb.ErrUnavailable):
		status = http.StatusBadGateway
	}
	http.Error(w, err.Error(), status)
}

// optionalID 是 PUT 路由里的编号；POST（新建）时为 0。
func optionalID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	if r.PathValue("id") == "" {
		return 0, true
	}
	return pathID(w, r)
}

// apiOrganizeSettingsGet handles GET /api/media/organize/settings
func (s *Server) apiOrganizeSettingsGet(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	got, err := svc.Settings(r.Context())
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiOrganizeSettingsPut handles PUT /api/media/organize/settings
func (s *Server) apiOrganizeSettingsPut(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	var in organize.SettingsInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SaveSettings(r.Context(), in)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiOrganizePreview handles POST /api/media/organize/preview
func (s *Server) apiOrganizePreview(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	var in organize.Request
	if !decodeStrictBody(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), organizeAPITimeout)
	defer cancel()
	got, err := svc.Preview(ctx, in)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiOrganizeRun handles POST /api/media/organize
func (s *Server) apiOrganizeRun(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	var in organize.Request
	if !decodeStrictBody(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), organizeAPITimeout)
	defer cancel()
	got, err := svc.Organize(ctx, in)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiLibraries handles GET /api/media/libraries
func (s *Server) apiLibraries(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	list, err := svc.Libraries(r.Context())
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, list)
}

// apiLibrarySave handles POST /api/media/libraries and PUT /api/media/libraries/{id}
func (s *Server) apiLibrarySave(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, ok := optionalID(w, r)
	if !ok {
		return
	}
	var in organize.LibraryInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SaveLibrary(r.Context(), id, in)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiLibraryDelete handles DELETE /api/media/libraries/{id}
func (s *Server) apiLibraryDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteLibrary(r.Context(), id); err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// apiLibraryCheck handles POST /api/media/libraries/check
func (s *Server) apiLibraryCheck(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	var in organize.LibraryCheckInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.CheckLibrary(r.Context(), in)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, got)
}

type templatePreviewInput struct {
	Kind     string `json:"kind"`
	Template string `json:"template"`
}

// apiTemplatePreview handles POST /api/media/libraries/template-preview：用示例渲染模板，空模板用默认的。
func (s *Server) apiTemplatePreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.organizeService(w); !ok {
		return
	}
	var in templatePreviewInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	tpl := in.Template
	if tpl == "" {
		tpl = organize.DefaultTemplate(in.Kind)
	}
	if len(tpl) > 4096 {
		http.Error(w, "模板太长", http.StatusBadRequest)
		return
	}
	out, err := transfer.Check(tpl, in.Kind)
	resp := map[string]any{"template": tpl, "preview": out}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, resp)
}

// apiPathMaps handles GET /api/media/path-maps
func (s *Server) apiPathMaps(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	list, err := svc.PathMaps(r.Context())
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, list)
}

// apiPathMapSave handles POST /api/media/path-maps and PUT /api/media/path-maps/{id}
func (s *Server) apiPathMapSave(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, ok := optionalID(w, r)
	if !ok {
		return
	}
	var in organize.PathMapInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SavePathMap(r.Context(), id, in)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiPathMapDelete handles DELETE /api/media/path-maps/{id}
func (s *Server) apiPathMapDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeletePathMap(r.Context(), id); err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// apiMediaServers handles GET /api/media/servers
func (s *Server) apiMediaServers(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	list, err := svc.Servers(r.Context())
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, list)
}

// apiMediaServerSave handles POST /api/media/servers and PUT /api/media/servers/{id}
func (s *Server) apiMediaServerSave(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, ok := optionalID(w, r)
	if !ok {
		return
	}
	var in organize.ServerInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SaveServer(r.Context(), id, in)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiMediaServerDelete handles DELETE /api/media/servers/{id}
func (s *Server) apiMediaServerDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteServer(r.Context(), id); err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// apiMediaServerTest handles POST /api/media/servers/test
func (s *Server) apiMediaServerTest(w http.ResponseWriter, r *http.Request) {
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
	got, err := svc.TestServer(ctx, in)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiMediaHistory handles GET /api/media/history?status=&q=&limit=&offset=
func (s *Server) apiMediaHistory(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	got, err := svc.History(r.Context(), organize.HistoryQuery{Status: q.Get("status"), Keyword: q.Get("q"), Limit: limit, Offset: offset})
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiMediaHistoryRetry handles POST /api/media/history/{id}/retry
func (s *Server) apiMediaHistoryRetry(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), organizeAPITimeout)
	defer cancel()
	got, err := svc.Retry(ctx, id)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiMediaHistoryDelete handles DELETE /api/media/history/{id}?files=1
func (s *Server) apiMediaHistoryDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteHistory(r.Context(), id, r.URL.Query().Get("files") == "1"); err != nil {
		writeOrganizeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
