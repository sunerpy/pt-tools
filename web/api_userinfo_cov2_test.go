package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApiUserInfoSync_AllAndSpecific(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	t.Cleanup(func() { InitUserInfoService(nil) })

	s := &Server{}

	t.Run("sync all sites", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sync", nil)
		s.apiUserInfoSync(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp SyncResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.GreaterOrEqual(t, len(resp.Success), 1)
	})

	t.Run("sync specific site", func(t *testing.T) {
		body, _ := json.Marshal(SyncRequest{Sites: []string{"site1"}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sync", bytes.NewReader(body))
		s.apiUserInfoSync(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp SyncResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp.Success, "site1")
	})

	t.Run("sync unknown site records failure", func(t *testing.T) {
		body, _ := json.Marshal(SyncRequest{Sites: []string{"no-such-site"}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sync", bytes.NewReader(body))
		s.apiUserInfoSync(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp SyncResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.GreaterOrEqual(t, len(resp.Failed), 1)
	})

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sync", bytes.NewBufferString(`{bad`))
		s.apiUserInfoSync(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/sync", nil)
		s.apiUserInfoSync(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiUserInfoSiteDetail_PostSyncError(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	t.Cleanup(func() { InitUserInfoService(nil) })

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sites/no-such-site", nil)
	s.apiUserInfoSiteDetail(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
