package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginHandler_LegacyPasswordUpgrade(t *testing.T) {
	srv := setupServer(t)

	h := sha256.Sum256([]byte("legacypass"))
	legacyHash := hex.EncodeToString(h[:])
	require.NoError(t, srv.store.EnsureAdmin("legacyok", legacyHash))

	body, _ := json.Marshal(map[string]string{"username": "legacyok", "password": "legacypass"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.loginHandler(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Result().Cookies())
}

func TestVerifyLegacyPassword_Direct(t *testing.T) {
	assert.False(t, verifyLegacyPassword("has|pipe", "x"))
	assert.True(t, verifyLegacyPassword("plain", "plain"))
	h := sha256.Sum256([]byte("secret"))
	assert.True(t, verifyLegacyPassword(hex.EncodeToString(h[:]), "secret"))
	assert.False(t, verifyLegacyPassword("nomatch", "secret"))
}
