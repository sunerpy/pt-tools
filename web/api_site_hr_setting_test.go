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

// #727：站点设置里的「H&R 做种要求（小时）」能保存、读回；读回时带上站点定义里有没有现成的 H&R 规则。
func TestApiSiteDetail_HRSeedTimeSetting(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.SiteTemplate{}))
	for site, body := range map[string]string{
		"hdsky":    `{"enabled":true,"auth_method":"cookie","cookie":"c=1","hr_seed_time_h":72,"rss":[]}`,
		"btschool": `{"enabled":true,"auth_method":"cookie","cookie":"c=1","rss":[]}`,
	} {
		post := httptest.NewRecorder()
		srv.apiSiteDetail(post, httptest.NewRequest(http.MethodPost, "/api/sites/"+site, bytes.NewBufferString(body)))
		require.Equal(t, http.StatusOK, post.Code, post.Body.String())
	}

	for site, builtin := range map[string]bool{"hdsky": false, "btschool": true} {
		get := httptest.NewRecorder()
		srv.apiSiteDetail(get, httptest.NewRequest(http.MethodGet, "/api/sites/"+site, nil))
		require.Equal(t, http.StatusOK, get.Code, get.Body.String())
		var got map[string]any
		require.NoError(t, json.Unmarshal(get.Body.Bytes(), &got))
		require.Contains(t, got, "hr_rule_builtin", site)
		assert.Equal(t, builtin, got["hr_rule_builtin"], site)
		if site == "hdsky" {
			require.Contains(t, got, "hr_seed_time_h")
			assert.EqualValues(t, 72, got["hr_seed_time_h"])
		}
	}
}
