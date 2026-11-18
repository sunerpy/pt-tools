package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/web"
)

// qqLikeChannel 照 QQ 适配器的规则收件：Targets 里没有 chat_id、也没有 UserID 时报同一个错，
// 有收件人时记下发出的通知。
type qqLikeChannel struct {
	mu   sync.Mutex
	sent []notify.Notification
}

func (c *qqLikeChannel) Type() string                                         { return "qq_onebot" }
func (c *qqLikeChannel) Init(context.Context, *models.NotificationConf) error { return nil }
func (c *qqLikeChannel) SupportsInbound() bool                                { return true }
func (c *qqLikeChannel) OnInbound(notify.InboundHandler)                      {}
func (c *qqLikeChannel) Close(context.Context) error                          { return nil }
func (c *qqLikeChannel) Healthy() bool                                        { return true }

func (c *qqLikeChannel) Send(_ context.Context, n notify.Notification) error {
	if n.Targets["chat_id"] == "" && n.UserID == "" {
		return errors.New("QQ 适配器: 缺少目标 chat_id")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, n)
	return nil
}

func newDeliveryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	crypto.ResetForTest()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.NotificationConf{}, &models.ChannelBinding{}, &models.NotificationOutbox{},
		&models.MonitorNotificationLog{}, &models.ActionAudit{}))
	return db
}

// 2026-10-28 用户反馈：每日战报经 QQ 通道发送失败「QQ 适配器: 缺少目标 chat_id」，测试消息却能发；
// 而且失败了在操作审计里看不到。生产的整条路径：MonitorNotifier → loginReminderSender → NotificationService.PushSync
// → liveNotifyManager → 通道。现在发给第一个管理员 QQ（与测试消息相同），每次投递写一条审计。
func TestDailyReportReachesQQAdminAndIsAudited(t *testing.T) {
	db := newDeliveryTestDB(t)
	cipher, err := crypto.Encrypt([]byte(`{"listen_addr":"127.0.0.1:6700","access_token":"t","admin_qq_users":[454196976]}`))
	require.NoError(t, err)
	conf := models.NotificationConf{ChannelType: "qq_onebot", Name: "qq", ConfigJSON: cipher, Enabled: true}
	require.NoError(t, db.Create(&conf).Error)

	ch := &qqLikeChannel{}
	live := newLiveNotifyManager(map[uint]notify.Channel{conf.ID: ch})
	svc := app.NewNotificationService(db, live, 5*time.Second)
	audits := app.NewAuditService(db)
	bs := &chatopsBootstrap{deps: &web.ChatOpsDeps{NotificationSvc: svc, AuditSvc: audits}}
	notifier := scheduler.NewMonitorNotifier(db, loginReminderSender(bs), nil, zap.NewNop().Sugar())
	notifier.SetAuditor(monitorDeliveryAudit{svc: audits})

	_, err = notifier.Enqueue(context.Background(), scheduler.MonitorNotifyEntry{
		Source: "daily_report", Subject: "*", Kind: "daily", EventKey: "2026-10-28",
		Title: "每日战报 2026-10-28", Text: "今日上传 12 GB",
	})
	require.NoError(t, err)
	require.Equal(t, 1, notifier.DeliverDue(context.Background()))

	require.Len(t, ch.sent, 1)
	assert.Equal(t, "454196976", ch.sent[0].Targets["chat_id"])
	assert.Equal(t, "private", ch.sent[0].Targets["message_type"], "QQ 号是私聊，不能按群发")

	var row models.MonitorNotificationLog
	require.NoError(t, db.First(&row).Error)
	assert.Equal(t, models.MonitorNotifySent, row.Result)

	items, total, err := audits.Query(context.Background(), app.AuditQuery{})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, "notify:daily_report:daily", items[0].Command)
	assert.Equal(t, "qq_onebot", items[0].ChannelType)
	assert.Equal(t, "success", items[0].Result)
	assert.Empty(t, items[0].ChannelUserID, "系统发的通知没有触发用户")
	var args map[string]any
	require.NoError(t, json.Unmarshal([]byte(items[0].ArgsJSON), &args))
	assert.Equal(t, "每日战报 2026-10-28", args["title"])
	assert.EqualValues(t, 1, args["attempt"])
	assert.NotContains(t, args, "subject", "「*」是全部站点，不写")
}

// 没有收件人时：失败原因说清楚怎么补，写进审计（每次重试一条），telego 带出的 bot token 不进审计。
func TestNotificationFailureReasonIsAuditedWithoutSecrets(t *testing.T) {
	db := newDeliveryTestDB(t)
	empty, err := crypto.Encrypt([]byte(`{"listen_addr":"127.0.0.1:6700"}`))
	require.NoError(t, err)
	qq := models.NotificationConf{ChannelType: "qq_onebot", Name: "qq", ConfigJSON: empty, Enabled: true}
	require.NoError(t, db.Create(&qq).Error)

	live := newLiveNotifyManager(map[uint]notify.Channel{qq.ID: &qqLikeChannel{}})
	svc := app.NewNotificationService(db, live, 5*time.Second)
	audits := app.NewAuditService(db)
	bs := &chatopsBootstrap{deps: &web.ChatOpsDeps{NotificationSvc: svc, AuditSvc: audits}}
	notifier := scheduler.NewMonitorNotifier(db, loginReminderSender(bs), nil, zap.NewNop().Sugar())
	notifier.SetAuditor(monitorDeliveryAudit{svc: audits})
	_, err = notifier.Enqueue(context.Background(), scheduler.MonitorNotifyEntry{
		Source: "attendance", Subject: "hdsky", Kind: "daily", EventKey: "2026-10-28", Title: "签到结果", Text: "x",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, notifier.DeliverDue(context.Background()))

	items, _, err := audits.Query(context.Background(), app.AuditQuery{Result: "error"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "notify:attendance:daily", items[0].Command)
	assert.Equal(t, "error:send_failed", items[0].Result)
	var args map[string]any
	require.NoError(t, json.Unmarshal([]byte(items[0].ArgsJSON), &args))
	assert.Contains(t, args["error"], "QQ 通道无可用收件人")
	assert.Equal(t, "hdsky", args["subject"])
	assert.NotEmpty(t, args["next_retry_at"], "第一次失败还会重试")

	// 审计里的错误遮掉凭证
	monitorDeliveryAudit{svc: audits}.RecordDelivery(context.Background(), scheduler.MonitorDelivery{
		Source: "login", Kind: "tier", ConfID: 9, ChannelType: "telegram", Attempt: 4, Final: true,
		Err: errors.New(`Post "https://api.telegram.org/bot1234567890:AAH_kx-Yz9Q8wE7rT6yU5iO4pA3sD2fG1hJ/sendMessage": i/o timeout`),
	})
	items, _, err = audits.Query(context.Background(), app.AuditQuery{Command: "notify:login:tier"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.NotContains(t, items[0].ArgsJSON, "AAH_kx")
	assert.Contains(t, items[0].ArgsJSON, `"final":true`)
}
