package nas

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 假飞牛：握手、加密登录、两步验证、签名校验与 token 重连照 pyfnos（Timandes/pyfnos）的 client.py、specs/twofa-login 与
// 测试记录的格式写；回应的字段名照 pyfnos 测试里核对的那些，数值是编的。序列号是假的，测试核对它不进快照。
// 错误码 4352（不是管理员）与 135168（token 过期）来自 pyfnos，52429312（不是管理员读 Docker 的容器）是在 fnOS 1.2.1000
// 真机上看到的，别的是编的。下面几处照 fnOS 1.2.1000 真机的回应写：内存的 total − available 等于 used + reserved
// （飞牛网页显示的已用是 used），没有容器时容器列表回 succ 但不带 rsp。
const (
	fnosCPUBody = `{"data":{"cpu":{"name":"Intel(R) N100","num":1,"core":4,"thread":4,"maxFreq":3400,
		"temp":[45,47,0],"busy":{"all":23,"user":15,"system":6,"iowait":2,"other":0},
		"loadavg":{"avg1min":1.25,"avg5min":0.9,"avg15min":0.7}}}}`
	fnosMemBody = `{"data":{"mem":{"reserved":104857600,"total":16483282944,"free":2147483648,"used":5368709120,
		"available":11009716224,"cached":8589934592,"buffers":268435456},"swap":{"total":0,"free":0,"used":0}}}`
	fnosGenBody = `{"data":{"item":{"netSpeed":{"transmit":131072,"receive":2621440}}}}`
	fnosNetBody = `{"data":{"ifs":[{"name":"lo","index":1,"ifType":772,"receive":0,"transmit":0},
		{"name":"enp1s0","index":2,"ifType":1,"receive":2621440,"transmit":131072},
		{"name":"enp2s0","index":3,"ifType":1,"receive":0,"transmit":0}]}}`
	fnosHostBody    = `{"data":{"hostName":"fnos-home","trimVersion":"0.9.27"}}`
	fnosUptimeBody  = `{"data":{"uptime":1268777}}`
	fnosGeneralBody = `{"array":[
		{"name":"dm-0","uuid":"trim_1a2b-0","mountpoint":"/vol1","frsize":2299364490240,"fssize":3829364490240,
		 "level":"raid1","storId":1,"md":[{"name":"md0"}]},
		{"name":"dm-1","uuid":"trim_3c4d-0","mountpoint":"/vol2","frsize":380107374182,"fssize":480107374182,
		 "level":"basic","storId":2,"md":[{"name":"md1"}]}],"block":[{"name":"sdc"}]}`
	fnosStateBody = `{"state":[
		{"name":"dm-0","uuid":"trim_1a2b-0","frsize":2299364490240,"fssize":3829364490240,"level":"raid1","storId":1,
		 "md":[{"name":"md0","uuid":"md-uuid-0","raidDisks":2,"level":"raid1","arrayState":"clean","syncAction":"idle",
		 "disk":[{"name":"sda1","arrState":"in_sync"},{"name":"sdb1","arrState":"in_sync"}]}]},
		{"name":"dm-1","uuid":"trim_3c4d-0","frsize":380107374182,"fssize":480107374182,"level":"basic","storId":2,
		 "md":[{"name":"md1","uuid":"md-uuid-1","raidDisks":1,"level":"raid1","arrayState":"clean","syncAction":"idle",
		 "disk":[{"name":"nvme0n1p1","arrState":"in_sync"}]}]}]}`
	fnosDisksBody = `{"disk":[
		{"name":"sda","size":4000787030016,"modelName":"WDC WD40EFRX-68N32N0","serialNumber":"FAKESERIAL-A","type":"HDD",
		 "protocol":"SATA","firmwareVersion":"82.00A82","logicalBlockSize":512,"diskGroup":"data","diskGroupEx":"data"},
		{"name":"sdb","size":"4000787030016","modelName":"WDC WD40EFRX-68N32N0","serialNumber":"FAKESERIAL-B","type":"HDD",
		 "protocol":"SATA","firmwareVersion":"82.00A82","logicalBlockSize":512,"diskGroup":"data","diskGroupEx":"data"},
		{"name":"nvme0n1","size":500107862016,"modelName":"Samsung SSD 980 500GB","serialNumber":"FAKESERIAL-C","type":"SSD",
		 "protocol":"NVMe","firmwareVersion":"3B4QFXO7","logicalBlockSize":512,"diskGroup":"data","diskGroupEx":"data"},
		{"name":"sdc","size":1000204886016,"modelName":"Samsung SSD 870 EVO 1TB","serialNumber":"FAKESERIAL-D","type":"SSD",
		 "protocol":"SATA","firmwareVersion":"SVT02B6Q","logicalBlockSize":512,"diskGroup":"","diskGroupEx":""}]}`
	fnosDiskMonBody = `{"data":{"num":4,"disk":[
		{"name":"sda","temp":36,"standby":false,"busy":3,"read":0,"write":4096},
		{"name":"sdb","temp":"38","standby":false,"busy":2,"read":0,"write":4096},
		{"name":"nvme0n1","temp":0,"standby":false,"busy":1,"read":8192,"write":0},
		{"name":"sdc","temp":31,"standby":true,"busy":0,"read":0,"write":0}]}}`
	fnosContainersBody = `{"rsp":[
		{"Id":"a1","Command":"/init","Created":1762400000,"HostConfig":{},"Image":"jellyfin/jellyfin","ImageID":"sha256:a",
		 "Names":["/jellyfin"],"State":"running","Status":"Up 2 days","Project":"media","Icon":""},
		{"Id":"b2","Command":"/init","Created":1762400000,"HostConfig":{},"Image":"linuxserver/qbittorrent","ImageID":"sha256:b",
		 "Names":["/qbittorrent"],"State":"exited","Status":"Exited (0)","Project":"media","Icon":""},
		{"Id":"c3","Command":"/bin/sh","Created":1762400000,"HostConfig":{},"Image":"nginx","ImageID":"sha256:c",
		 "Names":["/nginx"],"State":"running","Status":"Up 5 hours","Project":"","Icon":""}]}`
)

// fakeFnosKey 是假飞牛的 RSA 私钥（各个测试共用一把，免得每个测试都生成）。
var fakeFnosKey = sync.OnceValue(func() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
})

// fakeFnosPub 是假飞牛给的公钥（PEM）。
func fakeFnosPub() string {
	der, err := x509.MarshalPKIXPublicKey(&fakeFnosKey().PublicKey)
	if err != nil {
		panic(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// fnosFakeReply 是假飞牛对一个请求的回应：errno 不为 0 时回失败；drop 为真时不回；close 为真时断开连接。
type fnosFakeReply struct {
	body     string // JSON 对象：回应里除了 reqid 的字段
	errno    int
	msg      string
	noResult bool // 不带 result（pyfnos 说 stor.listDisk 有时这样）
	drop     bool
	close    bool
	// noise 为真时先发几条不相干的消息：飞牛推的通知、别的请求号的回应、不是 JSON 的文本
	noise bool
}

// fakeFnosToken 是假飞牛发出的一组凭据。
type fakeFnosToken struct {
	token   string
	long    string
	secret  []byte
	expired bool // user.authToken 回 135168
	revoked bool // user.authToken 与 user.tokenLogin 都不认
}

type fakeFnOS struct {
	t  *testing.T
	ts *httptest.Server

	mu       sync.Mutex
	user     string
	pass     string
	otp      string
	twofa    bool // 账号开了两步验证（绑定了验证器）
	enforced bool // 要求开两步验证、还没绑定
	admin    bool
	trusted  map[string]bool
	tokens   map[string]*fakeFnosToken
	longs    map[string]*fakeFnosToken
	issued   int
	replies  map[string][]fnosFakeReply
	params   map[string]map[string]any
	reqs     []string
	frames   []string
	conns    []*websocket.Conn
	upgrades int
	pings    int
	logins   int
	status   int    // 不为 0 时 /websocket 直接回这个状态码
	location string // 回重定向时的 Location
	hello    *fnosFakeReply
	login    *fnosFakeReply // 不为空时 user.login 回这个
	verify   *fnosFakeReply // 不为空时 user.2fa.loginVerify 回这个
	resume   *fnosFakeReply // 不为空时 user.authToken 回这个
	refresh  *fnosFakeReply // 不为空时 user.tokenLogin 回这个
	rotate   bool           // user.tokenLogin 同时换一个新的 longToken
	last     map[string]any // 最近一次登录的内层（核对设备号与设备名）
}

func newFakeFnOS(t *testing.T, tls bool) *fakeFnOS {
	t.Helper()
	f := &fakeFnOS{
		t: t, user: "ptmon", pass: "s3cret-pass", otp: "246810", admin: true,
		trusted: map[string]bool{}, tokens: map[string]*fakeFnosToken{}, longs: map[string]*fakeFnosToken{},
		replies: map[string][]fnosFakeReply{}, params: map[string]map[string]any{},
	}
	for req, body := range map[string]string{
		fnosReqCPU: fnosCPUBody, fnosReqMem: fnosMemBody, fnosReqGen: fnosGenBody, fnosReqNet: fnosNetBody,
		fnosReqHostName: fnosHostBody, fnosReqUptime: fnosUptimeBody, fnosReqStorage: fnosGeneralBody,
		fnosReqStorState: fnosStateBody, fnosReqDisks: fnosDisksBody, fnosReqDiskMon: fnosDiskMonBody,
		fnosReqContainers: fnosContainersBody,
	} {
		f.replies[req] = []fnosFakeReply{{body: body}}
	}
	f.ts = httptest.NewUnstartedServer(http.HandlerFunc(f.serve))
	f.ts.Config.ErrorLog = log.New(io.Discard, "", 0) // 握手被拒是一些测试要的
	if tls {
		f.ts.StartTLS()
	} else {
		f.ts.Start()
	}
	t.Cleanup(f.ts.Close)
	// 先断开服务端的 WebSocket（被接管的连接 httptest 不管），再关服务
	t.Cleanup(func() {
		f.mu.Lock()
		conns := append([]*websocket.Conn(nil), f.conns...)
		f.mu.Unlock()
		for _, c := range conns {
			_ = c.CloseNow()
		}
	})
	return f
}

// with 在锁里改假飞牛的设置：测试改设置时，服务端的连接可能还在读它们。
func (f *fakeFnOS) with(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

// set 把一个请求的回应换成这几份（用完以后一直回最后一份）。
func (f *fakeFnOS) set(req string, r ...fnosFakeReply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies[req] = r
}

func (f *fakeFnOS) url() *url.URL {
	u, err := url.Parse(f.ts.URL)
	require.NoError(f.t, err)
	return u
}

// roots 是认这台 TLS 假飞牛证书的根证书池（没开 TLS 时为空）。
func (f *fakeFnOS) roots() *x509.CertPool {
	if f.ts.TLS == nil {
		return nil
	}
	pool := x509.NewCertPool()
	pool.AddCert(f.ts.Certificate())
	return pool
}

func (f *fakeFnOS) testInput() TestInput {
	return TestInput{URL: f.url(), Username: f.user, Secret: f.pass}
}

// config 是连这台假飞牛的设置（http 时确认过明文风险），extra 是库里存的凭据。
func (f *fakeFnOS) config(extra map[string]string) SourceConfig {
	return SourceConfig{URL: f.ts.URL, Username: f.user, Secret: f.pass, InsecureAck: f.ts.TLS == nil, Extra: extra}
}

func (f *fakeFnOS) source(extra map[string]string) *fnos {
	f.t.Helper()
	s, err := newFnOS(f.config(extra), f.roots())
	require.NoError(f.t, err)
	f.t.Cleanup(func() { _ = s.Close() })
	return s
}

func (f *fakeFnOS) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.reqs...)
}

func (f *fakeFnOS) resetRequests() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = nil
}

func (f *fakeFnOS) count(req string) int {
	n := 0
	for _, r := range f.requests() {
		if r == req {
			n++
		}
	}
	return n
}

func (f *fakeFnOS) allFrames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.frames...)
}

func (f *fakeFnOS) counts() (upgrades, logins, pings int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.upgrades, f.logins, f.pings
}

// token 是假飞牛发出的那组凭据的一份拷贝（找不到时为空）。
func (f *fakeFnOS) token(tok string) *fakeFnosToken {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tokens[tok]
	if !ok {
		return nil
	}
	c := *t
	return &c
}

// expire 让 token 过期（user.authToken 回 135168，longToken 还能换新的）。
func (f *fakeFnOS) expire(tok string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens[tok].expired = true
}

// revoke 让 token 与它的 longToken 都不能再用（比如在飞牛上删了这台设备）。
func (f *fakeFnOS) revoke(tok string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens[tok].revoked = true
}

// lastLogin 是最近一次加密请求的内层。
func (f *fakeFnOS) lastLogin() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

// dropConns 从服务端断开全部连接。
func (f *fakeFnOS) dropConns() {
	f.mu.Lock()
	conns := append([]*websocket.Conn(nil), f.conns...)
	f.conns = nil
	f.mu.Unlock()
	for _, c := range conns {
		_ = c.CloseNow()
	}
}

func (f *fakeFnOS) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	status, loc := f.status, f.location
	f.mu.Unlock()
	if r.URL.Path != fnosPath || r.URL.RawQuery != fnosQuery {
		http.NotFound(w, r)
		return
	}
	if status != 0 {
		if loc != "" {
			w.Header().Set("Location", loc)
		}
		w.WriteHeader(status)
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(16 << 20)
	f.mu.Lock()
	f.upgrades++
	f.conns = append(f.conns, ws)
	si := fmt.Sprintf("si-%d", f.upgrades)
	f.mu.Unlock()
	(&fakeFnosConn{f: f, ws: ws, si: si}).loop()
}

// fakeFnosConn 是假飞牛上的一条连接：会话号、登录以后的签名密钥。
type fakeFnosConn struct {
	f      *fakeFnOS
	ws     *websocket.Conn
	si     string
	key    []byte
	access string // 两步验证的 accessToken
	queue  [][]byte
}

func (c *fakeFnosConn) loop() {
	ctx := context.Background()
	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		c.f.mu.Lock()
		c.f.frames = append(c.f.frames, string(data))
		c.f.mu.Unlock()
		out, closeNow := c.handle(data)
		if closeNow {
			_ = c.ws.CloseNow()
			return
		}
		frames := c.queue
		c.queue = nil
		if out != nil {
			frames = append(frames, out)
		}
		for _, fr := range frames {
			if err := c.ws.Write(ctx, websocket.MessageText, fr); err != nil {
				return
			}
		}
	}
}

// reply 是一个回应：fields 加上 reqid（与 result，没给时是 succ）。
func fakeFnosReplyJSON(reqid string, fields map[string]any) []byte {
	if fields == nil {
		fields = map[string]any{}
	}
	if reqid != "" {
		fields["reqid"] = reqid
	}
	b, err := json.Marshal(fields)
	if err != nil {
		panic(err)
	}
	return b
}

func fakeFnosFail(reqid string, errno int, msg string) []byte {
	return fakeFnosReplyJSON(reqid, map[string]any{"result": "fail", "errno": errno, "msg": msg})
}

// render 是一份 fnosFakeReply 写成的回应。
func (c *fakeFnosConn) render(reqid string, r fnosFakeReply) ([]byte, bool) {
	if r.noise {
		c.queue = append(c.queue, []byte(`{"notify":"stor.changed"}`), []byte(`{"reqid":"0000000000000aaaaaaaaaaaa","result":"succ"}`),
			[]byte(`not json`))
	}
	switch {
	case r.close:
		return nil, true
	case r.drop:
		return nil, false
	case r.errno != 0:
		return fakeFnosFail(reqid, r.errno, r.msg), false
	}
	fields := map[string]any{}
	if r.body != "" && !assert.NoError(c.f.t, json.Unmarshal([]byte(r.body), &fields), r.body) {
		return nil, true
	}
	if _, ok := fields["result"]; !ok && !r.noResult {
		fields["result"] = "succ"
	}
	return fakeFnosReplyJSON(reqid, fields), false
}

func (c *fakeFnosConn) handle(data []byte) ([]byte, bool) {
	t := c.f.t
	if bytes.HasPrefix(data, []byte("{")) {
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Errorf("假飞牛收到看不懂的请求: %v", err)
			return nil, true
		}
		reqid, _ := m["reqid"].(string)
		switch m["req"] {
		case "ping":
			c.f.mu.Lock()
			c.f.pings++
			c.f.mu.Unlock()
			return []byte(`{"res":"pong"}`), false
		case "util.crypto.getRSAPub":
			c.f.mu.Lock()
			c.f.reqs = append(c.f.reqs, "util.crypto.getRSAPub")
			hello := c.f.hello
			c.f.mu.Unlock()
			if hello != nil {
				return c.render(reqid, *hello)
			}
			return fakeFnosReplyJSON(reqid, map[string]any{"result": "succ", "pub": fakeFnosPub(), "si": c.si}), false
		case "encrypted":
			return c.encrypted(m)
		}
		t.Errorf("假飞牛收到没签名的请求: %v", m["req"])
		return fakeFnosFail(reqid, 4097, "没签名"), false
	}
	return c.signed(string(data))
}

// signed 校验签名（前 44 个字是 base64 的 HMAC-SHA256）后按请求回应。
func (c *fakeFnosConn) signed(text string) ([]byte, bool) {
	t := c.f.t
	if len(text) < 44 {
		t.Errorf("假飞牛收到太短的签名请求")
		return nil, true
	}
	sig, body := text[:44], text[44:]
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Errorf("签名后面不是 JSON: %v", err)
		return nil, true
	}
	req, _ := m["req"].(string)
	reqid, _ := m["reqid"].(string)
	c.f.mu.Lock()
	c.f.reqs = append(c.f.reqs, req)
	c.f.params[req] = m
	c.f.mu.Unlock()
	// user.authToken 按 token、user.tokenLogin 按 longToken 找到那组凭据，用它的签名密钥核对签名；别的请求用这条连接登录得到的
	key := c.key
	var cred *fakeFnosToken
	if req == "user.authToken" || req == "user.tokenLogin" {
		c.f.mu.Lock()
		override := c.f.resume
		creds, failNo, failMsg := c.f.tokens, 131074, "token 无效"
		if req == "user.tokenLogin" {
			override, creds, failNo, failMsg = c.f.refresh, c.f.longs, 131075, "longToken 无效"
		}
		if found := creds[fmt.Sprint(m["token"])]; found != nil && !found.revoked {
			cp := *found
			cred = &cp
		}
		c.f.mu.Unlock()
		if override != nil {
			return c.render(reqid, *override)
		}
		if cred == nil {
			return fakeFnosFail(reqid, failNo, failMsg), false
		}
		key = cred.secret
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(body))
	if len(key) == 0 || !hmac.Equal([]byte(base64.StdEncoding.EncodeToString(mac.Sum(nil))), []byte(sig)) {
		t.Errorf("假飞牛收到签名不对的请求 %s", req)
		return fakeFnosFail(reqid, 4098, "签名不对"), false
	}
	switch req {
	case "user.authToken":
		assert.Equal(t, c.si, m["si"])
		assert.Equal(t, true, m["main"])
		if cred.expired {
			return fakeFnosFail(reqid, fnosErrTokenExpiredFake, "token 过期了"), false
		}
		c.key = cred.secret
		return fakeFnosReplyJSON(reqid, map[string]any{"result": "succ"}), false
	case "user.tokenLogin":
		assert.Equal(t, c.si, m["si"])
		assert.Equal(t, fnosDeviceName, m["deviceName"])
		c.f.mu.Lock()
		long := cred.long
		if c.f.rotate {
			long = ""
		}
		c.f.mu.Unlock()
		tok := c.f.issue(cred.secret, long)
		c.key = tok.secret
		reply := map[string]any{"result": "succ", "token": tok.token}
		if long == "" {
			reply["longToken"] = tok.long
		}
		return fakeFnosReplyJSON(reqid, reply), false
	}
	if len(c.key) == 0 {
		return fakeFnosFail(reqid, 4099, "没有登录"), false
	}
	return c.data(req, reqid)
}

// fnosErrTokenExpiredFake 是 token 过期的错误码（pyfnos 据此改用 longToken 登录）。
const fnosErrTokenExpiredFake = 135168

// issue 发一组新凭据：secret 为空时生成新的签名密钥，long 为空时生成新的 longToken。
func (f *fakeFnOS) issue(secret []byte, long string) *fakeFnosToken {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issued++
	if secret == nil {
		secret = make([]byte, 16)
		_, _ = rand.Read(secret)
	}
	tok := &fakeFnosToken{token: fmt.Sprintf("tok-%d", f.issued), long: long, secret: secret}
	if tok.long == "" {
		tok.long = fmt.Sprintf("long-%d", f.issued)
	}
	f.tokens[tok.token] = tok
	f.longs[tok.long] = tok
	return tok
}

func (c *fakeFnosConn) data(req, reqid string) ([]byte, bool) {
	c.f.mu.Lock()
	admin := c.f.admin
	q := c.f.replies[req]
	var r fnosFakeReply
	switch {
	case len(q) == 0:
		r = fnosFakeReply{errno: 4100, msg: "没有这个接口 " + req}
	case len(q) == 1:
		r = q[0]
	default:
		r, c.f.replies[req] = q[0], q[1:]
	}
	c.f.mu.Unlock()
	if !admin && strings.HasPrefix(req, "stor.") {
		return fakeFnosFail(reqid, fnosErrNotAdmin, "没有权限"), false
	}
	if !admin && strings.HasPrefix(req, "appcgi.dockermgr.") {
		return fakeFnosFail(reqid, fnosErrDockerNotAdmin, ""), false
	}
	return c.render(reqid, r)
}

// encrypted 解开加密的登录请求：RSA（PKCS#1 v1.5）解开 AES 密钥，AES-CBC 解开请求。这里不用来源的 fnosDecrypt，
// 免得加解密写错了两边一起错。
func (c *fakeFnosConn) encrypted(m map[string]any) ([]byte, bool) {
	t := c.f.t
	b64 := func(k string) []byte {
		s, _ := m[k].(string)
		b, err := base64.StdEncoding.DecodeString(s)
		assert.NoError(t, err, k)
		return b
	}
	iv, wrapped, data := b64("iv"), b64("rsa"), b64("aes")
	//nolint:staticcheck // 假飞牛照飞牛的做法用 PKCS#1 v1.5
	aesKey, err := rsa.DecryptPKCS1v15(nil, fakeFnosKey(), wrapped)
	if !assert.NoError(t, err) || !assert.Len(t, aesKey, 32) || !assert.Len(t, iv, aes.BlockSize) ||
		!assert.True(t, len(data) > 0 && len(data)%aes.BlockSize == 0) {
		return nil, true
	}
	block, err := aes.NewCipher(aesKey)
	if !assert.NoError(t, err) {
		return nil, true
	}
	plain := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, data)
	pad := int(plain[len(plain)-1])
	if !assert.True(t, pad > 0 && pad <= aes.BlockSize, "填充不对") {
		return nil, true
	}
	var inner map[string]any
	if !assert.NoError(t, json.Unmarshal(plain[:len(plain)-pad], &inner)) {
		return nil, true
	}
	reqid, _ := inner["reqid"].(string)
	req, _ := inner["req"].(string)
	assert.Equal(t, c.si, inner["si"], "加密的请求要带这条连接的会话号")
	c.f.mu.Lock()
	c.f.reqs = append(c.f.reqs, req)
	c.f.last = inner
	c.f.mu.Unlock()
	switch req {
	case "user.login":
		return c.login(reqid, inner, aesKey, iv)
	case "user.2fa.loginVerify":
		return c.verify(reqid, inner, aesKey, iv)
	}
	t.Errorf("假飞牛收到不认得的加密请求 %s", req)
	return fakeFnosFail(reqid, 4101, "不认得"), false
}

func (c *fakeFnosConn) login(reqid string, in map[string]any, key, iv []byte) ([]byte, bool) {
	f := c.f
	f.mu.Lock()
	f.logins++
	user, pass, twofa, enforced, override := f.user, f.pass, f.twofa, f.enforced, f.login
	trusted := f.trusted[fmt.Sprint(in["did"])]
	f.mu.Unlock()
	if override != nil {
		return c.render(reqid, *override)
	}
	assert.Equal(f.t, fnosDeviceName, in["deviceName"])
	assert.Equal(f.t, fnosDeviceType, in["deviceType"])
	assert.Equal(f.t, true, in["stay"])
	if in["user"] != user || in["password"] != pass {
		// 飞牛的原因里带着试的密码：来源要把它去掉再写进日志与界面
		return fakeFnosFail(reqid, 131073, fmt.Sprintf("用户名或密码错误（%v / %v）", in["user"], in["password"])), false
	}
	switch {
	case enforced:
		c.access = "at-setup"
		return fakeFnosReplyJSON(reqid, map[string]any{
			"result": "succ", "accessToken": c.access, "isTwofaEnforced": true, "isBindTwofaSecret": false,
		}), false
	case twofa && !trusted:
		c.access = "at-" + c.si
		return fakeFnosReplyJSON(reqid, map[string]any{
			"result": "succ", "accessToken": c.access, "isBindTwofaSecret": true, "isTrustedDevice": false,
		}), false
	}
	return c.grant(reqid, key, iv), false
}

func (c *fakeFnosConn) verify(reqid string, in map[string]any, key, iv []byte) ([]byte, bool) {
	f := c.f
	f.mu.Lock()
	otp, override := f.otp, f.verify
	f.mu.Unlock()
	if override != nil {
		return c.render(reqid, *override)
	}
	assert.Equal(f.t, c.access, in["accessToken"])
	assert.EqualValues(f.t, 1, in["stay"])
	if in["code"] != otp {
		return fakeFnosFail(reqid, 131080, "验证码错误"), false
	}
	if in["isTrustedDevice"] == true {
		f.mu.Lock()
		f.trusted[fmt.Sprint(in["did"])] = true
		f.mu.Unlock()
	}
	return c.grant(reqid, key, iv), false
}

// grant 登录成功：发一组新凭据，签名密钥用这次请求的 AES 密钥与 IV 加密（PKCS#7 填充）。
func (c *fakeFnosConn) grant(reqid string, key, iv []byte) []byte {
	tok := c.f.issue(nil, "")
	c.key = tok.secret
	block, err := aes.NewCipher(key)
	if !assert.NoError(c.f.t, err) {
		return fakeFnosFail(reqid, 4102, "加密失败")
	}
	pad := aes.BlockSize - len(tok.secret)%aes.BlockSize
	buf := append(append([]byte(nil), tok.secret...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(buf, buf)
	c.f.mu.Lock()
	admin := c.f.admin
	c.f.mu.Unlock()
	return fakeFnosReplyJSON(reqid, map[string]any{
		"result": "succ", "token": tok.token, "longToken": tok.long, "uid": 1000, "admin": admin,
		"secret": base64.StdEncoding.EncodeToString(buf),
	})
}
