package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/models"
)

func TestApiRSSFilterAssociation_Router(t *testing.T) {
	db := setupTestDB(t)
	server := NewServer(core.NewConfigStore(db), nil)

	rss := models.RSSSubscription{Name: "r1", URL: "http://e/rss", IntervalMinutes: 10}
	require.NoError(t, db.DB.Create(&rss).Error)
	rule := models.FilterRule{Name: "rule1", Pattern: ".*", PatternType: "regex", Enabled: true, Priority: 1}
	require.NoError(t, db.DB.Create(&rule).Error)

	t.Run("invalid path", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/rss/1/other", nil)
		server.apiRSSFilterAssociation(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid rss id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/rss/abc/filter-rules", nil)
		server.apiRSSFilterAssociation(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/rss/1/filter-rules", nil)
		server.apiRSSFilterAssociation(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("get via router", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/rss/1/filter-rules", nil)
		server.apiRSSFilterAssociation(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("put via router associates rule", func(t *testing.T) {
		body, _ := json.Marshal(RSSFilterAssociationRequest{FilterRuleIDs: []uint{rule.ID}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/rss/1/filter-rules", bytes.NewReader(body))
		server.apiRSSFilterAssociation(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp RSSFilterAssociationResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp.FilterRuleIDs, rule.ID)
	})

	t.Run("get missing rss", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/rss/999/filter-rules", nil)
		server.apiRSSFilterAssociation(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
