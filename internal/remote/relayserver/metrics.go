package relayserver

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/coder/websocket"

	"github.com/sunerpy/pt-tools/internal/remote"
)

// rejectReason 是新连接被拒绝的原因（/metrics 的 reason 标签）。
type rejectReason int

const (
	rejectFull rejectReason = iota
	rejectDraining
	rejectRateLimited
	rejectDisabled
	rejectReasons
)

var rejectNames = [rejectReasons]string{"full", "draining", "rate_limited", "disabled"}

// closeCodes 是 /metrics 里单独计数的关闭码（relay 发起关闭时用的，包括转给对端的）；别的记作 other，标签的取值是固定的。
var closeCodes = [...]int{
	int(websocket.StatusNormalClosure), int(websocket.StatusGoingAway), int(websocket.StatusPolicyViolation),
	int(websocket.StatusInternalError), int(websocket.StatusServiceRestart), int(websocket.StatusTryAgainLater),
	remote.CloseProtocol, remote.CloseAuthFailed, remote.CloseHostOffline, remote.CloseReplaced, remote.CloseLimited, remote.CloseDisabled,
}

// stats 是 /metrics 的计数（只有聚合数，没有 hostId 与 IP）。
type stats struct {
	rejected     [rejectReasons]atomic.Int64
	closed       [len(closeCodes) + 1]atomic.Int64 // closeCodes 各一个，最后一个是 other
	toHost       atomic.Int64
	toClient     atomic.Int64
	authFailures atomic.Int64
}

func (st *stats) closedWith(code int) { st.closed[closeIndex(code)].Add(1) }

// closeIndex 是关闭码在计数里的位置（不在固定集合里的是最后一个 other）。
func closeIndex(code int) int {
	for i, c := range closeCodes {
		if c == code {
			return i
		}
	}
	return len(closeCodes)
}

// serveMetrics 是 GET /metrics：Prometheus 文本格式（0.0.4），手写，不引入依赖。
func (s *Server) serveMetrics(w http.ResponseWriter, _ *http.Request) {
	var b strings.Builder
	metric := func(name, typ, help string, samples ...string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
		for _, line := range samples {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	gauge := func(name, help string, v int64) {
		metric(name, "gauge", help, name+" "+strconv.FormatInt(v, 10))
	}
	bool01 := func(v bool) int64 {
		if v {
			return 1
		}
		return 0
	}
	ready, _ := s.ready()
	s.mu.Lock()
	hosts := len(s.hosts)
	s.mu.Unlock()
	gauge("pt_relay_ready", "Whether the relay accepts new connections (see /ready).", bool01(ready))
	gauge("pt_relay_draining", "Whether the relay is draining before shutdown.", bool01(s.draining.Load()))
	gauge("pt_relay_connections", "WebSocket connections holding a slot (hosts, including those still authenticating, and clients).", s.conns.Load())
	gauge("pt_relay_connection_limit", "Maximum concurrent connections (0 = unlimited).", max(int64(s.cfg.MaxConnections), 0))
	gauge("pt_relay_host_connections", "Host WebSocket connections, including those still authenticating.", s.hostConns.Load())
	gauge("pt_relay_hosts", "Authenticated hosts online.", int64(hosts))
	gauge("pt_relay_clients", "Client WebSocket connections.", s.clientConns.Load())

	rejected := make([]string, 0, rejectReasons)
	for i := range rejectReasons {
		rejected = append(rejected, fmt.Sprintf(`pt_relay_rejected_total{reason=%q} %d`, rejectNames[i], s.stats.rejected[i].Load()))
	}
	metric("pt_relay_rejected_total", "counter", "New connections refused, by reason.", rejected...)

	closed := make([]string, 0, len(closeCodes)+1)
	for i, c := range closeCodes {
		closed = append(closed, fmt.Sprintf(`pt_relay_closed_total{code="%d"} %d`, c, s.stats.closed[i].Load()))
	}
	closed = append(closed, fmt.Sprintf(`pt_relay_closed_total{code="other"} %d`, s.stats.closed[len(closeCodes)].Load()))
	metric("pt_relay_closed_total", "counter", "WebSocket closes the relay initiated, by close code (counted when the close starts, whether or not the peer still receives the frame).", closed...)

	metric("pt_relay_bytes_total", "counter", "Bytes forwarded between clients and hosts (WebSocket message payloads).",
		fmt.Sprintf(`pt_relay_bytes_total{direction="to_host"} %d`, s.stats.toHost.Load()),
		fmt.Sprintf(`pt_relay_bytes_total{direction="to_client"} %d`, s.stats.toClient.Load()))
	metric("pt_relay_auth_failures_total", "counter", "Host connections that failed authentication.",
		"pt_relay_auth_failures_total "+strconv.FormatInt(s.stats.authFailures.Load(), 10))

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	gauge("go_goroutines", "Number of goroutines that currently exist.", int64(runtime.NumGoroutine()))
	gauge("go_memstats_heap_inuse_bytes", "Number of heap bytes that are in use.", int64(ms.HeapInuse))

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}
