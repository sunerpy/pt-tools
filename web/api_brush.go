package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 刷流任务接口：任务的增删改查、立即运行一轮、任务名下的种子与每日收益。全部只认 session（s.auth）。

const (
	brushRunAPITimeout = 6 * time.Minute
	maxBrushStatDays   = 90
)

// BrushTaskRequest 是新建 / 修改刷流任务的请求体；字段含义见 models.BrushTask。
type BrushTaskRequest struct {
	Name                        string  `json:"name"`
	Enabled                     bool    `json:"enabled"`
	SiteName                    string  `json:"site_name"`
	DownloaderID                uint    `json:"downloader_id"`
	SavePath                    string  `json:"save_path"`
	Category                    string  `json:"category"`
	Tags                        string  `json:"tags"`
	IntervalMin                 int     `json:"interval_min"`
	Discounts                   string  `json:"discounts"`
	MinFreeRemainMin            int     `json:"min_free_remain_min"`
	MinSizeGB                   float64 `json:"min_size_gb"`
	MaxSizeGB                   float64 `json:"max_size_gb"`
	MaxSeeders                  int     `json:"max_seeders"`
	MinLeechers                 int     `json:"min_leechers"`
	MaxPublishAgeMin            int     `json:"max_publish_age_min"`
	ExcludeHR                   bool    `json:"exclude_hr"`
	IncludeKeywords             string  `json:"include_keywords"`
	ExcludeKeywords             string  `json:"exclude_keywords"`
	MaxDownloading              int     `json:"max_downloading"`
	MaxTotalSizeGB              float64 `json:"max_total_size_gb"`
	MaxDailyDownloadGB          float64 `json:"max_daily_download_gb"`
	RemoveSeedTimeH             float64 `json:"remove_seed_time_h"`
	RemoveRatio                 float64 `json:"remove_ratio"`
	RemoveLowSpeedKBs           float64 `json:"remove_low_speed_kbs"`
	RemoveLowSpeedWindowMin     int     `json:"remove_low_speed_window_min"`
	RemoveInactiveH             float64 `json:"remove_inactive_h"`
	RemoveFreeExpiredIncomplete bool    `json:"remove_free_expired_incomplete"`
	RemoveWithData              bool    `json:"remove_with_data"`
}

// BrushStatView 是一段时间的收益。
type BrushStatView struct {
	Uploaded   int64 `json:"uploaded"`
	Downloaded int64 `json:"downloaded"`
	Added      int   `json:"added"`
	Removed    int   `json:"removed"`
}

// BrushTaskView 是任务列表 / 详情里的一项：配置加上当前状态与收益。
type BrushTaskView struct {
	models.BrushTask
	DownloaderName   string        `json:"downloader_name"`
	SiteEnabled      bool          `json:"site_enabled"`
	ActiveCount      int           `json:"active_count"`
	DownloadingCount int           `json:"downloading_count"`
	ActiveSizeBytes  int64         `json:"active_size_bytes"`
	Today            BrushStatView `json:"today"`
	Total            BrushStatView `json:"total"`
}

// BrushRunResponse 是「立即运行」的结果；Error 不为空表示这一轮没跑完（站点或下载器出错）。
type BrushRunResponse struct {
	Result scheduler.BrushRunResult `json:"result"`
	Error  string                   `json:"error,omitempty"`
}

// BrushStatsSeriesPoint 是某天的收益。
type BrushStatsSeriesPoint struct {
	Date string `json:"date"`
	BrushStatView
}

// BrushStatsTask 是一个任务最近若干天每天的收益。
type BrushStatsTask struct {
	TaskID uint                    `json:"task_id"`
	Name   string                  `json:"name"`
	Series []BrushStatsSeriesPoint `json:"series"`
}

// BrushStatsResponse 是最近若干天各任务与合计的每日收益（按进程时区分天）。
type BrushStatsResponse struct {
	Days   int                     `json:"days"`
	From   string                  `json:"from"`
	To     string                  `json:"to"`
	Dates  []string                `json:"dates"`
	Tasks  []BrushStatsTask        `json:"tasks"`
	Totals []BrushStatsSeriesPoint `json:"totals"`
}

func (s *Server) registerBrushRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/brush/tasks", s.auth(s.apiBrushTasks))
	mux.HandleFunc("/api/brush/tasks/", s.auth(s.apiBrushTaskDetail))
	mux.HandleFunc("/api/brush/stats", s.auth(s.apiBrushStats))
}

func brushRepo(w http.ResponseWriter) (*models.BrushRepository, bool) {
	if global.GlobalDB == nil {
		writeJSONError(w, "数据库未初始化", http.StatusServiceUnavailable)
		return nil, false
	}
	return models.NewBrushRepository(global.GlobalDB.DB), true
}

func (s *Server) brushMonitor() *scheduler.BrushMonitor {
	if s == nil || s.mgr == nil {
		return nil
	}
	return s.mgr.GetBrushMonitor()
}

// apiBrushTasks handles GET（列表）/ POST（新建）/api/brush/tasks
func (s *Server) apiBrushTasks(w http.ResponseWriter, r *http.Request) {
	repo, ok := brushRepo(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		tasks, err := repo.ListTasks()
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		views, err := brushTaskViews(tasks)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, views)
	case http.MethodPost:
		task, status, err := decodeBrushTask(r, &models.BrushTask{})
		if err != nil {
			writeJSONError(w, err.Error(), status)
			return
		}
		if err := repo.SaveTask(task); err != nil {
			writeJSONError(w, brushSaveError(err), brushSaveStatus(err))
			return
		}
		s.writeBrushTask(w, repo, task.ID, http.StatusCreated)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// apiBrushTaskDetail handles /api/brush/tasks/{id}、/{id}/run、/{id}/torrents
func (s *Server) apiBrushTaskDetail(w http.ResponseWriter, r *http.Request) {
	repo, ok := brushRepo(w)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/brush/tasks/"), "/")
	idPart, action, _ := strings.Cut(rest, "/")
	id64, err := strconv.ParseUint(idPart, 10, 64)
	if err != nil || id64 == 0 {
		writeJSONError(w, "无效的刷流任务 ID", http.StatusBadRequest)
		return
	}
	id := uint(id64)
	switch action {
	case "":
		s.brushTaskCRUD(w, r, repo, id)
	case "run":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.runBrushTask(w, r, repo, id)
	case "torrents":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		listBrushTorrents(w, r, repo, id)
	default:
		writeJSONError(w, "未知的刷流接口", http.StatusNotFound)
	}
}

func (s *Server) brushTaskCRUD(w http.ResponseWriter, r *http.Request, repo *models.BrushRepository, id uint) {
	switch r.Method {
	case http.MethodGet:
		s.writeBrushTask(w, repo, id, http.StatusOK)
	case http.MethodPut:
		existing, err := repo.GetTask(id)
		if err != nil {
			writeJSONError(w, err.Error(), brushLookupStatus(err))
			return
		}
		task, status, err := decodeBrushTask(r, existing)
		if err != nil {
			writeJSONError(w, err.Error(), status)
			return
		}
		if err := repo.SaveTask(task); err != nil {
			writeJSONError(w, brushSaveError(err), brushSaveStatus(err))
			return
		}
		s.writeBrushTask(w, repo, id, http.StatusOK)
	case http.MethodDelete:
		err := s.brushMonitor().WithTaskLock(id, func() error { return repo.DeleteTask(id) })
		switch {
		case errors.Is(err, scheduler.ErrBrushBusy):
			writeJSONError(w, err.Error(), http.StatusConflict)
		case errors.Is(err, models.ErrBrushTaskNotFound):
			writeJSONError(w, err.Error(), http.StatusNotFound)
		case err != nil:
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
		default:
			writeJSON(w, map[string]any{"success": true})
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) writeBrushTask(w http.ResponseWriter, repo *models.BrushRepository, id uint, status int) {
	task, err := repo.GetTask(id)
	if err != nil {
		writeJSONError(w, err.Error(), brushLookupStatus(err))
		return
	}
	views, err := brushTaskViews([]models.BrushTask{*task})
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(views[0])
}

func (s *Server) runBrushTask(w http.ResponseWriter, r *http.Request, repo *models.BrushRepository, id uint) {
	mon := s.brushMonitor()
	if mon == nil {
		writeJSONError(w, "刷流服务未启动", http.StatusServiceUnavailable)
		return
	}
	if _, err := repo.GetTask(id); err != nil {
		writeJSONError(w, err.Error(), brushLookupStatus(err))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), brushRunAPITimeout)
	defer cancel()
	res, err := mon.RunTask(ctx, id)
	switch {
	case errors.Is(err, scheduler.ErrBrushBusy):
		writeJSONError(w, err.Error(), http.StatusConflict)
		return
	case errors.Is(err, models.ErrBrushTaskNotFound):
		writeJSONError(w, err.Error(), http.StatusNotFound)
		return
	}
	resp := BrushRunResponse{Result: res}
	if err != nil {
		resp.Error = err.Error()
	}
	writeJSON(w, resp)
}

func listBrushTorrents(w http.ResponseWriter, r *http.Request, repo *models.BrushRepository, id uint) {
	if _, err := repo.GetTask(id); err != nil {
		writeJSONError(w, err.Error(), brushLookupStatus(err))
		return
	}
	q := r.URL.Query()
	state := q.Get("state")
	switch state {
	case "", models.BrushTorrentActive, models.BrushTorrentRemoved, models.BrushTorrentGone:
	default:
		writeJSONError(w, "state 只能是 active、removed 或 gone", http.StatusBadRequest)
		return
	}
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	rows, total, err := repo.ListTorrents(id, state, page, pageSize)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"items": rows, "total": total, "page": page, "page_size": pageSize})
}

// apiBrushStats handles GET /api/brush/stats?days=
func (s *Server) apiBrushStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	repo, ok := brushRepo(w)
	if !ok {
		return
	}
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > maxBrushStatDays {
			writeJSONError(w, fmt.Sprintf("days 应为 1–%d 的整数", maxBrushStatDays), http.StatusBadRequest)
			return
		}
		days = v
	}
	resp, err := buildBrushStats(repo, time.Now(), days)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func buildBrushStats(repo *models.BrushRepository, now time.Time, days int) (BrushStatsResponse, error) {
	to := now.Format("2006-01-02")
	from := now.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	dates := make([]string, 0, days)
	index := make(map[string]int, days)
	for i := range days {
		d := now.AddDate(0, 0, -(days - 1 - i)).Format("2006-01-02")
		index[d] = i
		dates = append(dates, d)
	}
	tasks, err := repo.ListTasks()
	if err != nil {
		return BrushStatsResponse{}, err
	}
	stats, err := repo.DailyStats(0, from, to)
	if err != nil {
		return BrushStatsResponse{}, err
	}
	blank := func() []BrushStatsSeriesPoint {
		out := make([]BrushStatsSeriesPoint, len(dates))
		for i, d := range dates {
			out[i].Date = d
		}
		return out
	}
	resp := BrushStatsResponse{Days: days, From: from, To: to, Dates: dates, Totals: blank(), Tasks: make([]BrushStatsTask, 0, len(tasks))}
	byTask := make(map[uint]int, len(tasks))
	for _, t := range tasks {
		byTask[t.ID] = len(resp.Tasks)
		resp.Tasks = append(resp.Tasks, BrushStatsTask{TaskID: t.ID, Name: t.Name, Series: blank()})
	}
	for _, st := range stats {
		i, ok := index[st.Day]
		if !ok {
			continue
		}
		add := func(p *BrushStatsSeriesPoint) {
			p.Uploaded += st.Uploaded
			p.Downloaded += st.Downloaded
			p.Added += st.Added
			p.Removed += st.Removed
		}
		add(&resp.Totals[i])
		if ti, ok := byTask[st.TaskID]; ok {
			add(&resp.Tasks[ti].Series[i])
		}
	}
	return resp, nil
}

// brushTaskViews 给任务补上下载器名、站点是否启用、在做的种子数与收益。
func brushTaskViews(tasks []models.BrushTask) ([]BrushTaskView, error) {
	db := global.GlobalDB.DB
	repo := models.NewBrushRepository(db)
	totals, err := repo.TaskTotals()
	if err != nil {
		return nil, err
	}
	today := time.Now().Format("2006-01-02")
	todayStats, err := repo.DailyStats(0, today, today)
	if err != nil {
		return nil, err
	}
	todayBy := make(map[uint]models.BrushDailyStat, len(todayStats))
	for _, st := range todayStats {
		todayBy[st.TaskID] = st
	}
	var dls []models.DownloaderSetting
	if err := db.Select("id", "name").Find(&dls).Error; err != nil {
		return nil, fmt.Errorf("读取下载器失败: %w", err)
	}
	dlNames := make(map[uint]string, len(dls))
	for _, d := range dls {
		dlNames[d.ID] = d.Name
	}
	var sites []models.SiteSetting
	if err := db.Select("name", "enabled").Find(&sites).Error; err != nil {
		return nil, fmt.Errorf("读取站点失败: %w", err)
	}
	siteOn := make(map[string]bool, len(sites))
	for _, st := range sites {
		siteOn[st.Name] = st.Enabled
	}
	out := make([]BrushTaskView, 0, len(tasks))
	for _, t := range tasks {
		v := BrushTaskView{BrushTask: t, DownloaderName: dlNames[t.DownloaderID], SiteEnabled: siteOn[t.SiteName]}
		active, err := repo.ActiveTorrents(t.ID)
		if err != nil {
			return nil, err
		}
		v.ActiveCount = len(active)
		for _, bt := range active {
			v.ActiveSizeBytes += bt.SizeBytes
			if bt.Progress < 1 {
				v.DownloadingCount++
			}
		}
		if st, ok := todayBy[t.ID]; ok {
			v.Today = BrushStatView{Uploaded: st.Uploaded, Downloaded: st.Downloaded, Added: st.Added, Removed: st.Removed}
		}
		if st, ok := totals[t.ID]; ok {
			v.Total = BrushStatView{Uploaded: st.Uploaded, Downloaded: st.Downloaded, Added: st.Added, Removed: st.Removed}
		}
		out = append(out, v)
	}
	return out, nil
}

// decodeBrushTask 解析请求体、校验后写到 into 上（保留 ID 与运行状态列）；返回出错时的 HTTP 状态码。
func decodeBrushTask(r *http.Request, into *models.BrushTask) (*models.BrushTask, int, error) {
	var req BrushTaskRequest
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<10)).Decode(&req); err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("请求格式错误: %w", err)
	}
	if err := validateBrushTask(&req); err != nil {
		return nil, http.StatusBadRequest, err
	}
	t := into
	t.Name, t.Enabled, t.SiteName, t.DownloaderID = req.Name, req.Enabled, req.SiteName, req.DownloaderID
	t.SavePath, t.Category, t.Tags, t.IntervalMin = req.SavePath, req.Category, req.Tags, req.IntervalMin
	t.Discounts, t.MinFreeRemainMin, t.MinSizeGB, t.MaxSizeGB = req.Discounts, req.MinFreeRemainMin, req.MinSizeGB, req.MaxSizeGB
	t.MaxSeeders, t.MinLeechers, t.MaxPublishAgeMin, t.ExcludeHR = req.MaxSeeders, req.MinLeechers, req.MaxPublishAgeMin, req.ExcludeHR
	t.IncludeKeywords, t.ExcludeKeywords = req.IncludeKeywords, req.ExcludeKeywords
	t.MaxDownloading, t.MaxTotalSizeGB, t.MaxDailyDownloadGB = req.MaxDownloading, req.MaxTotalSizeGB, req.MaxDailyDownloadGB
	t.RemoveSeedTimeH, t.RemoveRatio = req.RemoveSeedTimeH, req.RemoveRatio
	t.RemoveLowSpeedKBs, t.RemoveLowSpeedWindowMin, t.RemoveInactiveH = req.RemoveLowSpeedKBs, req.RemoveLowSpeedWindowMin, req.RemoveInactiveH
	t.RemoveFreeExpiredIncomplete, t.RemoveWithData = req.RemoveFreeExpiredIncomplete, req.RemoveWithData
	return t, 0, nil
}

// validateBrushTask 校验并规整请求（去空白、把优惠类型写成大写、窗口缺省 30 分钟）。
func validateBrushTask(req *BrushTaskRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.SiteName = strings.TrimSpace(req.SiteName)
	req.SavePath = strings.TrimSpace(req.SavePath)
	req.Category = strings.TrimSpace(req.Category)
	req.Tags = strings.TrimSpace(req.Tags)
	switch n := utf8.RuneCountInString(req.Name); {
	case n == 0:
		return errors.New("任务名称不能为空")
	case n > 64:
		return errors.New("任务名称不能超过 64 个字")
	}
	if req.SiteName == "" {
		return errors.New("请选择站点")
	}
	if _, ok := v2.GetDefinitionRegistry().Get(req.SiteName); !ok {
		return fmt.Errorf("站点 %s 没有内置定义，刷流只支持内置站点", req.SiteName)
	}
	if err := global.GlobalDB.DB.Select("id").Where("name = ?", req.SiteName).First(&models.SiteSetting{}).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("站点 %s 还没有配置", req.SiteName)
		}
		return fmt.Errorf("读取站点失败: %w", err)
	}
	if req.DownloaderID == 0 {
		return errors.New("请选择下载器")
	}
	if err := global.GlobalDB.DB.Select("id").First(&models.DownloaderSetting{}, req.DownloaderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("选中的下载器已经不存在")
		}
		return fmt.Errorf("读取下载器失败: %w", err)
	}
	if req.IntervalMin < 5 || req.IntervalMin > 1440 {
		return errors.New("检查间隔应为 5–1440 分钟")
	}
	if req.MaxDownloading < 1 || req.MaxDownloading > 50 {
		return errors.New("同时下载数应为 1–50")
	}
	var levels []string
	allowed := map[string]bool{}
	for _, lv := range scheduler.BrushDiscountLevels {
		allowed[string(lv)] = true
	}
	for raw := range strings.SplitSeq(req.Discounts, ",") {
		lv := strings.ToUpper(strings.TrimSpace(raw))
		if lv == "" {
			continue
		}
		if !allowed[lv] {
			return fmt.Errorf("不认识的优惠类型 %s", lv)
		}
		levels = append(levels, lv)
	}
	req.Discounts = strings.Join(levels, ",")
	for name, v := range map[string]float64{
		"最小体积": req.MinSizeGB, "最大体积": req.MaxSizeGB, "任务总体积": req.MaxTotalSizeGB, "每日下载量": req.MaxDailyDownloadGB,
		"做种时长": req.RemoveSeedTimeH, "分享率": req.RemoveRatio, "上传速度阈值": req.RemoveLowSpeedKBs, "无活动时长": req.RemoveInactiveH,
	} {
		if v < 0 {
			return fmt.Errorf("%s不能是负数", name)
		}
	}
	for name, v := range map[string]int{"免费剩余时间": req.MinFreeRemainMin, "做种人数上限": req.MaxSeeders, "下载人数下限": req.MinLeechers, "发布时长上限": req.MaxPublishAgeMin} {
		if v < 0 {
			return fmt.Errorf("%s不能是负数", name)
		}
	}
	if req.MaxSizeGB > 0 && req.MinSizeGB > req.MaxSizeGB {
		return errors.New("最小体积不能大于最大体积")
	}
	if req.RemoveLowSpeedWindowMin == 0 {
		req.RemoveLowSpeedWindowMin = 30
	}
	if req.RemoveLowSpeedKBs > 0 && (req.RemoveLowSpeedWindowMin < 5 || req.RemoveLowSpeedWindowMin > 1440) {
		return errors.New("低速判断的时间窗应为 5–1440 分钟")
	}
	if utf8.RuneCountInString(req.IncludeKeywords) > 1000 || utf8.RuneCountInString(req.ExcludeKeywords) > 1000 {
		return errors.New("关键词不能超过 1000 个字")
	}
	if utf8.RuneCountInString(req.Tags) > 200 || utf8.RuneCountInString(req.Category) > 120 || utf8.RuneCountInString(req.SavePath) > 500 {
		return errors.New("标签、分类或保存路径太长")
	}
	return nil
}

func brushLookupStatus(err error) int {
	if errors.Is(err, models.ErrBrushTaskNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func brushSaveStatus(err error) int {
	if strings.Contains(strings.ToLower(err.Error()), "unique") {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func brushSaveError(err error) string {
	if brushSaveStatus(err) == http.StatusConflict {
		return "已经有同名的刷流任务"
	}
	return err.Error()
}
