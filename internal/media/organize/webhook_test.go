package organize

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/server"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

const (
	hookAdded = `{"NotificationType":"ItemAdded","ItemId":"m1","ItemType":"Movie","Name":"奥本海默"}`
	hookStart = `{"NotificationType":"PlaybackStart","ItemId":"e1","ItemType":"Episode","Name":"当你迷失在黑暗中","SeriesName":"最后生还者",
"SeasonNumber":"1","EpisodeNumber":"1","NotificationUsername":"admin","DeviceName":"Living Room TV","ClientName":"Jellyfin Web"}`
	hookStop = `{"Event":"playback.stop","Item":{"Id":"m1","Name":"奥本海默","Type":"Movie","ProductionYear":2023},"User":{"Name":"admin"},
"Session":{"DeviceName":"Chrome","Client":"Chrome"}}`
)

func (e *env) hook(id uint, token, body string) error {
	return e.svc.HandleWebhook(e.ctx, id, token, "text/plain", strings.NewReader(body))
}

func TestWebhookToken(t *testing.T) {
	e := newEnv(t)
	v := e.saveJellyfin(newFakeJellyfin(t), ServerInput{})
	token, err := e.svc.CreateWebhookToken(e.ctx, v.ID)
	require.NoError(t, err)
	assert.Len(t, token, 43, "32 字节 base64url")
	var row models.MediaServer
	require.NoError(t, e.db.First(&row, v.ID).Error)
	assert.Equal(t, tokenHash(token), row.WebhookTokenHash, "只存 SHA-256")
	assert.NotContains(t, row.WebhookTokenHash, token)
	list, _ := e.svc.Servers(e.ctx)
	assert.True(t, list[0].Webhook)

	require.NoError(t, e.hook(v.ID, token, hookAdded))
	require.ErrorIs(t, e.hook(v.ID, token+"x", hookAdded), ErrNotFound, "令牌不对")
	require.ErrorIs(t, e.hook(v.ID, "", hookAdded), ErrNotFound)
	require.ErrorIs(t, e.hook(99, token, hookAdded), ErrNotFound, "服务器不存在")

	again, err := e.svc.CreateWebhookToken(e.ctx, v.ID)
	require.NoError(t, err)
	assert.NotEqual(t, token, again)
	require.ErrorIs(t, e.hook(v.ID, token, hookAdded), ErrNotFound, "重新生成以后旧的作废")
	require.NoError(t, e.hook(v.ID, again, hookAdded))

	require.NoError(t, e.svc.DeleteWebhookToken(e.ctx, v.ID))
	require.ErrorIs(t, e.hook(v.ID, again, hookAdded), ErrNotFound, "关掉以后不认")
	_, err = e.svc.CreateWebhookToken(e.ctx, 99)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, e.svc.DeleteWebhookToken(e.ctx, 99), ErrNotFound)
}

func TestWebhookSchedulesSync(t *testing.T) {
	e := newEnv(t)
	v := e.saveJellyfin(newFakeJellyfin(t), ServerInput{})
	token, err := e.svc.CreateWebhookToken(e.ctx, v.ID)
	require.NoError(t, err)
	require.NoError(t, e.hook(v.ID, token, hookAdded))
	var row models.MediaServer
	require.NoError(t, e.db.First(&row, v.ID).Error)
	require.NotNil(t, row.WebhookAt)
	assert.True(t, row.WebhookAt.Equal(e.Now()))
	assert.Equal(t, "ItemAdded", row.WebhookEvent, "记下最近一次收到的事件")
	e.svc.idx.mu.Lock()
	at, ok := e.svc.idx.pending[v.ID]
	e.svc.idx.mu.Unlock()
	require.True(t, ok)
	assert.True(t, at.Equal(e.Now().Add(webhookSyncDelay)), "两分钟后同步")

	// 一分钟后又来一次删除：合进同一次
	e.advance(time.Minute)
	require.NoError(t, e.hook(v.ID, token, `{"NotificationType":"ItemDeleted","ItemId":"m1"}`))
	e.svc.idx.mu.Lock()
	at2 := e.svc.idx.pending[v.ID]
	e.svc.idx.mu.Unlock()
	assert.True(t, at2.Equal(at), "多次合并成一次")

	// 关了索引的不排；关着的服务器只记下来
	other := e.saveJellyfin(newFakeJellyfin(t), ServerInput{Name: "关了索引", IndexHours: ptr(0)})
	tok2, err := e.svc.CreateWebhookToken(e.ctx, other.ID)
	require.NoError(t, err)
	require.NoError(t, e.hook(other.ID, tok2, hookAdded))
	e.svc.idx.mu.Lock()
	_, queued := e.svc.idx.pending[other.ID]
	e.svc.idx.mu.Unlock()
	assert.False(t, queued)

	require.NoError(t, e.db.Model(&models.MediaServer{}).Where("id = ?", v.ID).Update("enabled", false).Error)
	e.svc.idx.mu.Lock()
	delete(e.svc.idx.pending, v.ID)
	e.svc.idx.mu.Unlock()
	require.NoError(t, e.hook(v.ID, token, hookAdded))
	e.svc.idx.mu.Lock()
	_, queued = e.svc.idx.pending[v.ID]
	e.svc.idx.mu.Unlock()
	assert.False(t, queued, "关着的服务器不处理")
}

func TestWebhookRateLimitAndBadBody(t *testing.T) {
	e := newEnv(t)
	v := e.saveJellyfin(newFakeJellyfin(t), ServerInput{})
	token, err := e.svc.CreateWebhookToken(e.ctx, v.ID)
	require.NoError(t, err)
	require.ErrorIs(t, e.hook(v.ID, token, "nope"), server.ErrBadWebhook)
	for range webhookPerMinute - 1 {
		require.NoError(t, e.hook(v.ID, token, `{"NotificationType":"UserDataSaved"}`))
	}
	require.ErrorIs(t, e.hook(v.ID, token, hookAdded), ErrRateLimited, "每分钟 60 次（看不懂的那次也算）")
	e.advance(time.Minute)
	require.NoError(t, e.hook(v.ID, token, hookAdded), "过了一分钟重新算")
	var row models.MediaServer
	require.NoError(t, e.db.First(&row, v.ID).Error)
	assert.Equal(t, "ItemAdded", row.WebhookEvent)

	var l hookLimiter
	now := time.Now()
	assert.True(t, l.allow(1, now))
	assert.True(t, l.allow(2, now), "每台服务器分开算")
	assert.True(t, l.allow(1, now.Add(-time.Hour)), "时钟往回调时重新算")
}

func TestWebhookPlaybackNotice(t *testing.T) {
	e := newEnv(t)
	v := e.saveJellyfin(newFakeJellyfin(t), ServerInput{})
	token, err := e.svc.CreateWebhookToken(e.ctx, v.ID)
	require.NoError(t, err)
	require.NoError(t, e.hook(v.ID, token, hookStart))
	assert.Empty(t, e.gotNotices(), "没选通道时不发")

	_, err = e.svc.SaveServer(e.ctx, v.ID, ServerInput{
		Name: "Jellyfin", Kind: models.MediaServerJellyfin, URL: v.URL, Enabled: true, LocalPrefix: "/media", ServerPrefix: "/data", PlaybackChannels: ptr([]uint{7}),
	})
	require.NoError(t, err)
	require.NoError(t, e.hook(v.ID, token, hookStart))
	require.NoError(t, e.hook(v.ID, token, hookStart), "同一集再播一次也发")
	e.advance(time.Second)
	require.NoError(t, e.hook(v.ID, token, hookStop))
	got := e.gotNotices()
	require.Len(t, got, 3)
	n := got[0]
	assert.Equal(t, NoticePlayback, n.Kind)
	assert.Equal(t, []uint{7}, n.ChannelIDs)
	assert.Equal(t, "▶️ 开始播放：最后生还者 S01E01「当你迷失在黑暗中」", n.Title)
	assert.Equal(t, "Jellyfin · admin · Living Room TV（Jellyfin Web）", n.Text)
	assert.True(t, strings.HasPrefix(n.Key, "playback-1:start:e1:"), n.Key)
	assert.NotEqual(t, got[0].Key, got[1].Key, "键带纳秒")
	require.NotNil(t, n.Event)
	assert.Equal(t, notify.EventMediaPlayback, n.Event.Type)
	assert.Equal(t, map[string]any{
		"server": "Jellyfin", "user": "admin", "action": "start", "action_label": "开始播放", "name": "最后生还者 S01E01「当你迷失在黑暗中」",
		"media_type": "tv", "season": int64(1), "episode": int64(1), "device": "Living Room TV", "client": "Jellyfin Web",
	}, n.Event.Fields)

	stop := got[2]
	assert.Equal(t, "⏹️ 停止播放：奥本海默 (2023)", stop.Title, "Emby 格式；电影带年份")
	assert.Equal(t, "Jellyfin · admin · Chrome", stop.Text, "设备和客户端一样时不重复")
	assert.Equal(t, "stop", stop.Event.Fields["action"])
	assert.Equal(t, "movie", stop.Event.Fields["media_type"])

	// 通知发不出去时不让 Webhook 失败
	e.mu.Lock()
	e.notifyEr = assert.AnError
	e.mu.Unlock()
	require.NoError(t, e.hook(v.ID, token, hookStart))
}

func TestPlaybackName(t *testing.T) {
	assert.Equal(t, "最后生还者 S01E02", playbackName(server.WebhookEvent{Kind: models.MediaServerItemEpisode, SeriesTitle: "最后生还者", Season: 1, Episode: 2}))
	assert.Equal(t, "最后生还者 S01E02「无尽的等待」", playbackName(server.WebhookEvent{Kind: models.MediaServerItemEpisode, SeriesTitle: "最后生还者", Season: 1, Episode: 2, Title: " 无尽的等待 "}))
	// Jellyfin 没取到元数据时拿文件名当集名：已经有季集，不再写一遍
	assert.Equal(t, "逆袭的千金 S01E02", playbackName(server.WebhookEvent{Kind: models.MediaServerItemEpisode, SeriesTitle: "逆袭的千金", Season: 1, Episode: 2, Title: "逆袭的千金 S01E02"}))
	assert.Equal(t, "逆袭的千金 S01E02", playbackName(server.WebhookEvent{Kind: models.MediaServerItemEpisode, SeriesTitle: "逆袭的千金", Season: 1, Episode: 2, Title: "nixi.s01e02.1080p"}))
	assert.Equal(t, "逆袭的千金 S01E02「S01E20 的回忆」", playbackName(server.WebhookEvent{Kind: models.MediaServerItemEpisode, SeriesTitle: "逆袭的千金", Season: 1, Episode: 2, Title: "S01E20 的回忆"}),
		"别的集号不算")
	assert.Equal(t, "奥本海默", playbackName(server.WebhookEvent{Kind: models.MediaServerItemMovie, Title: "奥本海默"}))
	assert.Equal(t, "一首歌", playbackName(server.WebhookEvent{Title: "一首歌"}))
	assert.Equal(t, "单集", playbackName(server.WebhookEvent{Kind: models.MediaServerItemEpisode, Title: "单集"}), "没有剧名时写集名")
}
