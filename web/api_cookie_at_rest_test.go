package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// 站点 Cookie 只以密文落库：扩展同步凭证、新建动态站点、导入模板这三个入口都不再写明文列。
func TestCookieIsStoredOnlyEncrypted(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.SiteTemplate{}))
	db := global.GlobalDB.DB

	assertEncryptedOnly := func(t *testing.T, name, want string) {
		t.Helper()
		var row models.SiteSetting
		require.NoError(t, db.Where("name = ?", name).First(&row).Error)
		assert.Empty(t, row.Cookie, "%s keeps no plaintext cookie", name)
		plain, err := srv.store.DecryptCookie(row.CookieEncrypted)
		require.NoError(t, err, name)
		assert.Equal(t, want, plain, name)
	}

	t.Run("extension credential sync", func(t *testing.T) {
		enabled := true
		require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
			Enabled: &enabled, AuthMethod: "cookie", Cookie: "old=1", APIUrl: "https://hdsky.me",
		}))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/hdsky", bytes.NewBufferString(`{"cookie":"uid=9; pass=new"}`))
		srv.apiSiteDetail(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assertEncryptedOnly(t, "hdsky", "uid=9; pass=new")
	})

	t.Run("dynamic site", func(t *testing.T) {
		rec := httptest.NewRecorder()
		body := `{"name":"cookie-dyn","display_name":"Cookie Dyn","base_url":"https://dyn.example","auth_method":"cookie","cookie":"dyn=1"}`
		srv.apiDynamicSites(rec, httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewBufferString(body)))
		require.Less(t, rec.Code, 300, rec.Body.String())
		assertEncryptedOnly(t, "cookie-dyn", "dyn=1")
	})

	t.Run("template import", func(t *testing.T) {
		tpl, _ := json.Marshal(models.SiteTemplateExport{Name: "cookie-tpl", DisplayName: "Cookie Tpl", BaseURL: "https://tpl.example", AuthMethod: "cookie"})
		body, _ := json.Marshal(TemplateImportRequest{Template: tpl, Cookie: "tpl=1"})
		rec := httptest.NewRecorder()
		srv.apiSiteTemplateImport(rec, httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body)))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assertEncryptedOnly(t, "cookie-tpl", "tpl=1")
	})
}
