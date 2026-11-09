package web

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
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
