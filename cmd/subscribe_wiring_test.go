package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

// 订阅服务：没有识别服务时不建；建好后能读写设置，停下时断开 RSS 的入口
func TestNewSubscribeService(t *testing.T) {
	db := organizeTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&models.MediaQualityProfile{}, &models.MediaSubscription{}, &models.MediaSubscriptionTorrent{},
		&models.MediaSubscribeSetting{}, &models.MediaDoubanSource{}, &models.MediaDoubanItem{},
		&models.MediaSubscriptionEpisode{}, &models.MediaEpisodeRule{}, &models.MediaSubscriptionRun{},
	))
	store := core.NewConfigStore(global.GlobalDB)
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	ctx := context.Background()
	assert.Nil(t, newSubscribeService(ctx, mgr, nil, nil, nil, nil))

	svc := newSubscribeService(ctx, mgr, newMediaService(store), nil, nil, nil)
	require.NotNil(t, svc)
	set, err := svc.SaveSettings(ctx, subscribe.Settings{Enabled: true, SearchIntervalHours: 6})
	require.NoError(t, err)
	assert.True(t, set.Enabled)
	stopSubscribeService(svc)
	stopSubscribeService(nil)
	assert.Empty(t, qaDoubanURL(), "正式构建里豆瓣地址不能改")
}

func TestSubscribeNotifierDisabled(t *testing.T) {
	assert.Nil(t, subscribeNotifier(nil), "没有通知投递器时不发")
}

// /sub、/subs 经订阅服务：订阅的名字带年份与季，进度写成一句话
func TestChatOpsSubscribeAdapter(t *testing.T) {
	assert.Equal(t, "沙丘2 (2024)", subscriptionName(&models.MediaSubscription{MediaType: models.MediaKindMovie, Title: "沙丘2", Year: 2024}))
	assert.Equal(t, "最后生还者 (2025) 第 2 季", subscriptionName(&models.MediaSubscription{MediaType: models.MediaKindTV, Title: "最后生还者", Year: 2025, Season: 2}))
	assert.Equal(t, "待确认", subscriptionStatus(models.MediaSubPending))
	tv := &subscribe.SubscriptionView{MediaSubscription: models.MediaSubscription{MediaType: models.MediaKindTV}, Progress: &subscribe.Progress{Total: 3, InLibrary: 1, Downloading: 1, Missing: []int{2}}}
	assert.Equal(t, "已入库 1/3，下载中 1，缺 1", progressText(tv))
	follow := &subscribe.SubscriptionView{MediaSubscription: models.MediaSubscription{MediaType: models.MediaKindTV}, Progress: &subscribe.Progress{Total: 6, InLibrary: 2, Missing: []int{3, 4}, Waiting: 1, Skipped: 1}}
	assert.Equal(t, "已入库 2/6，缺 2（等版本 1），不追 1", progressText(follow))
	movie := &subscribe.SubscriptionView{MediaSubscription: models.MediaSubscription{MediaType: models.MediaKindMovie}, Progress: &subscribe.Progress{Total: 1, Downloading: 1}}
	assert.Equal(t, "下载中", progressText(movie))
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local)
	later, earlier := now.Add(9*time.Hour), now.Add(-time.Minute)
	assert.Equal(t, "10-21 21:00", nextSearchText(&models.MediaSubscription{Status: models.MediaSubActive, NextSearchAt: &later}, now))
	assert.Equal(t, "马上", nextSearchText(&models.MediaSubscription{Status: models.MediaSubActive, NextSearchAt: &earlier}, now))
	assert.Empty(t, nextSearchText(&models.MediaSubscription{Status: models.MediaSubPaused, NextSearchAt: &later}, now))
	assert.Empty(t, nextSearchText(&models.MediaSubscription{Status: models.MediaSubActive}, now))
	assert.Empty(t, progressText(&subscribe.SubscriptionView{}))
	_, err := chatopsSubscribe{}.Subscribe(context.Background(), "book", 1, 0)
	require.Error(t, err)
}

type subscribeTestCipher struct{}

func (subscribeTestCipher) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (subscribeTestCipher) Decrypt(c string) (string, error) {
	return strings.TrimPrefix(c, "enc:"), nil
}

// /sub 搜电影与剧集、按热度排好；订阅以后 /subs 列出来
func TestChatOpsSubscribeAdapterFlow(t *testing.T) {
	db := organizeTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{}, &models.MediaCache{},
		&models.MediaQualityProfile{}, &models.MediaSubscription{}, &models.MediaSubscriptionTorrent{},
		&models.MediaSubscribeSetting{}, &models.MediaDoubanSource{}, &models.MediaDoubanItem{},
		&models.MediaSubscriptionEpisode{}, &models.MediaEpisodeRule{}, &models.MediaSubscriptionRun{},
	))
	const key = "0123456789abcdef0123456789abcdef"
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		en := r.URL.Query().Get("language") == "en-US"
		switch {
		case r.URL.Path == "/3/search/movie":
			_, _ = w.Write([]byte(`{"results":[{"id":278,"title":"肖申克的救赎","release_date":"1994-09-23","popularity":50}]}`))
		case r.URL.Path == "/3/search/tv":
			_, _ = w.Write([]byte(`{"results":[{"id":1396,"name":"绝命毒师","first_air_date":"2008-01-20","popularity":90}]}`))
		case r.URL.Path == "/3/movie/278" && en:
			_, _ = w.Write([]byte(`{"id":278,"title":"The Shawshank Redemption","alternative_titles":{"titles":[]}}`))
		case r.URL.Path == "/3/movie/278":
			_, _ = w.Write([]byte(`{"id":278,"title":"肖申克的救赎","original_title":"The Shawshank Redemption","release_date":"1994-09-23"}`))
		case r.URL.Path == "/3/tv/1396" && en:
			_, _ = w.Write([]byte(`{"id":1396,"name":"Breaking Bad","alternative_titles":{"results":[]}}`))
		case r.URL.Path == "/3/tv/1396":
			_, _ = w.Write([]byte(`{"id":1396,"name":"绝命毒师","original_name":"Breaking Bad","first_air_date":"2008-01-20","number_of_seasons":1}`))
		case r.URL.Path == "/3/tv/1396/season/1":
			_, _ = w.Write([]byte(`{"season_number":1,"air_date":"2008-01-20","episodes":[{"episode_number":1,"air_date":"2008-01-20"},{"episode_number":2,"air_date":"2008-01-27"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(fake.Close)
	rec := recognize.New(recognize.Config{DB: db, Cipher: subscribeTestCipher{}, BaseURL: fake.URL + "/3", RatePerSecond: 1000})
	k := key
	_, err := rec.SaveSettings(context.Background(), recognize.SettingsInput{TMDBKey: &k})
	require.NoError(t, err)
	c := chatopsSubscribe{svc: subscribe.New(subscribe.Config{DB: db, Recognizer: rec})}
	ctx := context.Background()

	cands, err := c.Search(ctx, "肖申克")
	require.NoError(t, err)
	require.Len(t, cands, 2)
	assert.Equal(t, 1396, cands[0].TMDBID, "热度高的在前")
	assert.Equal(t, models.MediaKindTV, cands[0].Kind)
	name, err := c.Subscribe(ctx, models.MediaKindMovie, 278, 0)
	require.NoError(t, err)
	assert.Equal(t, "肖申克的救赎 (1994)", name)
	cands, err = c.Search(ctx, "肖申克")
	require.NoError(t, err)
	assert.True(t, cands[1].Subscribed)
	lines, err := c.List(ctx)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "在找", lines[0].Status)
	assert.Equal(t, "还没下载", lines[0].Progress)
	_, err = c.Subscribe(ctx, models.MediaKindMovie, 278, 0)
	require.Error(t, err, "订阅过了")

	// /follow 按追剧的默认设置建；/runs 看到建立的那次；/unlock 一集换版本
	text, err := c.Follow(ctx, 1396, 0)
	require.NoError(t, err)
	assert.Equal(t, "绝命毒师 (2008) 第 1 季，按播出时间搜索（20:00 播出，2 小时后开始搜），版本锁开（按第一个下载的版本定）", text)
	lines, err = c.List(ctx)
	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.Equal(t, "绝命毒师 (2008) 第 1 季", lines[0].Title, "新的在前")
	assert.NotZero(t, lines[0].ID)
	assert.Equal(t, "马上", lines[0].Next, "建好以后马上搜")
	assert.Equal(t, "已入库 0/2，缺 2", lines[0].Progress)
	name, runs, err := c.Runs(ctx, lines[0].ID, 5)
	require.NoError(t, err)
	assert.Equal(t, "绝命毒师 (2008) 第 1 季", name)
	require.Len(t, runs, 1)
	assert.Equal(t, models.MediaRunCreate, runs[0].Trigger)
	assert.Contains(t, runs[0].Summary, "建立了追剧")
	name, err = c.Unlock(ctx, lines[0].ID, 2)
	require.NoError(t, err)
	assert.Equal(t, "绝命毒师 (2008) 第 1 季", name)
	_, err = c.Unlock(ctx, lines[0].ID, 9)
	require.Error(t, err, "这一季没有第 9 集")
	_, err = c.Unlock(ctx, lines[1].ID, 1)
	require.Error(t, err, "电影没有分集")
	_, _, err = c.Runs(ctx, 999, 5)
	require.Error(t, err)
	_, runs, err = c.Runs(ctx, lines[0].ID, 5)
	require.NoError(t, err)
	assert.Len(t, runs, 2, "换版本记一条手动的运行")

	// 搜索出错时写明
	_, err = chatopsSubscribe{svc: subscribe.New(subscribe.Config{DB: db, Recognizer: noTMDB{rec}})}.Search(ctx, "x")
	require.Error(t, err)
	for status, want := range map[string]string{models.MediaSubActive: "在找", models.MediaSubPaused: "暂停", models.MediaSubDone: "完成", "x": "x"} {
		assert.Equal(t, want, subscriptionStatus(status))
	}
}

type noTMDB struct{ subscribe.Recognizer }

func (noTMDB) TMDB(context.Context) (*tmdb.Client, error) { return nil, tmdb.ErrNoKey }

// 订阅的通知写进监控通知日志：来源 media，海报地址接在正文后面，主题截到 64 字节
func TestSubscribeNotifierEnqueues(t *testing.T) {
	db := organizeTestDB(t)
	n := scheduler.NewMonitorNotifier(db, scheduler.MonitorSenderFunc(func(context.Context, uint, scheduler.MonitorMessage) error { return nil }), nil, nil)
	conf := models.NotificationConf{Name: "tg", ChannelType: "telegram", Enabled: true}
	require.NoError(t, db.Create(&conf).Error)
	send := subscribeNotifier(n)
	require.NotNil(t, send)
	ev := notify.NewEvent(notify.EventSubscribeDownloaded, time.Now(), map[string]any{"subscription": "沙丘2 (2024)", "site": "hdtime"})
	require.NoError(t, send(context.Background(), subscribe.Notice{
		Channels: []uint{conf.ID}, Kind: subscribe.NoticeDownloaded, Subject: strings.Repeat("s", 80), EventKey: "k1",
		Title: "订阅下载：沙丘2 (2024)", Body: "种子：x", ImageURL: "https://image.tmdb.org/t/p/w500/p.jpg", Event: ev,
	}))
	var rows []models.MonitorNotificationLog
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "media", rows[0].Source)
	assert.Len(t, rows[0].Subject, 64)
	assert.Equal(t, subscribe.NoticeDownloaded, rows[0].Kind)
	assert.Contains(t, rows[0].PayloadJSON, `"type":"subscribe.downloaded"`, "事件一起写进日志行")
	assert.Contains(t, rows[0].PayloadJSON, `"subscription":"沙丘2 (2024)"`)
}
