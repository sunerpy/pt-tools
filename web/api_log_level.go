package web

import (
	"encoding/json"
	"net/http"

	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/global"
)

// LogLevelRequest 日志级别请求
type LogLevelRequest struct {
	Level string `json:"level"` // debug, info, warn, error
}

// LogLevelResponse 日志级别响应
type LogLevelResponse struct {
	Level   string   `json:"level"`
	Levels  []string `json:"levels"` // 可用的日志级别列表
	Message string   `json:"message,omitempty"`
}

// apiLogLevel 处理日志级别 API
// GET /api/log-level - 获取当前日志级别
// PUT /api/log-level - 设置日志级别
func (s *Server) apiLogLevel(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getLogLevel(w, r)
	case http.MethodPut:
		s.setLogLevel(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// getLogLevel 获取当前日志级别
func (s *Server) getLogLevel(w http.ResponseWriter, r *http.Request) {
	currentLevel := global.GetLogLevel()
	writeJSON(w, LogLevelResponse{
		Level:  string(currentLevel),
		Levels: []string{"debug", "info", "warn", "error"},
	})
}

// setLogLevel 设置日志级别
func (s *Server) setLogLevel(w http.ResponseWriter, r *http.Request) {
	var req LogLevelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "无效的请求格式", http.StatusBadRequest)
		return
	}

	level, valid := global.ParseLogLevel(req.Level)
	if !valid {
		http.Error(w, "无效的日志级别，可选值: debug, info, warn, error", http.StatusBadRequest)
		return
	}

	// 同时更新两处日志级别设置
	global.SetLogLevel(level)
	if err := config.SetLogLevel(req.Level); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[LogLevel] 日志级别已更改为: %s", level)

	writeJSON(w, LogLevelResponse{
		Level:   string(level),
		Levels:  []string{"debug", "info", "warn", "error"},
		Message: "日志级别已更新",
	})
}
