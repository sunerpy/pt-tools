package qbitcompat

import (
	"net/http"
)

// qB 的 server_state 字段就是这样拼的。
const (
	fieldAllTimeDL = "alltime_dl" //nolint:misspell // qBittorrent API field name
	fieldAllTimeUL = "alltime_ul" //nolint:misspell // qBittorrent API field name
)

// appVersion 是 GET /api/v2/app/version。
func (s *Server) appVersion(w http.ResponseWriter, _ *http.Request, _ *call) {
	text(w, http.StatusOK, AppVersion)
}

// webapiVersion 是 GET /api/v2/app/webapiVersion。
func (s *Server) webapiVersion(w http.ResponseWriter, _ *http.Request, _ *call) {
	text(w, http.StatusOK, WebAPIVersion)
}

// buildInfo 是 GET /api/v2/app/buildInfo（客户端只看有没有，给 qB 4.6 常见的组合）。
func (s *Server) buildInfo(w http.ResponseWriter, _ *http.Request, _ *call) {
	writeJSON(w, map[string]any{"qt": "6.4.2", "libtorrent": "2.0.9.0", "boost": "1.83.0", "openssl": "3.1.4", "zlib": "1.3", "bitness": 64})
}

// savePath 是绑定下载器的默认保存目录（qB 的 save_path、Transmission 的 download-dir）；读不到时是空的。
func savePath(b *backend) string {
	paths, err := b.dl.GetClientPaths()
	if err != nil {
		return ""
	}
	for _, p := range paths {
		if p != "" {
			return p
		}
	}
	return ""
}

// preferences 是 GET /api/v2/app/preferences：只给客户端常读的几项。保存目录取后端下载器的；
// 排队、做种限制都报成没开，客户端不会因此去调 pt-tools 没实现的接口。
func (s *Server) preferences(w http.ResponseWriter, r *http.Request, _ *call) {
	b, ok := s.withBackend(w, r)
	if !ok {
		return
	}
	writeJSON(w, map[string]any{
		"locale": "zh_CN", "save_path": savePath(b), "temp_path_enabled": false, "temp_path": "",
		"create_subfolder_enabled": true, "start_paused_enabled": !b.setting.AutoStart, "auto_tmm_enabled": false,
		"queueing_enabled": false, "max_active_downloads": -1, "max_active_torrents": -1, "max_active_uploads": -1,
		"max_ratio_enabled": false, "max_ratio": -1, "max_ratio_act": 0, "max_seeding_time_enabled": false,
		"max_seeding_time": -1, "dht": false, "pex": false, "lsd": false, "encryption": 0, "anonymous_mode": false,
		"dl_limit": 0, "up_limit": 0, "web_ui_username": "pt-tools",
	})
}

// defaultSavePath 是 GET /api/v2/app/defaultSavePath。
func (s *Server) defaultSavePath(w http.ResponseWriter, r *http.Request, _ *call) {
	b, ok := s.withBackend(w, r)
	if !ok {
		return
	}
	text(w, http.StatusOK, savePath(b))
}

// transfer 是全局的传输状态（transfer/info 与 maindata 的 server_state）；读不到的项是 0。
func (s *Server) transfer(b *backend) map[string]any {
	out := map[string]any{
		"dl_info_speed": int64(0), "dl_info_data": int64(0), "up_info_speed": int64(0), "up_info_data": int64(0),
		"dl_rate_limit": int64(0), "up_rate_limit": int64(0), "dht_nodes": 0, "connection_status": "connected",
		fieldAllTimeDL: int64(0), fieldAllTimeUL: int64(0),
	}
	if st, err := b.dl.GetClientStatus(); err == nil {
		out["dl_info_speed"], out["up_info_speed"] = st.DlSpeed, st.UpSpeed
		out["dl_info_data"], out["up_info_data"] = st.SessionDlData, st.SessionUpData
		out[fieldAllTimeDL], out[fieldAllTimeUL] = st.DlData, st.UpData
	}
	if lim, err := b.dl.GetSpeedLimit(); err == nil && lim.LimitEnabled {
		out["dl_rate_limit"], out["up_rate_limit"] = lim.DownloadLimit, lim.UploadLimit
	}
	return out
}

// transferInfo 是 GET /api/v2/transfer/info。
func (s *Server) transferInfo(w http.ResponseWriter, r *http.Request, _ *call) {
	b, ok := s.withBackend(w, r)
	if !ok {
		return
	}
	out := s.transfer(b)
	delete(out, fieldAllTimeDL)
	delete(out, fieldAllTimeUL)
	writeJSON(w, out)
}
