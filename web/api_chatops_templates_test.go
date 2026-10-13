package web

import (
	"net/http"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

func TestChatOpsTemplateAPI(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	// 没接上模板服务时回 503
	resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/templates", tok, nil)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	resp.Body.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.NotificationTemplate{}))
	set := notify.NewTemplates(nil)
	deps.TemplateSvc = app.NewTemplateService(db, set)

	var cat app.TemplateCatalog
	resp = chatopsReq(t, srv, http.MethodGet, "/api/chatops/templates", tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	decodeBody(t, resp, &cat)
	assert.Len(t, cat.Events, len(notify.Events))
	assert.NotEmpty(t, cat.CommonVars)

	var v app.TemplateView
	resp = chatopsReq(t, srv, http.MethodPut, "/api/chatops/templates/rss.matched", tok, map[string]string{"title": "[{{.site}}] {{.torrent_title}}"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	decodeBody(t, resp, &v)
	assert.True(t, v.Customized)
	ev, title, text := notify.SampleEvent(notify.EventRSSMatched)
	assert.Equal(t, "[hdtime] Fake.Movie.2026.1080p.WEB-DL.H.264-FAKE", set.Apply(notify.Notification{Event: ev, Title: title, Text: text}).Title)

	var e errResp
	resp = chatopsReq(t, srv, http.MethodPut, "/api/chatops/templates/rss.matched", tok, map[string]string{"text": "{{.nope}}"})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	decodeBody(t, resp, &e)
	assert.Equal(t, "invalid_template", e.Error)
	assert.Contains(t, e.Detail, "没有这个变量：nope")

	resp = chatopsReq(t, srv, http.MethodPut, "/api/chatops/templates/nope", tok, map[string]string{"text": "x"})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
	resp = chatopsReq(t, srv, http.MethodPut, "/api/chatops/templates/rss.matched", tok, map[string]any{"text": 1})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "请求体不对")
	resp.Body.Close()

	var p app.TemplatePreview
	resp = chatopsReq(t, srv, http.MethodPost, "/api/chatops/templates/preview", tok, map[string]string{"type": "test", "text": "通道 {{.channel}}"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	decodeBody(t, resp, &p)
	assert.Equal(t, "pt-tools 测试通知", p.Title)
	assert.Equal(t, "通道 我的 Telegram", p.Text)
	resp = chatopsReq(t, srv, http.MethodPost, "/api/chatops/templates/preview", tok, map[string]string{"type": "test", "text": "{{range .count}}{{end}}"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	resp = chatopsReq(t, srv, http.MethodDelete, "/api/chatops/templates/rss.matched", tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	assert.Equal(t, title, set.Apply(notify.Notification{Event: ev, Title: title, Text: text}).Title, "恢复默认")
	resp = chatopsReq(t, srv, http.MethodDelete, "/api/chatops/templates/nope", tok, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}
