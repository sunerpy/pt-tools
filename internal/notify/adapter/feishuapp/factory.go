package feishuapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// httpTimeout 是每个 HTTP 请求（换 token、发消息、取长连接地址）与长连接握手的时限。
// lark.NewClient 默认用 http.DefaultClient，没有超时。
const httpTimeout = 15 * time.Second

// wsHooks 是长连接客户端回调到通道的入口：收消息与连接状态的变化。
// 生命周期回调在 SDK 的协调协程里同步执行，只能改状态、不能阻塞。
type wsHooks struct {
	onMessage      func(ctx context.Context, ev *larkim.P2MessageReceiveV1) error
	onReady        func()
	onReconnecting func()
	onReconnected  func()
	onDisconnected func()
	onError        func(err error)
}

// wsRunner 是长连接客户端：生产实现是 larkws.Client。客户端停下以后不能再 Start，每一轮都要新建。
type wsRunner interface {
	Start(ctx context.Context) error
	CloseAndWait(ctx context.Context) error
}

type runnerFactory func(cfg *Config, hooks wsHooks, log larkcore.Logger) wsRunner

// sendResult 是开放平台对一次请求的回答。
type sendResult struct {
	Code   int
	Msg    string
	Status int
	// RetryAfter 是限流时 x-ogw-ratelimit-reset 给的等待时间；没有时为 0
	RetryAfter time.Duration
}

// codeNotJSON 表示响应里没有 code（不是 JSON，或者 JSON 里没有这个字段）。
const codeNotJSON = -1

type botInfo struct {
	OpenID  string
	AppName string
}

// apiSender 是发消息与取机器人信息的接口：生产实现包着 lark.Client，tenant_access_token 由 SDK 获取并缓存。
type apiSender interface {
	SendText(ctx context.Context, receiveIDType, receiveID, content string) (sendResult, error)
	BotInfo(ctx context.Context) (botInfo, sendResult, error)
}

type apiFactory func(cfg *Config, log larkcore.Logger) apiSender

func newTransport(cfg *Config) *http.Transport {
	return &http.Transport{
		Proxy:                 proxyFunc(cfg),
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

// larkAPI 是 apiSender 的生产实现。
type larkAPI struct {
	cli *lark.Client
}

func newLarkAPI(cfg *Config, log larkcore.Logger) apiSender {
	return newLarkAPIAt(baseURL(cfg), cfg, log)
}

// newLarkAPIAt 建一个连 base 的 client，不发任何请求。不调 lark.WithTokenCache：SDK 的 token 管理器是包级的，
// 传自定义缓存会替换所有客户端的；默认缓存按 app_id 与 secret 指纹分键，多个实例共用没有问题。
// 也不开 WithLogReqAtDebug，它会打印请求体。
func newLarkAPIAt(base string, cfg *Config, log larkcore.Logger) apiSender {
	cli := lark.NewClient(cfg.AppID, cfg.AppSecret,
		lark.WithOpenBaseUrl(base),
		lark.WithHttpClient(&http.Client{Timeout: httpTimeout, Transport: newTransport(cfg)}),
		lark.WithLogger(log),
		lark.WithLogLevel(larkcore.LogLevelInfo),
	)
	return &larkAPI{cli: cli}
}

// SendText 调 POST /open-apis/im/v1/messages?receive_id_type=…（应用身份）。自己解析响应，
// 这样限流时也拿得到 HTTP 状态与 x-ogw-ratelimit-reset。
func (a *larkAPI) SendText(ctx context.Context, receiveIDType, receiveID, content string) (sendResult, error) {
	resp, err := a.cli.Do(ctx, &larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/im/v1/messages",
		Body: map[string]string{
			"receive_id": receiveID,
			"msg_type":   "text",
			"content":    content,
		},
		QueryParams:               larkcore.QueryParams{"receive_id_type": []string{receiveIDType}},
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant},
	})
	if err != nil {
		return sendResult{}, err
	}
	return parseResult(resp.StatusCode, resp.Header, resp.RawBody), nil
}

// BotInfo 调 GET /open-apis/bot/v3/info：本应用机器人的 open_id，群里判断有没有 @ 它要用。
func (a *larkAPI) BotInfo(ctx context.Context) (botInfo, sendResult, error) {
	resp, err := a.cli.Get(ctx, "/open-apis/bot/v3/info", nil, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return botInfo{}, sendResult{}, err
	}
	res := parseResult(resp.StatusCode, resp.Header, resp.RawBody)
	var body struct {
		Bot struct {
			OpenID  string `json:"open_id"`
			AppName string `json:"app_name"`
		} `json:"bot"`
	}
	if res.Code != 0 || json.Unmarshal(resp.RawBody, &body) != nil || body.Bot.OpenID == "" {
		return botInfo{}, res, fmt.Errorf("获取机器人信息失败: code=%d, msg=%s", res.Code, res.Msg)
	}
	return botInfo{OpenID: body.Bot.OpenID, AppName: body.Bot.AppName}, res, nil
}

// parseResult 只取 code 与 msg（服务端的其余内容不进错误），以及限流时的等待秒数。
func parseResult(status int, header http.Header, body []byte) sendResult {
	res := sendResult{Status: status}
	if s := strings.TrimSpace(header.Get("x-ogw-ratelimit-reset")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			res.RetryAfter = time.Duration(n) * time.Second
		}
	}
	var parsed struct {
		Code *int   `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		res.Code, res.Msg = codeNotJSON, "响应不是 JSON"
		return res
	}
	if parsed.Code == nil {
		res.Code, res.Msg = codeNotJSON, "响应里没有 code"
		return res
	}
	res.Code, res.Msg = *parsed.Code, parsed.Msg
	return res
}

func newLarkRunner(cfg *Config, hooks wsHooks, log larkcore.Logger) wsRunner {
	return newLarkRunnerAt(baseURL(cfg), cfg, hooks, log)
}

// newLarkRunnerAt 建一个长连接客户端（不连网）。事件分发器只注册「接收消息」，另外给几种常见事件注册空处理：
// 用户在后台多订阅了别的事件时，SDK 找不到处理函数会回 500，平台会一直重推。
// 注意 dispatcher.NewEventDispatcher 构造时会往标准输出打一行「event-dispatch is ready」，SDK 没有办法关掉。
func newLarkRunnerAt(base string, cfg *Config, hooks wsHooks, log larkcore.Logger) wsRunner {
	d := dispatcher.NewEventDispatcher("", "")
	d.InitConfig(larkevent.WithLogger(log), larkevent.WithLogLevel(larkcore.LogLevelInfo))
	d.OnP2MessageReceiveV1(hooks.onMessage)
	d.OnP2MessageReadV1(func(context.Context, *larkim.P2MessageReadV1) error { return nil })
	d.OnP2ChatMemberBotAddedV1(func(context.Context, *larkim.P2ChatMemberBotAddedV1) error { return nil })
	d.OnP2ChatMemberBotDeletedV1(func(context.Context, *larkim.P2ChatMemberBotDeletedV1) error { return nil })
	d.OnP2ChatAccessEventBotP2pChatEnteredV1(func(context.Context, *larkim.P2ChatAccessEventBotP2pChatEnteredV1) error { return nil })

	return larkws.NewClient(cfg.AppID, cfg.AppSecret,
		larkws.WithEventHandler(d),
		larkws.WithDomain(base),
		larkws.WithLogger(log),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
		larkws.WithHttpClient(&http.Client{Timeout: httpTimeout, Transport: newTransport(cfg)}),
		larkws.WithWebSocketDialer(&websocket.Dialer{Proxy: proxyFunc(cfg), HandshakeTimeout: httpTimeout}),
		larkws.WithOnReady(hooks.onReady),
		larkws.WithOnReconnecting(hooks.onReconnecting),
		larkws.WithOnReconnected(hooks.onReconnected),
		larkws.WithOnDisconnected(hooks.onDisconnected),
		larkws.WithOnError(hooks.onError),
	)
}

// isClientError 报告长连接的错误是不是不可重试的（凭据不对、握手被拒、超过连接数）。
func isClientError(err error) bool {
	var ce *larkws.ClientError
	return errors.As(err, &ce)
}
