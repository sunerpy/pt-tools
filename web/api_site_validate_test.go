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

func TestApiSiteValidate_Branches(t *testing.T) {
	s := &Server{}

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/validate", nil)
		s.apiSiteValidate(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/validate", bytes.NewBufferString(`{bad`))
		s.apiSiteValidate(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	cases := []struct {
		name      string
		req       SiteValidationRequest
		wantValid bool
	}{
		{"empty name", SiteValidationRequest{}, false},
		{"empty auth", SiteValidationRequest{Name: "hdsky"}, false},
		{"cookie missing", SiteValidationRequest{Name: "hdsky", AuthMethod: "cookie"}, false},
		{"cookie ok", SiteValidationRequest{Name: "hdsky", AuthMethod: "cookie", Cookie: "c=1"}, true},
		{"api_key missing", SiteValidationRequest{Name: "mteam", AuthMethod: "api_key"}, false},
		{"api_key ok", SiteValidationRequest{Name: "mteam", AuthMethod: "api_key", APIKey: "k"}, true},
		{"cookie_and_api_key missing", SiteValidationRequest{Name: "x", AuthMethod: "cookie_and_api_key", Cookie: "c"}, false},
		{"cookie_and_api_key ok", SiteValidationRequest{Name: "x", AuthMethod: "cookie_and_api_key", Cookie: "c", APIKey: "k"}, true},
		{"passkey missing", SiteValidationRequest{Name: "x", AuthMethod: "passkey"}, false},
		{"passkey ok", SiteValidationRequest{Name: "x", AuthMethod: "passkey", Passkey: "p"}, true},
		{"rss_passkey", SiteValidationRequest{Name: "x", AuthMethod: "rss_passkey"}, true},
		{"unknown auth", SiteValidationRequest{Name: "x", AuthMethod: "bogus"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.req)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/sites/validate", bytes.NewReader(body))
			s.apiSiteValidate(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp SiteValidationResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, tc.wantValid, resp.Valid)
		})
	}
}
