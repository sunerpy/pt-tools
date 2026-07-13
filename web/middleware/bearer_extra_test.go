package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/middleware"
)

type errStore struct{}

func (errStore) Lookup(_ context.Context, _ string) (*models.BotToken, error) {
	return nil, errors.New("db down")
}

func TestRequireBearerStoreError(t *testing.T) {
	handler := middleware.RequireBearer(errStore{})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireBearerEmptyHashRejected(t *testing.T) {
	store := &mockBotTokenStore{tokens: map[string]*models.BotToken{
		"1": {ID: 1, Kind: "bearer", CodeOrTokenHash: "", Scope: "x"},
	}}
	handler := middleware.RequireBearer(store)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer whatever")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "empty hash → Lookup returns match via bcrypt mismatch → 401")
}

func TestGetIdentityNilWhenAbsent(t *testing.T) {
	assert.Nil(t, middleware.GetIdentity(context.Background()))
}

func TestRequireAuthNilStoreFallsBackToSession(t *testing.T) {
	sessionOK := func(_ http.ResponseWriter, _ *http.Request) bool { return true }
	handler := middleware.RequireAuth(nil, sessionOK)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "nil store → checkBearerQuiet false → session fallback")
}

func TestRequireAuthNonBearerHeaderSessionFallback(t *testing.T) {
	store := &mockBotTokenStore{tokens: map[string]*models.BotToken{}}
	sessionOK := func(_ http.ResponseWriter, _ *http.Request) bool { return true }
	handler := middleware.RequireAuth(store, sessionOK)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Basic xyz")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequireAuthEmptyBearerTokenSessionFallback(t *testing.T) {
	store := &mockBotTokenStore{tokens: map[string]*models.BotToken{}}
	called := false
	sessionOK := func(_ http.ResponseWriter, _ *http.Request) bool { called = true; return true }
	handler := middleware.RequireAuth(store, sessionOK)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequireAuthExpiredBearerSessionFallback(t *testing.T) {
	tok := "expired-tok"
	store := &mockBotTokenStore{tokens: map[string]*models.BotToken{
		"1": {ID: 1, Kind: "bearer", CodeOrTokenHash: hashToken(tok), Scope: "x", ExpiresAt: timePtr(time.Now().Add(-time.Hour))},
	}}
	sessionOK := func(_ http.ResponseWriter, _ *http.Request) bool { return true }
	handler := middleware.RequireAuth(store, sessionOK)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "expired bearer → quiet-check false → session fallback")
}

func TestRequireAuthEmptyScopeBearerSessionFallback(t *testing.T) {
	tok := "noscope-tok"
	store := &mockBotTokenStore{tokens: map[string]*models.BotToken{
		"1": {ID: 1, Kind: "bearer", CodeOrTokenHash: hashToken(tok), Scope: ""},
	}}
	sessionFail := func(_ http.ResponseWriter, _ *http.Request) bool { return false }
	handler := middleware.RequireAuth(store, sessionFail)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
