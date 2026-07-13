package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestHandleLoginStateConfigUpdate_Cov(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec, authedRequest(http.MethodGet, "/api/sites/login-state/hdsky/config", nil), "hdsky")
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("invalid cron", func(t *testing.T) {
		cron := "bad cron"
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec,
			authedRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", SiteLoginConfigUpdateRequest{ReminderCron: &cron}), "hdsky")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ban threshold out of range", func(t *testing.T) {
		bad := 9999
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec,
			authedRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", SiteLoginConfigUpdateRequest{BanThresholdDays: &bad}), "hdsky")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("remind before out of range", func(t *testing.T) {
		bad := 9999
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec,
			authedRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", SiteLoginConfigUpdateRequest{RemindBeforeDays: &bad}), "hdsky")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("invalid probe mode", func(t *testing.T) {
		bad := "bogus"
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec,
			authedRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", SiteLoginConfigUpdateRequest{ProbeMode: &bad}), "hdsky")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("no fields provided", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec,
			authedRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", SiteLoginConfigUpdateRequest{}), "hdsky")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid multi-field update", func(t *testing.T) {
		ban := 45
		remind := 15
		cron := "0 9 * * *"
		mode := "manual"
		chans := []uint{1, 2}
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec,
			authedRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", SiteLoginConfigUpdateRequest{
				BanThresholdDays: &ban, RemindBeforeDays: &remind, ReminderCron: &cron,
				ProbeMode: &mode, NotificationChannelIDs: &chans,
			}), "hdsky")
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestLoginHandler_JSONSuccess(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("admin", hashPassword("secret")))

	rec := httptest.NewRecorder()
	req := authedRequest(http.MethodPost, "/login", map[string]string{"username": "admin", "password": "secret"})
	req.Header.Set("Content-Type", "application/json")
	srv.loginHandler(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "success")
}

func TestParseVisitTimestamp_Cov(t *testing.T) {
	_, err := parseVisitTimestamp("")
	assert.Error(t, err)
	_, err = parseVisitTimestamp("garbage")
	assert.Error(t, err)
	ts, err := parseVisitTimestamp("2024-01-02T03:04:05Z")
	require.NoError(t, err)
	assert.Equal(t, 2024, ts.Year())
}

func TestApiSiteLoginStateList_NilDB(t *testing.T) {
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })

	srv := &Server{sessions: map[string]string{"sess-test": "admin"}}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateList(rec, authedRequest(http.MethodGet, "/api/sites/login-state", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
