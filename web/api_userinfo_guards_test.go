package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserInfoHandlers_ServiceAndMethodGuards(t *testing.T) {
	InitUserInfoService(nil)
	s := &Server{}

	cases := []struct {
		name       string
		handler    http.HandlerFunc
		method     string
		path       string
		wantStatus int
	}{
		{"sites method", s.apiUserInfoSites, http.MethodPost, "/api/v2/userinfo/sites", http.StatusMethodNotAllowed},
		{"sites no service", s.apiUserInfoSites, http.MethodGet, "/api/v2/userinfo/sites", http.StatusServiceUnavailable},
		{"registered method", s.apiUserInfoRegisteredSites, http.MethodPost, "/api/v2/userinfo/registered", http.StatusMethodNotAllowed},
		{"registered no service", s.apiUserInfoRegisteredSites, http.MethodGet, "/api/v2/userinfo/registered", http.StatusServiceUnavailable},
		{"sync method", s.apiUserInfoSync, http.MethodGet, "/api/v2/userinfo/sync", http.StatusMethodNotAllowed},
		{"sync no service", s.apiUserInfoSync, http.MethodPost, "/api/v2/userinfo/sync", http.StatusServiceUnavailable},
		{"clearcache method", s.apiUserInfoClearCache, http.MethodGet, "/api/v2/userinfo/cache/clear", http.StatusMethodNotAllowed},
		{"clearcache no service", s.apiUserInfoClearCache, http.MethodPost, "/api/v2/userinfo/cache/clear", http.StatusServiceUnavailable},
		{"detail empty site", s.apiUserInfoSiteDetail, http.MethodGet, "/api/v2/userinfo/sites/", http.StatusBadRequest},
		{"detail no service", s.apiUserInfoSiteDetail, http.MethodGet, "/api/v2/userinfo/sites/site1", http.StatusServiceUnavailable},
		{"aggregated method", s.apiUserInfoAggregated, http.MethodPost, "/api/v2/userinfo/aggregated", http.StatusMethodNotAllowed},
		{"aggregated no service", s.apiUserInfoAggregated, http.MethodGet, "/api/v2/userinfo/aggregated", http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			tc.handler(w, req)
			assert.Equal(t, tc.wantStatus, w.Code)
		})
	}
}

func TestUserInfoSiteDetail_UnsupportedMethod(t *testing.T) {
	svc := setupTestUserInfoService()
	InitUserInfoService(svc)
	t.Cleanup(func() { InitUserInfoService(nil) })

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v2/userinfo/sites/site1", nil)
	s.apiUserInfoSiteDetail(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestApiVersionCheck(t *testing.T) {
	s := &Server{}

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/version/check", nil)
		s.apiVersionCheck(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("returns result or error", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/version/check?proxy=http://127.0.0.1:1", nil)
		s.apiVersionCheck(w, req)
		// Network likely fails in test env; either a JSON body or 500 is acceptable.
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
	})
}
