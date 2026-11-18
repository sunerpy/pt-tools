package feishuapp

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
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

func TestInbound_GroupIgnoredUntilBotKnown(t *testing.T) {
	c, api, runners := newTestChannel(t, testConfig)
	api.botErr = errors.New("获取机器人信息失败: code=230006")
	rec := &chainRecorder{}
	c.OnInbound(rec.handle)
	r := runners.waitRunner(t, 1)
	r.Ready()
	require.Eventually(t, func() bool { return api.botCallCount() >= 1 }, 5*time.Second, time.Millisecond)
	require.Eventually(t, func() bool { return !c.Healthy() }, 5*time.Second, time.Millisecond, "取不到机器人信息时标异常")

	bot := mention("@_user_1", "ou_bot", "pt-tools", "bot")
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_g", "ou_admin", "@_user_1 /status", inGroup("oc_group", bot))))
	require.NoError(t, r.Emit(context.Background(), msgEvent("om_p", "ou_admin", "/help")))
	require.Eventually(t, func() bool { return len(rec.got()) == 1 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, "/help", rec.got()[0].Text, "私聊照常")
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
	apps.release("cli_test", owner, map[string][]*queued{"ou_admin": mine, "ou_removed": gone})

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
