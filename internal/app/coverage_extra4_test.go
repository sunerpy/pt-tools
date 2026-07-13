// MIT License
// Copyright (c) 2025 pt-tools

package app

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestTorrentService_Resume_NotFoundAndUnknown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().ResumeTorrent("x").Return(downloader.ErrTorrentNotFound)
	svc := newTestService(t, "qb", mockDl)
	require.ErrorIs(t, svc.Resume(context.Background(), "qb", "x"), ErrTorrentNotFound)
}

func TestTorrentService_Delete_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().RemoveTorrent("x", false).Return(downloader.ErrTorrentNotFound)
	svc := newTestService(t, "qb", mockDl)
	require.ErrorIs(t, svc.Delete(context.Background(), "qb", "x", false), ErrTorrentNotFound)
}

func TestListSites_UserGetError(t *testing.T) {
	store := &stubSiteLister{sites: map[models.SiteGroup]models.SiteConfig{"mteam": {Enabled: boolPtr(true)}}}
	users := &stubUserInfoSource{err: v2.ErrSiteNotFound}
	svc := newSiteServiceWithDeps(store, users)
	got, err := svc.ListSites(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.True(t, got[0].LastScrapedAt.IsZero())
}

func TestGetSiteUserInfo_ListError(t *testing.T) {
	store := &stubSiteLister{err: assertGenericErr}
	svc := newSiteServiceWithDeps(store, &stubUserInfoSource{})
	_, err := svc.GetSiteUserInfo(context.Background(), "mteam")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list sites")
}

func TestGetSiteUserInfo_UsersNil(t *testing.T) {
	store := &stubSiteLister{sites: map[models.SiteGroup]models.SiteConfig{"mteam": {}}}
	svc := newSiteServiceWithDeps(store, nil)
	_, err := svc.GetSiteUserInfo(context.Background(), "mteam")
	require.ErrorIs(t, err, ErrUserInfoUnavailable)
}

func TestNotifyFilteredItem_QuotaThrottles(t *testing.T) {
	db := setupRSSNotifierDB(t)
	n := NewRSSNotifier(db, &fakePushSvc{})
	rss := &models.RSSConfig{ID: 1, NotifyMode: "filtered", NotifyConfIDs: "[1]", MaxNotificationsPerHour: 1}
	require.NoError(t, db.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "old", NotifyKind: "filtered",
		NotificationConfID: 1, Result: "sent",
	}).Error)
	tItem := &v2.TorrentItem{ID: "t2", Title: "X", URL: "http://z"}
	require.NoError(t, n.NotifyFilteredItem(context.Background(), RSSFilteredEvent{
		RSS: rss, Torrent: tItem, SiteName: "s", TorrentID: "t2",
	}))
	var throttled int64
	require.NoError(t, db.Model(&models.RSSNotificationLog{}).
		Where("result = ?", "throttled").Count(&throttled).Error)
	assert.EqualValues(t, 1, throttled)
}

func TestNotifyFilteredItem_InvalidConfIDs(t *testing.T) {
	db := setupRSSNotifierDB(t)
	n := NewRSSNotifier(db, &fakePushSvc{})
	rss := &models.RSSConfig{ID: 1, NotifyMode: "filtered", NotifyConfIDs: "{bad"}
	tItem := &v2.TorrentItem{ID: "t", Title: "X"}
	require.Error(t, n.NotifyFilteredItem(context.Background(), RSSFilteredEvent{
		RSS: rss, Torrent: tItem, SiteName: "s", TorrentID: "t",
	}))
}

func TestConsumeCode_LookupError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.BotToken{}, &models.ChannelBinding{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	svc := NewBindingService(db, "admin")
	_, err = svc.ConsumeCode(context.Background(), "code", "telegram", "u")
	require.Error(t, err)
}

func TestListConfs_DBError(t *testing.T) {
	db := setupClosedNotifDB(t)
	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	_, err := svc.ListConfs(context.Background())
	require.Error(t, err)
}

func TestRevoke_DBError(t *testing.T) {
	db := setupClosedNotifDB(t)
	svc := NewBindingService(db, "admin")
	err := svc.Revoke(context.Background(), 1)
	require.Error(t, err)
}

func setupClosedNotifDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.NotificationConf{}, &models.ChannelBinding{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return db
}

func TestResolveDownloaderID_QueryError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.DownloaderSetting{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	a := NewRSSCallbackActions(db, nil)
	zero := uint(0)
	_, err = a.resolveDownloaderID(context.Background(), &zero)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "查询默认下载器")
}

func TestTestConf_TelegramDecryptError(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	row := models.NotificationConf{ChannelType: "telegram", Name: "tg", ConfigJSON: "corrupt-cipher", Enabled: true}
	require.NoError(t, db.Create(&row).Error)
	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	err := svc.TestConf(context.Background(), row.ID)
	require.Error(t, err)
}

func TestUpdateConf_CorruptExistingConfig(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	row := models.NotificationConf{ChannelType: "telegram", Name: "tg", ConfigJSON: "corrupt", Enabled: true}
	require.NoError(t, db.Create(&row).Error)
	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	err := svc.UpdateConf(context.Background(), row.ID, UpdateConfReq{ConfigJSON: []byte(`{"a":1}`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "解密旧配置")
}

func TestCreateConf_Persists(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	dto, err := svc.CreateConf(context.Background(), CreateConfReq{
		ChannelType: "telegram", Name: "tg", ConfigJSON: []byte(`{"bot_token":"x"}`), Enabled: true,
	})
	require.NoError(t, err)
	assert.NotZero(t, dto.ID)

	var row models.NotificationConf
	require.NoError(t, db.First(&row, dto.ID).Error)
	plain, err := crypto.Decrypt(row.ConfigJSON)
	require.NoError(t, err)
	assert.Contains(t, string(plain), "bot_token")
}
