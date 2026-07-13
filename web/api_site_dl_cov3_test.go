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
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestApiSiteTemplateImport_AllAuthMethods(t *testing.T) {
	writeWebTestSecretKey(t)

	cases := []struct {
		name string
		tpl  models.SiteTemplateExport
		req  TemplateImportRequest
	}{
		{"api_key", models.SiteTemplateExport{Name: "impk", AuthMethod: "api_key", BaseURL: "https://a.example.com"}, TemplateImportRequest{APIKey: "k1"}},
		{"passkey", models.SiteTemplateExport{Name: "imppass", AuthMethod: "passkey", BaseURL: "https://b.example.com"}, TemplateImportRequest{Passkey: "p1"}},
		{"cookie_and_api_key", models.SiteTemplateExport{Name: "impboth", AuthMethod: "cookie_and_api_key", BaseURL: "https://c.example.com"}, TemplateImportRequest{Cookie: "c=1", APIKey: "k2"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, db := setupTestServer(t)
			require.NoError(t, db.AutoMigrate(&models.SiteTemplate{}, &models.SiteSetting{}))
			tplBytes, _ := json.Marshal(tc.tpl)
			tc.req.Template = tplBytes
			body, _ := json.Marshal(tc.req)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
			server.apiSiteTemplateImport(w, req)
			require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		})
	}
}

func TestApiAddDownloaderTorrent_SpecificIDsMultiSuccess(t *testing.T) {
	fake := &fakeDownloader{
		addResult: downloader.AddTorrentResult{Success: true, ID: "n1", Hash: "h1", Message: "ok"},
		freSpace:  1 << 40,
	}
	server, id := setupServerWithFakeDownloader(t, fake)
	gs, err := server.store.GetGlobalSettings()
	require.NoError(t, err)
	require.NoError(t, server.store.SaveGlobalSettings(gs))
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

	body, _ := json.Marshal(AddDownloaderTorrentRequest{
		DownloaderIDs: []uint{id},
		MagnetLink:    "magnet:?xt=urn:btih:xyz",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp AddDownloaderTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.SuccessCount)
}

var _ = downloader.AddTorrentResult{}
