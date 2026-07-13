package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

func TestHandleLoginStateGet_Cov(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "hdsky", DisplayName: "HDSky", Enabled: true, BaseURL: "https://hdsky.me",
	}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleLoginStateGet(rec, authedRequest(http.MethodPost, "/api/sites/hdsky/login-state", nil), "hdsky")
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("site not found", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleLoginStateGet(rec, authedRequest(http.MethodGet, "/api/sites/nosuch/login-state", nil), "nosuch")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("returns state for existing site", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleLoginStateGet(rec, authedRequest(http.MethodGet, "/api/sites/hdsky/login-state", nil), "hdsky")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "hdsky")
	})
}

func TestHandleLoginStateTestReminder_Cov(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	t.Run("method not allowed", func(t *testing.T) {
		srv.mgr = nil
		rec := httptest.NewRecorder()
		srv.handleLoginStateTestReminder(rec, authedRequest(http.MethodGet, "/api/sites/hdsky/login-state/test-reminder", nil), "hdsky")
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("monitor not initialized", func(t *testing.T) {
		mgr := scheduler.NewManager()
		t.Cleanup(func() { mgr.StopAll() })
		srv.mgr = mgr
		rec := httptest.NewRecorder()
		srv.handleLoginStateTestReminder(rec, authedRequest(http.MethodPost, "/api/sites/hdsky/login-state/test-reminder", nil), "hdsky")
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})
}

func TestBuildLoginStateResponse_Cov(t *testing.T) {
	now := time.Now()
	past := now.Add(-24 * time.Hour)

	site := models.SiteSetting{Name: "hdsky", DisplayName: "HDSky", Enabled: true, BaseURL: "https://hdsky.me"}

	t.Run("with active timestamps computes tier", func(t *testing.T) {
		state := models.SiteLoginState{
			SiteName:         "hdsky",
			LastAccessAt:     &past,
			BanThresholdDays: 30,
			RemindBeforeDays: 10,
		}
		resp := buildLoginStateResponse(site, state, now)
		assert.Equal(t, "hdsky", resp.SiteName)
		require.NotNil(t, resp.EffectiveLastActiveAt)
		assert.NotEqual(t, "unknown", resp.Tier)
	})

	t.Run("no active timestamp yields unknown tier", func(t *testing.T) {
		state := models.SiteLoginState{SiteName: "hdsky", BanThresholdDays: 30, RemindBeforeDays: 10}
		resp := buildLoginStateResponse(site, state, now)
		assert.Equal(t, "unknown", resp.Tier)
		assert.Nil(t, resp.EffectiveLastActiveAt)
	})
}

func TestTimestampPtr(t *testing.T) {
	assert.Nil(t, timestampPtr(nil))
	zero := time.Time{}
	assert.Nil(t, timestampPtr(&zero))
	now := time.Now()
	got := timestampPtr(&now)
	require.NotNil(t, got)
	assert.Equal(t, now.Unix(), *got)
}
