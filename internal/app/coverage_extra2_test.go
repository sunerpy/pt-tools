// MIT License
// Copyright (c) 2025 pt-tools

// Wave-2 coverage top-ups for the app package: notification config-merge and
// decrypt error branches, testChatID recipient fallbacks/errors, Enqueue via
// manager-less Push, RSS callback resolveDownloaderID + recordPushError paths,
// rss_notifier render/quota edge cases, site_service classOf/formatBytes, and
// torrentService generic error mapping.

package app

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestUpdateConf_MergesExistingConfigJSON(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	cipher, err := crypto.Encrypt([]byte(`{"bot_token":"old","default_chat_id":"111"}`))
	require.NoError(t, err)
	row := models.NotificationConf{ChannelType: "telegram", Name: "tg", ConfigJSON: cipher, Enabled: true}
	require.NoError(t, db.Create(&row).Error)

	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	// Only send bot_token; default_chat_id must be preserved from the merge.
	require.NoError(t, svc.UpdateConf(context.Background(), row.ID, UpdateConfReq{
		ConfigJSON: []byte(`{"bot_token":"new"}`),
	}))

	dto, err := svc.GetConf(context.Background(), row.ID)
	require.NoError(t, err)
	assert.Contains(t, string(dto.ConfigJSON), `"new"`)
	assert.Contains(t, string(dto.ConfigJSON), `"111"`)
}

func TestUpdateConf_InvalidNewConfigJSON(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	cipher, err := crypto.Encrypt([]byte(`{"bot_token":"old"}`))
	require.NoError(t, err)
	row := models.NotificationConf{ChannelType: "telegram", Name: "tg", ConfigJSON: cipher, Enabled: true}
	require.NoError(t, db.Create(&row).Error)

	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	err = svc.UpdateConf(context.Background(), row.ID, UpdateConfReq{
		ConfigJSON: []byte(`{not-json`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "解析新配置")
}

func TestUpdateConf_ConfigJSONForMissingRow(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	err := svc.UpdateConf(context.Background(), 4321, UpdateConfReq{
		ConfigJSON: []byte(`{"x":1}`),
	})
	require.ErrorIs(t, err, ErrConfNotFound)
}

func TestUpdateConf_MetadataOnly(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	cipher, err := crypto.Encrypt([]byte(`{"bot_token":"t"}`))
	require.NoError(t, err)
	row := models.NotificationConf{ChannelType: "telegram", Name: "tg", ConfigJSON: cipher, Enabled: false}
	require.NoError(t, db.Create(&row).Error)

	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	newName := "renamed"
	enabled := true
	require.NoError(t, svc.UpdateConf(context.Background(), row.ID, UpdateConfReq{
		Name: &newName, Enabled: &enabled,
	}))
	var got models.NotificationConf
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Equal(t, "renamed", got.Name)
	assert.True(t, got.Enabled)
}

func TestTestChatID_QQFallbackToBinding(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.ChannelBinding{}))
	cipher, err := crypto.Encrypt([]byte(`{}`)) // no admin/allowed users
	require.NoError(t, err)
	row := models.NotificationConf{ChannelType: "qq_onebot", Name: "qq", ConfigJSON: cipher, Enabled: true}
	require.NoError(t, db.Create(&row).Error)
	require.NoError(t, db.Create(&models.ChannelBinding{
		NotificationConfID: row.ID, ChannelType: "qq_onebot", ChannelUserID: "9001", PtAdmin: true,
	}).Error)

	mgr := &mockNotifyManager{}
	svc := NewNotificationService(db, mgr, 0)
	require.NoError(t, svc.TestConf(context.Background(), row.ID))
	assert.Contains(t, mgr.calls, row.ID)
}

func TestTestChatID_QQNoRecipient(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.ChannelBinding{}))
	cipher, err := crypto.Encrypt([]byte(`{}`))
	require.NoError(t, err)
	row := models.NotificationConf{ChannelType: "qq_onebot", Name: "qq", ConfigJSON: cipher, Enabled: true}
	require.NoError(t, db.Create(&row).Error)

	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	err = svc.TestConf(context.Background(), row.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "无可用收件人")
}

func TestTestChatID_UnknownChannelIsNoop(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	cipher, err := crypto.Encrypt([]byte(`{}`))
	require.NoError(t, err)
	row := models.NotificationConf{ChannelType: "webhook", Name: "wh", ConfigJSON: cipher, Enabled: true}
	require.NoError(t, db.Create(&row).Error)

	mgr := &mockNotifyManager{}
	svc := NewNotificationService(db, mgr, 0)
	// default branch returns "" chatID, no error → still dispatches TestConf.
	require.NoError(t, svc.TestConf(context.Background(), row.ID))
}

func TestDecryptConfigJSON_Errors(t *testing.T) {
	setupTestKey(t)
	// Empty string is a no-op success.
	var dst map[string]any
	require.NoError(t, decryptConfigJSON("", &dst))

	// Non-decryptable ciphertext.
	err := decryptConfigJSON("not-a-valid-cipher", &dst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "解密")

	// Valid cipher but wrong destination shape → unmarshal error.
	cipher, err := crypto.Encrypt([]byte(`{"a":1}`))
	require.NoError(t, err)
	var wrong []string
	err = decryptConfigJSON(cipher, &wrong)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "解析")
}

func TestEnqueue_ViaManagerlessPush(t *testing.T) {
	db := setupTestDB(t)
	svc := NewNotificationService(db, nil, 0) // no manager → Push delegates to Enqueue
	require.NoError(t, svc.(*notificationService).Push(context.Background(), Notification{
		Title: "t", Text: "b", SourceConfID: 7,
	}))
	var cnt int64
	require.NoError(t, db.Model(&models.NotificationOutbox{}).Count(&cnt).Error)
	assert.EqualValues(t, 1, cnt)
}

func TestPushSync_NoManager(t *testing.T) {
	db := setupTestDB(t)
	svc := NewNotificationService(db, nil, 0)
	err := svc.(*notificationService).PushSync(context.Background(), Notification{Title: "t"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未初始化")
}

func TestListConfs_ReturnsRows(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	c, err := crypto.Encrypt([]byte(`{}`))
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.NotificationConf{ChannelType: "telegram", Name: "a", ConfigJSON: c}).Error)
	require.NoError(t, db.Create(&models.NotificationConf{ChannelType: "qq_onebot", Name: "b", ConfigJSON: c}).Error)

	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	list, err := svc.ListConfs(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestResolveDownloaderID_ConfiguredWins(t *testing.T) {
	db := setupCallbackDB(t)
	a := NewRSSCallbackActions(db, nil)
	id := uint(42)
	got, err := a.resolveDownloaderID(context.Background(), &id)
	require.NoError(t, err)
	assert.Equal(t, uint(42), got)
}

func TestResolveDownloaderID_ZeroFallsThroughToDefault(t *testing.T) {
	db := setupCallbackDB(t)
	a := NewRSSCallbackActions(db, nil)
	ds := models.DownloaderSetting{Name: "d", Type: "qbittorrent", URL: "http://x", Enabled: true, IsDefault: true}
	require.NoError(t, db.Create(&ds).Error)

	zero := uint(0)
	got, err := a.resolveDownloaderID(context.Background(), &zero)
	require.NoError(t, err)
	assert.Equal(t, ds.ID, got)
}

func TestRecordPushError_NilCauseIsNoop(t *testing.T) {
	db := setupCallbackDB(t)
	a := NewRSSCallbackActions(db, nil)
	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "t", NotifyKind: "all",
		NotificationConfID: 1, Result: "sent", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)
	assert.NoError(t, a.recordPushError(context.Background(), &row, nil))
}

func TestNotifyNewItem_WrongModeSkips(t *testing.T) {
	db := setupRSSNotifierDB(t)
	n := NewRSSNotifier(db, &fakePushSvc{})
	rss := &models.RSSConfig{ID: 1, NotifyMode: "filtered", NotifyConfIDs: "[1]"}
	require.NoError(t, n.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, SiteName: "s", TorrentID: "t", FeedItem: newFeedItem(),
	}))
	var cnt int64
	require.NoError(t, db.Model(&models.RSSNotificationLog{}).Count(&cnt).Error)
	assert.EqualValues(t, 0, cnt)
}

func TestNotifyNewItem_InvalidConfIDs(t *testing.T) {
	db := setupRSSNotifierDB(t)
	n := NewRSSNotifier(db, &fakePushSvc{})
	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "{bad"}
	err := n.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, SiteName: "s", TorrentID: "t", FeedItem: newFeedItem(),
	})
	require.Error(t, err)
}

func TestExceededHourlyQuota_TriggersThrottle(t *testing.T) {
	db := setupRSSNotifierDB(t)
	n := NewRSSNotifier(db, &fakePushSvc{})
	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "[1]", MaxNotificationsPerHour: 1}
	// Pre-seed one recent log so the quota is already met.
	now := time.Now()
	require.NoError(t, db.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "old", NotifyKind: "all",
		NotificationConfID: 1, Result: "sent", CreatedAt: now, UpdatedAt: now,
	}).Error)

	require.NoError(t, n.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, SiteName: "s", TorrentID: "t2", FeedItem: newFeedItem(),
	}))
	var throttled int64
	require.NoError(t, db.Model(&models.RSSNotificationLog{}).
		Where("result = ?", "throttled").Count(&throttled).Error)
	assert.EqualValues(t, 1, throttled)
}

func TestNotifyFilteredItem_NilGuards(t *testing.T) {
	db := setupRSSNotifierDB(t)
	n := NewRSSNotifier(db, &fakePushSvc{})
	require.Error(t, n.NotifyFilteredItem(context.Background(), RSSFilteredEvent{RSS: nil}))
	rss := &models.RSSConfig{ID: 1, NotifyMode: "filtered", NotifyConfIDs: "[1]"}
	require.Error(t, n.NotifyFilteredItem(context.Background(), RSSFilteredEvent{RSS: rss, Torrent: nil}))
}

func TestRenderAllPayload_TitleAndTimeFallbacks(t *testing.T) {
	ev := RSSItemEvent{
		SiteName: "s",
		FeedItem: &gofeed.Item{Title: "", Published: "Mon, 01 Jan 2026", Link: "http://x"},
	}
	got := renderAllPayload(ev)
	assert.Equal(t, "(无标题)", got.Title)
	assert.Contains(t, got.Text, "Mon, 01 Jan 2026")

	ev2 := RSSItemEvent{SiteName: "s", FeedItem: &gofeed.Item{Title: "T", Link: "http://y"}}
	got2 := renderAllPayload(ev2)
	assert.Contains(t, got2.Text, "未知时间")
}

func TestRenderFilteredPayload_FreeAndRuleBranches(t *testing.T) {
	end := time.Now().Add(3 * time.Hour)
	tItem := &v2.TorrentItem{
		Title: "", URL: "http://z", SizeBytes: 2 * 1024 * 1024 * 1024,
		DiscountLevel: v2.DiscountFree, DiscountEndTime: end,
	}
	rule := &models.FilterRule{ID: 1, Name: "r1"}
	got := renderFilteredPayload(RSSFilteredEvent{SiteName: "s", Torrent: tItem, Rule: rule})
	assert.Equal(t, "(无标题)", got.Title)
	assert.Contains(t, got.Text, "免费")
	assert.Contains(t, got.Text, "剩余")
	assert.Contains(t, got.Text, "匹配规则")

	tItem2 := &v2.TorrentItem{Title: "X", URL: "http://z", DiscountLevel: v2.DiscountFree}
	got2 := renderFilteredPayload(RSSFilteredEvent{SiteName: "s", Torrent: tItem2})
	assert.Contains(t, got2.Text, "免费")
}

func TestFormatBytesRSS_And_FormatRemaining(t *testing.T) {
	assert.Equal(t, "512 B", formatBytesRSS(512))
	assert.Contains(t, formatBytesRSS(1024*1024), "MB")

	assert.Equal(t, "已结束", formatRemaining(time.Now().Add(-time.Hour)))
	assert.Contains(t, formatRemaining(time.Now().Add(90*time.Minute)), "h")
	assert.Contains(t, formatRemaining(time.Now().Add(30*time.Minute)), "min")
}

func TestOnRSSDownload_PushFailureRecordsError(t *testing.T) {
	setupTestKey(t)
	db := setupCallbackDB(t)
	dlID := uint(5)
	ds := models.DownloaderSetting{Name: "d", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true}
	ds.ID = dlID
	require.NoError(t, db.Create(&ds).Error)
	sub := models.RSSSubscription{Name: "r", URL: "http://x", IntervalMinutes: 1, DownloaderID: &dlID}
	require.NoError(t, db.Create(&sub).Error)
	row := models.RSSNotificationLog{
		RSSID: sub.ID, SiteName: "s", TorrentID: "t", NotifyKind: "all",
		NotificationConfID: 1, Result: "sent", LastError: "prev",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)

	fetcher := func(_ context.Context, _, _ string) ([]byte, error) { return []byte("d8:announce"), nil }
	a := NewRSSCallbackActions(db, fetcher)
	err := a.OnRSSDownload(context.Background(), row.ID, 0)
	require.Error(t, err)
	var got models.RSSNotificationLog
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.NotEmpty(t, got.LastError)
}

func TestClassOf_FallsBackToRank(t *testing.T) {
	assert.Equal(t, "VIP", classOf(v2.UserInfo{LevelName: "VIP", Rank: "R"}))
	assert.Equal(t, "R", classOf(v2.UserInfo{Rank: "R"}))
	assert.Equal(t, "", classOf(v2.UserInfo{}))
}

func TestFormatBytes_Units(t *testing.T) {
	assert.Equal(t, "512 B", formatBytes(512))
	assert.Equal(t, "1.00 KiB", formatBytes(1024))
	assert.Equal(t, "1.00 MiB", formatBytes(1024*1024))
	assert.Equal(t, "1.00 GiB", formatBytes(1024*1024*1024))
}

func TestGetSiteUserInfo_EmptyName(t *testing.T) {
	svc := NewSiteService(nil, nil)
	_, err := svc.GetSiteUserInfo(context.Background(), "  ")
	require.ErrorIs(t, err, ErrSiteNotFound)
}

func TestMapTorrentErr_GenericPassThrough(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().PauseTorrent("x").Return(assertGenericErr)
	svc := newTestService(t, "qb", mockDl)
	err := svc.Pause(context.Background(), "qb", "x")
	require.Error(t, err)
	// Not wrapped as ErrTorrentNotFound.
	assert.NotErrorIs(t, err, ErrTorrentNotFound)
	assert.Equal(t, assertGenericErr, err)
}

func TestTorrentService_ListByDownloader_GetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetTorrentsBy(gomock.Any()).Return(nil, assertGenericErr)
	svc := newTestService(t, "qb", mockDl)
	_, _, err := svc.ListByDownloader(context.Background(), "qb", 1, 20)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list torrents")
}

func TestTorrentService_ListByDownloader_DefaultsClamp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetTorrentsBy(gomock.Any()).Return(seedTorrents(2), nil)
	svc := newTestService(t, "qb", mockDl)
	items, total, err := svc.ListByDownloader(context.Background(), "qb", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, items, 2)
}

func TestTorrentService_Get_GenericError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetTorrent("x").Return(downloader.Torrent{}, assertGenericErr)
	svc := newTestService(t, "qb", mockDl)
	_, err := svc.Get(context.Background(), "qb", "x")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrTorrentNotFound)
}

var assertGenericErr = &genericErr{"boom"}

type genericErr struct{ s string }

func (e *genericErr) Error() string { return e.s }
