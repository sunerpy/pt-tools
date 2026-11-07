package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/internal/health"
	"github.com/sunerpy/pt-tools/internal/nas"
	"github.com/sunerpy/pt-tools/models"
)

// fakeNASKind 是测试里登记的来源类型：用户名可填可不填，填了才要密钥；http:// 不带凭据时直接可用（同 Glances）。
const fakeNASKind = "fakenas"

// fakeNASReq 是假 NAS 收到的一个请求的凭据头。
type fakeNASReq struct {
	Path, User, Secret, Device, OTP string
}

// fakeNAS 是测试用的 NAS（HTTPS，自签证书，系统根证书不认）：记下收到的每个请求。secret 不为空时 X-Secret 要对上；
// otp 为真时还要带它发过的设备令牌（X-Device）或正确的验证码（X-OTP: 123456），对上验证码时发一个新的设备令牌。
type fakeNAS struct {
	ts      *httptest.Server
	mu      sync.Mutex
	secret  string
	otp     bool
	devices map[string]bool
	issued  int
	reqs    []fakeNASReq
}

func newFakeNAS(t *testing.T, secret string, otp bool) *fakeNAS {
	t.Helper()
	f := &fakeNAS{secret: secret, otp: otp, devices: map[string]bool{}}
	ts := httptest.NewUnstartedServer(http.HandlerFunc(f.serve))
	ts.Config.ErrorLog = log.New(io.Discard, "", 0) // 证书被拒的握手是这些测试要的，不打日志
	ts.StartTLS()
	t.Cleanup(ts.Close)
	f.ts = ts
	return f
}

func (f *fakeNAS) url() string { return f.ts.URL }

func (f *fakeNAS) pin() string { return nas.Fingerprint(f.ts.Certificate()) }

func (f *fakeNAS) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	req := fakeNASReq{
		Path: r.URL.Path, User: r.Header.Get("X-User"), Secret: r.Header.Get("X-Secret"),
		Device: r.Header.Get("X-Device"), OTP: r.Header.Get("X-OTP"),
	}
	f.reqs = append(f.reqs, req)
	if f.secret != "" && req.Secret != f.secret {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"密码不对"}`))
		return
	}
	resp := map[string]any{"hostname": "fake-nas", "platform": "FakeOS", "version": "1.0", "model": "FN-1"}
	if f.otp && !f.devices[req.Device] {
		if req.OTP != "123456" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"要验证码"}`))
			return
		}
		f.issued++
		did := fmt.Sprintf("dev-%d", f.issued)
		f.devices[did] = true
		resp["did"] = did
	}
	writeJSON(w, resp)
}

func (f *fakeNAS) requests() []fakeNASReq {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeNASReq(nil), f.reqs...)
}

func (f *fakeNAS) setSecret(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secret = s
}

// fakeNASCall 是假类型的一次登录与读取：GET /api/info，凭据放进请求头。认证失败的原因故意带上用过的密钥（看它会不会被遮掉）。
func fakeNASCall(ctx context.Context, in nas.TestInput) (*nas.TestResult, error) {
	c := nas.HTTPClient(in.URL, in.PinSHA256, 5*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.URL.String()+"/api/info", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-User", in.Username)
	req.Header.Set("X-Secret", in.Secret)
	req.Header.Set("X-Device", in.Extra["did"])
	req.Header.Set("X-OTP", in.OTP)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Hostname string `json:"hostname"`
		Platform string `json:"platform"`
		Version  string `json:"version"`
		Model    string `json:"model"`
		DID      string `json:"did"`
		Error    string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	switch resp.StatusCode {
	case http.StatusOK:
		res := &nas.TestResult{System: nas.System{Hostname: body.Hostname, Platform: body.Platform, Version: body.Version, Model: body.Model}}
		if body.DID != "" {
			res.Extra = map[string]string{"did": body.DID}
		}
		return res, nil
	case http.StatusUnauthorized:
		return nil, &nas.AuthError{Reason: body.Error + "（用了 " + in.Secret + "）"}
	case http.StatusForbidden:
		return nil, &nas.AuthError{Reason: body.Error, OTP: true}
	default:
		return nil, fmt.Errorf("回了 %d", resp.StatusCode)
	}
}

// fakeNASSource 是假类型的采集：同测试一样请求 /api/info。
type fakeNASSource struct{ in nas.TestInput }

func (s *fakeNASSource) Collect(ctx context.Context, _ bool) (*nas.Snapshot, error) {
	res, err := fakeNASCall(ctx, s.in)
	if err != nil {
		return nil, err
	}
	return &nas.Snapshot{System: res.System, CPU: nas.CPU{Percent: 12}, Memory: nas.Memory{Total: 100, Used: 40, Available: 60}}, nil
}

func (s *fakeNASSource) Close() error { return nil }

// fakeLocalNAS 是假的本机来源（不读这台机器的 /proc）。
type fakeLocalNAS struct{}

func (fakeLocalNAS) Collect(context.Context, bool) (*nas.Snapshot, error) {
	return &nas.Snapshot{System: nas.System{Hostname: "pt-tools-host", Cores: 4}, CPU: nas.CPU{Percent: 5}}, nil
}

func (fakeLocalNAS) Close() error { return nil }

func registerFakeNASKind(t *testing.T) {
	t.Helper()
	unregister := nas.RegisterKind(nas.Kind{
		Name: fakeNASKind, Label: "假 NAS", Username: nas.UsernameOptional,
		NeedsSecret: func(username string) bool { return username != "" },
		Plain:       nas.PlainWithoutCredentials,
		New: func(cfg nas.SourceConfig) (nas.Source, error) {
			u, err := url.Parse(cfg.URL)
			if err != nil {
				return nil, err
			}
			return &fakeNASSource{in: nas.TestInput{URL: u, Username: cfg.Username, Secret: cfg.Secret, Extra: cfg.Extra, PinSHA256: cfg.PinSHA256}}, nil
		},
		Test: fakeNASCall,
	})
	t.Cleanup(unregister)
}

// testNASLoad 读测试库里的设置（同 cmd 的 loadNASConfig）。
func testNASLoad(ctx context.Context, srv *Server) (nas.Config, error) {
	db := global.GlobalDB.DB.WithContext(ctx)
	set, err := models.GetNasSetting(db)
	if err != nil {
		return nas.Config{}, err
	}
	var rows []models.NasSource
	if err = db.Order("id").Find(&rows).Error; err != nil {
		return nas.Config{}, err
	}
	cfg := nas.Config{LocalEnabled: set.LocalEnabled}
	for _, r := range rows {
		sc := nas.SourceConfig{
			ID: r.ID, Name: r.Name, Kind: r.Kind, Enabled: r.Enabled, URL: r.URL, Username: r.Username,
			PinSHA256: r.TLSPinSHA256, InsecureAck: r.InsecureAck, Revision: r.Revision,
		}
		if r.SecretEncrypted != "" {
			if sc.Secret, err = srv.store.DecryptCookie(r.SecretEncrypted); err != nil {
				return nas.Config{}, err
			}
		}
		if r.ExtraEncrypted != "" {
			plain, derr := srv.store.DecryptCookie(r.ExtraEncrypted)
			if derr != nil {
				return nas.Config{}, derr
			}
			if sc.Extra, err = nas.DecodeExtra(plain); err != nil {
				return nas.Config{}, err
			}
		}
		cfg.Sources = append(cfg.Sources, sc)
	}
	return cfg, nil
}

// nasEnv 是一套带采集器的服务：假类型已登记，采集只在启动、保存与「重新连接」时各做一次（间隔与退避都是 1 小时）。
// now 是测试限速与暂存表用的时间。
type nasEnv struct {
	srv *Server
	mux *http.ServeMux
	col *nas.Collector
	db  *gorm.DB
	now time.Time
}

func setupNASEnv(t *testing.T) *nasEnv {
	t.Helper()
	snap := crypto.SnapshotKeyState()
	t.Cleanup(func() { crypto.RestoreKeyState(snap) })
	writeWebTestSecretKey(t)
	crypto.ResetForTest()
	srv := setupServer(t)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.NasSource{}, &models.NasSetting{}))
	registerFakeNASKind(t)
	e := &nasEnv{srv: srv, db: global.GlobalDB.DB, now: time.Now()}
	srv.nasTests.now = func() time.Time { return e.now }
	srv.nasPending.Now = func() time.Time { return e.now }
	e.col = &nas.Collector{
		Load:     func(ctx context.Context) (nas.Config, error) { return testNASLoad(ctx, srv) },
		NewLocal: func() nas.Source { return fakeLocalNAS{} },
		Light:    time.Hour, Full: time.Hour, Timeout: 5 * time.Second, Backoff: []time.Duration{time.Hour},
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.col.Start(ctx)
	t.Cleanup(func() {
		cancel()
		_ = e.col.Stop(context.Background())
	})
	srv.SetNASCollector(e.col)
	e.mux = http.NewServeMux()
	srv.registerNASRoutes(e.mux)
	srv.registerHomeRoutes(e.mux)
	srv.sessions.put("sess-test", "admin")
	return e
}

func (e *nasEnv) do(method, path string, body any) *httptest.ResponseRecorder {
	s := ""
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		s = string(b)
	}
	return serveAuthed(e.mux, method, path, s)
}

func nasDecode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v), w.Body.String())
	return v
}

// test 调一次测试连接（要 200），并把时间往后拨 11 秒（下一次测试不受 10 秒限一次影响）。
func (e *nasEnv) test(t *testing.T, in map[string]any) (nasTestView, *httptest.ResponseRecorder) {
	t.Helper()
	w := e.do(http.MethodPost, "/api/nas/sources/test", in)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	e.now = e.now.Add(nasTestInterval + time.Second)
	return nasDecode[nasTestView](t, w), w
}

// save 新建（id 为 0）或修改一个来源，要 200。
func (e *nasEnv) save(t *testing.T, id uint, in map[string]any) nasSourceEdit {
	t.Helper()
	w := e.saveRaw(id, in)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return nasDecode[nasSourceEdit](t, w)
}

func (e *nasEnv) saveRaw(id uint, in map[string]any) *httptest.ResponseRecorder {
	if id == 0 {
		return e.do(http.MethodPost, "/api/nas/sources", in)
	}
	return e.do(http.MethodPut, "/api/nas/sources/"+strconv.FormatUint(uint64(id), 10), in)
}

func (e *nasEnv) row(t *testing.T, id uint) models.NasSource {
	t.Helper()
	var row models.NasSource
	require.NoError(t, e.db.First(&row, id).Error)
	return row
}

func (e *nasEnv) decrypt(t *testing.T, text string) string {
	t.Helper()
	plain, err := e.srv.store.DecryptCookie(text)
	require.NoError(t, err)
	return plain
}

func (e *nasEnv) waitState(t *testing.T, id, state string) nas.Status {
	t.Helper()
	var st nas.Status
	require.Eventually(t, func() bool {
		for _, s := range e.col.List() {
			if s.ID == id {
				st = s
				return s.State == state
			}
		}
		return false
	}, 5*time.Second, 5*time.Millisecond, "等来源 %s 变成 %s", id, state)
	return st
}

func (e *nasEnv) homeNAS(t *testing.T) (*homeNAS, string) {
	t.Helper()
	w := e.do(http.MethodGet, "/api/home?refresh=1", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		NAS json.RawMessage `json:"nas"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return block[*homeNAS](t, body.NAS)
}

// assertNASCiphertextOnly 直接查表：库里没有这些明文。
func assertNASCiphertextOnly(t *testing.T, db *gorm.DB, plaintexts ...string) {
	t.Helper()
	var rows []map[string]any
	require.NoError(t, db.Raw("SELECT * FROM nas_sources").Scan(&rows).Error)
	require.NotEmpty(t, rows)
	for _, row := range rows {
		for col, v := range row {
			for _, p := range plaintexts {
				assert.NotContains(t, fmt.Sprint(v), p, "nas_sources.%s 里不能有明文", col)
			}
		}
	}
}

func idOf(id uint) string { return strconv.FormatUint(uint64(id), 10) }

func TestNASAPI_RequiresSession(t *testing.T) {
	e := setupNASEnv(t)
	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/api/nas"},
		{http.MethodPost, "/api/nas/sources"},
		{http.MethodPost, "/api/nas/sources/test"},
		{http.MethodGet, "/api/nas/sources/1"},
		{http.MethodPut, "/api/nas/sources/1"},
		{http.MethodDelete, "/api/nas/sources/1"},
		{http.MethodPost, "/api/nas/sources/local/retry"},
		{http.MethodPut, "/api/nas/settings"},
	} {
		w := httptest.NewRecorder()
		e.mux.ServeHTTP(w, httptest.NewRequest(r.method, r.path, nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code, "%s %s", r.method, r.path)
	}
}

// 新建：自签证书在发出凭据以前就断开，用户确认以后再测、保存；库里只有密文，接口不回密钥。
func TestNASAPI_TrustCertificateThenSave(t *testing.T) {
	e := setupNASEnv(t)
	f := newFakeNAS(t, "s3cret", false)
	in := map[string]any{"kind": fakeNASKind, "url": f.url() + "/", "username": "admin", "secret": "s3cret"}

	w := e.do(http.MethodPost, "/api/nas/sources/test", in)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	v := nasDecode[nasTestView](t, w)
	assert.Equal(t, nasTestCertificate, v.Status)
	require.NotNil(t, v.Certificate)
	assert.Equal(t, f.pin(), v.Certificate.SHA256)
	assert.Equal(t, nas.ErrCertUntrusted.Error(), v.Message)
	assert.Empty(t, f.requests(), "确认证书以前假 NAS 一个请求都没收到（凭据没发出去）")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))

	in["trust_sha256"] = strings.ToUpper(f.pin())
	w = e.do(http.MethodPost, "/api/nas/sources/test", in)
	require.Equal(t, http.StatusOK, w.Code, "证书那一次没发出凭据，不算进 10 秒一次："+w.Body.String())
	v = nasDecode[nasTestView](t, w)
	assert.Equal(t, nasTestOK, v.Status)
	require.NotNil(t, v.System)
	assert.Equal(t, nas.System{Hostname: "fake-nas", Platform: "FakeOS", Version: "1.0", Model: "FN-1"}, *v.System)
	assert.Empty(t, v.Pending, "没有新的长期凭据")
	reqs := f.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, fakeNASReq{Path: "/api/info", User: "admin", Secret: "s3cret"}, reqs[0])

	got := e.save(t, 0, map[string]any{
		"name": " 家里的 NAS ", "kind": fakeNASKind, "enabled": true, "url": f.url() + "/", "username": "admin",
		"secret": "s3cret", "tls_pin_sha256": f.pin(),
	})
	assert.Equal(t, nasSourceEdit{
		ID: got.ID, Name: "家里的 NAS", Kind: fakeNASKind, Enabled: true, URL: f.url(), Username: "admin",
		HasSecret: true, TLSPinSHA256: f.pin(),
	}, got)
	row := e.row(t, got.ID)
	assert.Equal(t, int64(1), row.Revision)
	assert.Equal(t, "s3cret", e.decrypt(t, row.SecretEncrypted))
	assertNASCiphertextOnly(t, e.db, "s3cret")

	st := e.waitState(t, idOf(got.ID), nas.StateOK)
	require.NotNil(t, st.Snapshot)
	assert.Equal(t, "fake-nas", st.Snapshot.System.Hostname, "保存以后采集器马上连上了这个来源")

	w = e.do(http.MethodGet, "/api/nas", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "s3cret")
	view := nasDecode[nasView](t, w)
	require.Len(t, view.Sources, 2)
	assert.Equal(t, nas.LocalID, view.Sources[0].ID, "本机在最前")
	assert.Equal(t, "本机", view.Sources[0].Name)
	src := view.Sources[1]
	assert.Equal(t, idOf(got.ID), src.ID)
	assert.True(t, src.HasSecret)
	assert.Equal(t, f.pin(), src.TLSPinSHA256)
	assert.Equal(t, nas.StateOK, src.State)
	assert.Len(t, src.History, 1)
	assert.Equal(t, idOf(got.ID), view.Home, "没选首页来源时显示第一个启用的来源")
	assert.Empty(t, view.HomeSource)
	assert.True(t, view.LocalEnabled)
	assert.Equal(t, nas.LocalSupported, view.LocalSupported)
	assert.Equal(t, []nasKindView{
		{Name: fakeNASKind, Label: "假 NAS", Username: "optional", Secret: "with_username", Plain: "without_credentials"},
		{Name: nas.KindFnOS, Label: "飞牛", Experimental: true, Verified: "fnOS 1.2.1000", Username: "required", Secret: "required", Plain: "with_ack"},
		{Name: nas.KindGlances, Label: "Glances", Username: "optional", Secret: "with_username", Plain: "without_credentials"},
		{Name: nas.KindSynology, Label: "群晖", Experimental: true, Username: "required", Secret: "required", Plain: "without_credentials"},
		{Name: nas.KindTrueNAS, Label: "TrueNAS", Username: "none", Secret: "required", Plain: "never"},
	}, view.Kinds, "按名字排：测试登记的假类型、飞牛、Glances、群晖与 TrueNAS")
	var kindsRaw struct {
		Kinds []map[string]any `json:"kinds"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &kindsRaw))
	for _, k := range kindsRaw.Kinds {
		if k["name"] == nas.KindFnOS {
			assert.Equal(t, "fnOS 1.2.1000", k["verified"], "飞牛带上在哪个版本的真机上验证过")
		} else {
			assert.NotContains(t, k, "verified", "没在真机上验证过的类型不带 verified：%v", k["name"])
		}
	}

	w = e.do(http.MethodGet, "/api/nas/sources/"+idOf(got.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	for _, k := range []string{"secret", "secret_encrypted", "extra", "extra_encrypted"} {
		assert.NotContains(t, raw, k, "编辑用的字段不回密钥")
	}
	assert.Equal(t, got, nasDecode[nasSourceEdit](t, w))
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/api/nas/sources/99", nil).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodGet, "/api/nas/sources/x", nil).Code)
}

// 验证码与暂存凭据：pending 只给绑定的那组目标用，10 分钟过期，只能用一次；明文设备令牌不出现在回应与库里。
func TestNASAPI_OTPPendingAndDeviceToken(t *testing.T) {
	e := setupNASEnv(t)
	f := newFakeNAS(t, "otp-pass-9", true)
	g := newFakeNAS(t, "otp-pass-9", true)
	test := map[string]any{"kind": fakeNASKind, "url": f.url(), "username": "admin", "secret": "otp-pass-9", "trust_sha256": f.pin()}
	v, _ := e.test(t, test)
	assert.Equal(t, nasTestOTP, v.Status)
	assert.Contains(t, v.Message, "要验证码")
	assert.Empty(t, v.Pending)

	test["otp"] = " 123456 "
	v, w := e.test(t, test)
	assert.Equal(t, nasTestOK, v.Status)
	require.NotEmpty(t, v.Pending)
	assert.NotContains(t, w.Body.String(), "dev-1", "设备令牌不回明文")
	assert.Equal(t, "123456", f.requests()[1].OTP)

	save := map[string]any{
		"name": "群晖", "kind": fakeNASKind, "enabled": true, "url": g.url(), "username": "admin", "secret": "otp-pass-9",
		"tls_pin_sha256": g.pin(), "pending": v.Pending,
	}
	bad := e.saveRaw(0, save)
	assert.Equal(t, http.StatusBadRequest, bad.Code, "pending 是给 f 的，存到 g 不行")
	assert.Contains(t, bad.Body.String(), nas.ErrPendingInvalid.Error())
	save["username"] = "root"
	save["url"], save["tls_pin_sha256"] = f.url(), f.pin()
	assert.Equal(t, http.StatusBadRequest, e.saveRaw(0, save).Code, "换了账号也不行")

	save["username"] = "admin"
	got := e.save(t, 0, save)
	assert.True(t, got.HasExtra)
	e.waitState(t, idOf(got.ID), nas.StateOK)
	row := e.row(t, got.ID)
	assert.Equal(t, `{"did":"dev-1"}`, e.decrypt(t, row.ExtraEncrypted))
	assertNASCiphertextOnly(t, e.db, "dev-1", "otp-pass-9")
	save["name"] = "再存一次"
	assert.Equal(t, http.StatusBadRequest, e.saveRaw(0, save).Code, "pending 只能用一次")
	var n int64
	require.NoError(t, e.db.Model(&models.NasSource{}).Count(&n).Error)
	assert.Equal(t, int64(1), n)

	// 编辑时测试：没填的密钥、设备令牌与证书指纹用库里的，不用再输验证码
	v, _ = e.test(t, map[string]any{"id": got.ID, "kind": fakeNASKind, "url": f.url(), "username": "admin"})
	assert.Equal(t, nasTestOK, v.Status)
	reqs := f.requests()
	assert.Equal(t, fakeNASReq{Path: "/api/info", User: "admin", Secret: "otp-pass-9", Device: "dev-1"}, reqs[len(reqs)-1])

	// 过期：拿到以后 10 分钟再保存
	v, _ = e.test(t, map[string]any{"kind": fakeNASKind, "url": g.url(), "username": "admin", "secret": "otp-pass-9", "trust_sha256": g.pin(), "otp": "123456"})
	require.NotEmpty(t, v.Pending)
	e.now = e.now.Add(nas.PendingTTL)
	w = e.saveRaw(0, map[string]any{
		"name": "过期", "kind": fakeNASKind, "enabled": true, "url": g.url(), "username": "admin", "secret": "otp-pass-9",
		"tls_pin_sha256": g.pin(), "pending": v.Pending,
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "请再测一次")

	// 删掉来源时丢掉绑定它的暂存凭据
	v, _ = e.test(t, map[string]any{"id": got.ID, "kind": fakeNASKind, "url": g.url(), "username": "admin", "secret": "otp-pass-9", "trust_sha256": g.pin(), "otp": "123456"})
	require.NotEmpty(t, v.Pending)
	before := e.srv.nasPending.Len()
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, "/api/nas/sources/"+idOf(got.ID), nil).Code)
	assert.Equal(t, before-1, e.srv.nasPending.Len())
}

// 改了地址或账号：测试与保存都不带库里的密钥、设备令牌与钉住的证书，没填新密钥时回 400、不连新目标。
func TestNASAPI_ChangedTargetNeverGetsStoredCredentials(t *testing.T) {
	e := setupNASEnv(t)
	a := newFakeNAS(t, "pw-a", true)
	b := newFakeNAS(t, "", false)
	v, _ := e.test(t, map[string]any{"kind": fakeNASKind, "url": a.url(), "username": "admin", "secret": "pw-a", "trust_sha256": a.pin(), "otp": "123456"})
	require.NotEmpty(t, v.Pending)
	src := e.save(t, 0, map[string]any{
		"name": "A", "kind": fakeNASKind, "enabled": true, "url": a.url(), "username": "admin", "secret": "pw-a",
		"tls_pin_sha256": a.pin(), "pending": v.Pending,
	})
	e.waitState(t, idOf(src.ID), nas.StateOK)
	aHits := len(a.requests())

	for _, in := range []map[string]any{
		{"id": src.ID, "kind": fakeNASKind, "url": b.url(), "username": "admin", "trust_sha256": b.pin()},
		{"id": src.ID, "kind": fakeNASKind, "url": a.url(), "username": "root"},
	} {
		w := e.do(http.MethodPost, "/api/nas/sources/test", in)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "改了地址或账号，要重新填密钥")
	}
	assert.Empty(t, b.requests(), "没填新密钥时不连新目标")
	assert.Len(t, a.requests(), aHits, "换了账号也不拿旧密钥去试")

	v, _ = e.test(t, map[string]any{"id": src.ID, "kind": fakeNASKind, "url": b.url(), "username": "admin", "secret": "pw-b"})
	assert.Equal(t, nasTestCertificate, v.Status, "a 钉住的证书不给 b 用")
	assert.Empty(t, b.requests())
	v, _ = e.test(t, map[string]any{"id": src.ID, "kind": fakeNASKind, "url": b.url(), "username": "admin", "secret": "pw-b", "trust_sha256": b.pin()})
	assert.Equal(t, nasTestOK, v.Status)
	require.Len(t, b.requests(), 1)
	assert.Equal(t, fakeNASReq{Path: "/api/info", User: "admin", Secret: "pw-b"}, b.requests()[0], "b 只收到新密钥，没有 a 的设备令牌")

	edit := map[string]any{"name": "A", "kind": fakeNASKind, "enabled": true, "url": b.url(), "username": "admin", "tls_pin_sha256": b.pin()}
	w := e.saveRaw(src.ID, edit)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "改了地址或账号，要重新填密钥")
	edit["enabled"] = false
	assert.Equal(t, http.StatusBadRequest, e.saveRaw(src.ID, edit).Code, "停用时也不把旧密钥挪到新地址")
	assert.Equal(t, int64(1), e.row(t, src.ID).Revision, "保存失败时库里不变")

	edit["enabled"], edit["secret"] = true, "pw-b"
	got := e.save(t, src.ID, edit)
	assert.True(t, got.HasSecret)
	assert.False(t, got.HasExtra, "设备令牌属于原来的目标，清掉")
	row := e.row(t, src.ID)
	assert.Equal(t, int64(2), row.Revision)
	assert.Equal(t, "pw-b", e.decrypt(t, row.SecretEncrypted))
	assert.Empty(t, row.ExtraEncrypted)

	require.Eventually(t, func() bool { return len(b.requests()) >= 2 }, 5*time.Second, 5*time.Millisecond)
	e.waitState(t, idOf(src.ID), nas.StateOK)
	for _, r := range b.requests() {
		assert.Empty(t, r.Device)
		assert.Equal(t, "pw-b", r.Secret)
	}
	assert.Len(t, a.requests(), aHits, "采集器不再连 a")
}

// 清除密钥：测试不带库里的任何凭据；保存时清掉密钥与设备令牌。改成不要密钥的账号时库里的密钥也清掉。
func TestNASAPI_ClearSecret(t *testing.T) {
	e := setupNASEnv(t)
	f := newFakeNAS(t, "", true)
	v, _ := e.test(t, map[string]any{"kind": fakeNASKind, "url": f.url(), "secret": "tok-123", "trust_sha256": f.pin(), "otp": "123456"})
	require.NotEmpty(t, v.Pending)
	src := e.save(t, 0, map[string]any{
		"name": "令牌", "kind": fakeNASKind, "enabled": true, "url": f.url(), "secret": "tok-123",
		"tls_pin_sha256": f.pin(), "pending": v.Pending,
	})
	require.True(t, src.HasSecret)
	require.True(t, src.HasExtra)
	e.waitState(t, idOf(src.ID), nas.StateOK) // 采集器保存以后的那一次请求先到，下面按请求数比较

	v, _ = e.test(t, map[string]any{"id": src.ID, "kind": fakeNASKind, "url": f.url(), "clear_secret": true})
	assert.Equal(t, nasTestCertificate, v.Status, "清除时连钉住的证书也不用")
	n := len(f.requests())
	v, _ = e.test(t, map[string]any{"id": src.ID, "kind": fakeNASKind, "url": f.url(), "clear_secret": true, "trust_sha256": f.pin()})
	assert.Equal(t, nasTestOTP, v.Status)
	reqs := f.requests()
	require.Len(t, reqs, n+1)
	assert.Equal(t, fakeNASReq{Path: "/api/info"}, reqs[n], "清除以后测试不带库里的密钥与设备令牌")

	w := e.do(http.MethodPost, "/api/nas/sources/test", map[string]any{"id": src.ID, "kind": fakeNASKind, "url": f.url(), "clear_secret": true, "secret": "x"})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	edit := map[string]any{"name": "令牌", "kind": fakeNASKind, "enabled": true, "url": f.url(), "tls_pin_sha256": f.pin(), "clear_secret": true}
	for _, extra := range []map[string]any{{"secret": "x"}, {"pending": "p"}} {
		in := map[string]any{}
		for k, val := range edit {
			in[k] = val
		}
		for k, val := range extra {
			in[k] = val
		}
		w = e.saveRaw(src.ID, in)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "清除密钥")
	}
	got := e.save(t, src.ID, edit)
	assert.False(t, got.HasSecret)
	assert.False(t, got.HasExtra)

	// 填了用户名的（要密钥）：清掉密钥还开着不行，存成停用可以
	pw := e.save(t, 0, map[string]any{"name": "账号", "kind": fakeNASKind, "enabled": false, "url": f.url(), "username": "admin", "secret": "pw-1"})
	acct := map[string]any{"name": "账号", "kind": fakeNASKind, "enabled": true, "url": f.url(), "username": "admin", "clear_secret": true}
	w = e.saveRaw(pw.ID, acct)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "要填密钥")
	acct["enabled"] = false
	assert.False(t, e.save(t, pw.ID, acct).HasSecret)

	// 改成不填用户名（这种来源不填用户名时不要密钥）：库里的密钥清掉
	pw = e.save(t, 0, map[string]any{"name": "改账号", "kind": fakeNASKind, "enabled": false, "url": f.url(), "username": "admin", "secret": "pw-2"})
	got = e.save(t, pw.ID, map[string]any{"name": "改账号", "kind": fakeNASKind, "enabled": true, "url": f.url()})
	assert.False(t, got.HasSecret)
	assert.Empty(t, e.row(t, pw.ID).SecretEncrypted)
}

// 同一个来源 10 秒只测一次；证书不受信任的那次、请求不对的那次不算。
func TestNASAPI_TestRateLimit(t *testing.T) {
	e := setupNASEnv(t)
	f := newFakeNAS(t, "", false)
	g := newFakeNAS(t, "", false)
	in := map[string]any{"kind": fakeNASKind, "url": f.url(), "trust_sha256": f.pin()}
	call := func(body map[string]any) *httptest.ResponseRecorder {
		return e.do(http.MethodPost, "/api/nas/sources/test", body)
	}
	require.Equal(t, http.StatusOK, call(in).Code)
	w := call(in)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "刚测过，过几秒再试")
	in["username"] = "admin"
	in["secret"] = "pw"
	assert.Equal(t, http.StatusOK, call(in).Code, "换了账号是另一个来源")
	e.now = e.now.Add(nasTestInterval)
	assert.Equal(t, http.StatusOK, call(in).Code, "10 秒以后可以再测")

	cert := map[string]any{"kind": fakeNASKind, "url": g.url()}
	for range 3 {
		w = call(cert)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, nasTestCertificate, nasDecode[nasTestView](t, w).Status, "证书那一次没发出凭据，不算")
	}
	badPin := map[string]any{"kind": fakeNASKind, "url": g.url(), "trust_sha256": "xyz"}
	assert.Equal(t, http.StatusBadRequest, call(badPin).Code)
	badPin["trust_sha256"] = g.pin()
	assert.Equal(t, http.StatusOK, call(badPin).Code, "校验失败的那次不算")

	src := e.save(t, 0, map[string]any{"name": "F", "kind": fakeNASKind, "enabled": false, "url": f.url(), "tls_pin_sha256": f.pin()})
	byID := map[string]any{"id": src.ID, "kind": fakeNASKind, "url": g.url(), "trust_sha256": g.pin()}
	require.Equal(t, http.StatusOK, call(byID).Code)
	assert.Equal(t, http.StatusTooManyRequests, call(byID).Code, "编辑中的来源按编号算，改了地址也是同一个来源")
}

// 认证失败以后停住（熔断），用户点「重新连接」时只试一次；原因里没有密钥。
func TestNASAPI_RetryAfterAuthFailure(t *testing.T) {
	e := setupNASEnv(t)
	f := newFakeNAS(t, "right-pw", false)
	src := e.save(t, 0, map[string]any{
		"name": "N", "kind": fakeNASKind, "enabled": true, "url": f.url(), "username": "admin", "secret": "wrong-pw", "tls_pin_sha256": f.pin(),
	})
	st := e.waitState(t, idOf(src.ID), nas.StateAuthFailed)
	assert.Contains(t, st.Message, "密码不对")
	assert.NotContains(t, st.Message, "wrong-pw", "来源的错误里带了密钥也遮掉")

	f.setSecret("wrong-pw") // 用户在 NAS 上把密码改成了 pt-tools 里存的那个
	w := e.do(http.MethodPost, "/api/nas/sources/"+idOf(src.ID)+"/retry", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got := nasDecode[nasSourceView](t, w)
	assert.Equal(t, nas.StateOK, got.State)
	require.NotNil(t, got.Snapshot)
	assert.Equal(t, "fake-nas", got.Snapshot.System.Hostname)
	assert.NotContains(t, w.Body.String(), "wrong-pw")

	w = e.do(http.MethodPost, "/api/nas/sources/local/retry", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, nas.LocalID, nasDecode[nasSourceView](t, w).ID)

	off := e.save(t, 0, map[string]any{"name": "停用", "kind": fakeNASKind, "enabled": false, "url": f.url()})
	w = e.do(http.MethodPost, "/api/nas/sources/"+idOf(off.ID)+"/retry", nil)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "没有在采集")
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPost, "/api/nas/sources/99/retry", nil).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/api/nas/sources/abc/retry", nil).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/api/nas/sources/0/retry", nil).Code)
}

// 设置：本机来源的开关与首页来源；首页的 nas 块跟着设置走，删掉首页正显示的来源时回到自动。
func TestNASAPI_SettingsAndHomeBlock(t *testing.T) {
	e := setupNASEnv(t)
	e.waitState(t, nas.LocalID, nas.StateOK)
	h, herr := e.homeNAS(t)
	require.Empty(t, herr)
	require.NotNil(t, h.Source)
	assert.Equal(t, nas.LocalID, h.Source.ID, "只有本机来源时首页显示本机")
	assert.Equal(t, "pt-tools-host", h.Source.Snapshot.System.Hostname)
	assert.Empty(t, h.Others)

	f := newFakeNAS(t, "", false)
	src := e.save(t, 0, map[string]any{"name": "NAS", "kind": fakeNASKind, "enabled": true, "url": f.url(), "tls_pin_sha256": f.pin()})
	e.waitState(t, idOf(src.ID), nas.StateOK)
	h, _ = e.homeNAS(t)
	require.NotNil(t, h.Source)
	assert.Equal(t, idOf(src.ID), h.Source.ID, "自动时显示第一个启用的来源")
	assert.Equal(t, []homeNASOther{{ID: nas.LocalID, Name: "本机", Kind: nas.LocalKind, State: nas.StateOK}}, h.Others)

	w := e.do(http.MethodPut, "/api/nas/settings", map[string]any{"home_source": " local "})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	view := nasDecode[nasView](t, w)
	assert.Equal(t, nas.LocalID, view.HomeSource)
	assert.Equal(t, nas.LocalID, view.Home)
	h, _ = e.homeNAS(t)
	assert.Equal(t, nas.LocalID, h.Source.ID)

	for _, bad := range []map[string]any{{"home_source": "99"}, {"home_source": "abc"}, {"home_source": "0"}, {"local": true}} {
		assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPut, "/api/nas/settings", bad).Code, "%v", bad)
	}
	w = e.do(http.MethodPut, "/api/nas/settings", map[string]any{"home_source": "0" + idOf(src.ID)})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, idOf(src.ID), nasDecode[nasView](t, w).HomeSource, "编号规范成十进制")

	w = e.do(http.MethodPut, "/api/nas/settings", map[string]any{"home_source": "local"})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(http.MethodPut, "/api/nas/settings", map[string]any{"local_enabled": false})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	view = nasDecode[nasView](t, w)
	assert.False(t, view.LocalEnabled)
	assert.Equal(t, nas.LocalID, view.HomeSource, "没给的项不改")
	assert.Equal(t, idOf(src.ID), view.Home, "选的本机关掉了，回到自动")
	e.waitState(t, nas.LocalID, nas.StateDisabled)
	h, _ = e.homeNAS(t)
	assert.Equal(t, idOf(src.ID), h.Source.ID)
	assert.Empty(t, h.Others, "停用的本机不列")

	require.Equal(t, http.StatusOK, e.do(http.MethodPut, "/api/nas/settings", map[string]any{"home_source": idOf(src.ID)}).Code)
	w = e.do(http.MethodDelete, "/api/nas/sources/"+idOf(src.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	set, err := models.GetNasSetting(e.db)
	require.NoError(t, err)
	assert.Empty(t, set.HomeSource, "删掉首页正显示的来源，回到自动")
	require.Eventually(t, func() bool { return len(e.col.List()) == 1 }, 5*time.Second, 5*time.Millisecond, "采集器停掉了它")
	h, herr = e.homeNAS(t)
	require.Empty(t, herr)
	assert.Nil(t, h.Source, "没有能显示的来源时是空的（首页显示「接入 NAS」）")
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodDelete, "/api/nas/sources/"+idOf(src.ID), nil).Code)

	w = e.do(http.MethodPut, "/api/nas/settings", map[string]any{"local_enabled": true})
	require.Equal(t, http.StatusOK, w.Code)
	e.waitState(t, nas.LocalID, nas.StateOK)
}

func TestNASAPI_SaveValidation(t *testing.T) {
	e := setupNASEnv(t)
	base := func() map[string]any {
		return map[string]any{"name": "N", "kind": fakeNASKind, "enabled": true, "url": "https://127.0.0.1:9", "username": "admin", "secret": "pw"}
	}
	cases := []struct {
		name string
		set  map[string]any
		want string
	}{
		{"类型不认得", map[string]any{"kind": "nope"}, "不支持的来源类型"},
		{"本机不是能存的类型", map[string]any{"kind": "local"}, "不支持的来源类型"},
		{"没填地址", map[string]any{"url": " "}, "要填地址"},
		{"带路径", map[string]any{"url": "https://127.0.0.1:9/api"}, "地址只要协议、主机和端口"},
		{"带账号密码", map[string]any{"url": "https://u:p@127.0.0.1:9"}, "地址只要协议、主机和端口"},
		{"带查询", map[string]any{"url": "https://127.0.0.1:9?x=1"}, "地址只要协议、主机和端口"},
		{"别的协议", map[string]any{"url": "ftp://127.0.0.1"}, "地址要以 https:// 或 http:// 开头"},
		{"明文带凭据没确认", map[string]any{"url": "http://127.0.0.1:9"}, "勾选"},
		{"没填名字", map[string]any{"name": "  "}, "要填名字"},
		{"名字太长", map[string]any{"name": strings.Repeat("名", 65)}, "名字最多 64 个字"},
		{"用户名太长", map[string]any{"username": strings.Repeat("u", 129)}, "用户名最多 128 个字"},
		{"指纹不对", map[string]any{"tls_pin_sha256": "xyz"}, "证书指纹要是 64 位十六进制"},
		{"清除又填新密钥", map[string]any{"clear_secret": true}, "清除密钥"},
		{"要密钥没填", map[string]any{"secret": ""}, "要填密钥"},
		{"密钥太长", map[string]any{"secret": strings.Repeat("x", nasMaxSecret+1)}, "密钥太长了"},
		{"不认得的 pending", map[string]any{"pending": "nope"}, nas.ErrPendingInvalid.Error()},
		{"多余的字段", map[string]any{"foo": 1}, "请求格式错误"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base()
			for k, v := range tc.set {
				in[k] = v
			}
			w := e.saveRaw(0, in)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), tc.want)
		})
	}
	var n int64
	require.NoError(t, e.db.Model(&models.NasSource{}).Count(&n).Error)
	require.Zero(t, n, "校验失败的一个都没存")

	// 明文：确认了风险可以存；https:// 不记确认；不带凭据的 http:// 不用确认（Glances 不开认证）
	in := base()
	in["enabled"], in["url"], in["insecure_ack"] = false, "http://127.0.0.1:9", true
	assert.True(t, e.save(t, 0, in).InsecureAck)
	in["url"] = "https://127.0.0.1:9"
	assert.False(t, e.save(t, 0, in).InsecureAck)
	assert.False(t, e.save(t, 0, map[string]any{"name": "开放", "kind": fakeNASKind, "enabled": false, "url": "http://127.0.0.1:9"}).HasSecret)
	got := e.save(t, 0, map[string]any{"name": "草稿", "kind": fakeNASKind, "enabled": false, "url": "https://127.0.0.1:9", "username": "admin"})
	assert.False(t, got.HasSecret, "存成停用时可以不填密钥")

	for i := 4; i < nasMaxSources; i++ {
		e.save(t, 0, map[string]any{"name": fmt.Sprintf("N%d", i), "kind": fakeNASKind, "enabled": false, "url": "https://127.0.0.1:9"})
	}
	w := e.saveRaw(0, map[string]any{"name": "多一个", "kind": fakeNASKind, "enabled": false, "url": "https://127.0.0.1:9"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "最多接入 20 个来源")
	assert.Equal(t, http.StatusNotFound, e.saveRaw(99, base()).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPut, "/api/nas/sources/x", base()).Code)
}

// 飞牛的 http:// 一律要确认明文风险（明文的 WebSocket 上，登录用的公钥可以被中间人换掉），没填密码的草稿也一样；
// 确认以后能存，没写端口时补上 5666
func TestNASAPI_FnOSPlain(t *testing.T) {
	e := setupNASEnv(t)
	draft := map[string]any{"name": "飞牛", "kind": nas.KindFnOS, "enabled": false, "url": "http://127.0.0.1", "username": "ptmon"}
	w := e.saveRaw(0, draft)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "勾选")
	w = e.do(http.MethodPost, "/api/nas/sources/test", map[string]any{"kind": nas.KindFnOS, "url": "http://127.0.0.1", "username": "ptmon", "secret": "pw"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "勾选")

	draft["insecure_ack"] = true
	got := e.save(t, 0, draft)
	assert.True(t, got.InsecureAck)
	assert.Equal(t, "http://127.0.0.1:5666", got.URL)
	got = e.save(t, 0, map[string]any{"name": "飞牛 https", "kind": nas.KindFnOS, "enabled": false, "url": "https://127.0.0.1:5667", "username": "ptmon"})
	assert.False(t, got.InsecureAck, "https:// 不用确认")
}

func TestNASAPI_TestValidation(t *testing.T) {
	e := setupNASEnv(t)
	src := e.save(t, 0, map[string]any{"name": "N", "kind": fakeNASKind, "enabled": false, "url": "https://127.0.0.1:9", "username": "admin", "secret": "pw"})
	require.NoError(t, e.db.Model(&models.NasSource{}).Where("id = ?", src.ID).Update("secret_encrypted", "不是密文").Error)
	cases := []struct {
		in   map[string]any
		code int
		want string
	}{
		{map[string]any{"kind": "nope", "url": "https://127.0.0.1:9"}, http.StatusBadRequest, "不支持的来源类型"},
		{map[string]any{"kind": fakeNASKind, "url": "https://127.0.0.1:9", "username": "admin"}, http.StatusBadRequest, "要填密钥"},
		{map[string]any{"kind": fakeNASKind, "url": "https://127.0.0.1:9", "secret": strings.Repeat("x", nasMaxSecret+1)}, http.StatusBadRequest, "密钥太长了"},
		{map[string]any{"kind": fakeNASKind, "url": "http://127.0.0.1:9", "username": "admin", "secret": "pw"}, http.StatusBadRequest, "勾选"},
		{map[string]any{"kind": fakeNASKind, "url": "http://127.0.0.1:9", "otp": "123456"}, http.StatusBadRequest, "勾选"},
		{map[string]any{"id": 99, "kind": fakeNASKind, "url": "https://127.0.0.1:9"}, http.StatusNotFound, "没有这个来源"},
		{map[string]any{"id": src.ID, "kind": fakeNASKind, "url": "https://127.0.0.1:9", "username": "admin"}, http.StatusBadRequest, "保存的密钥解不开"},
	}
	for _, tc := range cases {
		w := e.do(http.MethodPost, "/api/nas/sources/test", tc.in)
		assert.Equal(t, tc.code, w.Code, "%v", tc.in)
		assert.Contains(t, w.Body.String(), tc.want, "%v", tc.in)
	}
}

func TestNASAPI_NoCollector(t *testing.T) {
	srv := setupServer(t)
	mux := http.NewServeMux()
	srv.registerNASRoutes(mux)
	srv.sessions.put("sess-test", "admin")
	for _, r := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/nas", ""},
		{http.MethodPost, "/api/nas/sources", "{}"},
		{http.MethodPost, "/api/nas/sources/test", "{}"},
		{http.MethodGet, "/api/nas/sources/1", ""},
		{http.MethodPut, "/api/nas/sources/1", "{}"},
		{http.MethodDelete, "/api/nas/sources/1", ""},
		{http.MethodPost, "/api/nas/sources/local/retry", ""},
		{http.MethodPut, "/api/nas/settings", "{}"},
	} {
		w := serveAuthed(mux, r.method, r.path, r.body)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code, "%s %s", r.method, r.path)
		assert.Contains(t, w.Body.String(), "NAS 监控没有启动")
	}
	_, err := srv.homeNASBlock(context.Background(), global.GlobalDB.DB)
	assert.EqualError(t, err, "NAS 监控没有启动")
}

func TestNASAPI_ExcludedFromConfigGateway(t *testing.T) {
	found := false
	for _, x := range configExcluded {
		if strings.Contains(x, "NAS 监控") {
			found = true
		}
	}
	assert.True(t, found, "App API 与 MCP 的配置网关读改不到 NAS 监控的来源与密钥")
}

func TestNASHome(t *testing.T) {
	local := nas.Status{ID: nas.LocalID, Enabled: true, State: nas.StateOK}
	a := nas.Status{ID: "1", Enabled: true, State: nas.StateError}
	b := nas.Status{ID: "2", Enabled: true, State: nas.StateOK}
	off := nas.Status{ID: "3", Enabled: false, State: nas.StateDisabled}
	gone := nas.Status{ID: "4", Enabled: true, State: nas.StateUnsupported}
	for _, tc := range []struct {
		setting string
		list    []nas.Status
		want    string
	}{
		{"", []nas.Status{local, a, b}, "1"},
		{"2", []nas.Status{local, a, b}, "2"},
		{"local", []nas.Status{local, a, b}, nas.LocalID},
		{"3", []nas.Status{local, off, b}, "2"},
		{"", []nas.Status{local, off, gone}, nas.LocalID},
		{"", []nas.Status{{ID: nas.LocalID, Enabled: true, State: nas.StateUnsupported}, off}, ""},
		{"local", []nas.Status{{ID: nas.LocalID, Enabled: false, State: nas.StateDisabled}}, ""},
		{"", nil, ""},
	} {
		assert.Equal(t, tc.want, nasHome(tc.setting, tc.list), "%q %v", tc.setting, tc.list)
	}
}

func TestNASKindRules(t *testing.T) {
	assert.Equal(t, "required", nasSecretRule(nas.Kind{}))
	assert.Equal(t, "with_username", nasSecretRule(nas.Kind{NeedsSecret: func(u string) bool { return u != "" }}))
	assert.Equal(t, "none", nasSecretRule(nas.Kind{NeedsSecret: func(string) bool { return false }}))
	assert.Equal(t, "required", nasSecretRule(nas.Kind{NeedsSecret: func(u string) bool { return u == "" }}))
	assert.Equal(t, "never", nasPlainRule(nas.PlainNever))
	assert.Equal(t, "without_credentials", nasPlainRule(nas.PlainWithoutCredentials))
	assert.Equal(t, "with_ack", nasPlainRule(nas.PlainWithAck))
}

func TestNASTestResultView(t *testing.T) {
	assert.Equal(t, nasTestView{Status: nasTestError, Message: "来源没有返回结果"}, nasTestResultView(nil, nil))
	assert.Equal(t, nasTestView{Status: nasTestError, Message: "20 秒内没有回应"},
		nasTestResultView(nil, fmt.Errorf("连接: %w", context.DeadlineExceeded)))
	v := nasTestResultView(nil, fmt.Errorf("登录 hunter22 失败 password=abc"), "hunter22", "abc")
	assert.Equal(t, nasTestView{Status: nasTestError, Message: "登录 *** 失败 password=***"}, v)
	v = nasTestResultView(nil, &nas.AuthError{Reason: "验证码 654321 过期了", OTP: true}, "pw", "654321")
	assert.Equal(t, nasTestView{Status: nasTestOTP, Message: "验证码 *** 过期了"}, v)
	v = nasTestResultView(nil, &nas.AuthError{OTP: true})
	assert.Equal(t, nasTestView{Status: nasTestOTP, Message: nas.ErrAuth.Error()}, v, "没有原因时写认证失败")
	v = nasTestResultView(nil, &nas.CertError{Changed: true, Cert: nas.CertInfo{SHA256: "ab"}})
	assert.Equal(t, nasTestCertificate, v.Status)
	assert.Equal(t, nas.ErrCertChanged.Error(), v.Message)
	assert.Equal(t, "ab", v.Certificate.SHA256)
	assert.Equal(t, "a pw b", nasScrub("a pw b", "pw"), "短于 4 个字的不换")
}

func TestNASTestLimiter(t *testing.T) {
	now := time.Now()
	l := &nasTestLimiter{now: func() time.Time { return now }}
	assert.True(t, l.allow("a"))
	assert.False(t, l.allow("a"))
	assert.True(t, l.allow("b"))
	l.release("a")
	assert.True(t, l.allow("a"))
	now = now.Add(nasTestInterval)
	assert.True(t, l.allow("a"))
	assert.Len(t, l.last, 1, "过期的记录清掉")
	assert.False(t, (&nasTestLimiter{}).clock().IsZero())
}

// 体检的「NAS」一组读采集器内存里的状态：连上的来源一项正常，体检本身不连 NAS；没有采集器时写「没有启动」。
func TestSystemCheck_NASGroup(t *testing.T) {
	e := setupNASEnv(t)
	f := newFakeNAS(t, "s3cret", false)
	in := map[string]any{"kind": fakeNASKind, "url": f.url(), "username": "admin", "secret": "s3cret", "trust_sha256": f.pin()}
	v, _ := e.test(t, in)
	require.Equal(t, nasTestOK, v.Status)
	got := e.save(t, 0, map[string]any{
		"name": "家里的 NAS", "kind": fakeNASKind, "enabled": true, "url": f.url(), "username": "admin",
		"secret": "s3cret", "tls_pin_sha256": f.pin(),
	})
	e.waitState(t, idOf(got.ID), nas.StateOK)
	before := len(f.requests())

	items := runNASCheck(t, e.srv)
	require.Len(t, items, 1, "本机来源不算")
	assert.Equal(t, "nas:"+idOf(got.ID), items[0].Key)
	assert.Equal(t, "家里的 NAS", items[0].Title)
	assert.Equal(t, health.StatusOK, items[0].Status, "%+v", items[0])
	assert.True(t, strings.HasPrefix(items[0].Detail, "FakeOS 1.0（fake-nas） · "), items[0].Detail)
	assert.Len(t, f.requests(), before, "体检不连 NAS")

	e.srv.SetNASCollector(nil)
	assert.Equal(t, []health.Item{{Key: "nas", Group: health.GroupNAS, Title: "NAS", Status: health.StatusSkip, Detail: "NAS 监控没有启动"}}, runNASCheck(t, e.srv))
}

// runNASCheck 只跑体检里的「NAS」一组。
func runNASCheck(t *testing.T, srv *Server) []health.Item {
	t.Helper()
	for _, c := range srv.healthChecks(global.GlobalDB.DB) {
		if c.Group == health.GroupNAS {
			return c.Run(context.Background())
		}
	}
	require.FailNow(t, "体检里没有 NAS 一组")
	return nil
}
