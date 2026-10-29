package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

type credentialUpdateRequest struct {
	Cookie  *string `json:"cookie,omitempty"`
	APIKey  *string `json:"api_key,omitempty"`
	Passkey *string `json:"passkey,omitempty"`
	// LastVisitAt 是浏览器扩展同步凭证时附带的最近访问时间（RFC3339），可选。
	LastVisitAt *string `json:"last_visit_at,omitempty"`
	// KeepEnabled 为 true 时不改站点的启用状态：停用的站点只把凭据存下、保持停用，不刷新站点、不探测、不重载调度。
	// 给浏览器扩展的定时同步用 —— 它会推浏览器里登录着、但 pt-tools 里还没启用的站点，不能替用户把站点启用。
	// 不带这个字段时照旧：推了凭据就启用站点（扩展里手动点的同步）。
	KeepEnabled *bool `json:"keep_enabled,omitempty"`
	// VerifyCookie 为 true 时先用这次的 Cookie 实际访问站点一次（与登录探测同一套），登录上了才保存；
	// 没登录上不保存、原来的凭据不动，回 422 和原因（见 verifyCookieOrReject）。给浏览器扩展同步它认不出格式的 Cookie 用。
	VerifyCookie *bool `json:"verify_cookie,omitempty"`
}

// FeatureCredentialKeepEnabled 写在 /api/ping 的 features 里：扩展看到它才会让定时同步推没启用的站点。
// 旧版 pt-tools 不认 keep_enabled，照样会把站点启用，所以扩展要先确认。
const FeatureCredentialKeepEnabled = "site_credential_keep_enabled"

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
	// 要验证的 Cookie 先拿它登录一次，登录上了才往下走到任何一条保存的路
	if req.VerifyCookie != nil && *req.VerifyCookie {
		if req.Cookie == nil || strings.TrimSpace(*req.Cookie) == "" {
			http.Error(w, "verify_cookie 要和 cookie 一起提供", http.StatusBadRequest)
			return
		}
		cookie, ok := s.verifyCookieOrReject(r.Context(), w, string(sg), sc)
		if !ok {
			return
		}
		// 验证那次访问要是续了期（BYR 换发令牌、refresh_token 也换），原来那组已经用掉了：存换发后的
		sc.Cookie = cookie
		req.Cookie = &cookie
	}
	enabled := sc.Enabled != nil && *sc.Enabled
	// 调用方要求不改启用状态、站点又是停用的：只存凭据，站点照旧停用。
	if req.KeepEnabled != nil && *req.KeepEnabled && !enabled {
		if err := s.store.SaveSiteCredentials(sg, req.Cookie, req.APIKey, req.Passkey); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		global.GetSlogger().Infof("[Site] 凭据已保存，按请求保持停用: site=%s", string(sg))
		if req.LastVisitAt != nil {
			recordExtensionVisit(string(sg), *req.LastVisitAt)
		}
		writeJSON(w, map[string]any{"success": true, "site": string(sg), "enabled": false})
		return
	}
	// 凭据还没配齐的停用站点（HDDolby 要 Cookie 和 API Key，扩展只同步来了 Cookie）：先把这次的凭据存下，
	// 站点保持停用，告诉调用方还缺什么；不再走完整校验回 500，免得扩展每次同步都报失败、Cookie 也存不进去。
	if missing := core.MissingCredential(sc.AuthMethod, sc.Cookie, sc.APIKey, sc.Passkey); missing != "" && !enabled && credentialProvided(req) {
		if err := s.store.SaveSiteCredentials(sg, req.Cookie, req.APIKey, req.Passkey); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		global.GetSlogger().Infof("[Site] 凭据已保存，站点还缺 %s，保持停用: site=%s", missing, string(sg))
		if req.LastVisitAt != nil {
			recordExtensionVisit(string(sg), *req.LastVisitAt)
		}
		writeJSON(w, map[string]any{
			"success": true, "site": string(sg), "enabled": false,
			"warning": incompleteCredentialWarning(string(sg), missing),
		})
		return
	}
	if credentialProvided(req) {
		enabled := true
		sc.Enabled = &enabled
	}

	// 校验失败（缺凭据、认证方式不对）是请求的问题，和 POST 保存站点一样回 400
	if err := s.store.UpsertSiteWithRSS(sg, sc); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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

// incompleteCredentialWarning 说明凭据存下了、站点还缺哪一项才能启用。
func incompleteCredentialWarning(siteName, missing string) string {
	name := siteLabel(siteName)
	label := map[string]string{"cookie": "Cookie", "api_key": "API Key", "passkey": "Passkey"}[missing]
	return fmt.Sprintf("已保存，但 %s 还需要 %s 才能启用：在 pt-tools 的「站点列表」里编辑 %s，填上 %s", name, label, name, label)
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
