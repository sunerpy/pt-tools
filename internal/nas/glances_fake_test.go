package nas

import (
	"context"
	"crypto/x509"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// glancesFixture 是 testdata/glances-4.5.8 里的一份回应（格式化过）：一台真的 Glances 4.5.8（官方镜像 nicolargo/glances:4.5.8-full，
// --pid host、host 网络）回的原文，主机名与容器名、ID、镜像换掉了，percpu 只留两项。
func glancesFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "glances-4.5.8", name))
	require.NoError(t, err)
	return string(b)
}

// glancesReply 是假 Glances 对一个路径的回应。
type glancesReply struct {
	status int
	body   string
	header map[string]string
}

// fakeGlances 是测试用的 Glances：按路径回 glancesReply，默认是 testdata 里那台真机的回应；记下收到的每个请求。
// user、pass 不为空时，除了 /api/4/status 都要 HTTP Basic 认证（同 Glances 4.5.8：status 不要认证）。
type fakeGlances struct {
	t  *testing.T
	ts *httptest.Server

	mu      sync.Mutex
	replies map[string][]glancesReply // 同一路径有几份时按顺序给，最后一份一直给
	reqs    []fakeGlancesReq
	user    string
	pass    string
}

type fakeGlancesReq struct {
	path string
	auth string // Authorization 头（没有时为空）
}

func newFakeGlances(t *testing.T, tls bool) *fakeGlances {
	t.Helper()
	f := &fakeGlances{t: t, replies: map[string][]glancesReply{}}
	for _, p := range []string{"status", "system", "quicklook", "cpu", "mem", "load", "uptime", "network", "fs", "sensors", "containers"} {
		f.set(p, http.StatusOK, glancesFixture(t, p+".json"))
	}
	f.set("raid", http.StatusBadRequest, glancesFixture(t, "raid.400.json"))
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

// set 把 /api/4/<plugin> 的回应换成这几份（按顺序给，最后一份一直给）。
func (f *fakeGlances) set(plugin string, status int, body string, more ...glancesReply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies["/api/4/"+plugin] = append([]glancesReply{{status: status, body: body}}, more...)
}

func (f *fakeGlances) setPath(path string, r ...glancesReply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies[path] = r
}

func (f *fakeGlances) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.reqs = append(f.reqs, fakeGlancesReq{path: r.URL.Path, auth: r.Header.Get("Authorization")})
	needAuth := f.pass != "" && r.URL.Path != "/api/4/status"
	user, pass := f.user, f.pass
	list := f.replies[r.URL.Path]
	var rep glancesReply
	switch {
	case len(list) == 0:
		rep = glancesReply{status: http.StatusNotFound, body: `{"detail":"Not Found"}`}
	default:
		rep = list[0]
		if len(list) > 1 {
			f.replies[r.URL.Path] = list[1:]
		}
	}
	f.mu.Unlock()
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if needAuth {
		u, p, ok := r.BasicAuth()
		if !ok || u != user || p != pass {
			w.Header().Set("WWW-Authenticate", "Basic")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"detail":"Incorrect authentication"}`)
			return
		}
	}
	for k, v := range rep.header {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(rep.status)
	_, _ = io.WriteString(w, rep.body)
}

// paths 是收到的请求的路径（按顺序）。
func (f *fakeGlances) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.reqs))
	for _, r := range f.reqs {
		out = append(out, strings.TrimPrefix(r.path, "/api/4/"))
	}
	return out
}

func (f *fakeGlances) requests() []fakeGlancesReq {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeGlancesReq(nil), f.reqs...)
}

func (f *fakeGlances) resetRequests() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = nil
}

func (f *fakeGlances) url() *url.URL {
	u, err := url.Parse(f.ts.URL)
	require.NoError(f.t, err)
	return u
}

// roots 是认这台 TLS 假 Glances 证书的根证书池（没开 TLS 时为空）。
func (f *fakeGlances) roots() *x509.CertPool {
	if f.ts.TLS == nil {
		return nil
	}
	pool := x509.NewCertPool()
	pool.AddCert(f.ts.Certificate())
	return pool
}

// source 是连这台假 Glances 的来源；不用真的等（第一次读 CPU 全是 0 时的那一下）。
func (f *fakeGlances) source(cfg SourceConfig, roots *x509.CertPool) *glances {
	f.t.Helper()
	if cfg.URL == "" {
		cfg.URL = f.ts.URL
	}
	g, err := newGlances(cfg, roots)
	require.NoError(f.t, err)
	g.wait = func(context.Context, time.Duration) error { return nil }
	f.t.Cleanup(func() { _ = g.Close() })
	return g
}
