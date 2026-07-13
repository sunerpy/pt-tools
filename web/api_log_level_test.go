package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
)

// ==== merged from api_log_level_cov_test.go ====
func TestApiLogLevel_Get(t *testing.T) {
	global.InitLogger(zap.NewNop())
	s := &Server{}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/log-level", nil)
	s.apiLogLevel(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp LogLevelResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Level)
	assert.ElementsMatch(t, []string{"debug", "info", "warn", "error"}, resp.Levels)
}

func TestApiLogLevel_Set(t *testing.T) {
	global.InitLogger(zap.NewNop())
	s := &Server{}

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantLevel  string
	}{
		{"set debug", `{"level":"debug"}`, http.StatusOK, "debug"},
		{"set info", `{"level":"info"}`, http.StatusOK, "info"},
		{"set warn", `{"level":"warn"}`, http.StatusOK, "warn"},
		{"invalid level", `{"level":"bogus"}`, http.StatusBadRequest, ""},
		{"malformed json", `{bad`, http.StatusBadRequest, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/log-level", bytes.NewBufferString(tt.body))
			s.apiLogLevel(w, req)
			require.Equal(t, tt.wantStatus, w.Code)

			if tt.wantStatus == http.StatusOK {
				var resp LogLevelResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantLevel, resp.Level)
				assert.Equal(t, "日志级别已更新", resp.Message)
			}
		})
	}
}

func TestApiLogLevel_MethodNotAllowed(t *testing.T) {
	global.InitLogger(zap.NewNop())
	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/log-level", nil)
	s.apiLogLevel(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
