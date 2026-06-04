package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/extension"
)

func (s *Server) registerExtensionActionRoutes(mux *http.ServeMux) {
	if global.GlobalDB != nil && global.GlobalDB.DB != nil {
		if err := extension.AutoMigrate(global.GlobalDB.DB); err != nil {
			global.GetSlogger().Warnf("[Extension] 迁移 pending_actions 表失败: %v", err)
		}
	}
	mux.HandleFunc("/api/extension/actions/pending", s.auth(s.apiExtensionActionsPending))
	mux.HandleFunc("/api/extension/actions/", s.auth(s.apiExtensionActionsRouter))
}

func (s *Server) apiExtensionActionsPending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	db, ok := s.requireDB(w)
	if !ok {
		return
	}
	since := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			writeJSONError(w, "since 必须为非负整数 unix timestamp", http.StatusBadRequest)
			return
		}
		since = v
	}
	actions, err := extension.ListPending(db, since)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("查询待办动作失败: %v", err), http.StatusInternalServerError)
		return
	}
	if actions == nil {
		actions = []extension.PendingAction{}
	}
	writeJSON(w, actions)
}

func (s *Server) apiExtensionActionsRouter(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/extension/actions/")
	if rest == "" || rest == "pending" {
		writeJSONError(w, "未知操作", http.StatusNotFound)
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[1] != "ack" {
		writeJSONError(w, "未知操作", http.StatusNotFound)
		return
	}
	idRaw := strings.TrimSpace(parts[0])
	id, err := strconv.ParseUint(idRaw, 10, 64)
	if err != nil || id == 0 {
		writeJSONError(w, "action id 必须为正整数", http.StatusBadRequest)
		return
	}
	s.handleExtensionActionAck(w, r, uint(id))
}

func (s *Server) handleExtensionActionAck(w http.ResponseWriter, r *http.Request, id uint) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	db, ok := s.requireDB(w)
	if !ok {
		return
	}
	var existing extension.PendingAction
	err := db.Where("id = ?", id).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSONError(w, fmt.Sprintf("action %d 不存在", id), http.StatusNotFound)
			return
		}
		writeJSONError(w, fmt.Sprintf("加载 action 失败: %v", err), http.StatusInternalServerError)
		return
	}
	alreadyAcked := existing.AckedAt != nil
	if err := extension.Ack(db, id); err != nil {
		writeJSONError(w, fmt.Sprintf("ack 失败: %v", err), http.StatusInternalServerError)
		return
	}
	status := "acked"
	if alreadyAcked {
		status = "already_acked"
	}
	writeJSON(w, map[string]any{"ok": true, "status": status, "id": id})
}

func (s *Server) requireDB(w http.ResponseWriter) (*gorm.DB, bool) {
	if global.GlobalDB == nil || global.GlobalDB.DB == nil {
		writeJSONError(w, "数据库未初始化", http.StatusServiceUnavailable)
		return nil, false
	}
	return global.GlobalDB.DB, true
}
