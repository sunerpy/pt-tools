package web

import (
	"encoding/json"
	"net/http"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

type credentialUpdateRequest struct {
	Cookie  *string `json:"cookie,omitempty"`
	APIKey  *string `json:"api_key,omitempty"`
	Passkey *string `json:"passkey,omitempty"`
}

func (s *Server) updateSiteCredential(w http.ResponseWriter, r *http.Request, sg models.SiteGroup) {
	var req credentialUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Cookie == nil && req.APIKey == nil && req.Passkey == nil {
		http.Error(w, "至少提供一个凭据字段 (cookie, api_key, passkey)", http.StatusBadRequest)
		return
	}

	sc, err := s.store.GetSiteConf(sg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if req.Cookie != nil {
		sc.Cookie = *req.Cookie
	}
	if req.APIKey != nil {
		sc.APIKey = *req.APIKey
	}
	if req.Passkey != nil {
		sc.Passkey = *req.Passkey
	}

	if err := s.store.UpsertSiteWithRSS(sg, sc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[Site] 凭据更新成功: site=%s", string(sg))

	go func() {
		if err := RefreshSiteRegistrations(s.store); err != nil {
			global.GetSlogger().Warnf("[Site] 刷新站点注册失败: %v", err)
		}
		cfg, _ := s.store.Load()
		if cfg != nil {
			s.mgr.Reload(cfg)
		}
	}()

	writeJSON(w, map[string]any{"success": true, "site": string(sg)})
}
