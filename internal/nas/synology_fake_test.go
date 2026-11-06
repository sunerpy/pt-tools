package nas

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// 假 DSM 的回应：字段名照 py-synologydsm-api（Home Assistant 的群晖集成用它）的 TypedDict 与 Homarr 群晖集成的测试数据，
// 数值是编的，不是真机抓的（用户没有群晖，H5 只用假服务）。序列号是假的，测试核对它不进快照。
const (
	dsmSystemInfo = `{"cpu_clock_speed":2000,"cpu_cores":"4","cpu_family":"Celeron","cpu_series":"J4125",
		"enabled_ntp":true,"firmware_ver":"DSM 7.2.2-72806 Update 3","model":"DS920+","ram_size":8192,
		"serial":"FAKESERIAL0000","sys_temp":46,"time":"2026-11-07 10:00:00","up_time":"352:06:17"}`
	dsmUtilization = `{"cpu":{"15min_load":152,"1min_load":231,"5min_load":187,"device":"System","other_load":1,
		"system_load":7,"user_load":12},
		"memory":{"avail_real":3203072,"avail_swap":4194300,"buffer":171012,"cached":2416896,"device":"Memory",
		"memory_size":8388608,"real_usage":21,"si_disk":0,"so_disk":0,"swap_usage":0,"total_real":7969616,"total_swap":4194300},
		"network":[{"device":"total","rx":524288,"tx":131072},{"device":"eth0","rx":524288,"tx":131072},
		{"device":"eth1","rx":0,"tx":0}],"time":1762480800}`
	dsmLoadInfo = `{"disks":[
		{"id":"sata1","name":"Drive 1","longName":"Drive 1","device":"/dev/sata1","diskType":"SATA","isSsd":false,
		 "model":"WD40EFRX-68N32N0        ","vendor":"WDC     ","size_total":"4000787030016","status":"normal",
		 "smart_status":"normal","temp":36,"used_by":"reuse_1"},
		{"id":"sata2","name":"Drive 2","longName":"Drive 2","device":"/dev/sata2","diskType":"SATA","isSsd":false,
		 "model":"WD40EFRX-68N32N0        ","vendor":"WDC     ","size_total":"4000787030016","status":"normal",
		 "smart_status":"normal","temp":"38","used_by":"reuse_1"},
		{"id":"nvme0n1","name":"M.2 Drive 1","longName":"M.2 Drive 1","device":"/dev/nvme0n1","diskType":"NVMe","isSsd":true,
		 "model":"SNV3410-400G","vendor":"Synology","size_total":"400088457216","status":"normal","temp":48,"used_by":"reuse_2"},
		{"id":"sata3","name":"Drive 3","longName":"Drive 3","device":"/dev/sata3","diskType":"SATA","isSsd":true,
		 "model":"Samsung SSD 870 EVO 1TB","vendor":"Samsung","size_total":"1000204886016","status":"not_initialized","temp":0}],
		"storagePools":[
		{"id":"reuse_1","disks":["sata1","sata2"],"device_type":"shr_with_1_disk_protect","pool_path":"reuse_1",
		 "status":"normal","deploy_path":"","pool_child":[{"id":"volume_1","size":{"total":"3829364490240","used":"3829364490240"}}],
		 "size":{"total":"3995917156352","used":"3829364490240"}},
		{"id":"reuse_2","disks":["nvme0n1"],"device_type":"basic","pool_path":"reuse_2","status":"normal",
		 "deploy_path":"volume_2","pool_child":[],"size":{"total":"398458871808","used":"398458871808"}}],
		"volumes":[
		{"id":"volume_1","vol_path":"/volume1","pool_path":"reuse_1","status":"normal","fs_type":"btrfs",
		 "device_type":"shr_with_1_disk_protect",
		 "size":{"free_inode":"0","total":"3829364490240","total_device":"3995917156352","total_inode":"0","used":"1530000000000"}},
		{"id":"volume_2","vol_path":"/volume2","status":"normal","fs_type":"btrfs",
		 "size":{"total":"380107374182","used":"95026843545"}}]}`
	dsmLegacyStorage = `{"hdd_info":[{"diskno":"Disk 1","model":"WD40EFRX","temp":36,"status":"normal"}],
		"vol_info":[{"name":"volume_1","vol_desc":"","used_size":"1530000000000","total_size":"3829364490240","status":"normal"},
		{"name":"volume_2","vol_desc":"","used_size":95026843545,"total_size":380107374182,"status":"degraded"}]}`
)

// dsmReply 是假 DSM 对一个接口的回应：code 不为 0 时回 DSM 的错误；status 不为 0 时是这个 HTTP 状态、body 原样回。
type dsmReply struct {
	code   int
	data   string
	status int
	body   string
	header map[string]string
}

// dsmReq 是假 DSM 收到的一个请求：query 是网址里的参数，form 是请求体里的。
type dsmReq struct {
	method string
	path   string
	query  url.Values
	form   url.Values
}

// api 是这个请求调的接口与方法（「SYNO.Core.System info」）。
func (r dsmReq) api() string {
	v := r.form
	if r.method == http.MethodGet {
		v = r.query
	}
	return v.Get("api") + " " + v.Get("method")
}

// fakeDSM 是测试用的群晖 DSM：SYNO.API.Info 按 apis 回，SYNO.API.Auth 按账号、验证码与设备令牌登录，
// 别的接口要带有效的会话，按 replies 回（同一个接口有几份时按顺序给，最后一份一直给）。记下收到的每个请求。
type fakeDSM struct {
	t  *testing.T
	ts *httptest.Server

	mu   sync.Mutex
	apis map[string]synoAPI
	// info 不为空时 query.cgi 原样回它（模拟不是 DSM 的地址等）
	info *dsmReply
	user string
	pass string
	// otp 不为空时账号开了两步验证：要这个验证码，或者 devices 里的一个设备令牌
	otp     string
	devices map[string]bool
	// issue 是登录成功时发的设备令牌（用了验证码时才发，issueAlways 为真时每次都发）；tokenField 是放它的字段（did 或 device_id）
	issue       string
	issueAlways bool
	tokenField  string
	// loginCodes 不为空时登录按顺序回这些错误码（0 是照常登录），最后一个一直回
	loginCodes []int
	// sessionCodes 不为空时带会话的调用先按顺序回这些错误码并让会话失效（106、107、119）
	sessionCodes []int
	sessions     map[string]bool
	nextSID      int
	replies      map[string][]dsmReply
	reqs         []dsmReq
}

func newFakeDSM(t *testing.T, tls bool) *fakeDSM {
	t.Helper()
	f := &fakeDSM{
		t: t, user: "ptmon", pass: "s3cret-pass", devices: map[string]bool{}, sessions: map[string]bool{},
		issue: "dev-token-1", tokenField: "device_id", replies: map[string][]dsmReply{},
		apis: map[string]synoAPI{
			synoAPIInfo:        {Path: "query.cgi", MinVersion: 1, MaxVersion: 1},
			synoAPIAuth:        {Path: "entry.cgi", MinVersion: 1, MaxVersion: 7},
			synoAPISystem:      {Path: "entry.cgi", MinVersion: 1, MaxVersion: 3},
			synoAPIUtilization: {Path: "entry.cgi", MinVersion: 1, MaxVersion: 1},
			synoAPIStorage:     {Path: "entry.cgi", MinVersion: 1, MaxVersion: 1},
		},
	}
	f.set(synoAPISystem+" info", dsmReply{data: dsmSystemInfo})
	f.set(synoAPISystem+" info storage", dsmReply{data: dsmLegacyStorage})
	f.set(synoAPIUtilization+" get", dsmReply{data: dsmUtilization})
	f.set(synoAPIStorage+" load_info", dsmReply{data: dsmLoadInfo})
	f.ts = httptest.NewUnstartedServer(http.HandlerFunc(f.serve))
	f.ts.Config.ErrorLog = log.New(io.Discard, "", 0) // 握手被拒是一些测试要的
	if tls {
		f.ts.StartTLS()
	} else {
		f.ts.Start()
	}
	t.Cleanup(f.ts.Close)
	return f
}

// set 把一个接口（「接口 方法」，type="storage" 时后面加「 storage」）的回应换成这几份。
func (f *fakeDSM) set(key string, r ...dsmReply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies[key] = r
}

func (f *fakeDSM) write(w http.ResponseWriter, r dsmReply) {
	for k, v := range r.header {
		w.Header().Set(k, v)
	}
	if r.status != 0 {
		w.WriteHeader(r.status)
		_, _ = io.WriteString(w, r.body)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.code != 0 {
		_, _ = fmt.Fprintf(w, `{"error":{"code":%d},"success":false}`, r.code)
		return
	}
	data := r.data
	if data == "" {
		data = "{}"
	}
	_, _ = fmt.Fprintf(w, `{"data":%s,"success":true}`, data)
}

func (f *fakeDSM) serve(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	form := url.Values{}
	if r.Method == http.MethodPost {
		form = r.PostForm
	}
	f.mu.Lock()
	f.reqs = append(f.reqs, dsmReq{method: r.Method, path: r.URL.Path, query: r.URL.Query(), form: form})
	f.mu.Unlock()
	if r.URL.Path == "/webapi/query.cgi" {
		f.serveInfo(w, r)
		return
	}
	if r.URL.Path != "/webapi/entry.cgi" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	api, method := form.Get("api"), form.Get("method")
	if api != r.URL.Query().Get("api") || method != r.URL.Query().Get("method") {
		f.write(w, dsmReply{code: 101})
		return
	}
	if api == synoAPIAuth {
		f.serveAuth(w, method, form)
		return
	}
	f.mu.Lock()
	sid := form.Get("_sid")
	if !f.sessions[sid] {
		f.mu.Unlock()
		f.write(w, dsmReply{code: synoErrInvalidToken})
		return
	}
	if len(f.sessionCodes) > 0 {
		code := f.sessionCodes[0]
		f.sessionCodes = f.sessionCodes[1:]
		delete(f.sessions, sid)
		f.mu.Unlock()
		f.write(w, dsmReply{code: code})
		return
	}
	key := api + " " + method
	if typ := strings.Trim(form.Get("type"), `"`); typ != "" {
		key += " " + typ
	}
	list := f.replies[key]
	var rep dsmReply
	switch {
	case len(list) == 0:
		rep = dsmReply{code: synoErrNoMethod}
	default:
		rep = list[0]
		if len(list) > 1 {
			f.replies[key] = list[1:]
		}
	}
	f.mu.Unlock()
	f.write(w, rep)
}

func (f *fakeDSM) serveInfo(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.info != nil {
		f.write(w, *f.info)
		return
	}
	q := r.URL.Query()
	if r.Method != http.MethodGet || q.Get("api") != synoAPIInfo || q.Get("method") != "query" {
		f.write(w, dsmReply{code: synoErrNoAPI})
		return
	}
	out := map[string]synoAPI{}
	for _, name := range strings.Split(q.Get("query"), ",") {
		if a, ok := f.apis[name]; ok {
			out[name] = a
		}
	}
	b, err := json.Marshal(out)
	require.NoError(f.t, err)
	f.write(w, dsmReply{data: string(b)})
}

// serveAuth 是登录与注销：用户名密码不对回 400；开了两步验证时没有有效的设备令牌、也没给验证码回 403，验证码不对回 404。
func (f *fakeDSM) serveAuth(w http.ResponseWriter, method string, form url.Values) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch method {
	case "logout":
		delete(f.sessions, form.Get("_sid"))
		f.write(w, dsmReply{})
		return
	case "login":
	default:
		f.write(w, dsmReply{code: synoErrNoMethod})
		return
	}
	if len(f.loginCodes) > 0 {
		code := f.loginCodes[0]
		if len(f.loginCodes) > 1 {
			f.loginCodes = f.loginCodes[1:]
		}
		if code != 0 {
			f.write(w, dsmReply{code: code})
			return
		}
	}
	if form.Get("account") != f.user || form.Get("passwd") != f.pass {
		f.write(w, dsmReply{code: 400})
		return
	}
	usedOTP := false
	if f.otp != "" && !f.devices[form.Get("device_id")] {
		switch otp := form.Get("otp_code"); {
		case otp == "":
			f.write(w, dsmReply{code: 403})
			return
		case otp != f.otp:
			f.write(w, dsmReply{code: 404})
			return
		}
		usedOTP = true
	}
	f.nextSID++
	sid := fmt.Sprintf("sid-%d-abcdef", f.nextSID)
	f.sessions[sid] = true
	data := map[string]string{"sid": sid, "synotoken": fmt.Sprintf("synotoken-%d", f.nextSID)}
	if (usedOTP || f.issueAlways) && f.issue != "" {
		data[f.tokenField] = f.issue
		f.devices[f.issue] = true
	}
	b, err := json.Marshal(data)
	require.NoError(f.t, err)
	f.write(w, dsmReply{data: string(b)})
}

func (f *fakeDSM) requests() []dsmReq {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]dsmReq(nil), f.reqs...)
}

// calls 是收到的请求调的接口（「SYNO.API.Auth login」…），按顺序。
func (f *fakeDSM) calls() []string {
	out := []string{}
	for _, r := range f.requests() {
		out = append(out, r.api())
	}
	return out
}

func (f *fakeDSM) count(api string) int {
	n := 0
	for _, c := range f.calls() {
		if c == api {
			n++
		}
	}
	return n
}

func (f *fakeDSM) resetRequests() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = nil
}

func (f *fakeDSM) liveSessions() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sessions)
}

func (f *fakeDSM) url() *url.URL {
	u, err := url.Parse(f.ts.URL)
	require.NoError(f.t, err)
	return u
}

// roots 是认这台 TLS 假 DSM 证书的根证书池（没开 TLS 时为空）。
func (f *fakeDSM) roots() *x509.CertPool {
	if f.ts.TLS == nil {
		return nil
	}
	pool := x509.NewCertPool()
	pool.AddCert(f.ts.Certificate())
	return pool
}

// config 是连这台假 DSM 的设置（http 时确认过明文风险）。
func (f *fakeDSM) config() SourceConfig {
	return SourceConfig{URL: f.ts.URL, Username: f.user, Secret: f.pass, InsecureAck: f.ts.TLS == nil}
}

func (f *fakeDSM) source(cfg SourceConfig) *synology {
	f.t.Helper()
	s, err := newSynology(cfg, f.roots())
	require.NoError(f.t, err)
	f.t.Cleanup(func() { _ = s.Close() })
	return s
}

func (f *fakeDSM) testInput() TestInput {
	return TestInput{URL: f.url(), Username: f.user, Secret: f.pass}
}
