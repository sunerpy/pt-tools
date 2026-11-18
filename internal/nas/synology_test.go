package nas

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynologyKind(t *testing.T) {
	k, ok := LookupKind(KindSynology)
	require.True(t, ok)
	assert.Equal(t, "群晖", k.Label)
	assert.True(t, k.Experimental, "没在真机上验证过")
	assert.Equal(t, UsernameRequired, k.Username)
	assert.True(t, k.RequiresSecret("admin"))
	assert.Equal(t, PlainWithoutCredentials, k.Plain)

	cases := []struct{ in, want string }{
		{"http://192.168.1.10", "http://192.168.1.10:5000"},
		{"http://DS920.local/", "http://ds920.local:5000"},
		{"http://192.168.1.10:5050", "http://192.168.1.10:5050"},
		{"http://[fd00::10]", "http://[fd00::10]:5000"},
		{"https://192.168.1.10:5001", "https://192.168.1.10:5001"},
		{"https://nas.example.com", "https://nas.example.com"},
	}
	for _, c := range cases {
		u, err := ParseURL(KindSynology, c.in)
		require.NoError(t, err, c.in)
		assert.Equal(t, c.want, u.String(), c.in)
	}
	_, err := ParseURL(KindSynology, "https://192.168.1.10:5001/webapi")
	require.ErrorIs(t, err, errURLShape)

	u, err := ParseURL(KindSynology, "http://192.168.1.10")
	require.NoError(t, err)
	require.ErrorIs(t, CheckTransport(k, u, true, false), ErrPlainNeedsAck, "要填用户名，经 http 总会发出账号")
	require.NoError(t, CheckTransport(k, u, true, true))
}

func TestSynologyNewRequiresAccountAndAck(t *testing.T) {
	_, err := newSynology(SourceConfig{URL: "https://ds:5001", Username: "u"}, nil)
	require.ErrorIs(t, err, errSynoNoAccount)
	_, err = newSynology(SourceConfig{URL: "https://ds:5001", Username: " ", Secret: "p"}, nil)
	require.ErrorIs(t, err, errSynoNoAccount)
	_, err = newSynology(SourceConfig{URL: "http://ds:5000", Username: "u", Secret: "p"}, nil)
	require.ErrorIs(t, err, ErrPlainNeedsAck)
	_, err = newSynology(SourceConfig{URL: "http://ds:5000", Username: "u", Secret: "p", InsecureAck: true}, nil)
	require.NoError(t, err)
	_, err = newSynology(SourceConfig{URL: "%zz", Username: "u", Secret: "p"}, nil)
	require.Error(t, err)

	k, _ := LookupKind(KindSynology)
	src, err := k.New(SourceConfig{URL: "https://ds:5001", Username: "u", Secret: "p"})
	require.NoError(t, err)
	_, ok := src.(ExtraSource)
	assert.True(t, ok, "群晖来源会交出刷新的设备令牌")
	require.NoError(t, src.Close())
	_, err = k.New(SourceConfig{URL: "https://ds:5001"})
	require.ErrorIs(t, err, errSynoNoAccount)

	_, err = testSynology(context.Background(), TestInput{}, nil)
	require.Error(t, err)
	_, err = testSynology(context.Background(), TestInput{URL: &url.URL{Scheme: "https", Host: "ds"}, Username: "u"}, nil)
	require.ErrorIs(t, err, errSynoNoAccount)
}

func TestSynologyCollect(t *testing.T) {
	f := newFakeDSM(t, false)
	s := f.source(f.config())

	snap, err := s.Collect(context.Background(), true)
	require.NoError(t, err)

	assert.Equal(t, System{
		Platform: "DSM", Version: "7.2.2-72806 Update 3", Model: "DS920+", Cores: 4, UptimeSeconds: 352*3600 + 6*60 + 17,
	}, snap.System)
	assert.InDelta(t, 20.0, snap.CPU.Percent, 1e-9, "用户、系统与其他三项之和")
	assert.InDelta(t, 2.31, snap.CPU.Load1, 1e-9, "DSM 的平均负载乘了 100")
	assert.InDelta(t, 1.87, snap.CPU.Load5, 1e-9)
	assert.InDelta(t, 1.52, snap.CPU.Load15, 1e-9)
	assert.Nil(t, snap.CPU.IOWait)
	require.NotNil(t, snap.CPU.TempC)
	assert.InDelta(t, 46.0, *snap.CPU.TempC, 1e-9)
	assert.Equal(t, Memory{Total: 7969616 * 1024, Used: 1713786225, Available: 7969616*1024 - 1713786225}, snap.Memory,
		"KB 换成字节，已用按 DSM 的 real_usage")
	assert.Equal(t, Network{RxBps: 524288, TxBps: 131072, Interfaces: []string{"eth0", "eth1"}}, snap.Network)

	yes := true
	assert.Equal(t, []Pool{
		{Name: "/volume1", Status: "正常", Healthy: &yes, Size: 3829364490240, Used: 1530000000000, Free: 2299364490240, Disks: 2},
		{Name: "/volume2", Status: "正常", Healthy: &yes, Size: 380107374182, Used: 95026843545, Free: 285080530637, Disks: 1},
	}, snap.Pools)
	t36, t38, t48 := 36.0, 38.0, 48.0
	assert.Equal(t, []Disk{
		{Name: "Drive 1", Model: "WD40EFRX-68N32N0", Kind: DiskHDD, Size: 4000787030016, TempC: &t36, Pool: "/volume1"},
		{Name: "Drive 2", Model: "WD40EFRX-68N32N0", Kind: DiskHDD, Size: 4000787030016, TempC: &t38, Pool: "/volume1"},
		{Name: "M.2 Drive 1", Model: "SNV3410-400G", Kind: DiskNVMe, Size: 400088457216, TempC: &t48, Pool: "/volume2"},
		{Name: "Drive 3", Model: "Samsung SSD 870 EVO 1TB", Kind: DiskSSD, Size: 1000204886016},
	}, snap.Disks, "型号去掉 DSM 补的空格；温度 0 是没有；不在池里的盘不写池")
	assert.Empty(t, snap.Alerts)
	assert.Equal(t, Apps{}, snap.Apps)
	assert.Equal(t, map[string]Facet{
		FacetSystem:  {Status: FacetOK},
		FacetCPU:     {Status: FacetOK},
		FacetCPUTemp: {Status: FacetOK},
		FacetMemory:  {Status: FacetOK},
		FacetNetwork: {Status: FacetOK},
		FacetPools:   {Status: FacetOK},
		FacetDisks:   {Status: FacetOK},
		FacetAlerts:  {Status: FacetUnsupported, Reason: synologyNoAlerts},
		FacetApps:    {Status: FacetUnsupported, Reason: synologyNoApps},
	}, snap.Facets)
	b, err := json.Marshal(snap)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "FAKESERIAL", "不读序列号")

	assert.Equal(t, []string{
		"SYNO.API.Info query", "SYNO.API.Auth login", "SYNO.Core.System.Utilization get", "SYNO.Core.System info",
		"SYNO.Storage.CGI.Storage load_info",
	}, f.calls())
	reqs := f.requests()
	info := reqs[0]
	assert.Equal(t, http.MethodGet, info.method)
	assert.Equal(t, strings.Join(synoAPIs, ","), info.query.Get("query"))
	login := reqs[1]
	assert.Equal(t, http.MethodPost, login.method)
	assert.Equal(t, url.Values{"api": {synoAPIAuth}, "version": {"7"}, "method": {"login"}}, login.query,
		"网址里只有 api、version、method")
	assert.Equal(t, "ptmon", login.form.Get("account"))
	assert.Equal(t, "s3cret-pass", login.form.Get("passwd"))
	assert.Equal(t, "sid", login.form.Get("format"))
	assert.Equal(t, "yes", login.form.Get("enable_device_token"))
	assert.Equal(t, "pt-tools", login.form.Get("device_name"))
	assert.NotContains(t, login.form, "otp_code")
	assert.NotContains(t, login.form, "device_id", "还没有设备令牌")
	versions := map[string]string{}
	for _, r := range reqs[2:] {
		assert.Equal(t, http.MethodPost, r.method)
		assert.Equal(t, []string{"api", "method", "version"}, slices.Sorted(maps.Keys(r.query)), "会话与令牌只在请求体里")
		assert.Equal(t, "sid-1-abcdef", r.form.Get("_sid"))
		assert.Equal(t, "synotoken-1", r.form.Get("SynoToken"))
		versions[r.form.Get("api")] = r.form.Get("version")
	}
	assert.Equal(t, map[string]string{synoAPIUtilization: "1", synoAPISystem: "3", synoAPIStorage: "1"}, versions)
	assert.Equal(t, "load_info", reqs[4].form.Get("action"), "同 py-synologydsm-api：存储接口还要 action")
}

func TestSynologyLightCollectReusesSession(t *testing.T) {
	f := newFakeDSM(t, false)
	s := f.source(f.config())
	_, err := s.Collect(context.Background(), true)
	require.NoError(t, err)
	f.resetRequests()
	f.set(synoAPISystem+" info", dsmReply{data: strings.Replace(dsmSystemInfo, `"sys_temp":46`, `"sys_temp":"51"`, 1)})

	snap, err := s.Collect(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, []string{"SYNO.Core.System.Utilization get", "SYNO.Core.System info"}, f.calls(),
		"会话复用：不再查接口、不再登录；轻量采集不读存储")
	require.NotNil(t, snap.CPU.TempC)
	assert.InDelta(t, 51.0, *snap.CPU.TempC, 1e-9, "轻量采集也更新温度")
	assert.Equal(t, int64(352*3600+6*60+17), snap.System.UptimeSeconds)
	assert.Empty(t, snap.Pools)
	assert.NotContains(t, snap.Facets, FacetPools)
	assert.NotContains(t, snap.Facets, FacetAlerts)
	assert.Nil(t, s.TakeExtra(), "没有两步验证的账号不发设备令牌")
}

func TestSynologySessionExpired(t *testing.T) {
	for _, code := range []int{synoErrTimeout, synoErrInterrupted, synoErrInvalidToken} {
		f := newFakeDSM(t, false)
		s := f.source(f.config())
		_, err := s.Collect(context.Background(), false)
		require.NoError(t, err)
		f.resetRequests()
		f.mu.Lock()
		f.sessionCodes = []int{code}
		f.mu.Unlock()

		snap, err := s.Collect(context.Background(), false)
		require.NoError(t, err, code)
		assert.Equal(t, []string{
			"SYNO.Core.System.Utilization get", "SYNO.API.Auth login", "SYNO.Core.System.Utilization get",
			"SYNO.Core.System info",
		}, f.calls(), "错误码 %d：重新登录一次再调", code)
		assert.Equal(t, FacetOK, snap.Facets[FacetCPU].Status)
		assert.Equal(t, "sid-2-abcdef", f.requests()[2].form.Get("_sid"))
	}

	f := newFakeDSM(t, false)
	s := f.source(f.config())
	f.mu.Lock()
	f.sessionCodes = []int{synoErrTimeout, synoErrInterrupted}
	f.mu.Unlock()
	_, err := s.Collect(context.Background(), false)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrAuth, "登录是成功的：不是认证失败，按退避重试")
	assert.Contains(t, err.Error(), "重新登录以后还是说会话失效（错误码 107）")
	assert.Equal(t, 2, f.count("SYNO.API.Auth login"), "只重新登录一次")
}

func TestSynologyLoginErrors(t *testing.T) {
	cases := []struct {
		code   int
		test   bool
		reason string
		otp    bool
	}{
		{400, false, "群晖说用户名或密码不对", false},
		{401, false, "群晖说这个账号停用了（或是访客账号）", false},
		{402, false, "群晖说这个账号没有登录的权限：在 DSM 里给它允许「DSM」这个应用", false},
		{403, false, "DSM 要验证码（设备令牌失效了，或者账号刚开了两步验证），到「NAS 监控」里编辑这个来源、填验证码重新测一次", true},
		{406, false, "DSM 要验证码（设备令牌失效了，或者账号刚开了两步验证），到「NAS 监控」里编辑这个来源、填验证码重新测一次", true},
		{403, true, "这个账号开了两步验证，填一次验证码；以后 pt-tools 用这台设备的令牌登录，不用再填", true},
		{406, true, "DSM 要求这个账号开两步验证：先用它登录一次 DSM 网页设置好，再填验证码", true},
		{404, true, "验证码不对或过期了，重新输入", true},
		{407, false, "登录失败太多次，DSM 封锁了 pt-tools 所在的 IP：在 DSM 的「Control Panel → Security → Protection」里解除", false},
		{408, false, "这个账号的密码过期了，先在 DSM 网页上改密码", false},
		{410, false, "这个账号的密码过期了，先在 DSM 网页上改密码", false},
		{999, false, "群晖登录失败（错误码 999）", false},
	}
	for _, c := range cases {
		f := newFakeDSM(t, false)
		f.loginCodes = []int{c.code}
		var err error
		if c.test {
			_, err = testSynology(context.Background(), f.testInput(), nil)
		} else {
			_, err = f.source(f.config()).Collect(context.Background(), true)
		}
		var ae *AuthError
		require.ErrorAs(t, err, &ae, c.code)
		assert.Equal(t, c.reason, ae.Reason, c.code)
		assert.Equal(t, c.otp, ae.OTP, c.code)
		assert.Equal(t, 1, f.count("SYNO.API.Auth login"), "错误码 %d：登录失败一次就停，不反复试", c.code)
		assert.Equal(t, 0, f.count("SYNO.Core.System info"), c.code)
	}

	// 真的密码不对（假 DSM 按账号判）
	f := newFakeDSM(t, false)
	cfg := f.config()
	cfg.Secret = "wrong-pass"
	_, err := f.source(cfg).Collect(context.Background(), true)
	require.ErrorIs(t, err, ErrAuth)
	assert.NotContains(t, err.Error(), "wrong-pass")
	state, msg, parked := classify(err, defaultTimeout)
	assert.Equal(t, StateAuthFailed, state)
	assert.Equal(t, "认证失败：群晖说用户名或密码不对", msg)
	assert.True(t, parked, "认证失败停住，不按退避重试")
}

func TestSynologyTwoFactor(t *testing.T) {
	f := newFakeDSM(t, false)
	f.otp = "123456"
	ctx := context.Background()

	_, err := testSynology(ctx, f.testInput(), nil)
	var ae *AuthError
	require.ErrorAs(t, err, &ae)
	assert.True(t, ae.OTP, "没填验证码：让用户填")

	in := f.testInput()
	in.OTP = "000000"
	_, err = testSynology(ctx, in, nil)
	require.ErrorAs(t, err, &ae)
	assert.True(t, ae.OTP)
	assert.Equal(t, "验证码不对或过期了，重新输入", ae.Reason)

	f.resetRequests()
	in.OTP = " 123456 "
	res, err := testSynology(ctx, in, nil)
	require.NoError(t, err)
	assert.Equal(t, "DS920+", res.System.Model)
	assert.Equal(t, map[string]string{synologyDeviceKey: "dev-token-1"}, res.Extra, "登录得到的设备令牌交给保存")
	login := f.requests()[1]
	assert.Equal(t, "123456", login.form.Get("otp_code"), "验证码去掉空白")
	assert.Equal(t, []string{
		"SYNO.API.Info query", "SYNO.API.Auth login", "SYNO.Core.System info", "SYNO.API.Auth logout",
	}, f.calls(), "测试完注销")
	assert.Equal(t, 0, f.liveSessions())

	// 带着存下的设备令牌：不用验证码，令牌没变时不再交出
	in = f.testInput()
	in.Extra = res.Extra
	res, err = testSynology(ctx, in, nil)
	require.NoError(t, err)
	assert.Nil(t, res.Extra)

	cfg := f.config()
	cfg.Extra = map[string]string{synologyDeviceKey: "dev-token-1"}
	s := f.source(cfg)
	f.resetRequests()
	_, err = s.Collect(ctx, true)
	require.NoError(t, err)
	login = f.requests()[1]
	assert.Equal(t, "dev-token-1", login.form.Get("device_id"))
	assert.NotContains(t, login.form, "otp_code")
	assert.Nil(t, s.TakeExtra())

	// 设备令牌在 DSM 上被删了：采集停下来，等用户重新测
	cfg.Extra = map[string]string{synologyDeviceKey: "revoked"}
	_, err = f.source(cfg).Collect(ctx, true)
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, "DSM 要验证码（设备令牌失效了，或者账号刚开了两步验证），到「NAS 监控」里编辑这个来源、填验证码重新测一次", ae.Reason)
}

func TestSynologyDeviceTokenRefresh(t *testing.T) {
	for _, field := range []string{"did", "device_id"} {
		f := newFakeDSM(t, false)
		f.issueAlways, f.tokenField = true, field
		s := f.source(f.config())
		_, err := s.Collect(context.Background(), true)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{synologyDeviceKey: "dev-token-1"}, s.TakeExtra(), field)
		assert.Nil(t, s.TakeExtra(), "交出一次以后没有新的")

		// 会话失效后重新登录拿到另一个令牌：再交出一次，下次登录带新的
		f.mu.Lock()
		f.issue = "dev-token-2"
		f.sessionCodes = []int{synoErrTimeout}
		f.mu.Unlock()
		_, err = s.Collect(context.Background(), false)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{synologyDeviceKey: "dev-token-2"}, s.TakeExtra(), field)
		f.mu.Lock()
		f.sessionCodes = []int{synoErrTimeout}
		f.mu.Unlock()
		f.resetRequests()
		_, err = s.Collect(context.Background(), false)
		require.NoError(t, err)
		assert.Equal(t, "dev-token-2", f.requests()[1].form.Get("device_id"))
		assert.Nil(t, s.TakeExtra())
	}

	// 两个字段都有时用 device_id（API 版本 7 起的字段）
	f := newFakeDSM(t, false)
	s := f.source(f.config())
	s.apis = map[string]synoAPI{synoAPIAuth: {Path: "entry.cgi", MaxVersion: 7}}
	srv := newSynologyLoginStub(t, `{"data":{"sid":"x","did":"old","device_id":"new"},"success":true}`)
	s.base = srv
	require.NoError(t, s.login(context.Background(), "", false))
	assert.Equal(t, map[string]string{synologyDeviceKey: "new"}, s.TakeExtra())
}

// newSynologyLoginStub 是只回一份登录回应的服务。
func newSynologyLoginStub(t *testing.T, body string) *url.URL {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	u, err := url.Parse(ts.URL)
	require.NoError(t, err)
	return u
}

func TestSynologyPermissionDenied(t *testing.T) {
	f := newFakeDSM(t, false)
	f.set(synoAPIUtilization+" get", dsmReply{code: synoErrPermission})
	f.set(synoAPISystem+" info", dsmReply{code: synoErrPermission})
	f.set(synoAPIStorage+" load_info", dsmReply{code: synoErrPermission})
	f.set(synoAPISystem+" info storage", dsmReply{code: synoErrPermission})
	snap, err := f.source(f.config()).Collect(context.Background(), true)
	require.NoError(t, err, "没权限只记在那几类上，采集照样成功")
	denied := func(what string) Facet {
		return Facet{Status: FacetDenied, Reason: "这个账号没有读" + what + "的权限：DSM 只让管理员组（administrators）的账号读"}
	}
	assert.Equal(t, denied("资源监控"), snap.Facets[FacetCPU])
	assert.Equal(t, denied("资源监控"), snap.Facets[FacetMemory])
	assert.Equal(t, denied("资源监控"), snap.Facets[FacetNetwork])
	assert.Equal(t, denied("主机信息"), snap.Facets[FacetSystem])
	assert.Equal(t, denied("系统温度"), snap.Facets[FacetCPUTemp])
	assert.Equal(t, denied("硬盘"), snap.Facets[FacetDisks])
	assert.Equal(t, denied("存储空间"), snap.Facets[FacetPools])
	assert.Equal(t, Memory{}, snap.Memory)
	assert.Nil(t, snap.CPU.TempC)
	assert.Empty(t, snap.Pools)

	_, err = testSynology(context.Background(), f.testInput(), nil)
	var ae *AuthError
	require.ErrorAs(t, err, &ae, "测试连接时读不到主机信息：告诉用户要管理员组的账号")
	assert.Equal(t, synologyNotAdmin, ae.Reason)
	assert.False(t, ae.OTP)
	assert.Equal(t, "SYNO.API.Auth logout", f.calls()[len(f.calls())-1], "失败了也注销")
}

func TestSynologyStorageFallback(t *testing.T) {
	f := newFakeDSM(t, false)
	f.set(synoAPIStorage+" load_info", dsmReply{code: 117})
	snap, err := f.source(f.config()).Collect(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, Facet{Status: FacetError, Reason: "DSM 读硬盘回了错误码 117"}, snap.Facets[FacetDisks])
	assert.Empty(t, snap.Disks)
	assert.Equal(t, Facet{Status: FacetOK}, snap.Facets[FacetPools], "存储空间改读 type=storage")
	yes, no := true, false
	assert.Equal(t, []Pool{
		{Name: "/volume1", Status: "正常", Healthy: &yes, Size: 3829364490240, Used: 1530000000000, Free: 2299364490240},
		{Name: "/volume2", Status: "降级", Healthy: &no, Size: 380107374182, Used: 95026843545, Free: 285080530637},
	}, snap.Pools, "大小有的写成字符串、有的写成数")
	var legacy *dsmReq
	for _, r := range f.requests() {
		if r.form.Get("type") != "" {
			legacy = &r
		}
	}
	require.NotNil(t, legacy)
	assert.Equal(t, `"storage"`, legacy.form.Get("type"), "同 Homepage 的群晖小组件：type 的值带引号")

	// 这台 DSM 没有存储接口：硬盘写不支持
	f = newFakeDSM(t, false)
	delete(f.apis, synoAPIStorage)
	f.set(synoAPISystem+" info storage", dsmReply{code: 117})
	snap, err = f.source(f.config()).Collect(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, Facet{Status: FacetUnsupported, Reason: "这台 DSM 读不到硬盘（没有这个接口）"}, snap.Facets[FacetDisks])
	assert.Equal(t, Facet{Status: FacetError, Reason: "DSM 读存储空间回了错误码 117"}, snap.Facets[FacetPools])
	assert.Equal(t, 0, f.count("SYNO.Storage.CGI.Storage load_info"), "没有的接口不调")
}

func TestSynologyNotDSM(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		reply dsmReply
		want  string
	}{
		{"404", dsmReply{status: http.StatusNotFound, body: "not found"}, errSynoNoAPI.Error()},
		{"网页", dsmReply{status: http.StatusOK, body: "<html>login</html>"}, errSynoNotDSM.Error()},
		{"别的 JSON", dsmReply{status: http.StatusOK, body: `{"detail":"Not Found"}`}, errSynoNotDSM.Error()},
		{"500", dsmReply{status: http.StatusInternalServerError, body: "boom"}, "群晖的 SYNO.API.Info 回了 HTTP 500"},
		{"错误码", dsmReply{code: 101}, "群晖的 SYNO.API.Info 回了错误码 101"},
		{"没有登录接口", dsmReply{data: `{"SYNO.Core.System":{"path":"entry.cgi","maxVersion":3}}`}, errSynoNoAuthAPI.Error()},
		{"路径不对", dsmReply{data: `{"SYNO.API.Auth":{"path":"../../etc/x.cgi","maxVersion":7}}`}, errSynoNoAuthAPI.Error()},
		{
			"转到 https",
			dsmReply{status: http.StatusFound, header: map[string]string{"Location": "https://ds:5001/webapi/query.cgi?_sid=leak#x"}},
			"群晖回了重定向（HTTP 302，到 https://ds:5001/webapi/query.cgi）：DSM 开了 HTTPS 跳转，地址改成 https://（默认端口 5001）",
		},
		{
			"转到别处",
			dsmReply{status: http.StatusMovedPermanently, header: map[string]string{"Location": "/login"}},
			"这个地址回了重定向（HTTP 301，到 /login），不像群晖 DSM 的接口：地址或端口对吗？DSM 默认端口是 5000（http）、5001（https）",
		},
	}
	for _, c := range cases {
		f := newFakeDSM(t, false)
		rep := c.reply
		f.info = &rep
		_, err := testSynology(ctx, f.testInput(), nil)
		require.Error(t, err, c.name)
		assert.Equal(t, c.want, err.Error(), c.name)
		assert.NotContains(t, err.Error(), "leak", c.name)
		assert.Equal(t, 0, f.count("SYNO.API.Auth login"), "%s：不是 DSM 时不发账号", c.name)
		_, err = f.source(f.config()).Collect(ctx, true)
		assert.Equal(t, c.want, err.Error(), c.name)
	}

	// 登录以后某个接口回了不是 DSM 的东西：整次放弃
	f := newFakeDSM(t, false)
	f.set(synoAPIUtilization+" get", dsmReply{status: http.StatusBadGateway, body: "bad gateway"})
	_, err := f.source(f.config()).Collect(ctx, true)
	require.EqualError(t, err, "群晖的 SYNO.Core.System.Utilization 回了 HTTP 502")
	f.set(synoAPIUtilization+" get", dsmReply{data: `{"cpu":"oops"}`})
	_, err = f.source(f.config()).Collect(ctx, true)
	require.ErrorContains(t, err, "群晖的 SYNO.Core.System.Utilization 回应格式不对")
	f.set(synoAPIUtilization+" get", dsmReply{status: http.StatusOK, body: strings.Repeat(" ", synologyMaxBody+1)})
	_, err = f.source(f.config()).Collect(ctx, true)
	require.EqualError(t, err, "群晖的 SYNO.Core.System.Utilization 回应太大了")
	f.set(synoAPIUtilization+" get", dsmReply{status: http.StatusOK, body: `{"success":false}`})
	snap, err := f.source(f.config()).Collect(ctx, true)
	require.NoError(t, err)
	assert.Equal(t, Facet{Status: FacetError, Reason: "DSM 读资源监控回了错误码 100"}, snap.Facets[FacetCPU],
		"失败却没有错误码：按 100（未知错误）")

	// 登录成功却没有会话
	f = newFakeDSM(t, false)
	s := f.source(f.config())
	s.apis = map[string]synoAPI{synoAPIAuth: {Path: "entry.cgi", MaxVersion: 7}}
	s.base = newSynologyLoginStub(t, `{"data":{},"success":true}`)
	require.EqualError(t, s.login(ctx, "", false), "群晖登录成功了，却没有回会话（sid）")
}

func TestSynologyAPIVersion(t *testing.T) {
	assert.Equal(t, 7, synoAPI{MaxVersion: 9}.version(synoAPIAuth), "超过写好的版本时用写好的")
	assert.Equal(t, 6, synoAPI{MaxVersion: 6}.version(synoAPIAuth))
	assert.Equal(t, 3, synoAPI{MaxVersion: 3}.version(synoAPISystem))
	assert.Equal(t, 2, synoAPI{MinVersion: 2, MaxVersion: 2}.version(synoAPIStorage), "不低于它支持的最低版本")
	assert.Equal(t, 1, synoAPI{}.version("SYNO.Other"))
	assert.Equal(t, 4, synoAPI{MaxVersion: 4}.version("SYNO.Other"), "没写上限的接口用它的最高版本")

	f := newFakeDSM(t, false)
	f.apis[synoAPIAuth] = synoAPI{Path: "auth.cgi", MinVersion: 1, MaxVersion: 6}
	s := f.source(f.config())
	_, err := s.Collect(context.Background(), true)
	require.ErrorContains(t, err, "群晖的 SYNO.API.Auth 回了 HTTP 404", "登录发到 Info 给的路径（假 DSM 只有 entry.cgi）")
	reqs := f.requests()
	assert.Equal(t, "/webapi/auth.cgi", reqs[1].path)
	assert.Equal(t, "6", reqs[1].query.Get("version"))
}

func TestSynologyMappings(t *testing.T) {
	for in, want := range map[string]int64{
		"48:30:15": 48*3600 + 30*60 + 15, "0:0:5": 5, " 12:01:02 ": 12*3600 + 62, "1 day, 2:03:04": 0, "": 0, "x": 0,
	} {
		assert.Equal(t, want, parseSynologyUptime(in), in)
	}

	sys := synoSystemInfo{VersionStr: "DSM 6.2.4-25556", Model: " DS918+ ", CPUCores: synoNumOf(t, `"x"`)}.system()
	assert.Equal(t, System{Platform: "DSM", Version: "6.2.4-25556", Model: "DS918+"}, sys, "没有 firmware_ver 时用 version_string")
	assert.Equal(t, "7.1", synoSystemInfo{FirmwareVer: "7.1"}.system().Version)
	for _, raw := range []string{`0`, `-1`, `200`, `null`, `"?"`} {
		_, ok := synoSystemInfo{SysTemp: synoNumOf(t, raw)}.temp()
		assert.False(t, ok, raw)
	}

	cases := []struct {
		name string
		util string
		cpu  Facet
		mem  Facet
		net  Facet
		want Snapshot
	}{
		{
			name: "空的",
			util: `{}`,
			cpu:  Facet{Status: FacetError, Reason: "DSM 没给 CPU 占用"},
			mem:  Facet{Status: FacetError, Reason: "DSM 没给内存"},
			net:  Facet{Status: FacetError, Reason: "DSM 没给网络的合计（total）"},
		},
		{
			name: "没有各项占用",
			util: `{"cpu":{"1min_load":50},"memory":{"total_real":0,"memory_size":0},"network":[{"device":"eth0","rx":1,"tx":2}]}`,
			cpu:  Facet{Status: FacetError, Reason: "DSM 没给 CPU 占用"},
			mem:  Facet{Status: FacetError, Reason: "DSM 没给内存总量"},
			net:  Facet{Status: FacetError, Reason: "DSM 没给网络的合计（total）"},
		},
		{
			name: "没有 real_usage：按空闲、缓冲与缓存算",
			util: `{"cpu":{"user_load":"95","system_load":"9"},"memory":{"memory_size":"1000","avail_real":"100","buffer":50,"cached":"250"},
				"network":[{"device":"total","rx":"-3","tx":7.9}]}`,
			cpu: Facet{Status: FacetOK}, mem: Facet{Status: FacetOK}, net: Facet{Status: FacetOK},
			want: Snapshot{
				CPU:     CPU{Percent: 100},
				Memory:  Memory{Total: 1000 * 1024, Used: 600 * 1024, Available: 400 * 1024},
				Network: Network{TxBps: 7, Interfaces: []string{}},
			},
		},
		{
			name: "空闲比总量还多",
			util: `{"cpu":{"other_load":3,"5min_load":-20},"memory":{"total_real":1000,"avail_real":900,"buffer":900,"cached":900},
				"network":[{"device":"total"},{"device":""}]}`,
			cpu: Facet{Status: FacetOK}, mem: Facet{Status: FacetOK}, net: Facet{Status: FacetOK},
			want: Snapshot{
				CPU:     CPU{Percent: 3},
				Memory:  Memory{Total: 1000 * 1024, Available: 1000 * 1024},
				Network: Network{Interfaces: []string{}},
			},
		},
		{
			name: "没有用量",
			util: `{"cpu":{"user_load":1},"memory":{"total_real":1000},"network":[]}`,
			cpu:  Facet{Status: FacetOK},
			mem:  Facet{Status: FacetError, Reason: "DSM 没给内存用量"},
			net:  Facet{Status: FacetError, Reason: "DSM 没给网络的合计（total）"},
			want: Snapshot{CPU: CPU{Percent: 1}},
		},
		{
			name: "real_usage 超过 100",
			util: `{"cpu":{"user_load":1},"memory":{"total_real":1000,"real_usage":150},"network":[{"device":"total","rx":1,"tx":1}]}`,
			cpu:  Facet{Status: FacetOK}, mem: Facet{Status: FacetOK}, net: Facet{Status: FacetOK},
			want: Snapshot{
				CPU:     CPU{Percent: 1},
				Memory:  Memory{Total: 1000 * 1024, Used: 1000 * 1024},
				Network: Network{RxBps: 1, TxBps: 1, Interfaces: []string{}},
			},
		},
	}
	for _, c := range cases {
		var u synoUtilization
		require.NoError(t, json.Unmarshal([]byte(c.util), &u), c.name)
		snap := &Snapshot{}
		snap.Network.Interfaces = []string{}
		assert.Equal(t, c.cpu, u.fillCPU(snap), c.name)
		assert.Equal(t, c.mem, u.fillMemory(snap), c.name)
		assert.Equal(t, c.net, u.fillNetwork(snap), c.name)
		if c.want.Network.Interfaces == nil {
			c.want.Network.Interfaces = []string{}
		}
		assert.Equal(t, c.want.CPU, snap.CPU, c.name)
		assert.Equal(t, c.want.Memory, snap.Memory, c.name)
		assert.Equal(t, c.want.Network, snap.Network, c.name)
	}
}

func TestSynologyStorageMapping(t *testing.T) {
	var st synoStorage
	require.NoError(t, json.Unmarshal([]byte(`{
		"disks":[
		 {"id":"nvme1n1","longName":"M.2 Drive 2","isSsd":true,"temp":-1},
		 {"id":"sas1","name":"","diskType":"SAS","temp":"42.5"},
		 {"id":"usb1","name":"USB","isSsd":false,"size_total":"12.7","used_by":"reuse_9"}],
		"storagePools":[{"id":"reuse_1","disks":["sas1","usb1"],"pool_child":[{"id":"volume_3"},{"id":""},{"id":"volume_5"}]},
		 {"id":"reuse_2","disks":["nvme1n1"],"deploy_path":"volume_4"}],
		"volumes":[
		 {"id":"volume_3","status":"repairing","size":{"total":"100","used":"300"}},
		 {"id":"volume_4","vol_path":" /volume4 ","status":"Crashed","size":{"total":"0","used":"0"}},
		 {"id":"vg1","status":"attention"},
		 {"id":"volume_5","status":"normal","size":{"total":10,"used":1}}]}`), &st))
	yes, no := true, false
	assert.Equal(t, []Pool{
		{Name: "/volume3", Status: "repairing", Size: 100, Used: 100, Free: 0, Disks: 2},
		{Name: "/volume4", Status: "已损毁", Healthy: &no, Disks: 1},
		{Name: "vg1", Status: "attention"},
		{Name: "/volume5", Status: "正常", Healthy: &yes, Size: 10, Used: 1, Free: 9, Disks: 2},
	}, st.pools(), "不认识的状态照原样写、不说健不健康；已用不超过总量；没有路径时按编号写")
	t42 := 42.5
	assert.Equal(t, []Disk{
		{Name: "M.2 Drive 2", Kind: DiskNVMe, Pool: "/volume4"},
		{Name: "sas1", Kind: DiskUnknown, TempC: &t42, Pool: "/volume3"},
		{Name: "USB", Kind: DiskHDD, Size: 12, Pool: "/volume3"},
	}, st.disks(), "名字依次用 name、longName、编号；-1 °C 不是温度；同一块盘只记第一个存储空间")

	f := newFakeDSM(t, false)
	f.set(synoAPIStorage+" load_info", dsmReply{data: `null`})
	snap, err := f.source(f.config()).Collect(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, Facet{Status: FacetOK}, snap.Facets[FacetPools], "一个存储空间也没有")
	assert.Empty(t, snap.Pools)
	assert.Empty(t, snap.Disks)
}

func synoNumOf(t *testing.T, raw string) synoNum {
	t.Helper()
	var n synoNum
	require.NoError(t, json.Unmarshal([]byte(raw), &n))
	return n
}

func TestSynoNum(t *testing.T) {
	for raw, want := range map[string]uint64{
		`4000787030016`:          4000787030016,
		`"4000787030016"`:        4000787030016,
		`"18446744073709551615"`: 18446744073709551615,
		`"9007199254740993"`:     9007199254740993, // 2^53+1：按 float64 解会丢精度
		`" 7 "`:                  7,
		`1.9`:                    1,
		`-5`:                     0,
		`"-5"`:                   0,
		`1e3`:                    1000,
	} {
		v, ok := synoNumOf(t, raw).uint()
		assert.True(t, ok, raw)
		assert.Equal(t, want, v, raw)
	}
	for _, raw := range []string{`null`, `"x"`, `""`, `true`, `{}`, `"NaN"`, `"Inf"`} {
		n := synoNumOf(t, raw)
		_, ok := n.uint()
		assert.False(t, ok, raw)
		_, ok = n.float()
		assert.False(t, ok, raw)
	}
	f, ok := synoNumOf(t, `"46.5"`).float()
	assert.True(t, ok)
	assert.InDelta(t, 46.5, f, 1e-9)
	var bad synoNum
	require.NoError(t, bad.UnmarshalJSON([]byte(`"unterminated`)))
	assert.False(t, bad.ok)
}

func TestSynologyClose(t *testing.T) {
	f := newFakeDSM(t, false)
	s := f.source(f.config())
	require.NoError(t, s.Close(), "还没登录：什么也不发")
	assert.Empty(t, f.requests())
	_, err := s.Collect(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, 1, f.liveSessions())
	f.resetRequests()
	require.NoError(t, s.Close())
	reqs := f.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "SYNO.API.Auth logout", reqs[0].api())
	assert.Equal(t, "sid-1-abcdef", reqs[0].form.Get("_sid"))
	assert.NotContains(t, reqs[0].query, "_sid")
	assert.Equal(t, 0, f.liveSessions(), "关掉来源时注销会话")
	require.NoError(t, s.Close())
	assert.Len(t, f.requests(), 1, "注销过了不再发")
}

func TestSynologyCertificate(t *testing.T) {
	f := newFakeDSM(t, true)
	ctx := context.Background()
	_, err := testSynology(ctx, f.testInput(), nil)
	var ce *CertError
	require.ErrorAs(t, err, &ce)
	assert.ErrorIs(t, err, ErrCertUntrusted)
	assert.Empty(t, f.requests(), "证书不认：握手时就断开，什么也没发出")

	in := f.testInput()
	in.PinSHA256 = ce.Cert.SHA256
	res, err := testSynology(ctx, in, nil)
	require.NoError(t, err, "信任这张证书以后连得上")
	assert.Equal(t, "DS920+", res.System.Model)

	cfg := f.config()
	cfg.PinSHA256 = strings.Repeat("0", 64)
	s, err := newSynology(cfg, nil)
	require.NoError(t, err)
	_, err = s.Collect(ctx, true)
	require.ErrorIs(t, err, ErrCertChanged)
	assert.NotContains(t, err.Error(), f.ts.URL, "去掉 Go 加的「Get \"地址\":」前缀")
	require.NoError(t, s.Close())
}

func TestSynologyContextAndNetwork(t *testing.T) {
	f := newFakeDSM(t, false)
	s := f.source(f.config())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Collect(ctx, true)
	require.ErrorIs(t, err, context.Canceled)

	cfg := f.config()
	f.ts.Close()
	_, err = f.source(cfg).Collect(context.Background(), true)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "连不上群晖："), err.Error())
	assert.False(t, errors.Is(err, ErrAuth))
}

func TestSynologyCallFailures(t *testing.T) {
	ctx := context.Background()
	bad := dsmReply{status: http.StatusBadGateway, body: "bad gateway"}
	for _, key := range []string{synoAPISystem + " info", synoAPIStorage + " load_info"} {
		f := newFakeDSM(t, false)
		f.set(key, bad)
		_, err := f.source(f.config()).Collect(ctx, true)
		require.Error(t, err, key)
		assert.Contains(t, err.Error(), "回了 HTTP 502", key)
	}
	f := newFakeDSM(t, false)
	f.set(synoAPIStorage+" load_info", dsmReply{code: 117})
	f.set(synoAPISystem+" info storage", bad)
	_, err := f.source(f.config()).Collect(ctx, true)
	require.EqualError(t, err, "群晖的 SYNO.Core.System 回了 HTTP 502", "改读 type=storage 也出错：整次放弃")

	// 出错以后下一次采集重新查接口列表（会话还在，不重新登录）
	f = newFakeDSM(t, false)
	s := f.source(f.config())
	f.set(synoAPIUtilization+" get", bad, dsmReply{data: dsmUtilization})
	_, err = s.Collect(ctx, false)
	require.Error(t, err)
	f.resetRequests()
	_, err = s.Collect(ctx, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"SYNO.API.Info query", "SYNO.Core.System.Utilization get", "SYNO.Core.System info"}, f.calls())

	// 测试连接时读主机信息出错
	f = newFakeDSM(t, false)
	f.set(synoAPISystem+" info", bad)
	_, err = testSynology(ctx, f.testInput(), nil)
	require.EqualError(t, err, "群晖的 SYNO.Core.System 回了 HTTP 502")
	assert.Equal(t, "SYNO.API.Auth logout", f.calls()[len(f.calls())-1], "失败了也注销")
	f.set(synoAPISystem+" info", dsmReply{code: 117})
	_, err = testSynology(ctx, f.testInput(), nil)
	require.EqualError(t, err, "DSM 读主机信息回了错误码 117")
	assert.NotErrorIs(t, err, ErrAuth)

	// 经登记的类型测试连接；系统温度是 0 时写读不到
	f = newFakeDSM(t, false)
	f.set(synoAPISystem+" info", dsmReply{data: strings.Replace(dsmSystemInfo, `"sys_temp":46`, `"sys_temp":0`, 1)})
	k, _ := LookupKind(KindSynology)
	res, err := k.Test(ctx, f.testInput())
	require.NoError(t, err)
	assert.Equal(t, "DS920+", res.System.Model)
	snap, err := f.source(f.config()).Collect(ctx, false)
	require.NoError(t, err)
	assert.Nil(t, snap.CPU.TempC)
	assert.Equal(t, Facet{Status: FacetUnsupported, Reason: "DSM 没给系统温度"}, snap.Facets[FacetCPUTemp])
}

func TestSynologyBodyCut(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, `{"data":`)
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler) // 回到一半断开
	}))
	t.Cleanup(ts.Close)
	u, err := url.Parse(ts.URL)
	require.NoError(t, err)
	_, err = testSynology(context.Background(), TestInput{URL: u, Username: "u", Secret: "p"}, nil)
	require.ErrorContains(t, err, "读群晖的回应断了")
}

func TestSynologyLogoutAfterFailure(t *testing.T) {
	ctx := context.Background()
	// 采集出错以后关掉来源：照旧注销（接口列表要重新查，但旧的那份还认得注销的路径）
	f := newFakeDSM(t, false)
	s := f.source(f.config())
	f.set(synoAPIUtilization+" get", dsmReply{status: http.StatusBadGateway, body: "bad gateway"})
	_, err := s.Collect(ctx, false)
	require.Error(t, err)
	f.resetRequests()
	require.NoError(t, s.Close())
	reqs := f.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "SYNO.API.Auth logout", reqs[0].api())
	assert.Equal(t, "/webapi/entry.cgi", reqs[0].path)
	assert.Equal(t, 0, f.liveSessions())

	// 测试连接时会话失效、重新登录又失败：没有会话可注销，不再发
	f = newFakeDSM(t, false)
	f.loginCodes = []int{0, 400}
	f.sessionCodes = []int{synoErrTimeout}
	_, err = testSynology(ctx, f.testInput(), nil)
	require.ErrorIs(t, err, ErrAuth)
	assert.Equal(t, []string{
		"SYNO.API.Info query", "SYNO.API.Auth login", "SYNO.Core.System info", "SYNO.API.Auth login",
	}, f.calls())
}
