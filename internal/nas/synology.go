package nas

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// KindSynology 是群晖 DSM 来源（实验性：只用假服务测过，没在真机上验证）：DSM 的 Web API（/webapi/*.cgi），
// 用账号密码登录，开了两步验证的账号第一次测试时填验证码，之后带设备令牌登录。
//
// 请求与回应照 py-synologydsm-api（Home Assistant 的群晖集成用它）与 Homarr 的群晖集成记录的格式写，没有搬它们的代码：
// 先用 SYNO.API.Info 查各接口的路径与版本，再用 SYNO.API.Auth 登录拿会话（sid），之后的请求都带上它。
const KindSynology = "synology"

func init() {
	RegisterKind(Kind{
		Name:         KindSynology,
		Label:        "群晖",
		Experimental: true,
		Username:     UsernameRequired,
		Plain:        PlainWithoutCredentials,
		ParseURL:     synologyParseURL,
		New: func(cfg SourceConfig) (Source, error) {
			s, err := newSynology(cfg, nil)
			if err != nil {
				return nil, err
			}
			return s, nil
		},
		Test: func(ctx context.Context, in TestInput) (*TestResult, error) { return testSynology(ctx, in, nil) },
	})
}

const (
	// synologyHTTPPort 是 DSM 网页的 http 端口：http:// 地址没写端口时补上（https:// 多半是反向代理或改过的端口，不补）
	synologyHTTPPort = "5000"
	// synologyTimeout 是一次请求最多等多久
	synologyTimeout = 15 * time.Second
	// synologyMaxBody 是一个回应最多读多少字节（硬盘多的时候 load_info 最大）
	synologyMaxBody = 4 << 20
	// synologyDeviceName 是登录时报给 DSM 的设备名：DSM 的「受信任的设备」里显示它
	synologyDeviceName = "pt-tools"
	// synologyDeviceKey 是设备令牌在长期凭据（Extra）里的键
	synologyDeviceKey = "device_id"
	// synologyLogoutTimeout 是关掉来源时注销会话最多等多久（NAS 连不上时不拖住关闭）
	synologyLogoutTimeout = 2 * time.Second
)

// DSM 的各个接口。
const (
	synoAPIInfo        = "SYNO.API.Info"
	synoAPIAuth        = "SYNO.API.Auth"
	synoAPISystem      = "SYNO.Core.System"
	synoAPIUtilization = "SYNO.Core.System.Utilization"
	synoAPIStorage     = "SYNO.Storage.CGI.Storage"
)

// synoAPIs 是向 SYNO.API.Info 查的接口。
var synoAPIs = []string{synoAPIAuth, synoAPISystem, synoAPIUtilization, synoAPIStorage}

// synoVersionCap 是各接口最高用到的版本：回应的格式按这些版本写，DSM 以后出了新版本也先用这些
// （SYNO.API.Auth 7 起回 device_id；SYNO.Core.System 照 Homarr 记录的 3）。
var synoVersionCap = map[string]int{synoAPIAuth: 7, synoAPISystem: 3, synoAPIUtilization: 1, synoAPIStorage: 1}

// DSM 的错误码（py-synologydsm-api 的 const.py）。
const (
	synoErrUnknown      = 100
	synoErrNoAPI        = 102
	synoErrNoMethod     = 103
	synoErrNoVersion    = 104
	synoErrPermission   = 105
	synoErrTimeout      = 106
	synoErrInterrupted  = 107
	synoErrInvalidToken = 119
)

var (
	errSynoNoAPI     = errors.New("这个地址没有群晖 DSM 的接口（/webapi/query.cgi 回 404）：地址或端口对吗？DSM 默认端口是 5000（http）、5001（https）")
	errSynoNotDSM    = errors.New("回的不是群晖 DSM 的数据（地址是不是指到了别的网页？）")
	errSynoNoAuthAPI = errors.New("这个地址的接口列表里没有登录接口（SYNO.API.Auth），不像群晖 DSM")
	errSynoNoAccount = errors.New("要填群晖的用户名和密码")
)

// synologyNotAdmin 是账号读不到 SYNO.Core.System 时的原因：DSM 只让管理员组的账号读系统信息、资源监控与存储。
const synologyNotAdmin = "这个账号读不到系统信息：DSM 只让管理员组（administrators）的账号读，见「添加来源」里群晖的准备步骤"

// synologyParseURL：http:// 地址没写端口时补上 DSM 的 http 端口 5000。
func synologyParseURL(u *url.URL) error {
	if u.Scheme == "http" && u.Port() == "" {
		u.Host = net.JoinHostPort(u.Hostname(), synologyHTTPPort)
	}
	return nil
}

// synoAPI 是 SYNO.API.Info 回的一个接口：路径（/webapi/ 下的 .cgi）与支持的版本。
type synoAPI struct {
	Path       string `json:"path"`
	MinVersion int    `json:"minVersion"`
	MaxVersion int    `json:"maxVersion"`
}

// version 是调 name 这个接口用的版本：不超过 synoVersionCap，也不低于它支持的最低版本。
func (a synoAPI) version(name string) int {
	v := a.MaxVersion
	if c, ok := synoVersionCap[name]; ok && v > c {
		v = c
	}
	return max(v, a.MinVersion, 1)
}

// synoPathRe 是认作接口路径的：/webapi/ 下的一个 .cgi（entry.cgi、auth.cgi…）。别的（带 ..、绝对路径、查询参数）不认。
var synoPathRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_/-]*\.cgi$`)

// synoEnvelope 是 DSM 每个回应的外层：success 为假时 error.code 是错误码。没有 success 的不是 DSM 的回应。
type synoEnvelope struct {
	Success *bool           `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code int `json:"code"`
	} `json:"error"`
}

// synoLoginData 是登录成功时回的：sid 是会话；did（API 版本 6）或 device_id（版本 7 起）是设备令牌。
type synoLoginData struct {
	SID       string `json:"sid"`
	SynoToken string `json:"synotoken"`
	DID       string `json:"did"`
	DeviceID  string `json:"device_id"`
}

// synology 是一个群晖来源：会话（sid）在采集之间复用，失效时重新登录一次。
type synology struct {
	base   *url.URL
	user   string
	pass   string
	client *http.Client

	mu sync.Mutex
	// apis 是 SYNO.API.Info 查到的接口（第一次用到时查）；stale 为真时下一次调用前重新查（上一次调用出错了），
	// 旧的那份留着给注销用。sid、token 是登录拿到的会话与 SynoToken
	apis  map[string]synoAPI
	stale bool
	sid   string
	token string
	// deviceID 是登录时带上的设备令牌（开了两步验证的账号凭它免验证码）；fresh 是登录得到的新令牌，等 TakeExtra 交出
	deviceID string
	fresh    map[string]string
}

func newSynologyClient(u *url.URL, pin string, roots *x509.CertPool) *http.Client {
	return &http.Client{
		Timeout:   synologyTimeout,
		Transport: Transport(u, pin, roots),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func newSynology(cfg SourceConfig, roots *x509.CertPool) (*synology, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("地址不对：%w", err)
	}
	if strings.TrimSpace(cfg.Username) == "" || cfg.Secret == "" {
		return nil, errSynoNoAccount
	}
	if u.Scheme != "https" && !cfg.InsecureAck {
		return nil, ErrPlainNeedsAck
	}
	return &synology{
		base: u, user: cfg.Username, pass: cfg.Secret, client: newSynologyClient(u, cfg.PinSHA256, roots),
		deviceID: cfg.Extra[synologyDeviceKey],
	}, nil
}

// testSynology 是「测试连接」：先不带账号查接口（认出是不是 DSM），再登录（填了验证码时带上）、读主机信息，最后注销。
// 登录得到新的设备令牌时放进 TestResult.Extra，保存时存下。
func testSynology(ctx context.Context, in TestInput, roots *x509.CertPool) (*TestResult, error) {
	if in.URL == nil {
		return nil, errors.New("要填地址")
	}
	if strings.TrimSpace(in.Username) == "" || in.Secret == "" {
		return nil, errSynoNoAccount
	}
	s := &synology{
		base: in.URL, user: in.Username, pass: in.Secret, client: newSynologyClient(in.URL, in.PinSHA256, roots),
		deviceID: in.Extra[synologyDeviceKey],
	}
	defer s.client.CloseIdleConnections()
	if err := s.queryAPIs(ctx); err != nil {
		return nil, err
	}
	if err := s.login(ctx, strings.TrimSpace(in.OTP), true); err != nil {
		return nil, err
	}
	defer s.logout(ctx)
	var info synoSystemInfo
	code, err := s.call(ctx, synoAPISystem, "info", nil, &info)
	switch {
	case err != nil:
		return nil, err
	case code == synoErrPermission:
		return nil, &AuthError{Reason: synologyNotAdmin}
	case code != 0:
		return nil, fmt.Errorf("DSM 读主机信息回了错误码 %d", code)
	}
	return &TestResult{System: info.system(), Extra: s.fresh}, nil
}

// TakeExtra 交出登录得到的新设备令牌（上一次调用以后没有新的时为 nil）。
func (s *synology) TakeExtra() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	x := s.fresh
	s.fresh = nil
	return x
}

// Close 注销会话（最多等 synologyLogoutTimeout）并断开空闲的连接。
func (s *synology) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sid != "" {
		ctx, cancel := context.WithTimeout(context.Background(), synologyLogoutTimeout)
		s.logout(ctx)
		cancel()
	}
	s.client.CloseIdleConnections()
	return nil
}

// queryAPIs 不带账号查各接口的路径与版本（SYNO.API.Info 总在 /webapi/query.cgi）。没有登录接口时不是 DSM；
// 路径不像 .cgi 的接口当作没有。
func (s *synology) queryAPIs(ctx context.Context) error {
	q := url.Values{
		"api": {synoAPIInfo}, "version": {"1"}, "method": {"query"}, "query": {strings.Join(synoAPIs, ",")},
	}
	u := *s.base
	u.Path, u.RawQuery = "/webapi/query.cgi", q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	var data map[string]synoAPI
	code, err := s.do(req, synoAPIInfo, &data)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("群晖的 %s 回了错误码 %d", synoAPIInfo, code)
	}
	apis := map[string]synoAPI{}
	for name, a := range data {
		if synoPathRe.MatchString(a.Path) && !strings.Contains(a.Path, "..") {
			apis[name] = a
		}
	}
	if _, ok := apis[synoAPIAuth]; !ok {
		return errSynoNoAuthAPI
	}
	s.apis, s.stale = apis, false
	return nil
}

// ensureAPIs 查接口列表：查过以后接着用这一份；某次调用出错以后下一次重新查（DSM 升级以后接口的路径或版本可能变了）。
func (s *synology) ensureAPIs(ctx context.Context) error {
	if s.apis != nil && !s.stale {
		return nil
	}
	return s.queryAPIs(ctx)
}

// login 用账号密码登录（otp 不为空时带上验证码，有设备令牌时带上它），记下会话；回了新的设备令牌时记进 fresh。
// test 为真是「测试连接」：要验证码时让用户填（OTP 为真）；采集时要验证码说明设备令牌失效了，停下来等用户重新测。
// 调用方先查过接口列表（queryAPIs 保证里面有登录接口）。
func (s *synology) login(ctx context.Context, otp string, test bool) error {
	form := url.Values{
		"account": {s.user}, "passwd": {s.pass}, "format": {"sid"},
		"enable_device_token": {"yes"}, "device_name": {synologyDeviceName},
	}
	if otp != "" {
		form.Set("otp_code", otp)
	}
	if s.deviceID != "" {
		form.Set("device_id", s.deviceID)
	}
	var data synoLoginData
	code, err := s.post(ctx, s.apis[synoAPIAuth], synoAPIAuth, "login", form, &data)
	if err != nil {
		return err
	}
	if code != 0 {
		return synoLoginError(code, test)
	}
	if data.SID == "" {
		return errors.New("群晖登录成功了，却没有回会话（sid）")
	}
	s.sid, s.token = data.SID, data.SynoToken
	tok := data.DID
	if data.DeviceID != "" {
		tok = data.DeviceID
	}
	if tok != "" && tok != s.deviceID {
		s.deviceID = tok
		s.fresh = map[string]string{synologyDeviceKey: tok}
	}
	return nil
}

// synoLoginError 是登录时的错误码（py-synologydsm-api 的 synology_dsm.py 与 const.py）：都是认证失败，采集器停下来，
// 不再用同一组账号反复试（DSM 的自动封锁按登录失败的次数封 IP）。
func synoLoginError(code int, test bool) error {
	switch code {
	case 400:
		return &AuthError{Reason: "群晖说用户名或密码不对"}
	case 401:
		return &AuthError{Reason: "群晖说这个账号停用了（或是访客账号）"}
	case 402:
		return &AuthError{Reason: "群晖说这个账号没有登录的权限：在 DSM 里给它允许「DSM」这个应用"}
	case 403, 406:
		if !test {
			return &AuthError{Reason: "DSM 要验证码（设备令牌失效了，或者账号刚开了两步验证），到「NAS 监控」里编辑这个来源、填验证码重新测一次", OTP: true}
		}
		if code == 406 {
			return &AuthError{Reason: "DSM 要求这个账号开两步验证：先用它登录一次 DSM 网页设置好，再填验证码", OTP: true}
		}
		return &AuthError{Reason: "这个账号开了两步验证，填一次验证码；以后 pt-tools 用这台设备的令牌登录，不用再填", OTP: true}
	case 404:
		return &AuthError{Reason: "验证码不对或过期了，重新输入", OTP: true}
	case 407:
		return &AuthError{Reason: "登录失败太多次，DSM 封锁了 pt-tools 所在的 IP：在 DSM 的「Control Panel → Security → Protection」里解除"}
	case 408, 409, 410:
		return &AuthError{Reason: "这个账号的密码过期了，先在 DSM 网页上改密码"}
	}
	return &AuthError{Reason: fmt.Sprintf("群晖登录失败（错误码 %d）", code)}
}

// logout 注销会话（没有会话时什么也不发；出错也不管：会话在 DSM 上会自己过期）。
func (s *synology) logout(ctx context.Context) {
	if s.sid == "" {
		return
	}
	_, _ = s.post(ctx, s.apis[synoAPIAuth], synoAPIAuth, "logout", url.Values{"_sid": {s.sid}}, nil)
	s.sid, s.token = "", ""
}

// synoSession 是会话失效的错误码：超时、被同一个账号在别处登录挤掉、会话无效。
func synoSession(code int) bool {
	return code == synoErrTimeout || code == synoErrInterrupted || code == synoErrInvalidToken
}

// call 调 name 这个接口的 method：没登录时先登录；会话失效时重新登录一次再调。返回 DSM 的错误码（0 是成功，
// 接口列表里没有这个接口时是 102）；连不上、证书、认证失败与回应格式不对是 err（这一次采集整个放弃）。
func (s *synology) call(ctx context.Context, name, method string, params url.Values, out any) (int, error) {
	if err := s.ensureAPIs(ctx); err != nil {
		return 0, err
	}
	api, ok := s.apis[name]
	if !ok {
		return synoErrNoAPI, nil
	}
	for retried := false; ; retried = true {
		if s.sid == "" {
			if err := s.login(ctx, "", false); err != nil {
				return 0, err
			}
		}
		form := url.Values{"_sid": {s.sid}}
		if s.token != "" {
			form.Set("SynoToken", s.token)
		}
		for k, v := range params {
			form[k] = v
		}
		code, err := s.post(ctx, api, name, method, form, out)
		if err != nil {
			s.stale = true
			return 0, err
		}
		if !synoSession(code) {
			return code, nil
		}
		s.sid, s.token = "", ""
		if retried {
			return 0, fmt.Errorf("群晖重新登录以后还是说会话失效（错误码 %d）", code)
		}
	}
}

// post 发一个请求：同 py-synologydsm-api 的 POST，参数都放在请求体里，网址里只放 api、version、method，
// 账号密码、会话与令牌不进 NAS 与反向代理的访问日志。
func (s *synology) post(ctx context.Context, api synoAPI, name, method string, form url.Values, out any) (int, error) {
	v := strconv.Itoa(api.version(name))
	u := *s.base
	u.Path = "/webapi/" + api.Path
	u.RawQuery = url.Values{"api": {name}, "version": {v}, "method": {method}}.Encode()
	body := url.Values{}
	for k, vs := range form {
		body[k] = vs
	}
	body.Set("api", name)
	body.Set("version", v)
	body.Set("method", method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(body.Encode()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return s.do(req, name, out)
}

// do 发出请求并解开回应：DSM 的错误放在 HTTP 200 的回应里（success 为假，返回错误码）；别的 HTTP 状态、
// 重定向（不跟着走：可能把会话带到别处）与不是 DSM 格式的回应是 err。连不上、证书不认时返回的错误去掉了 Go 加的
// 「Post "地址":」前缀。
func (s *synology) do(req *http.Request, name string, out any) (int, error) {
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		if ctxErr := req.Context().Err(); ctxErr != nil {
			return 0, ctxErr
		}
		return 0, fmt.Errorf("连不上群晖：%w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, synologyMaxBody+1))
	if err != nil {
		return 0, fmt.Errorf("读群晖的回应断了：%w", err)
	}
	switch {
	case len(body) > synologyMaxBody:
		return 0, fmt.Errorf("群晖的 %s 回应太大了", name)
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return 0, synoRedirect(resp)
	case resp.StatusCode == http.StatusNotFound && req.URL.Path == "/webapi/query.cgi":
		return 0, errSynoNoAPI
	case resp.StatusCode != http.StatusOK:
		return 0, fmt.Errorf("群晖的 %s 回了 HTTP %d", name, resp.StatusCode)
	}
	var env synoEnvelope
	if json.Unmarshal(body, &env) != nil || env.Success == nil {
		return 0, errSynoNotDSM
	}
	if !*env.Success {
		if env.Error == nil {
			return synoErrUnknown, nil
		}
		return env.Error.Code, nil
	}
	if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return 0, fmt.Errorf("群晖的 %s 回应格式不对：%w", name, err)
		}
	}
	return 0, nil
}

// synoRedirect 是回了重定向：转到 https:// 多半是 DSM 开了「自动把 HTTP 连接重定向到 HTTPS」；转到别处说明这个地址不是 DSM。
// 原因里只写目标的协议、主机与路径（不带查询参数）。
func synoRedirect(resp *http.Response) error {
	loc := resp.Header.Get("Location")
	if t, err := url.Parse(loc); err == nil {
		t.User, t.RawQuery, t.ForceQuery, t.Fragment, t.RawFragment = nil, "", false, "", ""
		loc = t.String()
	}
	if strings.HasPrefix(strings.ToLower(loc), "https://") {
		return fmt.Errorf("群晖回了重定向（HTTP %d，到 %s）：DSM 开了 HTTPS 跳转，地址改成 https://（默认端口 5001）", resp.StatusCode, loc)
	}
	return fmt.Errorf("这个地址回了重定向（HTTP %d，到 %s），不像群晖 DSM 的接口：地址或端口对吗？DSM 默认端口是 5000（http）、5001（https）", resp.StatusCode, loc)
}

// synoFacet 是一类数据没读成时的情况（code 是 DSM 的错误码）：105 是账号没有权限，102–104 是这台 DSM 没有这个接口。
func synoFacet(code int, what string) Facet {
	switch code {
	case synoErrPermission:
		return Facet{Status: FacetDenied, Reason: "这个账号没有读" + what + "的权限：DSM 只让管理员组（administrators）的账号读"}
	case synoErrNoAPI, synoErrNoMethod, synoErrNoVersion:
		return Facet{Status: FacetUnsupported, Reason: "这台 DSM 读不到" + what + "（没有这个接口）"}
	}
	return Facet{Status: FacetError, Reason: fmt.Sprintf("DSM 读%s回了错误码 %d", what, code)}
}

// Collect 采集一次：每次读资源监控（CPU、内存、网络）与主机信息（温度、运行时长）；完整采集再读存储。
// 连不上、证书、认证失败时整次失败；某个接口没权限或出错时只记在那一类上。
func (s *synology) Collect(ctx context.Context, full bool) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := &Snapshot{
		At:     time.Now(),
		Pools:  []Pool{},
		Disks:  []Disk{},
		Alerts: []Alert{},
		Facets: map[string]Facet{},
	}
	snap.Network.Interfaces = []string{}
	if err := s.fillUtilization(ctx, snap); err != nil {
		return nil, err
	}
	if err := s.fillSystem(ctx, snap); err != nil {
		return nil, err
	}
	if full {
		if err := s.fillStorage(ctx, snap); err != nil {
			return nil, err
		}
		snap.Facets[FacetAlerts] = Facet{Status: FacetUnsupported, Reason: synologyNoAlerts}
		snap.Facets[FacetApps] = Facet{Status: FacetUnsupported, Reason: synologyNoApps}
	}
	return snap, nil
}
