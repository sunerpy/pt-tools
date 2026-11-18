package sitelogin

import (
	"context"
	"testing"
	"time"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 解析失败要说清楚缺了什么、下一步做什么，而且写成中文：用户反馈过界面上只有「解析失败 (PARSE_ERROR)」，
// 看不出是 Cookie 失效、站点改版还是别的原因。
func TestParseErrorDiagnosticsAreSpecific(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC))
	nexus := &v2.SiteDefinition{Schema: v2.SchemaNexusPHP}
	emptyName, err := UserInfoServiceTransport{Service: emptyUsernameFetcher{}, SiteID: "audiences"}.
		FetchUserInfo(context.Background(), nexus, nil, clock)
	require.NoError(t, err)

	cases := []struct {
		name string
		res  *ProbeResult
		want []string
	}{
		{"NexusPHP 页面上没有用户名", ClassifyUserInfo(nexus, v2.UserInfo{}, nil, clock), []string{"用户名", "Cookie"}},
		{"用户信息服务没取到用户名", emptyName, []string{"用户名", "Cookie"}},
		{"HDDolby 没有最近访问", ClassifyUserInfo(&v2.SiteDefinition{Schema: v2.SchemaHDDolby}, v2.UserInfo{Username: "u"}, nil, clock), []string{"最近访问"}},
		{"M-Team 没有最近访问", classifyMTorrentResult(v2.UserInfo{Username: "u"}, nil), []string{"最近访问"}},
		{"Unit3D 没有最近登录与活动", classifyUnit3DResult(v2.UserInfo{Username: "u"}, nil), []string{"最近"}},
		{"Gazelle 没有最近访问", classifyGazelleResult(v2.UserInfo{Username: "u"}, nil), []string{"最近访问"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.res)
			assert.Equal(t, PARSE_ERROR, tc.res.Status)
			assert.True(t, hasHan(tc.res.Diagnostic), "说明要写成中文：%q", tc.res.Diagnostic)
			for _, w := range tc.want {
				assert.Contains(t, tc.res.Diagnostic, w)
			}
		})
	}
}

type emptyUsernameFetcher struct{}

func (emptyUsernameFetcher) FetchAndSave(context.Context, string) (v2.UserInfo, error) {
	return v2.UserInfo{}, v2.ErrEmptyUsername
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
