package internal

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 推送时说明的条目写进种子记录：合法的覆盖旧值，电影不记季；不带或不合法的不动库里的值；
// 下载器里已有这个种子时只更新已有的记录，不为条目新建一条。
func TestPushTorrentToDownloader_Media(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "qb-main", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.DB.Create(&ds).Error)
	disableDiskProtect(t, db)

	ctrl := gomock.NewController(t)
	dl := sm.NewMockDownloader(ctrl)
	dl.EXPECT().CheckTorrentExists(gomock.Any()).Return(false, nil).Times(4)
	dl.EXPECT().CheckTorrentExists(gomock.Any()).Return(true, nil).Times(2)
	dl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).Return(downloader.AddTorrentResult{Success: true, Hash: "h"}, nil).Times(4)
	dl.EXPECT().Close().Return(nil).Times(6)
	restore := SwapPushDownloaderFactory(func(models.DownloaderSetting) (downloader.Downloader, error) { return dl, nil })
	t.Cleanup(restore)
	data := makeSizedTorrentBytes(t, "m", gb)
	push := func(id string, m *PushMedia) *PushTorrentResult {
		t.Helper()
		res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{SiteID: "hdsky", TorrentID: id, TorrentData: data, DownloaderID: ds.ID, Media: m})
		require.NoError(t, err)
		return res
	}
	media := func(id string) (string, int, int) {
		t.Helper()
		ti, err := db.GetTorrentBySiteAndID("hdsky", id)
		require.NoError(t, err)
		require.NotNil(t, ti)
		return ti.MediaType, ti.TMDBID, ti.MediaSeason
	}

	require.True(t, push("m1", &PushMedia{Type: models.MediaKindTV, TMDBID: 100088, Season: 2}).Success)
	kind, id, season := media("m1")
	assert.Equal(t, models.MediaKindTV, kind)
	assert.Equal(t, 100088, id)
	assert.Equal(t, 2, season)

	push("m1", nil)
	kind, id, season = media("m1")
	assert.Equal(t, []any{models.MediaKindTV, 100088, 2}, []any{kind, id, season}, "不带条目的推送不清掉")

	push("m1", &PushMedia{Type: "book", TMDBID: 1})
	kind, id, _ = media("m1")
	assert.Equal(t, []any{models.MediaKindTV, 100088}, []any{kind, id}, "不合法的条目不写")

	push("m1", &PushMedia{Type: models.MediaKindMovie, TMDBID: 872585, Season: 3})
	kind, id, season = media("m1")
	assert.Equal(t, []any{models.MediaKindMovie, 872585, 0}, []any{kind, id, season}, "合法的覆盖旧值，电影不记季")

	res := push("m1", &PushMedia{Type: models.MediaKindTV, TMDBID: 209867, Season: 1})
	assert.True(t, res.Skipped)
	kind, id, season = media("m1")
	assert.Equal(t, []any{models.MediaKindTV, 209867, 1}, []any{kind, id, season}, "下载器里已有：更新已有的记录")

	res = push("m2", &PushMedia{Type: models.MediaKindTV, TMDBID: 209867, Season: 1})
	assert.True(t, res.Skipped)
	ti, err := db.GetTorrentBySiteAndID("hdsky", "m2")
	require.NoError(t, err)
	assert.Nil(t, ti, "不为条目新建记录")
}
