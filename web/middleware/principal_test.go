package middleware

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrincipalHas(t *testing.T) {
	var none *Principal
	assert.False(t, none.Has("app:read"))
	assert.True(t, (&Principal{Kind: KindSession}).Has("mcp:write"), "session 拥有全部权限范围")
	tok := &Principal{Kind: KindAPIToken, ID: "3", Scopes: []string{"app:read"}}
	assert.True(t, tok.Has("app:read"))
	assert.False(t, tok.Has("app:write"))
	assert.False(t, (&Principal{Kind: KindAPIToken, Scopes: []string{"app:*"}}).Has("app:read"), "不支持通配")
}

func TestPrincipalContext(t *testing.T) {
	assert.Nil(t, PrincipalFrom(context.Background()))
	p := &Principal{Kind: KindAPIToken, ID: "1"}
	assert.Same(t, p, PrincipalFrom(WithPrincipal(context.Background(), p)))
}

func TestBearerToken(t *testing.T) {
	for _, c := range []struct {
		header, want string
		ok           bool
	}{
		{"Bearer ptt_1_abc", "ptt_1_abc", true},
		{"bearer  ptt_1_abc ", "ptt_1_abc", true},
		{"Basic dXNlcjpwYXNz", "", false},
		{"Bearer", "", false},
		{"Bearer   ", "", false},
		{"", "", false},
	} {
		r := httptest.NewRequest("GET", "/api/app/v1/meta?token=ptt_9_x", nil)
		if c.header != "" {
			r.Header.Set("Authorization", c.header)
		}
		got, ok := BearerToken(r)
		assert.Equal(t, c.ok, ok, c.header)
		assert.Equal(t, c.want, got, c.header)
	}
}
