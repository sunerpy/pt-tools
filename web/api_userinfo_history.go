package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// maxHistoryDays 是 /api/v2/userinfo/history 一次最多取的天数，与快照保留天数一致。
const maxHistoryDays = v2.SnapshotRetentionDays

// UserInfoHistoryResponse 是某站最近若干天的每日数据与增量。
type UserInfoHistoryResponse struct {
	Site   string          `json:"site"`
	Days   int             `json:"days"`
	From   string          `json:"from"`
	To     string          `json:"to"`
	Points []v2.DailyPoint `json:"points"`
}

func userInfoHistoryRepo(w http.ResponseWriter) (v2.UserInfoHistoryRepo, bool) {
	if userInfoService == nil {
		http.Error(w, "User info service not initialized", http.StatusServiceUnavailable)
		return nil, false
	}
	repo, ok := userInfoService.History()
	if !ok {
		http.Error(w, "用户数据仓库不支持历史数据", http.StatusServiceUnavailable)
		return nil, false
	}
	return repo, true
}

// apiUserInfoHistory handles GET /api/v2/userinfo/history?site=&days=
//
// 返回该站最近 days 天（含今天）每天的数据与相对上一份快照的增量；区间第一天的基线是区间之前最近的一份快照。
func (s *Server) apiUserInfoHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	site := strings.TrimSpace(r.URL.Query().Get("site"))
	if site == "" {
		http.Error(w, "缺少 site 参数", http.StatusBadRequest)
		return
	}
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > maxHistoryDays {
			http.Error(w, "days 应为 1–400 的整数", http.StatusBadRequest)
			return
		}
		days = v
	}
	repo, ok := userInfoHistoryRepo(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	to := repo.Today()
	from, err := v2.AddDays(to, -(days - 1))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	snaps, err := repo.ListSnapshots(ctx, site, from, to)
	if err != nil {
		global.GetSlogger().Errorf("[UserInfo] 读取 %s 的历史数据失败: %v", site, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	baselines, err := repo.SnapshotBaselines(ctx, from)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var baseline *v2.UserInfoDailySnapshot
	if b, ok := baselines[site]; ok {
		baseline = &b
	}
	writeJSON(w, UserInfoHistoryResponse{
		Site: site, Days: days, From: from, To: to,
		Points: v2.BuildDailyPoints(snaps, baseline),
	})
}

// apiUserInfoSummary handles GET /api/v2/userinfo/summary?range=today|7d|30d
//
// 各站在区间内的增量与合计，只算已启用的站点（与 /api/v2/userinfo/aggregated 一致）。
// 数据回退的那一项按 0 计、不进合计。
func (s *Server) apiUserInfoSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "today"
	}
	repo, ok := userInfoHistoryRepo(w)
	if !ok {
		return
	}
	if _, _, err := v2.RangeBounds(rangeName, repo.Today()); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	sum, err := v2.LoadDeltaSummary(ctx, repo, rangeName)
	if err != nil {
		global.GetSlogger().Errorf("[UserInfo] 计算增量失败: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, filterSummaryByEnabledSites(sum, s.enabledSiteSet()))
}

// enabledSiteSet 返回已启用站点名（小写）。读不到站点配置时返回空表（什么都不算），
// 与 /api/v2/userinfo/aggregated 的口径一致，两处的合计才对得上；没有配置存储时返回 nil，不过滤。
func (s *Server) enabledSiteSet() map[string]bool {
	if s.store == nil {
		return nil
	}
	out := map[string]bool{}
	sites, err := s.store.ListSites()
	if err != nil {
		global.GetSlogger().Warnf("[UserInfo] 读取站点配置失败，增量按没有已启用站点处理: %v", err)
		return out
	}
	for group, cfg := range sites {
		if cfg.Enabled != nil && *cfg.Enabled {
			out[strings.ToLower(string(group))] = true
		}
	}
	return out
}

func filterSummaryByEnabledSites(sum v2.DeltaSummary, enabled map[string]bool) v2.DeltaSummary {
	if enabled == nil {
		return sum
	}
	out := v2.DeltaSummary{Range: sum.Range, From: sum.From, To: sum.To, Sites: make([]v2.SiteDelta, 0, len(sum.Sites))}
	for _, d := range sum.Sites {
		if !enabled[strings.ToLower(d.Site)] {
			continue
		}
		out.Sites = append(out.Sites, d)
		out.TotalUploaded += d.Uploaded
		out.TotalDownloaded += d.Downloaded
		out.TotalBonus += d.Bonus
	}
	return out
}

// apiDailyReportSettings handles GET/PUT /api/v2/userinfo/daily-report
func (s *Server) apiDailyReportSettings(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.store.DailyReportSettings()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, cfg)
	case http.MethodPut, http.MethodPost:
		var in core.DailyReportSettings
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
			http.Error(w, "请求格式错误: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.store.SaveDailyReportSettings(in); err != nil {
			status := http.StatusBadRequest
			if !isDailyReportValidationError(err) {
				status = http.StatusInternalServerError
			}
			http.Error(w, err.Error(), status)
			return
		}
		cfg, err := s.store.DailyReportSettings()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, cfg)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// isDailyReportValidationError 区分用户输入错误（400）与存储故障（500）。
func isDailyReportValidationError(err error) bool {
	if errors.Is(err, core.ErrDailyReportNoChannel) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "HH:MM") || strings.Contains(msg, "已经不存在") || strings.Contains(msg, "尚未初始化")
}

// maxTrendDays 是 /api/v2/userinfo/trends 一次最多取的天数（走势柱图用不了更多）。
const maxTrendDays = 60

// UserInfoTrendsResponse 是最近若干天每天的增量：各站一条序列，加上各站合计。
type UserInfoTrendsResponse struct {
	Days   int                        `json:"days"`
	From   string                     `json:"from"`
	To     string                     `json:"to"`
	Dates  []string                   `json:"dates"`
	Totals []v2.TrendPoint            `json:"totals"`
	Sites  map[string][]v2.TrendPoint `json:"sites"`
}

// apiUserInfoTrends handles GET /api/v2/userinfo/trends?days=
//
// 站点列表行卡与 KPI 带的柱图用：每站最近 days 天每天的增量，只算已启用的站点。
func (s *Server) apiUserInfoTrends(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	days := 8
	if raw := r.URL.Query().Get("days"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > maxTrendDays {
			http.Error(w, "days 应为 1–60 的整数", http.StatusBadRequest)
			return
		}
		days = v
	}
	repo, ok := userInfoHistoryRepo(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	dates, _, sites, err := v2.LoadTrends(ctx, repo, days)
	if err != nil {
		global.GetSlogger().Errorf("[UserInfo] 计算走势失败: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	enabled := s.enabledSiteSet()
	resp := UserInfoTrendsResponse{Days: days, Dates: dates, Sites: map[string][]v2.TrendPoint{}}
	if len(dates) > 0 {
		resp.From, resp.To = dates[0], dates[len(dates)-1]
	}
	resp.Totals = make([]v2.TrendPoint, len(dates))
	for i, d := range dates {
		resp.Totals[i].Date = d
	}
	for site, series := range sites {
		if enabled != nil && !enabled[strings.ToLower(site)] {
			continue
		}
		resp.Sites[site] = series
		for i, p := range series {
			resp.Totals[i].Uploaded += p.Uploaded
			resp.Totals[i].Downloaded += p.Downloaded
			resp.Totals[i].Bonus += p.Bonus
		}
	}
	writeJSON(w, resp)
}
