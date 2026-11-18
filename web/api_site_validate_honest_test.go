package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 站点验证只检查了必填字段、没有连接站点：原来照样回「站点配置验证成功」，
// 填错域名或过期 Cookie 也显示成功。现在说明没有在线验证，verified 为 false。
func TestSiteValidation_DoesNotClaimVerified(t *testing.T) {
	server, _ := setupTestServer(t)
	for _, req := range []SiteValidationRequest{
		{Name: "test", AuthMethod: "cookie", Cookie: "c"},
		{Name: "test", AuthMethod: "api_key", APIKey: "k"},
		{Name: "test", AuthMethod: "rss_passkey"},
	} {
		body, _ := json.Marshal(req)
		w := httptest.NewRecorder()
		server.apiSiteValidate(w, httptest.NewRequest(http.MethodPost, "/api/sites/validate", bytes.NewReader(body)))
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		verified, ok := resp["verified"]
		require.True(t, ok, "响应要说明是否真的连接站点验证过 auth=%s", req.AuthMethod)
		assert.Equal(t, false, verified)
		assert.NotContains(t, resp["message"], "验证成功", "auth=%s", req.AuthMethod)
	}
}
