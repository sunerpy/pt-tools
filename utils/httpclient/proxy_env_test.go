package httpclient

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveProxyFromEnvironmentInvalidURL(t *testing.T) {
	assert.Equal(t, "", ResolveProxyFromEnvironment("://bad"))
}

func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(k, "")
	}
}

func TestResolveProxyFromEnvironmentAllProxyFallback(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("ALL_PROXY", "socks5://proxy.test:1080")
	got := ResolveProxyFromEnvironment("http://target.test/")
	assert.Equal(t, "socks5://proxy.test:1080", got)
}

func TestResolveProxyFromEnvironmentAllProxyInvalid(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("ALL_PROXY", "://no-host")
	assert.Equal(t, "", ResolveProxyFromEnvironment("http://target.test/"))
}

func TestResolveProxyFromEnvironmentNoEnv(t *testing.T) {
	clearProxyEnv(t)
	assert.Equal(t, "", ResolveProxyFromEnvironment("http://target.test/"))
}

func TestUseNoProxyDirect(t *testing.T) {
	clearProxyEnv(t)
	target, _ := url.Parse("http://svc.internal.test/")

	t.Run("no_env", func(t *testing.T) {
		t.Setenv("NO_PROXY", "")
		assert.False(t, useNoProxy(target))
	})
	t.Run("wildcard", func(t *testing.T) {
		t.Setenv("NO_PROXY", "*")
		assert.True(t, useNoProxy(target))
	})
	t.Run("dot_suffix", func(t *testing.T) {
		t.Setenv("NO_PROXY", ".internal.test")
		assert.True(t, useNoProxy(target))
	})
	t.Run("bare_suffix", func(t *testing.T) {
		t.Setenv("NO_PROXY", "internal.test")
		assert.True(t, useNoProxy(target))
	})
	t.Run("exact_host", func(t *testing.T) {
		exact, _ := url.Parse("http://target.test/")
		t.Setenv("NO_PROXY", "target.test")
		assert.True(t, useNoProxy(exact))
	})
	t.Run("no_match", func(t *testing.T) {
		t.Setenv("NO_PROXY", "other.test")
		assert.False(t, useNoProxy(target))
	})
	t.Run("empty_entries_skipped", func(t *testing.T) {
		t.Setenv("NO_PROXY", ", ,other.test")
		assert.False(t, useNoProxy(target))
	})
}

func TestResolveProxyAllProxyWithNoProxyBypass(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("ALL_PROXY", "socks5://proxy.test:1080")
	t.Setenv("NO_PROXY", "target.test")
	assert.Equal(t, "", ResolveProxyFromEnvironment("http://target.test/"))
}

func TestGetEnvAnyFirstNonEmpty(t *testing.T) {
	t.Setenv("FIRST_VAR", "")
	t.Setenv("SECOND_VAR", "value2")
	assert.Equal(t, "value2", getEnvAny("FIRST_VAR", "SECOND_VAR"))
	assert.Equal(t, "", getEnvAny("MISSING_A", "MISSING_B"))
}
