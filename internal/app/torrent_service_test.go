// MIT License
// Copyright (c) 2025 pt-tools

package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

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

type stubDownloaderResolver struct {
	name string
	dl   downloader.Downloader
}

func (s *stubDownloaderResolver) GetDownloader(name string) (downloader.Downloader, error) {
	if name != s.name {
		return nil, fmt.Errorf("stub: unknown downloader %q", name)
	}
	return s.dl, nil
}

func seedTorrents(n int) []downloader.Torrent {
	out := make([]downloader.Torrent, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, downloader.Torrent{
			ID:        fmt.Sprintf("torrent-%d", i),
			InfoHash:  fmt.Sprintf("hash-%d", i),
			Name:      fmt.Sprintf("Torrent %d", i),
			Progress:  0.5,
			Ratio:     1.0,
			State:     downloader.TorrentDownloading,
			TotalSize: int64(1024 * 1024 * i),
			ETA:       3600,
			DateAdded: int64(1700000000 + i),
			Tracker:   "https://tracker.example.com/announce",
			Category:  "movies",
			Tags:      "auto",
		})
	}
	return out
}

func newTestService(t *testing.T, downloaderName string, dl downloader.Downloader) TorrentService {
	t.Helper()
	resolver := &stubDownloaderResolver{name: downloaderName, dl: dl}
	return newTorrentServiceWithResolver(resolver)
}

func TestListByDownloader_Pagination(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDl := mocks.NewMockDownloader(ctrl)
	all := seedTorrents(5000)
	mockDl.EXPECT().
		GetTorrentsBy(gomock.Any()).
		Return(all, nil).
		AnyTimes()

	svc := newTestService(t, "qb1", mockDl)
	ctx := context.Background()

	items, total, err := svc.ListByDownloader(ctx, "qb1", 2, 20)
	require.NoError(t, err)
	assert.Equal(t, 5000, total)
	assert.Len(t, items, 20)
	assert.Equal(t, "torrent-21", items[0].ID)
	assert.Equal(t, "torrent-40", items[19].ID)
}

func TestListByDownloader_DownloaderNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)

	svc := newTestService(t, "qb1", mockDl)
	_, _, err := svc.ListByDownloader(context.Background(), "qb-missing", 1, 20)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrDownloaderNotFound), "expect ErrDownloaderNotFound, got %v", err)
}

func TestGet_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDl := mocks.NewMockDownloader(ctrl)
	tr := downloader.Torrent{
		ID:        "abc123",
		Name:      "Sample.Movie.2024",
		Progress:  0.75,
		Ratio:     1.5,
		State:     downloader.TorrentSeeding,
		TotalSize: 5_000_000_000,
		ETA:       0,
		DateAdded: 1700000000,
		Tracker:   "https://secret.tracker/announce?passkey=SECRET",
	}
	mockDl.EXPECT().GetTorrent("abc123").Return(tr, nil)

	svc := newTestService(t, "qb1", mockDl)
	dto, err := svc.Get(context.Background(), "qb1", "abc123")
	require.NoError(t, err)
	assert.Equal(t, "abc123", dto.ID)
	assert.Equal(t, "Sample.Movie.2024", dto.Name)
	assert.Equal(t, "seeding", dto.State)
	assert.Equal(t, int64(5_000_000_000), dto.Size)
	assert.InDelta(t, 0.75, dto.Progress, 0.0001)
	assert.InDelta(t, 1.5, dto.Ratio, 0.0001)
	assert.Equal(t, int64(0), dto.ETA)
	assert.False(t, dto.AddedAt.IsZero())
}

func TestGet_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().
		GetTorrent("ghost").
		Return(downloader.Torrent{}, downloader.ErrTorrentNotFound)

	svc := newTestService(t, "qb1", mockDl)
	_, err := svc.Get(context.Background(), "qb1", "ghost")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrTorrentNotFound), "expect ErrTorrentNotFound, got %v", err)
}

func TestPause_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().
		PauseTorrent("nonexistent").
		Return(downloader.ErrTorrentNotFound)

	svc := newTestService(t, "qb1", mockDl)
	err := svc.Pause(context.Background(), "qb1", "nonexistent")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrTorrentNotFound), "expect ErrTorrentNotFound, got %v", err)
}

func TestDelete_RemoveData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().RemoveTorrent("t1", true).Return(nil)

	svc := newTestService(t, "qb1", mockDl)
	err := svc.Delete(context.Background(), "qb1", "t1", true)
	assert.NoError(t, err)
}

func TestResume_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().ResumeTorrent("t1").Return(nil)

	svc := newTestService(t, "qb1", mockDl)
	require.NoError(t, svc.Resume(context.Background(), "qb1", "t1"))
}

func TestListByDownloader_PageOutOfRange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDl := mocks.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetTorrentsBy(gomock.Any()).Return(seedTorrents(10), nil)

	svc := newTestService(t, "qb1", mockDl)
	items, total, err := svc.ListByDownloader(context.Background(), "qb1", 99, 20)
	require.NoError(t, err)
	assert.Equal(t, 10, total)
	assert.Empty(t, items)
}
