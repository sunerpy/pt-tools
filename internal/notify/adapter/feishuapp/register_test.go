package feishuapp

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/larksuite/oapi-sdk-go/v3/scene/registration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAccounts 是假的飞书账号服务：照 SDK v3.13.0 scene/registration 的协议实现 /oauth/v1/app/registration
// 的 begin 与 poll（表单 POST，回 JSON；interval 回 1 秒）。poll 按第几次轮询回答（从 1 数起）。
type fakeAccounts struct {
	srv      *httptest.Server
	expireIn int
	poll     func(n int) map[string]any
	// pollWait 是每次 poll 回答以前先等多久（测请求在路上时链接过期）
	pollWait time.Duration

	mu     sync.Mutex
	begins []url.Values
	polls  []url.Values
}

func newFakeAccounts(t *testing.T, poll func(n int) map[string]any) *fakeAccounts {
	t.Helper()
	f := &fakeAccounts{expireIn: 600, poll: poll}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	saved := registerBaseURL
	registerBaseURL = func() string { return f.srv.URL + "/" }
	t.Cleanup(func() { registerBaseURL = saved })
	return f
}

func (f *fakeAccounts) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/oauth/v1/app/registration" {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	f.mu.Lock()
	wait, expireIn := f.pollWait, f.expireIn
	f.mu.Unlock()
	switch r.PostForm.Get("action") {
	case "begin":
		f.mu.Lock()
		f.begins = append(f.begins, r.PostForm)
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":               "dc_qa_device",
			"user_code":                 "UC-QA",
			"verification_uri":          f.srv.URL + "/page/launcher",
			"verification_uri_complete": f.srv.URL + "/page/launcher?user_code=UC-QA",
			"interval":                  1,
			"expire_in":                 expireIn,
		})
	case "poll":
		f.mu.Lock()
		f.polls = append(f.polls, r.PostForm)
		n := len(f.polls)
		f.mu.Unlock()
		if wait > 0 {
			select {
			case <-time.After(wait):
			case <-r.Context().Done():
				return
			}
		}
		_ = json.NewEncoder(w).Encode(f.poll(n))
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}

func (f *fakeAccounts) pollCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.polls)
}

// forms 是收到的 begin 与 poll 表单（副本）。
func (f *fakeAccounts) forms() (begins, polls []url.Values) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.begins...), append([]url.Values(nil), f.polls...)
}

// set 改假服务的有效期与轮询等待（加锁：上一个请求的处理协程可能还在读）。
func (f *fakeAccounts) set(expireIn int, pollWait time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.expireIn, f.pollWait = expireIn, pollWait
}

func pending(int) map[string]any { return map[string]any{"error": "authorization_pending"} }

// decodeAddons 把链接里的 addons（gzip 以后 base64url，不带填充）解回 JSON。
func decodeAddons(t *testing.T, s string) string {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(s)
	require.NoError(t, err)
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	require.NoError(t, err)
	body, err := io.ReadAll(zr)
	require.NoError(t, err)
	return string(body)
}

// 第一次轮询还在等用户确认，第二次拿到应用：返回 App ID、App Secret、扫码人的 open_id 与品牌；
// 链接里预填的名称、描述、来源，以及最小模板的三项权限与一个事件（计划 E3），正好是这些、不多不少。
func TestRegisterApp_ConfirmAfterPending(t *testing.T) {
	f := newFakeAccounts(t, func(n int) map[string]any {
		if n == 1 {
			return pending(n)
		}
		return map[string]any{
			"client_id": "cli_new_app", "client_secret": "new-app-secret",
			"user_info": map[string]any{"open_id": "ou_scanner", "tenant_brand": "feishu"},
		}
	})
	var qrURL string
	var qrExpire, qrCalls int
	got, err := RegisterApp(context.Background(), func(u string, expireIn int) {
		qrCalls++
		qrURL, qrExpire = u, expireIn
	})
	require.NoError(t, err)
	assert.Equal(t, &RegisteredApp{AppID: "cli_new_app", AppSecret: "new-app-secret", OpenID: "ou_scanner", Brand: DomainFeishu}, got)
	assert.Equal(t, 1, qrCalls, "链接只给一次")
	assert.Equal(t, 600, qrExpire)
	begins, polls := f.forms()
	require.Len(t, polls, 2)
	require.Len(t, begins, 1)
	assert.Equal(t, "PersonalAgent", begins[0].Get("archetype"))
	assert.Equal(t, "client_secret", begins[0].Get("auth_method"))
	assert.Equal(t, "open_id", begins[0].Get("request_user_info"))
	assert.Equal(t, "dc_qa_device", polls[0].Get("device_code"))

	u, err := url.Parse(qrURL)
	require.NoError(t, err)
	q := u.Query()
	assert.Equal(t, "UC-QA", q.Get("user_code"))
	assert.Equal(t, "go-sdk/pt-tools", q.Get("source"))
	assert.Equal(t, "pt-tools", q.Get("name"))
	assert.Equal(t, "pt-tools 的飞书机器人：收发命令、和 AI 助手对话、接收通知", q.Get("desc"))
	assert.NotContains(t, q, "avatar")
	assert.NotContains(t, q, "createOnly")
	assert.NotContains(t, q, "clientID", "建的是新应用，不是给已有应用补配置")
	assert.JSONEq(t, `{
		"preset": false,
		"scopes": {"tenant": ["im:message:send_as_bot", "im:message.p2p_msg:readonly", "im:message.group_at_msg:readonly"]},
		"events": {"items": {"tenant": ["im.message.receive_v1"]}}
	}`, decodeAddons(t, q.Get("addons")))
}

// Lark 租户：SDK 先拿到品牌 lark，改连 Lark 的账号服务再问一次；结果的品牌是 lark。
func TestRegisterApp_LarkTenant(t *testing.T) {
	f := newFakeAccounts(t, func(n int) map[string]any {
		if n == 1 {
			return map[string]any{"user_info": map[string]any{"tenant_brand": "lark"}}
		}
		return map[string]any{
			"client_id": "cli_lark_app", "client_secret": "lark-secret",
			"user_info": map[string]any{"open_id": "ou_lark_user", "tenant_brand": "lark"},
		}
	})
	got, err := RegisterApp(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, &RegisteredApp{AppID: "cli_lark_app", AppSecret: "lark-secret", OpenID: "ou_lark_user", Brand: DomainLark}, got)
	assert.Equal(t, 2, f.pollCount())
}

// 没给品牌、没给扫码人时：品牌按 feishu，open_id 为空（调用方据此不填管理员）。
func TestRegisterApp_NoUserInfo(t *testing.T) {
	newFakeAccounts(t, func(int) map[string]any {
		return map[string]any{"client_id": " cli_bare ", "client_secret": "bare-secret"}
	})
	got, err := RegisterApp(context.Background(), func(string, int) {})
	require.NoError(t, err)
	assert.Equal(t, &RegisteredApp{AppID: "cli_bare", AppSecret: "bare-secret", Brand: DomainFeishu}, got)
}

func TestRegisterApp_Denied(t *testing.T) {
	newFakeAccounts(t, func(int) map[string]any {
		return map[string]any{"error": "access_denied", "error_description": "user denied"}
	})
	got, err := RegisterApp(context.Background(), func(string, int) {})
	assert.Nil(t, got)
	require.ErrorIs(t, err, ErrRegisterDenied)
	assert.Equal(t, "你在飞书里取消了创建", err.Error())
}

func TestRegisterApp_Expired(t *testing.T) {
	newFakeAccounts(t, func(int) map[string]any {
		return map[string]any{"error": "expired_token", "error_description": "device code expired"}
	})
	got, err := RegisterApp(context.Background(), func(string, int) {})
	assert.Nil(t, got)
	require.ErrorIs(t, err, ErrRegisterExpired)
	assert.Equal(t, "二维码过期了", err.Error())
}

// 链接的有效期（SDK 按 expire_in 给轮询设的时限）到的时候，轮询请求正好在路上：也算过期。
func TestRegisterApp_ExpiredWhilePolling(t *testing.T) {
	f := newFakeAccounts(t, pending)
	f.set(1, 3*time.Second)
	start := time.Now()
	_, err := RegisterApp(context.Background(), func(string, int) {})
	require.ErrorIs(t, err, ErrRegisterExpired)
	assert.Less(t, time.Since(start), 2500*time.Millisecond, "请求随时限结束，不等假服务回答")
}

// ctx 取消：原样返回 context.Canceled，请求马上结束。
func TestRegisterApp_Canceled(t *testing.T) {
	f := newFakeAccounts(t, pending)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got, err := RegisterApp(ctx, func(string, int) {
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()
	})
	assert.Nil(t, got)
	assert.Equal(t, context.Canceled, err)
	assert.LessOrEqual(t, f.pollCount(), 1)

	// 轮询请求在路上时取消也一样
	f.set(600, 3*time.Second)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	start := time.Now()
	_, err = RegisterApp(ctx2, func(string, int) {
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel2()
		}()
	})
	assert.Equal(t, context.Canceled, err)
	assert.Less(t, time.Since(start), 2*time.Second)
}

// 其他错误包一层「飞书一键创建失败」，原来的错误还在（errors.As 拿得到），错误里没有 device_code。
func TestRegisterApp_OtherErrors(t *testing.T) {
	newFakeAccounts(t, func(int) map[string]any {
		return map[string]any{"error": "invalid_client", "error_description": "client not allowed"}
	})
	_, err := RegisterApp(context.Background(), func(string, int) {})
	require.Error(t, err)
	assert.Equal(t, "飞书一键创建失败: register app error: invalid_client: client not allowed", err.Error())
	var rae *registration.RegisterAppError
	require.ErrorAs(t, err, &rae)
	assert.Equal(t, "invalid_client", rae.Code)
	assert.NotContains(t, err.Error(), "dc_qa_device")
	assert.False(t, errors.Is(err, ErrRegisterDenied) || errors.Is(err, ErrRegisterExpired))

	// 账号服务出错、回空的响应：同样包一层
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer empty.Close()
	registerBaseURL = func() string { return empty.URL }
	qrCalls := 0
	_, err = RegisterApp(context.Background(), func(string, int) { qrCalls++ })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "飞书一键创建失败: ")
	assert.Contains(t, err.Error(), "empty response body")
	assert.Zero(t, qrCalls, "开始就失败时没有链接")
}

// 账号服务给的链接解析不了：url.Parse 的错误里本来带着整条链接（含一次性的 user_code），这里只留问号以前的部分；
// 网络错误（地址本来就不带查询参数）照旧，原来的错误还拿得到。
func TestRegisterApp_URLErrorsDropQuery(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":               "dc_qa_device",
			"verification_uri_complete": "http://bad host/page/launcher?user_code=UC-LEAK-0001#frag",
			"interval":                  1,
			"expire_in":                 600,
		})
	}))
	defer bad.Close()
	saved := registerBaseURL
	t.Cleanup(func() { registerBaseURL = saved })
	registerBaseURL = func() string { return bad.URL }
	_, err := RegisterApp(context.Background(), func(string, int) {})
	require.Error(t, err)
	assert.Equal(t, `飞书一键创建失败: parse "http://bad host/page/launcher": invalid character " " in host name`, err.Error())
	assert.NotContains(t, err.Error(), "UC-LEAK-0001")
	var ue *url.Error
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "parse", ue.Op)

	// 连不上：*url.Error 的地址是接口地址，没有查询参数，内容照旧
	gone := httptest.NewServer(http.NotFoundHandler())
	addr := gone.URL
	gone.Close()
	registerBaseURL = func() string { return addr }
	_, err = RegisterApp(context.Background(), func(string, int) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `飞书一键创建失败: Post "`+addr+`/oauth/v1/app/registration": `)
	assert.Contains(t, err.Error(), "connection refused")
}

// 正式构建里不改地址（SDK 默认 accounts.feishu.cn，Lark 租户由它自己切到 accounts.larksuite.com）；
// 预填按计划 E3；onQR 为空时 SDK 要的回调也在。
func TestRegisterOptions(t *testing.T) {
	t.Setenv("PT_TOOLS_QA_FEISHU_ACCOUNTS_URL", "")
	opts := registerOptions(nil)
	assert.Empty(t, opts.Domain)
	assert.Empty(t, opts.LarkDomain)
	assert.Equal(t, "pt-tools", opts.Source)
	require.NotNil(t, opts.AppPreset)
	assert.Equal(t, "pt-tools", opts.AppPreset.Name)
	assert.Equal(t, "pt-tools 的飞书机器人：收发命令、和 AI 助手对话、接收通知", opts.AppPreset.Desc)
	assert.Empty(t, opts.AppPreset.Avatar)
	require.NotNil(t, opts.Addons)
	require.NotNil(t, opts.Addons.Preset, "nil 是平台默认模板，要显式传 false")
	assert.False(t, *opts.Addons.Preset)
	assert.Equal(t, []string{"im:message:send_as_bot", "im:message.p2p_msg:readonly", "im:message.group_at_msg:readonly"}, opts.Addons.Scopes.Tenant)
	assert.Empty(t, opts.Addons.Scopes.User)
	assert.Equal(t, []string{"im.message.receive_v1"}, opts.Addons.Events.Items.Tenant)
	assert.Empty(t, opts.Addons.Events.Items.User)
	assert.Empty(t, opts.Addons.Callbacks.Items, "卡片回调放到 1.0 以后")
	assert.False(t, opts.CreateOnly)
	assert.Empty(t, opts.AppID)
	require.NotNil(t, opts.OnQRCode)
	opts.OnQRCode(&registration.QRCodeInfo{URL: "https://example.invalid/x", ExpireIn: 600})

	var seen string
	opts = registerOptions(func(u string, _ int) { seen = u })
	opts.OnQRCode(&registration.QRCodeInfo{URL: "https://example.invalid/y", ExpireIn: 600})
	assert.Equal(t, "https://example.invalid/y", seen)
}
