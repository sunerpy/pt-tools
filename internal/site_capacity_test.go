// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestTorrentBelongsToSite(t *testing.T) {
	cases := []struct {
		name     string
		siteName string
		torrent  downloader.Torrent
		want     bool
	}{
		{"category 精确匹配", "mteam", downloader.Torrent{Category: "mteam"}, true},
		{"category 大小写不敏感", "MTeam", downloader.Torrent{Category: "mteam"}, true},
		{"category 带空格 trim", "mteam", downloader.Torrent{Category: " mteam "}, true},
		{"tags 单标签匹配", "hdsky", downloader.Torrent{Tags: "hdsky"}, true},
		{"tags 多标签命中其一", "hdsky", downloader.Torrent{Tags: "movie, hdsky, 4k"}, true},
		{"tags 大小写不敏感", "HDSky", downloader.Torrent{Tags: "hdsky"}, true},
		{"category 与 tags 均不匹配", "mteam", downloader.Torrent{Category: "other", Tags: "a,b"}, false},
		{"空 category 空 tags", "mteam", downloader.Torrent{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, torrentBelongsToSite(c.siteName, c.torrent))
		})
	}
}

func TestSumSiteSeedingSize(t *testing.T) {
	torrents := []downloader.Torrent{
		{Category: "mteam", TotalSize: 10 * gibiByte},
		{Tags: "mteam", TotalSize: 5 * gibiByte},
		{Category: "hdsky", TotalSize: 100 * gibiByte},
		{Category: "other", Tags: "x,y", TotalSize: 200 * gibiByte},
	}

	t.Run("聚合 mteam（category + tag 命中）", func(t *testing.T) {
		got := sumSiteSeedingSize("mteam", torrents)
		assert.Equal(t, int64(15*gibiByte), got)
	})
	t.Run("聚合 hdsky", func(t *testing.T) {
		got := sumSiteSeedingSize("hdsky", torrents)
		assert.Equal(t, int64(100*gibiByte), got)
	})
	t.Run("无命中返回 0", func(t *testing.T) {
		assert.Equal(t, int64(0), sumSiteSeedingSize("nosuch", torrents))
	})
	t.Run("空 siteName 返回 0", func(t *testing.T) {
		assert.Equal(t, int64(0), sumSiteSeedingSize("", torrents))
	})
	t.Run("空列表返回 0", func(t *testing.T) {
		assert.Equal(t, int64(0), sumSiteSeedingSize("mteam", nil))
	})
}

// TestGetSiteSeedingSizeBytes 验证 getSiteSeedingSizeBytes 是 GetAllTorrents +
// sumSiteSeedingSize 的组合：nil 下载器返回 0；注入固定种子列表求和；错误透传。
func TestGetSiteSeedingSizeBytes(t *testing.T) {
	ctx := context.Background()

	t.Run("nil 下载器返回 0 且无错误", func(t *testing.T) {
		got, err := getSiteSeedingSizeBytes(ctx, "mteam", nil)
		require.NoError(t, err)
		assert.Equal(t, int64(0), got)
	})

	t.Run("聚合命中站点的做种总量", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		dl := sm.NewMockDownloader(ctrl)
		dl.EXPECT().GetAllTorrents().Return([]downloader.Torrent{
			{Category: "mteam", TotalSize: 10 * gibiByte},
			{Tags: "free,mteam", TotalSize: 5 * gibiByte},
			{Category: "hdsky", TotalSize: 100 * gibiByte},
		}, nil)

		got, err := getSiteSeedingSizeBytes(ctx, "mteam", dl)
		require.NoError(t, err)
		assert.Equal(t, int64(15*gibiByte), got)
	})

	t.Run("GetAllTorrents 失败时透传 error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		dl := sm.NewMockDownloader(ctrl)
		wantErr := errors.New("下载器不可达")
		dl.EXPECT().GetAllTorrents().Return(nil, wantErr)

		got, err := getSiteSeedingSizeBytes(ctx, "mteam", dl)
		require.ErrorIs(t, err, wantErr)
		assert.Equal(t, int64(0), got)
	})
}
