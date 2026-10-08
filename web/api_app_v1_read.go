package web

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/dlassistant"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// App API v1 的读接口：概览、站点、RSS 推送记录、站点图标。

var (
	// urlQueryRe 是文本里带查询串的地址（不分大小写，含 udp）：passkey、签名之类多在查询串里，整段去掉。
	urlQueryRe = regexp.MustCompile(`(?i)\b((?:https?|udp)://[^\s?#"'<>]+)\?[^\s"'<>]*`)
	// urlUserinfoRe 是地址里的用户名与密码。
	urlUserinfoRe = regexp.MustCompile(`(?i)\b((?:https?|udp)://)[^/\s@"'<>]+@`)
	// jsonSecretRe 是上游回应体里 JSON 形式的凭证字段。
	jsonSecretRe = regexp.MustCompile(`(?i)"(passkey|authkey|torrent_pass|credential|token|apikey|api_key|sign|secret|rsskey|key|password|passwd|cookie)"\s*:\s*"[^"]*"`)
	// plainSecretRe 补上 RedactTrackerMessage 不管的 password=、cookie=。
	plainSecretRe = regexp.MustCompile(`(?i)\b(password|passwd|pwd|cookie)=([^&\s"'<>(),;]+)`)
)

// appRedact 遮住要交给 App 的文本里可能带的凭证：错误信息常拼着上游的回应体与请求地址。去掉地址的用户信息与整段查询串，
// 遮 JSON 与 key=value 形式的凭证字段，再按下载器助手遮 tracker 回复的规则（RedactTrackerMessage）遮地址路径里的长串、
// passkey=… 一类参数与 24 位以上的长串。
func appRedact(s string) string {
	s = urlUserinfoRe.ReplaceAllString(s, "$1***@")
	s = urlQueryRe.ReplaceAllString(s, "$1?…")
	s = jsonSecretRe.ReplaceAllString(s, `"$1":"***"`)
	s = plainSecretRe.ReplaceAllString(s, "$1=***")
	return dlassistant.RedactTrackerMessage(s)
}

var (
	// addrURLRe 是任意的绝对地址（下载器的地址也在里面）。
	addrURLRe = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`)
	// addrDialRe、addrLookupRe 是 Go 网络错误里的「dial tcp 主机:端口:」与「lookup 主机 on DNS:」。
	addrDialRe   = regexp.MustCompile(`\b(dial (?:tcp|udp)[46]?) [^\s]+?:(\s)`)
	addrLookupRe = regexp.MustCompile(`\blookup [^\s:]+(?: on [^\s]+:\d+)?`)
	// addrIPRe 是剩下的 IPv4 与带方括号的 IPv6（可以带端口）。
	addrIPRe = regexp.MustCompile(`\[[0-9a-fA-F:.]+\](?::\d{1,5})?|\b(?:\d{1,3}\.){3}\d{1,3}(?::\d{1,5})?\b`)
)

// appRedactAddr 在 appRedact 之外再去掉网络地址：下载器的错误常带着它的内网地址与端口（Go 的 HTTP、网络错误会带上完整的请求地址），
// App 与 MCP 拿到的只说是什么错。
func appRedactAddr(s string) string {
	s = addrURLRe.ReplaceAllString(appRedact(s), "<地址>")
	s = addrDialRe.ReplaceAllString(s, "$1 <地址>:$2")
	s = addrLookupRe.ReplaceAllString(s, "lookup <地址>")
	return addrIPRe.ReplaceAllString(s, "<地址>")
}

func unixPtr(t *time.Time) *int64 {
	if t == nil || t.IsZero() {
		return nil
	}
	v := t.Unix()
	return &v
}

// ---- 概览 ----

// AppOverview 是 GET /overview：已启用站点的合计与今天的增量。
type AppOverview struct {
	Totals    AppTotals `json:"totals"`
	Today     AppDelta  `json:"today"`
	UpdatedAt int64     `json:"updated_at"`
}

// AppTotals 是已启用站点的合计。
type AppTotals struct {
	Uploaded       int64   `json:"uploaded"`
	Downloaded     int64   `json:"downloaded"`
	Ratio          float64 `json:"ratio"`
	Seeding        int     `json:"seeding"`
	Leeching       int     `json:"leeching"`
	Bonus          float64 `json:"bonus"`
	BonusPerHour   float64 `json:"bonus_per_hour"`
	SeedingSize    int64   `json:"seeding_size"`
	SiteCount      int     `json:"site_count"`
	UnreadMessages int     `json:"unread_messages"`
}

// AppDelta 是一段时间里的增量（站点数据历史，M2）。Error 不为空时增量没算出来，数字都是 0，不代表这段时间没有流量。
type AppDelta struct {
	From       string         `json:"from"`
	To         string         `json:"to"`
	Uploaded   int64          `json:"uploaded"`
	Downloaded int64          `json:"downloaded"`
	Bonus      float64        `json:"bonus"`
	Sites      []AppSiteDelta `json:"sites"`
	Error      string         `json:"error,omitempty"`
}

// AppSiteDelta 是一个站点的增量。
type AppSiteDelta struct {
	Site       string  `json:"site"`
	Uploaded   int64   `json:"uploaded"`
	Downloaded int64   `json:"downloaded"`
	Bonus      float64 `json:"bonus"`
}

func (s *Server) appOverview(w http.ResponseWriter, r *http.Request) {
	if userInfoService == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "站点数据服务没有启动")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	enabled, err := s.enabledSiteSet()
	if err != nil {
		appError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	stats, err := userInfoService.GetAggregated(ctx)
	if err != nil {
		appError(w, http.StatusInternalServerError, "internal", "读取站点数据失败: "+err.Error())
		return
	}
	if enabled != nil {
		stats = filterStatsByEnabledSites(stats, enabled)
	}
	out := AppOverview{
		Totals: AppTotals{
			Uploaded: stats.TotalUploaded, Downloaded: stats.TotalDownloaded, Ratio: stats.AverageRatio,
			Seeding: stats.TotalSeeding, Leeching: stats.TotalLeeching, Bonus: stats.TotalBonus, BonusPerHour: stats.TotalBonusPerHour,
			SeedingSize: stats.TotalSeederSize, SiteCount: stats.SiteCount, UnreadMessages: stats.TotalUnreadMessages,
		},
		Today:     AppDelta{Sites: []AppSiteDelta{}},
		UpdatedAt: stats.LastUpdate,
	}
	if repo, ok := userInfoService.History(); !ok {
		out.Today.Error = "没有站点数据的历史，算不出今天的增量"
	} else if sum, serr := v2.LoadDeltaSummary(ctx, repo, "today"); serr != nil {
		global.GetSlogger().Warnf("[App API] 计算今天的增量失败: %v", serr)
		out.Today.Error = "计算今天的增量失败: " + appRedact(serr.Error())
	} else {
		sum = sum.Filter(enabled)
		out.Today.From, out.Today.To = sum.From, sum.To
		out.Today.Uploaded, out.Today.Downloaded, out.Today.Bonus = sum.TotalUploaded, sum.TotalDownloaded, sum.TotalBonus
		for _, d := range sum.Sites {
			out.Today.Sites = append(out.Today.Sites, AppSiteDelta{Site: d.Site, Uploaded: d.Uploaded, Downloaded: d.Downloaded, Bonus: d.Bonus})
		}
	}
	appJSON(w, out)
}

// ---- 站点 ----

// AppSiteList 是 GET /sites 的回应。UserError 不为空时站点上的用户数据没读到，各项都没有 user，不代表从没同步过。
type AppSiteList struct {
	Items     []AppSite `json:"items"`
	UserError string    `json:"user_error,omitempty"`
}

// AppSite 是 GET /sites 的一项：站点、登录状态、今天的签到与站点上的用户数据。不含 Cookie、API key、passkey 与站点地址。
type AppSite struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Enabled     bool              `json:"enabled"`
	Login       AppSiteLogin      `json:"login"`
	Attendance  AppSiteAttendance `json:"attendance"`
	User        *AppSiteUser      `json:"user,omitempty"`
}

// AppSiteLogin 是登录状态：Tier 是提醒档位（none、30d、14d、7d、3d、banned-imminent，没有访问记录时是 unknown），
// DaysRemaining 是离封号阈值还有几天。
type AppSiteLogin struct {
	Tier            string `json:"tier"`
	DaysRemaining   int    `json:"days_remaining"`
	LastActiveAt    *int64 `json:"last_active_at,omitempty"`
	LastProbeAt     *int64 `json:"last_probe_at,omitempty"`
	LastProbeStatus string `json:"last_probe_status,omitempty"`
}

// AppSiteAttendance 是今天的签到。
type AppSiteAttendance struct {
	Supported     bool   `json:"supported"`
	Enabled       bool   `json:"enabled"`
	Day           string `json:"day"`
	Status        string `json:"status,omitempty"`
	Message       string `json:"message,omitempty"`
	LastAttemptAt *int64 `json:"last_attempt_at,omitempty"`
}

// AppSiteUser 是站点上的用户数据。
type AppSiteUser struct {
	Username       string  `json:"username"`
	Level          string  `json:"level,omitempty"`
	Uploaded       int64   `json:"uploaded"`
	Downloaded     int64   `json:"downloaded"`
	Ratio          float64 `json:"ratio"`
	Bonus          float64 `json:"bonus"`
	BonusPerHour   float64 `json:"bonus_per_hour,omitempty"`
	Seeding        int     `json:"seeding"`
	Leeching       int     `json:"leeching"`
	SeedingSize    int64   `json:"seeding_size,omitempty"`
	UnreadMessages int     `json:"unread_messages"`
	HnRUnsatisfied int     `json:"hnr_unsatisfied,omitempty"`
	UpdatedAt      int64   `json:"updated_at"`
}

func (s *Server) appSites(w http.ResponseWriter, r *http.Request) {
	if global.GlobalDB == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "数据库没有初始化")
		return
	}
	db := global.GlobalDB.DB
	sites, err := models.NewSiteRepository(db).ListSites()
	if err != nil {
		appError(w, http.StatusInternalServerError, "internal", "读取站点失败: "+err.Error())
		return
	}
	states, err := models.NewSiteLoginStateRepository(db).ListLoginStates(false)
	if err != nil {
		appError(w, http.StatusInternalServerError, "internal", "读取登录状态失败: "+err.Error())
		return
	}
	stateByName := make(map[string]models.SiteLoginState, len(states))
	for i := range states {
		stateByName[states[i].SiteName] = states[i]
	}
	day := s.attendanceDay()
	logs, err := models.NewSiteAttendanceRepository(db).ListDay(day)
	if err != nil {
		appError(w, http.StatusInternalServerError, "internal", "读取签到记录失败: "+err.Error())
		return
	}
	logByName := make(map[string]models.SiteAttendanceLog, len(logs))
	for _, l := range logs {
		logByName[l.SiteName] = l
	}
	users := map[string]v2.UserInfo{}
	userErr := ""
	if userInfoService == nil {
		userErr = "站点数据服务没有启动"
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		infos, uerr := userInfoService.GetAllUserInfo(ctx)
		cancel()
		if uerr != nil {
			global.GetSlogger().Warnf("[App API] 读取站点用户数据失败: %v", uerr)
			userErr = "读取站点用户数据失败: " + appRedact(uerr.Error())
		}
		for _, u := range infos {
			users[strings.ToLower(u.Site)] = u
		}
	}
	now := time.Now()
	out := make([]AppSite, 0, len(sites))
	for _, st := range sites {
		state, ok := stateByName[st.Name]
		if !ok {
			ban, remind, _ := models.ApplyPresetIfMissing(st.Name)
			state = models.SiteLoginState{SiteName: st.Name, BanThresholdDays: ban, RemindBeforeDays: remind, LastReminderTier: "none", ProbeMode: "auto"}
		}
		login := buildLoginStateResponse(st, state, now)
		var today *models.SiteAttendanceLog
		if l, ok := logByName[st.Name]; ok {
			today = &l
		}
		att := buildAttendanceResponse(st, day, today)
		item := AppSite{
			Name: st.Name, DisplayName: login.DisplayName, Enabled: st.Enabled,
			Login: AppSiteLogin{
				Tier: login.Tier, DaysRemaining: login.DaysRemaining, LastActiveAt: login.EffectiveLastActiveAt,
				LastProbeAt: login.LastProbeAt, LastProbeStatus: login.LastProbeStatus,
			},
			Attendance: AppSiteAttendance{
				Supported: att.Supported, Enabled: att.AttendanceEnabled, Day: att.Day, Status: att.Status,
				Message: appRedact(att.Message), LastAttemptAt: att.LastAttemptAt,
			},
		}
		if item.DisplayName == "" {
			item.DisplayName = st.Name
		}
		if u, ok := users[strings.ToLower(st.Name)]; ok {
			item.User = &AppSiteUser{
				Username: u.Username, Level: u.LevelName, Uploaded: u.Uploaded, Downloaded: u.Downloaded, Ratio: u.Ratio,
				Bonus: u.Bonus, BonusPerHour: u.BonusPerHour, Seeding: u.Seeding, Leeching: u.Leeching, SeedingSize: u.SeederSize,
				UnreadMessages: u.UnreadMessageCount, HnRUnsatisfied: u.HnRUnsatisfied, UpdatedAt: u.LastUpdate,
			}
		}
		out = append(out, item)
	}
	appJSON(w, AppSiteList{Items: out, UserError: userErr})
}

// ---- RSS 推送记录 ----

// AppTask 是 GET /tasks 的一项：RSS 推送记录。不含下载地址；错误信息去掉了地址里的查询串。
type AppTask struct {
	ID          uint    `json:"id"`
	Site        string  `json:"site"`
	TorrentID   string  `json:"torrent_id"`
	Title       string  `json:"title"`
	Size        int64   `json:"size"`
	Category    string  `json:"category,omitempty"`
	Tags        string  `json:"tags,omitempty"`
	Free        bool    `json:"free"`
	FreeLevel   string  `json:"free_level,omitempty"`
	FreeEndAt   *int64  `json:"free_end_at,omitempty"`
	HasHR       bool    `json:"has_hr"`
	Pushed      bool    `json:"pushed"`
	PushedAt    *int64  `json:"pushed_at,omitempty"`
	Downloader  string  `json:"downloader,omitempty"`
	Progress    float64 `json:"progress"`
	Completed   bool    `json:"completed"`
	CompletedAt *int64  `json:"completed_at,omitempty"`
	Source      string  `json:"source,omitempty"`
	Error       string  `json:"error,omitempty"`
	CreatedAt   int64   `json:"created_at"`
}

// AppPage 是分页的回应。
type AppPage[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// appPaging 读 page、page_size（默认 20，最多 max）。
func appPaging(r *http.Request, max int) (page, size int, ok bool) {
	page, size = 1, 20
	if v := r.URL.Query().Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return 0, 0, false
		}
		page = n
	}
	if v := r.URL.Query().Get("page_size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > max {
			return 0, 0, false
		}
		size = n
	}
	return page, size, true
}

func (s *Server) appTasks(w http.ResponseWriter, r *http.Request) {
	if global.GlobalDB == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "数据库没有初始化")
		return
	}
	page, size, ok := appPaging(r, 100)
	if !ok {
		appError(w, http.StatusBadRequest, "invalid_argument", "page 从 1 开始，page_size 在 1 到 100 之间")
		return
	}
	q := r.URL.Query()
	tx := global.GlobalDB.DB.WithContext(r.Context()).Model(&models.TorrentInfo{})
	if site := strings.TrimSpace(q.Get("site")); site != "" {
		tx = tx.Where("site_name = ?", site)
	}
	if kw := strings.TrimSpace(q.Get("q")); kw != "" {
		if len(kw) > 200 {
			appError(w, http.StatusBadRequest, "invalid_argument", "关键字太长")
			return
		}
		tx = tx.Where("title LIKE ?", "%"+kw+"%")
	}
	if q.Get("pushed") == "1" {
		tx = tx.Where("is_pushed = ?", true)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		appError(w, http.StatusInternalServerError, "internal", "读取推送记录失败: "+err.Error())
		return
	}
	var rows []models.TorrentInfo
	if err := tx.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		appError(w, http.StatusInternalServerError, "internal", "读取推送记录失败: "+err.Error())
		return
	}
	out := AppPage[AppTask]{Items: make([]AppTask, 0, len(rows)), Total: int(total), Page: page, PageSize: size}
	for _, t := range rows {
		out.Items = append(out.Items, AppTask{
			ID: t.ID, Site: t.SiteName, TorrentID: t.TorrentID, Title: t.Title, Size: t.TorrentSize, Category: t.Category, Tags: t.Tag,
			Free: t.IsFree, FreeLevel: t.FreeLevel, FreeEndAt: unixPtr(t.FreeEndTime), HasHR: t.HasHR,
			Pushed: t.IsPushed != nil && *t.IsPushed, PushedAt: unixPtr(t.PushTime), Downloader: t.DownloaderName,
			Progress: t.Progress, Completed: t.IsCompleted, CompletedAt: unixPtr(t.CompletedAt), Source: t.DownloadSource,
			Error: appRedactAddr(t.LastError), CreatedAt: t.CreatedAt.Unix(),
		})
	}
	appJSON(w, out)
}

// ---- 站点图标 ----

func (s *Server) appFavicon(w http.ResponseWriter, r *http.Request) {
	site := strings.ToLower(strings.TrimSpace(r.PathValue("site")))
	if site == "" || len(site) > 64 || strings.ContainsAny(site, "/\\") {
		appError(w, http.StatusBadRequest, "invalid_argument", "站点名不对")
		return
	}
	s.serveSiteFavicon(w, r, site)
}
