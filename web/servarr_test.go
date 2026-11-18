package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sunerpy/pt-tools/global"
)

// Servarr 兼容入口挂在 /servarr/ 下：不经会话（Seerr 带的是 API Key），没接上时回 404；访问日志只记路径，查询参数里的 Key 不进日志。
func TestServarrRoutes(t *testing.T) {
	srv, _ := setupTestServer(t)
	core, logs := observer.New(zapcore.DebugLevel)
	saved := global.GlobalLogger
	global.GlobalLogger = zap.New(core)
	t.Cleanup(func() { global.GlobalLogger = saved })

	const key = "ptt_9_SECRETSECRETSECRET"
	rec := httptest.NewRecorder()
	srv.buildHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/servarr/radarr/api/v3/system/status?apikey="+key, nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "没有启动")
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

	var got *http.Request
	srv.SetServarr(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.WriteHeader(http.StatusTeapot)
	}))
	rec = httptest.NewRecorder()
	srv.buildHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/servarr/sonarr/api/v3/series?apikey="+key, nil))
	assert.Equal(t, http.StatusTeapot, rec.Code, "不要会话，交给兼容入口")
	require.NotNil(t, got)
	assert.Equal(t, key, got.URL.Query().Get("apikey"))

	logged := 0
	for _, e := range logs.All() {
		assert.NotContains(t, e.Message, "SECRET")
		if strings.Contains(e.Message, "path=/servarr/") {
			logged++
		}
	}
	assert.Equal(t, 2, logged, "两次请求都记了路径")
}
