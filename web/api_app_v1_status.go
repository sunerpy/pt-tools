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
		for _, it := range s.collectTransferStats(r.Context(), dm, records).Downloaders {
			stats[it.DownloaderID] = it
		}
	}
	out := AppDownloaderList{Items: make([]AppDownloader, 0, len(settings))}
	for _, ds := range settings {
		d := AppDownloader{ID: ds.ID, Name: ds.Name, Type: ds.Type, Default: ds.IsDefault}
		if it, ok := stats[ds.ID]; ok {
			d.Reachable, d.Error, d.Version = it.Reachable, appRedact(it.Error), it.ClientVersion
			d.UploadSpeed, d.DownloadSpeed, d.Uploaded, d.Downloaded, d.FreeSpace = it.UploadSpeed, it.DownloadSpeed, it.Uploaded, it.Downloaded, it.FreeSpace
		} else {
			d.Error = "连不上这台下载器"
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
	if err != nil && res == nil {
		appError(w, http.StatusBadGateway, "upstream", "检查更新失败: "+appRedact(err.Error()))
		return
	}
	appJSON(w, res)
}
