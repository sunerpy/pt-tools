package web

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/sunerpy/pt-tools/internal/cookiecloud"
)

// CookieCloud 导入接口：设置、预览、导入。全部只认 session（s.auth）。
// 密码只写不读；预览只返回站点与 Cookie 名，不返回 Cookie 的值。

// cookieCloudAPITimeout 是取回 CookieCloud 数据并写入站点的时限。
const cookieCloudAPITimeout = time.Minute

func (s *Server) registerCookieCloudRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/cookiecloud/settings", s.auth(s.apiCookieCloudSettings))
	mux.HandleFunc("/api/cookiecloud/preview", s.auth(s.apiCookieCloudPreview))
	mux.HandleFunc("/api/cookiecloud/import", s.auth(s.apiCookieCloudImport))
}

func (s *Server) cookieCloudService(w http.ResponseWriter) (*cookiecloud.Service, bool) {
	if s != nil && s.mgr != nil {
		if worker := s.mgr.GetCookieCloudWorker(); worker != nil && worker.Service() != nil {
			return worker.Service(), true
		}
	}
	http.Error(w, "CookieCloud 服务没有启动", http.StatusServiceUnavailable)
	return nil, false
}

func writeCookieCloudError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, cookiecloud.ErrInvalid), errors.Is(err, cookiecloud.ErrNotConfigured), errors.Is(err, cookiecloud.ErrDecrypt):
		status = http.StatusBadRequest
	case errors.Is(err, cookiecloud.ErrBusy):
		status = http.StatusConflict
	case errors.Is(err, cookiecloud.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	}
	http.Error(w, err.Error(), status)
}

// apiCookieCloudSettings handles GET / PUT /api/cookiecloud/settings
func (s *Server) apiCookieCloudSettings(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.cookieCloudService(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		got, err := svc.Settings(r.Context())
		if err != nil {
			writeCookieCloudError(w, err)
			return
		}
		writeJSON(w, got)
	case http.MethodPut:
		var req cookiecloud.SettingsUpdate
		if !decodeStrictBody(w, r, &req) {
			return
		}
		got, err := svc.SaveSettings(r.Context(), req)
		if err != nil {
			writeCookieCloudError(w, err)
			return
		}
		writeJSON(w, got)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// apiCookieCloudPreview handles POST /api/cookiecloud/preview：取回、在本机解密并匹配，列出能导入的站点（不写任何东西）。
func (s *Server) apiCookieCloudPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	svc, ok := s.cookieCloudService(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), cookieCloudAPITimeout)
	defer cancel()
	got, err := svc.Preview(ctx)
	if err != nil {
		writeCookieCloudError(w, err)
		return
	}
	writeJSON(w, got)
}

type cookieCloudImportRequest struct {
	Sites []string `json:"sites"`
}

// apiCookieCloudImport handles POST /api/cookiecloud/import {sites}：把选中站点的 Cookie 写进 pt-tools（没启用的站点会被启用）。
func (s *Server) apiCookieCloudImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	svc, ok := s.cookieCloudService(w)
	if !ok {
		return
	}
	var req cookieCloudImportRequest
	if !decodeStrictBody(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), cookieCloudAPITimeout)
	defer cancel()
	got, err := svc.Import(ctx, req.Sites)
	if err != nil {
		writeCookieCloudError(w, err)
		return
	}
	writeJSON(w, got)
}
