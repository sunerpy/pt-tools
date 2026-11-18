package servarr

import (
	"context"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
)

// 两个前缀都有的接口：系统状态、质量档案、根目录、标签、命令、下载队列。

// systemStatus 是 GET /system/status。Seerr 只读 version（Sonarr 的主版本决定要不要语言档案）与 urlBase（测试连接以后填进 Base URL）。
func (s *Server) systemStatus(w http.ResponseWriter, _ *http.Request, c *call) {
	writeJSON(w, http.StatusOK, map[string]any{
		"appName": c.arr.name, "instanceName": "pt-tools", "version": c.arr.version, "buildTime": timeText(time.Time{}),
		"isDebug": false, "isProduction": true, "isAdmin": false, "isUserInteractive": false,
		"startupPath": "", "appData": "", "osName": runtime.GOOS, "osVersion": "",
		"isNetCore": false, "isMono": false, "isLinux": runtime.GOOS == "linux", "isOsx": runtime.GOOS == "darwin",
		"isWindows": runtime.GOOS == "windows", "isDocker": false, "mode": "console", "branch": "main",
		"authentication": "external", "sqliteVersion": "", "migrationVersion": 0, "urlBase": c.arr.base,
		"runtimeVersion": runtime.Version(), "runtimeName": "go", "startTime": timeText(s.started),
		"packageUpdateMechanism": "external",
	})
}

// qualityProfile 是 Servarr 的质量档案（Seerr 只读编号与名字）。
type qualityProfile struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// qualityProfiles 是 GET /qualityProfile：「默认档案」（订阅设置里的默认档案）在最前，然后是 pt-tools 的质量档案。
func (s *Server) qualityProfiles(w http.ResponseWriter, r *http.Request, _ *call) {
	list, err := s.deps.Subs.Profiles(r.Context())
	if err != nil {
		writeSubError(w, err, "质量档案")
		return
	}
	out := []qualityProfile{{ID: defaultProfileID, Name: "默认档案"}}
	for _, p := range list {
		out = append(out, qualityProfile{ID: p.ID, Name: p.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

// servarrProfile 是订阅的档案在 Servarr 里的编号：0（默认档案）写 defaultProfileID。
func servarrProfile(id uint) uint {
	if id == 0 {
		return defaultProfileID
	}
	return id
}

// ptProfile 把 Servarr 的档案编号换成订阅的：defaultProfileID 与 0 是默认档案；别的要是存在的档案。
func (s *Server) ptProfile(ctx context.Context, id uint) (uint, error) {
	if id == defaultProfileID || id == 0 {
		return 0, nil
	}
	list, err := s.deps.Subs.Profiles(ctx)
	if err != nil {
		return 0, err
	}
	if !slices.ContainsFunc(list, func(p subscribe.Profile) bool { return p.ID == id }) {
		return 0, errBadRequest("质量档案不存在（编号 " + strconv.FormatUint(uint64(id), 10) + "），在 Seerr 里重新测试连接、选一个档案")
	}
	return id, nil
}

// rootFolder 是 Servarr 的根目录。
type rootFolder struct {
	ID              uint     `json:"id"`
	Path            string   `json:"path"`
	Accessible      bool     `json:"accessible"`
	FreeSpace       uint64   `json:"freeSpace"`
	TotalSpace      uint64   `json:"totalSpace"`
	UnmappedFolders []string `json:"unmappedFolders"`
}

// libraries 是这种应用能用的媒体库（启用的、类型相同的，按编号）。
func (s *Server) libraries(ctx context.Context, kind string) ([]models.MediaLibrary, error) {
	if s.deps.Libraries == nil {
		return nil, nil
	}
	all, err := s.deps.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(l models.MediaLibrary) bool { return !l.Enabled || l.Kind != kind }), nil
}

// rootFolders 是 GET /rootfolder：auto（按类型自动选库）在最前，然后是这种类型的媒体库。空间读不到时写 0。
func (s *Server) rootFolders(w http.ResponseWriter, r *http.Request, c *call) {
	libs, err := s.libraries(r.Context(), c.arr.kind)
	if err != nil {
		writeSubError(w, err, "媒体库")
		return
	}
	out := []rootFolder{{ID: autoRootID, Path: autoRootPath, Accessible: true, UnmappedFolders: []string{}}}
	for i, l := range libs {
		f := rootFolder{ID: l.ID, Path: l.Path, UnmappedFolders: []string{}}
		if u, uerr := s.deps.DiskUsage(l.Path); uerr == nil {
			f.Accessible, f.FreeSpace, f.TotalSpace = true, u.Free, u.Total
		}
		// auto 多半选的是这种类型的第一个库：空间写它的
		if i == 0 {
			out[0].FreeSpace, out[0].TotalSpace = f.FreeSpace, f.TotalSpace
		}
		out = append(out, f)
	}
	writeJSON(w, http.StatusOK, out)
}

// libraryOf 把 Servarr 的根目录换成媒体库编号：auto 与空串是按类型自动选（0）；别的要是这种类型的、启用的媒体库的目录。
func (s *Server) libraryOf(ctx context.Context, kind, path string) (uint, error) {
	p := strings.TrimSpace(path)
	if p == "" || p == autoRootPath {
		return 0, nil
	}
	libs, err := s.libraries(ctx, kind)
	if err != nil {
		return 0, err
	}
	trim := func(v string) string { return strings.TrimRight(v, `/\`) }
	for _, l := range libs {
		if l.Path == p || trim(l.Path) == trim(p) {
			return l.ID, nil
		}
	}
	return 0, errBadRequest("根目录「" + p + "」不是 pt-tools 里启用的媒体库，在 Seerr 里重新测试连接、选一个根目录")
}

// rootPathOf 是订阅的媒体库在 Servarr 里的根目录：0 或库不在了写 auto。
func rootPathOf(libs []models.MediaLibrary, id uint) string {
	if id == 0 {
		return autoRootPath
	}
	for _, l := range libs {
		if l.ID == id {
			return l.Path
		}
	}
	return autoRootPath
}

// tag 是 Servarr 的标签。
type tag struct {
	ID    uint   `json:"id"`
	Label string `json:"label"`
}

// listTags 是 GET /tag。
func (s *Server) listTags(w http.ResponseWriter, r *http.Request, _ *call) {
	var rows []models.ServarrTag
	if err := s.deps.DB.WithContext(r.Context()).Order("id").Find(&rows).Error; err != nil {
		writeSubError(w, err, "标签")
		return
	}
	out := make([]tag, 0, len(rows))
	for _, t := range rows {
		out = append(out, tag{ID: t.ID, Label: t.Label})
	}
	writeJSON(w, http.StatusOK, out)
}

// cleanLabel 检查标签名：不能是空的，最长 128 个字。
func cleanLabel(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" || utf8.RuneCountInString(v) > 128 {
		return "", errBadRequest("标签名不能是空的，最长 128 个字")
	}
	return v, nil
}

// createTag 是 POST /tag：同名的回已有的那个。
func (s *Server) createTag(w http.ResponseWriter, r *http.Request, c *call) {
	var in tag
	if !decode(w, r, &in) {
		return
	}
	label, err := cleanLabel(in.Label)
	if err != nil {
		writeSubError(w, err, "标签")
		return
	}
	c.args["label"] = label
	row := models.ServarrTag{Label: label, CreatedAt: s.deps.Now()}
	db := s.deps.DB.WithContext(r.Context())
	if err = db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err == nil && row.ID == 0 {
		err = db.Where("label = ?", label).First(&row).Error
	}
	if err != nil {
		writeSubError(w, err, "标签")
		return
	}
	c.args["tag_id"] = row.ID
	writeJSON(w, http.StatusCreated, tag{ID: row.ID, Label: row.Label})
}

// renameTag 是 PUT /tag/{id}：没有这个编号回 404，和别的标签重名回 400。
func (s *Server) renameTag(w http.ResponseWriter, r *http.Request, c *call) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in tag
	if !decode(w, r, &in) {
		return
	}
	label, err := cleanLabel(in.Label)
	if err != nil {
		writeSubError(w, err, "标签")
		return
	}
	c.args["tag_id"], c.args["label"] = id, label
	db := s.deps.DB.WithContext(r.Context())
	var row models.ServarrTag
	if err = db.Where("id = ?", id).Limit(1).Find(&row).Error; err != nil {
		writeSubError(w, err, "标签")
		return
	}
	if row.ID == 0 {
		writeError(w, http.StatusNotFound, "没有这个标签")
		return
	}
	var dup int64
	if err = db.Model(&models.ServarrTag{}).Where("label = ? AND id <> ?", label, id).Count(&dup).Error; err != nil {
		writeSubError(w, err, "标签")
		return
	}
	if dup > 0 {
		writeError(w, http.StatusBadRequest, "已经有叫「"+label+"」的标签")
		return
	}
	if err = db.Model(&row).Update("label", label).Error; err != nil {
		writeSubError(w, err, "标签")
		return
	}
	writeJSON(w, http.StatusOK, tag{ID: row.ID, Label: label})
}

// tagsOf 是条目的标签编号（按编号排，没有时是空数组）。kind 是 movie 或 series，external 是 TMDB 编号。
func (s *Server) tagsOf(ctx context.Context, kind string, external int) []uint {
	var ids []uint
	if err := s.deps.DB.WithContext(ctx).Model(&models.ServarrTagLink{}).Where("kind = ? AND external_id = ?", kind, external).
		Order("tag_id").Pluck("tag_id", &ids).Error; err != nil {
		global.GetSlogger().Warnf("[Servarr] 读取标签失败: %v", err)
	}
	if ids == nil {
		ids = []uint{}
	}
	return ids
}

// setTags 把条目的标签换成 ids（不存在的标签编号忽略），返回存下的编号。
func (s *Server) setTags(ctx context.Context, kind string, external int, ids []uint) ([]uint, error) {
	var known []uint
	err := s.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(ids) > 0 {
			if err := tx.Model(&models.ServarrTag{}).Where("id IN ?", ids).Order("id").Pluck("id", &known).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("kind = ? AND external_id = ?", kind, external).Delete(&models.ServarrTagLink{}).Error; err != nil {
			return err
		}
		for _, id := range known {
			if err := tx.Create(&models.ServarrTagLink{Kind: kind, ExternalID: uint(external), TagID: id}).Error; err != nil { //nolint:gosec // TMDB 编号是正数
				return err
			}
		}
		return nil
	})
	if known == nil {
		known = []uint{}
	}
	return known, err
}

// dropTags 删掉条目的标签（条目删除时）。
func (s *Server) dropTags(ctx context.Context, kind string, external int) {
	if err := s.deps.DB.WithContext(ctx).Where("kind = ? AND external_id = ?", kind, external).Delete(&models.ServarrTagLink{}).Error; err != nil {
		global.GetSlogger().Warnf("[Servarr] 删除标签失败: %v", err)
	}
}

// commandInput 是 POST /command 的请求体。
type commandInput struct {
	Name     string `json:"name"`
	MovieIDs []int  `json:"movieIds"`
	SeriesID int    `json:"seriesId"`
}

// command 是 POST /command。Radarr 认 MoviesSearch（让这些电影订阅马上搜），Sonarr 认 MissingEpisodeSearch（让这部剧在找的季订阅
// 马上搜），两个都认 RefreshMonitoredDownloads（什么都不做，Seerr 每分钟发一次，不记审计）；别的名字回 400。
func (s *Server) command(w http.ResponseWriter, r *http.Request, c *call) {
	var in commandInput
	if !decode(w, r, &in) {
		return
	}
	c.args["command"] = in.Name
	switch {
	case in.Name == "RefreshMonitoredDownloads":
		c.skipAudit = true
	case in.Name == "MoviesSearch" && c.arr.kind == models.MediaKindMovie:
		if !s.searchMovies(w, r, c, in.MovieIDs) {
			return
		}
	case in.Name == "MissingEpisodeSearch" && c.arr.kind == models.MediaKindTV:
		if !s.searchSeries(w, r, c, in.SeriesID) {
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "不支持的命令「"+in.Name+"」："+c.arr.name+" 兼容入口只认 "+supportedCommands(c.arr))
		return
	}
	now := s.deps.Now()
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": s.cmdID.Add(1), "name": in.Name, "commandName": in.Name, "status": "queued",
		"queued": timeText(now), "trigger": "manual",
	})
}

func supportedCommands(a arr) string {
	if a.kind == models.MediaKindMovie {
		return "MoviesSearch 与 RefreshMonitoredDownloads"
	}
	return "MissingEpisodeSearch 与 RefreshMonitoredDownloads"
}

// searchMovies 让这些电影订阅马上搜（没在找的跳过）；一个都不是电影订阅时回 404。
func (s *Server) searchMovies(w http.ResponseWriter, r *http.Request, c *call, ids []int) bool {
	var searched, skipped []int
	found := 0
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		row, err := s.deps.Subs.SubscriptionRow(r.Context(), uint(id)) //nolint:gosec // 上面检查过是正数
		if err != nil || row.MediaType != models.MediaKindMovie {
			continue
		}
		found++
		if err = s.deps.Subs.SearchSoon(r.Context(), row.ID); err != nil {
			skipped = append(skipped, id)
			continue
		}
		searched = append(searched, id)
	}
	c.args["searched"], c.args["skipped"] = searched, skipped
	if found == 0 {
		writeError(w, http.StatusNotFound, "没有这些电影订阅")
		return false
	}
	return true
}

// searchSeries 让这部剧在找的季订阅马上搜；没有这部剧的订阅时回 404。
func (s *Server) searchSeries(w http.ResponseWriter, r *http.Request, c *call, tmdbID int) bool {
	c.args["tmdb_id"] = tmdbID
	if tmdbID <= 0 {
		writeError(w, http.StatusBadRequest, "要指定 seriesId")
		return false
	}
	subs, err := s.deps.Subs.SubscriptionsOf(r.Context(), models.MediaKindTV, tmdbID)
	if err != nil {
		writeSubError(w, err, "追剧")
		return false
	}
	if len(subs) == 0 {
		writeError(w, http.StatusNotFound, "没有这部剧的追剧")
		return false
	}
	var searched []int
	for _, sub := range subs {
		if sub.Status == models.MediaSubActive && s.deps.Subs.SearchSoon(r.Context(), sub.ID) == nil {
			searched = append(searched, sub.Season)
		}
	}
	c.args["searched_seasons"] = searched
	return true
}

// queue 是 GET /queue：不报下载进度，回空的一页（Seerr 里就不显示进度）。
func (s *Server) queue(w http.ResponseWriter, _ *http.Request, _ *call) {
	writeJSON(w, http.StatusOK, map[string]any{
		"page": 1, "pageSize": 0, "sortKey": "timeleft", "sortDirection": "ascending", "totalRecords": 0, "records": []any{},
	})
}

// languageProfiles 是 Sonarr 的 GET /languageprofile（版本报 4 时 Seerr 不调，留着防旧版）。
func (s *Server) languageProfiles(w http.ResponseWriter, _ *http.Request, _ *call) {
	writeJSON(w, http.StatusOK, []map[string]any{{"id": 1, "name": "Any"}})
}
