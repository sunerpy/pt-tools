// MIT License
// Copyright (c) 2025 pt-tools

// Coverage for TorrentService Pause/Resume/Delete/Get and their
// downloader-not-found + torrent-not-found error mappings.

package app

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestTorrentService_Pause(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().PauseTorrent("t1").Return(nil)
	svc := newTestService(t, "qb", mockDl)
	require.NoError(t, svc.Pause(context.Background(), "qb", "t1"))
}

func TestTorrentService_Pause_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().PauseTorrent("t1").Return(downloader.ErrTorrentNotFound)
	svc := newTestService(t, "qb", mockDl)
	err := svc.Pause(context.Background(), "qb", "t1")
	require.ErrorIs(t, err, ErrTorrentNotFound)
}

func TestTorrentService_Resume(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().ResumeTorrent("t2").Return(nil)
	svc := newTestService(t, "qb", mockDl)
	require.NoError(t, svc.Resume(context.Background(), "qb", "t2"))
}

func TestTorrentService_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().RemoveTorrent("t3", true).Return(nil)
	svc := newTestService(t, "qb", mockDl)
	require.NoError(t, svc.Delete(context.Background(), "qb", "t3", true))
}

func TestTorrentService_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetTorrent("t4").Return(downloader.Torrent{ID: "t4", Name: "N"}, nil)
	svc := newTestService(t, "qb", mockDl)
	dto, err := svc.Get(context.Background(), "qb", "t4")
	require.NoError(t, err)
	assert.Equal(t, "t4", dto.ID)
	assert.Equal(t, "N", dto.Name)
}

func TestTorrentService_UnknownDownloader(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	svc := newTestService(t, "qb", mockDl)

	require.ErrorIs(t, svc.Pause(context.Background(), "other", "x"), ErrDownloaderNotFound)
	require.ErrorIs(t, svc.Resume(context.Background(), "other", "x"), ErrDownloaderNotFound)
	require.ErrorIs(t, svc.Delete(context.Background(), "other", "x", false), ErrDownloaderNotFound)
	_, err := svc.Get(context.Background(), "other", "x")
	require.ErrorIs(t, err, ErrDownloaderNotFound)
}
