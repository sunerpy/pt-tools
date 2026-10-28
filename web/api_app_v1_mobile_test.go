package web

import (
	"encoding/json"
	"net/http"
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
