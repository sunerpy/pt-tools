package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/feishuapp"
)

// 飞书应用机器人「扫码一键创建」的接口（计划 pt-tools-feishu-register 第 3.2、6 节）：
// 开始、按编号查状态与取消；扫码确认以后建通道、把扫码人直接绑定并记审计；App Secret 不出现在任何回应里。

const (
	fakeRegURL    = "https://open.feishu.cn/page/launcher?user_code=UC-ONCE-ONLY&from=sdk"
	fakeRegSecret = "scan-secret-value-0001"
)

func scannedApp() *feishuapp.RegisteredApp {
	return &feishuapp.RegisteredApp{AppID: "cli_scan_app", AppSecret: fakeRegSecret, OpenID: "ou_scanner", Brand: feishuapp.DomainFeishu}
}

// fakeRegCall 是假的一键创建的一次调用：测试从 fakeFeishuRun.calls 拿到它，决定结果。
type fakeRegCall struct {
	n      int
	ctx    context.Context
	result chan fakeRegResult
}

type fakeRegResult struct {
	app *feishuapp.RegisteredApp
	err error
}

func (c *fakeRegCall) finish(a *feishuapp.RegisteredApp, err error) {
	c.result <- fakeRegResult{app: a, err: err}
}

// fakeFeishuRun 是假的 feishuapp.RegisterApp：默认马上给出链接，然后等测试送来结果或 ctx 取消。
type fakeFeishuRun struct {
	calls chan *fakeRegCall

	mu        sync.Mutex
	n         int
	url       string
	early     error            // 给链接以前就失败
	quick     error            // 给了链接（两次）以后马上就这样结束
	skipQR    func(n int) bool // 第 n 次调用不给链接
	ignoreCtx bool             // 不理 ctx（模拟 SDK 拿到凭据的那一刻正好被取消）
}

func newFakeFeishuRun() *fakeFeishuRun {
	return &fakeFeishuRun{calls: make(chan *fakeRegCall, 16), url: fakeRegURL}
}

func (f *fakeFeishuRun) run(ctx context.Context, onQR func(string, int)) (*feishuapp.RegisteredApp, error) {
	f.mu.Lock()
	f.n++
	c := &fakeRegCall{n: f.n, ctx: ctx, result: make(chan fakeRegResult, 1)}
	early, quick, skip, ignore, link := f.early, f.quick, f.skipQR != nil && f.skipQR(f.n), f.ignoreCtx, f.url
	f.mu.Unlock()
	f.calls <- c
	if early != nil {
		return nil, early
	}
	if quick != nil {
		onQR(link, 600)
		onQR(link+"&again=1", 600)
		return nil, quick
	}
	if !skip {
		onQR(link, 600)
	}
	if ignore {
		r := <-c.result
		return r.app, r.err
	}
	select {
	case r := <-c.result:
		return r.app, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *fakeFeishuRun) next(t *testing.T) *fakeRegCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(3 * time.Second):
		t.Fatal("一键创建没有被调用")
		return nil
	}
}

type fakeBind struct {
	confID uint
	openID string
}

// fakeRegStore 记下建通道、绑定与审计；gate 不为空时建通道要等它关上（测保存期间的取消与取代）。
type fakeRegStore struct {
	mu         sync.Mutex
	creates    []app.CreateConfReq
	createCtx  []error // 建通道被调用时 ctx.Err()
	createErr  error
	gate       chan struct{}
	entered    chan struct{} // 建通道被调用时送一个
	nextID     uint
	binds      []fakeBind
	bindErr    error
	audits     []app.AuditEntry
	auditErr   error
	bindCalled int
}

func newFakeRegStore() *fakeRegStore { return &fakeRegStore{entered: make(chan struct{}, 8)} }

func (s *fakeRegStore) create(ctx context.Context, req app.CreateConfReq) (app.NotificationConfDTO, error) {
	s.mu.Lock()
	gate := s.gate
	s.mu.Unlock()
	s.entered <- struct{}{}
	if gate != nil {
		<-gate
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	req.ConfigJSON = append(json.RawMessage(nil), req.ConfigJSON...)
	s.creates = append(s.creates, req)
	s.createCtx = append(s.createCtx, ctx.Err())
	if s.createErr != nil {
		return app.NotificationConfDTO{}, s.createErr
	}
	s.nextID++
	return app.NotificationConfDTO{ID: 40 + s.nextID, ChannelType: req.ChannelType, Name: req.Name, Enabled: req.Enabled}, nil
}

func (s *fakeRegStore) bind(_ context.Context, confID uint, openID string) (app.BindingDTO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bindCalled++
	s.binds = append(s.binds, fakeBind{confID: confID, openID: openID})
	if s.bindErr != nil {
		return app.BindingDTO{ConfID: confID, ChannelType: feishuapp.ChannelType, ChannelUserID: openID}, s.bindErr
	}
	return app.BindingDTO{ID: 9, ConfID: confID, ChannelType: feishuapp.ChannelType, ChannelUserID: openID, PtAdmin: true, Allowed: true}, nil
}

func (s *fakeRegStore) audit(_ context.Context, e app.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audits = append(s.audits, e)
	return s.auditErr
}

func (s *fakeRegStore) snapshot() (creates []app.CreateConfReq, ctxErrs []error, binds []fakeBind, audits []app.AuditEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]app.CreateConfReq(nil), s.creates...), append([]error(nil), s.createCtx...),
		append([]fakeBind(nil), s.binds...), append([]app.AuditEntry(nil), s.audits...)
}

type regEnv struct {
	r   *feishuRegistrar
	run *fakeFeishuRun
	st  *fakeRegStore
	h   http.Handler
}

// newRegEnv 起一个连假一键创建、假通道服务的接口（不经鉴权）。结束时取消还在等的、等后台协程都退出。
func newRegEnv(t *testing.T) *regEnv {
	t.Helper()
	e := &regEnv{run: newFakeFeishuRun(), st: newFakeRegStore()}
	e.r = newFeishuRegistrar(&ChatOpsDeps{})
	e.r.run, e.r.create, e.r.bind, e.r.audit = e.run.run, e.st.create, e.st.bind, e.st.audit
	e.r.linkWait = 2 * time.Second
	mux := http.NewServeMux()
	e.r.register(mux, func(f http.HandlerFunc) http.Handler { return f })
	e.h = mux
	t.Cleanup(func() {
		e.r.mu.Lock()
		if j := e.r.cur; j != nil && j.state == feishuRegPending {
			j.cancel()
		}
		e.r.mu.Unlock()
		done := make(chan struct{})
		go func() {
			e.r.jobs.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("一键创建的后台协程没有退出")
		}
	})
	return e
}

func (e *regEnv) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rd = bytes.NewReader(b)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, httptest.NewRequest(method, path, rd))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"), "%s %s 要带 no-store", method, path)
	assert.NotContains(t, w.Body.String(), fakeRegSecret, "%s %s 的回应里有 App Secret", method, path)
	return w
}

func (e *regEnv) start(t *testing.T, name string) feishuRegisterStart {
	t.Helper()
	w := e.do(t, http.MethodPost, "/api/chatops/feishu-app/register", map[string]string{"name": name})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out feishuRegisterStart
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out
}

func (e *regEnv) status(t *testing.T, id string) (int, map[string]any) {
	t.Helper()
	w := e.do(t, http.MethodGet, "/api/chatops/feishu-app/register/"+id, nil)
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
	return w.Code, out
}

func (e *regEnv) cancel(t *testing.T, id string) (int, map[string]any) {
	t.Helper()
	w := e.do(t, http.MethodPost, "/api/chatops/feishu-app/register/"+id+"/cancel", nil)
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
	return w.Code, out
}

// waitState 按编号查状态，等到 want（最多 3 秒），返回最后一次的回应。
func (e *regEnv) waitState(t *testing.T, id, want string) map[string]any {
	t.Helper()
	var last map[string]any
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		code, out := e.status(t, id)
		require.Equal(t, http.StatusOK, code, out)
		last = out
		if out["state"] == want {
			return out
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("状态没有变成 %s：%v", want, last)
	return nil
}

func waitEntered(t *testing.T, st *fakeRegStore) {
	t.Helper()
	select {
	case <-st.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("没有开始建通道")
	}
}

func waitCtxDone(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("ctx 没有被取消")
	}
}

// 开始：回链接与二维码（只在这里给一次）；按编号查到 pending → saving → done；建通道的配置里管理员是扫码人、
// 域名按品牌、启用；扫码人直接绑定，记一条 bind 审计（via=feishu_register）。
func TestFeishuRegister_StartToDone(t *testing.T) {
	e := newRegEnv(t)
	e.st.gate = make(chan struct{})
	before := time.Now()
	s := e.start(t, "  团队飞书  ")
	assert.Regexp(t, `^[0-9a-f]{32}$`, s.ID)
	assert.Equal(t, fakeRegURL, s.URL)
	assert.True(t, strings.HasPrefix(s.QRSVG, "<svg"), s.QRSVG)
	assert.Equal(t, feishuRegPending, s.State)
	assert.WithinDuration(t, before.Add(600*time.Second), s.ExpiresAt, 5*time.Second)
	call := e.run.next(t)

	code, st := e.status(t, s.ID)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, feishuRegPending, st["state"])
	assert.Equal(t, s.ID, st["id"])
	assert.NotContains(t, st, "url", "链接只在开始接口的回应里给一次")
	assert.NotContains(t, st, "qr_svg")
	assert.NotEmpty(t, st["expires_at"])

	call.finish(scannedApp(), nil)
	waitEntered(t, e.st)
	st = e.waitState(t, s.ID, feishuRegSaving)
	assert.Equal(t, "cli_scan_app", st["app_id"])
	assert.NotContains(t, st, "url")
	close(e.st.gate)
	st = e.waitState(t, s.ID, feishuRegDone)
	assert.EqualValues(t, 41, st["conf_id"])
	assert.Equal(t, "cli_scan_app", st["app_id"])
	assert.NotContains(t, st, "message", "绑定成功时没有提示")
	assert.NotContains(t, st, "qr_svg")

	creates, ctxErrs, binds, audits := e.st.snapshot()
	require.Len(t, creates, 1)
	assert.Equal(t, "feishu_app", creates[0].ChannelType)
	assert.Equal(t, "团队飞书", creates[0].Name)
	assert.True(t, creates[0].Enabled)
	assert.JSONEq(t, `{"app_id":"cli_scan_app","app_secret":"`+fakeRegSecret+`","domain":"feishu",
		"admin_users":["ou_scanner"],"allowed_users":[],"default_chat_id":"","proxy_url":""}`, string(creates[0].ConfigJSON))
	assert.NoError(t, ctxErrs[0])
	assert.Equal(t, []fakeBind{{confID: 41, openID: "ou_scanner"}}, binds)
	assert.Equal(t, []app.AuditEntry{{
		NotificationConfID: 41, ChannelType: "feishu_app", ChannelUserID: "ou_scanner",
		Command: "bind", Args: map[string]any{"via": "feishu_register"}, Result: "success",
	}}, audits)
}

// 名称：可空（默认「飞书应用机器人」）、最长 64 个字；请求体可以不带；不认识的字段拒绝。
func TestFeishuRegister_StartName(t *testing.T) {
	e := newRegEnv(t)
	w := e.do(t, http.MethodPost, "/api/chatops/feishu-app/register", map[string]string{"name": strings.Repeat("飞", 65)})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "通道名称最长 64 个字")
	w = e.do(t, http.MethodPost, "/api/chatops/feishu-app/register", map[string]any{"name": "x", "app_secret": "y"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Zero(t, len(e.run.calls), "参数不对时不开始")

	for _, body := range []any{nil, map[string]string{"name": "   "}, map[string]string{"name": strings.Repeat("飞", 64)}} {
		w = e.do(t, http.MethodPost, "/api/chatops/feishu-app/register", body)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var s feishuRegisterStart
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &s))
		e.run.next(t).finish(scannedApp(), nil)
		e.waitState(t, s.ID, feishuRegDone)
	}
	creates, _, _, _ := e.st.snapshot()
	require.Len(t, creates, 3)
	assert.Equal(t, "飞书应用机器人", creates[0].Name)
	assert.Equal(t, "飞书应用机器人", creates[1].Name)
	assert.Equal(t, strings.Repeat("飞", 64), creates[2].Name)
}

// 品牌与扫码人：lark 建成 Lark 国际版；open_id 不是 ou_ 开头（或没有）时不填管理员、不绑定、不记审计，
// 通道照建，提示写明要手动拿 open_id。
func TestFeishuRegister_BrandAndOpenID(t *testing.T) {
	cases := []struct {
		name, brand, openID, domain string
		admins                      string
	}{
		{"lark", feishuapp.DomainLark, "ou_lark", "lark", `["ou_lark"]`},
		{"空品牌按飞书", "", "ou_x", "feishu", `["ou_x"]`},
		{"没有 open_id", feishuapp.DomainFeishu, "", "feishu", `[]`},
		{"union_id 不是 open_id", feishuapp.DomainFeishu, "on_union", "feishu", `[]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newRegEnv(t)
			s := e.start(t, "")
			e.run.next(t).finish(&feishuapp.RegisteredApp{AppID: "cli_b", AppSecret: fakeRegSecret, OpenID: tc.openID, Brand: tc.brand}, nil)
			st := e.waitState(t, s.ID, feishuRegDone)
			creates, _, binds, audits := e.st.snapshot()
			require.Len(t, creates, 1)
			var cfg map[string]any
			require.NoError(t, json.Unmarshal(creates[0].ConfigJSON, &cfg))
			assert.Equal(t, tc.domain, cfg["domain"])
			admins, _ := json.Marshal(cfg["admin_users"])
			assert.JSONEq(t, tc.admins, string(admins))
			if tc.admins == `[]` {
				assert.Empty(t, binds)
				assert.Empty(t, audits)
				assert.Contains(t, st["message"], "open_id")
				assert.Contains(t, st["message"], "管理员 open_id")
			} else {
				assert.Len(t, binds, 1)
				assert.NotContains(t, st, "message")
			}
		})
	}
}

// 绑定的结果与审计：取值和网页「直接绑定」同一套；绑定失败时通道仍是 done，提示去绑定页手动绑定；审计写失败只记日志。
func TestFeishuRegister_BindOutcomes(t *testing.T) {
	cases := []struct {
		name     string
		bindErr  error
		auditErr error
		result   string
	}{
		{"成功", nil, nil, "success"},
		{"名单外", app.ErrBindNotInWhitelist, nil, "denied:not_in_whitelist"},
		{"已经绑定", app.ErrAlreadyBound, nil, "denied:already_bound"},
		{"失败", errors.New("database is locked"), nil, "error:bind_failed"},
		{"审计写失败", nil, errors.New("disk full"), "success"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newRegEnv(t)
			e.st.bindErr, e.st.auditErr = tc.bindErr, tc.auditErr
			s := e.start(t, "")
			e.run.next(t).finish(scannedApp(), nil)
			st := e.waitState(t, s.ID, feishuRegDone)
			_, _, binds, audits := e.st.snapshot()
			require.Len(t, binds, 1)
			require.Len(t, audits, 1)
			assert.Equal(t, tc.result, audits[0].Result)
			assert.Equal(t, "bind", audits[0].Command)
			assert.Equal(t, map[string]any{"via": "feishu_register"}, audits[0].Args)
			assert.EqualValues(t, 41, st["conf_id"])
			if tc.bindErr != nil {
				assert.Equal(t, "通道已经建好，绑定失败："+tc.bindErr.Error()+"，请在绑定页手动绑定", st["message"])
			} else {
				assert.NotContains(t, st, "message")
			}
		})
	}
}

// 建通道失败：终态 failed，提示写明应用已经建好（带 App ID）、要手动新建；不绑定；提示里没有 App Secret。
func TestFeishuRegister_CreateFails(t *testing.T) {
	e := newRegEnv(t)
	e.st.createErr = fmt.Errorf("%w: App Secret 不能为空", app.ErrInvalidConf)
	s := e.start(t, "")
	e.run.next(t).finish(scannedApp(), nil)
	st := e.waitState(t, s.ID, feishuRegFailed)
	assert.Equal(t, "飞书应用已经建好（App ID cli_scan_app），但保存通道失败：通道配置无效: App Secret 不能为空；请在开发者后台复制 App Secret 手动新建通道", st["message"])
	assert.Equal(t, "cli_scan_app", st["app_id"])
	assert.NotContains(t, st, "conf_id")
	_, _, binds, audits := e.st.snapshot()
	assert.Empty(t, binds)
	assert.Empty(t, audits)
}

// 拒绝、过期与其他失败：各自的终态与原因；失败时提示可以改用手动填写。
func TestFeishuRegister_Outcomes(t *testing.T) {
	cases := []struct {
		err   error
		state string
		msg   string
	}{
		{feishuapp.ErrRegisterDenied, feishuRegDenied, "你在飞书里取消了创建"},
		{feishuapp.ErrRegisterExpired, feishuRegExpired, "二维码过期了"},
		{
			errors.New("飞书一键创建失败: register app error: invalid_client: nope"), feishuRegFailed,
			"飞书一键创建失败: register app error: invalid_client: nope。可以重新生成二维码，或者改用「填写 App ID 与 App Secret」",
		},
	}
	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			e := newRegEnv(t)
			s := e.start(t, "")
			e.run.next(t).finish(nil, tc.err)
			st := e.waitState(t, s.ID, tc.state)
			assert.Equal(t, tc.msg, st["message"])
			creates, _, _, _ := e.st.snapshot()
			assert.Empty(t, creates)
			// 终态以后取消什么都不做
			code, out := e.cancel(t, s.ID)
			assert.Equal(t, http.StatusOK, code)
			assert.Equal(t, tc.state, out["state"])
		})
	}
}

// 取消：只在 pending 时起作用，调作业 ctx 的 cancel，状态 canceled；再取消一次照旧。
func TestFeishuRegister_CancelPending(t *testing.T) {
	e := newRegEnv(t)
	s := e.start(t, "")
	call := e.run.next(t)
	code, out := e.cancel(t, s.ID)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, feishuRegCanceled, out["state"])
	assert.NotContains(t, out, "url")
	waitCtxDone(t, call.ctx)
	code, out = e.cancel(t, s.ID)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, feishuRegCanceled, out["state"])
	st := e.waitState(t, s.ID, feishuRegCanceled)
	assert.NotEmpty(t, st["message"])
	creates, _, _, _ := e.st.snapshot()
	assert.Empty(t, creates)
}

// 再开始一个会取消上一个还在 pending 的；旧编号查与取消都回 404，不影响新的。
func TestFeishuRegister_SupersedePending(t *testing.T) {
	e := newRegEnv(t)
	a := e.start(t, "")
	callA := e.run.next(t)
	b := e.start(t, "")
	e.run.next(t)
	assert.NotEqual(t, a.ID, b.ID)
	waitCtxDone(t, callA.ctx)

	for _, do := range []func(*testing.T, string) (int, map[string]any){e.status, e.cancel} {
		code, out := do(t, a.ID)
		assert.Equal(t, http.StatusNotFound, code)
		assert.Equal(t, "这个一键创建已经被新的取代或过期", out["detail"])
	}
	code, out := e.status(t, b.ID)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, feishuRegPending, out["state"])
}

// 编号不对：查与取消都回 404，当前这一个照旧。
func TestFeishuRegister_WrongID(t *testing.T) {
	e := newRegEnv(t)
	code, out := e.status(t, "0123456789abcdef0123456789abcdef")
	assert.Equal(t, http.StatusNotFound, code, "还没有开始过")
	assert.Equal(t, "not_found", out["error"])
	s := e.start(t, "")
	e.run.next(t)
	for _, do := range []func(*testing.T, string) (int, map[string]any){e.status, e.cancel} {
		code, out = do(t, "0123456789abcdef0123456789abcdef")
		assert.Equal(t, http.StatusNotFound, code)
		assert.Equal(t, "这个一键创建已经被新的取代或过期", out["detail"])
	}
	_, out = e.status(t, s.ID)
	assert.Equal(t, feishuRegPending, out["state"])
}

// 飞书已经建好应用（saving）以后，取消与「被新的取代」都不影响它：保存用的 ctx 没被取消，通道照常建完、绑定、记审计。
func TestFeishuRegister_SavingSurvivesCancelAndSupersede(t *testing.T) {
	e := newRegEnv(t)
	e.st.gate = make(chan struct{})
	a := e.start(t, "")
	callA := e.run.next(t)
	callA.finish(scannedApp(), nil)
	waitEntered(t, e.st)
	e.waitState(t, a.ID, feishuRegSaving)

	code, out := e.cancel(t, a.ID)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, feishuRegSaving, out["state"], "saving 时取消不起作用")
	b := e.start(t, "")
	e.run.next(t)
	close(e.st.gate)

	require.Eventually(t, func() bool {
		_, _, binds, audits := e.st.snapshot()
		return len(binds) == 1 && len(audits) == 1
	}, 3*time.Second, 10*time.Millisecond)
	creates, ctxErrs, _, _ := e.st.snapshot()
	require.Len(t, creates, 1)
	assert.NoError(t, ctxErrs[0], "保存用的 ctx 不受作业 ctx 取消的影响")
	code, _ = e.status(t, a.ID)
	assert.Equal(t, http.StatusNotFound, code, "被新的取代以后，旧的不再是当前这一个")
	_, out = e.status(t, b.ID)
	assert.Equal(t, feishuRegPending, out["state"])
}

// 取消的那一刻飞书正好建好应用：凭据已经拿到，照常建完通道（丢掉的话飞书那边留下一个没人管的应用）。
func TestFeishuRegister_CredentialsAfterCancel(t *testing.T) {
	e := newRegEnv(t)
	e.run.ignoreCtx = true
	s := e.start(t, "")
	call := e.run.next(t)
	_, out := e.cancel(t, s.ID)
	assert.Equal(t, feishuRegCanceled, out["state"])
	call.finish(scannedApp(), nil)
	st := e.waitState(t, s.ID, feishuRegDone)
	assert.EqualValues(t, 41, st["conf_id"])
}

// 20 秒（测试里调小）内没拿到链接：取消作业、终态 failed，回 504。
func TestFeishuRegister_NoLinkInTime(t *testing.T) {
	e := newRegEnv(t)
	e.run.skipQR = func(int) bool { return true }
	e.r.linkWait = 100 * time.Millisecond
	w := e.do(t, http.MethodPost, "/api/chatops/feishu-app/register", nil)
	assert.Equal(t, http.StatusGatewayTimeout, w.Code)
	assert.Contains(t, w.Body.String(), "飞书没有及时返回二维码")
	waitCtxDone(t, e.run.next(t).ctx)
	e.r.mu.Lock()
	state, msg := e.r.cur.state, e.r.cur.message
	e.r.mu.Unlock()
	assert.Equal(t, feishuRegFailed, state)
	assert.Equal(t, "飞书没有及时返回二维码", msg)
}

// 拿到链接以前就失败（例如连不上账号服务）：回 502 与原因。
func TestFeishuRegister_FailsBeforeLink(t *testing.T) {
	e := newRegEnv(t)
	e.run.early = errors.New("飞书一键创建失败: Post \"https://accounts.feishu.cn/oauth/v1/app/registration\": dial tcp: connection refused")
	w := e.do(t, http.MethodPost, "/api/chatops/feishu-app/register", map[string]string{"name": "x"})
	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Contains(t, w.Body.String(), "connection refused")
	assert.Contains(t, w.Body.String(), "改用「填写 App ID 与 App Secret」")
}

// 拿到链接以前被新的取代：回 409，不给链接。
func TestFeishuRegister_SupersededBeforeLink(t *testing.T) {
	e := newRegEnv(t)
	e.run.skipQR = func(n int) bool { return n == 1 }
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/chatops/feishu-app/register", nil))
		first <- w
	}()
	e.run.next(t)
	e.start(t, "")
	e.run.next(t)
	select {
	case w := <-first:
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), "新的")
		assert.NotContains(t, w.Body.String(), "user_code")
	case <-time.After(3 * time.Second):
		t.Fatal("第一个开始请求没有返回")
	}
}

// 拿到链接以后马上就结束了（这里是拒绝）：照样回第一次给的链接，状态按现在的；后来再给的链接不要。
func TestFeishuRegister_EndsRightAfterLink(t *testing.T) {
	e := newRegEnv(t)
	e.run.quick = feishuapp.ErrRegisterDenied
	s := e.start(t, "")
	e.run.next(t)
	assert.Equal(t, fakeRegURL, s.URL)
	assert.Contains(t, []string{feishuRegPending, feishuRegDenied}, s.State)
	st := e.waitState(t, s.ID, feishuRegDenied)
	assert.Equal(t, "你在飞书里取消了创建", st["message"])
}

// 网页在拿到二维码以前断开：没人看得到链接，作业取消。
func TestFeishuRegister_ClientGoneBeforeLink(t *testing.T) {
	e := newRegEnv(t)
	e.run.skipQR = func(int) bool { return true }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/chatops/feishu-app/register", nil).WithContext(ctx))
	}()
	call := e.run.next(t)
	cancel()
	<-done
	waitCtxDone(t, call.ctx)
	e.r.mu.Lock()
	state := e.r.cur.state
	e.r.mu.Unlock()
	assert.Equal(t, feishuRegCanceled, state)
}

// 链接编不成二维码（超出二维码容量）：作业取消、失败，回 500。
func TestFeishuRegister_QRFails(t *testing.T) {
	e := newRegEnv(t)
	e.run.url = "https://open.feishu.cn/page/launcher?x=" + strings.Repeat("a", 4000)
	w := e.do(t, http.MethodPost, "/api/chatops/feishu-app/register", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "生成二维码失败")
	waitCtxDone(t, e.run.next(t).ctx)
}

// 终态留 30 分钟（测试里拨钟）以后清掉：查状态回 404。
func TestFeishuRegister_FinishedStateExpires(t *testing.T) {
	e := newRegEnv(t)
	var mu sync.Mutex
	now := time.Now()
	e.r.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	s := e.start(t, "")
	e.run.next(t).finish(nil, feishuapp.ErrRegisterExpired)
	e.waitState(t, s.ID, feishuRegExpired)
	mu.Lock()
	now = now.Add(29 * time.Minute)
	mu.Unlock()
	code, _ := e.status(t, s.ID)
	assert.Equal(t, http.StatusOK, code)
	mu.Lock()
	now = now.Add(2 * time.Minute)
	mu.Unlock()
	code, out := e.status(t, s.ID)
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, "这个一键创建已经被新的取代或过期", out["detail"])
}

// 日志：开始与终态各一行，写状态、App ID 与通道编号；不写链接（带一次性的 user_code）与 App Secret。
func TestFeishuRegister_LogsWithoutLinkOrSecret(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	saved := global.GlobalLogger
	global.GlobalLogger = zap.New(core)
	t.Cleanup(func() { global.GlobalLogger = saved }) // 先登记的后执行：newRegEnv 等协程退出以后才换回
	e := newRegEnv(t)
	s := e.start(t, "")
	e.run.next(t).finish(scannedApp(), nil)
	e.waitState(t, s.ID, feishuRegDone)
	require.Eventually(t, func() bool {
		for _, l := range logs.All() {
			if strings.Contains(l.Message, "结束") {
				return true
			}
		}
		return false
	}, 3*time.Second, 10*time.Millisecond)
	var all []string
	for _, l := range logs.All() {
		all = append(all, l.Message)
	}
	text := strings.Join(all, "\n")
	assert.Contains(t, text, "开始")
	assert.Contains(t, text, "state=done")
	assert.Contains(t, text, "cli_scan_app")
	assert.Contains(t, text, "conf_id=41")
	assert.NotContains(t, text, "UC-ONCE-ONLY")
	assert.NotContains(t, text, "launcher")
	assert.NotContains(t, text, fakeRegSecret)
}

// 没有日志器时（进程初始化以前）日志什么也不做，不会因为空指针退出。
func TestFeishuRegister_LoggerNil(t *testing.T) {
	saved := global.GlobalLogger
	global.GlobalLogger = nil
	logf := regLogger()
	global.GlobalLogger = saved
	assert.NotPanics(t, func() {
		logf(false, "开始 id=%s", "x")
		logf(true, "记审计失败: %v", errors.New("x"))
	})
}

// 没接上一键创建（FeishuRegister 为 nil）时三个接口都回 503。
func TestFeishuRegister_NotWired(t *testing.T) {
	mux := http.NewServeMux()
	RegisterChatOpsRoutes(mux, &ChatOpsDeps{NotificationSvc: &stubNotificationSvc{}, BindingSvc: &stubBindingSvc{}, AuditSvc: &stubAuditSvc{}},
		func(h http.Handler) http.Handler { return h })
	for _, rq := range [][2]string{
		{http.MethodPost, "/api/chatops/feishu-app/register"},
		{http.MethodGet, "/api/chatops/feishu-app/register/abc"},
		{http.MethodPost, "/api/chatops/feishu-app/register/abc/cancel"},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(rq[0], rq[1], nil))
		assert.Equal(t, http.StatusServiceUnavailable, w.Code, rq[1])
		assert.Contains(t, w.Body.String(), "这个版本没有接入一键创建", rq[1])
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
}

// 生产的接线：经 RegisterChatOpsRoutes 用 ChatOpsDeps 里的建通道、直接绑定与审计；只认 session，Bearer 令牌一律 401。
func TestFeishuRegister_SessionOnlyAndWired(t *testing.T) {
	run := newFakeFeishuRun()
	notif := &stubNotificationSvc{createResp: app.NotificationConfDTO{ID: 77, ChannelType: "feishu_app"}}
	bind := &stubBindingSvc{}
	audit := &stubAuditSvc{}
	s := &Server{sessions: newSessionStore()}
	s.SetChatOpsDeps(&ChatOpsDeps{NotificationSvc: notif, BindingSvc: bind, AuditSvc: audit, FeishuRegister: run.run})
	mux := http.NewServeMux()
	s.registerChatOpsIfWired(mux)

	for _, rq := range [][2]string{
		{http.MethodPost, "/api/chatops/feishu-app/register"},
		{http.MethodGet, "/api/chatops/feishu-app/register/abc"},
		{http.MethodPost, "/api/chatops/feishu-app/register/abc/cancel"},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(rq[0], rq[1], nil)
		req.Header.Set("Authorization", "Bearer ptt_1_anything")
		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code, rq[1])
	}
	assert.Zero(t, len(run.calls))

	s.sessions.put("sess-feishu-reg", "admin")
	call := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: "sess-feishu-reg"})
		mux.ServeHTTP(w, req)
		return w
	}
	w := call(http.MethodPost, "/api/chatops/feishu-app/register")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var started feishuRegisterStart
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &started))
	run.next(t).finish(scannedApp(), nil)
	var st map[string]any
	require.Eventually(t, func() bool {
		w = call(http.MethodGet, "/api/chatops/feishu-app/register/"+started.ID)
		st = nil
		_ = json.Unmarshal(w.Body.Bytes(), &st)
		return st["state"] == feishuRegDone
	}, 3*time.Second, 10*time.Millisecond)
	assert.EqualValues(t, 77, st["conf_id"])
	assert.Equal(t, "feishu_app", notif.lastCreate.ChannelType)
	assert.Equal(t, uint(77), bind.lastBindConf)
	assert.Equal(t, "ou_scanner", bind.lastBindUser)
	require.Len(t, audit.recorded, 1)
	assert.Equal(t, "feishu_app", audit.recorded[0].ChannelType)
	assert.Equal(t, "success", audit.recorded[0].Result)
}

// 一键创建只给登录了网页的管理员用：不进配置网关的资源表、不进 App API（MCP 与远程设备都只经 App API）。
func TestFeishuRegister_NotExposedElsewhere(t *testing.T) {
	s := &Server{}
	for _, r := range s.configResources() {
		assert.NotContains(t, r.Path, "feishu-app", r.Name)
		assert.NotContains(t, r.ItemPath, "feishu-app", r.Name)
	}
	for _, r := range s.appRoutes() {
		assert.NotContains(t, r.Path, "feishu-app")
		assert.NotContains(t, r.Path, "register")
	}
}
