package home

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/health"
	"github.com/sunerpy/pt-tools/models"
)

func TestBuildStatus(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 4, 9, 0, 0, 0, time.Local)

	// 没体检过、不看备份、没有媒体服务器
	st, err := BuildStatus(ctx, StatusInput{DB: db, Now: now})
	require.NoError(t, err)
	assert.Equal(t, &Status{Servers: []ServerLine{}}, st)

	at := now.Add(-2 * time.Hour)
	for _, s := range []models.MediaServer{
		{ID: 1, Name: "家里的 Jellyfin", Kind: models.MediaServerJellyfin, URL: "http://jf:8096", Enabled: true, IndexedAt: &at, IndexMovies: 120, IndexSeries: 30, IndexEpisodes: 900},
		{ID: 2, Name: "Emby", Kind: models.MediaServerEmby, Enabled: true, IndexError: "HTTP 500 token=abcdefabcdefabcdefabcdef"},
		{ID: 3, Name: "关着", Kind: models.MediaServerEmby, Enabled: false},
	} {
		require.NoError(t, db.Create(&s).Error)
	}
	lastAt := now.Add(-5 * time.Hour)
	in := StatusInput{
		DB: db, Now: now,
		Health: &health.Report{At: at, OK: 10, Warn: 2, Fail: 1, Skip: 3},
		Backup: func() (health.BackupState, error) {
			return health.BackupState{Enabled: true, LastAt: &lastAt, LastOKAt: &lastAt}, nil
		},
		Test: func(_ context.Context, srv models.MediaServer) (health.ServerInfo, error) {
			if srv.ID == 2 {
				return health.ServerInfo{}, errors.New("连不上")
			}
			return health.ServerInfo{Version: " 10.11.2 "}, nil
		},
		HomeURL: func(srv models.MediaServer) string { return srv.URL + "/web/" },
	}
	st, err = BuildStatus(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, &HealthLine{At: at, OK: 10, Warn: 2, Fail: 1, Skip: 3}, st.Health, "报告里没有项时没有问题可写")
	assert.Equal(t, &BackupLine{Enabled: true, LastAt: &lastAt, LastOKAt: &lastAt, Days: 0}, st.Backup)
	require.Len(t, st.Servers, 2, "关着的不列")
	first := st.Servers[0]
	require.NotNil(t, first.IndexedAt)
	assert.True(t, first.IndexedAt.Equal(at))
	first.IndexedAt = nil
	assert.Equal(t, ServerLine{
		ID: 1, Name: "家里的 Jellyfin", Kind: models.MediaServerJellyfin, Version: "10.11.2", Online: true,
		Movies: 120, Series: 30, Episodes: 900, OpenURL: "http://jf:8096/web/",
	}, first)
	assert.False(t, st.Servers[1].Online)
	assert.Equal(t, "连不上", st.Servers[1].Error)
	assert.Contains(t, st.Servers[1].IndexError, "HTTP 500")
	assert.NotContains(t, st.Servers[1].IndexError, "abcdefabcdef", "索引的错误去掉凭证")

	// 备份失败；不测服务器时只给索引
	in.Backup = func() (health.BackupState, error) {
		return health.BackupState{Enabled: true, LastAt: &lastAt, LastError: "磁盘满了"}, nil
	}
	in.Test, in.HomeURL = nil, nil
	st, err = BuildStatus(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, &BackupLine{Enabled: true, LastAt: &lastAt, Error: "磁盘满了", Late: true, Days: -1}, st.Backup)
	assert.False(t, st.Servers[0].Online)
	assert.Empty(t, st.Servers[0].OpenURL)

	in.Backup = func() (health.BackupState, error) {
		return health.BackupState{}, errors.New("读取备份设置失败")
	}
	_, err = BuildStatus(ctx, in)
	require.EqualError(t, err, "读取备份设置失败")

	in.Backup = nil
	require.NoError(t, db.Migrator().DropTable(&models.MediaServer{}))
	_, err = BuildStatus(ctx, in)
	require.ErrorContains(t, err, "读取媒体服务器失败")
}

func TestFirstProblem(t *testing.T) {
	items := []health.Item{
		{Key: "tmdb", Title: "TMDB 接口", Status: health.StatusOK},
		{Key: "site:a", Title: "A", Status: health.StatusWarn, Detail: "Cookie 过期了"},
		{Key: "downloader:2", Title: "tr", Status: health.StatusFail, Detail: "连不上：connection refused"},
		{Key: "backup", Title: "备份", Status: health.StatusFail},
		{Key: "server:1", Title: "家里的 Jellyfin", Status: health.StatusFail, Detail: "连不上"},
		{Key: "library:1", Title: "媒体库「电影」", Status: health.StatusFail, Detail: "目录不可写"},
	}
	assert.Equal(t, "下载器「tr」：连不上：connection refused", firstProblem(&health.Report{Items: items}), "失败的先写；下载器的项写明是下载器")
	assert.Equal(t, "站点「A」：Cookie 过期了", firstProblem(&health.Report{Items: items[:2]}), "没有失败时写第一项提醒")
	assert.Equal(t, "备份", firstProblem(&health.Report{Items: items[3:4]}), "没有说明时只写标题")
	assert.Equal(t, "媒体服务器「家里的 Jellyfin」：连不上", firstProblem(&health.Report{Items: items[4:5]}))
	assert.Equal(t, "NAS「家里的 TrueNAS」：存储池不健康：Cold（DEGRADED）", firstProblem(&health.Report{Items: []health.Item{
		{Key: "nas:3", Title: "家里的 TrueNAS", Status: health.StatusFail, Detail: "存储池不健康：Cold（DEGRADED）"},
	}}), "NAS 的项写明是 NAS")
	assert.Equal(t, "NAS：还没添加 NAS 来源", firstProblem(&health.Report{Items: []health.Item{
		{Key: "nas", Title: "NAS", Status: health.StatusWarn, Detail: "还没添加 NAS 来源"},
	}}), "整组的那一项不套")
	assert.Equal(t, "媒体库「电影」：目录不可写", firstProblem(&health.Report{Items: items[5:]}), "标题里已经写了是哪一类的照写")
	assert.Equal(t, "下载器：没有启用的下载器", firstProblem(&health.Report{Items: []health.Item{
		{Key: "downloaders", Title: "下载器", Status: health.StatusWarn, Detail: "没有启用的下载器"},
	}}), "整组的那一项不再套一层")
	assert.Empty(t, firstProblem(&health.Report{Items: items[:1]}))
	assert.Equal(t, "下载器：连不上 passkey=***", firstProblem(&health.Report{Items: []health.Item{
		{Title: "下载器", Status: health.StatusFail, Detail: "连不上 passkey=0123456789abcdef0123456789abcdef"},
	}}), "说明里的凭证去掉")
}
