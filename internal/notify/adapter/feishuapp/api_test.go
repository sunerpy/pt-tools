package feishuapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/notify"
)

// fakeOpenAPI 是开放平台的假接口：换 tenant_access_token、发消息、取机器人信息。
type fakeOpenAPI struct {
	srv *httptest.Server

	mu       sync.Mutex
	messages []fakeMessage
	auths    []string
	tokens   atomic.Int32
	// next 不为空时，下一次发消息按它回（回完清空）
	next func(w http.ResponseWriter)
	// tokenCode 不为 0 时换 token 失败
	tokenCode int
	botCode   int
}

type fakeMessage struct {
	ReceiveIDType string
	ReceiveID     string `json:"receive_id"`
	MsgType       string `json:"msg_type"`
	Content       string `json:"content"`
}

func newFakeOpenAPI(t *testing.T) *fakeOpenAPI {
	t.Helper()
	f := &fakeOpenAPI{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /open-apis/auth/v3/tenant_access_token/internal", func(w http.ResponseWriter, r *http.Request) {
		f.tokens.Add(1)
		var body struct {
			AppID     string `json:"app_id"`
			AppSecret string `json:"app_secret"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		f.mu.Lock()
		code := f.tokenCode
		f.mu.Unlock()
		if code != 0 {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": "token request failed"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "ok", "tenant_access_token": "t-g1044fakefaketoken" + body.AppID, "expire": 7200})
	})
	mux.HandleFunc("POST /open-apis/im/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		var m fakeMessage
		_ = json.NewDecoder(r.Body).Decode(&m)
		m.ReceiveIDType = r.URL.Query().Get("receive_id_type")
		f.mu.Lock()
		f.messages = append(f.messages, m)
		f.auths = append(f.auths, r.Header.Get("Authorization"))
		next := f.next
		f.next = nil
		f.mu.Unlock()
		if next != nil {
			next(w)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, `{"code":0,"msg":"success","data":{"message_id":"om_sent"}}`)
	})
	mux.HandleFunc("GET /open-apis/bot/v3/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		f.mu.Lock()
		code := f.botCode
		f.mu.Unlock()
		if code != 0 {
			_, _ = io.WriteString(w, `{"code":230006,"msg":"bot ability is not activated"}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","bot":{"open_id":"ou_bot_real","app_name":"pt-tools"}}`)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOpenAPI) sent() []fakeMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeMessage(nil), f.messages...)
}

// testAPI 用真的 lark.Client 连假接口。SDK 的 token 缓存是进程级的，按 app_id 分键：每个用例（连同 -count 的每一轮）
// 用不同的 app_id。
func testAPI(t *testing.T, f *fakeOpenAPI, prefix string) apiSender {
	t.Helper()
	appID := uniqueAppID(prefix)
	cfg := &Config{AppID: appID, AppSecret: "api-secret-" + prefix, Domain: DomainFeishu}
	return newLarkAPIAt(f.srv.URL, cfg, sdkLogger{log: func() *zap.SugaredLogger { return zap.NewNop().Sugar() }})
}

var appIDSeq atomic.Int64

func uniqueAppID(prefix string) string {
	return prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 36) + strconv.FormatInt(appIDSeq.Add(1), 10)
}

func TestLarkAPI_SendText(t *testing.T) {
	f := newFakeOpenAPI(t)
	api := testAPI(t, f, "cli_api_send")
	res, err := api.SendText(context.Background(), "open_id", "ou_a", `{"text":"你好"}`)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Code)
	assert.Equal(t, 200, res.Status)

	sent := f.sent()
	require.Len(t, sent, 1)
	assert.Equal(t, fakeMessage{ReceiveIDType: "open_id", ReceiveID: "ou_a", MsgType: "text", Content: `{"text":"你好"}`}, sent[0])
	assert.True(t, strings.HasPrefix(f.auths[0], "Bearer t-g1044fakefaketokencli_api_send_"), f.auths[0])

	_, err = api.SendText(context.Background(), "chat_id", "oc_b", `{"text":"2"}`)
	require.NoError(t, err)
	assert.Equal(t, int32(1), f.tokens.Load(), "tenant_access_token 由 SDK 缓存")
}

func TestLarkAPI_RateLimitHeader(t *testing.T) {
	f := newFakeOpenAPI(t)
	f.next = func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-ogw-ratelimit-reset", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"code":99991400,"msg":"request trigger frequency limit"}`)
	}
	res, err := testAPI(t, f, "cli_api_429").SendText(context.Background(), "open_id", "ou_a", `{"text":"x"}`)
	require.NoError(t, err)
	assert.Equal(t, sendResult{Code: 99991400, Msg: "request trigger frequency limit", Status: 429, RetryAfter: time.Second}, res)
}

func TestLarkAPI_NonJSONResponse(t *testing.T) {
	f := newFakeOpenAPI(t)
	f.next = func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "<html>bad gateway</html>")
	}
	res, err := testAPI(t, f, "cli_api_502").SendText(context.Background(), "open_id", "ou_a", `{"text":"x"}`)
	require.NoError(t, err)
	assert.Equal(t, codeNotJSON, res.Code)
	assert.Equal(t, 502, res.Status)
}

func TestLarkAPI_TokenFailureHidesSecret(t *testing.T) {
	f := newFakeOpenAPI(t)
	f.tokenCode = 10014
	_, err := testAPI(t, f, "cli_api_badsecret").SendText(context.Background(), "open_id", "ou_a", `{"text":"x"}`)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "api-secret-cli_api_badsecret")
	assert.Contains(t, err.Error(), "10014")
}

func TestLarkAPI_BotInfo(t *testing.T) {
	f := newFakeOpenAPI(t)
	info, res, err := testAPI(t, f, "cli_api_bot").BotInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, res.Code)
	assert.Equal(t, botInfo{OpenID: "ou_bot_real", AppName: "pt-tools"}, info)

	f.botCode = 230006
	_, res, err = testAPI(t, f, "cli_api_bot2").BotInfo(context.Background())
	require.Error(t, err)
	assert.Equal(t, 230006, res.Code)
	assert.Contains(t, err.Error(), "230006")
	assert.Contains(t, err.Error(), "bot ability is not activated")
}

func TestLarkAPI_TransportError(t *testing.T) {
	cfg := &Config{AppID: "cli_api_down", AppSecret: "s", Domain: DomainFeishu}
	api := newLarkAPIAt("http://127.0.0.1:1", cfg, sdkLogger{log: func() *zap.SugaredLogger { return zap.NewNop().Sugar() }})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := api.SendText(ctx, "open_id", "ou_a", `{"text":"x"}`)
	require.Error(t, err)
	_, _, err = api.BotInfo(ctx)
	require.Error(t, err)
}

func TestParseResult(t *testing.T) {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("x-ogw-ratelimit-reset", "bad")
	res := parseResult(200, h, []byte(`{"code":5,"msg":"m"}`))
	assert.Equal(t, sendResult{Code: 5, Msg: "m", Status: 200}, res, "reset 不是数字时不等")
	res = parseResult(200, h, []byte(`{"msg":"no code"}`))
	assert.Equal(t, codeNotJSON, res.Code, "没有 code 不算成功")
	assert.True(t, strings.Contains(parseResult(500, http.Header{}, nil).Msg, "JSON"))
}

// 换 tenant_access_token 失败（App ID、App Secret 填错，应用被停用）时也给中文提示。错误码与含义照飞书官方的
// 「服务端错误码说明」（https://open.feishu.cn/document/ukTMukTMukTM/ugjM14COyUjL4ITN）。
func TestSend_TokenErrorHints(t *testing.T) {
	cases := map[int]string{
		10005:    "App ID",
		10014:    "App Secret 不对",
		10015:    "App Secret 不对",
		20002:    "对不上",
		99991543: "App ID 或 App Secret 不对",
		10003:    "",
	}
	for code, hint := range cases {
		f := newFakeOpenAPI(t)
		f.tokenCode = code
		c, _, _ := newTestChannel(t, testConfig)
		real := testAPI(t, f, "cli_api_tokenhint")
		c.api = real
		err := c.Send(context.Background(), notify.Notification{Text: "x", UserID: "ou_a"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), strconv.Itoa(code))
		if hint == "" {
			assert.NotContains(t, err.Error(), "（", code)
			continue
		}
		assert.Contains(t, err.Error(), hint, code)
	}
}

func TestTokenHint_PointerCodeError(t *testing.T) {
	assert.Contains(t, tokenHint(&larkcore.CodeError{Code: 10015}), "App Secret")
	assert.Contains(t, tokenHint(fmt.Errorf("wrapped: %w", larkcore.CodeError{Code: 10014})), "App Secret 不对")
	assert.Empty(t, tokenHint(errors.New("dial tcp: refused")))
	assert.Empty(t, tokenHint(nil))
}
