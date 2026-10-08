package web

import (
	"context"
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
