package feishuapp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/larksuite/oapi-sdk-go/v3/scene/registration"
)

/*
 * 扫码一键创建：用飞书 SDK 的 scene/registration（v3.13.0，archetype PersonalAgent）在扫码人所在的企业里建一个应用，
 * 预填 pt-tools 要的三项权限与「接收消息 v2.0」事件（最小模板，不加回调）。2026-11-19 真飞书实测：这样建的应用
 * 不用改订阅方式、不用发布版本，长连接、取机器人信息、私聊与群里 @、通知都直接能用。
 *
 * SDK 的几处行为（scene/registration/registration.go）：
 *   - 开始与轮询都是表单 POST，device_code 在请求体里；网络错误里只有不带查询参数的接口地址，RegisterAppError
 *     只有 code 与 description，所以这里返回的错误里没有凭据。解析 verification_uri_complete 失败时 url.Parse 的
 *     错误带着整条链接（含一次性的 user_code），registerError 只留问号以前的部分。
 *   - 请求都用调用方的 ctx（http.NewRequestWithContext），ctx 取消或到期时马上结束；轮询另有链接有效期
 *     （expire_in，通常 10 分钟）的时限。
 *   - 用 http.DefaultClient：跟随 HTTPS_PROXY / NO_PROXY，不认通道级 proxy_url（建应用的时候通道还不存在）。
 *   - 轮询时会 fmt.Printf("tenant brand: …") 往标准输出写一行租户品牌（feishu 或 lark），不含敏感信息，
 *     SDK 没有办法关掉（与 newLarkRunnerAt 注释里 dispatcher 的那一行一样）。
 */

// 用户在飞书里拒绝、二维码过期时的错误：网页按它们写状态。
var (
	ErrRegisterDenied  = errors.New("你在飞书里取消了创建")
	ErrRegisterExpired = errors.New("二维码过期了")
)

// RegisteredApp 是一键创建拿到的应用。AppSecret 只给调用方建通道用，不要打印、不要回给网页。
type RegisteredApp struct{ AppID, AppSecret, OpenID, Brand string } // Brand 是 feishu 或 lark

// 确认页上预填的名称与描述（用户在确认页上可以改），以及权限与事件：与手动创建的文档要求的三项权限一致。
const (
	registerSource  = "pt-tools"
	registerAppName = "pt-tools"
	registerAppDesc = "pt-tools 的飞书机器人：收发命令、和 AI 助手对话、接收通知"
)

var (
	registerScopes = []string{"im:message:send_as_bot", "im:message.p2p_msg:readonly", "im:message.group_at_msg:readonly"}
	registerEvents = []string{"im.message.receive_v1"}
)

// registerBaseURL 是账号服务的地址：空串时用 SDK 默认的 https://accounts.feishu.cn（Lark 租户由 SDK 自己切到
// accounts.larksuite.com）。qa 构建可以改连本地的假服务（PT_TOOLS_QA_FEISHU_ACCOUNTS_URL）；测试改这个变量。
var registerBaseURL = qaAccountsURL

// RegisterApp 阻塞到用户在飞书里确认、拒绝、链接过期或 ctx 取消；onQR 在拿到链接时调一次（链接与有效秒数）。
// 拒绝时返回 ErrRegisterDenied，过期时返回 ErrRegisterExpired，ctx 取消时原样返回 context.Canceled，
// 其他错误包一层「飞书一键创建失败」。品牌不是 lark 时一律按 feishu。
func RegisterApp(ctx context.Context, onQR func(url string, expireIn int)) (*RegisteredApp, error) {
	res, err := registration.RegisterApp(ctx, registerOptions(onQR))
	if err != nil {
		return nil, registerError(ctx, err)
	}
	app := &RegisteredApp{AppID: strings.TrimSpace(res.ClientID), AppSecret: res.ClientSecret, Brand: DomainFeishu}
	if res.UserInfo != nil {
		app.OpenID = strings.TrimSpace(res.UserInfo.OpenID)
		if strings.EqualFold(strings.TrimSpace(res.UserInfo.TenantBrand), DomainLark) {
			app.Brand = DomainLark
		}
	}
	return app, nil
}

// registerOptions 是交给 SDK 的选项：来源、预填的名称与描述、最小模板的权限与事件。
func registerOptions(onQR func(url string, expireIn int)) *registration.Options {
	minimal := false
	opts := &registration.Options{
		Source:    registerSource,
		AppPreset: &registration.AppPreset{Name: registerAppName, Desc: registerAppDesc},
		Addons: &registration.AppAddons{
			// nil 是平台默认模板；false 是最小模板，确认页上只有下面这几项
			Preset: &minimal,
			Scopes: registration.AppAddonsScopes{Tenant: registerScopes},
			Events: registration.AppAddonsEvents{Items: registration.AppAddonsEventItems{Tenant: registerEvents}},
		},
		// SDK 要求这个回调不为空
		OnQRCode: func(info *registration.QRCodeInfo) {
			if onQR != nil {
				onQR(info.URL, info.ExpireIn)
			}
		},
	}
	if u := strings.TrimRight(registerBaseURL(), "/"); u != "" {
		opts.Domain, opts.LarkDomain = u, u
	}
	return opts
}

// registerError 把 SDK 的错误换成调用方认得的几种。
func registerError(ctx context.Context, err error) error {
	var denied *registration.AccessDeniedError
	var expired *registration.ExpiredError
	switch {
	case errors.As(err, &denied):
		return ErrRegisterDenied
	case errors.As(err, &expired):
		return ErrRegisterExpired
	case errors.Is(ctx.Err(), context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		// 链接的有效期或调用方的时限到的时候，请求正好在路上（SDK 只在两次轮询之间的等待里认出过期）
		return ErrRegisterExpired
	default:
		var ue *url.Error
		if errors.As(err, &ue) {
			err = &url.Error{Op: ue.Op, URL: withoutQuery(ue.URL), Err: ue.Err}
		}
		return fmt.Errorf("飞书一键创建失败: %w", err)
	}
}

// withoutQuery 去掉地址里问号或井号以后的部分。
func withoutQuery(s string) string {
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		return s[:i]
	}
	return s
}
