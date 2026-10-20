package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// rssTaskStatus 是 RSS 任务的状态（GET /api/control/status，POST /api/control/auto-start 也回它）。
// state 的取值见 scheduler.RSSState；started_at 是最早一个在运行的任务的启动时间，没有任务在运行时不给。
type rssTaskStatus struct {
	State         string     `json:"state"`
	Jobs          int        `json:"jobs"`
	Subscriptions int        `json:"subscriptions"`
	AutoStart     bool       `json:"auto_start"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
}

var errRSSTaskUnavailable = errors.New("RSS 任务未就绪")

func (s *Server) currentRSSTaskStatus() (rssTaskStatus, error) {
	if s.mgr == nil || s.store == nil {
		return rssTaskStatus{}, errRSSTaskUnavailable
	}
	gl, err := s.store.GetGlobalOnly()
	if err != nil {
		return rssTaskStatus{}, err
	}
	rss, err := s.store.EnabledRSS()
	if err != nil {
		return rssTaskStatus{}, err
	}
	st := s.mgr.RSSStatus(gl, rss)
	out := rssTaskStatus{State: string(st.State), Jobs: st.Jobs, Subscriptions: st.Subscriptions, AutoStart: st.AutoStart}
	if !st.StartedAt.IsZero() {
		at := st.StartedAt
		out.StartedAt = &at
	}
	return out, nil
}

func (s *Server) writeRSSTaskStatus(w http.ResponseWriter) {
	st, err := s.currentRSSTaskStatus()
	if errors.Is(err, errRSSTaskUnavailable) {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		http.Error(w, "读取 RSS 任务状态失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, st)
}

// apiControlStatus 返回 RSS 任务此刻的真实状态，网页状态栏与「RSS 任务没有运行」的提示按它显示。
func (s *Server) apiControlStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.writeRSSTaskStatus(w)
}

// apiControlAutoStart 只切换「自动启动 RSS 任务」，其余全局设置不动，返回切换后的状态。
// 和在全局设置页保存一样会发布配置变更：任务没在运行、也没被手动停止时，打开它会马上按配置启动。
func (s *Server) apiControlAutoStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.Enabled == nil {
		http.Error(w, `请求体应为 {"enabled": true} 或 {"enabled": false}`, http.StatusBadRequest)
		return
	}
	if s.store == nil {
		http.Error(w, errRSSTaskUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	if err := s.store.SetAutoStart(*req.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeRSSTaskStatus(w)
}
