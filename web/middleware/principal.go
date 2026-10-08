// Package middleware 放请求主体（路线图 M12）：谁发起的请求、有哪些权限范围。
//
// 网页与现有 /api/* 只认 session；API 令牌（以后还有远程设备）只能访问 /api/app/v1/*、/mcp 与 qB 兼容入口，
// 由那几个入口自己解析主体、检查权限范围。
package middleware

import (
	"context"
	"net/http"
	"slices"
	"strings"
)

// 主体的种类。
const (
	KindSession      = "session"
	KindAPIToken     = "api_token"
	KindRemoteDevice = "remote_device"
)

// Principal 是发起请求的主体。session 拥有全部权限范围；令牌与设备只有自己的。
type Principal struct {
	Kind string
	// ID 是 session 的用户名、令牌编号或设备编号
	ID string
	// Name 是令牌或设备的名字
	Name   string
	Scopes []string
}

// Has 报告主体有没有这个权限范围（精确匹配，不支持通配）。
func (p *Principal) Has(scope string) bool {
	if p == nil {
		return false
	}
	return p.Kind == KindSession || slices.Contains(p.Scopes, scope)
}

type principalKey struct{}

// WithPrincipal 把主体放进 context。
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom 从 context 取主体，没有时返回 nil。
func PrincipalFrom(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}

// BearerToken 取 `Authorization: Bearer <令牌>` 里的令牌（scheme 不分大小写）。只认请求头，不认查询串。
func BearerToken(r *http.Request) (string, bool) {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	scheme, token, ok := strings.Cut(h, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}
