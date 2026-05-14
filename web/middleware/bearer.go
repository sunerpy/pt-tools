package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// BotIdentity 注入到 context 的用户身份信息。
type BotIdentity struct {
	TokenKind string // "bind_code" 或 "bearer"（api_token）
	Scope     string // 权限范围，如 "chatops:*" 或 "mcp:read"
	ExpiresAt *time.Time
}

// BotTokenStore 定义 Bearer Token 查询接口。
// 实现由 T13 audit/store 任务完成；此任务使用 mock 实现测试。
type BotTokenStore interface {
	// Lookup 按明文 token 查询对应的 bcrypt hash 和其他属性。
	// 返回 (*BotToken, error)；token 不存在时返回 (*BotToken, nil)。
	Lookup(ctx context.Context, plainToken string) (*models.BotToken, error)
}

// contextKey 用于在 context 中存储 BotIdentity。
type contextKey string

const identityContextKey contextKey = "bot_identity"

// RequireBearer 创建 Bearer Token 验证中间件。
// 期望 Authorization 头格式为 "Bearer <token>"。
// 合法 token 会注入 BotIdentity 到 context；非法 token 返回 401 JSON。
func RequireBearer(store BotTokenStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				respondUnauthorized(w, "missing authorization header")
				return
			}

			// 解析 "Bearer <token>" 格式
			const bearerPrefix = "Bearer "
			if !strings.HasPrefix(authHeader, bearerPrefix) {
				respondUnauthorized(w, "invalid authorization header format")
				return
			}

			token := strings.TrimPrefix(authHeader, bearerPrefix)
			if token == "" {
				respondUnauthorized(w, "empty token")
				return
			}

			// 从 store 查询 token
			botToken, err := store.Lookup(r.Context(), token)
			if err != nil {
				if global.GetLogger() != nil {
					global.GetSlogger().Warnf("bearer_token_lookup_error token_hash_len=%d err=%v", len(token), err)
				}
				respondUnauthorized(w, "token lookup failed")
				return
			}

			// Token 不存在或已过期
			if botToken == nil {
				respondUnauthorized(w, "token not found or invalid")
				return
			}

			// 检查过期
			if botToken.ExpiresAt != nil && time.Now().After(*botToken.ExpiresAt) {
				respondUnauthorized(w, "token expired")
				return
			}

			// Bcrypt 比对（mock store 可能直接返回已比对的结果，此处作为防御层）
			// 若 store 返回的 CodeOrTokenHash 为空则视为无效
			if botToken.CodeOrTokenHash == "" {
				respondUnauthorized(w, "token hash empty")
				return
			}

			// 比对明文 token 与 bcrypt hash
			if err := bcrypt.CompareHashAndPassword([]byte(botToken.CodeOrTokenHash), []byte(token)); err != nil {
				if global.GetLogger() != nil {
					global.GetSlogger().Warnf("bearer_token_bcrypt_mismatch err=%v", err)
				}
				respondUnauthorized(w, "token mismatch")
				return
			}

			// Scope 检查（当前仅检查非空；T25+ 会实现细粒度权限检查）
			if botToken.Scope == "" {
				respondUnauthorized(w, "token scope empty")
				return
			}

			// 注入 BotIdentity 到 context
			identity := &BotIdentity{
				TokenKind: botToken.Kind,
				Scope:     botToken.Scope,
				ExpiresAt: botToken.ExpiresAt,
			}
			ctx := context.WithValue(r.Context(), identityContextKey, identity)

			// 更新 UsedAt 时间戳（异步，不阻塞请求）
			if store != nil {
				go func() {
					botToken.UsedAt = timePtr(time.Now())
					// 实现由 T13 store 层完成：store.UpdateTokenUsedAt(context.Background(), botToken.ID, botToken.UsedAt)
				}()
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuth 组合中间件：先尝试 Bearer，失败则尝试 Session。
// - Bearer 通过：注入 BotIdentity，继续
// - Bearer 失败且 Session 存在：继续（原 session 中间件行为）
// - Bearer 失败且 Session 不存在：返回 401 JSON（不重定向）
func RequireAuth(store BotTokenStore, sessionChecker func(w http.ResponseWriter, r *http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 先检查 Bearer token
			if hasBearer := checkBearerQuiet(store, r); hasBearer {
				// Bearer 通过，继续
				next.ServeHTTP(w, r)
				return
			}

			// Bearer 失败，尝试 Session
			if sessionChecker(w, r) {
				// Session 通过，继续
				next.ServeHTTP(w, r)
				return
			}

			// 都失败，返回 401 JSON
			respondUnauthorized(w, "authentication failed")
		})
	}
}

// checkBearerQuiet 无声地检查 Bearer token，返回是否通过。
// 若通过，会返回 true；失败返回 false（不影响 request）。
func checkBearerQuiet(store BotTokenStore, r *http.Request) bool {
	if store == nil {
		return false
	}

	authHeader := r.Header.Get("Authorization")
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return false
	}

	token := strings.TrimPrefix(authHeader, bearerPrefix)
	if token == "" {
		return false
	}

	botToken, err := store.Lookup(r.Context(), token)
	if err != nil || botToken == nil {
		return false
	}

	if botToken.ExpiresAt != nil && time.Now().After(*botToken.ExpiresAt) {
		return false
	}

	if botToken.CodeOrTokenHash == "" || botToken.Scope == "" {
		return false
	}

	if err := bcrypt.CompareHashAndPassword([]byte(botToken.CodeOrTokenHash), []byte(token)); err != nil {
		return false
	}

	return true
}

// GetIdentity 从 context 读取 BotIdentity。
func GetIdentity(ctx context.Context) *BotIdentity {
	identity, ok := ctx.Value(identityContextKey).(*BotIdentity)
	if !ok {
		return nil
	}
	return identity
}

// respondUnauthorized 返回标准 401 JSON 响应。
// 不暴露具体失败原因，仅返回通用 "unauthorized" 消息。
func respondUnauthorized(w http.ResponseWriter, logReason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)

	resp := map[string]string{
		"error": "unauthorized",
	}
	_ = json.NewEncoder(w).Encode(resp)

	// 详细原因写日志（避免 nil logger panic）
	if logReason != "" && global.GetLogger() != nil {
		global.GetSlogger().Infof("bearer_auth_failed reason=%s", logReason)
	}
}

// timePtr 返回时间指针（工具函数）。
func timePtr(t time.Time) *time.Time {
	return &t
}
