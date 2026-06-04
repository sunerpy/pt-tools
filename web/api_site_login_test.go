package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func newSiteLoginTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteLoginState{}))

	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}

	srv := &Server{
		sessions: map[string]string{"sess-test": "admin"},
	}
	cleanup := func() { global.GlobalDB = prevDB }
	return srv, cleanup
}

func authedRequest(method, path string, body any) *http.Request {
	var reader *bytes.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		reader = bytes.NewReader(buf)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
	return req
}

func TestApiSiteLoginStateListEmpty(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateList(rec, authedRequest(http.MethodGet, "/api/sites/login-state", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var out []SiteLoginStateResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Empty(t, out)
}

func TestApiSiteLoginStateListIncludesAllSites(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", DisplayName: "HDSky", BaseURL: "https://hdsky.me/", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "MTEAM", DisplayName: "M-Team", BaseURL: "https://kp.m-team.cc/", Enabled: false}).Error)

	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateList(rec, authedRequest(http.MethodGet, "/api/sites/login-state", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var out []SiteLoginStateResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out, 2)

	for _, row := range out {
		assert.NotEmpty(t, row.SiteName)
		assert.NotZero(t, row.BanThresholdDays, "default ban threshold should populate")
		assert.NotZero(t, row.RemindBeforeDays)
		raw, _ := json.Marshal(row)
		body := strings.ToLower(string(raw))
		assert.NotContains(t, body, "\"cookie\":", "response must not leak cookie field")
		assert.NotContains(t, body, "\"cookie_encrypted\":", "response must not leak cookie_encrypted")
	}
}

func TestApiSiteLoginStateConfigUpdateSuccess(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	body := SiteLoginConfigUpdateRequest{
		BanThresholdDays:       intPtr(60),
		RemindBeforeDays:       intPtr(20),
		ReminderCron:           strPtr("0 8,20 * * *"),
		NotificationChannelIDs: uintSlicePtr([]uint{1, 3}),
	}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodPut, "/api/sites/login-state/HDSKY/config", body))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	assert.Equal(t, 60, state.BanThresholdDays)
	assert.Equal(t, 20, state.RemindBeforeDays)
	assert.Equal(t, "0 8,20 * * *", state.ReminderCron)
	assert.Equal(t, "[1,3]", state.NotificationChannelIDs)
}

func TestApiSiteLoginStateConfigUpdateRejectsInvalidCron(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	body := SiteLoginConfigUpdateRequest{ReminderCron: strPtr("invalid cron")}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodPut, "/api/sites/login-state/HDSKY/config", body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "cron")
}

func TestApiSiteLoginStateConfigUpdateRejectsBadBounds(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	body := SiteLoginConfigUpdateRequest{BanThresholdDays: intPtr(0)}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodPut, "/api/sites/login-state/HDSKY/config", body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestApiSiteVisitClampsFutureTimestamp(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	body := SiteVisitReportRequest{SiteName: "HDSKY", LastVisitAt: future}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodPost, "/api/sites/visit", body))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	require.NotNil(t, state.LastVisitAt)
	assert.True(t, state.LastVisitAt.Before(time.Now().Add(time.Minute)),
		"server must clamp future timestamp to now (got %s)", state.LastVisitAt)
}

func TestApiSiteVisitRejectsBadTimestamp(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	body := SiteVisitReportRequest{SiteName: "HDSKY", LastVisitAt: "not-a-date"}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodPost, "/api/sites/visit", body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestApiSiteVisitRejectsEmptySiteName(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	body := SiteVisitReportRequest{SiteName: "", LastVisitAt: time.Now().UTC().Format(time.RFC3339)}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodPost, "/api/sites/visit", body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestApiSiteLoginStateGetSingle(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", DisplayName: "HDSky", Enabled: true}).Error)

	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodGet, "/api/sites/login-state/HDSKY", nil))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var resp SiteLoginStateResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "HDSKY", resp.SiteName)
	assert.Equal(t, "HDSky", resp.DisplayName)
}

func TestParseChannelIDs(t *testing.T) {
	cases := []struct {
		raw  string
		want []uint
	}{
		{"", []uint{}},
		{"[]", []uint{}},
		{"[1,3,5]", []uint{1, 3, 5}},
		{"not json", []uint{}},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			assert.Equal(t, tc.want, parseChannelIDs(tc.raw))
		})
	}
}

func TestApiSiteLoginStateListProbeModeSynthesizedDefault(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "testsite", Enabled: true}).Error)

	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateList(rec, authedRequest(http.MethodGet, "/api/sites/login-state", nil))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var out []SiteLoginStateResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out, 1)
	require.Equal(t, "testsite", out[0].SiteName)
	assert.Equal(t, "auto", out[0].ProbeMode, "synthesized SiteLoginState must have ProbeMode='auto', not ''")
}

func intPtr(v int) *int             { return &v }
func strPtr(v string) *string       { return &v }
func uintSlicePtr(v []uint) *[]uint { return &v }
