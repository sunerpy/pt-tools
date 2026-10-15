package organize

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/scrape"
	"github.com/sunerpy/pt-tools/models"
)

// 媒体库选了刮削内容与图片语言：只写选了的，海报与 Logo 按语言挑
func TestOrganizeScrapeItems(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	lib := e.library(LibraryInput{
		Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true,
		ScrapeItems: []string{"logo", "poster"}, ScrapeImageLang: "en",
	})
	assert.Equal(t, []string{scrape.ItemPoster, scrape.ItemLogo}, lib.ScrapeItems)
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeItems: []string{"nfo", "logo"}})
	e.oppenheimer()

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	assert.Empty(t, res.Messages)
	video := filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023) - 2160p BluRay HDR H.265.mkv")
	dir := filepath.Dir(video)
	assert.Equal(t, "JPEG/t/p/w780/opp-en.jpg", readFile(t, filepath.Join(dir, "poster.jpg")), "英文海报")
	assert.Equal(t, "JPEG/t/p/w500/opp-logo-en.png", readFile(t, filepath.Join(dir, "clearlogo.png")), "英文 Logo，SVG 不要")
	assert.False(t, exists(filepath.Join(dir, "fanart.jpg")), "没选背景")
	assert.False(t, exists(strings.TrimSuffix(video, ".mkv")+".nfo"), "没选 NFO")
	rows := e.history()
	require.Len(t, rows, 1)
	var meta []string
	for _, x := range decodeExtras(rows[0].Extras) {
		if x.Kind == extraMeta {
			meta = append(meta, filepath.Base(x.Target))
		}
	}
	assert.ElementsMatch(t, []string{"poster.jpg", "clearlogo.png"}, meta, "Logo 也记进刮削写的文件")

	e.lastOfUs()
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	require.Equal(t, 2, res.Created)
	assert.Empty(t, res.Messages)
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	e2 := filepath.Join(show, "Season 1", "最后生还者 - S01E02 - 感染.mkv")
	assert.True(t, exists(filepath.Join(show, "tvshow.nfo")))
	assert.True(t, exists(filepath.Join(show, "Season 1", "season.nfo")))
	assert.True(t, exists(strings.TrimSuffix(e2, ".mkv")+".nfo"))
	assert.Equal(t, "JPEG/t/p/w500/tlou-logo.png", readFile(t, filepath.Join(show, "clearlogo.png")), "没有中文 Logo 时用英文的")
	for _, f := range []string{"poster.jpg", "fanart.jpg", "season01-poster.jpg"} {
		assert.False(t, exists(filepath.Join(show, f)), f)
	}
	assert.False(t, exists(strings.TrimSuffix(e2, ".mkv")+"-thumb.jpg"), "没选集截图")
}

// 取不到图片列表：海报与背景用 TMDB 给的那张，不写 Logo，消息里写明
func TestOrganizeScrapeImagesDown(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{
		Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true,
		ScrapeItems: []string{"poster", "fanart", "logo"}, ScrapeImageLang: scrape.ImageLangNone,
	})
	e.tmdb.mu.Lock()
	e.tmdb.imagesDown = true
	e.tmdb.mu.Unlock()
	e.oppenheimer()

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	require.Len(t, res.Messages, 1)
	assert.Contains(t, res.Messages[0], "取 TMDB 图片列表失败")
	dir := filepath.Join(e.movies, "奥本海默 (2023)")
	assert.Equal(t, "JPEG/t/p/w780/opp.jpg", readFile(t, filepath.Join(dir, "poster.jpg")))
	assert.Equal(t, "JPEG/t/p/w1280/opp-bd.jpg", readFile(t, filepath.Join(dir, "fanart.jpg")))
	assert.False(t, exists(filepath.Join(dir, "clearlogo.png")))
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferDone, rows[0].Status)
}

// 剧集目录里的 Logo 跟海报一样：目录里还有别的集时留着，删到最后一集时一起删
func TestDeleteHistoryKeepsShowLogo(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeItems: []string{"nfo", "logo"}})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.lastOfUs()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	require.Equal(t, 2, res.Created)
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	logo := filepath.Join(show, "clearlogo.png")
	require.True(t, exists(logo))

	var ep1, ep2 models.MediaTransferHistory
	require.NoError(t, e.db.Where("episode = ?", 1).First(&ep1).Error)
	require.NoError(t, e.db.Where("episode = ?", 2).First(&ep2).Error)
	require.NoError(t, e.del(ep2.ID, true))
	assert.True(t, exists(logo), "目录里还有第 1 集")
	require.NoError(t, e.del(ep1.ID, true))
	assert.False(t, exists(logo))
	assert.False(t, exists(show), "剧集目录删干净")
}

// 开着覆盖、分两批整理同一部剧：后一批重写了目录里的 Logo 与海报。先删前一批时，过期的归属不能转给后一批、
// 盖掉后一批记下的文件编号；删到最后一批时这些文件照样删掉
func TestDeleteHistoryOverwrittenShowArt(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{
		Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeOverwrite: true,
		ScrapeItems: []string{"nfo", "poster", "logo"},
	})
	e.settings(SettingsInput{MinVideoMB: 1})
	const h1, h2 = "1111111111111111111111111111111111111111", "2222222222222222222222222222222222222222"
	e.addTorrent(h1, "The.Last.of.Us.S01E01.2160p", map[string]int{"The.Last.of.Us.S01E01.2160p/The.Last.of.Us.S01E01.2160p.mkv": 2}, nil)
	e.addTorrent(h2, "The.Last.of.Us.S01E02.2160p", map[string]int{"The.Last.of.Us.S01E02.2160p/The.Last.of.Us.S01E02.2160p.mkv": 2}, nil)
	for _, h := range []string{h1, h2} {
		res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: h})
		require.NoError(t, err)
		require.Equal(t, 1, res.Created, res.Plan.Problem)
	}
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	require.True(t, exists(filepath.Join(show, "clearlogo.png")))

	var ep1, ep2 models.MediaTransferHistory
	require.NoError(t, e.db.Where("info_hash = ?", h1).First(&ep1).Error)
	require.NoError(t, e.db.Where("info_hash = ?", h2).First(&ep2).Error)
	require.NoError(t, e.del(ep1.ID, true))
	assert.True(t, exists(filepath.Join(show, "clearlogo.png")), "目录里还有第 2 集")
	require.NoError(t, e.del(ep2.ID, true))
	for _, f := range []string{"clearlogo.png", "poster.jpg", "tvshow.nfo"} {
		assert.False(t, exists(filepath.Join(show, f)), f)
	}
	assert.False(t, exists(show), "剧集目录删干净")
}

// 转交目录里的刮削文件时读不了文件（权限、网络存储）：不能当成过期悄悄跳过，返回错误、记录留着；好了以后再删照常转交
func TestDeleteHistoryHandOverReadError(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeItems: []string{"nfo", "logo"}})
	e.settings(SettingsInput{MinVideoMB: 1})
	const h1, h2 = "1111111111111111111111111111111111111111", "2222222222222222222222222222222222222222"
	e.addTorrent(h1, "The.Last.of.Us.S01E01.2160p", map[string]int{"The.Last.of.Us.S01E01.2160p/The.Last.of.Us.S01E01.2160p.mkv": 2}, nil)
	e.addTorrent(h2, "The.Last.of.Us.S01E02.2160p", map[string]int{"The.Last.of.Us.S01E02.2160p/The.Last.of.Us.S01E02.2160p.mkv": 2}, nil)
	for _, h := range []string{h1, h2} {
		res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: h})
		require.NoError(t, err)
		require.Equal(t, 1, res.Created, res.Plan.Problem)
	}
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	logo := filepath.Join(show, "clearlogo.png")
	var ep1, ep2 models.MediaTransferHistory
	require.NoError(t, e.db.Where("info_hash = ?", h1).First(&ep1).Error)
	require.NoError(t, e.db.Where("info_hash = ?", h2).First(&ep2).Error)

	orig := fileIDOf
	t.Cleanup(func() { fileIDOf = orig })
	fileIDOf = func(p string) (string, error) {
		if filepath.Base(p) == "clearlogo.png" {
			return "", errors.New("permission denied")
		}
		return orig(p)
	}
	err := e.del(ep1.ID, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "clearlogo.png")
	var n int64
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", ep1.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n, "记录留着，下次再删")

	fileIDOf = orig
	require.NoError(t, e.del(ep1.ID, true))
	assert.True(t, exists(logo), "目录里还有第 2 集")
	require.NoError(t, e.del(ep2.ID, true))
	assert.False(t, exists(logo), "转给了第 2 集，删到最后一集时删掉")
	assert.False(t, exists(show))
}
