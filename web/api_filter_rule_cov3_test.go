package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestApiFilterRuleDetail_GetDeleteSuccess(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	id := createFilterRuleForCov(t, server, "GDRule")

	t.Run("get success", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/filter-rules/1", nil)
		server.apiFilterRuleDetail(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("delete not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/filter-rules/999", nil)
		server.apiFilterRuleDetail(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("delete success", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/filter-rules/1", nil)
		server.apiFilterRuleDetail(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "deleted")
	})

	_ = id
}

func TestApiFilterRules_ListWithData(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.FilterRule{
		Name: "listrule", Pattern: "x*", PatternType: "wildcard", Enabled: true, Priority: 1,
	}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/filter-rules", nil)
	server.apiFilterRules(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp []FilterRuleResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, len(resp), 1)
}

func TestApiFilterRules_MethodNotAllowed(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/filter-rules", nil)
	server.apiFilterRules(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestUpdateFilterRule_MinMaxSize(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	id := createFilterRuleForCov(t, server, "SizeRule")
	body, _ := json.Marshal(FilterRuleRequest{
		Name: "SizeRule", Pattern: "foo*", PatternType: "wildcard",
		MinSizeGB: 1, MaxSizeGB: 50, Enabled: true, Priority: 2, RequireFree: true,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
	server.updateFilterRule(w, req, id)
	require.Equal(t, http.StatusOK, w.Code)
}
