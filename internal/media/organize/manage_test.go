package organize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

func TestSettingsDefaultsAndValidation(t *testing.T) {
	e := newEnv(t)
	got, err := e.svc.Settings(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, Settings{
		ScanIntervalMin: DefaultScanIntervalMin, MinVideoMB: DefaultMinVideoMB,
		Downloaders: []uint{}, Categories: []string{}, Tags: []string{}, SavePaths: []string{}, NotifyChannels: []uint{},
	}, got)

	got = e.settings(SettingsInput{
		AutoEnabled: true, ScanEnabled: true, ScanIntervalMin: 30, Downloaders: []uint{1, 1}, Categories: []string{" movies ", "tv", "movies"},
		Tags: []string{"keep"}, SavePaths: []string{"/downloads", "D:\\Downloads"}, MinVideoMB: 100, NotifyChannels: []uint{3}, DeleteLinksOnRemove: true,
	})
	assert.Equal(t, []uint{1}, got.Downloaders)
	assert.Equal(t, []string{"movies", "tv"}, got.Categories)
	assert.Equal(t, []string{"/downloads", "D:\\Downloads"}, got.SavePaths)
	require.NotNil(t, got.AutoSince)
	assert.Equal(t, e.now, got.AutoSince.UTC(), "打开自动整理时记下时间")

	e.advance(time.Hour)
	again := e.settings(SettingsInput{AutoEnabled: true})
	assert.Equal(t, e.now.Add(-time.Hour), again.AutoSince.UTC(), "一直开着时不改")
	assert.Equal(t, DefaultScanIntervalMin, again.ScanIntervalMin)
	assert.Equal(t, DefaultMinVideoMB, again.MinVideoMB)
	off := e.settings(SettingsInput{AutoEnabled: false})
	assert.Nil(t, off.AutoSince, "关掉时清掉")
	on := e.settings(SettingsInput{AutoEnabled: true})
	assert.Equal(t, e.now, on.AutoSince.UTC(), "重新打开时重新记")

	for _, bad := range []SettingsInput{
		{ScanIntervalMin: 5},
		{ScanIntervalMin: 2000},
		{MinVideoMB: -1},
		{MinVideoMB: MaxMinVideoMB + 1},
		{Downloaders: []uint{0}},
		{NotifyChannels: []uint{0}},
		{Categories: []string{"a,b"}},
		{Tags: []string{strings.Repeat("x", 600)}},
		{SavePaths: []string{"a\nb"}},
		{Categories: func() []string {
			var out []string
			for i := range maxScopeItems + 1 {
				out = append(out, strings.Repeat("c", i+1))
			}
			return out
		}()},
	} {
		_, err := e.svc.SaveSettings(e.ctx, bad)
		require.ErrorIs(t, err, ErrInvalid, bad)
	}
}

func TestLibrariesCRUD(t *testing.T) {
	e := newEnv(t)
	v := e.library(LibraryInput{Name: " 电影 ", Kind: models.MediaKindMovie, Path: e.movies + "/", Scrape: false, ScrapeOverwrite: true})
	assert.Equal(t, "电影", v.Name)
	assert.Equal(t, e.movies, v.Path, "路径规整")
	assert.Equal(t, models.MediaModeHardlink, v.Mode, "默认硬链接")
	assert.False(t, v.ScrapeOverwrite, "没开刮削时覆盖也是关的")
	assert.Equal(t, transfer.DefaultMovieTemplate, v.EffectiveTemplate)
	assert.Equal(t, "奥本海默 (2023)/奥本海默 (2023) - 2160p BluRay HDR10 H.265", v.Preview)

	upd, err := e.svc.SaveLibrary(e.ctx, v.ID, LibraryInput{Name: "电影库", Kind: models.MediaKindMovie, Path: e.movies, Template: "{{.Title}}", Mode: "copy", Enabled: false})
	require.NoError(t, err)
	assert.Equal(t, "电影库", upd.Name)
	assert.Equal(t, "copy", upd.Mode)
	assert.Equal(t, "奥本海默", upd.Preview)
	assert.False(t, upd.Enabled)

	list, err := e.svc.Libraries(e.ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	_, err = e.svc.SaveLibrary(e.ctx, 0, LibraryInput{Name: "电影库", Kind: models.MediaKindTV, Path: e.tv})
	require.ErrorIs(t, err, ErrInvalid, "名称重复")
	_, err = e.svc.SaveLibrary(e.ctx, 99, LibraryInput{Name: "x", Kind: models.MediaKindTV, Path: e.tv})
	require.ErrorIs(t, err, ErrNotFound)
	for _, bad := range []LibraryInput{
		{Name: "", Kind: "movie", Path: e.movies},
		{Name: strings.Repeat("长", 65), Kind: "movie", Path: e.movies},
		{Name: "x", Kind: "music", Path: e.movies},
		{Name: "x", Kind: "movie", Path: "relative/dir"},
		{Name: "x", Kind: "movie", Path: "/"},
		{Name: "x", Kind: "movie", Path: "C:\\"},
		{Name: "x", Kind: "movie", Path: e.movies, Mode: "rsync"},
		{Name: "x", Kind: "movie", Path: e.movies, Template: "{{.Nope}}"},
		{Name: "x", Kind: "movie", Path: e.movies, Template: strings.Repeat("a", 1025)},
	} {
		_, err := e.svc.SaveLibrary(e.ctx, 0, bad)
		require.ErrorIs(t, err, ErrInvalid, bad)
	}
	require.NoError(t, e.svc.DeleteLibrary(e.ctx, v.ID))
	require.ErrorIs(t, e.svc.DeleteLibrary(e.ctx, v.ID), ErrNotFound)
	assert.True(t, isAbs(`\\nas\share`))
	assert.True(t, isAbs(`D:/media`))
}

func TestPathMapsCRUD(t *testing.T) {
	e := newEnv(t)
	maps, err := e.svc.PathMaps(e.ctx)
	require.NoError(t, err)
	require.Len(t, maps, 1, "newEnv 建了一条")

	m, err := e.svc.SavePathMap(e.ctx, 0, PathMapInput{DownloaderID: 1, DownloaderPrefix: "/data/tv", LocalPrefix: "/mnt/tv/"})
	require.NoError(t, err)
	assert.Equal(t, "/mnt/tv", m.LocalPrefix)
	local, ok, err := e.svc.localPath(e.ctx, 1, "/data/tv/Show/S01E01.mkv")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "/mnt/tv/Show/S01E01.mkv", local)
	local, ok, err = e.svc.localPath(e.ctx, 1, "/other/x.mkv")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "/other/x.mkv", local, "没有匹配时原样返回")

	m2, err := e.svc.SavePathMap(e.ctx, m.ID, PathMapInput{DownloaderID: 1, DownloaderPrefix: "/data/series", LocalPrefix: "/mnt/series"})
	require.NoError(t, err)
	assert.Equal(t, "/data/series", m2.DownloaderPrefix)
	_, err = e.svc.SavePathMap(e.ctx, 0, PathMapInput{DownloaderID: 1, DownloaderPrefix: "/data/series", LocalPrefix: "/mnt/x"})
	require.ErrorIs(t, err, ErrInvalid, "同一个下载器的同一个前缀只能有一条")
	_, err = e.svc.SavePathMap(e.ctx, 0, PathMapInput{DownloaderID: 9, DownloaderPrefix: "/x", LocalPrefix: "/y"})
	require.ErrorIs(t, err, ErrInvalid, "下载器不存在")
	_, err = e.svc.SavePathMap(e.ctx, 0, PathMapInput{DownloaderID: 1, DownloaderPrefix: " ", LocalPrefix: "/y"})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.SavePathMap(e.ctx, 0, PathMapInput{DownloaderID: 1, DownloaderPrefix: "/x", LocalPrefix: "y"})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.SavePathMap(e.ctx, 99, PathMapInput{DownloaderID: 1, DownloaderPrefix: "/z", LocalPrefix: "/y"})
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, e.svc.DeletePathMap(e.ctx, m.ID))
	require.ErrorIs(t, e.svc.DeletePathMap(e.ctx, m.ID), ErrNotFound)
}

func TestServersCRUDAndTest(t *testing.T) {
	e := newEnv(t)
	emby := newFakeEmby(t)
	_, err := e.svc.SaveServer(e.ctx, 0, ServerInput{Name: "Emby", Kind: models.MediaServerEmby, URL: emby.URL})
	require.ErrorIs(t, err, ErrInvalid, "新建时要填 Token")
	key := "embykey"
	v, err := e.svc.SaveServer(e.ctx, 0, ServerInput{Name: "Emby", Kind: models.MediaServerEmby, URL: emby.URL + "/", Token: &key, Enabled: true})
	require.NoError(t, err)
	assert.True(t, v.HasToken)
	assert.Equal(t, emby.URL, v.URL)
	assert.Equal(t, models.MediaRefreshPath, v.RefreshMode)
	var row models.MediaServer
	require.NoError(t, e.db.First(&row, v.ID).Error)
	assert.Equal(t, "enc:embykey", row.TokenEncrypted, "Token 加密保存")

	// 不带 Token 修改：保留原来的
	upd, err := e.svc.SaveServer(e.ctx, v.ID, ServerInput{Name: "Emby 2", Kind: models.MediaServerEmby, URL: emby.URL, RefreshMode: "library", LocalPrefix: "/media", ServerPrefix: "/data"})
	require.NoError(t, err)
	assert.True(t, upd.HasToken)
	assert.Equal(t, "library", upd.RefreshMode)
	sp, ok := serverPath(upd.MediaServer, "/media/movies/x")
	assert.True(t, ok)
	assert.Equal(t, "/data/movies/x", sp)
	_, ok = serverPath(upd.MediaServer, "/elsewhere")
	assert.False(t, ok, "配置了路径映射而路径不在里面")
	sp, ok = serverPath(models.MediaServer{}, "/same")
	assert.True(t, ok, "没配路径映射时原样")
	assert.Equal(t, "/same", sp)

	info, err := e.svc.TestServer(e.ctx, ServerTestInput{ID: v.ID, Kind: models.MediaServerEmby, URL: emby.URL})
	require.NoError(t, err, "没填 Token 时用保存的")
	assert.Equal(t, "Emby", info.Name)
	wrong := "wrong"
	_, err = e.svc.TestServer(e.ctx, ServerTestInput{ID: v.ID, Kind: models.MediaServerEmby, URL: emby.URL, Token: &wrong})
	require.Error(t, err, "填了 Token 时用填的")
	_, err = e.svc.TestServer(e.ctx, ServerTestInput{Kind: models.MediaServerEmby, URL: emby.URL})
	require.ErrorIs(t, err, ErrInvalid, "没有 Token")
	_, err = e.svc.TestServer(e.ctx, ServerTestInput{ID: 99, Kind: models.MediaServerEmby, URL: emby.URL})
	require.ErrorIs(t, err, ErrNotFound)

	list, err := e.svc.Servers(e.ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	for _, bad := range []ServerInput{
		{Name: "", Kind: "emby", URL: emby.URL, Token: &key},
		{Name: "x", Kind: "kodi", URL: emby.URL, Token: &key},
		{Name: "x", Kind: "emby", URL: "ftp://x", Token: &key},
		{Name: "x", Kind: "emby", URL: emby.URL, Token: &key, RefreshMode: "all"},
		{Name: "x", Kind: "emby", URL: emby.URL, Token: &key, LocalPrefix: "/a"},
		{Name: "x", Kind: "emby", URL: emby.URL, Token: func() *string { s := " "; return &s }()},
		{Name: "x", Kind: "emby", URL: emby.URL, Token: func() *string { s := `a"b`; return &s }()},
		{Name: "Emby 2", Kind: "emby", URL: emby.URL, Token: &key},
	} {
		_, saveErr := e.svc.SaveServer(e.ctx, 0, bad)
		require.ErrorIs(t, saveErr, ErrInvalid, bad.Name+bad.Kind)
	}
	_, err = e.svc.SaveServer(e.ctx, 99, ServerInput{Name: "y", Kind: "emby", URL: emby.URL})
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, e.svc.DeleteServer(e.ctx, v.ID))
	require.ErrorIs(t, e.svc.DeleteServer(e.ctx, v.ID), ErrNotFound)
}

func TestCheckLibrary(t *testing.T) {
	e := newEnv(t)
	items, err := e.svc.CheckLibrary(e.ctx, LibraryCheckInput{Path: e.movies})
	require.NoError(t, err)
	require.Len(t, items, 3)
	for _, it := range items {
		assert.True(t, it.OK, it)
	}
	assert.Equal(t, "硬链接："+e.dlDir, items[2].Name)
	left, _ := filepath.Glob(filepath.Join(e.movies, ".pt-tools-probe-*"))
	assert.Empty(t, left, "检查完删掉试建的文件")
	left, _ = filepath.Glob(filepath.Join(e.dlDir, ".pt-tools-probe-*"))
	assert.Empty(t, left)

	items, err = e.svc.CheckLibrary(e.ctx, LibraryCheckInput{Path: e.movies, Mode: "copy"})
	require.NoError(t, err)
	assert.Len(t, items, 2, "不是硬链接时不试建链接")

	items, err = e.svc.CheckLibrary(e.ctx, LibraryCheckInput{Path: filepath.Join(e.root, "nope")})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.False(t, items[0].OK)
	assert.Contains(t, items[0].Message, "Docker")

	file := filepath.Join(e.root, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o644))
	items, err = e.svc.CheckLibrary(e.ctx, LibraryCheckInput{Path: file})
	require.NoError(t, err)
	assert.Contains(t, items[0].Message, "不是目录")

	maps, _ := e.svc.PathMaps(e.ctx)
	require.NoError(t, e.svc.DeletePathMap(e.ctx, maps[0].ID))
	items, err = e.svc.CheckLibrary(e.ctx, LibraryCheckInput{Path: e.movies})
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Contains(t, items[2].Message, "没有路径映射")

	_, err = e.svc.SavePathMap(e.ctx, 0, PathMapInput{DownloaderID: 1, DownloaderPrefix: "/x", LocalPrefix: filepath.Join(e.root, "missing")})
	require.NoError(t, err)
	items, err = e.svc.CheckLibrary(e.ctx, LibraryCheckInput{Path: e.movies})
	require.NoError(t, err)
	assert.False(t, items[2].OK)
	assert.Contains(t, items[2].Message, "下载目录里建文件失败")

	_, err = e.svc.CheckLibrary(e.ctx, LibraryCheckInput{Path: "rel"})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.CheckLibrary(e.ctx, LibraryCheckInput{Path: e.movies, Mode: "x"})
	require.ErrorIs(t, err, ErrInvalid)
}

// 订阅问媒体服务器条目在不在：有一台说有就算有；没有启用的媒体服务器时是没有
func TestInLibrary(t *testing.T) {
	e := newEnv(t)
	ok, err := e.svc.InLibrary(e.ctx, models.MediaKindMovie, 872585, "", "奥本海默")
	require.NoError(t, err)
	assert.False(t, ok, "没有媒体服务器")
	emby := newFakeEmby(t)
	key := "embykey"
	_, err = e.svc.SaveServer(e.ctx, 0, ServerInput{Name: "Emby", Kind: models.MediaServerEmby, URL: emby.URL, Token: &key, Enabled: true})
	require.NoError(t, err)
	ok, err = e.svc.InLibrary(e.ctx, models.MediaKindMovie, 872585, "", "奥本海默")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = e.svc.InLibrary(e.ctx, models.MediaKindMovie, 1, "", "别的")
	require.NoError(t, err)
	assert.False(t, ok)
}
