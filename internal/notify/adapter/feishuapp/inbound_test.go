package feishuapp

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

type evOpt func(*larkim.P2MessageReceiveV1)

// msgEvent 造一条私聊的接收消息事件。
func msgEvent(id, openID, text string, opts ...evOpt) *larkim.P2MessageReceiveV1 {
	ev := &larkim.P2MessageReceiveV1{Event: &larkim.P2MessageReceiveV1Data{
		Sender: &larkim.EventSender{SenderId: &larkim.UserId{OpenId: ptr(openID)}, SenderType: ptr("user")},
		Message: &larkim.EventMessage{
			MessageId:   ptr(id),
			ChatId:      ptr("oc_p2p_" + openID),
			ChatType:    ptr("p2p"),
			MessageType: ptr("text"),
			Content:     ptr(textJSON(text)),
			CreateTime:  ptr(strconv.FormatInt(time.Now().UnixMilli(), 10)),
		},
	}}
	for _, o := range opts {
		o(ev)
	}
	return ev
}

func inGroup(chatID string, mentions ...*larkim.MentionEvent) evOpt {
	return func(ev *larkim.P2MessageReceiveV1) {
		ev.Event.Message.ChatType = ptr("group")
		ev.Event.Message.ChatId = ptr(chatID)
		ev.Event.Message.Mentions = mentions
	}
}

func createdAgo(d time.Duration) evOpt {
	return func(ev *larkim.P2MessageReceiveV1) {
		ev.Event.Message.CreateTime = ptr(strconv.FormatInt(time.Now().Add(-d).UnixMilli(), 10))
	}
}

// chainRecorder 是假的 MessageChain.Process：记下收到的消息；block 不为空时每条都等它。
type chainRecorder struct {
	mu    sync.Mutex
	msgs  []notify.InboundMessage
	block chan struct{}
	ctxs  []context.Context
}

func (r *chainRecorder) handle(ctx context.Context, msg notify.InboundMessage) error {
	r.mu.Lock()
	r.msgs = append(r.msgs, msg)
	r.ctxs = append(r.ctxs, ctx)
	block := r.block
	r.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
		}
	}
	return nil
}

func (r *chainRecorder) got() []notify.InboundMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]notify.InboundMessage(nil), r.msgs...)
}

func (r *chainRecorder) texts() []string {
	var out []string
	for _, m := range r.got() {
		out = append(out, m.Text)
	}
	return out
}

// startChannel 建通道、接上 chain、等长连接「连上」与机器人信息取到。
func startChannel(t *testing.T, raw string) (*Channel, *fakeAPI, *fakeRunners, *chainRecorder) {
	t.Helper()
	c, api, runners := newTestChannel(t, raw)
	rec := &chainRecorder{}
	c.OnInbound(rec.handle)
	r := runners.waitRunner(t, 1)
	r.Ready()
	require.Eventually(t, func() bool { return c.botOpenID() == "ou_bot" }, 5*time.Second, time.Millisecond)
	return c, api, runners, rec
}

func TestInbound_DeliversPrivateMessage(t *testing.T) {
	c, _, runners, rec := startChannel(t, testConfig)
	require.NoError(t, runners.last().Emit(context.Background(), msgEvent("om_1", "ou_admin", " /help ")))
	require.Eventually(t, func() bool { return len(rec.got()) == 1 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, notify.InboundMessage{
		ChannelType:   "feishu_app",
		SourceConfID:  7,
		ChannelUserID: "ou_admin",
		ChatID:        "oc_p2p_ou_admin",
		MessageType:   "private",
		Text:          "/help",
	}, rec.got()[0])
	assert.Equal(t, notify.LinkConnected, c.LinkState())
}

// 平台要求 3 秒内应答：处理函数只做内存操作，链处理卡住时也要马上返回。
func TestInbound_AcksFastWhileChainBlocks(t *testing.T) {
	_, _, runners, rec := startChannel(t, testConfig)
	rec.block = make(chan struct{})
	defer close(rec.block)
	start := time.Now()
	require.NoError(t, runners.last().Emit(context.Background(), msgEvent("om_slow", "ou_admin", "/status")))
	require.NoError(t, runners.last().Emit(context.Background(), msgEvent("om_slow2", "ou_admin", "/sites")))
	assert.Less(t, time.Since(start), 100*time.Millisecond)
}

func TestInbound_DedupeAcrossInstances(t *testing.T) {
	c, _, runners, rec := startChannel(t, testConfig)
	ev := msgEvent("om_dup", "ou_admin", "/status")
	require.NoError(t, runners.last().Emit(context.Background(), ev))
	require.NoError(t, runners.last().Emit(context.Background(), ev), "重复的也回 200，不让平台再推")
	require.Eventually(t, func() bool { return len(rec.got()) == 1 }, 5*time.Second, time.Millisecond)

	// 热重载：关掉旧实例、建新实例，去重表还在
	require.NoError(t, c.Close(context.Background()))
	c2 := New()
	api2 := &fakeAPI{bot: botInfo{OpenID: "ou_bot"}}
	runners2 := &fakeRunners{}
	c2.newAPI = func(*Config, larkcore.Logger) apiSender { return api2 }
	c2.newRunner = runners2.factory
	shortenTimings(c2)
	require.NoError(t, c2.Init(context.Background(), confOf(7, testConfig)))
	defer func() { _ = c2.Close(context.Background()) }()
	rec2 := &chainRecorder{}
	c2.OnInbound(rec2.handle)
	r2 := runners2.waitRunner(t, 1)
	require.NoError(t, r2.Emit(context.Background(), ev))
	require.NoError(t, r2.Emit(context.Background(), msgEvent("om_new", "ou_admin", "/help")))
	require.Eventually(t, func() bool { return len(rec2.got()) == 1 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, "/help", rec2.got()[0].Text)
}

// 离线几小时以后的重推不能执行（D6）：名单内的人收到提示。
func TestInbound_ExpiredMessage(t *testing.T) {
	_, api, runners, rec := startChannel(t, testConfig)
	require.NoError(t, runners.last().Emit(context.Background(), msgEvent("om_old", "ou_admin", "/delete 1", createdAgo(40*time.Minute))))
	require.Eventually(t, func() bool { return len(api.sent()) == 1 }, 5*time.Second, time.Millisecond)
	sent := api.sent()[0]
	assert.Equal(t, "chat_id", sent.typ)
	assert.Equal(t, "oc_p2p_ou_admin", sent.id)
	assert.Contains(t, decodeText(t, sent.content), "40 分钟前发的")
	assert.Contains(t, decodeText(t, sent.content), "没有执行")

	require.NoError(t, runners.last().Emit(context.Background(), msgEvent("om_older", "ou_admin", "/delete 1", createdAgo(6*time.Hour))))
	require.Eventually(t, func() bool { return len(api.sent()) == 2 }, 5*time.Second, time.Millisecond)
	assert.Contains(t, decodeText(t, api.sent()[1].content), "约 6 小时前发的")

	require.NoError(t, runners.last().Emit(context.Background(), msgEvent("om_recent", "ou_admin", "/status", createdAgo(20*time.Minute))))
	require.Eventually(t, func() bool { return len(rec.got()) == 1 }, 5*time.Second, time.Millisecond, "30 分钟以内的照常执行")
	assert.Equal(t, []string{"/status"}, rec.texts())
}

func TestInbound_Whitelist(t *testing.T) {
	_, api, runners, rec := startChannel(t, testConfig)
	r := runners.last()
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_a", "ou_admin", "a")))
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_b", "ou_user", "b")))
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_c", "ou_stranger", "c")))
	require.Eventually(t, func() bool { return len(rec.got()) == 2 && len(api.sent()) == 1 }, 5*time.Second, time.Millisecond)
	assert.ElementsMatch(t, []string{"a", "b"}, rec.texts())

	// 名单外私聊：回一条带 open_id 的提示，10 分钟内只回一次
	hint := decodeText(t, api.sent()[0].content)
	assert.Contains(t, hint, "ou_stranger")
	assert.Contains(t, hint, "管理员 open_id")
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_d", "ou_stranger", "d")))
	time.Sleep(50 * time.Millisecond)
	assert.Len(t, api.sent(), 1, "10 分钟内不再提示")

	// 名单外在群里 @ 机器人：不回
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_e", "ou_stranger2", "@_user_1 /help",
		inGroup("oc_group", mention("@_user_1", "ou_bot", "pt-tools", "bot")))))
	time.Sleep(50 * time.Millisecond)
	assert.Len(t, api.sent(), 1, "群里不回提示")
	assert.Len(t, rec.got(), 2)
}

func TestInbound_EmptyListsDenyAll(t *testing.T) {
	_, api, runners, rec := startChannel(t, `{"app_id":"cli_test","app_secret":"s"}`)
	require.NoError(t, runners.last().Emit(context.Background(), msgEvent("om_1", "ou_anyone", "/help")))
	require.Eventually(t, func() bool { return len(api.sent()) == 1 }, 5*time.Second, time.Millisecond)
	assert.Empty(t, rec.got())
}

func TestInbound_GroupNeedsBotMention(t *testing.T) {
	_, _, runners, rec := startChannel(t, testConfig)
	r := runners.last()
	bot := mention("@_user_1", "ou_bot", "pt-tools", "bot")
	other := mention("@_user_2", "ou_other_bot", "别的机器人", "bot")

	require.NoError(t, r.Emit(context.Background(), msgEvent("om_g1", "ou_admin", "/status", inGroup("oc_group"))))
	// 开了「获取群组中所有消息」时，白名单用户发给另一个机器人的 /delete 也会推过来：一律忽略
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_g2", "ou_admin", "@_user_2 /delete 1", inGroup("oc_group", other))))
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_g3", "ou_admin", "@_user_1 /status", inGroup("oc_group", bot))))
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_g4", "ou_admin", "@_user_1 @_user_2 hi", inGroup("oc_group", bot, other))))
	require.Eventually(t, func() bool { return len(rec.got()) == 2 }, 5*time.Second, time.Millisecond)
	got := rec.got()
	assert.Equal(t, "/status", got[0].Text)
	assert.Equal(t, "group", got[0].MessageType)
	assert.Equal(t, "oc_group", got[0].ChatID)
	assert.Equal(t, "@别的机器人 hi", got[1].Text)
}

// 还没取到本机器人的 open_id 时，判断不了群消息有没有 @ 它：带 @ 的群消息移出去重表、回 500，让平台稍后重推，
// 取到以后重推的那次照常处理；不带 @ 的群消息不可能是给它的，照常忽略；私聊照常。
func TestInbound_GroupBeforeBotKnownAsksForRedelivery(t *testing.T) {
	c, api, runners := newTestChannel(t, testConfig)
	api.botErr = errors.New("获取机器人信息失败: code=230006")
	logs := observedLogger(c)
	rec := &chainRecorder{}
	c.OnInbound(rec.handle)
	r := runners.waitRunner(t, 1)
	r.Ready()
	require.Eventually(t, func() bool { return api.botCallCount() >= 1 }, 5*time.Second, time.Millisecond)
	require.Eventually(t, func() bool { return !c.Healthy() }, 5*time.Second, time.Millisecond, "取不到机器人信息时标异常")

	bot := mention("@_user_1", "ou_bot", "pt-tools", "bot")
	group := msgEvent("om_g", "ou_admin", "@_user_1 /status", inGroup("oc_group", bot))
	require.Error(t, r.Emit(context.Background(), group), "回 500，让平台稍后重推")
	assert.False(t, seenMessages.check("cli_test\x00om_g"), "没有记进去重表")
	// 同一条的另一份投递正好已经记进去重表（两份并发到达）：这一份仍回 500，不会被当成重复吞掉
	require.Error(t, r.Emit(context.Background(), group), "判断在去重以前")
	seenMessages.forget("cli_test\x00om_g")
	assert.Contains(t, logJoined(logs.All()), "om_g")
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_g0", "ou_admin", "/status", inGroup("oc_group"))), "没 @ 任何人的群消息照常忽略")
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_p", "ou_admin", "/help")))
	require.Eventually(t, func() bool { return len(rec.got()) == 1 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, "/help", rec.got()[0].Text, "私聊照常")

	// 取到机器人信息以后，平台重推的那次照常处理
	api.mu.Lock()
	api.botErr = nil
	api.mu.Unlock()
	require.Eventually(t, func() bool { return c.botOpenID() == "ou_bot" }, 5*time.Second, time.Millisecond)
	require.NoError(t, r.Emit(context.Background(), group))
	require.Eventually(t, func() bool { return len(rec.got()) == 2 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, "/status", rec.got()[1].Text)
}

// 机器人的 open_id 按「域名 + App ID」放在进程级的缓存里：热重载以后的新实例直接用，不再去取，
// 刚连上就能处理群里 @ 它的消息。
func TestBotInfo_CachedAcrossReload(t *testing.T) {
	c1, api1, _, _ := startChannel(t, testConfig)
	require.Equal(t, 1, api1.botCallCount())
	require.NoError(t, c1.Close(context.Background()))

	var api2 *fakeAPI
	c2, runners2, rec2 := reloadChannel(t, testConfig, func(c *Channel) {
		inner := c.newAPI
		c.newAPI = func(cfg *Config, log larkcore.Logger) apiSender {
			a := inner(cfg, log).(*fakeAPI)
			a.botErr = errors.New("不该再去取")
			api2 = a
			return a
		}
	})
	require.Eventually(t, func() bool { return c2.botOpenID() == "ou_bot" }, 5*time.Second, time.Millisecond)
	r2 := runners2.waitRunner(t, 1)
	bot := mention("@_user_1", "ou_bot", "pt-tools", "bot")
	require.NoError(t, r2.Emit(context.Background(), msgEvent("om_cached", "ou_admin", "@_user_1 /status", inGroup("oc_group", bot))))
	require.Eventually(t, func() bool { return len(rec2.got()) == 1 }, 5*time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 0, api2.botCallCount(), "有缓存时不再去取")
	assert.True(t, c2.Healthy())

	// 换了平台（Lark）就是另一个应用：没有缓存，照常去取
	c3, api3, _, _ := startChannel(t, `{"app_id":"cli_test","app_secret":"sec-ret-value","domain":"lark","admin_users":["ou_admin"]}`)
	assert.Equal(t, 1, api3.botCallCount())
	assert.Equal(t, "ou_bot", c3.botOpenID())
}

func TestInbound_IgnoresBotsAndNonText(t *testing.T) {
	_, api, runners, rec := startChannel(t, testConfig)
	r := runners.last()
	fromBot := msgEvent("om_bot", "ou_admin", "/help")
	fromBot.Event.Sender.SenderType = ptr("app")
	require.NoError(t, r.Emit(context.Background(), fromBot))
	image := msgEvent("om_img", "ou_admin", "")
	image.Event.Message.MessageType = ptr("image")
	image.Event.Message.Content = ptr(`{"image_key":"img_x"}`)
	require.NoError(t, r.Emit(context.Background(), image))
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_blank", "ou_admin", "   ")))
	for _, ev := range []*larkim.P2MessageReceiveV1{
		nil,
		{},
		{Event: &larkim.P2MessageReceiveV1Data{}},
		{Event: &larkim.P2MessageReceiveV1Data{Sender: &larkim.EventSender{SenderType: ptr("user")}, Message: &larkim.EventMessage{}}},
	} {
		require.NoError(t, r.Emit(context.Background(), ev), "字段缺失时不出错")
	}
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, rec.got())
	assert.Empty(t, api.sent())
}

func TestInbound_PerUserOrder(t *testing.T) {
	_, _, runners, rec := startChannel(t, testConfig)
	r := runners.last()
	var want []string
	for i := range 8 {
		text := "msg-" + strconv.Itoa(i)
		want = append(want, text)
		require.NoError(t, r.Emit(context.Background(), msgEvent("om_o"+strconv.Itoa(i), "ou_admin", text)))
	}
	require.Eventually(t, func() bool { return len(rec.got()) == 8 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, want, rec.texts(), "同一个人的消息按顺序处理（/delete 之后的 YES 不能跑到前面）")
}

// 队列满了：这条移出去重表、返回 error，让平台 15 秒后重推，不静默丢掉。
func TestInbound_QueueFullAsksForRedelivery(t *testing.T) {
	_, _, runners, rec := startChannel(t, testConfig)
	r := runners.last()
	rec.block = make(chan struct{})
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_q0", "ou_admin", "0")))
	require.Eventually(t, func() bool { return len(rec.got()) == 1 }, 5*time.Second, time.Millisecond, "第一条正在处理")
	for i := 1; i <= userQueueSize; i++ {
		require.NoError(t, r.Emit(context.Background(), msgEvent("om_q"+strconv.Itoa(i), "ou_admin", strconv.Itoa(i))))
	}
	full := msgEvent("om_qfull", "ou_admin", "full")
	err := r.Emit(context.Background(), full)
	require.Error(t, err, "满了回 500")
	close(rec.block)
	require.Eventually(t, func() bool { return len(rec.got()) == userQueueSize+1 }, 5*time.Second, time.Millisecond)
	require.NoError(t, r.Emit(context.Background(), full), "重推时能处理")
	require.Eventually(t, func() bool { return len(rec.got()) == userQueueSize+2 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, "full", rec.got()[userQueueSize+1].Text)
}

func TestInbound_IdleWorkerExits(t *testing.T) {
	c, _, runners, rec := startChannel(t, testConfig)
	require.NoError(t, runners.last().Emit(context.Background(), msgEvent("om_idle", "ou_admin", "x")))
	require.Eventually(t, func() bool { return len(rec.got()) == 1 }, 5*time.Second, time.Millisecond)
	require.Eventually(t, func() bool { return c.queuedUsers() == 0 }, 5*time.Second, 5*time.Millisecond, "空闲以后工作协程退出")
	require.NoError(t, runners.last().Emit(context.Background(), msgEvent("om_idle2", "ou_admin", "y")))
	require.Eventually(t, func() bool { return len(rec.got()) == 2 }, 5*time.Second, time.Millisecond, "退出以后再来消息会重新起")
}

// 不记消息正文：/addrss 会带 RSS 地址里的 passkey，名单外的人发的内容也不该进日志。
func TestInbound_LogsWithoutText(t *testing.T) {
	c, _, runners := newTestChannel(t, testConfig)
	logs := observedLogger(c)
	rec := &chainRecorder{}
	c.OnInbound(rec.handle)
	r := runners.waitRunner(t, 1)
	r.Ready()
	require.Eventually(t, func() bool { return c.botOpenID() != "" }, 5*time.Second, time.Millisecond)

	secret := "passkey=TOPSECRET123"
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_l1", "ou_admin", "/addrss https://pt.example/rss?"+secret)))
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_l2", "ou_stranger", "名单外 "+secret)))
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_l3", "ou_admin", "旧的 "+secret, createdAgo(time.Hour))))
	require.Eventually(t, func() bool { return len(rec.got()) == 1 }, 5*time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	require.NotZero(t, logs.Len())
	for _, e := range logs.All() {
		assert.NotContains(t, e.Message, "TOPSECRET123")
		for _, f := range e.Context {
			assert.NotContains(t, f.String, "TOPSECRET123")
		}
	}
	assert.True(t, strings.Contains(logJoined(logs.All()), "om_l1"), "日志里有 message_id")
	// 默认会话 ID 从这一行拿：群里 @ 机器人发一条消息，日志里有 chat_id
	assert.Contains(t, logJoined(logs.All()), "chat_id=oc_p2p_ou_admin")
}

// Close 取消进行中的处理（与 Telegram、QQ 一致）。
func TestInbound_CloseCancelsProcessing(t *testing.T) {
	c, _, runners, rec := startChannel(t, testConfig)
	rec.block = make(chan struct{}) // 一直不放行，只能靠 ctx 取消
	require.NoError(t, runners.last().Emit(context.Background(), msgEvent("om_c1", "ou_admin", "/status")))
	require.Eventually(t, func() bool { return len(rec.got()) == 1 }, 5*time.Second, time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, c.Close(ctx))
	rec.mu.Lock()
	procCtx := rec.ctxs[0]
	rec.mu.Unlock()
	assert.Error(t, procCtx.Err(), "处理用的 ctx 已取消")
}

// 关闭中的实例收到事件：回 500 让平台重推给新实例，也不记进去重表。
func TestInbound_ClosedChannelAsksForRedelivery(t *testing.T) {
	c, _, runners, _ := startChannel(t, testConfig)
	r := runners.last()
	require.NoError(t, c.Close(context.Background()))
	ev := msgEvent("om_closing", "ou_admin", "/status")
	require.Error(t, r.Emit(context.Background(), ev))
	assert.False(t, seenMessages.check("cli_test\x00om_closing"), "没有记进去重表")
}

func TestNoticeTexts(t *testing.T) {
	assert.Contains(t, expiredText(31*time.Minute), "31 分钟前")
	assert.Contains(t, expiredText(150*time.Minute), "约 3 小时前")
	assert.Contains(t, denyText("ou_x"), "ou_x")
}

func logJoined(entries []observer.LoggedEntry) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(e.Message)
		for _, f := range e.Context {
			b.WriteString(" " + f.Key + "=" + f.String)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func confOf(id uint, raw string) *models.NotificationConf {
	return &models.NotificationConf{ID: id, ChannelType: ChannelType, ConfigJSON: raw}
}

func TestInbound_HandlerErrorIsLoggedRedacted(t *testing.T) {
	c, _, runners := newTestChannel(t, testConfig)
	logs := observedLogger(c)
	done := make(chan struct{})
	c.OnInbound(func(context.Context, notify.InboundMessage) error {
		defer close(done)
		return errors.New("写库失败 sec-ret-value")
	})
	r := runners.waitRunner(t, 1)
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_err", "ou_admin", "/status")))
	<-done
	require.Eventually(t, func() bool { return strings.Contains(logJoined(logs.All()), "ChatOps 处理出错") }, 5*time.Second, time.Millisecond)
	assert.NotContains(t, logJoined(logs.All()), "sec-ret-value")
}

func TestInbound_MissingCreateTimeAndChatID(t *testing.T) {
	_, api, runners, rec := startChannel(t, testConfig)
	ev := msgEvent("om_nt", "ou_admin", "/help")
	ev.Event.Message.CreateTime = ptr("not-a-number")
	require.NoError(t, runners.last().Emit(context.Background(), ev))
	require.Eventually(t, func() bool { return len(rec.got()) == 1 }, 5*time.Second, time.Millisecond, "没有发送时间时照常处理")

	stranger := msgEvent("om_nc", "ou_nobody", "/help")
	stranger.Event.Message.ChatId = nil
	require.NoError(t, runners.last().Emit(context.Background(), stranger))
	require.Eventually(t, func() bool { return len(api.sent()) == 1 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, sentText{typ: "open_id", id: "ou_nobody", content: api.sent()[0].content}, api.sent()[0], "没有会话 ID 时直接发给这个人")
}

func createdAt(t time.Time) evOpt {
	return func(ev *larkim.P2MessageReceiveV1) {
		ev.Event.Message.CreateTime = ptr(strconv.FormatInt(t.UnixMilli(), 10))
	}
}

// 后发的那帧先进队（SDK 每帧一个 goroutine，谁先抢到锁谁先进队）：同一个窗口里到的按发出时间处理。
func TestInbound_OrdersBySendTimeWithinWindow(t *testing.T) {
	_, _, runners, rec := startChannel(t, testConfig)
	r := runners.last()
	base := time.Now().Add(-time.Minute)
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_w2", "ou_admin", "确认", createdAt(base.Add(2*time.Second)))))
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_w1", "ou_admin", "/delete 1", createdAt(base.Add(time.Second)))))
	// 没有 create_time 的按收到的时间：排在两条一分钟前发的后面
	noTime := msgEvent("om_w3", "ou_admin", "/status")
	noTime.Event.Message.CreateTime = nil
	require.NoError(t, r.Emit(context.Background(), noTime))
	require.Eventually(t, func() bool { return len(rec.got()) == 3 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, []string{"/delete 1", "确认", "/status"}, rec.texts())
}

// 处理过程中新到的消息按发出时间插进还没开始的那部分；已经开始处理的那条不受影响。
func TestInbound_InsertsBySendTimeWhileProcessing(t *testing.T) {
	_, _, runners, rec := startChannel(t, testConfig)
	r := runners.last()
	rec.block = make(chan struct{})
	base := time.Now().Add(-time.Minute)
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_p0", "ou_admin", "m0", createdAt(base.Add(10*time.Second)))))
	require.Eventually(t, func() bool { return len(rec.got()) == 1 }, 5*time.Second, time.Millisecond, "第一条正在处理")
	for _, ev := range []*larkim.P2MessageReceiveV1{
		msgEvent("om_p3", "ou_admin", "m3", createdAt(base.Add(13*time.Second))),
		msgEvent("om_p1", "ou_admin", "m1", createdAt(base.Add(11*time.Second))),
		msgEvent("om_p2", "ou_admin", "m2", createdAt(base.Add(12*time.Second))),
		// 比正在处理的那条发得还早：只能排进还没开始的那部分
		msgEvent("om_pe", "ou_admin", "early", createdAt(base)),
	} {
		require.NoError(t, r.Emit(context.Background(), ev))
	}
	close(rec.block)
	require.Eventually(t, func() bool { return len(rec.got()) == 5 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, []string{"m0", "early", "m1", "m2", "m3"}, rec.texts())
}

// reloadChannel 建同一个通道（编号 7）热重载以后的新实例：假 API、假长连接，接上新的 chain。
func reloadChannel(t *testing.T, raw string, setup func(*Channel)) (*Channel, *fakeRunners, *chainRecorder) {
	t.Helper()
	c := New()
	api := &fakeAPI{bot: botInfo{OpenID: "ou_bot"}}
	runners := &fakeRunners{}
	c.newAPI = func(*Config, larkcore.Logger) apiSender { return api }
	c.newRunner = runners.factory
	shortenTimings(c)
	if setup != nil {
		setup(c)
	}
	require.NoError(t, c.Init(context.Background(), confOf(7, raw)))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.Close(ctx)
	})
	rec := &chainRecorder{}
	c.OnInbound(rec.handle)
	return c, runners, rec
}

// 热重载：一条在处理、两条排着队。旧实例关闭时正在处理的那条随 ctx 取消，排着队的交给重建以后的实例，
// 按原来的顺序各处理一次；平台重推排着队的那条也按重复丢掉。
func TestReload_HandsQueuedMessagesToNextInstance(t *testing.T) {
	c1, _, runners1, rec1 := startChannel(t, testConfig)
	r1 := runners1.last()
	rec1.block = make(chan struct{}) // 一直不放行，只能靠 ctx 取消
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_h1", "ou_admin", "/status")))
	require.Eventually(t, func() bool { return len(rec1.got()) == 1 }, 5*time.Second, time.Millisecond, "第一条正在处理")
	queuedA := msgEvent("om_h2", "ou_admin", "/delete 1")
	require.NoError(t, r1.Emit(context.Background(), queuedA), "排着队的已经回了 200")
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_h3", "ou_admin", "确认")))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, c1.Close(ctx))
	assert.Len(t, rec1.got(), 1, "旧实例只处理了第一条")

	_, runners2, rec2 := reloadChannel(t, testConfig, nil)
	require.Eventually(t, func() bool { return len(rec2.got()) == 2 }, 5*time.Second, time.Millisecond, "新实例接着处理排着队的")
	assert.Equal(t, []string{"/delete 1", "确认"}, rec2.texts())
	r2 := runners2.waitRunner(t, 1)
	require.NoError(t, r2.Emit(context.Background(), queuedA), "平台重推的副本回 200")
	time.Sleep(50 * time.Millisecond)
	assert.Len(t, rec2.got(), 2, "重推的副本按重复丢掉，不再处理")
}

// 处理槽全被占着时，排队的那个人的工作协程在等槽。关闭以后它不能再处理（ctx 取消以后绝不再调 deliver），
// 这条交给新实例。
func TestReload_NoDeliverAfterCancelWhileWaitingForSlot(t *testing.T) {
	users := make([]string, maxHandlers+1)
	for i := range users {
		users[i] = "ou_slot" + strconv.Itoa(i)
	}
	raw := `{"app_id":"cli_test","app_secret":"sec-ret-value","admin_users":["` + strings.Join(users, `","`) + `"]}`
	c1, _, runners1, rec1 := startChannel(t, raw)
	r1 := runners1.last()
	rec1.block = make(chan struct{})
	for i := range maxHandlers {
		require.NoError(t, r1.Emit(context.Background(), msgEvent("om_s"+strconv.Itoa(i), users[i], "busy")))
	}
	require.Eventually(t, func() bool { return len(rec1.got()) == maxHandlers }, 5*time.Second, time.Millisecond, "处理槽全被占着")
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_waiting", users[maxHandlers], "/status")))
	time.Sleep(50 * time.Millisecond)
	assert.Len(t, rec1.got(), maxHandlers, "第 17 个人在等处理槽")

	require.NoError(t, c1.Close(context.Background()))
	time.Sleep(20 * time.Millisecond)
	assert.Len(t, rec1.got(), maxHandlers, "关闭以后旧实例不再处理")

	_, _, rec2 := reloadChannel(t, raw, nil)
	require.Eventually(t, func() bool { return len(rec2.got()) == 1 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, users[maxHandlers], rec2.got()[0].ChannelUserID)
}

// Close 在队列锁里取消 ctx：drain 在同一把锁里看 ctx 才取出消息，「取出」与「取消」因此有确定的先后，
// 不会有一条消息在检查 ctx 以后、交给 MessageChain 以前被取消，既不算正在处理、也没留给交接。
func TestClose_CancelsUnderQueueLock(t *testing.T) {
	c, _, _, _ := startChannel(t, testConfig)
	c.queues.mu.Lock()
	var unlock sync.Once
	defer unlock.Do(c.queues.mu.Unlock) // 断言失败时也放开，免得收尾的 seal 一直等锁
	closed := make(chan error, 1)
	go func() { closed <- c.Close(context.Background()) }()
	// 先确认 Close 已经开始（标了关闭），再看它在锁放开以前不取消；否则 Close 没被调度到时旧实现也会误通过
	require.Eventually(t, c.closed.Load, 5*time.Second, time.Millisecond, "Close 已经开始")
	require.Never(t, func() bool { return c.ctx.Err() != nil }, 100*time.Millisecond, 5*time.Millisecond,
		"拿着队列锁时 Close 不能取消 ctx")
	unlock.Do(c.queues.mu.Unlock)
	require.Eventually(t, func() bool { return c.ctx.Err() != nil }, 5*time.Second, time.Millisecond)
	require.NoError(t, <-closed)
}

// 留下的消息只交给通道编号相同的实例；过了时限没人取就丢掉，记一条 Warn 写明几条。
func TestReload_LeftoversOnlyForSameConfAndExpire(t *testing.T) {
	c1, _, runners1, rec1 := startChannel(t, testConfig)
	apps.mu.Lock()
	apps.leftTTL = 200 * time.Millisecond
	apps.mu.Unlock()
	logs := observedLogger(c1)
	r1 := runners1.last()
	rec1.block = make(chan struct{})
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_l1", "ou_admin", "/status")))
	require.Eventually(t, func() bool { return len(rec1.got()) == 1 }, 5*time.Second, time.Millisecond)
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_l2", "ou_admin", "/sites")))
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_l3", "ou_user", "/help")))
	require.NoError(t, c1.Close(context.Background()))
	assert.Contains(t, logJoined(logs.All()), "关闭时还有 2 条排队的消息没开始处理")

	// 同一个应用换了通道（编号 8）：不给它
	other := New()
	api := &fakeAPI{bot: botInfo{OpenID: "ou_bot"}}
	runners := &fakeRunners{}
	other.newAPI = func(*Config, larkcore.Logger) apiSender { return api }
	other.newRunner = runners.factory
	shortenTimings(other)
	require.NoError(t, other.Init(context.Background(), confOf(8, testConfig)))
	defer func() { _ = other.Close(context.Background()) }()
	rec := &chainRecorder{}
	other.OnInbound(rec.handle)
	runners.waitRunner(t, 1)

	require.Eventually(t, func() bool { return strings.Contains(logJoined(logs.All()), "没有重建的通道接手") }, 5*time.Second, 5*time.Millisecond)
	assert.Contains(t, logJoined(logs.All()), "留下的 2 条排队的消息")
	assert.Empty(t, rec.got(), "别的通道编号拿不到")
	apps.mu.Lock()
	assert.Empty(t, apps.left, "过期的已经丢掉")
	apps.mu.Unlock()
}

// 交接不受每人 8 条的上限限制；名单在这次重建里改过、已经不在名单里的人留下的不再执行。
func TestReload_AdoptIgnoresQueueLimitAndRechecksLists(t *testing.T) {
	resetProcessState(t)
	owner := &Channel{confID: 7}
	_, ok, _, _ := apps.claim("cli_test", owner)
	require.True(t, ok)
	base := time.Now().Add(-time.Minute)
	var mine, gone []*queued
	for i := range userQueueSize + 3 {
		in := notify.InboundMessage{ChannelType: ChannelType, SourceConfID: 7, ChannelUserID: "ou_admin", Text: "m" + strconv.Itoa(i)}
		mine = append(mine, newQueued(in, base.Add(time.Duration(i)*time.Second), "om_a"+strconv.Itoa(i)))
	}
	gone = append(gone, newQueued(notify.InboundMessage{ChannelType: ChannelType, SourceConfID: 7, ChannelUserID: "ou_removed", Text: "/delete 1"}, base, "om_gone"))
	apps.release("cli_test", owner, handoff{items: map[string][]*queued{"ou_admin": mine, "ou_removed": gone}})

	var logs *observer.ObservedLogs
	_, _, rec := reloadChannel(t, testConfig, func(c *Channel) { logs = observedLogger(c) })
	require.Eventually(t, func() bool { return len(rec.got()) == userQueueSize+3 }, 5*time.Second, time.Millisecond)
	var want []string
	for i := range userQueueSize + 3 {
		want = append(want, "m"+strconv.Itoa(i))
	}
	assert.Equal(t, want, rec.texts())
	time.Sleep(20 * time.Millisecond)
	for _, m := range rec.got() {
		assert.NotEqual(t, "ou_removed", m.ChannelUserID, "不在名单里的人留下的不执行")
	}
	joined := logJoined(logs.All())
	assert.Contains(t, joined, "接手上一个实例留下的 11 条消息")
	assert.Contains(t, joined, "1 条消息的发送人已经不在名单里")
}

// 交出排队的消息以后（seal）不再收：onMessage 拿到 false 会把这条移出去重表、回 500，让平台重推给新实例。
// 通道正在关闭时新收到的也不收；接手上一个实例留下的照样放进队列（不起工作协程），等 seal 再交出去。
func TestEnqueue_RefusesAfterSealOrClose(t *testing.T) {
	c, _, _ := newTestChannel(t, testConfig)
	in := notify.InboundMessage{ChannelType: ChannelType, SourceConfID: 7, ChannelUserID: "ou_admin", Text: "x"}
	c.queues.seal()
	assert.False(t, c.enqueue("ou_admin", newQueued(in, time.Now(), "om_e1"), false))
	assert.False(t, c.enqueue("ou_admin", newQueued(in, time.Now(), "om_e2"), true), "seal 以后接手的也不收")

	closing, _, _ := newTestChannel(t, testConfig)
	closing.goMu.Lock()
	closing.stopping = true // Close 已经开始、ctx 还没取消的那一刻
	closing.goMu.Unlock()
	assert.False(t, closing.enqueue("ou_admin", newQueued(in, time.Now(), "om_e3"), false), "正在关闭：新收到的不收")
	assert.Equal(t, 0, closing.queuedUsers(), "不收的不留空队列")
	assert.True(t, closing.enqueue("ou_admin", newQueued(in, time.Now(), "om_e4"), true), "接手的照样放进队列")
	left := closing.queues.seal()
	require.Len(t, left.items["ou_admin"], 1, "seal 交出来")
	assert.Equal(t, "om_e4", left.items["ou_admin"][0].messageID)
}

func TestWaitClaim(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.False(t, waitClaim(ctx, nil, time.Hour), "ctx 取消时不再等")
	wake := make(chan struct{})
	close(wake)
	assert.True(t, waitClaim(context.Background(), wake, time.Hour), "释放时马上醒")
	assert.True(t, waitClaim(context.Background(), nil, time.Millisecond), "定时重试兜底")
}

// stubbornHandler 是不理会取消的处理函数（像 qB 慢时的 GetAllTorrents，用的是 context.Background 加超时）：
// 文字为 slow 的那条等 release 关闭才返回。started 在它开始时关闭，ended 记下它结束的时刻。
type stubbornHandler struct {
	release chan struct{}
	started chan struct{}
	once    sync.Once
	ended   atomic.Int64
}

func newStubbornHandler() *stubbornHandler {
	return &stubbornHandler{release: make(chan struct{}), started: make(chan struct{})}
}

func (s *stubbornHandler) handle(_ context.Context, msg notify.InboundMessage) error {
	if msg.Text == "slow" {
		s.once.Do(func() { close(s.started) })
		<-s.release
		s.ended.Store(time.Now().UnixNano())
	}
	return nil
}

// 热重载时旧实例里那条不理会取消、还在跑：交接要等它收尾，新实例才处理同一个人排在后面的消息，两条不同时跑。
func TestReload_WaitsForInflightBeforeHandoff(t *testing.T) {
	c1, _, runners1, _ := startChannel(t, testConfig)
	h := newStubbornHandler()
	c1.OnInbound(h.handle)
	r1 := runners1.last()
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_w1", "ou_admin", "slow")))
	select {
	case <-h.started:
	case <-time.After(5 * time.Second):
		t.Fatal("slow 没开始处理")
	}
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_w2", "ou_admin", "/status")))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.Error(t, c1.Close(ctx), "处理函数不理会取消，Close 到时限返回")
	_, _, rec2 := reloadChannel(t, testConfig, nil)
	require.Never(t, func() bool { return len(rec2.got()) > 0 }, 200*time.Millisecond, 5*time.Millisecond,
		"旧实例那条没收尾以前，新实例不处理同一个人的下一条")

	close(h.release)
	require.Eventually(t, func() bool { return len(rec2.got()) == 1 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, "/status", rec2.got()[0].Text)
	assert.NotZero(t, h.ended.Load(), "旧实例那条先收尾")
}

// 等正在处理的那条收尾有上限（handoffWait）：过了上限照常交接，记一条 Warn；新实例马上处理别人的消息，
// 这个人后面的消息等旧实例里他那条收尾以后才处理，两条不同时跑。
func TestReload_HandoffWaitIsBoundedButUserStaysSerial(t *testing.T) {
	c1, _, runners1, _ := startChannel(t, testConfig)
	c1.handoffWait = 50 * time.Millisecond
	logs := observedLogger(c1)
	h := newStubbornHandler()
	c1.OnInbound(h.handle)
	r1 := runners1.last()
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_b1", "ou_admin", "slow")))
	select {
	case <-h.started:
	case <-time.After(5 * time.Second):
		t.Fatal("slow 没开始处理")
	}
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_b2", "ou_admin", "/status")))
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_b3", "ou_user", "/sites")))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.Error(t, c1.Close(ctx))
	_, _, rec2 := reloadChannel(t, testConfig, nil)
	require.Eventually(t, func() bool { return len(rec2.got()) == 1 }, 5*time.Second, time.Millisecond,
		"过了上限照常交接：别人的消息马上处理")
	assert.Equal(t, "/sites", rec2.got()[0].Text)
	require.Never(t, func() bool { return len(rec2.got()) > 1 }, 150*time.Millisecond, 5*time.Millisecond,
		"旧实例里 ou_admin 那条没收尾，他后面的消息还在等")
	assert.Contains(t, logJoined(logs.All()), "内没收尾")

	close(h.release)
	require.Eventually(t, func() bool { return len(rec2.got()) == 2 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, "/status", rec2.got()[1].Text)
	assert.NotZero(t, h.ended.Load(), "旧实例那条先收尾")
}

// 上一个实例里那条一直不收尾时，这个人后面的消息一直等它（不设上限，与同一个实例里的保证一样）；
// 完成信号经过再一次热重载也照样传下去，信号也不随留下的消息一起过期。
func TestReload_BusySignalSurvivesReloadsAndTTL(t *testing.T) {
	c1, _, runners1, _ := startChannel(t, testConfig)
	c1.handoffWait = 20 * time.Millisecond
	h := newStubbornHandler()
	c1.OnInbound(h.handle)
	r1 := runners1.last()
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_d1", "ou_admin", "slow")))
	select {
	case <-h.started:
	case <-time.After(5 * time.Second):
		t.Fatal("slow 没开始处理")
	}
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_d2", "ou_admin", "/status")))
	apps.mu.Lock()
	apps.leftTTL = 50 * time.Millisecond
	apps.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.Error(t, c1.Close(ctx))
	// 留下的消息过期以后才来接手：消息丢掉，完成信号还在
	time.Sleep(150 * time.Millisecond)
	c2, _, rec2 := reloadChannel(t, testConfig, nil)
	require.NoError(t, c2.Close(context.Background()), "第二次热重载")
	_, runners3, rec3 := reloadChannel(t, testConfig, nil)
	r3 := runners3.waitRunner(t, 1)
	got := func() []notify.InboundMessage { return append(rec2.got(), rec3.got()...) }
	require.NoError(t, r3.Emit(context.Background(), msgEvent("om_d3", "ou_admin", "/sites")))
	require.Never(t, func() bool { return len(got()) > 0 }, 300*time.Millisecond, 10*time.Millisecond,
		"旧实例那条没收尾以前，这个人的消息一直等着")

	close(h.release)
	require.Eventually(t, func() bool { return len(got()) == 1 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, "/sites", got()[0].Text, "过期的那条丢掉，新来的处理")
}

// 关闭中 SDK 还调进来的消息：照常排进交接队列，回 500；新实例按发出时间把它和关闭前排着的一起处理，
// 平台重推的副本按去重表丢掉。
func TestReload_ClosingCallbackJoinsHandoff(t *testing.T) {
	c1, _, runners1, _ := startChannel(t, testConfig)
	h := newStubbornHandler()
	c1.OnInbound(h.handle)
	r1 := runners1.last()
	base := time.Now().Add(-time.Minute)
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_k0", "ou_admin", "slow", createdAt(base))))
	select {
	case <-h.started:
	case <-time.After(5 * time.Second):
		t.Fatal("slow 没开始处理")
	}
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_k2", "ou_admin", "确认", createdAt(base.Add(2*time.Second)))))

	closed := make(chan error, 1)
	go func() { closed <- c1.Close(context.Background()) }()
	require.Eventually(t, c1.closed.Load, 5*time.Second, time.Millisecond)
	late := msgEvent("om_k1", "ou_admin", "/delete 1", createdAt(base.Add(time.Second)))
	require.ErrorIs(t, r1.Emit(context.Background(), late), errClosing, "关闭中仍回 500，平台保留重推")
	close(h.release) // 旧实例那条收尾，交接
	require.NoError(t, <-closed)

	_, runners2, rec2 := reloadChannel(t, testConfig, nil)
	require.Eventually(t, func() bool { return len(rec2.got()) == 2 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, []string{"/delete 1", "确认"}, rec2.texts(), "按发出时间处理")
	r2 := runners2.waitRunner(t, 1)
	require.NoError(t, r2.Emit(context.Background(), late), "平台重推的副本回 200")
	time.Sleep(50 * time.Millisecond)
	assert.Len(t, rec2.got(), 2, "重推的副本按重复丢掉")
}

// 关闭中排进来、回了 500 的消息没人接手、过期时，去重记录删掉：平台之后的重推能照常处理。
func TestReload_UnackedForgottenOnExpire(t *testing.T) {
	c1, _, runners1, _ := startChannel(t, testConfig)
	apps.mu.Lock()
	apps.leftTTL = 50 * time.Millisecond
	apps.mu.Unlock()
	h := newStubbornHandler()
	c1.OnInbound(h.handle)
	r1 := runners1.last()
	require.NoError(t, r1.Emit(context.Background(), msgEvent("om_u0", "ou_admin", "slow")))
	select {
	case <-h.started:
	case <-time.After(5 * time.Second):
		t.Fatal("slow 没开始处理")
	}
	closed := make(chan error, 1)
	go func() { closed <- c1.Close(context.Background()) }()
	require.Eventually(t, c1.closed.Load, 5*time.Second, time.Millisecond)
	require.ErrorIs(t, r1.Emit(context.Background(), msgEvent("om_u1", "ou_admin", "/status")), errClosing)
	close(h.release)
	require.NoError(t, <-closed)
	require.Eventually(t, func() bool { return !seenMessages.check("cli_test\x00om_u1") }, 5*time.Second, 10*time.Millisecond,
		"过期以后去重记录删掉")
}

// 取出一条以后、交给 MessageChain 以前 ctx 被取消（Close 正好落在这里）：这条放回队首交接，不在旧实例里处理。
func TestDrain_PutsBackWhenCancelledAfterTake(t *testing.T) {
	c1, _, runners1, rec1 := startChannel(t, testConfig)
	c1.afterTake = func() {
		c1.queues.mu.Lock() // 与 Close 一样在队列锁里取消
		c1.cancel()
		c1.queues.mu.Unlock()
	}
	require.NoError(t, runners1.last().Emit(context.Background(), msgEvent("om_pb", "ou_admin", "/status")))
	require.Never(t, func() bool { return len(rec1.got()) > 0 }, 100*time.Millisecond, 5*time.Millisecond,
		"取消以后旧实例不处理")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, c1.Close(ctx))

	_, _, rec2 := reloadChannel(t, testConfig, nil)
	require.Eventually(t, func() bool { return len(rec2.got()) == 1 }, 5*time.Second, time.Millisecond, "交给新实例")
	assert.Equal(t, "/status", rec2.got()[0].Text)
	time.Sleep(50 * time.Millisecond)
	assert.Len(t, rec2.got(), 1, "只处理一次")
}

// 队列不空时到的帧也要等过 gather 才取：前一条刚处理完，比它先发、晚到几毫秒的那条仍排在它前面。
func TestInbound_OrdersAcrossBusyQueueGap(t *testing.T) {
	c, _, runners, rec := startChannel(t, testConfig)
	c.gather = 200 * time.Millisecond
	rec.block = make(chan struct{})
	r := runners.last()
	base := time.Now().Add(-time.Minute)
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_q0", "ou_admin", "m0", createdAt(base))))
	require.Eventually(t, func() bool { return len(rec.got()) == 1 }, 5*time.Second, time.Millisecond, "m0 正在处理")
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_q1", "ou_admin", "m1", createdAt(base.Add(time.Second)))))
	time.Sleep(300 * time.Millisecond) // m1 早就等够了 gather
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_q3", "ou_admin", "确认", createdAt(base.Add(3*time.Second)))))
	close(rec.block) // m0 收尾，m1 马上处理；「确认」刚进队，还要等 gather
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_q2", "ou_admin", "/delete 1", createdAt(base.Add(2*time.Second)))))
	require.Eventually(t, func() bool { return len(rec.got()) == 4 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, []string{"m0", "m1", "/delete 1", "确认"}, rec.texts())
}

// 处理槽被别人占满时，一个人的工作协程在等槽；等槽期间排到队首的新消息也要等够 gather 才取，
// 比它先发、晚到几毫秒的那条仍排在它前面。
func TestInbound_GatherHoldsAfterWaitingForSlot(t *testing.T) {
	users := make([]string, maxHandlers)
	for i := range users {
		users[i] = "ou_slot" + strconv.Itoa(i)
	}
	raw := `{"app_id":"cli_test","app_secret":"sec-ret-value","admin_users":["ou_x","` + strings.Join(users, `","`) + `"]}`
	c, _, runners, _ := startChannel(t, raw)
	c.gather = 100 * time.Millisecond
	release := make(chan struct{})
	defer close(release)
	started := make(chan struct{}, maxHandlers)
	var mu sync.Mutex
	var order []string
	c.OnInbound(func(_ context.Context, msg notify.InboundMessage) error {
		if msg.Text == "busy" {
			started <- struct{}{}
			<-release
			return nil
		}
		mu.Lock()
		order = append(order, msg.Text)
		mu.Unlock()
		return nil
	})
	got := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), order...)
	}
	r := runners.last()
	for i, u := range users {
		require.NoError(t, r.Emit(context.Background(), msgEvent("om_busy"+strconv.Itoa(i), u, "busy")))
	}
	for range users {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("处理槽没被占满")
		}
	}
	base := time.Now().Add(-time.Minute)
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_xc", "ou_x", "C", createdAt(base.Add(3*time.Second)))))
	time.Sleep(150 * time.Millisecond) // C 早就等够了 gather，ou_x 的工作协程在等处理槽
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_xb", "ou_x", "B", createdAt(base.Add(2*time.Second)))))
	release <- struct{}{} // 放开一个处理槽
	time.Sleep(5 * time.Millisecond)
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_xa", "ou_x", "A", createdAt(base.Add(time.Second)))))
	require.Eventually(t, func() bool { return len(got()) == 3 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, []string{"A", "B", "C"}, got())
}
