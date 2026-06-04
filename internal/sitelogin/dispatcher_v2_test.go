package sitelogin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

type mockTransport struct {
	name   string
	result *ProbeResult
	err    error
	calls  int
}

func (m *mockTransport) Name() string { return m.name }

func (m *mockTransport) FetchUserInfo(context.Context, *v2.SiteDefinition, v2.Site, Clock) (*ProbeResult, error) {
	m.calls++
	return m.result, m.err
}

func TestDispatcherTransportPluggable_HTTPDefault(t *testing.T) {
	primary := &mockTransport{name: "http", result: &ProbeResult{Status: OK, Diagnostic: "http ok"}}
	fallback := &mockTransport{name: "cloak", result: &ProbeResult{Status: OK, Diagnostic: "cloak ok"}}

	got, err := ProbeWithFallback(context.Background(), &v2.SiteDefinition{ID: "fake", Schema: v2.SchemaNexusPHP}, &fakeDispatchSite{}, newDispatchClock(), primary, nil)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, OK, got.Status)
	assert.Equal(t, "http ok", got.Diagnostic)
	assert.Equal(t, 1, primary.calls)
	assert.Equal(t, 0, fallback.calls)
}

func TestDispatcherTransportPluggable_FallbackOnRateLimited(t *testing.T) {
	assertFallbackOnStatus(t, RATE_LIMITED)
}

func TestDispatcherTransportPluggable_FallbackOnChallenge(t *testing.T) {
	assertFallbackOnStatus(t, CHALLENGE)
}

func TestDispatcherTransportPluggable_FallbackOnNetworkError(t *testing.T) {
	assertFallbackOnStatus(t, NETWORK_ERROR)
}

func TestDispatcherTransportPluggable_NoFallbackOnSessionExpired(t *testing.T) {
	assertNoFallbackOnStatus(t, SESSION_EXPIRED)
}

func TestDispatcherTransportPluggable_NoFallbackOnOK(t *testing.T) {
	assertNoFallbackOnStatus(t, OK)
}

func TestDispatcherTransportPluggable_NilFallback(t *testing.T) {
	primary := &mockTransport{name: "http", result: &ProbeResult{Status: RATE_LIMITED, Diagnostic: "rate limited"}}

	got, err := ProbeWithFallback(context.Background(), &v2.SiteDefinition{ID: "fake", Schema: v2.SchemaNexusPHP}, &fakeDispatchSite{}, newDispatchClock(), primary, nil)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, RATE_LIMITED, got.Status)
	assert.Equal(t, "rate limited", got.Diagnostic)
	assert.Equal(t, 1, primary.calls)
}

func TestDispatcherV1Compat(t *testing.T) {
	site := &countingDispatchSite{
		fakeDispatchSite: fakeDispatchSite{info: v2.UserInfo{LastAccess: 1, LastLogin: 1}},
	}
	def := &v2.SiteDefinition{ID: "fake", Schema: v2.SchemaNexusPHP}

	got, err := Probe(context.Background(), def, site, newDispatchClock())

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, OK, got.Status)
	assert.Equal(t, 1, site.calls, "v1 Probe must use the HTTP transport path exactly once")
}

func assertFallbackOnStatus(t *testing.T, status ProbeStatus) {
	t.Helper()
	primary := &mockTransport{name: "http", result: &ProbeResult{Status: status, Diagnostic: "primary"}}
	fallback := &mockTransport{name: "cloak", result: &ProbeResult{Status: OK, Diagnostic: "fallback"}}

	got, err := ProbeWithFallback(context.Background(), &v2.SiteDefinition{ID: "fake", Schema: v2.SchemaNexusPHP}, &fakeDispatchSite{}, newDispatchClock(), primary, fallback)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, OK, got.Status)
	assert.Equal(t, "fallback", got.Diagnostic)
	assert.Equal(t, 1, primary.calls)
	assert.Equal(t, 1, fallback.calls)
}

func assertNoFallbackOnStatus(t *testing.T, status ProbeStatus) {
	t.Helper()
	primary := &mockTransport{name: "http", result: &ProbeResult{Status: status, Diagnostic: "primary"}}
	fallback := &mockTransport{name: "cloak", result: &ProbeResult{Status: OK, Diagnostic: "fallback"}}

	got, err := ProbeWithFallback(context.Background(), &v2.SiteDefinition{ID: "fake", Schema: v2.SchemaNexusPHP}, &fakeDispatchSite{}, newDispatchClock(), primary, fallback)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, status, got.Status)
	assert.Equal(t, "primary", got.Diagnostic)
	assert.Equal(t, 1, primary.calls)
	assert.Equal(t, 0, fallback.calls)
}

type countingDispatchSite struct {
	fakeDispatchSite
	calls int
}

func (s *countingDispatchSite) GetUserInfo(ctx context.Context) (v2.UserInfo, error) {
	s.calls++
	return s.fakeDispatchSite.GetUserInfo(ctx)
}
