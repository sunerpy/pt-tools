package nas

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// KindGlances 是 Glances 来源：Glances 4.x 的 REST 接口（http(s)://主机:61208/api/4/<插件>）。Glances 是通用的系统监控，
// 群晖、飞牛、Unraid、OMV 等能跑 Docker 的 NAS 用官方镜像跑它就能接入。默认不开认证；开了时用 HTTP Basic，
// 用户名默认是 glances。
const KindGlances = "glances"

func init() {
	RegisterKind(Kind{
		Name:        KindGlances,
		Label:       "Glances",
		Username:    UsernameOptional,
		NeedsSecret: func(username string) bool { return username != "" },
		Plain:       PlainWithoutCredentials,
		ParseURL:    glancesParseURL,
		New: func(cfg SourceConfig) (Source, error) {
			g, err := newGlances(cfg, nil)
			if err != nil {
				return nil, err
			}
			return g, nil
		},
		Test: func(ctx context.Context, in TestInput) (*TestResult, error) { return testGlances(ctx, in, nil) },
	})
}

const (
	// glancesPort 是 Glances 网页服务的默认端口：http:// 地址没写端口时补上
	glancesPort = "61208"
	// glancesUser 是 Glances 开了认证时的默认用户名（只填了密码时用它）
	glancesUser = "glances"
	// glancesTimeout 是一次请求最多等多久
	glancesTimeout = 15 * time.Second
	// glancesMaxBody 是一个回应最多读多少字节（容器多的时候 containers 最大）
	glancesMaxBody = 4 << 20
	// glancesCPURetry 是 Glances 刚启动、第一次读 CPU 全是 0 时，等多久再读一次（同本机来源的两拍之差）
	glancesCPURetry = time.Second
)

var (
	errGlancesNoAPI   = errors.New("这个地址没有 Glances 4 的接口（/api/4 回 404）：地址或端口对吗？Glances 默认端口是 61208")
	errGlancesV5      = errors.New("这是 Glances 5，接口是 /api/5；pt-tools 现在接的是 Glances 4，请用 4.x 版本号的镜像（例如 nicolargo/glances:4.5.8-full）")
	errGlancesNotJSON = errors.New("回的不是 Glances 的数据（地址是不是指到了别的网页？）")
)

// glancesParseURL：http:// 地址没写端口时补上 Glances 的默认端口 61208（https:// 多半是反向代理，按 443）。
func glancesParseURL(u *url.URL) error {
	if u.Scheme == "http" && u.Port() == "" {
		u.Host = net.JoinHostPort(u.Hostname(), glancesPort)
	}
	return nil
}

// glances 是一个 Glances 来源：每次采集按插件发几个 GET，不保持登录状态。
type glances struct {
	base   *url.URL
	user   string
	pass   string
	client *http.Client
	// wait 是第一次读 CPU 全是 0 时等着再读的那一下（测试里换掉）
	wait func(ctx context.Context, d time.Duration) error

	mu sync.Mutex
	// hostname 是上一次完整采集读到的主机名：轻量采集不读主机信息，认容器网络时用它（mu 守着）
	hostname string
}

// glancesAuth 是这次要发的账号：只填了密码时用户名按 Glances 的默认 glances；都没填时不发。
func glancesAuth(username, secret string) (user, pass string) {
	if secret == "" {
		return username, ""
	}
	if username == "" {
		return glancesUser, secret
	}
	return username, secret
}

func newGlancesClient(u *url.URL, pin string, roots *x509.CertPool) *http.Client {
	return &http.Client{
		Timeout:   glancesTimeout,
		Transport: Transport(u, pin, roots),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func newGlances(cfg SourceConfig, roots *x509.CertPool) (*glances, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("地址不对：%w", err)
	}
	user, pass := glancesAuth(cfg.Username, cfg.Secret)
	if u.Scheme != "https" && (cfg.Username != "" || pass != "") && !cfg.InsecureAck {
		return nil, ErrPlainNeedsAck
	}
	return &glances{base: u, user: user, pass: pass, client: newGlancesClient(u, cfg.PinSHA256, roots), wait: sleepCtx}, nil
}

// testGlances 是「测试连接」：先不带账号读 /api/4/status（认出是不是 Glances 4，证书不认时也不会发出密码），
// 再带账号读主机信息。
func testGlances(ctx context.Context, in TestInput, roots *x509.CertPool) (*TestResult, error) {
	if in.URL == nil {
		return nil, errors.New("要填地址")
	}
	user, pass := glancesAuth(in.Username, in.Secret)
	g := &glances{base: in.URL, user: user, pass: pass, client: newGlancesClient(in.URL, in.PinSHA256, roots), wait: sleepCtx}
	defer g.client.CloseIdleConnections()
	if err := g.checkAPI(ctx); err != nil {
		return nil, err
	}
	sys, err := g.system(ctx)
	if err != nil {
		return nil, err
	}
	return &TestResult{System: sys}, nil
}

// Close 断开空闲的连接。
func (g *glances) Close() error {
	g.client.CloseIdleConnections()
	return nil
}

// glancesHTTPError 是一个插件回的非 200：Off 为真是 Glances 关着这个插件（回 400「Unknown plugin」）。
type glancesHTTPError struct {
	plugin string
	status int
	detail string
	off    bool
}

func (e *glancesHTTPError) Error() string {
	if e.detail == "" {
		return fmt.Sprintf("Glances 读 %s 回了 HTTP %d", e.plugin, e.status)
	}
	return fmt.Sprintf("Glances 读 %s 回了 HTTP %d：%s", e.plugin, e.status, e.detail)
}

func asGlancesHTTPError(err error) (*glancesHTTPError, bool) {
	var he *glancesHTTPError
	ok := errors.As(err, &he)
	return he, ok
}

// glancesFatal 是这一次采集要整个放弃的错误：认证失败、证书、连不上、没有接口；其余（某个插件关着或出错）只记在那一类上。
func glancesFatal(err error) bool {
	var he *glancesHTTPError
	return !errors.As(err, &he) && !errors.Is(err, errGlancesNotJSON)
}

// get 取 /api/4/<path> 并解码到 out。按状态码分：401 是认证失败；400 是这个插件关着（Glances 回「Unknown plugin」）；
// 404 时 detail 是「Not Found」说明没有这个接口（不是 Glances 4），否则是插件取数出错；3xx 不跟随（可能把密码带到别处）。
func (g *glances) get(ctx context.Context, path string, out any) error {
	resp, err := g.do(ctx, "/api/4/"+path, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, glancesMaxBody+1))
	if err != nil {
		return fmt.Errorf("读 Glances 的 %s 断了：%w", path, err)
	}
	if len(body) > glancesMaxBody {
		return &glancesHTTPError{plugin: path, status: resp.StatusCode, detail: "回应太大"}
	}
	switch {
	case resp.StatusCode == http.StatusOK:
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("%w：%s", errGlancesNotJSON, path)
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return g.authError()
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return glancesRedirect(resp)
	}
	detail := glancesDetail(body)
	if resp.StatusCode == http.StatusNotFound && detail == "Not Found" {
		return errGlancesNoAPI
	}
	return &glancesHTTPError{
		plugin: path, status: resp.StatusCode, detail: detail,
		off: resp.StatusCode == http.StatusBadRequest && strings.HasPrefix(detail, "Unknown plugin"),
	}
}

// do 发一个 GET；auth 为真时带上账号。连不上、证书不认时返回的错误去掉了 Go 加的「Get "地址":」前缀。
func (g *glances) do(ctx context.Context, path string, auth bool) (*http.Response, error) {
	u := *g.base
	u.Path = path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if auth && g.pass != "" {
		req.SetBasicAuth(g.user, g.pass)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("连不上 Glances：%w", err)
	}
	return resp, nil
}

// glancesRedirect 是回了重定向（不跟着走：可能把密码带到别处）。转到 https:// 多半是反向代理把 http 转成了 https；
// 转到别处（例如登录页）说明这个地址不是 Glances 的接口。
func glancesRedirect(resp *http.Response) error {
	loc := resp.Header.Get("Location")
	if strings.HasPrefix(strings.ToLower(loc), "https://") {
		return fmt.Errorf("Glances 回了重定向（HTTP %d，到 %s）：地址是不是要改成 https://", resp.StatusCode, loc)
	}
	return fmt.Errorf("这个地址回了重定向（HTTP %d，到 %s），不像 Glances 4 的接口：地址或端口对吗？Glances 默认端口是 61208", resp.StatusCode, loc)
}

func (g *glances) authError() error {
	if g.pass == "" {
		return &AuthError{Reason: "这台 Glances 开了认证，要填用户名（默认 glances）和密码"}
	}
	return &AuthError{Reason: "Glances 说用户名或密码不对"}
}

// glancesDetail 是 Glances（FastAPI）错误回应里的 detail；不是这个格式时截一段原文。
func glancesDetail(body []byte) string {
	var v struct {
		Detail string `json:"detail"`
	}
	if json.Unmarshal(body, &v) == nil && v.Detail != "" {
		return truncRunes(v.Detail, 120)
	}
	return truncRunes(strings.TrimSpace(string(body)), 120)
}

// checkAPI 不带账号读 /api/4/status：认出 Glances 4；404 时再看是不是 Glances 5。老版本的 status 也要认证，回 401 时算过。
func (g *glances) checkAPI(ctx context.Context) error {
	resp, err := g.do(ctx, "/api/4/status", false)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, glancesMaxBody))
	resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusUnauthorized:
		return nil
	case http.StatusNotFound:
		if g.isV5(ctx) {
			return errGlancesV5
		}
		return errGlancesNoAPI
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return glancesRedirect(resp)
	}
	return fmt.Errorf("Glances 的 /api/4/status 回了 HTTP %d", resp.StatusCode)
}

func (g *glances) isV5(ctx context.Context) bool {
	resp, err := g.do(ctx, "/api/5/status", false)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, glancesMaxBody))
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized
}

// Glances 各插件回应里用到的字段。数都按 float64 解（Glances 有时给整数、有时给小数），null 留空。
type (
	glSystem struct {
		Hostname    string `json:"hostname"`
		OSName      string `json:"os_name"`
		OSVersion   string `json:"os_version"`
		LinuxDistro string `json:"linux_distro"`
	}
	glQuicklook struct {
		CPUName    string  `json:"cpu_name"`
		CPULogCore float64 `json:"cpu_log_core"`
	}
	glCPU struct {
		Total   *float64 `json:"total"`
		Idle    *float64 `json:"idle"`
		IOWait  *float64 `json:"iowait"`
		CPUCore float64  `json:"cpucore"`
	}
	glMem struct {
		Total     float64 `json:"total"`
		Available float64 `json:"available"`
	}
	glLoad struct {
		Min1  float64 `json:"min1"`
		Min5  float64 `json:"min5"`
		Min15 float64 `json:"min15"`
	}
	glNet struct {
		Name     string   `json:"interface_name"`
		IsUp     *bool    `json:"is_up"`
		RecvRate *float64 `json:"bytes_recv_rate_per_sec"`
		SentRate *float64 `json:"bytes_sent_rate_per_sec"`
		Recv     *float64 `json:"bytes_recv"`
		Sent     *float64 `json:"bytes_sent"`
		Since    *float64 `json:"time_since_update"`
	}
	glFS struct {
		Device string  `json:"device_name"`
		Type   string  `json:"fs_type"`
		Mount  string  `json:"mnt_point"`
		Size   float64 `json:"size"`
		Used   float64 `json:"used"`
		Free   float64 `json:"free"`
	}
	glSensor struct {
		Label string   `json:"label"`
		Unit  string   `json:"unit"`
		Value *float64 `json:"value"`
		Type  string   `json:"type"`
	}
	glContainer struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	glRaid struct {
		Status    string  `json:"status"`
		Type      string  `json:"type"`
		Available flexInt `json:"available"`
		Used      flexInt `json:"used"`
		Config    string  `json:"config"`
	}
)

// flexInt 是可能写成数也可能写成字符串的整数（raid 插件的 available、used）；没有或解不出时 ok 为假。
type flexInt struct {
	n  int
	ok bool
}

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		return nil
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil && v >= 0 && v < math.MaxInt32 {
		f.n, f.ok = int(v), true
	}
	return nil
}

// Collect 采集一次。轻量采集读 CPU、内存、负载、网络、温度与运行时长；完整采集再读主机、CPU 型号、文件系统、容器与 RAID。
// 认证失败、证书、连不上时整次失败；某个插件关着或出错时只记在那一类上。
func (g *glances) Collect(ctx context.Context, full bool) (*Snapshot, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	snap := &Snapshot{
		At:     time.Now(),
		Pools:  []Pool{},
		Disks:  []Disk{},
		Alerts: []Alert{},
		Facets: map[string]Facet{},
	}
	snap.Network.Interfaces = []string{}

	if err := g.fillCPU(ctx, snap); err != nil {
		return nil, err
	}
	// 完整采集先读主机信息：认 Glances 的容器有没有用 host 网络要看主机名
	var steps []func(context.Context, *Snapshot, bool) error
	if full {
		steps = append(steps, g.fillSystem)
	}
	steps = append(steps, g.fillMemory, g.fillLoad, g.fillNetwork, g.fillSensors, g.fillUptime)
	if full {
		steps = append(steps, g.fillPools, g.fillApps, g.fillRaid)
	}
	for _, step := range steps {
		if err := step(ctx, snap, full); err != nil {
			return nil, err
		}
	}
	return snap, nil
}

// pluginFacet 是一个插件没读成时那一类的情况；返回的错误不为空时整次采集放弃。
func pluginFacet(err error, what string) (Facet, error) {
	if glancesFatal(err) {
		return Facet{}, err
	}
	var he *glancesHTTPError
	if errors.As(err, &he) && he.off {
		return Facet{Status: FacetUnsupported, Reason: fmt.Sprintf("Glances 关着 %s 插件，读不到%s", he.plugin, what)}, nil
	}
	return Facet{Status: FacetError, Reason: err.Error()}, nil
}

// fillCPU 读 CPU 占用与 IO 等待。Glances 刚启动时第一次读 CPU 全是 0（要两拍之差才算得出），这时等一下再读一次。
// 这是每次采集的第一个请求：认证失败、连不上、不是 Glances 4 都在这里出来。
func (g *glances) fillCPU(ctx context.Context, snap *Snapshot) error {
	var c glCPU
	err := g.get(ctx, "cpu", &c)
	if err == nil && glancesCPUCold(c) {
		if err = g.wait(ctx, glancesCPURetry); err != nil {
			return err
		}
		c = glCPU{}
		err = g.get(ctx, "cpu", &c)
	}
	if err != nil {
		f, fatal := pluginFacet(err, "CPU 占用")
		if fatal != nil {
			return fatal
		}
		snap.Facets[FacetCPU] = f
		return nil
	}
	if c.Total == nil || glancesCPUCold(c) {
		snap.Facets[FacetCPU] = Facet{Status: FacetError, Reason: "Glances 还没算出 CPU 占用"}
		return nil
	}
	snap.CPU.Percent = clampPercent(*c.Total)
	if c.IOWait != nil {
		iow := clampPercent(*c.IOWait)
		snap.CPU.IOWait = &iow
	}
	if c.CPUCore > 0 {
		snap.System.Cores = int(c.CPUCore)
	}
	snap.Facets[FacetCPU] = Facet{Status: FacetOK}
	return nil
}

// glancesCPUCold 是还没算出来的 CPU：占用与空闲都是 0（真读数里两者之和接近 100）。
func glancesCPUCold(c glCPU) bool {
	return (c.Total == nil || *c.Total == 0) && (c.Idle == nil || *c.Idle == 0)
}

func clampPercent(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return math.Min(100, math.Max(0, v))
}

// toBytes 把 Glances 给的字节数换成 uint64（负数、NaN 记 0）。
func toBytes(v float64) uint64 {
	if math.IsNaN(v) || v <= 0 {
		return 0
	}
	if v >= math.MaxUint64 {
		return math.MaxUint64
	}
	return uint64(v)
}

func (g *glances) fillMemory(ctx context.Context, snap *Snapshot, _ bool) error {
	var m glMem
	if err := g.get(ctx, "mem", &m); err != nil {
		f, fatal := pluginFacet(err, "内存")
		if fatal != nil {
			return fatal
		}
		snap.Facets[FacetMemory] = f
		return nil
	}
	total := toBytes(m.Total)
	if total == 0 {
		snap.Facets[FacetMemory] = Facet{Status: FacetError, Reason: "Glances 没给内存总量"}
		return nil
	}
	avail := min(toBytes(m.Available), total)
	snap.Memory = Memory{Total: total, Available: avail, Used: total - avail}
	snap.Facets[FacetMemory] = Facet{Status: FacetOK}
	return nil
}

// fillLoad 读负载；读不到时负载留 0（负载不单独成一类，算在 CPU 里，不改 CPU 那一类的情况）。
func (g *glances) fillLoad(ctx context.Context, snap *Snapshot, _ bool) error {
	var l glLoad
	if err := g.get(ctx, "load", &l); err != nil {
		if glancesFatal(err) {
			return err
		}
		return nil
	}
	snap.CPU.Load1, snap.CPU.Load5, snap.CPU.Load15 = l.Min1, l.Min5, l.Min15
	return nil
}

// glancesUptimeRe 是 Glances 的运行时长（Python timedelta 的写法）：「9 days, 11:22:59」「1 day, 0:01:02」「7:49:45」。
var glancesUptimeRe = regexp.MustCompile(`^(?:(\d+) days?, )?(\d+):(\d{2}):(\d{2})$`)

// parseGlancesUptime 是运行时长的秒数；看不懂时为 0（合并时沿用上一次的）。
func parseGlancesUptime(s string) int64 {
	m := glancesUptimeRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	var days int64
	if m[1] != "" {
		days, _ = strconv.ParseInt(m[1], 10, 64)
	}
	h, _ := strconv.ParseInt(m[2], 10, 64)
	mi, _ := strconv.ParseInt(m[3], 10, 64)
	sec, _ := strconv.ParseInt(m[4], 10, 64)
	return days*86400 + h*3600 + mi*60 + sec
}

func (g *glances) fillUptime(ctx context.Context, snap *Snapshot, _ bool) error {
	var s string
	if err := g.get(ctx, "uptime", &s); err != nil {
		if glancesFatal(err) {
			return err
		}
		return nil
	}
	snap.System.UptimeSeconds = parseGlancesUptime(s)
	return nil
}

// fillSystem 读主机名、系统与版本（system）与 CPU 型号、逻辑核数（quicklook）。Glances 在 Docker 里时，系统名要把宿主机的
// /etc/os-release 挂进容器才是 NAS 自己的，主机名要用 host 网络才是 NAS 的。
func (g *glances) fillSystem(ctx context.Context, snap *Snapshot, _ bool) error {
	sys, err := g.system(ctx)
	if err != nil {
		f, fatal := pluginFacet(err, "主机信息")
		if fatal != nil {
			return fatal
		}
		snap.Facets[FacetSystem] = f
		return nil
	}
	g.hostname = sys.Hostname
	cores, uptime := snap.System.Cores, snap.System.UptimeSeconds
	snap.System = sys
	if snap.System.Cores == 0 {
		snap.System.Cores = cores
	}
	snap.System.UptimeSeconds = uptime
	snap.Facets[FacetSystem] = Facet{Status: FacetOK}
	return nil
}

// system 读 system 与 quicklook（测试连接与完整采集共用）。quicklook 读不成时只缺 CPU 型号。
func (g *glances) system(ctx context.Context) (System, error) {
	var s glSystem
	if err := g.get(ctx, "system", &s); err != nil {
		return System{}, err
	}
	out := System{Hostname: s.Hostname, Platform: s.LinuxDistro, Version: s.OSVersion}
	if out.Platform == "" {
		out.Platform = s.OSName
	}
	var q glQuicklook
	if err := g.get(ctx, "quicklook", &q); err != nil {
		if glancesFatal(err) {
			return System{}, err
		}
	} else {
		out.Model = q.CPUName
		out.Cores = int(q.CPULogCore)
	}
	return out, nil
}

// physIfaceRe 是认作物理网卡的名字：eth0、enp3s0、eno1、ens160、enx…、wlan0、wlp2s0、em1、p1p1。网桥、聚合口、VLAN
// （eth0.100）、虚拟网卡都不算，免得同一份流量算两遍。
var physIfaceRe = regexp.MustCompile(`^(eth\d+|en[a-z0-9]+|wl[a-z0-9]+|em\d+|p\d+p\d+)$`)

// containerIDRe 是 Docker、Podman 给容器的默认主机名：容器 ID 的前 12 位。
var containerIDRe = regexp.MustCompile(`^[0-9a-f]{12}$`)

// containerEthRe 是容器自己的网卡（eth0、eth1…）。
var containerEthRe = regexp.MustCompile(`^eth\d+$`)

// tunnelIfacePrefixes 是加载了隧道模块时内核在每个网络命名空间里都建的网卡（tunl0、sit0、ip6tnl0…）：
// 判断是不是容器自己的网络时不看。
var tunnelIfacePrefixes = []string{"tunl", "sit", "ip6tnl", "gre", "erspan", "ip_vti", "ip6_vti", "ip6gre"}

const glancesBridgedNet = "Glances 的容器没用 host 网络，读到的只是它自己的网卡（eth0）：容器要用 network_mode: host 才看得到 NAS 的网卡与流量"

func hasAnyPrefix(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// glancesBridged 是 Glances 的容器没用 host 网络：主机名是容器 ID，网卡除了 lo 与隧道只有 eth0、eth1…（容器自己的，
// 没有 Docker 在主机上建的 docker0、veth…）。这时读到的是 Glances 容器自己的流量，不是 NAS 的。
// 主机名是真名字时不算（直接装在 NAS 上的 Glances 只有一块 eth0 也是对的）。
func glancesBridged(hostname string, list []glNet) bool {
	if !containerIDRe.MatchString(hostname) {
		return false
	}
	seen := false
	for _, n := range list {
		if n.Name == "" || n.Name == "lo" || hasAnyPrefix(n.Name, tunnelIfacePrefixes) {
			continue
		}
		if !containerEthRe.MatchString(n.Name) {
			return false
		}
		seen = true
	}
	return seen
}

// virtIfacePrefixes 是虚拟网卡的前缀：一块物理名字的网卡也没有时，退回「除了这些都算」。
var virtIfacePrefixes = []string{
	"lo", "docker", "veth", "br", "virbr", "vnet", "vmnet", "cni", "flannel", "cali", "kube", "tun", "tap",
	"wg", "tailscale", "zt", "ifb", "dummy", "sit", "ip6tnl", "gre", "bond", "team", "ovs", "lxc", "lxd", "podman",
	"vlan", "macvlan", "ipvlan", "qvs", "nm-", "zth",
}

func virtualIface(name string) bool {
	return hasAnyPrefix(name, virtIfacePrefixes) || strings.Contains(name, ".")
}

// glancesRate 是一块网卡的收发速度（字节/秒）：用 *_rate_per_sec；没有时（老的 4.x）用这段时间的字节数除以间隔。
func glancesRate(rate, delta, since *float64) uint64 {
	if rate != nil {
		return toBytes(*rate)
	}
	if delta != nil && since != nil && *since > 0 {
		return toBytes(*delta / *since)
	}
	return 0
}

func (g *glances) fillNetwork(ctx context.Context, snap *Snapshot, _ bool) error {
	var list []glNet
	if err := g.get(ctx, "network", &list); err != nil {
		f, fatal := pluginFacet(err, "网卡")
		if fatal != nil {
			return fatal
		}
		snap.Facets[FacetNetwork] = f
		return nil
	}
	if glancesBridged(g.hostname, list) {
		snap.Facets[FacetNetwork] = Facet{Status: FacetUnsupported, Reason: glancesBridgedNet}
		return nil
	}
	pick := func(ok func(string) bool) []glNet {
		var out []glNet
		for _, n := range list {
			if n.Name != "" && (n.IsUp == nil || *n.IsUp) && ok(n.Name) {
				out = append(out, n)
			}
		}
		return out
	}
	chosen := pick(physIfaceRe.MatchString)
	if len(chosen) == 0 {
		chosen = pick(func(name string) bool { return !virtualIface(name) })
	}
	if len(chosen) == 0 {
		snap.Facets[FacetNetwork] = Facet{Status: FacetUnsupported, Reason: "Glances 只给了虚拟网卡：Glances 的容器要用 host 网络（network_mode: host）才看得到 NAS 的网卡"}
		return nil
	}
	for _, n := range chosen {
		snap.Network.RxBps += glancesRate(n.RecvRate, n.Recv, n.Since)
		snap.Network.TxBps += glancesRate(n.SentRate, n.Sent, n.Since)
		snap.Network.Interfaces = append(snap.Network.Interfaces, n.Name)
	}
	snap.Facets[FacetNetwork] = Facet{Status: FacetOK}
	return nil
}
