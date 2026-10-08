package web

import (
	"errors"
	"net/http"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/qbitcompat"
)

// qB 兼容入口的设置接口（路线图 M13）：只认 session。监听地址是启动参数（--qbit-compat-addr），这里只读。

// SetQbitCompat 接上 qB 兼容入口（cmd 在开了监听时调用），设置页用它显示监听地址。
func (s *Server) SetQbitCompat(qc *qbitcompat.Server) { s.qbitCompat = qc }

// QbitCompatAddr 是 qB 兼容入口的监听地址；没开时是空的。
func (s *Server) QbitCompatAddr() string {
	if s.qbitCompat == nil {
		return ""
	}
	return s.qbitCompat.Addr()
}

func (s *Server) registerQbitCompatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/qbit-compat", s.auth(s.apiQbitCompatGet))
	mux.HandleFunc("PUT /api/qbit-compat", s.auth(s.apiQbitCompatPut))
}

// QbitCompatView 是 qB 兼容入口页看到的设置与状态。
type QbitCompatView struct {
	// ListenAddr 是监听地址；Listening 为假时兼容入口没开（启动时没给 --qbit-compat-addr）
	ListenAddr   string `json:"listen_addr"`
	Listening    bool   `json:"listening"`
	DownloaderID uint   `json:"downloader_id"`
	FullControl  bool   `json:"full_control"`
	// Downloader 是实际用的那台下载器的名字（没指定时是默认下载器）；没有可用的下载器时是空的
	Downloader string `json:"downloader"`
	// CompatTorrents 是经兼容入口加的种子数
	CompatTorrents int64 `json:"compat_torrents"`
}

func (s *Server) qbitCompatView(r *http.Request, row models.QbitCompatSetting) (QbitCompatView, error) {
	db := global.GlobalDB.DB.WithContext(r.Context())
	v := QbitCompatView{DownloaderID: row.DownloaderID, FullControl: row.FullControl}
	v.ListenAddr = s.QbitCompatAddr()
	v.Listening = v.ListenAddr != ""
	var ds models.DownloaderSetting
	q := db.Where("enabled = ?", true)
	if row.DownloaderID != 0 {
		q = q.Where("id = ?", row.DownloaderID)
	} else {
		q = q.Order("is_default DESC, id")
	}
	if err := q.Limit(1).Find(&ds).Error; err != nil {
		return v, err
	}
	v.Downloader = ds.Name
	if err := db.Model(&models.TorrentInfo{}).Where("download_source = ?", qbitcompat.Source).Count(&v.CompatTorrents).Error; err != nil {
		return v, err
	}
	return v, nil
}

func (s *Server) apiQbitCompatGet(w http.ResponseWriter, r *http.Request) {
	if global.GlobalDB == nil {
		http.Error(w, "数据库没有初始化", http.StatusServiceUnavailable)
		return
	}
	row, err := qbitcompat.LoadSettings(r.Context(), global.GlobalDB.DB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v, err := s.qbitCompatView(r, row)
	if err != nil {
		http.Error(w, "读取 qB 兼容入口状态失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, v)
}

func (s *Server) apiQbitCompatPut(w http.ResponseWriter, r *http.Request) {
	if global.GlobalDB == nil {
		http.Error(w, "数据库没有初始化", http.StatusServiceUnavailable)
		return
	}
	var in qbitcompat.Settings
	if !decodeStrictBody(w, r, &in) {
		return
	}
	row, err := qbitcompat.SaveSettings(r.Context(), global.GlobalDB.DB, in)
	if errors.Is(err, qbitcompat.ErrInvalid) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v, err := s.qbitCompatView(r, row)
	if err != nil {
		http.Error(w, "读取 qB 兼容入口状态失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, v)
}
