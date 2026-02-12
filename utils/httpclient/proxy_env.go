package httpclient

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

// ResolveProxyFromEnvironment resolves proxy URL for target URL,
// honoring HTTP_PROXY/HTTPS_PROXY/ALL_PROXY/NO_PROXY and lowercase variants.
//
// Go's http.ProxyFromEnvironment does NOT support ALL_PROXY/all_proxy,
// so we check it manually as a fallback when no protocol-specific proxy is found.
func ResolveProxyFromEnvironment(rawURL string) string {
	targetURL, err := url.Parse(rawURL)
	if err != nil || targetURL == nil {
		return ""
	}

	req := &http.Request{URL: targetURL}
	proxyURL, err := http.ProxyFromEnvironment(req)
	if err == nil && proxyURL != nil {
		return proxyURL.String()
	}

	allProxy := getEnvAny("all_proxy", "ALL_PROXY")
	if allProxy == "" {
		return ""
	}

	if useNoProxy(targetURL) {
		return ""
	}

	p, err := url.Parse(allProxy)
	if err != nil || p.Host == "" {
		return ""
	}
	return p.String()
}

// getEnvAny returns the first non-empty environment variable value.
func getEnvAny(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

// useNoProxy reports whether the target URL should bypass the proxy
// according to NO_PROXY / no_proxy.
func useNoProxy(target *url.URL) bool {
	noProxy := getEnvAny("no_proxy", "NO_PROXY")
	if noProxy == "" {
		return false
	}
	if noProxy == "*" {
		return true
	}
	host := target.Hostname()
	for _, p := range strings.Split(noProxy, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == host || (strings.HasPrefix(p, ".") && strings.HasSuffix(host, p)) ||
			(!strings.HasPrefix(p, ".") && strings.HasSuffix(host, "."+p)) {
			return true
		}
	}
	return false
}
