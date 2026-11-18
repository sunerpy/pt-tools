package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// #542：站点设置「H&R 上传量达标」网页接口能存能读，站点列表也带上；越界值回 400，库里的值不变。
func TestApiSiteDetail_HRUploadRatio(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.SiteTemplate{}))
	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.apiSiteDetail(rec, httptest.NewRequest(http.MethodPost, "/api/sites/hdsky", bytes.NewBufferString(body)))
		return rec
	}
	get := func() map[string]any {
		rec := httptest.NewRecorder()
		srv.apiSiteDetail(rec, httptest.NewRequest(http.MethodGet, "/api/sites/hdsky", nil))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var got map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		return got
	}

	rec := post(`{"enabled":true,"auth_method":"cookie","cookie":"c=1","hr_seed_time_h":72,"hr_upload_ratio":0.9,"rss":[]}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.EqualValues(t, 0.9, get()["hr_upload_ratio"])

	rec = post(`{"enabled":true,"auth_method":"cookie","cookie":"c=1","hr_upload_ratio":11,"rss":[]}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.EqualValues(t, 0.9, get()["hr_upload_ratio"], "越界的不存")

	list := httptest.NewRecorder()
	srv.apiSites(list, httptest.NewRequest(http.MethodGet, "/api/sites", nil))
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	var sites map[string]map[string]any
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &sites))
	assert.EqualValues(t, 0.9, sites["hdsky"]["hr_upload_ratio"])
}

// #542：配置网关（AI 助手与 App API 改配置）能读能改站点的「H&R 上传量达标」，越界的不收。
func TestConfigSites_HRUploadRatio(t *testing.T) {
	e := newConfigEnv(t)
	ctx := context.Background()
	db := global.GlobalDB.DB
	site := models.SiteSetting{Name: "hdsky", AuthMethod: "cookie", IsBuiltin: true}
	require.NoError(t, db.Create(&site).Error)

	_, err := e.srv.ConfigApply(ctx, "sites", ConfigChange{Op: "update", ID: "hdsky", Values: map[string]any{"hr_upload_ratio": 0.9}})
	require.NoError(t, err)
	require.NoError(t, db.First(&site, site.ID).Error)
	assert.InDelta(t, 0.9, site.HRUploadRatio, 1e-9)

	st, err := e.srv.ConfigRead(ctx, "sites", "hdsky")
	require.NoError(t, err)
	require.Len(t, st.Items, 1)
	assert.Equal(t, json.Number("0.9"), st.Items[0]["hr_upload_ratio"], "配置网关的数字按 json.Number 返回")

	_, err = e.srv.ConfigApply(ctx, "sites", ConfigChange{Op: "update", ID: "hdsky", Values: map[string]any{"hr_upload_ratio": 20}})
	require.Error(t, err)
	require.NoError(t, db.First(&site, site.ID).Error)
	assert.InDelta(t, 0.9, site.HRUploadRatio, 1e-9, "越界的不存")
}
