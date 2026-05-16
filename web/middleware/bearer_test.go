package middleware_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// mockBotTokenStore 用于测试的 mock store 实现。
type mockBotTokenStore struct {
	tokens map[string]*models.BotToken
}

func (m *mockBotTokenStore) Lookup(ctx context.Context, plainToken string) (*models.BotToken, error) {
	// 遍历所有 tokens，bcrypt 比对明文 token 与 hash
	for _, tok := range m.tokens {
		if err := bcrypt.CompareHashAndPassword([]byte(tok.CodeOrTokenHash), []byte(plainToken)); err == nil {
			// 找到匹配的 token
			return tok, nil
		}
	}
	return nil, nil
}

func hashToken(plainToken string) string {
	hash, _ := bcrypt.GenerateFromPassword([]byte(plainToken), bcrypt.DefaultCost)
	return string(hash)
}

func timePtr(t time.Time) *time.Time {
	return &t
}

// TestRequireBearer_Valid 合法 token → 200 + Identity 注入 ctx
func TestRequireBearer_Valid(t *testing.T) {
	validToken := "tok_valid_abcd1234"
	hash := hashToken(validToken)

	store := &mockBotTokenStore{
		tokens: map[string]*models.BotToken{
			"1": {
				ID:              1,
				Kind:            "bearer",
				CodeOrTokenHash: hash,
				Scope:           "chatops:*",
				ExpiresAt:       nil,
				CreatedAt:       time.Now(),
			},
		},
	}

	bearer := middleware.RequireBearer(store)
	handler := bearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity := middleware.GetIdentity(r.Context())
		assert.NotNil(t, identity)
		assert.Equal(t, "bearer", identity.TokenKind)
		assert.Equal(t, "chatops:*", identity.Scope)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "OK", rec.Body.String())
}

// TestRequireBearer_MissingHeader 缺失 header → 401 JSON
func TestRequireBearer_MissingHeader(t *testing.T) {
	store := &mockBotTokenStore{tokens: map[string]*models.BotToken{}}

	bearer := middleware.RequireBearer(store)
	handler := bearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	// 不设置 Authorization header
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "unauthorized")
}

// TestRequireBearer_InvalidToken 无效 token → 401 JSON
func TestRequireBearer_InvalidToken(t *testing.T) {
	store := &mockBotTokenStore{tokens: map[string]*models.BotToken{}}

	bearer := middleware.RequireBearer(store)
	handler := bearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer tok_nonexistent")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "unauthorized")
}

// TestRequireBearer_ExpiredToken 已过期 token → 401 JSON
func TestRequireBearer_ExpiredToken(t *testing.T) {
	expiredToken := "tok_expired_xyz"
	hash := hashToken(expiredToken)
	expiredTime := time.Now().Add(-1 * time.Hour)

	store := &mockBotTokenStore{
		tokens: map[string]*models.BotToken{
			"1": {
				ID:              1,
				Kind:            "bearer",
				CodeOrTokenHash: hash,
				Scope:           "chatops:*",
				ExpiresAt:       timePtr(expiredTime),
				CreatedAt:       time.Now().Add(-2 * time.Hour),
			},
		},
	}

	bearer := middleware.RequireBearer(store)
	handler := bearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "unauthorized")
}

// TestRequireBearer_ScopeInsufficient scope 不足（此版本仅检查非空）→ 401 JSON
// 注：当前实现仅检查 scope 非空；细粒度权限检查留待 T25+
func TestRequireBearer_EmptyScope(t *testing.T) {
	validToken := "tok_no_scope"
	hash := hashToken(validToken)

	store := &mockBotTokenStore{
		tokens: map[string]*models.BotToken{
			"1": {
				ID:              1,
				Kind:            "bearer",
				CodeOrTokenHash: hash,
				Scope:           "", // 空 scope
				ExpiresAt:       nil,
				CreatedAt:       time.Now(),
			},
		},
	}

	bearer := middleware.RequireBearer(store)
	handler := bearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "unauthorized")
}

// TestRequireBearer_InvalidAuthHeaderFormat 无效的 Authorization header 格式 → 401 JSON
func TestRequireBearer_InvalidAuthHeaderFormat(t *testing.T) {
	store := &mockBotTokenStore{tokens: map[string]*models.BotToken{}}

	bearer := middleware.RequireBearer(store)
	handler := bearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic somebase64") // 错误的认证方案
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "unauthorized")
}

// TestRequireAuth_SessionFallback Bearer 失败但 Session cookie 存在 → 302 / 200（保持 session 行为）
func TestRequireAuth_SessionFallback(t *testing.T) {
	store := &mockBotTokenStore{tokens: map[string]*models.BotToken{}}

	// Mock sessionChecker：检查 "session" cookie
	sessionChecker := func(w http.ResponseWriter, r *http.Request) bool {
		cookie, err := r.Cookie("session")
		if err != nil {
			return false
		}
		return cookie.Value == "valid_session_id"
	}

	requireAuth := middleware.RequireAuth(store, sessionChecker)
	handler := requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Authorized")
	}))

	// 测试：有 session cookie，无 bearer
	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "valid_session_id"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "Authorized", rec.Body.String())
}

// TestRequireAuth_BearerPriority Bearer token 优先于 Session
func TestRequireAuth_BearerPriority(t *testing.T) {
	validToken := "tok_bearer_priority"
	hash := hashToken(validToken)

	store := &mockBotTokenStore{
		tokens: map[string]*models.BotToken{
			"1": {
				ID:              1,
				Kind:            "bearer",
				CodeOrTokenHash: hash,
				Scope:           "chatops:*",
				ExpiresAt:       nil,
				CreatedAt:       time.Now(),
			},
		},
	}

	sessionChecker := func(w http.ResponseWriter, r *http.Request) bool {
		// 不应被调用，因为 Bearer 已通过
		t.Error("sessionChecker should not be called when Bearer is valid")
		return false
	}

	requireAuth := middleware.RequireAuth(store, sessionChecker)
	handler := requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Bearer OK")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "Bearer OK", rec.Body.String())
}

// TestRequireAuth_NoAuth 两种认证都失败 → 401 JSON
func TestRequireAuth_NoAuth(t *testing.T) {
	store := &mockBotTokenStore{tokens: map[string]*models.BotToken{}}

	sessionChecker := func(w http.ResponseWriter, r *http.Request) bool {
		return false // Session 检查失败
	}

	requireAuth := middleware.RequireAuth(store, sessionChecker)
	handler := requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	// 不设置 Bearer 也不设置 Session cookie
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "unauthorized")
}

// TestGetIdentity 从 context 读取 BotIdentity
func TestGetIdentity(t *testing.T) {
	// 为了真正测试，应该通过 RequireBearer 中间件
	validToken := "tok_for_ctx_test"
	hash := hashToken(validToken)

	store := &mockBotTokenStore{
		tokens: map[string]*models.BotToken{
			"1": {
				ID:              1,
				Kind:            "bearer",
				CodeOrTokenHash: hash,
				Scope:           "chatops:*",
				ExpiresAt:       nil,
				CreatedAt:       time.Now(),
			},
		},
	}

	bearer := middleware.RequireBearer(store)
	identityFromCtx := (*middleware.BotIdentity)(nil)

	handler := bearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identityFromCtx = middleware.GetIdentity(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.NotNil(t, identityFromCtx)
	assert.Equal(t, "bearer", identityFromCtx.TokenKind)
	assert.Equal(t, "chatops:*", identityFromCtx.Scope)
}

// TestRequireBearer_EmptyToken "Bearer " 后为空 → 401 JSON
func TestRequireBearer_EmptyToken(t *testing.T) {
	store := &mockBotTokenStore{tokens: map[string]*models.BotToken{}}

	bearer := middleware.RequireBearer(store)
	handler := bearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer ") // "Bearer " 后无 token
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "unauthorized")
}
