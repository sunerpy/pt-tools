package web

import (
	"net/http"
	"sort"
	"strings"

	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// App API 的配置接口（第五批）：经配置网关（config_gateway.go）读改 pt-tools 的配置，MCP 的 get_config、change_config
// 与内置 AI 助手也走这里。都要 app:config，另外读与预览要 app:read，保存要 app:write。
// 回应里的键与网页接口的一样（会有 password、cookie、url 这些名字），密钥的值换成 <已设置> 或者空串。

// appNeedConfig 包一个配置接口：没有 app:config 时回 403（审计记 denied:scope）。
func (s *Server) appNeedConfig(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !middleware.PrincipalFrom(r.Context()).Has(apitoken.ScopeAppConfig) {
			appSetOutcome(r, "denied:scope")
			appError(w, http.StatusForbidden, "forbidden", "令牌没有 "+apitoken.ScopeAppConfig+" 权限")
			return
		}
		h(w, r)
	}
}

// appConfigCatalog 是 GET /config：能读改的配置有哪些、各自能改哪些字段。
func (s *Server) appConfigCatalog(w http.ResponseWriter, _ *http.Request) {
	appJSON(w, s.ConfigCatalog())
}

// appConfigGet 是 GET /config/{resource}[?id=]：一类配置的现状（列表类给了 id 时只回那一项）。
func (s *Server) appConfigGet(w http.ResponseWriter, r *http.Request) {
	got, err := s.ConfigRead(r.Context(), r.PathValue("resource"), strings.TrimSpace(r.URL.Query().Get("id")))
	if err != nil {
		configWriteError(w, err)
		return
	}
	appJSON(w, got)
}

// appConfigPreview 是 POST /config/{resource}/preview：算出会改什么、可能的影响，不保存。
func (s *Server) appConfigPreview(w http.ResponseWriter, r *http.Request) {
	var in ConfigChange
	if !appDecode(w, r, &in) {
		return
	}
	got, err := s.ConfigPreview(r.Context(), r.PathValue("resource"), in)
	if err != nil {
		configWriteError(w, err)
		return
	}
	appJSON(w, got)
}

// appConfigApply 是 POST /config/{resource}/changes：按现状重新算一遍改动并保存，回改完的样子。
// 审计记资源、操作、编号与改了哪些字段（不记值）。
func (s *Server) appConfigApply(w http.ResponseWriter, r *http.Request) {
	var in ConfigChange
	if !appDecode(w, r, &in) {
		return
	}
	fields := make([]string, 0, len(in.Values))
	for k := range in.Values {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	appSetAuditArgs(r, map[string]any{"resource": r.PathValue("resource"), "op": in.Op, "id": in.ID, "fields": fields})
	got, err := s.ConfigApply(r.Context(), r.PathValue("resource"), in)
	if err != nil {
		configWriteError(w, err)
		return
	}
	appJSON(w, got)
}
