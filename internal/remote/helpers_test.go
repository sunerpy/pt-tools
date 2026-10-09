package remote

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// memConn 是内存里的 MsgConn：一对 memConn 互为两端，关掉任意一端两边都读写不了（像一条断开的连接）。
type memConn struct {
	in     chan []byte
	out    chan []byte
	closed chan struct{}
	once   *sync.Once
	// closes 记下本端 Close 的关闭码与原因
	mu     sync.Mutex
	closes []string
}

func memPipe() (*memConn, *memConn) {
	a2b, b2a := make(chan []byte, 256), make(chan []byte, 256)
	closed, once := make(chan struct{}), &sync.Once{}
	return &memConn{in: b2a, out: a2b, closed: closed, once: once}, &memConn{in: a2b, out: b2a, closed: closed, once: once}
}

var errMemClosed = errors.New("连接已经关闭")

func (c *memConn) ReadMsg(ctx context.Context) ([]byte, error) {
	select {
	case b := <-c.in:
		return b, nil
	default:
	}
	select {
	case b := <-c.in:
		return b, nil
	case <-c.closed:
		return nil, errMemClosed
	case <-ctx.Done():
		c.Close(1000, "")
		return nil, ctx.Err()
	}
}

func (c *memConn) WriteMsg(ctx context.Context, b []byte) error {
	cp := append([]byte(nil), b...)
	select {
	case <-c.closed:
		return errMemClosed
	default:
	}
	select {
	case c.out <- cp:
		return nil
	case <-c.closed:
		return errMemClosed
	case <-ctx.Done():
		c.Close(1000, "")
		return ctx.Err()
	}
}

func (c *memConn) Close(code int, reason string) {
	c.mu.Lock()
	c.closes = append(c.closes, reason)
	c.mu.Unlock()
	c.once.Do(func() { close(c.closed) })
}

// plainCipher 是测试用的 Cipher：加个前缀，能看出落库的是不是「密文」。
type plainCipher struct{}

func (plainCipher) Encrypt(plain string) (string, error) { return "enc:" + plain, nil }
func (plainCipher) Decrypt(text string) (string, error) {
	if !strings.HasPrefix(text, "enc:") {
		return "", errors.New("不是密文")
	}
	return strings.TrimPrefix(text, "enc:"), nil
}

func newTestDB(t testing.TB) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "remote.db")), &gorm.Config{})
	require.NoError(t, err)
	// 和生产一样只用一条连接（models 里 SQLite 是单连接、串行写）
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.RemoteSetting{}, &models.RemoteDevice{}))
	return db
}

func newTestStore(t testing.TB) *Store {
	t.Helper()
	return NewStore(newTestDB(t), plainCipher{})
}

// recorded 是假 Dispatcher 收到的一个请求。
type recorded struct {
	Method string
	Path   string
	Header http.Header
	Body   string
	Peer   Peer
}

// fakeDispatcher 把收到的请求记下来，回一个 JSON；路径里带 /slow 时等到请求被取消，带 /panic 时 panic。
type fakeDispatcher struct {
	mu   sync.Mutex
	reqs []recorded
	// started 在 /slow 请求开始处理时收到一个值
	started chan struct{}
	// canceled 在 /slow 请求被取消时收到一个值
	canceled chan struct{}
}

func newFakeDispatcher() *fakeDispatcher {
	return &fakeDispatcher{started: make(chan struct{}, 64), canceled: make(chan struct{}, 64)}
}

func (d *fakeDispatcher) ServeRemote(w http.ResponseWriter, r *http.Request, p Peer) {
	body, _ := io.ReadAll(r.Body)
	d.mu.Lock()
	d.reqs = append(d.reqs, recorded{Method: r.Method, Path: r.URL.RequestURI(), Header: r.Header.Clone(), Body: string(body), Peer: p})
	d.mu.Unlock()
	switch {
	case strings.Contains(r.URL.Path, "/slow"):
		d.started <- struct{}{}
		<-r.Context().Done()
		d.canceled <- struct{}{}
		return
	case strings.Contains(r.URL.Path, "/panic"):
		panic("boom")
	case strings.Contains(r.URL.Path, "/big"):
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(make([]byte, 3*MaxFramePayload+7))
		return
	case strings.Contains(r.URL.Path, "/nothing"):
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Set-Cookie", "session=leak")
	w.Header().Set("X-Internal", "secret")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"path": r.URL.Path, "device": p.Device.ID, "scopes": p.Device.Scopes, "via": p.Via, "body": string(body)})
}

func (d *fakeDispatcher) last() recorded {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.reqs[len(d.reqs)-1]
}

func (d *fakeDispatcher) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.reqs)
}

// testHost 是一台打开了远程访问的主机，加上它的 Store 与假 Dispatcher。
type testHost struct {
	*Host
	store  *Store
	disp   *fakeDispatcher
	keys   *HostKeys
	paired chan Device
	audits chan string
}

func newTestHost(t *testing.T, settings Settings, opts ...func(*Config)) *testHost {
	t.Helper()
	store := newTestStore(t)
	th := &testHost{store: store, disp: newFakeDispatcher(), paired: make(chan Device, 8), audits: make(chan string, 32)}
	cfg := Config{
		Store: store, Dispatcher: th.disp, Version: "v1.0.0-test",
		OnPaired: func(_ context.Context, d Device) { th.paired <- d },
		Audit: func(_ context.Context, id uint, command, result string) {
			th.audits <- command + " " + result
		},
	}
	for _, o := range opts {
		o(&cfg)
	}
	th.Host = New(cfg)
	t.Cleanup(th.Close)
	settings.Enabled = true
	_, err := store.SaveSettings(context.Background(), settings)
	require.NoError(t, err)
	require.NoError(t, th.Start(context.Background()))
	th.keys, err = store.HostKeys(context.Background(), false)
	require.NoError(t, err)
	require.NotNil(t, th.keys)
	return th
}

// connect 用内存连接连上主机（经 serveConn，和直连、relay 是同一条路），返回设备端的会话。
func (th *testHost) connect(t *testing.T, device []byte, via string) (*Client, error) {
	t.Helper()
	dk, err := KeypairFromPrivate(device)
	require.NoError(t, err)
	a, b := memPipe()
	go th.serveConn(b, via)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return Connect(ctx, a, ClientConfig{HostID: th.keys.HostID(), HostKey: th.keys.Noise.Public, Device: dk, Client: "test"})
}

// addDevice 直接在库里记一台设备，返回它的私钥。
func (th *testHost) addDevice(t *testing.T, name string, scopes []string) (Device, []byte) {
	t.Helper()
	dk, err := GenerateKeypair(nil)
	require.NoError(t, err)
	d, err := th.store.CreateDevice(context.Background(), name, dk.Public, scopes)
	require.NoError(t, err)
	return d, dk.Private
}

// get 经隧道发一个 GET，返回状态码与回应体。
func get(t *testing.T, c *Client, path string, header map[string]string) (int, http.Header, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	require.NoError(t, err)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := c.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, string(b)
}

// waitDone 等会话结束。
func waitDone(t *testing.T, c *Client) {
	t.Helper()
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("会话没有结束")
	}
}
