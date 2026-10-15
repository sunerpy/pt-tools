package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
)

// 自动删种的状态与预览：只认会话；没有下载器时预览是空的；范围、条件写错回 400
func TestCleanupAPI(t *testing.T) {
	e := newAppEnv(t)
	tok := e.token(apitoken.ScopeAppRead, apitoken.ScopeAppWrite)
	for _, q := range []appReq{
		{method: http.MethodGet, path: "/api/cleanup/status", bearer: tok},
		{method: http.MethodPost, path: "/api/cleanup/preview", bearer: tok, body: `{}`},
	} {
		assert.Equal(t, http.StatusUnauthorized, e.do(q).Code, q.path)
	}

	w := e.do(appReq{method: http.MethodGet, path: "/api/cleanup/status", session: true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var st CleanupStatusView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &st))
	assert.Nil(t, st.LastRun, "还没跑过")

	w = e.do(appReq{method: http.MethodPost, path: "/api/cleanup/preview", session: true, body: `{"cleanup_scope":"all","cleanup_max_seed_time_h":1}`})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var pv CleanupPreviewView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pv))
	assert.NotNil(t, pv.Results)

	for _, bad := range []string{`{"cleanup_scope":"x"}`, `{"cleanup_condition_mode":"xor"}`, `{"cleanup_scop":"all"}`} {
		assert.Equal(t, http.StatusBadRequest, e.do(appReq{method: http.MethodPost, path: "/api/cleanup/preview", session: true, body: bad}).Code, bad)
	}

	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	assert.Equal(t, http.StatusServiceUnavailable, e.do(appReq{method: http.MethodGet, path: "/api/cleanup/status", session: true}).Code)
	assert.Equal(t, http.StatusServiceUnavailable, e.do(appReq{method: http.MethodPost, path: "/api/cleanup/preview", session: true, body: `{}`}).Code)
}
