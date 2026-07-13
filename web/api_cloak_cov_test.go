package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/cloakdriver"
)

func TestApiCloakConfig_MethodDispatch(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	t.Run("unsupported method", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiCloakConfig(rec, cloakAuthedReq(http.MethodDelete, "/api/cloak/config", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("get with nil store", func(t *testing.T) {
		nilSrv := &Server{}
		rec := httptest.NewRecorder()
		nilSrv.handleCloakConfigGet(rec, cloakAuthedReq(http.MethodGet, "/api/cloak/config", nil))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})
}

func TestHandleCloakConfigPut_BadInput(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	t.Run("nil store", func(t *testing.T) {
		nilSrv := &Server{}
		rec := httptest.NewRecorder()
		nilSrv.handleCloakConfigPut(rec, cloakAuthedReq(http.MethodPut, "/api/cloak/config", cloakConfigPutRequest{Endpoint: "x"}))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("empty endpoint", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleCloakConfigPut(rec, cloakAuthedReq(http.MethodPut, "/api/cloak/config", cloakConfigPutRequest{Endpoint: "  "}))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid put", func(t *testing.T) {
		tok := "tok"
		rec := httptest.NewRecorder()
		srv.handleCloakConfigPut(rec, cloakAuthedReq(http.MethodPut, "/api/cloak/config", cloakConfigPutRequest{Endpoint: "http://m:8080", Token: &tok}))
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestApiCloakTest_BadInput(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiCloakTest(rec, cloakAuthedReq(http.MethodGet, "/api/cloak/test", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("nil store", func(t *testing.T) {
		nilSrv := &Server{}
		rec := httptest.NewRecorder()
		nilSrv.apiCloakTest(rec, cloakAuthedReq(http.MethodPost, "/api/cloak/test", nil))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("no endpoint configured returns unknown category", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiCloakTest(rec, cloakAuthedReq(http.MethodPost, "/api/cloak/test", cloakTestRequest{}))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), cloakCatUnknown)
	})
}

func TestClassifyCloakTestResult(t *testing.T) {
	tests := []struct {
		name    string
		info    *cloakdriver.ManagerStatusInfo
		err     error
		wantCat string
	}{
		{"success", &cloakdriver.ManagerStatusInfo{Version: "1.2.3"}, nil, cloakCatSuccess},
		{"success nil info", nil, nil, cloakCatSuccess},
		{"auth fail", nil, cloakdriver.ErrManagerAuthFailed, cloakCatAuthFail},
		{"not found", nil, cloakdriver.ErrManagerNotFound, cloakCatNotFound},
		{"server error", nil, cloakdriver.ErrManagerServerError, cloakCatServerError},
		{"dns fail", nil, cloakdriver.ErrManagerDNSFailed, cloakCatDNSFail},
		{"conn refused", nil, cloakdriver.ErrManagerConnRefused, cloakCatConnRefused},
		{"timeout", nil, cloakdriver.ErrManagerTimeout, cloakCatTimeout},
		{"protocol error", nil, cloakdriver.ErrManagerProtocolError, cloakCatProtocolError},
		{"unknown", nil, errors.New("boom"), cloakCatUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := classifyCloakTestResult(tt.info, tt.err)
			assert.Equal(t, tt.wantCat, resp.Category)
		})
	}

	t.Run("success carries version", func(t *testing.T) {
		resp := classifyCloakTestResult(&cloakdriver.ManagerStatusInfo{Version: "9.9"}, nil)
		assert.Equal(t, "9.9", resp.ManagerVersion)
	})
}
