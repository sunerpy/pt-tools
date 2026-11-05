package web

import (
	"net/http"

	"github.com/sunerpy/pt-tools/web/servarr"
)

// SetServarr 接上 Servarr 兼容入口（媒体中心 M10；cmd 在开始接请求以前调用）。
func (s *Server) SetServarr(h http.Handler) { s.servarr = h }

// registerServarrRoutes 挂上 Servarr 兼容入口：不认会话，凭有 servarr 权限的 API 令牌（Key 在查询参数 apikey 里，访问日志只记路径）。
func (s *Server) registerServarrRoutes(mux *http.ServeMux) {
	mux.HandleFunc(servarr.Prefix, func(w http.ResponseWriter, r *http.Request) {
		if s.servarr == nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Servarr 兼容入口没有启动"}` + "\n"))
			return
		}
		s.servarr.ServeHTTP(w, r)
	})
}
