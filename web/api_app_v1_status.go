package web

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/version"
)

// AppDownloader 是 GET /downloaders 的一项：启用的下载器与它现在的传输状态。不含地址、账号与密码。
type AppDownloader struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default bool   `json:"default"`
	// Reachable 是这一次读到了状态或剩余空间；读不到时 Error 写明原因
	Reachable     bool   `json:"reachable"`
	Error         string `json:"error,omitempty"`
	Version       string `json:"version,omitempty"`
	UploadSpeed   int64  `json:"upload_speed"`
	DownloadSpeed int64  `json:"download_speed"`
	// Uploaded、Downloaded 是下载器自己记的累计量
	Uploaded   int64 `json:"uploaded"`
	Downloaded int64 `json:"downloaded"`
	FreeSpace  int64 `json:"free_space"`
}

// AppDownloaderList 是 GET /downloaders。
type AppDownloaderList struct {
	Items []AppDownloader `json:"items"`
}

// appDownloadersTimeout 是 /downloaders 一共最多等多久（几台下载器同时探测，没回来的记成没有读到）。
const appDownloadersTimeout = 20 * time.Second

func (s *Server) appDownloaders(w http.ResponseWriter, r *http.Request) {
	if global.GlobalDB == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "数据库没有初始化")
		return
	}
	var settings []models.DownloaderSetting
	if err := global.GlobalDB.DB.WithContext(r.Context()).Where("enabled = ?", true).Order("id").Find(&settings).Error; err != nil {
		appError(w, http.StatusInternalServerError, "internal", "读取下载器失败: "+appRedact(err.Error()))
		return
	}
	records := make([]downloaderRecord, 0, len(settings))
	for _, ds := range settings {
		records = append(records, downloaderRecord{ID: ds.ID, Name: ds.Name, Type: ds.Type, URL: ds.URL})
	}
	stats := map[uint]DownloaderTransferStatItem{}
	if dm := s.getDownloaderManager(); dm != nil {
		ctx, cancel := context.WithTimeout(r.Context(), appDownloadersTimeout)
		defer cancel()
		for _, it := range s.collectTransferStats(ctx, dm, records).Downloaders {
			stats[it.DownloaderID] = it
		}
	}
	out := AppDownloaderList{Items: make([]AppDownloader, 0, len(settings))}
	for _, ds := range settings {
		d := AppDownloader{ID: ds.ID, Name: ds.Name, Type: ds.Type, Default: ds.IsDefault}
		if it, ok := stats[ds.ID]; ok {
			d.Reachable, d.Error, d.Version = it.Reachable, appRedactAddr(it.Error), it.ClientVersion
			d.UploadSpeed, d.DownloadSpeed, d.Uploaded, d.Downloaded, d.FreeSpace = it.UploadSpeed, it.DownloadSpeed, it.Uploaded, it.Downloaded, it.FreeSpace
		} else {
			d.Error = "这一次没有读到这台下载器（连不上或超时）"
		}
		out.Items = append(out.Items, d)
	}
	appJSON(w, out)
}

// AppDownloaderSeeding 是 GET /downloaders/seeding 的一项：一台启用的下载器现在正在做种的种子。不含地址、账号与密码。
type AppDownloaderSeeding struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	// Reachable 为假时这一台没读到种子列表，下面的数都是 0，原因在 Error（去掉了地址）
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
	// SeedingCount、SeedingSize 是状态是做种的种子（qBittorrent 的 uploading、stalledUP、forcedUP，Transmission 的 seed）
	SeedingCount int   `json:"seeding_count"`
	SeedingSize  int64 `json:"seeding_size"`
	// IdleCount、IdleSize 是下载完了、没在做种的种子（暂停、停止、排队、校验中、出错）
	IdleCount int   `json:"idle_count"`
	IdleSize  int64 `json:"idle_size"`
}

// AppDownloaderSeedingList 是 GET /downloaders/seeding：各台的做种情况与合计（合计只算读到了的那几台）。
// 体积按种子大小相加，辅种和原种子各算一次，和站点上报做种体积的口径一样。
type AppDownloaderSeedingList struct {
	TotalSeedingCount int                    `json:"total_seeding_count"`
	TotalSeedingSize  int64                  `json:"total_seeding_size"`
	Items             []AppDownloaderSeeding `json:"items"`
}

// appDownloaderSeeding 是 GET /downloaders/seeding：读各台启用的下载器的种子列表，数正在做种的个数与体积
// （网页用户统计页「下载器做种」卡的同一份数）。几台同时读，一共最多等 appDownloadersTimeout。
func (s *Server) appDownloaderSeeding(w http.ResponseWriter, r *http.Request) {
	if global.GlobalDB == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "数据库没有初始化")
		return
	}
	var settings []models.DownloaderSetting
	if err := global.GlobalDB.DB.WithContext(r.Context()).Where("enabled = ?", true).Order("id").Find(&settings).Error; err != nil {
		appError(w, http.StatusInternalServerError, "internal", "读取下载器失败: "+appRedact(err.Error()))
		return
	}
	records := make([]downloaderRecord, 0, len(settings))
	for _, ds := range settings {
		records = append(records, downloaderRecord{ID: ds.ID, Name: ds.Name, Type: ds.Type, URL: ds.URL})
	}
	var stats DownloaderSeedingResponse
	if dm := s.getDownloaderManager(); dm != nil {
		ctx, cancel := context.WithTimeout(r.Context(), appDownloadersTimeout)
		defer cancel()
		stats = collectSeedingStats(ctx, dm, records)
	} else {
		for _, rec := range records {
			stats.Downloaders = append(stats.Downloaders, unreachableSeeding(rec, errors.New("这一次没有读到这台下载器（连不上或超时）")))
		}
	}
	out := AppDownloaderSeedingList{
		TotalSeedingCount: stats.TotalSeedingCount,
		TotalSeedingSize:  stats.TotalSeedingSize,
		Items:             make([]AppDownloaderSeeding, 0, len(stats.Downloaders)),
	}
	for _, it := range stats.Downloaders {
		out.Items = append(out.Items, AppDownloaderSeeding{
			ID: it.DownloaderID, Name: it.DownloaderName, Type: it.DownloaderType,
			Reachable: it.Reachable, Error: appRedactAddr(it.Error),
			SeedingCount: it.SeedingCount, SeedingSize: it.SeedingSize, IdleCount: it.IdleCount, IdleSize: it.IdleSize,
		})
	}
	appJSON(w, out)
}

// AppDownloaderLabels 是 GET /downloaders/{id}/labels：一台启用的下载器现有的分类与标签，App 推送时给用户选。
// 不含地址、账号与密码。
type AppDownloaderLabels struct {
	DownloaderID uint   `json:"downloader_id"`
	Downloader   string `json:"downloader"`
	Type         string `json:"type"`
	// SupportsCategory 为假时这台下载器没有分类（Transmission 只有 labels：推送时给的分类会写成第一个 label），Categories 是空的
	SupportsCategory bool `json:"supports_category"`
	// Categories 是 qBittorrent 里建好的分类加上种子上用到的分类
	Categories []string `json:"categories"`
	// Tags 是种子上用到的标签（Transmission 是 labels）
	Tags []string `json:"tags"`
}

// appDownloaderLabelsTimeout 是 /downloaders/{id}/labels 最多等多久（要读整个种子列表）。
const appDownloaderLabelsTimeout = 15 * time.Second

// appDownloaderLabels 是 GET /downloaders/{id}/labels：读这台下载器的种子列表，汇总上面的分类与标签
// （和网页的 /api/downloader-torrents/meta 同一个口径，只看一台）。到了时限还没读完时回 504，不挂住。
func (s *Server) appDownloaderLabels(w http.ResponseWriter, r *http.Request) {
	id, ok := appID(w, r)
	if !ok {
		return
	}
	if global.GlobalDB == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "数据库没有初始化")
		return
	}
	var ds models.DownloaderSetting
	if err := global.GlobalDB.DB.WithContext(r.Context()).Where("id = ? AND enabled = ?", id, true).Limit(1).Find(&ds).Error; err != nil {
		appError(w, http.StatusInternalServerError, "internal", "读取下载器失败: "+appRedact(err.Error()))
		return
	}
	if ds.ID == 0 {
		appError(w, http.StatusNotFound, "not_found", "下载器不存在或没有启用")
		return
	}
	dm := s.getDownloaderManager()
	if dm == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "下载器管理器没有初始化")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), appDownloaderLabelsTimeout)
	defer cancel()
	type result struct {
		labels AppDownloaderLabels
		err    error
	}
	// 种子列表的请求不带 ctx：放在后台读，到了时限就不等它（它有自己的超时）
	done := make(chan result, 1)
	go func() {
		labels, err := downloaderLabelsOf(ctx, dm, ds)
		done <- result{labels, err}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			appError(w, http.StatusBadGateway, "upstream", "读取下载器 "+ds.Name+" 的分类与标签失败: "+appRedactAddr(res.err.Error()))
			return
		}
		appJSON(w, res.labels)
	case <-ctx.Done():
		appError(w, http.StatusGatewayTimeout, "timeout", "没有在时限内读到下载器 "+ds.Name+" 的种子列表")
	}
}

// downloaderLabelsOf 读一台下载器的种子列表，汇总分类与标签（去掉空的与重复的，排好序）。
// 只有 qBittorrent 有分类：它建好、没有种子用的分类也列出（读不到时只用种子上的）；Transmission 的 labels 都算标签。
func downloaderLabelsOf(ctx context.Context, dm *downloader.DownloaderManager, ds models.DownloaderSetting) (AppDownloaderLabels, error) {
	out := AppDownloaderLabels{
		DownloaderID: ds.ID, Downloader: ds.Name, Type: ds.Type,
		SupportsCategory: ds.Type == string(downloader.DownloaderQBittorrent),
	}
	dl, err := acquireDownloader(ctx, dm, ds.Name)
	if err != nil {
		return out, err
	}
	torrents, err := dl.GetAllTorrents()
	if err != nil {
		return out, err
	}
	categories, tags := map[string]struct{}{}, map[string]struct{}{}
	if out.SupportsCategory {
		if names, err := dl.GetClientLabels(); err != nil {
			global.GetSlogger().Warnf("[App API] 读取下载器 %s 建好的分类失败，只列种子上用到的: %v", ds.Name, err)
		} else {
			for _, name := range names {
				if strings.TrimSpace(name) != "" {
					categories[name] = struct{}{}
				}
			}
		}
	}
	for _, t := range torrents {
		if out.SupportsCategory && strings.TrimSpace(t.Category) != "" {
			categories[t.Category] = struct{}{}
		}
		for _, tag := range strings.Split(t.Tags, ",") {
			if tag = strings.TrimSpace(tag); tag != "" {
				tags[tag] = struct{}{}
			}
		}
	}
	out.Categories, out.Tags = sortedNames(categories), sortedNames(tags)
	return out, nil
}

// sortedNames 是 set 里的名字，排好序；一个也没有时是空数组（JSON 里是 []，不是 null）。
func sortedNames(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// appUpdates 是 GET /updates：有没有新版本（include_prerelease=1 时也报预览版）。只读，不会升级。
func (s *Server) appUpdates(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	res, err := s.checkUpdates(ctx, version.CheckOptions{IncludePrerelease: r.URL.Query().Get("include_prerelease") == "1"})
	// 查不了时 Checker 也给一个带 error 字段的结果：那不是检查的结论，一律回 502
	if err != nil {
		appError(w, http.StatusBadGateway, "upstream", "检查更新失败: "+appRedact(err.Error()))
		return
	}
	appJSON(w, res)
}
