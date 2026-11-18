package transport

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/cloakdriver"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// fakeManager 只回应 LaunchProfile；不起浏览器，驱动在 Manager 这一步就会给出分类结果。
type fakeManager struct {
	launchErr error
	launches  atomic.Int32
}

func (f *fakeManager) LaunchProfile(context.Context, string) (*cloakdriver.ProfileLaunchResult, error) {
	f.launches.Add(1)
	return nil, f.launchErr
}

func (f *fakeManager) GetProfileStatus(context.Context, string) (*cloakdriver.ProfileStatus, error) {
	return &cloakdriver.ProfileStatus{}, nil
}
func (f *fakeManager) StopProfile(context.Context, string) error   { return nil }
func (f *fakeManager) DeleteProfile(context.Context, string) error { return nil }
func (f *fakeManager) ManagerStatus(context.Context) error         { return nil }
func (f *fakeManager) ManagerStatusFull(context.Context) (*cloakdriver.ManagerStatusInfo, error) {
	return &cloakdriver.ManagerStatusInfo{}, nil
}

var (
	nexusDef  = &v2.SiteDefinition{ID: "hdtime", Schema: v2.SchemaNexusPHP, URLs: []string{"https://hdtime.org/"}}
	unit3dDef = &v2.SiteDefinition{ID: "u3d", Schema: v2.SchemaUnit3D, URLs: []string{"https://u3d.example/"}}
	gazDef    = &v2.SiteDefinition{ID: "mooko", Schema: v2.SchemaGazelle, URLs: []string{"https://mooko.example/"}}
	mteamDef  = &v2.SiteDefinition{ID: "mteam", Schema: v2.SchemaMTorrent, URLs: []string{"https://api.m-team.cc/"}, WebURL: "https://kp.m-team.cc/"}
)

// M1d：导航地址与主通道访问同一个站点地址（APIUrl 优先，否则定义的默认地址）；M-Team 的网页固定用 WebURL。
func TestNavigationURL(t *testing.T) {
	id := Identity{UserID: "93012", Username: "qa user"}
	custom := models.SiteSetting{APIUrl: "https://mirror.example/"}
	cases := []struct {
		name    string
		setting models.SiteSetting
		def     *v2.SiteDefinition
		id      Identity
		want    string
		ok      bool
	}{
		{"nexusphp default", models.SiteSetting{}, nexusDef, id, "https://hdtime.org/userdetails.php?id=93012", true},
		{"nexusphp custom api url", custom, nexusDef, id, "https://mirror.example/userdetails.php?id=93012", true},
		{"nexusphp without user id", models.SiteSetting{}, nexusDef, Identity{Username: "u"}, "", false},
		{"unit3d", models.SiteSetting{}, unit3dDef, id, "https://u3d.example/users/qa%20user", true},
		{"unit3d custom api url", custom, unit3dDef, id, "https://mirror.example/users/qa%20user", true},
		{"unit3d without username", models.SiteSetting{}, unit3dDef, Identity{UserID: "1"}, "", false},
		{"gazelle", models.SiteSetting{}, gazDef, id, "https://mooko.example/user.php?id=93012", true},
		{"gazelle custom api url", custom, gazDef, id, "https://mirror.example/user.php?id=93012", true},
		{"mteam uses the web url", custom, mteamDef, Identity{}, "https://kp.m-team.cc/profile", true},
		{"hddolby has no fallback", models.SiteSetting{}, &v2.SiteDefinition{ID: "hddolby", Schema: v2.SchemaHDDolby, URLs: []string{"https://x/"}}, id, "", false},
		{"rousi has no fallback", models.SiteSetting{}, &v2.SiteDefinition{ID: "rousipro", Schema: v2.SchemaRousi, URLs: []string{"https://x/"}}, id, "", false},
		{"no address", models.SiteSetting{}, &v2.SiteDefinition{ID: "x", Schema: v2.SchemaNexusPHP}, id, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NavigationURL(tc.setting, tc.def, tc.id)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func completeProvider(manager cloakdriver.ManagerClient) *Provider {
	return &Provider{
		Settings: func() (Settings, error) {
			return Settings{Endpoint: "http://cloak:8080", Token: "tok", ProfileID: "profile-1"}, nil
		},
		Identity: func(context.Context, string) (Identity, bool) {
			return Identity{UserID: "93012", Username: "qa"}, true
		},
		Cookie:     func(models.SiteSetting) (string, error) { return "uid=93012; pass=abc", nil },
		NewManager: func(string, string) cloakdriver.ManagerClient { return manager },
	}
}

// 提供者在配置不全、架构不支持、缺 Cookie、缺身份信息或读配置出错时返回 nil。
func TestProvider_NilWhenNotApplicable(t *testing.T) {
	site := models.SiteSetting{Name: "hdtime"}
	mutate := map[string]func(p *Provider){
		"no endpoint": func(p *Provider) {
			p.Settings = func() (Settings, error) { return Settings{Token: "t", ProfileID: "p"}, nil }
		},
		"no token": func(p *Provider) {
			p.Settings = func() (Settings, error) { return Settings{Endpoint: "e", ProfileID: "p"}, nil }
		},
		"no profile": func(p *Provider) {
			p.Settings = func() (Settings, error) { return Settings{Endpoint: "e", Token: "t"}, nil }
		},
		"settings err": func(p *Provider) { p.Settings = func() (Settings, error) { return Settings{}, errors.New("db") } },
		"no cookie":    func(p *Provider) { p.Cookie = func(models.SiteSetting) (string, error) { return "", nil } },
		"cookie err": func(p *Provider) {
			p.Cookie = func(models.SiteSetting) (string, error) { return "", errors.New("decrypt") }
		},
		"bad cookie": func(p *Provider) { p.Cookie = func(models.SiteSetting) (string, error) { return "@@@", nil } },
		"no identity": func(p *Provider) {
			p.Identity = func(context.Context, string) (Identity, bool) { return Identity{}, false }
		},
		"nil identity": func(p *Provider) { p.Identity = nil },
		"nil settings": func(p *Provider) { p.Settings = nil },
		"no driver":    func(p *Provider) { p.Drivers = func(v2.Schema, cloakdriver.ManagerClient) Prober { return nil } },
	}
	for name, change := range mutate {
		t.Run(name, func(t *testing.T) {
			p := completeProvider(&fakeManager{})
			change(p)
			assert.Nil(t, p.FallbackFor(context.Background(), site, nexusDef))
		})
	}
	p := completeProvider(&fakeManager{})
	assert.Nil(t, p.FallbackFor(context.Background(), site, &v2.SiteDefinition{ID: "hddolby", Schema: v2.SchemaHDDolby, URLs: []string{"https://x/"}}))
	assert.Nil(t, p.FallbackFor(context.Background(), site, nil))
	var nilProvider *Provider
	assert.Nil(t, nilProvider.FallbackFor(context.Background(), site, nexusDef))
	// M-Team 不需要身份信息，但需要 Cookie
	p.Identity = nil
	assert.NotNil(t, p.FallbackFor(context.Background(), models.SiteSetting{Name: "mteam"}, mteamDef))
}

// 提供者用三项配置建 Manager 客户端，把导航地址、解析好的 Cookie 和 profile 交给驱动；结果的来源记为 cloak。
func TestProvider_BuildsTransport(t *testing.T) {
	var gotEndpoint, gotToken, gotURL, gotProfile string
	var gotCookies []*http.Cookie
	p := completeProvider(nil)
	p.NewManager = func(endpoint, token string) cloakdriver.ManagerClient {
		gotEndpoint, gotToken = endpoint, token
		return &fakeManager{}
	}
	access := time.Now().Add(-time.Hour).UTC()
	p.Drivers = func(schema v2.Schema, _ cloakdriver.ManagerClient) Prober {
		require.Equal(t, v2.SchemaNexusPHP, schema)
		return func(_ context.Context, url string, cookies []*http.Cookie, profile string) (*sitelogin.ProbeResult, error) {
			gotURL, gotCookies, gotProfile = url, cookies, profile
			return &sitelogin.ProbeResult{Status: sitelogin.OK, LastAccessAt: &access}, nil
		}
	}

	tr := p.FallbackFor(context.Background(), models.SiteSetting{Name: "hdtime", APIUrl: "https://mirror.example"}, nexusDef)
	require.NotNil(t, tr)
	assert.Equal(t, "cloak", tr.Name())
	res, err := tr.FetchUserInfo(context.Background(), nexusDef, nil, sitelogin.NewRealClock())
	require.NoError(t, err)
	assert.Equal(t, sitelogin.OK, res.Status)
	assert.Equal(t, sitelogin.ProbeSourceCloak, res.Source)
	assert.Equal(t, "http://cloak:8080", gotEndpoint)
	assert.Equal(t, "tok", gotToken)
	assert.Equal(t, "https://mirror.example/userdetails.php?id=93012", gotURL)
	assert.Equal(t, "profile-1", gotProfile)
	require.Len(t, gotCookies, 2)
	assert.Equal(t, "uid", gotCookies[0].Name)
	assert.Equal(t, "pass", gotCookies[1].Name)
}

// 用真实的各架构驱动和假的 Manager：Manager 拒绝时得到驱动分类好的结果，不会起浏览器。
func TestProvider_DefaultDrivers(t *testing.T) {
	for _, def := range []*v2.SiteDefinition{nexusDef, unit3dDef, gazDef, mteamDef} {
		manager := &fakeManager{launchErr: cloakdriver.ErrManagerAuthFailed}
		p := completeProvider(manager)
		tr := p.FallbackFor(context.Background(), models.SiteSetting{Name: def.ID}, def)
		require.NotNil(t, tr, def.ID)
		res, err := tr.FetchUserInfo(context.Background(), def, nil, sitelogin.NewRealClock())
		require.NoError(t, err, def.ID)
		assert.NotEqual(t, sitelogin.OK, res.Status, def.ID)
		assert.Equal(t, sitelogin.ProbeSourceCloak, res.Source, def.ID)
		assert.Equal(t, int32(1), manager.launches.Load(), def.ID)
	}
	assert.Nil(t, defaultDriver(v2.SchemaHDDolby, &fakeManager{}))
	assert.NotNil(t, defaultManager("http://cloak:8080", "tok"))
}

// 两个站点同时需要后备时串行执行：所有站点共用一个 CloakBrowser profile。
func TestTransport_RunsOneAtATime(t *testing.T) {
	var running, peak atomic.Int32
	prober := func(context.Context, string, []*http.Cookie, string) (*sitelogin.ProbeResult, error) {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		running.Add(-1)
		return &sitelogin.ProbeResult{Status: sitelogin.OK}, nil
	}
	var wg sync.WaitGroup
	for range 3 {
		tr := &Transport{URL: "https://x/userdetails.php?id=1", ProfileID: "p", Probe: prober}
		wg.Go(func() {
			_, err := tr.FetchUserInfo(context.Background(), nexusDef, nil, nil)
			assert.NoError(t, err)
		})
	}
	wg.Wait()
	assert.Equal(t, int32(1), peak.Load())
}

// 驱动停止 profile 用的是独立 ctx，所以交给驱动的期限比探测预算早 stopReserve；没有期限时原样传下去。
func TestTransport_ReservesStopTimeWithinBudget(t *testing.T) {
	var got time.Time
	var hasDeadline bool
	tr := &Transport{Probe: func(ctx context.Context, _ string, _ []*http.Cookie, _ string) (*sitelogin.ProbeResult, error) {
		got, hasDeadline = ctx.Deadline()
		return &sitelogin.ProbeResult{Status: sitelogin.OK}, nil
	}}

	deadline := time.Now().Add(90 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	_, err := tr.FetchUserInfo(ctx, nexusDef, nil, nil)
	require.NoError(t, err)
	require.True(t, hasDeadline)
	// 10 秒即各驱动 defer 里 StopProfile 的 stopTimeout。
	want := deadline.Add(-10 * time.Second)
	assert.True(t, got.Equal(want), "got %v want %v", got, want)

	_, err = tr.FetchUserInfo(context.Background(), nexusDef, nil, nil)
	require.NoError(t, err)
	assert.False(t, hasDeadline)

	// 剩余预算不足以停止 profile 时，驱动拿到的 ctx 已经到期，启动请求立即失败，不会再去启动浏览器。
	short, cancelShort := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShort()
	var expired bool
	tr.Probe = func(ctx context.Context, _ string, _ []*http.Cookie, _ string) (*sitelogin.ProbeResult, error) {
		expired = ctx.Err() != nil
		return &sitelogin.ProbeResult{Status: sitelogin.NETWORK_ERROR}, nil
	}
	_, err = tr.FetchUserInfo(short, nexusDef, nil, nil)
	require.NoError(t, err)
	assert.True(t, expired)
}

// 排队等锁也受探测预算约束：等不到就按超时返回，不会拖过预算。
func TestTransport_WaitHonoursContext(t *testing.T) {
	release := make(chan struct{})
	holder := &Transport{Probe: func(context.Context, string, []*http.Cookie, string) (*sitelogin.ProbeResult, error) {
		<-release
		return &sitelogin.ProbeResult{Status: sitelogin.OK}, nil
	}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = holder.FetchUserInfo(context.Background(), nexusDef, nil, nil)
	}()
	require.Eventually(t, func() bool { return len(cloakSlot) == 1 }, time.Second, 5*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	waiter := &Transport{Probe: func(context.Context, string, []*http.Cookie, string) (*sitelogin.ProbeResult, error) {
		t.Fatal("must not run while another CloakBrowser probe holds the profile")
		return nil, nil
	}}
	_, err := waiter.FetchUserInfo(ctx, nexusDef, nil, nil)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	close(release)
	<-done

	empty := &Transport{Probe: func(context.Context, string, []*http.Cookie, string) (*sitelogin.ProbeResult, error) { return nil, nil }}
	res, err := empty.FetchUserInfo(context.Background(), nexusDef, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, sitelogin.UNKNOWN, res.Status)
	failing := &Transport{Probe: func(context.Context, string, []*http.Cookie, string) (*sitelogin.ProbeResult, error) {
		return nil, errors.New("empty profile")
	}}
	_, err = failing.FetchUserInfo(context.Background(), nexusDef, nil, nil)
	assert.ErrorContains(t, err, "empty profile")
}
