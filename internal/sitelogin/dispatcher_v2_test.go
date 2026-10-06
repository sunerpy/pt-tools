package sitelogin

import (
	"context"
	"fmt"
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

// M1d：后备只在返回 OK 时替换主通道结果；其他状态（含 NOT_APPLICABLE）与出错时保留主通道状态，只附上后备的说明。
func TestProbeWithFallback_KeepsPrimaryUnlessFallbackOK(t *testing.T) {
	cases := []struct {
		name     string
		fallback *mockTransport
		wantNote string
	}{
		{"fallback challenged too", &mockTransport{name: "cloak", result: &ProbeResult{Status: CHALLENGE, Diagnostic: "still blocked"}}, "still blocked"},
		{"fallback not applicable", &mockTransport{name: "cloak", result: &ProbeResult{Status: NOT_APPLICABLE, Diagnostic: "no cookie"}}, "no cookie"},
		{"fallback parse error without diagnostic", &mockTransport{name: "cloak", result: &ProbeResult{Status: PARSE_ERROR}}, "PARSE_ERROR"},
		{"fallback errored", &mockTransport{name: "cloak", err: assert.AnError}, assert.AnError.Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			primary := &mockTransport{name: "http", result: &ProbeResult{Status: NETWORK_ERROR, Diagnostic: "dial timeout"}}
			got, err := ProbeWithFallback(context.Background(), &v2.SiteDefinition{ID: "fake", Schema: v2.SchemaNexusPHP}, &fakeDispatchSite{}, newDispatchClock(), primary, tc.fallback)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, NETWORK_ERROR, got.Status, "the primary status stays")
			assert.Equal(t, "dial timeout", got.Diagnostic)
			assert.Contains(t, got.FallbackNote, "CloakBrowser")
			assert.Contains(t, got.FallbackNote, tc.wantNote)
			assert.Equal(t, 1, tc.fallback.calls)
		})
	}
}

func TestCloakPlaceholderTransportRemoved(t *testing.T) {
	// 占位的 CloakTransport 已删除：后备由 internal/cloakdriver/transport 提供。
	transports := []Transport{HTTPTransport{}, UserInfoServiceTransport{}}
	for _, tr := range transports {
		assert.NotEqual(t, "cloak", tr.Name())
	}
}

// M1d：site/v2 把 Cloudflare 质询页报成 ErrCloudflareChallenge（错误正文不一定带 cloudflare 字样）；
// 有后备的四种架构都按这个哨兵归为 CHALLENGE，后备才会执行。Gazelle 以前会把它当成会话过期。
func TestClassifyUserInfo_CloudflareChallengeSentinel(t *testing.T) {
	err := fmt.Errorf("fetch user info from x: %w", fmt.Errorf("HTTP 403: %w", errSentinelOnly{v2.ErrCloudflareChallenge}))
	for _, schema := range []v2.Schema{v2.SchemaNexusPHP, v2.SchemaUnit3D, v2.SchemaGazelle, v2.SchemaMTorrent} {
		got := ClassifyUserInfo(&v2.SiteDefinition{ID: "x", Schema: schema}, v2.UserInfo{}, err, NewRealClock())
		assert.Equal(t, CHALLENGE, got.Status, schema)
		assert.True(t, isFallbackEligible(got.Status), schema)
	}
}

// errSentinelOnly 包住哨兵但给出与之无关的文字，确保分类靠 errors.Is 而不是字符串匹配。
type errSentinelOnly struct{ sentinel error }

func (e errSentinelOnly) Error() string { return "blocked by upstream" }
func (e errSentinelOnly) Unwrap() error { return e.sentinel }
