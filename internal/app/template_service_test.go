package app

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/tmpl"
	"github.com/sunerpy/pt-tools/models"
)

func newTemplateSvc(t *testing.T) (TemplateService, *notify.Templates, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.NotificationTemplate{}))
	set := notify.NewTemplates(nil)
	return NewTemplateService(db, set), set, db
}

func sampleNotification(typ string) notify.Notification {
	ev, title, text := notify.SampleEvent(typ)
	return notify.Notification{Event: ev, Title: title, Text: text}
}

func TestTemplateServiceSaveApplyReset(t *testing.T) {
	svc, set, db := newTemplateSvc(t)
	ctx := context.Background()

	cat, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, cat.Events, len(notify.Events))
	assert.NotEmpty(t, cat.CommonVars)
	for _, e := range cat.Events {
		assert.False(t, e.Customized, e.Type)
	}

	v, err := svc.Save(ctx, notify.EventRSSMatched, TemplateInput{Text: "{{.site}}：{{.torrent_title}}（{{.size}}）"})
	require.NoError(t, err)
	assert.True(t, v.Customized)
	assert.NotNil(t, v.UpdatedAt)
	n := sampleNotification(notify.EventRSSMatched)
	got := set.Apply(n)
	assert.Equal(t, n.Title, got.Title, "只配了正文")
	assert.Equal(t, "hdtime：Fake.Movie.2026.1080p.WEB-DL.H.264-FAKE（15.00 GB）", got.Text)

	// 再存一次是更新，不是新增
	_, err = svc.Save(ctx, notify.EventRSSMatched, TemplateInput{Title: "[{{.site}}]", Text: "{{.rule}}"})
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.Model(&models.NotificationTemplate{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	got = set.Apply(n)
	assert.Equal(t, "[hdtime]", got.Title)
	assert.Equal(t, "只要免费", got.Text)

	cat, err = svc.List(ctx)
	require.NoError(t, err)
	for _, e := range cat.Events {
		assert.Equal(t, e.Type == notify.EventRSSMatched, e.Customized, e.Type)
	}

	require.NoError(t, svc.Reset(ctx, notify.EventRSSMatched))
	assert.Equal(t, n, set.Apply(n), "恢复默认以后照内置文字发")
	require.NoError(t, db.Model(&models.NotificationTemplate{}).Count(&count).Error)
	assert.EqualValues(t, 0, count)

	// 两项都清空等于恢复默认
	_, err = svc.Save(ctx, notify.EventTest, TemplateInput{Text: "{{.channel}}"})
	require.NoError(t, err)
	v, err = svc.Save(ctx, notify.EventTest, TemplateInput{})
	require.NoError(t, err)
	assert.False(t, v.Customized)
	require.NoError(t, db.Model(&models.NotificationTemplate{}).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestTemplateServiceRejectsBadTemplates(t *testing.T) {
	svc, set, db := newTemplateSvc(t)
	ctx := context.Background()

	_, err := svc.Save(ctx, notify.EventRSSMatched, TemplateInput{Text: "{{.torrent_titel}}"})
	require.ErrorIs(t, err, tmpl.ErrInvalid)
	assert.Contains(t, err.Error(), "正文")
	assert.Contains(t, err.Error(), "没有这个变量：torrent_titel")
	_, err = svc.Save(ctx, notify.EventRSSMatched, TemplateInput{Title: "{{range .size_bytes}}x{{end}}"})
	require.ErrorIs(t, err, tmpl.ErrInvalid)
	assert.Contains(t, err.Error(), "标题")
	_, err = svc.Save(ctx, "nope", TemplateInput{Text: "x"})
	assert.ErrorIs(t, err, ErrUnknownEvent)
	assert.ErrorIs(t, svc.Reset(ctx, "nope"), ErrUnknownEvent)
	var count int64
	require.NoError(t, db.Model(&models.NotificationTemplate{}).Count(&count).Error)
	assert.EqualValues(t, 0, count, "不合格的不写库")
	n := sampleNotification(notify.EventRSSMatched)
	assert.Equal(t, n, set.Apply(n))
}

func TestTemplateServicePreviewAndLoad(t *testing.T) {
	svc, set, db := newTemplateSvc(t)
	ctx := context.Background()

	p, err := svc.Preview(notify.EventTest, TemplateInput{Text: "通道 {{.channel}} 正常"})
	require.NoError(t, err)
	assert.Equal(t, "pt-tools 测试通知", p.Title, "没写的一项给内置文字的示例")
	assert.Equal(t, "通道 我的 Telegram 正常", p.Text)
	_, err = svc.Preview(notify.EventTest, TemplateInput{Text: "{{.nope}}"})
	assert.ErrorIs(t, err, tmpl.ErrInvalid)

	// 启动时读库：合格的生效，按现在的规则不合格的当没有
	require.NoError(t, db.Create(&models.NotificationTemplate{EventType: notify.EventTest, Text: "已读入 {{.channel}}"}).Error)
	require.NoError(t, db.Create(&models.NotificationTemplate{EventType: notify.EventRSSMatched, Text: "{{range .seeders}}x{{end}}"}).Error)
	require.NoError(t, svc.Load(ctx))
	assert.Equal(t, "已读入 我的 Telegram", set.Apply(sampleNotification(notify.EventTest)).Text)
	m := sampleNotification(notify.EventRSSMatched)
	assert.Equal(t, m, set.Apply(m))
}

// 命令回复与助手回复只有正文：填了标题被拒；正文按回复的上限校验，保存以后 ApplyText 用得上；目录里带着组。
func TestTemplateServiceReplyEvents(t *testing.T) {
	svc, set, _ := newTemplateSvc(t)
	ctx := context.Background()

	cat, err := svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, cat.Groups, 3)
	assert.Equal(t, notify.GroupNotify, cat.Groups[0].Key)
	groups := map[string]int{}
	for _, e := range cat.Events {
		groups[e.Group]++
	}
	assert.Positive(t, groups[notify.GroupReply])
	assert.Equal(t, 1, groups[notify.GroupAssistant])

	_, err = svc.Save(ctx, notify.EventReplyStatus, TemplateInput{Title: "x", Text: "{{.text}}"})
	assert.ErrorIs(t, err, tmpl.ErrInvalid)
	assert.ErrorContains(t, err, "只有正文，没有标题")
	_, err = svc.Preview(notify.EventReplyStatus, TemplateInput{Text: "{{.torrent_title}}"})
	assert.ErrorContains(t, err, "没有这个变量：torrent_title", "通知才有的变量在回复里查得出来")

	p, err := svc.Preview(notify.EventReplyStatus, TemplateInput{Text: "{{range .items}}{{.name}};{{end}}"})
	require.NoError(t, err)
	assert.Equal(t, "", p.Title)
	assert.Equal(t, "家里的 qB;NAS 的 TR;", p.Text)

	v, err := svc.Save(ctx, notify.EventAssistantReply, TemplateInput{Text: "🤖 {{.text}}"})
	require.NoError(t, err)
	assert.True(t, v.Customized)
	assert.True(t, v.NoTitle)
	ev, _, text := notify.SampleEvent(notify.EventAssistantReply)
	assert.Equal(t, "🤖 "+text, set.ApplyText(ev, text))
}
