package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// historyFixture 建一个带快照的用户数据服务，并把它装进包级变量（测试结束恢复）。
func historyFixture(t *testing.T) (*Server, *v2.DBUserInfoRepo) {
	t.Helper()
	srv, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.RSSSubscription{}))
	repo, err := v2.NewDBUserInfoRepo(global.GlobalDB.DB)
	require.NoError(t, err)
	svc := v2.NewUserInfoService(v2.UserInfoServiceConfig{Repo: repo, Logger: zap.NewNop()})
	prev := userInfoService
	userInfoService = svc
	t.Cleanup(func() { userInfoService = prev })

	enabled := true
	for _, name := range []string{"hdsky", "pter"} {
		_, err := srv.store.UpsertSite(models.SiteGroup(name), models.SiteConfig{Enabled: &enabled, AuthMethod: "cookie", Cookie: "c"})
		require.NoError(t, err)
	}
	return srv, repo
}

func saveOn(t *testing.T, repo *v2.DBUserInfoRepo, date, site string, up, down int64, bonus float64) {
	t.Helper()
	at, err := time.ParseInLocation("2006-01-02 15:04", date+" 12:00", time.UTC)
	require.NoError(t, err)
	repo.SetClock(func() time.Time { return at }, time.UTC)
	require.NoError(t, repo.Save(context.Background(), v2.UserInfo{Site: site, Username: "u", Uploaded: up, Downloaded: down, Bonus: bonus}))
}

func TestUserInfoHistoryAPI(t *testing.T) {
	srv, repo := historyFixture(t)
	saveOn(t, repo, "2026-10-04", "hdsky", 100, 10, 1)
	saveOn(t, repo, "2026-10-05", "hdsky", 150, 20, 3)
	saveOn(t, repo, "2026-10-06", "hdsky", 400, 20, 2)
	// 之后的请求按 2026-10-06 算「今天」
	repo.SetClock(func() time.Time { return time.Date(2026, 10, 6, 20, 0, 0, 0, time.UTC) }, time.UTC)

	w := httptest.NewRecorder()
	srv.apiUserInfoHistory(w, httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/history?site=hdsky&days=2", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp UserInfoHistoryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "2026-10-05", resp.From)
	assert.Equal(t, "2026-10-06", resp.To)
	require.Len(t, resp.Points, 2)
	assert.EqualValues(t, 50, resp.Points[0].DeltaUploaded, "区间第一天也有增量：基线取区间之前最近的快照")
	assert.EqualValues(t, 250, resp.Points[1].DeltaUploaded)
	assert.True(t, resp.Points[1].Negative, "魔力回退")

	for _, q := range []string{"", "?site=hdsky&days=0", "?site=hdsky&days=401", "?site=hdsky&days=x"} {
		w := httptest.NewRecorder()
		srv.apiUserInfoHistory(w, httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/history"+q, nil))
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
	}
}

func TestUserInfoSummaryAPI(t *testing.T) {
	srv, repo := historyFixture(t)
	saveOn(t, repo, "2026-10-05", "hdsky", 100, 10, 1)
	saveOn(t, repo, "2026-10-06", "hdsky", 300, 30, 5)
	saveOn(t, repo, "2026-10-05", "pter", 50, 5, 1)
	saveOn(t, repo, "2026-10-06", "pter", 80, 5, 1)
	saveOn(t, repo, "2026-10-06", "disabledsite", 999, 999, 999) // 未启用的站点不计入
	repo.SetClock(func() time.Time { return time.Date(2026, 10, 6, 20, 0, 0, 0, time.UTC) }, time.UTC)

	w := httptest.NewRecorder()
	srv.apiUserInfoSummary(w, httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/summary?range=today", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var sum v2.DeltaSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sum))
	assert.Equal(t, "today", sum.Range)
	require.Len(t, sum.Sites, 2)
	assert.EqualValues(t, 200+30, sum.TotalUploaded)
	assert.EqualValues(t, 20, sum.TotalDownloaded)

	w = httptest.NewRecorder()
	srv.apiUserInfoSummary(w, httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/summary?range=1y", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDailyReportSettingsAPI(t *testing.T) {
	srv, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.NotificationConf{}))
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10}))
	conf := models.NotificationConf{ChannelType: "webhook", Name: "w", Enabled: true}
	require.NoError(t, global.GlobalDB.DB.Create(&conf).Error)

	w := httptest.NewRecorder()
	srv.apiDailyReportSettings(w, httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/daily-report", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var got core.DailyReportSettings
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, core.DefaultDailyReportTime, got.Time)
	assert.False(t, got.Enabled)

	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.apiDailyReportSettings(rec, httptest.NewRequest(http.MethodPut, "/api/v2/userinfo/daily-report", bytes.NewBufferString(body)))
		return rec
	}
	w = put(`{"enabled":true,"time":"21:00","channel_ids":[]}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, "开启但没选通道被拒绝")
	assert.Contains(t, w.Body.String(), "通知通道")

	w = put(`{"enabled":true,"time":"21:00","channel_ids":[` + itoaUint(conf.ID) + `]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got, err := srv.store.DailyReportSettings()
	require.NoError(t, err)
	assert.Equal(t, core.DailyReportSettings{Enabled: true, Time: "21:00", ChannelIDs: []uint{conf.ID}}, got)

	w = httptest.NewRecorder()
	srv.apiDailyReportSettings(w, httptest.NewRequest(http.MethodDelete, "/api/v2/userinfo/daily-report", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestUserInfoTrendsAPI(t *testing.T) {
	srv, repo := historyFixture(t)
	saveOn(t, repo, "2026-10-04", "hdsky", 100, 10, 1)
	saveOn(t, repo, "2026-10-05", "hdsky", 150, 10, 1)
	saveOn(t, repo, "2026-10-06", "hdsky", 400, 20, 2)
	saveOn(t, repo, "2026-10-05", "disabledsite", 1, 1, 1)
	saveOn(t, repo, "2026-10-06", "disabledsite", 999, 999, 999)
	repo.SetClock(func() time.Time { return time.Date(2026, 10, 6, 20, 0, 0, 0, time.UTC) }, time.UTC)

	w := httptest.NewRecorder()
	srv.apiUserInfoTrends(w, httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/trends?days=3", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp UserInfoTrendsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, []string{"2026-10-04", "2026-10-05", "2026-10-06"}, resp.Dates)
	require.Contains(t, resp.Sites, "hdsky")
	assert.NotContains(t, resp.Sites, "disabledsite", "未启用的站点不计入")
	assert.EqualValues(t, 0, resp.Sites["hdsky"][0].Uploaded, "区间第一天之前没有快照：第一天不算增量")
	assert.EqualValues(t, 50, resp.Sites["hdsky"][1].Uploaded)
	assert.EqualValues(t, 250, resp.Totals[2].Uploaded)

	w = httptest.NewRecorder()
	srv.apiUserInfoTrends(w, httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/trends?days=61", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// 站点配置读不出来时 summary/trends 返回 500，不把故障伪装成「没有数据」。
func TestUserInfoSummaryAndTrends_SiteConfigErrorIs500(t *testing.T) {
	srv, repo := historyFixture(t)
	saveOn(t, repo, "2026-10-05", "hdsky", 100, 10, 1)
	saveOn(t, repo, "2026-10-06", "hdsky", 300, 30, 5)
	repo.SetClock(func() time.Time { return time.Date(2026, 10, 6, 20, 0, 0, 0, time.UTC) }, time.UTC)
	require.NoError(t, global.GlobalDB.DB.Migrator().DropTable(&models.SiteSetting{}))

	w := httptest.NewRecorder()
	srv.apiUserInfoSummary(w, httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/summary?range=today", nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	w = httptest.NewRecorder()
	srv.apiUserInfoTrends(w, httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/trends?days=3", nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// stubHistoryUserInfoRepo 同时是 UserInfoRepo 与 UserInfoHistoryRepo，按字段回错误，走接口的 500 分支。
type stubHistoryUserInfoRepo struct {
	*v2.InMemoryUserInfoRepo
	today       string
	listErr     error
	baselineErr error
}

func (s stubHistoryUserInfoRepo) ListSnapshots(context.Context, string, string, string) ([]v2.UserInfoDailySnapshot, error) {
	return nil, s.listErr
}

func (s stubHistoryUserInfoRepo) SnapshotBaselines(context.Context, string) (map[string]v2.UserInfoDailySnapshot, error) {
	return nil, s.baselineErr
}

func (s stubHistoryUserInfoRepo) PruneSnapshots(context.Context, string) (int64, error) {
	return 0, nil
}

func (s stubHistoryUserInfoRepo) Today() string { return s.today }

func useUserInfoService(t *testing.T, svc *v2.UserInfoService) {
	t.Helper()
	prev := userInfoService
	userInfoService = svc
	t.Cleanup(func() { userInfoService = prev })
}

func TestUserInfoHistoryAPIs_Unavailable(t *testing.T) {
	srv, _ := setupTestServer(t)
	call := func(h http.HandlerFunc, target string) int {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(http.MethodGet, target, nil))
		return w.Code
	}
	useUserInfoService(t, nil)
	assert.Equal(t, http.StatusServiceUnavailable, call(srv.apiUserInfoHistory, "/api/v2/userinfo/history?site=hdsky"))
	assert.Equal(t, http.StatusServiceUnavailable, call(srv.apiUserInfoSummary, "/api/v2/userinfo/summary"))
	assert.Equal(t, http.StatusServiceUnavailable, call(srv.apiUserInfoTrends, "/api/v2/userinfo/trends"))

	// 内存仓库没有每日快照
	useUserInfoService(t, v2.NewUserInfoService(v2.UserInfoServiceConfig{Repo: v2.NewInMemoryUserInfoRepo(), Logger: zap.NewNop()}))
	assert.Equal(t, http.StatusServiceUnavailable, call(srv.apiUserInfoHistory, "/api/v2/userinfo/history?site=hdsky"))
	assert.Equal(t, http.StatusServiceUnavailable, call(srv.apiUserInfoTrends, "/api/v2/userinfo/trends?days=3"))

	for _, h := range []http.HandlerFunc{srv.apiUserInfoHistory, srv.apiUserInfoSummary, srv.apiUserInfoTrends} {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/x", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	}
}

func TestUserInfoHistoryAPIs_StoreErrorsAre500(t *testing.T) {
	srv, _ := setupTestServer(t)
	boom := errors.New("disk I/O error")
	call := func(h http.HandlerFunc, target string) int {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(http.MethodGet, target, nil))
		return w.Code
	}
	svcWith := func(repo stubHistoryUserInfoRepo) {
		repo.InMemoryUserInfoRepo = v2.NewInMemoryUserInfoRepo()
		useUserInfoService(t, v2.NewUserInfoService(v2.UserInfoServiceConfig{Repo: repo, Logger: zap.NewNop()}))
	}

	svcWith(stubHistoryUserInfoRepo{today: "2026-10-06", listErr: boom})
	assert.Equal(t, http.StatusInternalServerError, call(srv.apiUserInfoHistory, "/api/v2/userinfo/history?site=hdsky"))
	assert.Equal(t, http.StatusInternalServerError, call(srv.apiUserInfoSummary, "/api/v2/userinfo/summary?range=7d"))
	assert.Equal(t, http.StatusInternalServerError, call(srv.apiUserInfoTrends, "/api/v2/userinfo/trends?days=3"))

	svcWith(stubHistoryUserInfoRepo{today: "2026-10-06", baselineErr: boom})
	assert.Equal(t, http.StatusInternalServerError, call(srv.apiUserInfoHistory, "/api/v2/userinfo/history?site=hdsky"))

	// 仓库的「今天」坏了：算不出区间
	svcWith(stubHistoryUserInfoRepo{today: "bad"})
	assert.Equal(t, http.StatusInternalServerError, call(srv.apiUserInfoHistory, "/api/v2/userinfo/history?site=hdsky"))
	assert.Equal(t, http.StatusBadRequest, call(srv.apiUserInfoSummary, "/api/v2/userinfo/summary"), "区间换算失败按参数错误回")
}

func TestDailyReportSettingsAPI_Errors(t *testing.T) {
	srv, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.NotificationConf{}))

	w := httptest.NewRecorder()
	srv.apiDailyReportSettings(w, httptest.NewRequest(http.MethodPut, "/api/v2/userinfo/daily-report", bytes.NewBufferString("{")))
	assert.Equal(t, http.StatusBadRequest, w.Code, "JSON 坏了")

	w = httptest.NewRecorder()
	srv.apiDailyReportSettings(w, httptest.NewRequest(http.MethodPut, "/api/v2/userinfo/daily-report", bytes.NewBufferString(`{"enabled":false,"time":"25:00"}`)))
	assert.Equal(t, http.StatusBadRequest, w.Code, "时刻格式错误")

	noStore := &Server{}
	w = httptest.NewRecorder()
	noStore.apiDailyReportSettings(w, httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/daily-report", nil))
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	require.NoError(t, global.GlobalDB.DB.Migrator().DropTable(&models.SettingsGlobal{}))
	w = httptest.NewRecorder()
	srv.apiDailyReportSettings(w, httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/daily-report", nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	assert.True(t, isDailyReportValidationError(core.ErrDailyReportNoChannel))
	assert.False(t, isDailyReportValidationError(errors.New("disk I/O error")))
}
