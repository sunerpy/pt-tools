package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NexusPHP 签到插件三种页面的形状（文字取自常见版本的 attendance.php）。
const (
	attendanceSuccessPage = `<html><body><table id="info_block"><tr><td>欢迎回来 <a href="attendance.php">[签到得魔力]</a></td></tr></table>
<h2>签到成功</h2><table><tr><td class="text"><p>这是您的第 <b>128</b> 次签到，已连续签到 <b>6</b> 天，本次签到获得 <b>60</b> 个魔力值。</p></td></tr></table></body></html>`
	attendanceAlreadyPage = `<html><body><table id="info_block"><tr><td>欢迎回来 <a href="attendance.php">[签到已得60]</a></td></tr></table>
<h2>抱歉</h2><table><tr><td class="text">您今天已经签到过了，请勿重复刷新。</td></tr></table></body></html>`
	attendanceEnglishSuccess = `<html><body><h2>Attendance successful</h2><p>This is your <b>12</b> attendance.</p></body></html>`
	attendanceUnknownPage    = `<html><body><table id="info_block"><tr><td>欢迎回来</td></tr></table><h2>404 Not Found</h2></body></html>`
	attendanceLoginPage      = `<html><body><form method="post" action="takelogin.php"><input name="username"/></form></body></html>`
)

func pageText(t *testing.T, html string) string {
	t.Helper()
	return attendancePageText([]byte(html))
}

// 标签换成空格：标题与下一段的文字不粘在一起，提示语从句首开始；脚本里的文字不参与匹配。
func TestAttendancePageText(t *testing.T) {
	assert.Equal(t, "签到成功 这是您的第 128 次签到，已连续签到 6 天 。",
		attendancePageText([]byte(`<h2>签到成功</h2><p>这是您的第 <b>128</b> 次签到，已连续签到 <b>6</b> 天 。</p><script>var t="签到成功";</script>`)))
	assert.Equal(t, "a & b", attendancePageText([]byte(`a &amp; b`)))

	res, ok := DefaultNexusPHPAttendance().Match(pageText(t, attendanceSuccessPage))
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(res.Message, "这是您的第 128 次签到"), "got %q", res.Message)
}

func TestAttendanceConfig_MatchDefaultNexusPHP(t *testing.T) {
	cfg := DefaultNexusPHPAttendance()
	cases := []struct {
		name    string
		page    string
		want    AttendStatus
		matched bool
		message string
	}{
		{name: "success", page: attendanceSuccessPage, want: AttendSigned, matched: true, message: "这是您的第 128 次签到"},
		{name: "already signed", page: attendanceAlreadyPage, want: AttendAlready, matched: true, message: "您今天已经签到过了"},
		{name: "english success", page: attendanceEnglishSuccess, want: AttendSigned, matched: true},
		{name: "unrelated page", page: attendanceUnknownPage, matched: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, ok := cfg.Match(pageText(t, tc.page))
			assert.Equal(t, tc.matched, ok)
			if !tc.matched {
				return
			}
			assert.Equal(t, tc.want, res.Status)
			if tc.message != "" {
				assert.Contains(t, res.Message, tc.message)
			}
			assert.LessOrEqual(t, len([]rune(res.Message)), 120, "the message is a short snippet, not the whole page")
		})
	}
}

func TestResolveAttendanceConfig(t *testing.T) {
	nexus := &SiteDefinition{ID: "x", Schema: SchemaNexusPHP}
	assert.Equal(t, "/attendance.php", ResolveAttendanceConfig(nexus).Path, "NexusPHP sites default to attendance.php")

	custom := &SiteDefinition{ID: "y", Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Path: "/attendance-ajax.php"}}
	assert.Equal(t, "/attendance-ajax.php", ResolveAttendanceConfig(custom).Path)
	assert.NotEmpty(t, ResolveAttendanceConfig(custom).SuccessPatterns, "patterns fall back to the NexusPHP defaults")

	unsupported := &SiteDefinition{ID: "z", Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Unsupported: "签到需要验证码"}}
	assert.Equal(t, "签到需要验证码", ResolveAttendanceConfig(unsupported).Unsupported)

	assert.Nil(t, ResolveAttendanceConfig(&SiteDefinition{ID: "m", Schema: SchemaMTorrent}))
	assert.Nil(t, ResolveAttendanceConfig(nil))
}

// fakeAttendanceSite 起一个只认 path 的假站点，按 body 返回页面并记录请求。
func fakeAttendanceSite(t *testing.T, path, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func newAttendanceDriver(baseURL string, def *SiteDefinition) *NexusPHPDriver {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: baseURL, Cookie: "uid=1; pass=2"})
	d.SetSiteDefinition(def)
	return d
}

func TestNexusPHPDriver_Attend(t *testing.T) {
	def := &SiteDefinition{ID: "qa", Schema: SchemaNexusPHP}

	t.Run("signed", func(t *testing.T) {
		srv, hits := fakeAttendanceSite(t, "/attendance.php", attendanceSuccessPage)
		res, err := newAttendanceDriver(srv.URL, def).Attend(context.Background())
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.Equal(t, int32(1), hits.Load())
	})
	t.Run("already signed today", func(t *testing.T) {
		srv, _ := fakeAttendanceSite(t, "/attendance.php", attendanceAlreadyPage)
		res, err := newAttendanceDriver(srv.URL, def).Attend(context.Background())
		require.NoError(t, err)
		assert.Equal(t, AttendAlready, res.Status)
	})
	t.Run("unrecognized page", func(t *testing.T) {
		srv, _ := fakeAttendanceSite(t, "/attendance.php", attendanceUnknownPage)
		_, err := newAttendanceDriver(srv.URL, def).Attend(context.Background())
		require.ErrorIs(t, err, ErrAttendanceUnrecognized)
	})
	t.Run("session expired", func(t *testing.T) {
		srv, _ := fakeAttendanceSite(t, "/attendance.php", attendanceLoginPage)
		_, err := newAttendanceDriver(srv.URL, def).Attend(context.Background())
		require.ErrorIs(t, err, ErrSessionExpired)
	})
	t.Run("custom path with query", func(t *testing.T) {
		custom := &SiteDefinition{ID: "qa2", Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Path: "/attendance-ajax.php?act=sign"}}
		var gotQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"status":"1","message":"<p>这是您的第<b>237</b>次签到</p>"}`))
		}))
		t.Cleanup(srv.Close)
		res, err := newAttendanceDriver(srv.URL, custom).Attend(context.Background())
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.Equal(t, "act=sign", gotQuery)
	})
	t.Run("unsupported site sends no request", func(t *testing.T) {
		srv, hits := fakeAttendanceSite(t, "/attendance.php", attendanceSuccessPage)
		unsupported := &SiteDefinition{ID: "qa3", Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Unsupported: "签到需要验证码"}}
		_, err := newAttendanceDriver(srv.URL, unsupported).Attend(context.Background())
		require.ErrorIs(t, err, ErrAttendanceUnsupported)
		assert.ErrorContains(t, err, "签到需要验证码")
		assert.Zero(t, hits.Load())
	})
}

func TestBaseSite_SupportsAttendance(t *testing.T) {
	nexus := NewBaseSite(newAttendanceDriver("http://127.0.0.1:1", &SiteDefinition{ID: "qa", Schema: SchemaNexusPHP}), BaseSiteConfig{ID: "qa"})
	assert.True(t, nexus.SupportsAttendance())

	unsupportedDef := &SiteDefinition{ID: "qa", Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Unsupported: "签到需要答题"}}
	assert.False(t, NewBaseSite(newAttendanceDriver("http://127.0.0.1:1", unsupportedDef), BaseSiteConfig{ID: "qa"}).SupportsAttendance())

	assert.False(t, NewBaseSite(NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "k"}), BaseSiteConfig{ID: "mteam"}).SupportsAttendance())
	assert.False(t, NewBaseSite(NewUnit3DDriver(Unit3DDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "k"}), BaseSiteConfig{ID: "u3d"}).SupportsAttendance())
	assert.False(t, NewBaseSite(NewGazelleDriver(GazelleDriverConfig{BaseURL: "http://127.0.0.1:1", Cookie: "c"}), BaseSiteConfig{ID: "gz"}).SupportsAttendance())

	_, err := NewBaseSite(NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "k"}), BaseSiteConfig{ID: "mteam"}).Attend(context.Background())
	assert.ErrorIs(t, err, ErrAttendanceUnsupported)

	var _ AttendanceCapable = nexus
}

// 计划 M1c：并发调用 BaseSite.Attend 时每次都经过限速器——低速率下 N 次调用的耗时不低于 (N-burst)/rate。
func TestBaseSite_AttendWaitsForTheRateLimiter(t *testing.T) {
	srv, hits := fakeAttendanceSite(t, "/attendance.php", attendanceAlreadyPage)
	site := NewBaseSite(newAttendanceDriver(srv.URL, &SiteDefinition{ID: "qa", Schema: SchemaNexusPHP}), BaseSiteConfig{ID: "qa", RateLimit: 20, RateBurst: 1})

	const calls = 5
	start := time.Now()
	var wg sync.WaitGroup
	for range calls {
		wg.Go(func() {
			_, err := site.Attend(context.Background())
			assert.NoError(t, err)
		})
	}
	wg.Wait()
	elapsed := time.Since(start)

	assert.Equal(t, int32(calls), hits.Load())
	// burst 1、每秒 20 个：第 2..5 次各要等 50ms，合计不少于 200ms（留 20ms 计时误差）。
	assert.GreaterOrEqual(t, elapsed, 180*time.Millisecond, "every call waits for the limiter")
}

func TestAttendanceSupport(t *testing.T) {
	ok, reason := AttendanceSupport(nil)
	assert.False(t, ok)
	assert.Contains(t, reason, "内置定义")

	ok, reason = AttendanceSupport(&SiteDefinition{ID: "m", Schema: SchemaMTorrent})
	assert.False(t, ok)
	assert.Contains(t, reason, "架构")

	ok, reason = AttendanceSupport(&SiteDefinition{ID: "h", Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Unsupported: "签到需要验证码"}})
	assert.False(t, ok)
	assert.Equal(t, "签到需要验证码", reason)

	ok, reason = AttendanceSupport(&SiteDefinition{ID: "n", Schema: SchemaNexusPHP})
	assert.True(t, ok)
	assert.Empty(t, reason)

	// 只给了匹配规则的配置补上默认路径
	cfg := ResolveAttendanceConfig(&SiteDefinition{ID: "p", Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{SuccessPatterns: []string{`ok`}}})
	assert.Equal(t, "/attendance.php", cfg.Path)
	assert.Equal(t, []string{`ok`}, cfg.SuccessPatterns)
}

func TestAttendanceConfig_MatchEdgeCases(t *testing.T) {
	var nilCfg *AttendanceConfig
	_, ok := nilCfg.Match("签到成功")
	assert.False(t, ok)

	cfg := &AttendanceConfig{SuccessPatterns: []string{`(`, `完成签到`}, AlreadyPatterns: []string{`已签`}}
	res, ok := cfg.Match("今天 完成签到！获得 10 魔力")
	require.True(t, ok, "an invalid pattern is skipped, the next one still matches")
	assert.Equal(t, AttendSigned, res.Status)
	assert.Equal(t, "完成签到！", res.Message)

	res, ok = cfg.Match("提示：已签")
	require.True(t, ok)
	assert.Equal(t, AttendAlready, res.Status)
}

func TestNexusPHPDriver_AttendEdgeCases(t *testing.T) {
	_, err := attendanceRequest("%zz")
	assert.Error(t, err)

	bad := &SiteDefinition{ID: "bad", Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Path: "%zz"}}
	_, err = newAttendanceDriver("http://127.0.0.1:1", bad).Attend(context.Background())
	assert.Error(t, err, "an unparsable path fails before any request")

	// 没有站点定义的 NexusPHP 驱动按默认的 attendance.php 签到
	srv, hits := fakeAttendanceSite(t, "/attendance.php", attendanceSuccessPage)
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: srv.URL, Cookie: "c=1"})
	res, err := d.Attend(context.Background())
	require.NoError(t, err)
	assert.Equal(t, AttendSigned, res.Status)
	assert.Equal(t, int32(1), hits.Load())
	assert.Empty(t, d.AttendanceUnsupportedReason())

	// 定义成了非 NexusPHP 架构又没配签到：驱动报告不支持
	other := newAttendanceDriver(srv.URL, &SiteDefinition{ID: "x", Schema: SchemaUnit3D})
	assert.NotEmpty(t, other.AttendanceUnsupportedReason())
	_, err = other.Attend(context.Background())
	assert.ErrorIs(t, err, ErrAttendanceUnsupported)
}

func TestBaseSite_AttendEdgeCases(t *testing.T) {
	unsupported := NewBaseSite(newAttendanceDriver("http://127.0.0.1:1", &SiteDefinition{ID: "qa", Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Unsupported: "签到需要答题"}}), BaseSiteConfig{ID: "qa"})
	_, err := unsupported.Attend(context.Background())
	assert.ErrorIs(t, err, ErrAttendanceUnsupported)
	assert.ErrorContains(t, err, "签到需要答题")

	srv, hits := fakeAttendanceSite(t, "/attendance.php", attendanceSuccessPage)
	site := NewBaseSite(newAttendanceDriver(srv.URL, &SiteDefinition{ID: "qa", Schema: SchemaNexusPHP}), BaseSiteConfig{ID: "qa", RateLimit: 0.001, RateBurst: 1})
	_, err = site.Attend(context.Background()) // 用掉唯一的令牌
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = site.Attend(ctx)
	assert.ErrorContains(t, err, "rate limit", "a cancelled wait on the limiter is reported")
	assert.Equal(t, int32(1), hits.Load())
}
