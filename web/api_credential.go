package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

type credentialUpdateRequest struct {
	Cookie  *string `json:"cookie,omitempty"`
	APIKey  *string `json:"api_key,omitempty"`
	Passkey *string `json:"passkey,omitempty"`
	// LastVisitAt 是浏览器扩展同步凭证时附带的最近访问时间（RFC3339），可选。
	LastVisitAt *string `json:"last_visit_at,omitempty"`
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
	if credentialProvided(req) {
		enabled := true
		sc.Enabled = &enabled
	}

	if err := s.store.UpsertSiteWithRSS(sg, sc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[Site] 凭据更新成功: site=%s", string(sg))
	if req.LastVisitAt != nil {
		recordExtensionVisit(string(sg), *req.LastVisitAt)
	}

	go func() {
		if err := RefreshSiteRegistrations(s.store); err != nil {
			global.GetSlogger().Warnf("[Site] 刷新站点注册失败: %v", err)
		} else {
			s.requestLoginProbe(string(sg))
		}
		cfg, _ := s.store.Load()
		if cfg != nil {
			s.mgr.Reload(cfg)
		}
	}()

	writeJSON(w, map[string]any{"success": true, "site": string(sg)})
}

func credentialProvided(req credentialUpdateRequest) bool {
	return req.Cookie != nil && strings.TrimSpace(*req.Cookie) != "" ||
		req.APIKey != nil && strings.TrimSpace(*req.APIKey) != "" ||
		req.Passkey != nil && strings.TrimSpace(*req.Passkey) != ""
}

// requestLoginProbe 在凭证或站点配置写库、站点注册刷新成功之后请求一次登录探测（探测循环一分钟内执行）。
// 顺序不能反：探测取锁后会重读配置，请求早于探测开始时，那次探测必然用上新凭证和新注册的实例。
func (s *Server) requestLoginProbe(siteName string) {
	if s == nil || s.mgr == nil {
		return
	}
	mon := s.mgr.GetLoginReminderMonitor()
	if mon == nil {
		return
	}
	if err := mon.RequestProbe(siteName); err != nil {
		global.GetSlogger().Warnf("[Site] 请求登录探测失败: site=%s err=%v", siteName, err)
	}
}

// recordExtensionVisit 记录扩展随凭证同步上报的访问时间。时间无法解析或写入失败只记告警，
// 不影响凭证更新本身，以免旧版扩展的同步请求因此失败。
func recordExtensionVisit(siteName, raw string) {
	if strings.TrimSpace(raw) == "" || global.GlobalDB == nil {
		return
	}
	ts, err := parseVisitTimestamp(raw)
	if err != nil {
		global.GetSlogger().Warnf("[Site] 忽略无法解析的 last_visit_at: site=%s err=%v", siteName, err)
		return
	}
	db := global.GlobalDB.DB
	if err := ensureLoginStateRow(db, siteName); err != nil {
		global.GetSlogger().Warnf("[Site] 初始化登录状态失败，忽略 last_visit_at: site=%s err=%v", siteName, err)
		return
	}
	if err := models.NewSiteLoginStateRepository(db).ClampLastVisit(siteName, ts, time.Now()); err != nil {
		global.GetSlogger().Warnf("[Site] 记录 last_visit_at 失败: site=%s err=%v", siteName, err)
	}
}
