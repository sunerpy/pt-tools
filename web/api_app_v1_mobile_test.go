package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestAppMobileBrushLifecycle(t *testing.T) {
	e := newAppEnv(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.BrushTask{}, &models.BrushTorrent{}, &models.BrushTorrentSample{}, &models.BrushDailyStat{}, &models.BrushRun{}))
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true, AuthMethod: "cookie", Cookie: "secret-cookie", IsBuiltin: true}).Error)
	dl := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Username: "secret-user", Password: "secret-password", Enabled: true}
	require.NoError(t, db.Create(&dl).Error)
	read := e.token(apitoken.ScopeAppRead)
	full := e.token(apitoken.ScopeAppRead, apitoken.ScopeAppWrite)
	body := brushBody("phone", dl.ID, nil)
	w := e.do(appReq{method: http.MethodPost, path: appPrefix + "/brush/tasks", body: body, bearer: read})
	require.Equal(t, 403, w.Code)
	w = e.do(appReq{method: http.MethodPost, path: appPrefix + "/brush/tasks", body: body, bearer: full})
	require.Equal(t, 200, w.Code, w.Body.String())
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	id := strconv.Itoa(int(got["id"].(float64)))
	assert.Equal(t, "phone", got["name"])
	path := appPrefix + "/brush/tasks/" + id
	w = e.do(appReq{method: http.MethodGet, path: path, bearer: read})
	require.Equal(t, 200, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	w = e.do(appReq{method: http.MethodPost, path: path + "/status", body: `{"enabled":true}`, bearer: full})
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "last_error")
	var row models.BrushTask
	require.NoError(t, db.First(&row, id).Error)
	assert.True(t, row.Enabled)
	w = e.do(appReq{method: http.MethodPost, path: path + "/status", body: `{}`, bearer: full})
	assert.Equal(t, 400, w.Code)
	require.NoError(t, db.Create(&models.BrushRun{TaskID: uint(got["id"].(float64)), StartedAt: time.Now(), Status: "failed", Error: "Get https://private.example/download.php?passkey=run-secret failed", Steps: `[{"kind":"error","text":"https://private.example/download.php?passkey=step-secret"}]`}).Error)
	w = e.do(appReq{method: http.MethodGet, path: path + "/runs", bearer: read})
	require.Equal(t, 200, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "run-secret")
	assert.NotContains(t, w.Body.String(), "step-secret")
	w = e.do(appReq{method: http.MethodPost, path: appPrefix + "/brush/tasks/preview", body: `{"op":"update","id":"` + id + `","values":{"name":"preview-only"}}`, bearer: read})
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"applied":false`)
	require.NoError(t, db.First(&row, id).Error)
	assert.Equal(t, "phone", row.Name)
	w = e.do(appReq{method: http.MethodGet, path: appPrefix + "/config/brush_tasks", bearer: full})
	assert.Equal(t, 403, w.Code, "no general config grant")
	w = e.do(appReq{method: http.MethodDelete, path: path, bearer: full})
	require.Equal(t, 200, w.Code, w.Body.String())
	w = e.do(appReq{method: http.MethodGet, path: path, bearer: read})
	assert.Equal(t, 404, w.Code)
	assert.NotEmpty(t, e.audit.all())
}

func TestAppMobileStatsAndOptionsScopes(t *testing.T) {
	e := newAppEnv(t)
	read := e.token(apitoken.ScopeAppRead)
	for _, path := range []string{"/mobile/options", "/stats/aggregated", "/stats/summary"} {
		assert.Equal(t, 401, e.do(appReq{method: http.MethodGet, path: appPrefix + path}).Code)
		w := e.do(appReq{method: http.MethodGet, path: appPrefix + path, bearer: read})
		assert.NotEqual(t, 404, w.Code)
		assertNoSecrets(t, w.Body.Bytes())
	}
}

func TestAppMobileStatsSuccess(t *testing.T) {
	srv, repo := historyFixture(t)
	saveOn(t, repo, "2026-10-05", "hdsky", 100, 10, 1)
	saveOn(t, repo, "2026-10-06", "hdsky", 300, 30, 5)
	saveOn(t, repo, "2026-10-06", "disabledsite", 999, 999, 999)
	repo.SetClock(func() time.Time { return time.Date(2026, 10, 6, 20, 0, 0, 0, time.UTC) }, time.UTC)
	w := appAs(t, srv.appStatsAggregated, http.MethodGet, appPrefix+"/stats/aggregated")
	require.Equal(t, 200, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var stats AggregatedStatsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))
	assert.EqualValues(t, 300, stats.TotalUploaded)
	assert.EqualValues(t, 30, stats.TotalDownloaded)
	for _, period := range []string{"today", "7d", "30d"} {
		w = appAs(t, srv.appStatsSummary, http.MethodGet, appPrefix+"/stats/summary?range="+period)
		require.Equal(t, 200, w.Code, w.Body.String())
		assertNoSecrets(t, w.Body.Bytes())
		var sum v2.DeltaSummary
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sum))
		assert.Equal(t, period, sum.Range)
	}
	assert.Equal(t, 400, appAs(t, srv.appStatsSummary, http.MethodGet, appPrefix+"/stats/summary?range=invalid").Code)
}

func TestAppMobileSubscriptionKeywords(t *testing.T) {
	srv, _ := newSubscribeEnv(t)
	w := appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, appPrefix+"/subscriptions", `{"media_type":"movie","tmdb_id":278,"include_keywords":"2160p","exclude_keywords":"CAM"}`, "app:write")
	require.Equal(t, 200, w.Code, w.Body.String())
	var sub AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sub))
	assert.Equal(t, "2160p", sub.IncludeKeywords)
	assert.Equal(t, "CAM", sub.ExcludeKeywords)
	id := strconv.Itoa(int(sub.ID))
	w = appWithID(t, srv.appSubscriptionUpdate, http.MethodPut, appPrefix+"/subscriptions/"+id, id, `{"exclude_keywords":""}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sub))
	assert.Equal(t, "2160p", sub.IncludeKeywords)
	assert.Empty(t, sub.ExcludeKeywords)
}

func TestAppMobileOptionsAdd(t *testing.T) {
	var o AppMobileOptions
	o.add("sites", []map[string]any{
		{"name": "hdsky", "display_name": "HDSky", "enabled": true},
		{"name": "plain", "enabled": true},
		{"name": "off", "display_name": "Off", "enabled": false},
		{"name": "unknown"},
	})
	o.add("downloaders", []map[string]any{{"id": "3", "name": "qb"}})
	o.add("quality_profiles", []map[string]any{{"id": float64(5), "name": "4K"}})
	o.add("libraries", []map[string]any{{"id": json.Number("7"), "name": "电影"}})
	assert.Equal(t, []AppMobileSiteOption{
		{Name: "hdsky", DisplayName: "HDSky"},
		{Name: "plain", DisplayName: "plain"},
		{Name: "unknown", DisplayName: "unknown"},
	}, o.Sites, "disabled sites are left out; a missing display name falls back to the name")
	assert.Equal(t, []AppMobileOption{{ID: 3, Name: "qb"}}, o.Downloaders)
	assert.Equal(t, []AppMobileOption{{ID: 5, Name: "4K"}}, o.Profiles)
	assert.Equal(t, []AppMobileOption{{ID: 7, Name: "电影"}}, o.Libraries)
}

func TestAppMobileOptionsListsEnabledChoicesWithoutSecrets(t *testing.T) {
	e := newAppEnv(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true, AuthMethod: "cookie", Cookie: "secret-cookie", IsBuiltin: true}).Error)
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Username: "secret-user", Password: "secret-password", Enabled: true}).Error)
	w := e.do(appReq{method: http.MethodGet, path: appPrefix + "/mobile/options", bearer: e.token(apitoken.ScopeAppRead)})
	require.Equal(t, 200, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "secret-")
	var got AppMobileOptions
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.NotEmpty(t, got.Downloaders)
	assert.Equal(t, "qb", got.Downloaders[0].Name)
	assert.NotZero(t, got.Downloaders[0].ID)
}

func TestAppMobileBrushRejectsBadRequests(t *testing.T) {
	e := newAppEnv(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.BrushTask{}, &models.BrushTorrent{}, &models.BrushTorrentSample{}, &models.BrushDailyStat{}, &models.BrushRun{}))
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true, AuthMethod: "cookie", Cookie: "secret-cookie", IsBuiltin: true}).Error)
	dl := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.Create(&dl).Error)
	read := e.token(apitoken.ScopeAppRead)
	full := e.token(apitoken.ScopeAppRead, apitoken.ScopeAppWrite)
	for _, q := range []appReq{
		{method: http.MethodGet, path: appPrefix + "/brush/tasks/abc", bearer: read},
		{method: http.MethodGet, path: appPrefix + "/brush/tasks/0/runs", bearer: read},
		{method: http.MethodPut, path: appPrefix + "/brush/tasks/abc", body: `{"name":"x"}`, bearer: full},
		{method: http.MethodPost, path: appPrefix + "/brush/tasks/preview", body: `{`, bearer: read},
		{method: http.MethodPost, path: appPrefix + "/brush/tasks", body: `{`, bearer: full},
		{method: http.MethodPost, path: appPrefix + "/brush/tasks/1/status", body: `{`, bearer: full},
		{method: http.MethodDelete, path: appPrefix + "/brush/tasks/1", body: `{"id":1}`, bearer: full},
	} {
		w := e.do(q)
		assert.Equal(t, 400, w.Code, "%s %s: %s", q.method, q.path, w.Body.String())
	}
	var stored int64
	require.NoError(t, db.Model(&models.BrushTask{}).Count(&stored).Error)
	assert.Zero(t, stored, "rejected requests change nothing")

	// null clears nothing: the field is dropped and keeps its stored value
	w := e.do(appReq{method: http.MethodPost, path: appPrefix + "/brush/tasks", body: brushBody("phone", dl.ID, nil), bearer: full})
	require.Equal(t, 200, w.Code, w.Body.String())
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	id := strconv.Itoa(int(got["id"].(float64)))
	w = e.do(appReq{method: http.MethodPut, path: appPrefix + "/brush/tasks/" + id, body: `{"name":"renamed","site_name":null}`, bearer: full})
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "renamed", got["name"])
	assert.Equal(t, "hdsky", got["site_name"])
}

func TestAppMobileJSONRejectsNonJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, appPrefix+"/stats/summary", nil)
	w := httptest.NewRecorder()
	appMobileJSON(w, r, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not json")) })
	assert.Equal(t, 500, w.Code)
	assert.Contains(t, w.Body.String(), `"error":"internal"`)
}

func TestAppBrushRunsWithoutDatabase(t *testing.T) {
	srv := setupServer(t)
	saved := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = saved })
	w := appWithID(t, srv.appBrushRuns, http.MethodGet, appPrefix+"/brush/tasks/1/runs", "1", "")
	assert.Equal(t, 503, w.Code)
	assert.Contains(t, w.Body.String(), `"error":"unavailable"`)
}
