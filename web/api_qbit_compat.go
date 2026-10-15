package web

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/qbitcompat"
)

// qB 兼容入口的设置接口（路线图 M13）：只认 session。监听地址可以在这里设（存进数据库，改了马上换监听）；
// 启动参数 --qbit-compat-addr 或环境变量 PT_QBIT_COMPAT_ADDR 给了地址时用那个，这里改了不管用。

// SetQbitCompat 接上 qB 兼容入口（cmd 在开始接请求以前调用），设置页用它显示登录情况。
func (s *Server) SetQbitCompat(qc *qbitcompat.Server) { s.qbitCompat = qc }

// QbitCompatControl 管 qB 兼容入口的监听（cmd 里实现）：网页上改了监听地址时换监听。
type QbitCompatControl interface {
	// Runtime 是现在的监听情况
	Runtime() QbitCompatRuntime
	// Apply 按网页上设的地址重新监听（空串是关掉）；地址由启动参数或环境变量指定时不动。返回监听失败的原因
	Apply(addr string) error
}

// QbitCompatRuntime 是兼容入口的监听情况。
type QbitCompatRuntime struct {
	// Addr 是正在监听的地址；没在监听时为空
	Addr string
	// Want 是要监听的地址（启动参数、环境变量或网页上设的）；都没设时为空
	Want string
	// Source 是 Want 从哪来：flag、env、settings；都没设时为空
	Source string
	// Error 是 Want 监听失败的原因
	Error string
}

// SetQbitCompatControl 接上监听的管理（cmd 在开始接请求以前调用）。
func (s *Server) SetQbitCompatControl(c QbitCompatControl) { s.qbitCompatCtl = c }

// QbitCompatAddr 是 qB 兼容入口正在监听的地址；没开时是空的。
func (s *Server) QbitCompatAddr() string {
	if s.qbitCompatCtl != nil {
		return s.qbitCompatCtl.Runtime().Addr
	}
	if s.qbitCompat == nil {
		return ""
	}
	return s.qbitCompat.Addr()
}

func (s *Server) registerQbitCompatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/qbit-compat", s.auth(s.apiQbitCompatGet))
	mux.HandleFunc("PUT /api/qbit-compat", s.auth(s.apiQbitCompatPut))
}

// QbitCompatLogin 是一次登录：时间、客户端的地址、客户端填的用户名、用的令牌（成功时）与结果
// （ok、bad_credentials 令牌不对、scope 令牌没有 qB 兼容权限、locked 失败太多次被锁了）。
type QbitCompatLogin struct {
	At       time.Time `json:"at"`
	IP       string    `json:"ip"`
	Username string    `json:"username,omitempty"`
	Token    string    `json:"token,omitempty"`
	Result   string    `json:"result"`
}

// QbitCompatView 是 qB 兼容入口页看到的设置与状态。
type QbitCompatView struct {
	// ListenAddr 是正在监听的地址；Listening 为假时兼容入口没在监听
	ListenAddr string `json:"listen_addr"`
	Listening  bool   `json:"listening"`
	// ListenSource 是要监听的地址从哪来：flag（启动参数）、env（环境变量）、settings（这里设的）；都没设时为空
	ListenSource string `json:"listen_source"`
	// ConfiguredAddr 是要监听的地址；ListenError 是它监听失败的原因（例如端口被占了）
	ConfiguredAddr string `json:"configured_addr"`
	ListenError    string `json:"listen_error,omitempty"`
	// SettingsAddr 是这里存的监听地址（ListenSource 是 flag、env 时不管用）
	SettingsAddr string `json:"settings_addr"`
	DownloaderID uint   `json:"downloader_id"`
	FullControl  bool   `json:"full_control"`
	// Downloader 是实际用的那台下载器的名字（没指定时是默认下载器）；没有可用的下载器时是空的
	Downloader string `json:"downloader"`
	// DownloaderIssue 说明选的下载器为什么用不了（停用了、删掉了、没有默认下载器）；用得了时为空
	DownloaderIssue string `json:"downloader_issue,omitempty"`
	// CompatTorrents 是经兼容入口加进实际用的那台下载器、还没经它删掉的种子数
	CompatTorrents int64 `json:"compat_torrents"`
	// Sessions 是登录着的客户端会话数；LastLogin、LastFailure 是最近一次成功、失败的登录（重启以后没有）
	Sessions    int              `json:"sessions"`
	LastLogin   *QbitCompatLogin `json:"last_login,omitempty"`
	LastFailure *QbitCompatLogin `json:"last_failure,omitempty"`
}

func (s *Server) qbitCompatView(r *http.Request, row models.QbitCompatSetting) (QbitCompatView, error) {
	db := global.GlobalDB.DB.WithContext(r.Context())
	v := QbitCompatView{DownloaderID: row.DownloaderID, FullControl: row.FullControl, SettingsAddr: row.ListenAddr}
	if s.qbitCompatCtl != nil {
		rt := s.qbitCompatCtl.Runtime()
		v.ListenAddr, v.ListenSource, v.ConfiguredAddr, v.ListenError = rt.Addr, rt.Source, rt.Want, rt.Error
	} else {
		v.ListenAddr = s.QbitCompatAddr()
	}
	v.Listening = v.ListenAddr != ""
	var ds models.DownloaderSetting
	if row.DownloaderID != 0 {
		if err := db.Where("id = ?", row.DownloaderID).Limit(1).Find(&ds).Error; err != nil {
			return v, err
		}
		switch {
		case ds.ID == 0:
			v.DownloaderIssue = "选的下载器已经删掉了：重新选一台"
		case !ds.Enabled:
			v.DownloaderIssue = "选的下载器「" + ds.Name + "」停用了：启用它，或者换一台"
			ds = models.DownloaderSetting{}
		}
	} else {
		if err := db.Where("enabled = ?", true).Order("is_default DESC, id").Limit(1).Find(&ds).Error; err != nil {
			return v, err
		}
		if ds.ID == 0 {
			v.DownloaderIssue = "没有启用的下载器：先在「下载器」里添加或启用一台"
		}
	}
	v.Downloader = ds.Name
	if ds.ID != 0 {
		if err := db.Model(&models.QbitCompatTorrent{}).Where("downloader_id = ?", ds.ID).Count(&v.CompatTorrents).Error; err != nil {
			return v, err
		}
	}
	if s.qbitCompat != nil {
		v.Sessions = s.qbitCompat.Sessions()
		ok, fail := s.qbitCompat.Logins()
		v.LastLogin, v.LastFailure = s.loginView(r, ok), s.loginView(r, fail)
	}
	return v, nil
}

// loginView 把登录记录换成页面上看的样子：令牌编号换成名字（令牌删了时写编号）。
func (s *Server) loginView(r *http.Request, rec *qbitcompat.LoginRecord) *QbitCompatLogin {
	if rec == nil {
		return nil
	}
	out := &QbitCompatLogin{At: rec.At, IP: rec.IP, Username: rec.Username, Result: rec.Result}
	if rec.TokenID != 0 {
		out.Token = fmt.Sprintf("#%d", rec.TokenID)
		if s.tokens != nil {
			if t, err := s.tokens.Get(r.Context(), rec.TokenID); err == nil && t.Name != "" {
				out.Token = t.Name
			}
		}
	}
	return out
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

// apiQbitCompatPut 存设置；带了监听地址时马上按它换监听（启动参数或环境变量给了地址时不换）。监听失败不算保存失败：
// 设置已经存下，回应里的 listen_error 写明原因。
func (s *Server) apiQbitCompatPut(w http.ResponseWriter, r *http.Request) {
	if global.GlobalDB == nil {
		http.Error(w, "数据库没有初始化", http.StatusServiceUnavailable)
		return
	}
	var in qbitcompat.Settings
	if !decodeStrictBody(w, r, &in) {
		return
	}
	// 存设置与换监听一次只做一个：两个页面同时保存时，存下的地址与实际监听的地址是同一次的
	s.qbitCompatMu.Lock()
	row, err := qbitcompat.SaveSettings(r.Context(), global.GlobalDB.DB, in)
	if err == nil && in.ListenAddr != nil && s.qbitCompatCtl != nil {
		if aerr := s.qbitCompatCtl.Apply(row.ListenAddr); aerr != nil {
			global.GetSlogger().Warnf("qB 兼容入口按新设置监听 %s 失败: %v", row.ListenAddr, aerr)
		}
	}
	s.qbitCompatMu.Unlock()
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
