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
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

type loginCovResolver struct{}

func (loginCovResolver) Resolve(models.SiteSetting) (*v2.SiteDefinition, v2.Site, error) {
	return &v2.SiteDefinition{ID: "hdsky"}, &fakeV2Site{id: "hdsky", name: "HDSky"}, nil
}

type loginCovDecryptor struct{}

func (loginCovDecryptor) Decrypt(models.SiteSetting) (string, error) { return "cookie=1", nil }

func newLoginMonitorServer(t *testing.T) *Server {
	t.Helper()
	srv, cleanup := newSiteLoginTestServer(t)
	t.Cleanup(cleanup)
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	mon := scheduler.NewLoginReminderMonitor(scheduler.LoginReminderConfig{
		DB:        global.GlobalDB.DB,
		Resolver:  loginCovResolver{},
		Decryptor: loginCovDecryptor{},
	})
	mgr.SetLoginReminderMonitor(mon)
	srv.mgr = mgr
	return srv
}

func TestApiSiteLoginStateList_WithSites(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", DisplayName: "HDSky", Enabled: true}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state", nil)
		srv.apiSiteLoginStateList(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("list synthesizes defaults", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state", nil)
		srv.apiSiteLoginStateList(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var out []SiteLoginStateResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		require.Len(t, out, 1)
		assert.Equal(t, "hdsky", out[0].SiteName)
	})
}

func TestHandleLoginStateProbe_Success(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/hdsky/probe", nil)
		srv.handleLoginStateProbe(w, req, "hdsky")
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("unknown site 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/nope/probe", nil)
		srv.handleLoginStateProbe(w, req, "nope")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("probe runs", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/hdsky/probe", nil)
		srv.handleLoginStateProbe(w, req, "hdsky")
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestHandleLoginStateTestReminder_Paths(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/hdsky/test-reminder", nil)
		srv.handleLoginStateTestReminder(w, req, "hdsky")
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("unknown site 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/nope/test-reminder", nil)
		srv.handleLoginStateTestReminder(w, req, "nope")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("router nil returns 500", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/hdsky/test-reminder", nil)
		srv.handleLoginStateTestReminder(w, req, "hdsky")
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandleLoginStateConfigUpdate_Success(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	ban := 40
	remind := 12
	cron := "0 8 * * *"
	mode := "manual"
	ids := []uint{1, 2}
	body, _ := json.Marshal(SiteLoginConfigUpdateRequest{
		BanThresholdDays:       &ban,
		RemindBeforeDays:       &remind,
		ReminderCron:           &cron,
		NotificationChannelIDs: &ids,
		ProbeMode:              &mode,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader(body))
	srv.handleLoginStateConfigUpdate(w, req, "hdsky")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestHandleLoginStateConfigUpdate_Validation(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/hdsky/config", nil)
		srv.handleLoginStateConfigUpdate(w, req, "hdsky")
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("bad cron", func(t *testing.T) {
		bad := "not a cron"
		body, _ := json.Marshal(SiteLoginConfigUpdateRequest{ReminderCron: &bad})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader(body))
		srv.handleLoginStateConfigUpdate(w, req, "hdsky")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ban out of range", func(t *testing.T) {
		bad := 999
		body, _ := json.Marshal(SiteLoginConfigUpdateRequest{BanThresholdDays: &bad})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader(body))
		srv.handleLoginStateConfigUpdate(w, req, "hdsky")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad probe mode", func(t *testing.T) {
		bad := "weird"
		body, _ := json.Marshal(SiteLoginConfigUpdateRequest{ProbeMode: &bad})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader(body))
		srv.handleLoginStateConfigUpdate(w, req, "hdsky")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no fields", func(t *testing.T) {
		body, _ := json.Marshal(SiteLoginConfigUpdateRequest{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader(body))
		srv.handleLoginStateConfigUpdate(w, req, "hdsky")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader([]byte(`{bad`)))
		srv.handleLoginStateConfigUpdate(w, req, "hdsky")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

var _ = context.Background
