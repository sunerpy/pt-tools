package feishuapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"golang.org/x/time/rate"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/outbound"
)

const (
	// maxContentBytes：text 消息的请求体上限是 150 KB（超了回 230025）。content 放进请求体时还要再 JSON 编码一次
	// （见 bodySize），按编码以后的长度超过 140 KiB 时按字截断。
	// ChatOps 的回复已经切成每条 3500 字、最多 4 条（chatops.SplitReply），远低于上限。
	maxContentBytes = 140 * 1024
	truncatedSuffix = "…（内容过长，已截断）"
	// rateRetryMax：限流时 x-ogw-ratelimit-reset 不超过它就等一次再发，否则把错误交回去，由 outbox 按退避重试。
	rateRetryMax = 5 * time.Second
	// 同一用户、同一群各 5 QPS（消息类接口不能申请提频）：每秒 4 条、桶里最多 1 个令牌，任意 1 秒的窗口里最多 5 条。
	sendRate  = 4
	sendBurst = 1
	// maxLimiters 是令牌桶表的上限，满了整表重来（收件人通常只有几个）。
	maxLimiters = 1024
)

// errCodeHints 是常见错误码的中文提示（错误码见「发送消息」文档）。限流的提示不写会不会重试：
// 后台通知会转进 outbox 按退避重试，测试消息与 ChatOps 回复不会。
var errCodeHints = map[int]string{
	230013:   "收件人不在应用的可用范围里：到开发者后台的版本管理把他加进可用范围，再发布一个版本",
	230002:   "机器人不在这个群里",
	230006:   "应用没有开启机器人能力",
	230053:   "对方设置了不再接收这个机器人的消息",
	99991663: "App ID 或 App Secret 不对，或者凭据已失效",
	99991664: "App ID 或 App Secret 不对，或者凭据已失效",
	99991672: "应用缺少权限：到开发者后台开通「以应用的身份发消息」（im:message:send_as_bot）并发布版本",
	99991400: "发送太频繁",
	230020:   "发送太频繁",
}

// tokenErrHints 是换 tenant_access_token 失败时的中文提示：SDK 把换 token 接口回的 code 与 msg 原样当错误返回
// （core/tokenmanager.go 的 getCustomTenantAccessTokenThenCache 返回 CodeError）。错误码与含义照飞书官方的
// 「服务端错误码说明」（https://open.feishu.cn/document/ukTMukTMukTM/ugjM14COyUjL4ITN），那里查不到的码不加。
var tokenErrHints = map[int]string{
	10005:    "应用信息有误：到开发者后台「凭证与基础信息」核对 App ID",
	10014:    "应用当前不可用：到开发者后台检查应用是否被停用",
	10015:    "App Secret 不对：到开发者后台「凭证与基础信息」核对，重置过的要填新的",
	20002:    "App ID 与 App Secret 对不上：到开发者后台「凭证与基础信息」核对",
	99991543: "App ID 或 App Secret 不对：到开发者后台「凭证与基础信息」核对",
}

// tokenHint 是换 token 失败时的中文提示；不是换 token 的错误、或者错误码不在表里时为空串。
func tokenHint(err error) string {
	var ce larkcore.CodeError
	if errors.As(err, &ce) {
		return tokenErrHints[ce.Code]
	}
	var pce *larkcore.CodeError
	if errors.As(err, &pce) && pce != nil {
		return tokenErrHints[pce.Code]
	}
	return ""
}

// withTokenHint 在换 token 失败的错误后面加上中文提示，Unwrap 仍是原错误。
func withTokenHint(err error) error {
	if hint := tokenHint(err); hint != "" {
		return fmt.Errorf("%w（%s）", err, hint)
	}
	return err
}

// Send 发一条纯文本消息。只在没有初始化或已关闭时拒发，不看 Healthy：长连接断了不代表发不出去（#838 的教训）。
// 按钮与图片不发（与 QQ 相同）。
func (c *Channel) Send(ctx context.Context, n notify.Notification) error {
	c.mu.RLock()
	api, cfg := c.api, c.cfg
	c.mu.RUnlock()
	if api == nil || cfg == nil {
		return errors.New("飞书应用机器人通道没有初始化")
	}
	if c.closed.Load() {
		return errors.New("飞书应用机器人通道已关闭")
	}
	target, err := resolveTarget(n, cfg)
	if err != nil {
		return err
	}
	typ, err := receiveIDType(target)
	if err != nil {
		return err
	}
	return c.sendContent(ctx, api, typ, target, textContent(outbound.Text(n)))
}

// sendContent 发一条已经编码好的 content；限流且等待时间不长时等一次再发。
func (c *Channel) sendContent(ctx context.Context, api apiSender, typ, target, content string) error {
	for attempt := 0; ; attempt++ {
		if err := c.limiters.get(target).Wait(ctx); err != nil {
			return fmt.Errorf("飞书应用机器人发送失败: %w", err)
		}
		res, err := api.SendText(ctx, typ, target, content)
		if err != nil {
			return c.red.wrap(fmt.Errorf("飞书应用机器人发送失败: %w", withTokenHint(err)))
		}
		if res.Code == 0 && res.Status < 300 {
			return nil
		}
		if attempt == 0 && rateLimited(res) && res.RetryAfter > 0 && res.RetryAfter <= rateRetryMax && canWait(ctx, res.RetryAfter) {
			timer := time.NewTimer(res.RetryAfter)
			select {
			case <-ctx.Done():
				timer.Stop()
				return fmt.Errorf("飞书应用机器人发送失败: %w", ctx.Err())
			case <-timer.C:
			}
			continue
		}
		return c.sendError(res)
	}
}

func rateLimited(res sendResult) bool {
	return res.Status == 429 || res.Code == 99991400 || res.Code == 230020
}

// canWait 报告等 d 以后还剩不剩时间再发一次（留 1 秒给请求本身）。
func canWait(ctx context.Context, d time.Duration) bool {
	deadline, ok := ctx.Deadline()
	return !ok || time.Until(deadline) > d+time.Second
}

func (c *Channel) sendError(res sendResult) error {
	if res.Code == codeNotJSON {
		return fmt.Errorf("飞书应用机器人发送失败: HTTP %d，%s", res.Status, res.Msg)
	}
	msg := fmt.Sprintf("飞书应用机器人发送失败: code=%d, msg=%s", res.Code, outbound.Clean(res.Msg, c.red.secret))
	if hint := errCodeHints[res.Code]; hint != "" {
		msg += "（" + hint + "）"
	}
	return errors.New(c.red.text(msg))
}

// resolveTarget 是收件人：Targets["chat_id"] → UserID → 默认会话 ID。管理员、绑定账号这两级回退在 app 层做。
func resolveTarget(n notify.Notification, cfg *Config) (string, error) {
	if v := strings.TrimSpace(n.Targets["chat_id"]); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(n.UserID); v != "" {
		return v, nil
	}
	if cfg.DefaultChatID != "" {
		return cfg.DefaultChatID, nil
	}
	return "", errors.New("飞书应用机器人通道没有收件人：通知没有指定会话，通道也没有填默认会话 ID")
}

// receiveIDType 按前缀定 receive_id_type：oc_ 是会话，ou_ 是 open_id，on_ 是 union_id（SDK 没有这几个值的常量）。
func receiveIDType(id string) (string, error) {
	switch {
	case strings.HasPrefix(id, "oc_"):
		return "chat_id", nil
	case strings.HasPrefix(id, "ou_"):
		return "open_id", nil
	case strings.HasPrefix(id, "on_"):
		return "union_id", nil
	default:
		return "", errors.New("飞书应用机器人：收件人 ID 要以 oc_、ou_ 或 on_ 开头")
	}
}

// atTagRe 是 text 消息里会被解析成 @ 的标签开头（不分大小写）。
var atTagRe = regexp.MustCompile(`(?i)<at`)

// textContent 把文字编码成 text 消息的 content（{"text":"…"} 的 JSON 字符串），超过上限时按字截断。
// 发出去的文字来自站点（种子标题）与模型，当成不可信的内容，先中和两种会被飞书渲染的写法：
//   - <at 换成全角的 ＜at：text 消息会解析 <at user_id="all"></at>，带着它就会在群里 @ 所有人；
//   - ]( 换成全角的 ］(：应用机器人的 text 消息会把 [文字](链接) 渲染成链接（「发送消息内容」文档），
//     种子标题能伪造一个看不见真实地址的链接；换掉以后链接地址照样以文字显示。
//
// 不用 SDK 的 NewTextMsgBuilder：它只拼字符串、不做 JSON 转义。
func textContent(s string) string {
	s = atTagRe.ReplaceAllStringFunc(s, func(m string) string { return "＜" + m[1:] })
	s = strings.ReplaceAll(s, "](", "］(")
	content := marshalText(s)
	size := bodySize(content)
	if size <= maxContentBytes {
		return content
	}
	runes := []rune(s)
	n := len(runes) * maxContentBytes / size
	for n > 0 {
		if c := marshalText(string(runes[:n]) + truncatedSuffix); bodySize(c) <= maxContentBytes {
			return c
		}
		n = n * 9 / 10
	}
	return marshalText(truncatedSuffix)
}

// bodySize 是 content 放进请求体以后占的字节数：SDK 用 json.Marshal 序列化请求体（core/serialization.go 的
// DefaultSerialization），HTML 转义开着，< > & 各变成 6 个字节，引号与反斜杠翻倍。
func bodySize(content string) int {
	b, _ := json.Marshal(content) // 字符串编码不会失败
	return len(b)
}

func marshalText(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]string{"text": s})
	return strings.TrimSuffix(buf.String(), "\n")
}

// limiterSet 是每个收件人一个令牌桶。
type limiterSet struct {
	mu    sync.Mutex
	m     map[string]*rate.Limiter
	rate  rate.Limit
	burst int
}

func newLimiterSet(r rate.Limit, burst int) *limiterSet {
	return &limiterSet{m: make(map[string]*rate.Limiter), rate: r, burst: burst}
}

func (s *limiterSet) get(id string) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l := s.m[id]; l != nil {
		return l
	}
	if len(s.m) >= maxLimiters {
		s.m = make(map[string]*rate.Limiter)
	}
	l := rate.NewLimiter(s.rate, s.burst)
	s.m[id] = l
	return l
}
