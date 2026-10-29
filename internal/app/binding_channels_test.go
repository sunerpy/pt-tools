package app

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
)

func setupBindingChannelsDB(t *testing.T) *gorm.DB {
	t.Helper()
	setupTestKey(t)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.NotificationConf{}, &models.ChannelBinding{}, &models.ActionAudit{}, &models.BotToken{}))
	return db
}

func createConf(t *testing.T, db *gorm.DB, typ, name, config string) models.NotificationConf {
	t.Helper()
	cipher, err := crypto.Encrypt([]byte(config))
	require.NoError(t, err)
	row := models.NotificationConf{ChannelType: typ, Name: name, ConfigJSON: cipher, Enabled: true}
	require.NoError(t, db.Create(&row).Error)
	return row
}

// 生产上的样子（2026-10-28）：Telegram 机器人在线、通知发到默认 Chat ID，账号在 10-21 发过 /unbind。
// 绑定页要能看出：名单里有这个账号、它没有绑定、最近一次是 /unbind（之后的 /help 被拒），通知照常发到默认 Chat ID。
func TestListChannels_ShowsWhitelistBindingAndLastActivity(t *testing.T) {
	db := setupBindingChannelsDB(t)
	tg := createConf(t, db, "telegram", "pt-tools-bot", `{"bot_token":"t","admin_users":[1234567890],"allowed_users":[555],"default_chat_id":"1234567890"}`)
	qq := createConf(t, db, "qq_onebot", "qq", `{"listen_addr":"127.0.0.1:6700","admin_qq_users":[454196976],"allowed_qq_users":[454196976,10001]}`)
	createConf(t, db, "webhook", "hook", `{"url":"https://example.com"}`)
	require.NoError(t, db.Create(&models.ChannelBinding{NotificationConfID: qq.ID, ChannelType: "qq_onebot", ChannelUserID: "454196976", PtAdmin: true, Allowed: true}).Error)
	// 已经绑定、却不在名单里的账号（名单后来改了）：也要列出来
	require.NoError(t, db.Create(&models.ChannelBinding{NotificationConfID: qq.ID, ChannelType: "qq_onebot", ChannelUserID: "777", PtAdmin: true, Allowed: true}).Error)
	base := time.Date(2026, 10, 21, 20, 57, 0, 0, time.Local)
	for i, a := range []models.ActionAudit{
		{ChannelType: "telegram", ChannelUserID: "1234567890", Command: "bind", Result: "success", CreatedAt: base.AddDate(0, 0, -2)},
		{NotificationConfID: tg.ID, ChannelType: "telegram", ChannelUserID: "1234567890", Command: "unbind", Result: "success", CreatedAt: base},
		{ChannelType: "telegram", ChannelUserID: "1234567890", Command: "help", Result: "denied:not_bound", CreatedAt: base.Add(44 * time.Minute)},
		{NotificationConfID: tg.ID, ChannelType: "telegram", Command: "notify:daily_report:daily", Result: "success", CreatedAt: base.Add(time.Hour)},
	} {
		a.ArgsJSON = "{}"
		require.NoError(t, db.Create(&a).Error, i)
	}

	views, err := NewBindingService(db, "admin").ListChannels(context.Background())
	require.NoError(t, err)
	require.Len(t, views, 2, "只列能收命令的通道")

	v := views[0]
	assert.Equal(t, tg.ID, v.ConfID)
	assert.Equal(t, "pt-tools-bot", v.Name)
	assert.Equal(t, "1234567890", v.NotifyTarget)
	assert.Equal(t, NotifyViaDefaultChat, v.NotifyVia)
	require.Len(t, v.Users, 2)
	me := v.Users[0]
	assert.Equal(t, "1234567890", me.UserID)
	assert.Equal(t, []string{RoleAdmin}, me.Roles)
	assert.False(t, me.Bound)
	require.NotNil(t, me.Last)
	assert.Equal(t, "help", me.Last.Command, "最近一次往来不算系统发的通知")
	assert.Equal(t, "denied:not_bound", me.Last.Result)
	assert.Equal(t, []string{RoleAllowed}, v.Users[1].Roles)
	assert.Nil(t, v.Users[1].Last)

	q := views[1]
	assert.Equal(t, "454196976", q.NotifyTarget)
	assert.Equal(t, NotifyViaAdmin, q.NotifyVia)
	require.Len(t, q.Users, 3)
	assert.Equal(t, []string{RoleAdmin, RoleAllowed}, q.Users[0].Roles)
	assert.True(t, q.Users[0].Bound)
	assert.NotZero(t, q.Users[0].BindingID)
	assert.Equal(t, "10001", q.Users[1].UserID)
	assert.False(t, q.Users[1].Bound)
	assert.Equal(t, "777", q.Users[2].UserID)
	assert.Empty(t, q.Users[2].Roles, "已经绑定、但不在名单里")
	assert.True(t, q.Users[2].Bound)
}

func TestListChannels_NotifyTargetSources(t *testing.T) {
	db := setupBindingChannelsDB(t)
	allowedOnly := createConf(t, db, "qq_onebot", "qq", `{"allowed_qq_users":[3]}`)
	bindingOnly := createConf(t, db, "telegram", "tg", `{"bot_token":"t","allowed_users":[8]}`)
	none := createConf(t, db, "telegram", "tg2", `{"bot_token":"t"}`)
	require.NoError(t, db.Create(&models.ChannelBinding{NotificationConfID: bindingOnly.ID, ChannelType: "telegram", ChannelUserID: "8", PtAdmin: true, Allowed: true}).Error)
	broken := models.NotificationConf{ChannelType: "qq_onebot", Name: "broken", ConfigJSON: "not-a-cipher", Enabled: true}
	require.NoError(t, db.Create(&broken).Error)

	views, err := NewBindingService(db, "admin").ListChannels(context.Background())
	require.NoError(t, err)
	byID := map[uint]ChannelBindingView{}
	for _, v := range views {
		byID[v.ConfID] = v
	}
	assert.Equal(t, NotifyViaAllowed, byID[allowedOnly.ID].NotifyVia)
	assert.Equal(t, "3", byID[allowedOnly.ID].NotifyTarget)
	assert.Equal(t, NotifyViaBinding, byID[bindingOnly.ID].NotifyVia, "Telegram 没有管理员时用绑定的账号，不用允许名单")
	assert.Equal(t, "8", byID[bindingOnly.ID].NotifyTarget)
	assert.Empty(t, byID[none.ID].NotifyTarget)
	assert.Contains(t, byID[none.ID].NotifyError, "无可用收件人")
	assert.Contains(t, byID[broken.ID].ConfigError, "解密")
	assert.Empty(t, byID[broken.ID].Users)
}

func TestBindDirect(t *testing.T) {
	db := setupBindingChannelsDB(t)
	tg := createConf(t, db, "telegram", "tg", `{"bot_token":"t","admin_users":[1234567890],"allowed_users":["555"]}`)
	hook := createConf(t, db, "webhook", "hook", `{"url":"https://example.com"}`)
	svc := NewBindingService(db, "admin")
	ctx := context.Background()

	got, err := svc.BindDirect(ctx, tg.ID, "1234567890")
	require.NoError(t, err)
	assert.True(t, got.PtAdmin, "与绑定码一样，能执行全部命令")
	assert.True(t, got.Allowed)
	assert.Equal(t, "网页绑定", got.Label)
	assert.Equal(t, "telegram", got.ChannelType)

	// 绑定以后命令走得通：MessageChain 按 (通道, 类型, 账号) 查到这一行
	var row models.ChannelBinding
	require.NoError(t, db.Where("notification_conf_id = ? AND channel_type = ? AND channel_user_id = ?", tg.ID, "telegram", "1234567890").First(&row).Error)

	refused, err := svc.BindDirect(ctx, tg.ID, "1234567890")
	require.ErrorIs(t, err, ErrAlreadyBound)
	assert.Equal(t, "telegram", refused.ChannelType, "拒绝时也带上通道，审计要用")

	_, err = svc.BindDirect(ctx, tg.ID, "555")
	require.NoError(t, err, "允许名单里的账号也能绑定")

	refused, err = svc.BindDirect(ctx, tg.ID, "999")
	require.ErrorIs(t, err, ErrBindNotInWhitelist)
	assert.Equal(t, tg.ID, refused.ConfID)

	_, err = svc.BindDirect(ctx, hook.ID, "1")
	require.ErrorIs(t, err, ErrBindUnsupportedChannel)
	_, err = svc.BindDirect(ctx, 999, "1")
	require.ErrorIs(t, err, ErrConfNotFound)
	_, err = svc.BindDirect(ctx, tg.ID, "")
	require.ErrorIs(t, err, ErrBindNotInWhitelist)

	var count int64
	require.NoError(t, db.Model(&models.ChannelBinding{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}
