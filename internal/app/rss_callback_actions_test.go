// MIT License
// Copyright (c) 2025 pt-tools

// Coverage for RSSCallbackActions: OnRSSIgnore (suppress + not-found),
// OnRSSDownload guard paths (nil-db, missing row, unwired fetcher, missing
// subscription, no default downloader, fetcher error, empty data), and the
// resolveDownloaderID branches.

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

func setupCallbackDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.RSSNotificationLog{},
		&models.RSSSubscription{},
		&models.DownloaderSetting{},
	))
	return db
}

func TestRSSCallbackActions_NilDB(t *testing.T) {
	a := NewRSSCallbackActions(nil, nil)
	require.Error(t, a.OnRSSIgnore(context.Background(), 1, 0))
	require.Error(t, a.OnRSSDownload(context.Background(), 1, 0))
}

func TestOnRSSIgnore_SuppressesRow(t *testing.T) {
	db := setupCallbackDB(t)
	a := NewRSSCallbackActions(db, nil)

	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "t", NotifyKind: "all",
		NotificationConfID: 1, Result: "sent", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)

	require.NoError(t, a.OnRSSIgnore(context.Background(), row.ID, 42))
	var got models.RSSNotificationLog
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Equal(t, "suppressed", got.Result)
}

func TestOnRSSIgnore_NotFound(t *testing.T) {
	db := setupCallbackDB(t)
	a := NewRSSCallbackActions(db, nil)
	err := a.OnRSSIgnore(context.Background(), 999, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不存在")
}

func TestOnRSSDownload_MissingRow(t *testing.T) {
	db := setupCallbackDB(t)
	a := NewRSSCallbackActions(db, nil)
	err := a.OnRSSDownload(context.Background(), 12345, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不存在")
}

func TestOnRSSDownload_FetcherUnwired(t *testing.T) {
	db := setupCallbackDB(t)
	a := NewRSSCallbackActions(db, nil) // fetcher nil

	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "t", NotifyKind: "all",
		NotificationConfID: 1, Result: "sent", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)

	err := a.OnRSSDownload(context.Background(), row.ID, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未接线")

	var got models.RSSNotificationLog
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Contains(t, got.LastError, "未接线")
}

func TestOnRSSDownload_MissingSubscription(t *testing.T) {
	db := setupCallbackDB(t)
	fetcher := func(_ context.Context, _, _ string) ([]byte, error) { return []byte("x"), nil }
	a := NewRSSCallbackActions(db, fetcher)

	row := models.RSSNotificationLog{
		RSSID: 777, SiteName: "s", TorrentID: "t", NotifyKind: "all",
		NotificationConfID: 1, Result: "sent", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)

	err := a.OnRSSDownload(context.Background(), row.ID, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RSS 订阅")
}

func TestOnRSSDownload_NoDefaultDownloader(t *testing.T) {
	db := setupCallbackDB(t)
	fetcher := func(_ context.Context, _, _ string) ([]byte, error) { return []byte("x"), nil }
	a := NewRSSCallbackActions(db, fetcher)

	sub := models.RSSSubscription{Name: "r", URL: "http://x", IntervalMinutes: 1}
	require.NoError(t, db.Create(&sub).Error)
	row := models.RSSNotificationLog{
		RSSID: sub.ID, SiteName: "s", TorrentID: "t", NotifyKind: "all",
		NotificationConfID: 1, Result: "sent", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)

	err := a.OnRSSDownload(context.Background(), row.ID, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "默认下载器")
}

func TestOnRSSDownload_FetcherError(t *testing.T) {
	db := setupCallbackDB(t)
	fetcher := func(_ context.Context, _, _ string) ([]byte, error) {
		return nil, errors.New("fetch boom")
	}
	a := NewRSSCallbackActions(db, fetcher)

	dlID := uint(3)
	ds := models.DownloaderSetting{Name: "d", Type: "qbittorrent", URL: "http://x", Enabled: true}
	ds.ID = dlID
	require.NoError(t, db.Create(&ds).Error)
	sub := models.RSSSubscription{Name: "r", URL: "http://x", IntervalMinutes: 1, DownloaderID: &dlID}
	require.NoError(t, db.Create(&sub).Error)
	row := models.RSSNotificationLog{
		RSSID: sub.ID, SiteName: "s", TorrentID: "t", NotifyKind: "all",
		NotificationConfID: 1, Result: "sent", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)

	err := a.OnRSSDownload(context.Background(), row.ID, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "下载种子文件失败")

	var got models.RSSNotificationLog
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Contains(t, got.LastError, "fetch boom")
}

func TestOnRSSDownload_EmptyData(t *testing.T) {
	db := setupCallbackDB(t)
	fetcher := func(_ context.Context, _, _ string) ([]byte, error) { return []byte{}, nil }
	a := NewRSSCallbackActions(db, fetcher)

	dlID := uint(4)
	ds := models.DownloaderSetting{Name: "d", Type: "qbittorrent", URL: "http://x", Enabled: true}
	ds.ID = dlID
	require.NoError(t, db.Create(&ds).Error)
	sub := models.RSSSubscription{Name: "r", URL: "http://x", IntervalMinutes: 1, DownloaderID: &dlID}
	require.NoError(t, db.Create(&sub).Error)
	row := models.RSSNotificationLog{
		RSSID: sub.ID, SiteName: "s", TorrentID: "t", NotifyKind: "all",
		NotificationConfID: 1, Result: "sent", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)

	err := a.OnRSSDownload(context.Background(), row.ID, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "空数据")
}
