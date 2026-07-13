package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestHandleLoginStateConfigUpdate_ChannelIDs(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	ids := []uint{3, 7}
	body, _ := json.Marshal(SiteLoginConfigUpdateRequest{NotificationChannelIDs: &ids})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader(body))
	srv.handleLoginStateConfigUpdate(w, req, "hdsky")
	require.Equal(t, http.StatusOK, w.Code)

	state, err := loadOrInitLoginState(global.GlobalDB.DB, "hdsky")
	require.NoError(t, err)
	require.NotEmpty(t, state.NotificationChannelIDs)
}

func TestApiSiteDetail_TestReminderRoute(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/hdsky/login-state/test-reminder", nil)
	srv.apiSiteDetail(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiSiteDetail_ConfigRoute(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	mode := "manual"
	body, _ := json.Marshal(SiteLoginConfigUpdateRequest{ProbeMode: &mode})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/sites/hdsky/login-state/config", bytes.NewReader(body))
	srv.apiSiteDetail(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}
