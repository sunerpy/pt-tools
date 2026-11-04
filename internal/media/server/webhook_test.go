package server

import (
	"bytes"
	"mime/multipart"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// Emby 4.8 的 Webhook（勾了 application/json 时直接是 JSON，否则 multipart 的 data 字段）。
const embyPlaybackStart = `{"Title":"admin 在 Chrome 上开始播放 最后生还者 - S01E01","Date":"2026-11-04T12:00:00.0000000Z","Event":"playback.start",
"User":{"Name":"admin","Id":"u1"},
"Item":{"Name":"当你迷失在黑暗中","Id":"e1","Type":"Episode","SeriesName":"最后生还者","SeriesId":"s1","ParentIndexNumber":1,"IndexNumber":1,"ProductionYear":2023,"ProviderIds":{"Tmdb":"2181581"}},
"Server":{"Name":"emby","Id":"srv","Version":"4.8.10.0"},
"Session":{"RemoteEndPoint":"10.0.0.2","Client":"Emby Web","DeviceName":"Chrome","DeviceId":"d1"},
"PlaybackInfo":{"PositionTicks":0}}`

// jellyfin-plugin-webhook 的 Generic 目标，按文档给的模板（数字有的带引号）。
const jellyfinItemAdded = `{"NotificationType":"ItemAdded","ServerId":"srv","ItemId":"m1","ItemType":"Movie","Name":"奥本海默","Year":2023,
"SeriesName":"","SeasonNumber":"","EpisodeNumber":"","Provider_tmdb":"872585","Provider_imdb":"tt15398776","NotificationUsername":"","DeviceName":"","ClientName":""}`

// Plex 的 Webhook：multipart 的 payload 字段（JSON），后面跟着缩略图。
const plexMediaPlay = `{"event":"media.play","user":true,"owner":true,
"Account":{"id":1,"title":"plexuser"},"Server":{"title":"NAS Plex","uuid":"abc123"},
"Player":{"local":true,"title":"Living Room TV","uuid":"p1"},
"Metadata":{"librarySectionType":"show","ratingKey":"301","type":"episode","title":"当你迷失在黑暗中","grandparentTitle":"最后生还者","parentIndex":1,"index":1,"year":2023,"Guid":[{"id":"tmdb://2181581"}]}}`

func multipartBody(t *testing.T, field, payload string, after []byte) (string, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField(field, payload))
	if after != nil {
		fw, err := w.CreateFormFile("thumb", "thumb.jpg")
		require.NoError(t, err)
		_, _ = fw.Write(after)
	}
	require.NoError(t, w.Close())
	return w.FormDataContentType(), &buf
}

func TestParseWebhookEmby(t *testing.T) {
	ev, err := ParseWebhook("application/json", strings.NewReader(embyPlaybackStart))
	require.NoError(t, err)
	assert.Equal(t, WebhookEvent{
		Action: WebhookPlaybackStart, Name: "playback.start", ItemID: "e1", Kind: models.MediaServerItemEpisode, Title: "当你迷失在黑暗中",
		SeriesTitle: "最后生还者", Season: 1, Episode: 1, Year: 2023, User: "admin", Device: "Chrome", Client: "Emby Web",
	}, ev)

	ct, body := multipartBody(t, "data", strings.Replace(embyPlaybackStart, "playback.start", "library.deleted", 1), nil)
	ev, err = ParseWebhook(ct, body)
	require.NoError(t, err)
	assert.Equal(t, WebhookRemoved, ev.Action, "multipart 的 data 字段")

	form := url.Values{"data": {strings.Replace(embyPlaybackStart, "playback.start", "library.new", 1)}}.Encode()
	ev, err = ParseWebhook("application/x-www-form-urlencoded", strings.NewReader(form))
	require.NoError(t, err)
	assert.Equal(t, WebhookAdded, ev.Action, "表单的 data 字段")

	ev, err = ParseWebhook("application/json", strings.NewReader(`{"Event":"system.notificationtest","Title":"测试"}`))
	require.NoError(t, err)
	assert.Equal(t, WebhookOther, ev.Action)
	assert.Equal(t, "system.notificationtest", ev.Name)
}

func TestParseWebhookJellyfin(t *testing.T) {
	ev, err := ParseWebhook("text/plain; charset=utf-8", strings.NewReader(jellyfinItemAdded))
	require.NoError(t, err)
	assert.Equal(t, WebhookEvent{Action: WebhookAdded, Name: "ItemAdded", ItemID: "m1", Kind: models.MediaServerItemMovie, Title: "奥本海默", Year: 2023}, ev,
		"插件默认按 text/plain 发；空串的数字当作 0")

	stop := `{"NotificationType":"PlaybackStop","ItemId":"e1","ItemType":"Episode","Name":"当你迷失在黑暗中","SeriesName":"最后生还者",
"SeasonNumber":"1","EpisodeNumber":2,"Year":"x","NotificationUsername":"admin","DeviceName":"iPhone","ClientName":"Jellyfin iOS"}`
	ev, err = ParseWebhook("", strings.NewReader(stop))
	require.NoError(t, err)
	assert.Equal(t, WebhookEvent{
		Action: WebhookPlaybackStop, Name: "PlaybackStop", ItemID: "e1", Kind: models.MediaServerItemEpisode, Title: "当你迷失在黑暗中",
		SeriesTitle: "最后生还者", Season: 1, Episode: 2, User: "admin", Device: "iPhone", Client: "Jellyfin iOS",
	}, ev, "带引号的数字也认，写错的当作 0")

	ev, err = ParseWebhook("application/json", strings.NewReader(`{"NotificationType":"ItemDeleted","ItemId":"e1","ItemType":"Series"}`))
	require.NoError(t, err)
	assert.Equal(t, WebhookRemoved, ev.Action)
	assert.Equal(t, models.MediaServerItemSeries, ev.Kind)

	ev, err = ParseWebhook("application/json", strings.NewReader(`{"NotificationType":"UserDataSaved","ItemId":"e1"}`))
	require.NoError(t, err)
	assert.Equal(t, WebhookOther, ev.Action)
}

func TestParseWebhookPlex(t *testing.T) {
	thumb := bytes.Repeat([]byte{0xff}, WebhookMaxBody/2)
	ct, body := multipartBody(t, "payload", plexMediaPlay, thumb)
	ev, err := ParseWebhook(ct, body)
	require.NoError(t, err)
	assert.Equal(t, WebhookEvent{
		Action: WebhookPlaybackStart, Name: "media.play", ItemID: "301", Kind: models.MediaServerItemEpisode, Title: "当你迷失在黑暗中",
		SeriesTitle: "最后生还者", Season: 1, Episode: 1, Year: 2023, User: "plexuser", Device: "Living Room TV", Client: "Plex",
	}, ev, "user 是布尔、不和 Emby 的 User 混；读到 payload 就停，后面的缩略图不读")

	ct, body = multipartBody(t, "payload", `{"event":"library.new","Metadata":{"ratingKey":"101","type":"movie","title":"奥本海默","parentIndex":3}}`, nil)
	ev, err = ParseWebhook(ct, body)
	require.NoError(t, err)
	assert.Equal(t, WebhookAdded, ev.Action)
	assert.Equal(t, 0, ev.Season, "电影不写季集")

	ct, body = multipartBody(t, "payload", `{"event":"media.scrobble","Metadata":{"type":"track"}}`, nil)
	ev, err = ParseWebhook(ct, body)
	require.NoError(t, err)
	assert.Equal(t, WebhookOther, ev.Action)
	assert.Empty(t, ev.Kind)
}

func TestParseWebhookErrors(t *testing.T) {
	for name, tc := range map[string]struct{ ct, body string }{
		"不是 JSON":      {"application/json", "hello"},
		"JSON 数组":      {"application/json", "[1]"},
		"没有事件名":        {"application/json", `{"Item":{}}`},
		"字段类型不对":       {"application/json", `{"Event":3}`},
		"表单没有 data":    {"application/x-www-form-urlencoded", "x=1"},
		"表单坏了":         {"application/x-www-form-urlencoded", "%zz"},
		"multipart 坏了": {"multipart/form-data; boundary=x", "nope"},
	} {
		_, err := ParseWebhook(tc.ct, strings.NewReader(tc.body))
		require.ErrorIs(t, err, ErrBadWebhook, name)
	}
	ct, body := multipartBody(t, "other", "{}", nil)
	_, err := ParseWebhook(ct, body)
	require.ErrorIs(t, err, ErrBadWebhook, "multipart 里没有 data 或 payload")

	big := `{"Event":"library.new","Title":"` + strings.Repeat("x", WebhookMaxBody) + `"}`
	_, err = ParseWebhook("application/json", strings.NewReader(big))
	require.ErrorIs(t, err, ErrBadWebhook, "超过 1 MiB")
	assert.Contains(t, err.Error(), "1 MiB")
}
