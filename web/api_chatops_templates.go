package web

import (
	"errors"
	"net/http"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/notify/tmpl"
)

// ----- 消息模板（M21）：只认网页登录，和其他 ChatOps 管理接口一样 -----

func (h *chatopsHandlers) templateSvc(w http.ResponseWriter) (app.TemplateService, bool) {
	if h.deps.TemplateSvc == nil {
		writeChatopsErr(w, http.StatusServiceUnavailable, "unavailable", "消息模板服务没有启动")
		return nil, false
	}
	return h.deps.TemplateSvc, true
}

func writeTemplateErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrUnknownEvent):
		writeChatopsErr(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, tmpl.ErrInvalid):
		writeChatopsErr(w, http.StatusBadRequest, "invalid_template", err.Error())
	default:
		writeChatopsErr(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}

func (h *chatopsHandlers) listTemplates(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.templateSvc(w)
	if !ok {
		return
	}
	out, err := svc.List(r.Context())
	if err != nil {
		writeTemplateErr(w, err)
		return
	}
	writeJSON(w, out)
}

func (h *chatopsHandlers) saveTemplate(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.templateSvc(w)
	if !ok {
		return
	}
	var in app.TemplateInput
	if err := decodeJSONBody(r, &in); err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	out, err := svc.Save(r.Context(), r.PathValue("type"), in)
	if err != nil {
		writeTemplateErr(w, err)
		return
	}
	writeJSON(w, out)
}

func (h *chatopsHandlers) resetTemplate(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.templateSvc(w)
	if !ok {
		return
	}
	if err := svc.Reset(r.Context(), r.PathValue("type")); err != nil {
		writeTemplateErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"success": true})
}

func (h *chatopsHandlers) previewTemplate(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.templateSvc(w)
	if !ok {
		return
	}
	var in struct {
		Type string `json:"type"`
		app.TemplateInput
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	out, err := svc.Preview(in.Type, in.TemplateInput)
	if err != nil {
		writeTemplateErr(w, err)
		return
	}
	writeJSON(w, out)
}
