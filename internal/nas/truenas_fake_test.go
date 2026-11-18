package nas

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// fakeTrueNASKey 是假 TrueNAS 认的 API Key。
const fakeTrueNASKey = "1-fakeTrueNASApiKeyForTestsOnly0123456789"

// fakeRPCError 是假 TrueNAS 回的错误（同真机的结构）。
type fakeRPCError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

var (
	errFakeDenied  = &fakeRPCError{Code: -32001, Message: "Method call error", Data: map[string]any{"error": 13, "errname": "EACCES", "reason": "[EACCES] Not authorized", "trace": map[string]any{"class": "CallError"}}}
	errFakeNoAuth  = &fakeRPCError{Code: -32001, Message: "Method call error", Data: map[string]any{"error": 207, "errname": "ENOTAUTHENTICATED", "reason": "[ENOTAUTHENTICATED] Not authenticated"}}
	errFakeMissing = &fakeRPCError{Code: -32601, Message: "Method does not exist"}
)

type fakeHandler func(params []json.RawMessage) (any, *fakeRPCError)

// fakeTrueNAS 是测试用的 TrueNAS：httptest 的 TLS 服务，/api/current 收 WebSocket，按方法名回 JSON-RPC 2.0。
// 默认的数据是一台有两个存储池、四块硬盘、两块连着的网卡的 TrueNAS；server 是它的时钟。
type fakeTrueNAS struct {
	t  *testing.T
	ts *httptest.Server

	mu       sync.Mutex
	handlers map[string]fakeHandler
	calls    []string
	windows  [][2]int64 // reporting.get_data 收到的 start、end
	logins   int
	upgrades int
	requests int
	conns    []*websocket.Conn
	status   int // 不为 0 时 /api/current 直接回这个状态码
	// dropAfter 不为 0 时，第一条连接回完这么多条回复以后断开
	dropAfter int
	replies   int

	server time.Time
	// series 给报表的每一格取值（nil 是空值）；默认按 graphValue
	series func(graph, ident, col string, t int64) *float64
	// noAggregations 为真时报表不带 aggregations（来源要自己按行算平均）
	noAggregations bool
}

func newFakeTrueNAS(t *testing.T) *fakeTrueNAS {
	t.Helper()
	f := &fakeTrueNAS{t: t, handlers: map[string]fakeHandler{}, server: time.Unix(1_800_000_000, 0)}
	f.series = f.graphValue
	f.defaults()
	f.ts = httptest.NewUnstartedServer(http.HandlerFunc(f.serve))
	f.ts.Config.ErrorLog = log.New(io.Discard, "", 0) // 握手被拒是一些测试要的
	f.ts.StartTLS()
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

func (f *fakeTrueNAS) url() *url.URL {
	u, err := url.Parse(f.ts.URL)
	require.NoError(f.t, err)
	return u
}

func (f *fakeTrueNAS) pin() string { return Fingerprint(f.ts.Certificate()) }

// source 是连这台假 TrueNAS 的来源（钉住它的证书）；本机时钟和 TrueNAS 一样。
func (f *fakeTrueNAS) source(key string) *trueNAS {
	f.t.Helper()
	src, err := newTrueNAS(SourceConfig{URL: f.ts.URL, Secret: key, PinSHA256: f.pin()}, nil)
	require.NoError(f.t, err)
	src.now = func() time.Time { return f.server }
	f.t.Cleanup(func() { _ = src.Close() })
	return src
}

func (f *fakeTrueNAS) handle(method string, h fakeHandler) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[method] = h
}

// fail 让 method 回 err。
func (f *fakeTrueNAS) fail(method string, err *fakeRPCError) {
	f.handle(method, func([]json.RawMessage) (any, *fakeRPCError) { return nil, err })
}

func (f *fakeTrueNAS) callList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeTrueNAS) resetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls, f.windows = nil, nil
}

func (f *fakeTrueNAS) count(method string) int {
	n := 0
	for _, c := range f.callList() {
		if c == method {
			n++
		}
	}
	return n
}

func (f *fakeTrueNAS) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	status := f.status
	f.mu.Unlock()
	if r.URL.Path != trueNASPath {
		http.NotFound(w, r)
		return
	}
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	f.mu.Lock()
	f.upgrades++
	first := len(f.conns) == 0
	f.conns = append(f.conns, ws)
	f.mu.Unlock()
	ctx := context.Background()
	authed := false
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			return
		}
		var req struct {
			ID     int64             `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if jerr := json.Unmarshal(data, &req); jerr != nil {
			f.t.Errorf("假 TrueNAS 收到看不懂的请求: %v", jerr)
			return
		}
		f.mu.Lock()
		f.calls = append(f.calls, req.Method)
		h := f.handlers[req.Method]
		f.mu.Unlock()
		var result any
		var rerr *fakeRPCError
		switch {
		case req.Method == "auth.login_with_api_key":
			var key string
			if len(req.Params) == 1 {
				_ = json.Unmarshal(req.Params[0], &key)
			}
			authed = key == fakeTrueNASKey
			f.mu.Lock()
			f.logins++
			f.mu.Unlock()
			result = authed
		case !authed:
			rerr = errFakeNoAuth
		case h == nil:
			rerr = errFakeMissing
		default:
			result, rerr = h(req.Params)
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if rerr != nil {
			resp["error"] = rerr
		} else {
			resp["result"] = result
		}
		out, err := json.Marshal(resp)
		if err != nil {
			f.t.Errorf("假 TrueNAS 的回复写不成 JSON: %v", err)
			return
		}
		if err := ws.Write(ctx, websocket.MessageText, out); err != nil {
			return
		}
		f.mu.Lock()
		f.replies++
		drop := first && f.dropAfter > 0 && f.replies >= f.dropAfter
		f.mu.Unlock()
		if drop {
			_ = ws.CloseNow()
			return
		}
	}
}

// defaults 是一台有两个存储池（UniData 是 RAIDZ、有根数据集；Cold 没有根数据集）、四块硬盘、两块连着的物理网卡的 TrueNAS。
func (f *fakeTrueNAS) defaults() {
	ok := func(v any) fakeHandler { return func([]json.RawMessage) (any, *fakeRPCError) { return v, nil } }
	f.handle("system.info", func([]json.RawMessage) (any, *fakeRPCError) {
		f.mu.Lock()
		now := f.server
		f.mu.Unlock()
		return map[string]any{
			"version": "TrueNAS-SCALE-25.04.1", "hostname": "nas", "model": "Intel(R) Core(TM) i5-12400", "cores": 12,
			"physmem": 64 << 30, "uptime_seconds": 90061.7, "loadavg": []float64{1.5, 1.25, 1}, "datetime": map[string]any{"$date": now.UnixMilli()},
		}, nil
	})
	f.handle("interface.query", ok([]map[string]any{
		{"name": "enp1s0", "type": "PHYSICAL", "state": map[string]any{"link_state": "LINK_STATE_UP"}},
		{"name": "enp2s0", "type": "PHYSICAL", "state": map[string]any{"link_state": "LINK_STATE_UP"}},
		{"name": "enp3s0", "type": "PHYSICAL", "state": map[string]any{"link_state": "LINK_STATE_DOWN"}},
		{"name": "br0", "type": "BRIDGE", "state": map[string]any{"link_state": "LINK_STATE_UP"}},
	}))
	f.handle("pool.query", ok([]map[string]any{
		{"name": "UniData", "status": "ONLINE", "healthy": true, "size": 48_000_000_000_000, "allocated": 40_000_000_000_000, "free": 8_000_000_000_000},
		{"name": "Cold", "status": "DEGRADED", "healthy": false, "size": 4_000_000_000_000, "allocated": 1_000_000_000_000, "free": 3_000_000_000_000},
	}))
	f.handle("pool.dataset.query", ok([]map[string]any{
		{"id": "UniData", "used": map[string]any{"parsed": 27_000_000_000_000, "value": "24.6T"}, "available": map[string]any{"parsed": 5_000_000_000_000}},
	}))
	f.handle("disk.query", ok([]map[string]any{
		{"name": "sda", "model": "Virtual_disk", "type": "SSD", "size": 50_000_000_000, "pool": "boot-pool", "serial": "SN-SHOULD-NOT-LEAK"},
		{"name": "sdb", "model": "ST16000NM000J", "type": "HDD", "size": 16_000_000_000_000, "pool": "UniData"},
		{"name": "sdc", "model": "ST16000NM000J", "type": "HDD", "size": 16_000_000_000_000, "pool": "UniData"},
		{"name": "nvme0n1", "model": "CN600 2TB", "type": "SSD", "size": 2_000_000_000_000, "pool": "Cold"},
	}))
	f.handle("disk.temperatures", ok(map[string]any{"sda": 0, "sdb": 41, "sdc": 38.5, "nvme0n1": 61}))
	f.handle("alert.list", ok([]map[string]any{
		{"klass": "ZpoolCapacityWarning", "level": "WARNING", "dismissed": false, "args": map[string]any{"volume": "UniData", "capacity": 85}, "formatted": "Space usage for pool \"UniData\" is 85%.", "datetime": map[string]any{"$date": int64(1_799_990_000_000)}},
		{"klass": "RsyncFailed", "level": "CRITICAL", "dismissed": false, "args": map[string]any{"direction": "PULL", "id": 3, "path": "/mnt/UniData/backup"}, "formatted": "Rsync task failed.", "datetime": map[string]any{"$date": int64(1_799_995_000_000)}},
		{"klass": "SomethingNew", "level": "ERROR", "dismissed": false, "args": nil, "formatted": "<b>Something</b> new happened.\nSecond line.", "datetime": map[string]any{"$date": int64(1_799_996_000_000)}},
		{"klass": "HasUpdate", "level": "INFO", "dismissed": false, "args": nil, "formatted": "A system update is available."},
		{"klass": "AppUpdate", "level": "NOTICE", "dismissed": true, "args": map[string]any{"name": "plex"}},
	}))
	f.handle("app.query", ok([]map[string]any{
		{"name": "portainer", "state": "RUNNING", "upgrade_available": false},
		{"name": "jellyfin", "state": "RUNNING", "upgrade_available": true},
		{"name": "old", "state": "STOPPED", "image_updates_available": true},
	}))
	f.handle("reporting.get_data", f.reporting)
}

// graphValue 是默认的报表：CPU 12.5%、可用内存 16 GiB、温度 0（读不到）、enp1s0 收 800 发 80、enp2s0 收 200 发 20（千比特/秒）。
func (f *fakeTrueNAS) graphValue(graph, ident, col string, _ int64) *float64 {
	v := func(x float64) *float64 { return &x }
	switch graph {
	case "cpu":
		if col == "cpu" {
			return v(12.5)
		}
		return v(10)
	case "memory":
		return v(16 << 30)
	case "cputemp":
		return v(0)
	case "interface":
		rates := map[string][2]float64{"enp1s0": {800, 80}, "enp2s0": {200, 20}}
		r := rates[ident]
		if col == "received" {
			return v(r[0])
		}
		return v(r[1])
	}
	return nil
}

var fakeLegends = map[string][]string{
	"cpu":       {"time", "cpu", "cpu0", "cpu1"},
	"memory":    {"time", "available"},
	"cputemp":   {"time", "cpu0", "cpu1", "cpu"},
	"interface": {"time", "received", "sent"},
}

// reporting 是 reporting.get_data：每秒一行，从 start 到 end（含两头）。
func (f *fakeTrueNAS) reporting(params []json.RawMessage) (any, *fakeRPCError) {
	var queries []struct {
		Name       string `json:"name"`
		Identifier string `json:"identifier"`
	}
	var opts struct {
		Aggregate bool  `json:"aggregate"`
		Start     int64 `json:"start"`
		End       int64 `json:"end"`
	}
	bad := &fakeRPCError{Code: -32602, Message: "Invalid params"}
	if len(params) != 2 || json.Unmarshal(params[0], &queries) != nil || json.Unmarshal(params[1], &opts) != nil {
		f.t.Errorf("reporting.get_data 的参数不对: %d 个", len(params))
		return nil, bad
	}
	if !opts.Aggregate {
		f.t.Errorf("aggregate:false 在真机上会让服务端自己的校验失败")
		return nil, bad
	}
	f.mu.Lock()
	f.windows = append(f.windows, [2]int64{opts.Start, opts.End})
	series, noAgg := f.series, f.noAggregations
	f.mu.Unlock()
	out := make([]map[string]any, 0, len(queries))
	for _, q := range queries {
		legend := fakeLegends[q.Name]
		var rows [][]*float64
		sum := map[string]float64{}
		cnt := map[string]int{}
		for ts := opts.Start; ts <= opts.End; ts++ {
			tv := float64(ts)
			row := []*float64{&tv}
			for _, col := range legend[1:] {
				val := series(q.Name, q.Identifier, col, ts)
				row = append(row, val)
				if val != nil {
					sum[col] += *val
					cnt[col]++
				}
			}
			rows = append(rows, row)
		}
		mean := map[string]any{}
		for col, s := range sum {
			mean[col] = s / float64(cnt[col])
		}
		g := map[string]any{"name": q.Name, "identifier": q.Name, "legend": legend, "data": rows, "start": opts.Start, "end": opts.End}
		if q.Identifier != "" {
			g["identifier"] = q.Identifier
		}
		if !noAgg {
			g["aggregations"] = map[string]any{"min": map[string]any{}, "mean": mean, "max": map[string]any{}}
		}
		out = append(out, g)
	}
	return out, nil
}
