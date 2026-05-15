package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/app"
)

type chatopsHandlers struct {
	deps *ChatOpsDeps
}

type errResp struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

func writeChatopsErr(w http.ResponseWriter, status int, errKey, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errResp{Error: errKey, Detail: detail})
}

func parseUintPathValue(r *http.Request, key string) (uint, error) {
	raw := r.PathValue(key)
	if raw == "" {
		return 0, errors.New("missing path value: " + key)
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(n), nil
}

func decodeJSONBody(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

func mapServiceErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrConfNotFound) || errors.Is(err, gorm.ErrRecordNotFound):
		writeChatopsErr(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, app.ErrInvalidReplyLang),
		errors.Is(err, app.ErrTooManyActiveCodes),
		errors.Is(err, app.ErrCodeUsedOrExpired):
		writeChatopsErr(w, http.StatusBadRequest, "invalid_argument", err.Error())
	default:
		writeChatopsErr(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}

// ----- notifications -----

func (h *chatopsHandlers) listNotifications(w http.ResponseWriter, r *http.Request) {
	items, err := h.deps.NotificationSvc.ListConfs(r.Context())
	if err != nil {
		mapServiceErr(w, err)
		return
	}
	if items == nil {
		items = []app.NotificationConfDTO{}
	}
	writeJSON(w, items)
}

func (h *chatopsHandlers) getNotification(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintPathValue(r, "id")
	if err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	dto, err := h.deps.NotificationSvc.GetConf(r.Context(), id)
	if err != nil {
		mapServiceErr(w, err)
		return
	}
	writeJSON(w, dto)
}

type createNotificationBody struct {
	ChannelType string          `json:"channel_type"`
	Name        string          `json:"name"`
	ConfigJSON  json.RawMessage `json:"config_json"`
	Enabled     bool            `json:"enabled"`
}

func (h *chatopsHandlers) createNotification(w http.ResponseWriter, r *http.Request) {
	var body createNotificationBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if strings.TrimSpace(body.ChannelType) == "" {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_argument", "channel_type is required")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_argument", "name is required")
		return
	}
	dto, err := h.deps.NotificationSvc.CreateConf(r.Context(), app.CreateConfReq{
		ChannelType: body.ChannelType,
		Name:        body.Name,
		ConfigJSON:  body.ConfigJSON,
		Enabled:     body.Enabled,
	})
	if err != nil {
		mapServiceErr(w, err)
		return
	}
	writeJSON(w, dto)
}

type updateNotificationBody struct {
	ChannelType *string         `json:"channel_type,omitempty"`
	Name        *string         `json:"name,omitempty"`
	ConfigJSON  json.RawMessage `json:"config_json,omitempty"`
	Enabled     *bool           `json:"enabled,omitempty"`
}

func (h *chatopsHandlers) updateNotification(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintPathValue(r, "id")
	if err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	var body updateNotificationBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	req := app.UpdateConfReq{
		ChannelType: body.ChannelType,
		Name:        body.Name,
		ConfigJSON:  body.ConfigJSON,
		Enabled:     body.Enabled,
	}
	if err := h.deps.NotificationSvc.UpdateConf(r.Context(), id, req); err != nil {
		mapServiceErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *chatopsHandlers) deleteNotification(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintPathValue(r, "id")
	if err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	if err := h.deps.NotificationSvc.DeleteConf(r.Context(), id); err != nil {
		mapServiceErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *chatopsHandlers) testNotification(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintPathValue(r, "id")
	if err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	if err := h.deps.NotificationSvc.TestConf(r.Context(), id); err != nil {
		mapServiceErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"success": true, "status": "sent"})
}

// ----- bindings -----

func (h *chatopsHandlers) listBindings(w http.ResponseWriter, r *http.Request) {
	items, err := h.deps.BindingSvc.ListBindings(r.Context())
	if err != nil {
		mapServiceErr(w, err)
		return
	}
	if items == nil {
		items = []app.BindingDTO{}
	}
	pending, err := h.deps.BindingSvc.ListPendingCodes(r.Context())
	if err != nil {
		mapServiceErr(w, err)
		return
	}
	if pending == nil {
		pending = []app.BindCodeDTO{}
	}
	writeJSON(w, map[string]any{
		"bindings": items,
		"pending":  pending,
	})
}

type issueCodeBody struct {
	ConfID uint   `json:"conf_id"`
	Label  string `json:"label"`
}

func (h *chatopsHandlers) issueBindCode(w http.ResponseWriter, r *http.Request) {
	var body issueCodeBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if body.ConfID == 0 {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_argument", "conf_id is required")
		return
	}
	dto, err := h.deps.BindingSvc.IssueCode(r.Context(), body.ConfID, body.Label)
	if err != nil {
		mapServiceErr(w, err)
		return
	}
	writeJSON(w, map[string]any{
		"code":       dto.Code,
		"conf_id":    dto.ConfID,
		"label":      dto.Label,
		"expires_at": dto.ExpiresAt,
		"created_at": dto.CreatedAt,
	})
}

func (h *chatopsHandlers) revokeBinding(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintPathValue(r, "id")
	if err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	if err := h.deps.BindingSvc.Revoke(r.Context(), id); err != nil {
		mapServiceErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

type patchBindingBody struct {
	ReplyLang *string `json:"reply_lang,omitempty"`
}

func (h *chatopsHandlers) patchBinding(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintPathValue(r, "id")
	if err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	var body patchBindingBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if body.ReplyLang == nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_argument", "no fields to patch")
		return
	}
	if err := h.deps.BindingSvc.SetReplyLang(r.Context(), id, *body.ReplyLang); err != nil {
		mapServiceErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// ----- audit -----

func parseAuditTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

func parseAuditInt(raw string, def int) (int, error) {
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (h *chatopsHandlers) queryAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since, err := parseAuditTime(q.Get("since"))
	if err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_argument", "since: "+err.Error())
		return
	}
	until, err := parseAuditTime(q.Get("until"))
	if err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_argument", "until: "+err.Error())
		return
	}
	page, err := parseAuditInt(q.Get("page"), 0)
	if err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_argument", "page: "+err.Error())
		return
	}
	pageSize, err := parseAuditInt(q.Get("page_size"), 0)
	if err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_argument", "page_size: "+err.Error())
		return
	}

	items, total, err := h.deps.AuditSvc.Query(r.Context(), app.AuditQuery{
		Since:         since,
		Until:         until,
		ChannelUserID: q.Get("channel_user_id"),
		Command:       q.Get("command"),
		Result:        q.Get("result"),
		Page:          page,
		PageSize:      pageSize,
	})
	if err != nil {
		mapServiceErr(w, err)
		return
	}
	if items == nil {
		items = []app.AuditDTO{}
	}
	writeJSON(w, map[string]any{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ----- tokens -----

type createTokenBody struct {
	Kind  string `json:"kind"`
	Scope string `json:"scope"`
	TTLS  int64  `json:"ttl_s"`
}

func (h *chatopsHandlers) createToken(w http.ResponseWriter, r *http.Request) {
	if h.deps.TokenAdmin == nil {
		writeChatopsErr(w, http.StatusServiceUnavailable, "not_wired", "token admin store not configured")
		return
	}
	var body createTokenBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if strings.TrimSpace(body.Kind) == "" {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_argument", "kind is required")
		return
	}
	ttl := time.Duration(body.TTLS) * time.Second
	dto, plain, err := h.deps.TokenAdmin.CreateToken(r.Context(), body.Kind, body.Scope, ttl)
	if err != nil {
		mapServiceErr(w, err)
		return
	}
	writeJSON(w, map[string]any{
		"token":     dto,
		"plaintext": plain,
	})
}

func (h *chatopsHandlers) listTokens(w http.ResponseWriter, r *http.Request) {
	if h.deps.TokenAdmin == nil {
		writeChatopsErr(w, http.StatusServiceUnavailable, "not_wired", "token admin store not configured")
		return
	}
	items, err := h.deps.TokenAdmin.ListTokens(r.Context())
	if err != nil {
		mapServiceErr(w, err)
		return
	}
	if items == nil {
		items = []TokenDTO{}
	}
	writeJSON(w, items)
}

func (h *chatopsHandlers) deleteToken(w http.ResponseWriter, r *http.Request) {
	if h.deps.TokenAdmin == nil {
		writeChatopsErr(w, http.StatusServiceUnavailable, "not_wired", "token admin store not configured")
		return
	}
	id, err := parseUintPathValue(r, "id")
	if err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	if err := h.deps.TokenAdmin.DeleteToken(r.Context(), id); err != nil {
		mapServiceErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}
