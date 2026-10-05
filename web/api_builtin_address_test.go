package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 内置站点的地址由站点定义管理（界面只读，启动时按定义对齐）。保存配置时改成别的地址会被拒绝并保持原值，
// 而不是先写进去、下次启动又悄悄恢复；带回原值的保存照常成功。
func TestSiteDetail_RejectsBuiltinAddressChange(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	_, builtin := v2.GetGlobalSiteRegistry().Get("hdsky")
	require.True(t, builtin, "hdsky is a built-in site in this test binary")
	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "uid=1",
	}))

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.apiSiteDetail(rec, httptest.NewRequest(http.MethodPost, "/api/sites/hdsky", bytes.NewBufferString(body)))
		return rec
	}
	storedURL := func() string {
		var row models.SiteSetting
		require.NoError(t, global.GlobalDB.DB.Where("name = ?", "hdsky").First(&row).Error)
		return row.APIUrl
	}
	before := storedURL()

	rec := post(`{"enabled":true,"auth_method":"cookie","api_url":"https://mirror.example","rss":[]}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "内置站点的地址")
	assert.Equal(t, before, storedURL(), "the address is not changed")

	rec = post(`{"enabled":true,"auth_method":"cookie","api_url":"` + before + `","rss":[]}`)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
