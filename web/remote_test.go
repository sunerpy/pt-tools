package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/remote"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// remoteCipher 是测试用的主机密钥加解密（加个前缀）。
type remoteCipher struct{}

func (remoteCipher) Encrypt(plain string) (string, error) { return "enc:" + plain, nil }
func (remoteCipher) Decrypt(text string) (string, error) {
	if !strings.HasPrefix(text, "enc:") {
		return "", errors.New("不是密文")
	}
	return strings.TrimPrefix(text, "enc:"), nil
}

type remoteEnv struct {
	*appEnv
	host  *remote.Host
	store *remote.Store
	// ts 跑的是 Serve 用的那一套完整路由：直连入口 /remote/v1/stream 也在里面
	ts *httptest.Server
}

func newRemoteEnv(t *testing.T) *remoteEnv {
	t.Helper()
	e := newAppEnv(t)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.RemoteSetting{}, &models.RemoteDevice{}))
	store := remote.NewStore(global.GlobalDB.DB, remoteCipher{})
	h := remote.New(remote.Config{Store: store, Dispatcher: e.srv.RemoteDispatcher(), Audit: e.srv.RemoteAudit, Version: "v1.0.0-test"})
	e.srv.SetRemoteHost(h)
	ts := httptest.NewServer(e.handler)
	t.Cleanup(func() {
		h.Close()
		ts.Close()
	})
	_, err := h.UpdateSettings(context.Background(), remote.Settings{Enabled: true, DirectURL: ts.URL})
	require.NoError(t, err)
	return &remoteEnv{appEnv: e, host: h, store: store, ts: ts}
}

// device 在库里记一台设备，并经直连入口连上，返回设备与会话。
func (e *remoteEnv) device(scopes []string) (remote.Device, *remote.Client) {
	e.t.Helper()
	k, err := remote.GenerateKeypair(nil)
	require.NoError(e.t, err)
	d, err := e.store.CreateDevice(context.Background(), "测试手机", k.Public, scopes)
	require.NoError(e.t, err)
	ov, err := e.host.Overview(context.Background())
	require.NoError(e.t, err)
	hostKey, err := remote.DecodeKey(ov.HostKey)
	require.NoError(e.t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := remote.DialDirect(ctx, e.ts.URL, e.ts.Client())
	require.NoError(e.t, err)
	c, err := remote.Connect(ctx, raw, remote.ClientConfig{HostID: ov.HostID, HostKey: hostKey, Device: k})
	require.NoError(e.t, err)
	require.Equal(e.t, remote.ModeDevice, c.Mode())
	e.t.Cleanup(c.Close)
	return d, c
}

// tunnel 经隧道发一个请求，返回状态码与回应体。
func tunnel(t *testing.T, c *remote.Client, method, path, body string, header map[string]string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, path, rd)
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := c.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(b)
}

// 经真实的 mux 与直连入口：设备的主体是 remote_device，权限范围是配对时给的；App API 每条路由的权限范围照样生效
func TestRemoteTunnel_ScopeMatrix(t *testing.T) {
	e := newRemoteEnv(t)
	readDev, readC := e.device(remote.ScopesRead)
	_, fullC := e.device(remote.ScopesFull)

	status, body := tunnel(t, readC, http.MethodGet, appPrefix+"/meta", "", nil)
	require.Equal(t, http.StatusOK, status, body)
	var meta AppMeta
	require.NoError(t, json.Unmarshal([]byte(body), &meta))
	assert.Equal(t, middleware.KindRemoteDevice, meta.Principal.Kind)
	assert.Equal(t, readDev.Name, meta.Principal.Name)
	assert.Equal(t, []string{apitoken.ScopeAppRead}, meta.Principal.Scopes)

	for _, rt := range e.srv.appRoutes() {
		path := appPrefix + concretePath(rt.Path)
		gotRead, _ := tunnel(t, readC, rt.Method, path, "{}", nil)
		gotFull, _ := tunnel(t, fullC, rt.Method, path, "{}", nil)
		assert.NotContains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, gotFull, "%s %s 完全控制", rt.Method, rt.Path)
		if rt.Scope == apitoken.ScopeAppRead {
			assert.NotContains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, gotRead, "%s %s 只读", rt.Method, rt.Path)
		} else {
			assert.Equal(t, http.StatusForbidden, gotRead, "%s %s 只读", rt.Method, rt.Path)
		}
	}
}

// 隧道里只有 App API：现有的 /api/*、/mcp、令牌管理一律到不了；App API 下不存在的路径回 JSON 的 404
func TestRemoteTunnel_OnlyAppAPI(t *testing.T) {
	e := newRemoteEnv(t)
	_, c := e.device(remote.ScopesFull)
	for _, p := range []string{"/api/sites", "/api/qbit", "/api/tokens", "/api/chatops/notifications/1", "/api/remote", "/mcp", "/", "/login"} {
		status, _ := tunnel(t, c, http.MethodGet, p, "", nil)
		assert.Equal(t, http.StatusForbidden, status, p)
	}
	status, body := tunnel(t, c, http.MethodGet, appPrefix+"/nope", "", nil)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Contains(t, body, "not_found")
}

// 分发器认的只有它放进 context 的设备主体：请求里带着有效的 session Cookie 或者全权限的令牌，也还是那台只读设备
func TestRemoteDispatcher_IgnoresCredentials(t *testing.T) {
	e := newRemoteEnv(t)
	full := e.token(apitoken.Scopes...)
	d := e.srv.RemoteDispatcher()
	dev := remote.Device{ID: 42, Name: "只读手机", Scopes: remote.ScopesRead}
	req := httptest.NewRequest(http.MethodPost, appPrefix+"/torrents/actions", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-app"})
	req.Header.Set("Authorization", "Bearer "+full)
	w := httptest.NewRecorder()
	d.ServeRemote(w, req, remote.Peer{Device: dev, Via: remote.ViaDirect})
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "forbidden", errorCode(t, w))

	// 反过来：主 mux 上的普通请求伪造不出设备主体（键没有导出），不带凭证就是 401
	assert.Equal(t, http.StatusUnauthorized, e.do(appReq{method: http.MethodGet, path: appPrefix + "/meta"}).Code)
	// 分发器的 mux 里只有 App API
	w = httptest.NewRecorder()
	d.ServeRemote(w, httptest.NewRequest(http.MethodGet, "/api/sites", nil), remote.Peer{Device: dev})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// 设备做的写操作记进操作审计：通道是 remote_device，用户是设备编号，被拒的也记
func TestRemoteTunnel_Audit(t *testing.T) {
	e := newRemoteEnv(t)
	readDev, readC := e.device(remote.ScopesRead)
	fullDev, fullC := e.device(remote.ScopesFull)
	status, _ := tunnel(t, readC, http.MethodPost, appPrefix+"/torrents/actions", `{}`, nil)
	assert.Equal(t, http.StatusForbidden, status)
	status, _ = tunnel(t, fullC, http.MethodPost, appPrefix+"/torrents/actions", `{"bad":1}`, nil)
	assert.Equal(t, http.StatusBadRequest, status)
	tunnel(t, fullC, http.MethodGet, appPrefix+"/meta", "", nil)

	entries := e.audit.all()
	require.Len(t, entries, 2, "读请求不记审计")
	assert.Equal(t, middleware.KindRemoteDevice, entries[0].ChannelType)
	assert.Equal(t, strconv.FormatUint(uint64(readDev.ID), 10), entries[0].ChannelUserID)
	assert.Equal(t, "denied:scope", entries[0].Result)
	assert.Equal(t, strconv.FormatUint(uint64(fullDev.ID), 10), entries[1].ChannelUserID)
	assert.Equal(t, "error:http_400", entries[1].Result)
	assert.Equal(t, "POST "+appPrefix+"/torrents/actions", entries[1].Command)
}

// 设置接口只认 session：令牌（哪怕全权限）与没登录都是 401
func TestRemoteAPI_SessionOnly(t *testing.T) {
	e := newRemoteEnv(t)
	full := e.token(apitoken.Scopes...)
	for _, q := range []appReq{
		{method: http.MethodGet, path: "/api/remote"},
		{method: http.MethodGet, path: "/api/remote", bearer: full},
		{method: http.MethodPost, path: "/api/remote/pairings", bearer: full, body: `{}`},
		{method: http.MethodGet, path: "/api/remote/devices", bearer: full},
		{method: http.MethodPost, path: "/api/remote/keys/rotate", bearer: full},
	} {
		assert.Equal(t, http.StatusUnauthorized, e.do(q).Code, "%s %s", q.method, q.path)
	}
	assert.Equal(t, http.StatusOK, e.do(appReq{method: http.MethodGet, path: "/api/remote", session: true}).Code)
}

func decodeJSON[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v), w.Body.String())
	return v
}

func TestRemoteAPI_SettingsAndPairing(t *testing.T) {
	e := newRemoteEnv(t)
	w := e.do(appReq{method: http.MethodGet, path: "/api/remote", session: true})
	require.Equal(t, http.StatusOK, w.Code)
	ov := decodeJSON[remote.Overview](t, w)
	assert.True(t, ov.Enabled)
	assert.True(t, remote.ValidHostID(ov.HostID))
	assert.Equal(t, remote.StreamPath, ov.StreamPath)
	assert.NotContains(t, w.Body.String(), "host_keys")

	for _, body := range []string{`{"enabled":true,"relays":["https://x"]}`, `{"enabled":true,"direct_url":"ftp://x"}`, `{"enabled":true,"extra":1}`} {
		assert.Equal(t, http.StatusBadRequest, e.do(appReq{method: http.MethodPut, path: "/api/remote", session: true, body: body}).Code, body)
	}
	w = e.do(appReq{method: http.MethodPut, path: "/api/remote", session: true, body: `{"enabled":true,"relays":["wss://Relay.example.com/"],"direct_url":"` + e.ts.URL + `/"}`})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	ov = decodeJSON[remote.Overview](t, w)
	assert.Equal(t, []string{"wss://relay.example.com"}, ov.Relays)
	assert.Equal(t, e.ts.URL, ov.DirectURL)
	require.Len(t, ov.RelayStatus, 1)

	// 添加设备：只读；链接与二维码只在回应里出现一次，回应不缓存
	w = e.do(appReq{method: http.MethodPost, path: "/api/remote/pairings", session: true, body: `{"scopes":["app:read"]}`})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	pv := decodeJSON[RemotePairingView](t, w)
	assert.True(t, strings.HasPrefix(pv.QRSVG, "<svg "))
	link, err := remote.ParseLink(pv.Link)
	require.NoError(t, err)
	assert.Equal(t, ov.HostID, link.HostID)
	assert.Equal(t, []string{"wss://relay.example.com"}, link.Relays)
	assert.Equal(t, []string{apitoken.ScopeAppRead}, pv.Scopes)

	w = e.do(appReq{method: http.MethodGet, path: "/api/remote/pairings/current", session: true})
	require.Equal(t, http.StatusOK, w.Code)
	st := decodeJSON[remote.PairingStatus](t, w)
	assert.Equal(t, remote.PairingWaiting, st.State)
	assert.NotContains(t, w.Body.String(), remote.EncodeKey(link.Secret), "状态里没有配对密钥")

	// 用链接配对一台设备（经直连入口）
	k, err := remote.GenerateKeypair(nil)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := remote.DialDirect(ctx, link.Direct, e.ts.Client())
	require.NoError(t, err)
	c, err := remote.Connect(ctx, raw, remote.ClientConfig{HostID: link.HostID, HostKey: link.HostKey, Device: k})
	require.NoError(t, err)
	require.Equal(t, remote.ModePairing, c.Mode())
	dev, err := c.Pair(ctx, link.Secret, "我的手机")
	require.NoError(t, err)
	assert.Equal(t, []string{apitoken.ScopeAppRead}, dev.Scopes)
	st = decodeJSON[remote.PairingStatus](t, e.do(appReq{method: http.MethodGet, path: "/api/remote/pairings/current", session: true}))
	assert.Equal(t, remote.PairingPaired, st.State)

	// 配对记审计：通道 remote_device，用户是新设备的编号
	var pairAudit []string
	for _, a := range e.audit.all() {
		if a.Command == "remote:pair" {
			pairAudit = append(pairAudit, a.ChannelType+" "+a.ChannelUserID+" "+a.Result)
		}
	}
	assert.Equal(t, []string{middleware.KindRemoteDevice + " " + strconv.FormatUint(uint64(dev.ID), 10) + " success"}, pairAudit)

	// 新开再取消
	require.Equal(t, http.StatusOK, e.do(appReq{method: http.MethodPost, path: "/api/remote/pairings", session: true, body: `{}`}).Code)
	w = e.do(appReq{method: http.MethodDelete, path: "/api/remote/pairings/current", session: true})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, remote.PairingClosed, decodeJSON[remote.PairingStatus](t, w).State)
	assert.Equal(t, http.StatusBadRequest, e.do(appReq{method: http.MethodPost, path: "/api/remote/pairings", session: true, body: `{"scopes":["mcp:read"]}`}).Code)

	// 关掉以后不能添加设备
	require.Equal(t, http.StatusOK, e.do(appReq{method: http.MethodPut, path: "/api/remote", session: true, body: `{"enabled":false}`}).Code)
	assert.Equal(t, http.StatusConflict, e.do(appReq{method: http.MethodPost, path: "/api/remote/pairings", session: true, body: `{}`}).Code)
}

func TestRemoteAPI_Devices(t *testing.T) {
	e := newRemoteEnv(t)
	d, c := e.device(remote.ScopesFull)
	w := e.do(appReq{method: http.MethodGet, path: "/api/remote/devices", session: true})
	require.Equal(t, http.StatusOK, w.Code)
	list := decodeJSON[[]remote.DeviceStatus](t, w)
	require.Len(t, list, 1)
	assert.True(t, list[0].Online)
	assert.Equal(t, remote.ViaDirect, list[0].OnlineVia)
	assert.NotContains(t, w.Body.String(), "public_key")

	id := strconv.FormatUint(uint64(d.ID), 10)
	w = e.do(appReq{method: http.MethodPut, path: "/api/remote/devices/" + id, session: true, body: `{"name":"新名字"}`})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "新名字", decodeJSON[remote.Device](t, w).Name)
	// 改成只读：会话立即断开
	w = e.do(appReq{method: http.MethodPut, path: "/api/remote/devices/" + id, session: true, body: `{"scopes":["app:read"]}`})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("改权限以后会话没有断开")
	}
	assert.Equal(t, remote.GoAwayScopeChanged, c.GoAway())

	for _, q := range []appReq{
		{method: http.MethodPut, path: "/api/remote/devices/x", body: `{}`},
		{method: http.MethodPut, path: "/api/remote/devices/" + id, body: `{"scopes":["app:write"]}`},
		{method: http.MethodPut, path: "/api/remote/devices/" + id, body: `{"other":1}`},
	} {
		q.session = true
		assert.Equal(t, http.StatusBadRequest, e.do(q).Code, q.body)
	}
	assert.Equal(t, http.StatusNotFound, e.do(appReq{method: http.MethodPost, path: "/api/remote/devices/999/revoke", session: true}).Code)
	assert.Equal(t, http.StatusConflict, e.do(appReq{method: http.MethodDelete, path: "/api/remote/devices/" + id, session: true}).Code, "没撤销不能删")
	w = e.do(appReq{method: http.MethodPost, path: "/api/remote/devices/" + id + "/revoke", session: true})
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotNil(t, decodeJSON[remote.Device](t, w).RevokedAt)
	assert.Equal(t, http.StatusConflict, e.do(appReq{method: http.MethodPut, path: "/api/remote/devices/" + id, session: true, body: `{"name":"x"}`}).Code)
	assert.Equal(t, http.StatusOK, e.do(appReq{method: http.MethodDelete, path: "/api/remote/devices/" + id, session: true}).Code)
	assert.Empty(t, decodeJSON[[]remote.DeviceStatus](t, e.do(appReq{method: http.MethodGet, path: "/api/remote/devices", session: true})))
}

// 轮换主机密钥：hostId 换了，设备全部撤销，在线的会话断开
func TestRemoteAPI_Rotate(t *testing.T) {
	e := newRemoteEnv(t)
	_, c := e.device(remote.ScopesFull)
	before := decodeJSON[remote.Overview](t, e.do(appReq{method: http.MethodGet, path: "/api/remote", session: true}))
	w := e.do(appReq{method: http.MethodPost, path: "/api/remote/keys/rotate", session: true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	after := decodeJSON[remote.Overview](t, w)
	assert.NotEqual(t, before.HostID, after.HostID)
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("轮换以后会话没有断开")
	}
	list := decodeJSON[[]remote.DeviceStatus](t, e.do(appReq{method: http.MethodGet, path: "/api/remote/devices", session: true}))
	require.Len(t, list, 1)
	assert.NotNil(t, list[0].RevokedAt)
}

// 没接远程访问：设置接口 503，直连入口 404；关着时直连入口也是 404
func TestRemoteNotWired(t *testing.T) {
	e := newAppEnv(t)
	assert.Equal(t, http.StatusServiceUnavailable, e.do(appReq{method: http.MethodGet, path: "/api/remote", session: true}).Code)
	assert.Equal(t, http.StatusNotFound, e.do(appReq{method: http.MethodGet, path: remote.StreamPath}).Code)

	re := newRemoteEnv(t)
	_, err := re.host.UpdateSettings(context.Background(), remote.Settings{Enabled: false})
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, re.do(appReq{method: http.MethodGet, path: remote.StreamPath}).Code)
}

func TestQRSVG(t *testing.T) {
	svg, err := qrSVG("pttools://pair?v=1&h=abc")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(svg, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 `))
	assert.True(t, strings.HasSuffix(svg, `"/></svg>`))
	assert.Contains(t, svg, `<path fill="#000" d="M`)
	assert.NotContains(t, svg, "<script")
}
