// MIT License
// Copyright (c) 2025 pt-tools

// Fills remaining app-package gaps: RSSRetryWorker invalid-payload failure,
// TorrentService pagination past the last page, and SiteService.GetSiteUserInfo
// using LevelName (classOf) with a stub UserInfoSource.

package app

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestRSSRetryWorker_InvalidPayloadMarksFailed(t *testing.T) {
	db := setupRetryWorkerDB(t)
	past := time.Now().Add(-time.Minute)
	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "bad", NotifyKind: "all",
		NotificationConfID: 1, Result: "pending", Attempts: 0,
		NextRetryAt: &past,
		PayloadJSON: "{not valid json",
		CreatedAt:   time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)

	w := NewRSSRetryWorker(db, &fakePushSvc{})
	require.NoError(t, w.drainOnce(context.Background()))

	var got models.RSSNotificationLog
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Equal(t, "failed", got.Result)
}

func TestTorrentService_ListByDownloader_PageBeyondEnd(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetTorrentsBy(gomock.Any()).Return(seedTorrents(3), nil)
	svc := newTestService(t, "qb", mockDl)

	items, total, err := svc.ListByDownloader(context.Background(), "qb", 99, 20)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Empty(t, items)
}

func TestSiteService_GetSiteUserInfo_UsesLevelName(t *testing.T) {
	store := &stubSiteLister{sites: map[models.SiteGroup]models.SiteConfig{"mteam": {}}}
	users := &stubUserInfoSource{infos: map[string]v2.UserInfo{
		"mteam": {Site: "mteam", Username: "bob", LevelName: "Elite", Uploaded: 1 << 30, Downloaded: 1 << 20, Ratio: 2.5, Bonus: 100},
	}}
	svc := newSiteServiceWithDeps(store, users)

	dto, err := svc.GetSiteUserInfo(context.Background(), "mteam")
	require.NoError(t, err)
	assert.Equal(t, "bob", dto.Username)
	assert.Equal(t, "Elite", dto.Class)
}

func TestSiteService_GetSiteUserInfo_Errors(t *testing.T) {
	store := &stubSiteLister{sites: map[models.SiteGroup]models.SiteConfig{"mteam": {}}}
	svc := newSiteServiceWithDeps(store, nil)

	_, err := svc.GetSiteUserInfo(context.Background(), "")
	require.ErrorIs(t, err, ErrSiteNotFound)

	_, err = svc.GetSiteUserInfo(context.Background(), "unknown")
	require.ErrorIs(t, err, ErrSiteNotFound)

	// users nil -> unavailable for a known site.
	_, err = svc.GetSiteUserInfo(context.Background(), "mteam")
	require.ErrorIs(t, err, ErrUserInfoUnavailable)
}
