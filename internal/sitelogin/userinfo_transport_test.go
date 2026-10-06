package sitelogin

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

type fakeUserInfoFetcher struct {
	info  v2.UserInfo
	err   error
	calls []string
}

func (f *fakeUserInfoFetcher) FetchAndSave(_ context.Context, siteID string) (v2.UserInfo, error) {
	f.calls = append(f.calls, siteID)
	return f.info, f.err
}

// M1b：经 UserInfoService 探测时按 SiteID 取已注册的实例，忽略传入的 site；结果按架构归类。
func TestUserInfoServiceTransport_Classifies(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	access := now.Add(-time.Hour).Unix()
	login := now.Add(-2 * time.Hour).Unix()
	nexus := &v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP}
	mteam := &v2.SiteDefinition{ID: "mteam", Schema: v2.SchemaMTorrent}

	cases := []struct {
		name       string
		def        *v2.SiteDefinition
		info       v2.UserInfo
		err        error
		wantStatus ProbeStatus
		wantSource ProbeSource
		wantAccess bool
		wantLogin  bool
	}{
		{name: "nexusphp ok", def: nexus, info: v2.UserInfo{Username: "u", LastAccess: access}, wantStatus: OK, wantSource: ProbeSourceHTTPCookie, wantAccess: true},
		{name: "site not registered", def: nexus, err: fmt.Errorf("site hdsky not registered: %w", v2.ErrSiteNotFound), wantStatus: NOT_CONFIGURED},
		{name: "empty username", def: nexus, err: fmt.Errorf("fetch user info from hdsky: %w", v2.ErrEmptyUsername), wantStatus: PARSE_ERROR},
		{
			// 保存失败的错误链里可能还带着 ErrSiteNotFound（仓库按 Site 字段查行），不能误判成未注册。
			name: "persist failure is judged by the returned info", def: nexus,
			info:       v2.UserInfo{Username: "u", LastAccess: access},
			err:        fmt.Errorf("save user info for hdsky: %w: %w", v2.ErrUserInfoPersist, v2.ErrSiteNotFound),
			wantStatus: OK, wantSource: ProbeSourceHTTPCookie, wantAccess: true,
		},
		{name: "session expired", def: nexus, err: fmt.Errorf("fetch user info from hdsky: %w", v2.ErrSessionExpired), wantStatus: SESSION_EXPIRED, wantSource: ProbeSourceHTTPCookie},
		{name: "mteam reports last login", def: mteam, info: v2.UserInfo{Username: "u", LastAccess: access, LastLogin: login}, wantStatus: OK, wantSource: ProbeSourceHTTPAPIKey, wantAccess: true, wantLogin: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := &fakeUserInfoFetcher{info: tc.info, err: tc.err}
			transport := UserInfoServiceTransport{Service: fetcher, SiteID: tc.def.ID}
			res, err := transport.FetchUserInfo(context.Background(), tc.def, nil, NewFakeClock(now))
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, []string{tc.def.ID}, fetcher.calls)
			assert.Equal(t, tc.wantStatus, res.Status)
			if tc.wantSource != "" {
				assert.Equal(t, tc.wantSource, res.Source)
			}
			assert.Equal(t, tc.wantAccess, res.LastAccessAt != nil)
			assert.Equal(t, tc.wantLogin, res.LastLoginAt != nil)
			if tc.wantLogin {
				assert.Equal(t, login, res.LastLoginAt.Unix())
			}
		})
	}
}

func TestUserInfoServiceTransport_NilService(t *testing.T) {
	res, err := UserInfoServiceTransport{SiteID: "hdsky"}.FetchUserInfo(context.Background(), &v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP}, nil, NewRealClock())
	require.NoError(t, err)
	assert.Equal(t, UNKNOWN, res.Status)
	assert.Equal(t, "userinfo", UserInfoServiceTransport{}.Name())
}

// M1b：HTTP 主通道与 UserInfoService 主通道共用同一套归类；M-Team 的最近登录写进结果。
func TestProbeMTorrent_ReportsLastLogin(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	login := now.Add(-3 * time.Hour).Unix()
	site := &fakeMTorrentSite{info: v2.UserInfo{LastAccess: now.Add(-time.Hour).Unix(), LastLogin: login}}
	res, err := ProbeMTorrent(context.Background(), site, NewFakeClock(now))
	require.NoError(t, err)
	require.Equal(t, OK, res.Status)
	require.NotNil(t, res.LastLoginAt)
	assert.Equal(t, login, res.LastLoginAt.Unix())
	assert.Equal(t, ProbeSourceHTTPAPIKey, res.Source)

	site.info.LastLogin = 0
	res, err = ProbeMTorrent(context.Background(), site, NewFakeClock(now))
	require.NoError(t, err)
	assert.Nil(t, res.LastLoginAt, "no last login reported, none recorded")
}

// 两条主通道共用的归类按架构分派，来源与原来的 HTTP 主通道一致。
func TestClassifyUserInfo_RoutesBySchema(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	info := v2.UserInfo{Username: "u", LastAccess: now.Add(-time.Hour).Unix(), LastLogin: now.Add(-2 * time.Hour).Unix()}
	cases := []struct {
		schema     v2.Schema
		wantStatus ProbeStatus
		wantSource ProbeSource
	}{
		{v2.SchemaNexusPHP, OK, ProbeSourceHTTPCookie},
		{v2.SchemaHDDolby, OK, ProbeSourceHTTPCookie},
		{v2.SchemaRousi, OK, ProbeSourceHTTPAPIKey},
		{v2.SchemaMTorrent, OK, ProbeSourceHTTPAPIKey},
		{v2.SchemaGazelle, OK, ProbeSourceHTTPCookie},
		{v2.SchemaUnit3D, OK, ProbeSourceHTTPCookie},
		{v2.Schema("Unknown"), UNKNOWN, ""},
	}
	for _, tc := range cases {
		t.Run(string(tc.schema), func(t *testing.T) {
			res := ClassifyUserInfo(&v2.SiteDefinition{ID: "x", Schema: tc.schema}, info, nil, NewFakeClock(now))
			require.NotNil(t, res)
			assert.Equal(t, tc.wantStatus, res.Status)
			assert.Equal(t, tc.wantSource, res.Source)
		})
	}
	res := ClassifyUserInfo(nil, info, nil, NewFakeClock(now))
	assert.Equal(t, UNKNOWN, res.Status)
}
