package web

import (
	"errors"
	"net/http"

	"github.com/sunerpy/pt-tools/internal/notify/adapter/webhook"
	"github.com/sunerpy/pt-tools/internal/notify/tmpl"
)

// ----- 自定义 Webhook 请求的预览（M21）：按示例数据渲染，不发送；只认网页登录 -----

func (h *chatopsHandlers) previewWebhook(w http.ResponseWriter, r *http.Request) {
	var in webhook.PreviewInput
	if err := decodeJSONBody(r, &in); err != nil {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	out, err := webhook.Preview(in)
	switch {
	case err == nil:
		writeJSON(w, out)
	case errors.Is(err, webhook.ErrUnknownEvent):
		writeChatopsErr(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, webhook.ErrInvalid), errors.Is(err, tmpl.ErrInvalid):
		writeChatopsErr(w, http.StatusBadRequest, "invalid_template", err.Error())
	default:
		writeChatopsErr(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}
