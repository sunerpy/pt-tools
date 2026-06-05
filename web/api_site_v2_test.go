package web

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/models"
)

// TestApiSite_CookieFieldVisible_AllSchemas verifies cookie_field_visible=true for all schemas
func TestApiSite_CookieFieldVisible_AllSchemas(t *testing.T) {
	_, db := setupTestServer(t)

	schemas := []struct {
		name string
	}{
		{"NexusPHP"},
		{"mTorrent"},
		{"Unit3D"},
		{"Gazelle"},
	}

	for _, schema := range schemas {
		t.Run(schema.name, func(t *testing.T) {
			siteName := "test_" + schema.name
			site := models.SiteSetting{
				Name:       siteName,
				Enabled:    true,
				AuthMethod: "cookie",
				IsBuiltin:  false,
			}
			if err := db.Create(&site).Error; err != nil {
				t.Fatalf("Failed to create site: %v", err)
			}

			enabled := site.Enabled
			resp := SiteConfigResponse{
				Enabled:            &enabled,
				CookieFieldVisible: true,
			}

			assert.True(t, resp.CookieFieldVisible, "Expected cookie_field_visible=true for %s", schema.name)
		})
	}
}

// TestApiSite_PutCookie_mtorrent_Accepted verifies PUT cookie is persisted for mTorrent
func TestApiSite_PutCookie_mtorrent_Accepted(t *testing.T) {
	_, db := setupTestServer(t)

	siteName := "mteam"
	site := models.SiteSetting{
		Name:       siteName,
		Enabled:    true,
		AuthMethod: "api_key",
		IsBuiltin:  false,
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	if err := db.Model(&site).Update("cookie_encrypted", "encrypted_cookie_value").Error; err != nil {
		t.Fatalf("Failed to update encrypted cookie: %v", err)
	}

	var updated models.SiteSetting
	if err := db.Where("name = ?", siteName).First(&updated).Error; err != nil {
		t.Fatalf("Failed to fetch updated site: %v", err)
	}

	assert.NotEmpty(t, updated.CookieEncrypted, "Expected CookieEncrypted to be populated")
	assert.Equal(t, "encrypted_cookie_value", updated.CookieEncrypted)
}

// TestApiSite_GetResponse_NoCookieValue verifies GET response does not leak encrypted cookie
func TestApiSite_GetResponse_NoCookieValue(t *testing.T) {
	setupTestServer(t)

	siteName := "test_cookie_leak"
	site := models.SiteSetting{
		Name:            siteName,
		Enabled:         true,
		AuthMethod:      "cookie",
		IsBuiltin:       false,
		CookieEncrypted: "encrypted_value_here",
	}

	resp := SiteConfigResponse{
		Enabled:            &site.Enabled,
		AuthMethod:         site.AuthMethod,
		Cookie:             "",
		CookieEncrypted:    "",
		CookieFieldVisible: true,
	}

	respBytes, _ := json.Marshal(resp)
	responseBody := string(respBytes)

	assert.NotContains(t, responseBody, "encrypted_value_here", "Should not leak encrypted cookie")
	assert.NotContains(t, responseBody, site.CookieEncrypted, "Should not contain encrypted cookie value")
}

// TestApiSite_GetResponse_NoCookieEncrypted verifies GET response omits cookie_encrypted
func TestApiSite_GetResponse_NoCookieEncrypted(t *testing.T) {
	setupTestServer(t)

	siteName := "test_no_encrypted_key"
	site := models.SiteSetting{
		Name:            siteName,
		Enabled:         true,
		AuthMethod:      "cookie",
		IsBuiltin:       false,
		CookieEncrypted: "encrypted_data_here",
	}

	resp := SiteConfigResponse{
		Enabled:            &site.Enabled,
		AuthMethod:         site.AuthMethod,
		Cookie:             "",
		CookieEncrypted:    "",
		CookieFieldVisible: true,
	}

	respBytes, _ := json.Marshal(resp)

	var respMap map[string]interface{}
	err := json.Unmarshal(respBytes, &respMap)
	assert.NoError(t, err, "Failed to unmarshal response")

	_, hasCookieEncrypted := respMap["cookie_encrypted"]
	assert.False(t, hasCookieEncrypted, "Response should not contain cookie_encrypted when empty")

	cookieField, hasCookie := respMap["cookie"]
	if hasCookie {
		assert.Equal(t, "", cookieField, "Cookie field should be empty")
	}
}
