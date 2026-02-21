package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

type Server struct {
	store    *core.ConfigStore
	mgr      *scheduler.Manager
	tpl      *template.Template
	sessions map[string]string // sessionID -> username
}

func NewServer(store *core.ConfigStore, mgr *scheduler.Manager) *Server {
	t := template.Must(template.New("login").Parse(loginHTML))
	return &Server{store: store, mgr: mgr, tpl: t, sessions: map[string]string{}}
}

func (s *Server) ensureAdminFromEnv() {
	user := strings.TrimSpace(os.Getenv("PT_ADMIN_USER"))
	pass := strings.TrimSpace(os.Getenv("PT_ADMIN_PASS"))
	if user == "" {
		user = "admin"
	}
	if pass == "" {
		pass = "adminadmin"
	}
	hash := hashPassword(pass)
	_ = s.store.EnsureAdmin(user, hash)
	// 启动时重置密码（一次性）
	if strings.TrimSpace(os.Getenv("PT_ADMIN_RESET")) == "1" {
		if err := s.store.UpdateAdminPassword(user, hash); err == nil {
			global.GetSlogger().Infow("admin_reset", "user", user)
		} else {
			global.GetSlogger().Warnw("admin_reset_failed", "user", user, "err", err)
		}
	}
}

func (s *Server) Serve(addr string) error {
	mux := http.NewServeMux()
	s.ensureAdminFromEnv()
	mux.HandleFunc("/login", s.loginHandler)
	mux.HandleFunc("/logout", s.logoutHandler)
	mux.HandleFunc("/api/ping", s.apiPing)
	// JSON APIs
	mux.HandleFunc("/api/global", s.auth(s.apiGlobal))
	mux.HandleFunc("/api/qbit", s.auth(s.apiQbit))
	mux.HandleFunc("/api/sites", s.auth(s.apiSites))
	mux.HandleFunc("/api/sites/", s.auth(s.apiSiteDetail))
	mux.HandleFunc("/api/password", s.auth(s.apiPassword))
	mux.HandleFunc("/api/tasks", s.auth(s.apiTasks))
	mux.HandleFunc("/api/logs", s.auth(s.apiLogs))
	mux.HandleFunc("/api/control/stop", s.auth(s.apiStopAll))
	mux.HandleFunc("/api/control/start", s.auth(s.apiStartAll))
	// Downloader management APIs
	mux.HandleFunc("/api/downloaders", s.auth(s.apiDownloaders))
	mux.HandleFunc("/api/downloaders/all-directories", s.auth(s.apiAllDownloaderDirectories))
	mux.HandleFunc("/api/downloaders/", s.auth(s.apiDownloaderRouter))
	// Site management APIs
	mux.HandleFunc("/api/sites/downloader-summary", s.auth(s.apiSiteDownloaderSummary))
	mux.HandleFunc("/api/sites/validate", s.auth(s.apiSiteValidate))
	mux.HandleFunc("/api/sites/dynamic", s.auth(s.apiDynamicSites))
	mux.HandleFunc("/api/sites/templates", s.auth(s.apiSiteTemplates))
	mux.HandleFunc("/api/sites/templates/import", s.auth(s.apiSiteTemplateImport))
	mux.HandleFunc("/api/sites/templates/", s.auth(s.apiSiteTemplateExport))
	// Filter rules API
	mux.HandleFunc("/api/filter-rules", s.auth(s.apiFilterRules))
	mux.HandleFunc("/api/filter-rules/", s.auth(s.apiFilterRuleDetail))
	// RSS-Filter association API
	mux.HandleFunc("/api/rss/", s.auth(s.apiRSSFilterAssociation))
	// Log level API
	mux.HandleFunc("/api/log-level", s.auth(s.apiLogLevel))
	// User info v2 APIs
	mux.HandleFunc("/api/v2/userinfo/aggregated", s.auth(s.apiUserInfoAggregated))
	mux.HandleFunc("/api/v2/userinfo/sites", s.auth(s.apiUserInfoSites))
	mux.HandleFunc("/api/v2/userinfo/sites/", s.auth(s.apiUserInfoSiteDetail))
	mux.HandleFunc("/api/v2/userinfo/sync", s.auth(s.apiUserInfoSync))
	mux.HandleFunc("/api/v2/userinfo/registered", s.auth(s.apiUserInfoRegisteredSites))
	mux.HandleFunc("/api/v2/userinfo/cache/clear", s.auth(s.apiUserInfoClearCache))
	// Site levels API
	mux.HandleFunc("/api/v2/sites/", s.auth(s.apiSiteLevelsRouter))
	// Site favicon API (with caching)
	mux.HandleFunc("/api/favicons", s.auth(s.apiFaviconList))
	mux.HandleFunc("/api/favicon/", s.auth(s.apiFavicon))
	// Search v2 APIs
	mux.HandleFunc("/api/v2/search/multi", s.auth(s.apiMultiSiteSearch))
	mux.HandleFunc("/api/v2/search/sites", s.auth(s.apiSearchSites))
	mux.HandleFunc("/api/v2/search/cache/clear", s.auth(s.apiSearchCacheClear))
	mux.HandleFunc("/api/v2/search/cache/stats", s.auth(s.apiSearchCacheStats))
	// Torrent push v2 APIs
	mux.HandleFunc("/api/v2/torrents/push", s.auth(s.apiTorrentPush))
	mux.HandleFunc("/api/v2/torrents/batch-push", s.auth(s.apiTorrentBatchPush))
	mux.HandleFunc("/api/v2/torrents/batch-download", s.auth(s.apiBatchTorrentDownload))
	// Torrent management APIs (paused torrents, archive)
	mux.HandleFunc("/api/torrents/paused", s.auth(s.apiPausedTorrents))
	mux.HandleFunc("/api/torrents/delete-paused", s.auth(s.apiDeletePausedTorrents))
	mux.HandleFunc("/api/torrents/archive", s.auth(s.apiArchiveTorrents))
	mux.HandleFunc("/api/torrents/", s.auth(s.apiTorrentManagementRouter))
	// Version check API
	mux.HandleFunc("/api/version", s.auth(s.apiVersion))
	mux.HandleFunc("/api/version/check", s.auth(s.apiVersionCheck))
	mux.HandleFunc("/api/version/runtime", s.auth(s.apiVersionRuntime))
	mux.HandleFunc("/api/version/upgrade", s.auth(s.apiVersionUpgrade))
	// Torrent download proxy API
	mux.HandleFunc("/api/site/", s.auth(s.apiSiteRouter))
	// Static UI - Vue 3 SPA
	distFS := mustSub(staticFS, "static/dist")
	assetsServer := http.FileServer(http.FS(distFS))
	mux.Handle("/assets/", assetsServer)
	// Legacy static files (for login page CSS) with proper MIME types
	legacyFS := mustSub(staticFS, "static")
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/static/")
		// Set proper Content-Type for CSS files
		if strings.HasSuffix(path, ".css") {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		} else if strings.HasSuffix(path, ".js") {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		}
		http.ServeFileFS(w, r, legacyFS, path)
	})
	// SPA fallback - serve index.html for all routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		sid, err := r.Cookie("session")
		if err != nil || sid.Value == "" || s.sessions[sid.Value] == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		// Serve Vue SPA index.html
		http.ServeFileFS(w, r, distFS, "index.html")
	})
	handler := logMiddleware(mux)
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	return srv.ListenAndServe()
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) { s.status = code; s.ResponseWriter.WriteHeader(code) }
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if strings.HasPrefix(origin, "chrome-extension://") || strings.HasPrefix(origin, "moz-extension://") || strings.HasPrefix(origin, "extension://") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(sr, r)
		d := time.Since(start).Milliseconds()
		if sr.status >= 400 {
			global.GetSlogger().Warnf("http method=%s path=%s status=%d dur=%dms remote=%s", r.Method, r.URL.Path, sr.status, d, r.RemoteAddr)
		} else {
			global.GetSlogger().Debugf("http method=%s path=%s status=%d dur=%dms remote=%s", r.Method, r.URL.Path, sr.status, d, r.RemoteAddr)
		}
	})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid, err := r.Cookie("session")
		if err != nil || sid.Value == "" || s.sessions[sid.Value] == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_ = s.tpl.ExecuteTemplate(w, "login", nil)
	case http.MethodPost:
		user, pass, err := readLogin(r)
		if err != nil {
			http.Error(w, "请求体错误", http.StatusBadRequest)
			return
		}
		if user == "" || pass == "" {
			http.Error(w, "用户名或密码为空", http.StatusBadRequest)
			return
		}
		global.GetSlogger().Infof("login_attempt username=%s", user)
		// 若管理员不存在则自动创建默认账户
		cnt, _ := s.store.AdminCount()
		if cnt == 0 {
			_ = s.store.EnsureAdmin("admin", hashPassword("adminadmin"))
		}
		u, err := s.store.GetAdmin(user)
		global.GetSlogger().Infof("login_admin_lookup username=%s found=%t err=%v", user, u != nil, err)
		if err != nil {
			if strings.Contains(err.Error(), "record not found") {
				http.Error(w, "用户不存在", http.StatusUnauthorized)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if u == nil {
			http.Error(w, "用户不存在", http.StatusUnauthorized)
			return
		}
		if !verifyPassword(u.PasswordHash, pass) {
			if verifyLegacyPassword(u.PasswordHash, pass) {
				u.PasswordHash = hashPassword(pass)
				_ = s.store.UpdateAdmin(u)
			} else {
				http.Error(w, "密码错误", http.StatusUnauthorized)
				return
			}
		}
		sid := randomID()
		s.sessions[sid] = u.Username
		cookie := &http.Cookie{Name: "session", Value: sid, HttpOnly: true, SameSite: http.SameSiteLaxMode, Path: "/"}
		http.SetCookie(w, cookie)

		// 登录成功后异步触发用户数据同步
		if userInfoService != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				results, errors := userInfoService.FetchAndSaveAllWithConcurrency(ctx, 3, 30*time.Second)
				global.GetSlogger().Infof("[Login] Async sync completed: %d success, %d failed", len(results), len(errors))
			}()
		}

		if isJSONRequest(r) {
			writeJSON(w, map[string]any{"success": true, "username": u.Username})
		} else {
			http.Redirect(w, r, "/", http.StatusFound)
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func readLogin(r *http.Request) (string, string, error) {
	ct := r.Header.Get("Content-Type")
	mt, _, _ := mime.ParseMediaType(ct)
	global.GetSlogger().Infof("login_ct ct=%s mt=%s", ct, mt)
	switch mt {
	case "application/json":
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return "", "", err
		}
		return strings.TrimSpace(body.Username), strings.TrimSpace(body.Password), nil
	case "multipart/form-data":
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			return "", "", err
		}
		return strings.TrimSpace(r.FormValue("username")), strings.TrimSpace(r.FormValue("password")), nil
	default:
		if err := r.ParseForm(); err != nil {
			return "", "", err
		}
		return strings.TrimSpace(r.FormValue("username")), strings.TrimSpace(r.FormValue("password")), nil
	}
}

func isJSONRequest(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.Contains(ct, "application/json")
}

func (s *Server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("session"); err == nil {
		delete(s.sessions, c.Value)
		c.MaxAge = -1
		http.SetCookie(w, c)
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// 轻量级口令哈希：salt + 多轮 sha256（避免引入新依赖以兼容 vendor）
func hashPassword(pw string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	iterations := 100000
	var sum []byte
	data := append([]byte{}, salt...)
	data = append(data, []byte(pw)...)
	h := sha256.Sum256(data)
	sum = h[:]
	for range iterations {
		h = sha256.Sum256(sum)
		sum = h[:]
	}
	return fmt.Sprintf("%s|%s|%d", hex.EncodeToString(salt), hex.EncodeToString(sum), iterations)
}

func verifyPassword(stored, pw string) bool {
	parts := strings.Split(stored, "|")
	if len(parts) != 3 {
		return false
	}
	saltHex, sumHex, itStr := parts[0], parts[1], parts[2]
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return false
	}
	iterations, err := strconv.Atoi(itStr)
	if err != nil || iterations <= 0 {
		return false
	}
	data := append([]byte{}, salt...)
	data = append(data, []byte(pw)...)
	h := sha256.Sum256(data)
	sum := h[:]
	for i := 0; i < iterations; i++ {
		h = sha256.Sum256(sum)
		sum = h[:]
	}
	expect, err := hex.DecodeString(sumHex)
	if err != nil {
		return false
	}
	if len(expect) != len(sum) {
		return false
	}
	return subtle.ConstantTimeCompare(expect, sum) == 1
}

func verifyLegacyPassword(stored, pw string) bool {
	if strings.Contains(stored, "|") {
		return false
	}
	if stored == pw {
		return true
	}
	h := sha256.Sum256([]byte(pw))
	return stored == hex.EncodeToString(h[:])
}

const loginHTML = `{{define "login"}}
<!doctype html><html><head><meta charset="utf-8"><title>登录</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="stylesheet" href="/static/style.css"></head>
<body class="login-page">
<main class="login-layout">
  <div class="login-card">
    <header class="login-card__header">
      <p class="login-eyebrow">PT TOOLS</p>
      <h1 class="login-title">欢迎登录</h1>
      <p class="login-subtitle">集中管理站点、RSS 与下载任务</p>
    </header>
    <form id="loginForm" method="post" class="login-form">
      <label for="username" class="login-label">用户名</label>
      <input id="username" name="username" placeholder="用户名" value="admin" autocomplete="username"/>
      <label for="password" class="login-label">密码</label>
      <input id="password" name="password" type="password" placeholder="密码" autocomplete="current-password"/>
      <div class="login-actions">
        <button type="submit" class="login-button">登录</button>
      </div>
    </form>
  </div>
</main>
<script>
  const form = document.getElementById('loginForm');
  form.addEventListener('submit', async (e)=>{
    e.preventDefault();
    const fd = new FormData(form);
    try{
      const r = await fetch('/login', {method:'POST', body: fd});
      if(!r.ok){ const msg = await r.text(); alert(msg || '密码错误'); return; }
      location.href = '/';
    }catch(err){ alert('登录失败: '+(err?.message||'未知错误')); }
  });
</script>
</body></html>{{end}}`

// JSON APIs
func (s *Server) apiGlobal(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		store := core.NewConfigStore(global.GlobalDB)
		gs, err := store.GetGlobalSettings()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, gs)
	case http.MethodPost:
		var req struct {
			DefaultIntervalMinutes int32   `json:"default_interval_minutes"`
			DefaultInterval        int64   `json:"default_interval"`
			DownloadDir            string  `json:"download_dir"`
			DownloadLimitEnabled   bool    `json:"download_limit_enabled"`
			DownloadSpeedLimit     int     `json:"download_speed_limit"`
			TorrentSizeGB          int     `json:"torrent_size_gb"`
			MinFreeMinutes         int     `json:"min_free_minutes"`
			AutoStart              bool    `json:"auto_start"`
			CleanupEnabled         bool    `json:"cleanup_enabled"`
			CleanupIntervalMin     int     `json:"cleanup_interval_min"`
			CleanupScope           string  `json:"cleanup_scope"`
			CleanupScopeTags       string  `json:"cleanup_scope_tags"`
			CleanupRemoveData      bool    `json:"cleanup_remove_data"`
			CleanupConditionMode   string  `json:"cleanup_condition_mode"`
			CleanupMaxSeedTimeH    int     `json:"cleanup_max_seed_time_h"`
			CleanupMinRatio        float64 `json:"cleanup_min_ratio"`
			CleanupMaxInactiveH    int     `json:"cleanup_max_inactive_h"`
			CleanupSlowSeedTimeH   int     `json:"cleanup_slow_seed_time_h"`
			CleanupSlowMaxRatio    float64 `json:"cleanup_slow_max_ratio"`
			CleanupDelFreeExpired  bool    `json:"cleanup_del_free_expired"`
			CleanupDiskProtect     bool    `json:"cleanup_disk_protect"`
			CleanupMinDiskSpaceGB  float64 `json:"cleanup_min_disk_space_gb"`
			CleanupProtectDL       bool    `json:"cleanup_protect_dl"`
			CleanupProtectHR       bool    `json:"cleanup_protect_hr"`
			CleanupMinRetainH      int     `json:"cleanup_min_retain_h"`
			CleanupProtectTags     string  `json:"cleanup_protect_tags"`
			AutoDeleteOnFreeEnd    bool    `json:"auto_delete_on_free_end"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.DefaultIntervalMinutes == 0 && req.DefaultInterval > 0 {
			req.DefaultIntervalMinutes = int32(req.DefaultInterval / int64(time.Minute))
		}
		if req.DefaultIntervalMinutes < models.MinIntervalMinutes {
			req.DefaultIntervalMinutes = models.MinIntervalMinutes
		}
		if strings.TrimSpace(req.DownloadDir) == "" {
			http.Error(w, "下载目录不能为空", http.StatusBadRequest)
			return
		}
		gs := models.SettingsGlobal{
			DefaultIntervalMinutes: req.DefaultIntervalMinutes,
			DownloadDir:            req.DownloadDir,
			DownloadLimitEnabled:   req.DownloadLimitEnabled,
			DownloadSpeedLimit:     req.DownloadSpeedLimit,
			TorrentSizeGB:          req.TorrentSizeGB,
			MinFreeMinutes:         req.MinFreeMinutes,
			AutoStart:              req.AutoStart,
			CleanupEnabled:         req.CleanupEnabled,
			CleanupIntervalMin:     req.CleanupIntervalMin,
			CleanupScope:           req.CleanupScope,
			CleanupScopeTags:       req.CleanupScopeTags,
			CleanupRemoveData:      req.CleanupRemoveData,
			CleanupConditionMode:   req.CleanupConditionMode,
			CleanupMaxSeedTimeH:    req.CleanupMaxSeedTimeH,
			CleanupMinRatio:        req.CleanupMinRatio,
			CleanupMaxInactiveH:    req.CleanupMaxInactiveH,
			CleanupSlowSeedTimeH:   req.CleanupSlowSeedTimeH,
			CleanupSlowMaxRatio:    req.CleanupSlowMaxRatio,
			CleanupDelFreeExpired:  req.CleanupDelFreeExpired,
			CleanupDiskProtect:     req.CleanupDiskProtect,
			CleanupMinDiskSpaceGB:  req.CleanupMinDiskSpaceGB,
			CleanupProtectDL:       req.CleanupProtectDL,
			CleanupProtectHR:       req.CleanupProtectHR,
			CleanupMinRetainH:      req.CleanupMinRetainH,
			CleanupProtectTags:     req.CleanupProtectTags,
			AutoDeleteOnFreeEnd:    req.AutoDeleteOnFreeEnd,
		}
		if err := s.store.SaveGlobalSettings(gs); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// 异步重新加载并触发任务重启，让 API 快速返回
		go func() {
			cfg, _ := s.store.Load()
			if cfg != nil {
				global.GetSlogger().Info("[Config] 异步重载配置...")
				s.mgr.Reload(cfg)
				global.GetSlogger().Info("[Config] 配置重载完成")
			}
		}()
		writeJSON(w, map[string]string{"status": "ok"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) apiQbit(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		store := core.NewConfigStore(global.GlobalDB)
		qb, err := store.GetQbitSettings()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, qb)
	case http.MethodPost:
		var qb models.QbitSettings
		if err := json.NewDecoder(r.Body).Decode(&qb); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.store.SaveQbitSettings(qb); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// 异步重新加载并触发任务重启，让 API 快速返回
		go func() {
			cfg, _ := s.store.Load()
			if cfg != nil {
				global.GetSlogger().Info("[Qbit] 异步重载配置...")
				s.mgr.Reload(cfg)
				global.GetSlogger().Info("[Qbit] 配置重载完成")
			}
		}()
		writeJSON(w, map[string]string{"status": "ok"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type SiteConfigResponse struct {
	Enabled           *bool              `json:"enabled"`
	AuthMethod        string             `json:"auth_method"`
	Cookie            string             `json:"cookie"`
	APIKey            string             `json:"api_key"`
	APIUrl            string             `json:"api_url"`
	Passkey           string             `json:"passkey"`
	RSS               []models.RSSConfig `json:"rss"`
	URLs              []string           `json:"urls,omitempty"`
	Unavailable       bool               `json:"unavailable,omitempty"`
	UnavailableReason string             `json:"unavailable_reason,omitempty"`
	IsBuiltin         bool               `json:"is_builtin"`
}

func (s *Server) apiSites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sites, err := s.store.ListSites()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defRegistry := v2.GetDefinitionRegistry()
		result := make(map[models.SiteGroup]SiteConfigResponse)
		var sitesToDisable []models.SiteGroup
		for sg, sc := range sites {
			resp := SiteConfigResponse{
				Enabled:    sc.Enabled,
				AuthMethod: sc.AuthMethod,
				Cookie:     sc.Cookie,
				APIKey:     sc.APIKey,
				APIUrl:     sc.APIUrl,
				Passkey:    sc.Passkey,
				RSS:        sc.RSS,
			}
			if def, ok := defRegistry.Get(string(sg)); ok {
				resp.URLs = def.URLs
				resp.Unavailable = def.Unavailable
				resp.UnavailableReason = def.UnavailableReason
				resp.IsBuiltin = true
				if def.Unavailable {
					f := false
					resp.Enabled = &f
					if sc.Enabled != nil && *sc.Enabled {
						sitesToDisable = append(sitesToDisable, sg)
					}
				}
			}
			result[sg] = resp
		}
		if len(sitesToDisable) > 0 {
			go s.disableUnavailableSites(sitesToDisable)
		}
		writeJSON(w, result)
	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(w, "缺少站点名称", http.StatusBadRequest)
			return
		}
		global.GetSlogger().Infof("[Site] 删除站点: name=%s", name)
		if err := s.store.DeleteSite(name); err != nil {
			global.GetSlogger().Errorf("[Site] 删除站点失败: name=%s, err=%v", name, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		global.GetSlogger().Infof("[Site] 站点删除成功: name=%s", name)
		// 异步重新加载并触发任务重启，让 API 快速返回
		go func() {
			// 刷新 UserInfoService 站点注册
			if err := RefreshSiteRegistrations(s.store); err != nil {
				global.GetSlogger().Warnf("[Site] 刷新站点注册失败: %v", err)
			}
			cfg, _ := s.store.Load()
			if cfg != nil {
				global.GetSlogger().Info("[Site] 异步重载配置...")
				s.mgr.Reload(cfg)
				global.GetSlogger().Info("[Site] 配置重载完成")
			}
		}()
		writeJSON(w, map[string]string{"status": "ok"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) disableUnavailableSites(sites []models.SiteGroup) {
	for _, sg := range sites {
		disabled := models.SiteConfig{Enabled: func() *bool { f := false; return &f }()}
		if _, err := s.store.UpsertSite(sg, disabled); err != nil {
			global.GetSlogger().Warnf("[Site] 禁用不可用站点失败: %s, err=%v", sg, err)
			continue
		}
		global.GetSlogger().Infof("[Site] 已自动禁用不可用站点: %s", sg)
	}
	cfg, _ := s.store.Load()
	if cfg != nil {
		s.mgr.Reload(cfg)
	}
}

func (s *Server) apiSiteDetail(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/sites/")
	registry := v2.GetGlobalSiteRegistry()
	if _, ok := registry.Get(strings.ToLower(name)); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	sg := models.SiteGroup(strings.ToLower(name))
	switch r.Method {
	case http.MethodGet:
		sc, err := s.store.GetSiteConf(sg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		resp := SiteConfigResponse{
			Enabled:    sc.Enabled,
			AuthMethod: sc.AuthMethod,
			Cookie:     sc.Cookie,
			APIKey:     sc.APIKey,
			APIUrl:     sc.APIUrl,
			Passkey:    sc.Passkey,
			RSS:        sc.RSS,
		}
		defRegistry := v2.GetDefinitionRegistry()
		if def, ok := defRegistry.Get(string(sg)); ok {
			resp.URLs = def.URLs
			resp.Unavailable = def.Unavailable
			resp.UnavailableReason = def.UnavailableReason
			resp.IsBuiltin = true
		}
		writeJSON(w, resp)
	case http.MethodPost:
		var sc models.SiteConfig
		if err := json.NewDecoder(r.Body).Decode(&sc); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		global.GetSlogger().Infof("[RSS] 保存站点配置: site=%s, rss_count=%d", name, len(sc.RSS))
		for i, r := range sc.RSS {
			global.GetSlogger().Infof("[RSS] RSS[%d]: name=%s, url=%s", i, r.Name, r.URL)
		}
		if err := s.store.UpsertSiteWithRSS(sg, sc); err != nil {
			global.GetSlogger().Errorf("[RSS] 保存站点配置失败: site=%s, err=%v", name, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		global.GetSlogger().Infof("[RSS] 站点配置保存成功: site=%s", name)
		// 异步重新加载并触发任务重启，让 API 快速返回
		go func() {
			// 刷新 UserInfoService 站点注册
			if err := RefreshSiteRegistrations(s.store); err != nil {
				global.GetSlogger().Warnf("[Site] 刷新站点注册失败: %v", err)
			}
			cfg, _ := s.store.Load()
			if cfg != nil {
				global.GetSlogger().Info("[RSS] 异步重载配置...")
				s.mgr.Reload(cfg)
				global.GetSlogger().Info("[RSS] 配置重载完成")
			}
		}()
		writeJSON(w, map[string]string{"status": "ok"})
	case http.MethodDelete:
		// 删除单条 RSS：通过查询参数 id 指定
		idStr := r.URL.Query().Get("id")
		if strings.TrimSpace(idStr) == "" {
			http.Error(w, "缺少 RSS id", http.StatusBadRequest)
			return
		}
		rid, convErr := strconv.ParseUint(idStr, 10, 64)
		if convErr != nil {
			http.Error(w, "RSS id 非法", http.StatusBadRequest)
			return
		}
		global.GetSlogger().Infof("[RSS] 删除 RSS: site=%s, rss_id=%d", name, rid)
		// 查找站点
		db := global.GlobalDB.DB
		var site models.SiteSetting
		if err := db.Where("name = ?", string(sg)).First(&site).Error; err != nil {
			global.GetSlogger().Errorf("[RSS] 删除 RSS 失败，站点不存在: site=%s", name)
			http.Error(w, "站点不存在", http.StatusBadRequest)
			return
		}
		// 先查询要删除的 RSS 信息用于日志
		var rssToDelete models.RSSSubscription
		if err := db.Where("site_id = ? AND id = ?", site.ID, uint(rid)).First(&rssToDelete).Error; err == nil {
			global.GetSlogger().Infof("[RSS] 删除 RSS 详情: name=%s, url=%s", rssToDelete.Name, rssToDelete.URL)
		}
		if err := db.Where("site_id = ? AND id = ?", site.ID, uint(rid)).Delete(&models.RSSSubscription{}).Error; err != nil {
			global.GetSlogger().Errorf("[RSS] 删除 RSS 失败: site=%s, rss_id=%d, err=%v", name, rid, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		global.GetSlogger().Infof("[RSS] RSS 删除成功: site=%s, rss_id=%d", name, rid)
		// 异步重新加载并触发任务重启，让 API 快速返回
		go func() {
			cfg, _ := s.store.Load()
			if cfg != nil {
				global.GetSlogger().Info("[RSS] 异步重载配置...")
				s.mgr.Reload(cfg)
				global.GetSlogger().Info("[RSS] 配置重载完成")
			}
		}()
		writeJSON(w, map[string]string{"status": "deleted"})
	case http.MethodPut:
		s.updateSiteCredential(w, r, sg)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) apiPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct{ Username, Old, New string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	u, err := s.store.GetAdmin(body.Username)
	if err != nil || u == nil || !verifyPassword(u.PasswordHash, body.Old) {
		http.Error(w, "原密码错误", http.StatusUnauthorized)
		return
	}
	u.PasswordHash = hashPassword(body.New)
	if err := s.store.UpdateAdmin(u); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// 控制接口：一键停止/启动任务
func (s *Server) apiStopAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.mgr.StopAll()
	writeJSON(w, map[string]string{"status": "stopped"})
}

func (s *Server) apiStartAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cfg, _ := s.store.Load()
	if cfg == nil || strings.TrimSpace(cfg.Global.DownloadDir) == "" {
		http.Error(w, "配置未就绪", http.StatusBadRequest)
		return
	}
	if !cfg.Global.AutoStart {
		// manual start: allow request to control start
	}
	s.mgr.StartAll(cfg)
	writeJSON(w, map[string]string{"status": "started"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// static assets
//
//go:embed static/* static/dist/* static/dist/assets/*
var staticFS embed.FS

func mustSub(fsys embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

// 任务列表 API：分页、过滤、搜索、排序
func (s *Server) apiTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	site := r.URL.Query().Get("site")
	sort := r.URL.Query().Get("sort")
	downloaded := r.URL.Query().Get("downloaded") == "1"
	pushed := r.URL.Query().Get("pushed") == "1"
	expired := r.URL.Query().Get("expired") == "1"
	page := 1
	size := 20
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ssz := r.URL.Query().Get("page_size"); ssz != "" {
		if v, err := strconv.Atoi(ssz); err == nil && v > 0 && v <= 500 {
			size = v
		}
	}
	// 简化：第一页，固定 50 条
	db := global.GlobalDB.DB
	tx := db.Model(&models.TorrentInfo{})
	if site != "" {
		tx = tx.Where("site_name = ?", site)
	}
	if q != "" {
		like := "%" + q + "%"
		tx = tx.Where("title LIKE ? OR torrent_hash LIKE ?", like, like)
	}
	if downloaded {
		tx = tx.Where("is_downloaded = ?", true)
	}
	if pushed {
		tx = tx.Where("is_pushed = ?", true)
	}
	if expired {
		tx = tx.Where("is_expired = ?", true)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch sort {
	case "created_at_asc":
		tx = tx.Order("created_at ASC")
	default:
		tx = tx.Order("created_at DESC")
	}
	var items []models.TorrentInfo
	if err := tx.Limit(size).Offset((page - 1) * size).Find(&items).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, struct {
		Items []models.TorrentInfo `json:"items"`
		Total int64                `json:"total"`
		Page  int                  `json:"page"`
		Size  int                  `json:"page_size"`
	}{Items: items, Total: total, Page: page, Size: size})
}

// 日志查看接口：最多返回 5000 行，实时读取当前日志文件
func (s *Server) apiLogs(w http.ResponseWriter, r *http.Request) {
	homeDir, _ := os.UserHomeDir()
	logPath := filepath.Join(homeDir, models.WorkDir, config.DefaultZapConfig.Directory, "all.log")
	f, err := os.Open(logPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	// 读取最后 5000 行（简化：读取整个文件并截取末尾）
	b, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	content := string(b)
	linesAll := strings.Split(content, "\n")
	truncated := false
	lines := linesAll
	if len(linesAll) > 5000 {
		truncated = true
		lines = linesAll[len(linesAll)-5000:]
	}
	type out struct {
		Lines     []string `json:"lines"`
		Path      string   `json:"path"`
		Truncated bool     `json:"truncated"`
	}
	writeJSON(w, out{Lines: lines, Path: logPath, Truncated: truncated})
}

// 已弃用：服务端模板 password，改为单页前端表单
