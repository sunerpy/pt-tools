package nas

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKind 登记一种只在测试里用的来源类型，测试结束时撤销。
func testKind(t *testing.T, k Kind) Kind {
	t.Helper()
	if k.New == nil {
		k.New = func(SourceConfig) (Source, error) { return &fakeSource{}, nil }
	}
	if k.Test == nil {
		k.Test = func(context.Context, TestInput) (*TestResult, error) { return &TestResult{}, nil }
	}
	if k.Label == "" {
		k.Label = k.Name
	}
	t.Cleanup(RegisterKind(k))
	return k
}

func TestRegisterKind(t *testing.T) {
	testKind(t, Kind{Name: "zeta"})
	testKind(t, Kind{Name: "alpha", Experimental: true})
	names := []string{}
	for _, k := range Kinds() {
		names = append(names, k.Name)
	}
	assert.Equal(t, []string{"alpha", "zeta"}, names, "按名字排")
	k, ok := LookupKind("alpha")
	require.True(t, ok)
	assert.True(t, k.Experimental)
	_, ok = LookupKind("nope")
	assert.False(t, ok)

	assert.Panics(t, func() { RegisterKind(Kind{Name: "zeta", New: k.New, Test: k.Test}) }, "登记两次")
	assert.Panics(t, func() { RegisterKind(Kind{Name: LocalID, New: k.New, Test: k.Test}) }, "local 是本机来源")
	assert.Panics(t, func() { RegisterKind(Kind{Name: "x", Test: k.Test}) }, "没有 New")
	assert.Panics(t, func() { RegisterKind(Kind{Name: "x", New: k.New}) }, "没有 Test")
	assert.Panics(t, func() { RegisterKind(Kind{New: k.New, Test: k.Test}) }, "没有名字")
}

func TestRegisterKind_Unregister(t *testing.T) {
	undo := RegisterKind(Kind{Name: "gone", New: func(SourceConfig) (Source, error) { return nil, nil }, Test: func(context.Context, TestInput) (*TestResult, error) { return nil, nil }})
	_, ok := LookupKind("gone")
	require.True(t, ok)
	undo()
	_, ok = LookupKind("gone")
	assert.False(t, ok)
}

func TestKindRequiresSecret(t *testing.T) {
	assert.True(t, Kind{}.RequiresSecret(""), "没写规则时总要密钥")
	glances := Kind{NeedsSecret: func(u string) bool { return u != "" }}
	assert.False(t, glances.RequiresSecret(""))
	assert.True(t, glances.RequiresSecret("admin"))
}

func TestParseURL(t *testing.T) {
	testKind(t, Kind{Name: "plain", Label: "Glances", Plain: PlainWithoutCredentials})
	testKind(t, Kind{Name: "tls", Label: "TrueNAS", Plain: PlainNever})
	testKind(t, Kind{Name: "hooked", Plain: PlainWithAck, ParseURL: func(u *url.URL) error {
		if u.Port() == "1" {
			return errors.New("端口 1 不行")
		}
		return nil
	}})

	ok := []struct{ kind, raw, want string }{
		{"plain", " https://NAS.Local:8443/ ", "https://nas.local:8443"},
		{"plain", "http://192.168.1.10:61208", "http://192.168.1.10:61208"},
		{"tls", "https://[fd00::1]:443", "https://[fd00::1]:443"},
		{"tls", "https://truenas/", "https://truenas"},
		{"hooked", "http://fnos:5666", "http://fnos:5666"},
	}
	for _, c := range ok {
		u, err := ParseURL(c.kind, c.raw)
		if assert.NoError(t, err, c.raw) {
			assert.Equal(t, c.want, u.String(), c.raw)
		}
	}

	bad := []struct{ kind, raw, msg string }{
		{"plain", "", "要填地址"},
		{"plain", "   ", "要填地址"},
		{"plain", "nas.local", "要以 https:// 或 http:// 开头"},
		{"plain", "ftp://nas", "要以 https:// 或 http:// 开头"},
		{"plain", "https://", "要以 https:// 或 http:// 开头"},
		{"plain", "https://user:pw@nas", "只要协议、主机和端口"},
		{"plain", "https://nas/api", "只要协议、主机和端口"},
		{"plain", "https://nas?x=1", "只要协议、主机和端口"},
		{"plain", "https://nas?", "只要协议、主机和端口"},
		{"plain", "https://nas#top", "只要协议、主机和端口"},
		{"plain", "https://nas/#", "只要协议、主机和端口"},
		{"plain", "https://:8443", "没有主机名"},
		{"tls", "http://truenas", "TrueNAS 只能用 https:// 地址"},
		{"hooked", "http://fnos:1", "端口 1 不行"},
		{"nope", "https://nas", `不支持的来源类型 "nope"`},
	}
	for _, c := range bad {
		_, err := ParseURL(c.kind, c.raw)
		if assert.Error(t, err, c.raw) {
			assert.Contains(t, err.Error(), c.msg, c.raw)
		}
	}
}

func TestCheckTransport(t *testing.T) {
	https, _ := url.Parse("https://nas")
	http, _ := url.Parse("http://nas")
	never := Kind{Label: "TrueNAS", Plain: PlainNever}
	noCreds := Kind{Plain: PlainWithoutCredentials}
	ack := Kind{Plain: PlainWithAck}

	for _, k := range []Kind{never, noCreds, ack} {
		assert.NoError(t, CheckTransport(k, https, true, false), "https 总可以")
	}
	assert.ErrorContains(t, CheckTransport(never, http, false, true), "只能用 https://", "确认了也不行")

	assert.NoError(t, CheckTransport(noCreds, http, false, false), "不带凭据的 http 可以")
	assert.ErrorIs(t, CheckTransport(noCreds, http, true, false), ErrPlainNeedsAck)
	assert.NoError(t, CheckTransport(noCreds, http, true, true))

	assert.ErrorIs(t, CheckTransport(ack, http, false, false), ErrPlainNeedsAck, "飞牛的 http 一律要确认")
	assert.NoError(t, CheckTransport(ack, http, false, true))
}

func TestAuthError(t *testing.T) {
	var err error = &AuthError{Reason: "API Key 不对", OTP: false}
	assert.ErrorIs(t, err, ErrAuth)
	assert.Equal(t, "认证失败：API Key 不对", err.Error())
	assert.Equal(t, "认证失败", (&AuthError{}).Error())
	wrapped := fmt.Errorf("登录: %w", &AuthError{OTP: true})
	var ae *AuthError
	require.ErrorAs(t, wrapped, &ae)
	assert.True(t, ae.OTP)
	assert.NotErrorIs(t, errors.New("x"), ErrAuth)
}
