package organize

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

const (
	oppHash  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tlouHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	oppName  = "Oppenheimer.2023.2160p.BluRay.x265.10bit.HDR.DTS-HD.MA.5.1-FRDS"
	tlouName = "The.Last.of.Us.S01.2160p.WEB-DL.DV.H.265-HHWEB"
)

func (e *env) oppenheimer() downloader.Torrent {
	return e.addTorrent(oppHash, oppName, map[string]int{
		oppName + "/" + oppName + ".mkv":       5,
		oppName + "/" + oppName + ".chs.ass":   0,
		oppName + "/Sample/sample.mkv":         3,
		oppName + "/" + oppName + ".nfo":       0,
		oppName + "/Featurettes/Making Of.mkv": 4,
	}, nil)
}

func (e *env) lastOfUs() downloader.Torrent {
	return e.addTorrent(tlouHash, tlouName, map[string]int{
		tlouName + "/The.Last.of.Us.S01E01.2160p.WEB-DL.DV.H.265-HHWEB.mkv":     5,
		tlouName + "/The.Last.of.Us.S01E02.2160p.WEB-DL.DV.H.265-HHWEB.mkv":     5,
		tlouName + "/The.Last.of.Us.S01E01.2160p.WEB-DL.DV.H.265-HHWEB.chs.srt": 0,
		tlouName + "/Subs/S01E02.eng.srt":                                       0,
	}, func(t *downloader.Torrent) { t.Category = "tv" })
}

func TestOrganizeMovie(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1, NotifyChannels: []uint{7}})
	emby := newFakeEmby(t)
	key := "embykey"
	_, err := e.svc.SaveServer(e.ctx, 0, ServerInput{
		Name: "Emby", Kind: models.MediaServerEmby, URL: emby.URL, Token: &key, Enabled: true,
		LocalPrefix: filepath.Join(e.root, "media"), ServerPrefix: "/data/media",
	})
	require.NoError(t, err)
	e.oppenheimer()

	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Empty(t, p.Problem)
	assert.True(t, p.Mapped)
	assert.Equal(t, filepath.Join(e.dlDir), p.LocalPath)
	assert.Equal(t, recognize.SourceSearch, p.Source)
	assert.Equal(t, 872585, p.Match.ID)
	assert.Equal(t, "电影", p.Library.Name)
	assert.Equal(t, models.MediaModeHardlink, p.Mode)
	require.Len(t, p.Items, 1, "样片与花絮不整理")
	want := filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023) - 2160p BluRay HDR H.265.mkv")
	assert.Equal(t, want, p.Items[0].Target)
	assert.Equal(t, ItemPending, p.Items[0].Status)
	require.Len(t, p.Items[0].Subtitles, 1)
	assert.Equal(t, strings.TrimSuffix(want, ".mkv")+".zh-CN.ass", p.Items[0].Subtitles[0].Target)
	assert.Len(t, p.Skipped, 2)
	assert.False(t, exists(want), "预览不改动文件")

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Empty(t, res.Messages)
	src := filepath.Join(e.dlDir, oppName, oppName+".mkv")
	assert.True(t, sameFile(t, src, want), "硬链接")
	assert.True(t, exists(strings.TrimSuffix(want, ".mkv")+".zh-CN.ass"))
	dir := filepath.Dir(want)
	assert.Contains(t, readFile(t, strings.TrimSuffix(want, ".mkv")+".nfo"), "<uniqueid type=\"tmdb\" default=\"true\">872585</uniqueid>")
	assert.Equal(t, "JPEG/t/p/w780/opp.jpg", readFile(t, filepath.Join(dir, "poster.jpg")))
	assert.Equal(t, "JPEG/t/p/w1280/opp-bd.jpg", readFile(t, filepath.Join(dir, "fanart.jpg")))

	rows := e.history()
	require.Len(t, rows, 1)
	r := rows[0]
	assert.Equal(t, models.MediaTransferDone, r.Status)
	assert.Equal(t, oppHash, r.InfoHash)
	assert.Equal(t, src, r.SourcePath)
	assert.Equal(t, want, r.TargetPath)
	assert.Equal(t, 872585, r.TMDBID)
	assert.Equal(t, "奥本海默", r.Title)
	assert.Equal(t, models.MediaTriggerManual, r.Trigger)
	assert.NotEmpty(t, r.TargetFileID)
	kinds := map[string][]string{}
	for _, e := range decodeExtras(r.Extras) {
		kinds[e.Kind] = append(kinds[e.Kind], filepath.Base(e.Target))
		assert.NotEmpty(t, e.FileID, "每个文件都记下文件编号")
	}
	assert.Equal(t, []string{"奥本海默 (2023) - 2160p BluRay HDR H.265.zh-CN.ass"}, kinds[""], "字幕")
	assert.ElementsMatch(t, []string{"奥本海默 (2023) - 2160p BluRay HDR H.265.nfo", "poster.jpg", "fanart.jpg"}, kinds[extraMeta], "刮削写的文件")

	notices := e.gotNotices()
	require.Len(t, notices, 1)
	assert.Equal(t, "入库：奥本海默 (2023)", notices[0].Title)
	assert.Equal(t, []uint{7}, notices[0].ChannelIDs)
	assert.Contains(t, notices[0].Text, "画质：2160p BluRay HDR H.265")
	assert.Contains(t, notices[0].Text, "媒体库：电影")
	assert.Contains(t, notices[0].Text, "海报：https://image.tmdb.org/t/p/w500/opp.jpg")

	paths := emby.got()
	require.Len(t, paths, 1)
	assert.Contains(t, paths[0], `"Path":"/data/media/movies/奥本海默 (2023)"`, "按媒体服务器的路径映射通知")
	var srv models.MediaServer
	require.NoError(t, e.db.First(&srv).Error)
	assert.Empty(t, srv.LastError)
	assert.NotNil(t, srv.LastRefreshAt)

	// 再整理一次：库里已经有了，不重复通知
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Created)
	assert.Equal(t, 1, res.Done)
	assert.Len(t, e.gotNotices(), 1)
	assert.Len(t, emby.got(), 1)
	assert.Len(t, e.history(), 1)
	p, err = e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, ItemDone, p.Items[0].Status)
	assert.Equal(t, r.ID, p.Items[0].HistoryID)
}

func TestOrganizeTVSeasonPack(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.lastOfUs()

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	require.Empty(t, res.Plan.Problem)
	assert.Equal(t, 2, res.Created)
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	e1 := filepath.Join(show, "Season 1", "最后生还者 - S01E01 - 当你迷失在黑暗中.mkv")
	e2 := filepath.Join(show, "Season 1", "最后生还者 - S01E02 - 感染.mkv")
	assert.True(t, exists(e1))
	assert.True(t, exists(e2))
	assert.True(t, exists(strings.TrimSuffix(e1, ".mkv")+".zh-CN.srt"), "字幕按文件名前缀分给第 1 集")
	assert.True(t, exists(strings.TrimSuffix(e2, ".mkv")+".en.srt"), "字幕按季集分给第 2 集")
	assert.Contains(t, readFile(t, filepath.Join(show, "tvshow.nfo")), "<title>最后生还者</title>")
	assert.True(t, exists(filepath.Join(show, "season01-poster.jpg")))
	assert.True(t, exists(filepath.Join(show, "Season 1", "season.nfo")))
	assert.Equal(t, "JPEG/t/p/w300/e2.jpg", readFile(t, strings.TrimSuffix(e2, ".mkv")+"-thumb.jpg"))
	assert.Contains(t, readFile(t, strings.TrimSuffix(e2, ".mkv")+".nfo"), "<episode>2</episode>")

	rows := e.history()
	require.Len(t, rows, 2)
	for _, r := range rows {
		assert.Equal(t, models.MediaTransferDone, r.Status)
		assert.Equal(t, tmdb.KindTV, r.MediaType)
		assert.Equal(t, 1, r.Season)
	}
}

func TestOrganizeAnimeLibrary(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.library(LibraryInput{Name: "动漫", Kind: models.MediaKindTV, Anime: true, Path: e.anime})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.addTorrent("cccccccccccccccccccccccccccccccccccccccc", "[Group] Sousou no Frieren - 01 [1080p]", map[string]int{
		"[Group] Sousou no Frieren - 01 [1080p].mkv": 2,
	}, nil)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "cccccccccccccccccccccccccccccccccccccccc"})
	require.NoError(t, err)
	require.Empty(t, res.Plan.Problem)
	assert.Equal(t, "动漫", res.Plan.Library.Name, "TMDB 类型有动画的进只收动画的库")
	assert.True(t, exists(filepath.Join(e.anime, "葬送的芙莉莲 (2023)", "Season 1", "葬送的芙莉莲 - S01E01.mkv")),
		"单文件的种子集数取种子名；没有季详情时不写集标题；这个库没开刮削")
	assert.False(t, exists(filepath.Join(e.anime, "葬送的芙莉莲 (2023)", "tvshow.nfo")))
}

func TestOrganizeModes(t *testing.T) {
	for _, mode := range []string{models.MediaModeCopy, models.MediaModeSymlink} {
		t.Run(mode, func(t *testing.T) {
			e := newEnv(t)
			e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: mode, Template: "{{.Title}} ({{.Year}})"})
			e.settings(SettingsInput{MinVideoMB: 1})
			e.oppenheimer()
			res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
			require.NoError(t, err)
			require.Equal(t, 1, res.Created, res.Plan.Problem)
			dst := filepath.Join(e.movies, "奥本海默 (2023).mkv")
			src := filepath.Join(e.dlDir, oppName, oppName+".mkv")
			info, err := os.Lstat(dst)
			require.NoError(t, err)
			if mode == models.MediaModeSymlink {
				assert.NotZero(t, info.Mode()&os.ModeSymlink)
			} else {
				assert.False(t, sameFile(t, src, dst))
			}
			assert.Equal(t, mode, e.history()[0].Mode)
		})
	}
}

func TestOrganizeMoveRefusedWhileSeeding(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeMove})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "还在做种")
	assert.Equal(t, 1, res.Failed)
	assert.True(t, exists(filepath.Join(e.dlDir, oppName, oppName+".mkv")), "源文件不动")

	e.dl.mu.Lock()
	tr := e.dl.torrents[oppHash]
	tr.State = downloader.TorrentPaused
	e.dl.torrents[oppHash] = tr
	e.dl.mu.Unlock()
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created, "暂停后可以移动")
	assert.False(t, exists(filepath.Join(e.dlDir, oppName, oppName+".mkv")))
}

func TestOrganizeProblems(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "没有启用的电影媒体库")
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferFailed, rows[0].Status)
	assert.Nil(t, rows[0].NextRetryAt, "要用户先加媒体库，不自动重试")
	assert.Equal(t, 1, rows[0].Attempts)

	// TMDB 暂时不能访问：可以自动重试
	e.defaultLibraries()
	e.tmdb.setDown(true)
	e.addTorrent("dddddddddddddddddddddddddddddddddddddddd", "Other.Movie.2019.1080p.WEB-DL.x264-GRP", map[string]int{"Other.Movie.2019.1080p.WEB-DL.x264-GRP.mkv": 2}, nil)
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "dddddddddddddddddddddddddddddddddddddddd"})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "识别失败")
	var row models.MediaTransferHistory
	require.NoError(t, e.db.Where("info_hash = ?", "dddddddddddddddddddddddddddddddddddddddd").First(&row).Error)
	require.NotNil(t, row.NextRetryAt)
	assert.Equal(t, e.now.Add(retryDelays[0]), row.NextRetryAt.UTC())
	e.tmdb.setDown(false)

	// 没识别出来：不自动重试
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "dddddddddddddddddddddddddddddddddddddddd"})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "没有识别出来")
	var again models.MediaTransferHistory
	require.NoError(t, e.db.First(&again, row.ID).Error)
	assert.Nil(t, again.NextRetryAt)
	assert.Equal(t, 2, again.Attempts)

	// 没下载完：不写记录
	e.addTorrent("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "Half.2020.mkv", map[string]int{"Half.2020.mkv": 2}, func(t *downloader.Torrent) { t.Progress = 0.5 })
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"})
	require.NoError(t, err)
	assert.Equal(t, "种子还没下载完", res.Plan.Problem)
	var n int64
	e.db.Model(&models.MediaTransferHistory{}).Where("info_hash = ?", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee").Count(&n)
	assert.Zero(t, n)

	// 原盘：用内容路径记一行跳过
	e.addTorrent("ffffffffffffffffffffffffffffffffffffffff", "Disc.2021.BluRay", map[string]int{"Disc.2021.BluRay/BDMV/STREAM/00001.m2ts": 3}, nil)
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "ffffffffffffffffffffffffffffffffffffffff"})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "原盘")
	var disc models.MediaTransferHistory
	require.NoError(t, e.db.Where("info_hash = ?", "ffffffffffffffffffffffffffffffffffffffff").First(&disc).Error)
	assert.Equal(t, models.MediaTransferSkipped, disc.Status)
	assert.Equal(t, filepath.Join(e.dlDir, "Disc.2021.BluRay"), disc.SourcePath)

	// pt-tools 里找不到文件：提示加路径映射
	e.addTorrent("1111111111111111111111111111111111111111", "Lost.2022.mkv", map[string]int{"Lost.2022.mkv": 2}, func(t *downloader.Torrent) { t.SavePath = "/elsewhere" })
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "1111111111111111111111111111111111111111"})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "路径映射")
	assert.False(t, res.Plan.Mapped)

	// 下载器里没有、参数不对
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "9999"})
	require.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash, TMDBID: 1})
	require.ErrorIs(t, err, ErrInvalid, "指定条目要写类型")
	_, err = e.svc.Preview(e.ctx, Request{DownloaderID: 2, Hash: oppHash})
	require.Error(t, err)
}

func TestOrganizeManualOverrideAndLibrary(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	other := e.library(LibraryInput{Name: "另一个剧集库", Kind: models.MediaKindTV, Path: filepath.Join(e.root, "media", "tv2")})
	require.NoError(t, os.MkdirAll(other.Path, 0o755))
	e.settings(SettingsInput{MinVideoMB: 1})
	e.addTorrent("2222222222222222222222222222222222222222", "TLOU.E02.mkv", map[string]int{"TLOU.E02.mkv": 2}, nil)

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "2222222222222222222222222222222222222222", MediaType: tmdb.KindTV, TMDBID: 100088, LibraryID: other.ID})
	require.NoError(t, err)
	require.Empty(t, res.Plan.Problem)
	assert.Equal(t, "manual", res.Plan.Source)
	assert.Equal(t, 1, res.Created)
	assert.True(t, exists(filepath.Join(other.Path, "最后生还者 (2023)", "Season 1", "最后生还者 - S01E02 - 感染.mkv")))

	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "2222222222222222222222222222222222222222", MediaType: tmdb.KindMovie, TMDBID: 872585, LibraryID: other.ID})
	require.NoError(t, err)
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "2222222222222222222222222222222222222222", MediaType: tmdb.KindMovie, TMDBID: 872585, LibraryID: other.ID})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "类型与条目不同")
}

func TestOrganizeTargetExistsAndDuplicates(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Template: "{{.Title}}"})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	occupied := filepath.Join(e.movies, "奥本海默.mkv")
	require.NoError(t, os.WriteFile(occupied, []byte("someone else"), 0o644))
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped)
	assert.Equal(t, "someone else", readFile(t, occupied), "不覆盖")
	assert.Equal(t, models.MediaTransferSkipped, e.history()[0].Status)

	e.addTorrent("3333333333333333333333333333333333333333", "Oppenheimer.2023.Pack", map[string]int{
		"Oppenheimer.2023.Pack/Oppenheimer.2023.1080p.mkv": 2,
		"Oppenheimer.2023.Pack/Oppenheimer.2023.2160p.mkv": 2,
	}, nil)
	require.NoError(t, os.Remove(occupied))
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "3333333333333333333333333333333333333333"})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 1, res.Failed)
	assert.Contains(t, res.Plan.Items[1].Message, "目标相同")
}

func TestOrganizeTVNoEpisode(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.addTorrent(tlouHash, tlouName, map[string]int{
		tlouName + "/The.Last.of.Us.S01E01.mkv": 2,
		tlouName + "/bonus-clip.mkv":            2,
	}, nil)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 1, res.Failed)
	var failed models.MediaTransferHistory
	require.NoError(t, e.db.Where("status = ?", models.MediaTransferFailed).First(&failed).Error)
	assert.Contains(t, failed.Message, "集数")
	assert.Nil(t, failed.NextRetryAt)
}

func TestOrganizeWordsOffset(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	_, err := e.rec.SaveWord(e.ctx, models.MediaWordRule{Kind: models.MediaWordOffset, Pattern: "Last.of.Us", Offset: 1, Enabled: true})
	require.NoError(t, err)
	e.addTorrent(tlouHash, tlouName, map[string]int{tlouName + "/The.Last.of.Us.S01E01.mkv": 2}, nil)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created, res.Plan.Problem)
	assert.Equal(t, 2, res.Plan.Items[0].Episode, "文件名也套用识别词的集数偏移")
}

func TestOrganizeCrossDevice(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("只在 Linux 上用 /dev/shm 测")
	}
	lib, err := os.MkdirTemp("/dev/shm", "pt-tools-lib-")
	if err != nil {
		t.Skipf("/dev/shm 不可写: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(lib) })
	e := newEnv(t)
	a, _ := os.Stat(e.dlDir)
	b, _ := os.Stat(lib)
	if sameDevice(a, b) {
		t.Skip("临时目录与 /dev/shm 在同一个文件系统")
	}
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: lib})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
	row := e.history()[0]
	assert.Contains(t, row.Message, "同一个文件系统")
	assert.Nil(t, row.NextRetryAt, "配置问题，不自动重试")

	items, err := e.svc.CheckLibrary(e.ctx, LibraryCheckInput{Path: lib})
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.True(t, items[0].OK && items[1].OK)
	assert.False(t, items[2].OK, "硬链接试建失败")
	assert.Contains(t, items[2].Message, "同一个文件系统")
}

func TestOrganizeServerAndNotifyErrors(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1, NotifyChannels: []uint{1}})
	bad := "wrong"
	emby := newFakeEmby(t)
	_, err := e.svc.SaveServer(e.ctx, 0, ServerInput{Name: "Emby", Kind: models.MediaServerEmby, URL: emby.URL, Token: &bad, Enabled: true})
	require.NoError(t, err)
	e.notifyEr = assert.AnError
	e.oppenheimer()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created, "通知与媒体服务器出错不影响整理")
	joined := strings.Join(res.Messages, "\n")
	assert.Contains(t, joined, "通知媒体服务器「Emby」失败")
	assert.Contains(t, joined, "入库通知没有发出")
	var srv models.MediaServer
	require.NoError(t, e.db.First(&srv).Error)
	assert.Contains(t, srv.LastError, "API Key")
}

func TestChooseLibrary(t *testing.T) {
	libs := []models.MediaLibrary{
		{ID: 1, Kind: "movie", Enabled: true},
		{ID: 2, Kind: "tv", Anime: true, Enabled: true},
		{ID: 3, Kind: "tv", Enabled: false},
		{ID: 4, Kind: "tv", Enabled: true},
		{ID: 5, Kind: "tv", Enabled: true},
	}
	assert.Equal(t, uint(1), chooseLibrary(libs, "movie", false).ID)
	assert.Equal(t, uint(1), chooseLibrary(libs, "movie", true).ID, "没有动画电影库时进普通电影库")
	assert.Equal(t, uint(2), chooseLibrary(libs, "tv", true).ID)
	assert.Equal(t, uint(4), chooseLibrary(libs, "tv", false).ID, "只收动画的库不收普通剧集；跳过没启用的")
	assert.Nil(t, chooseLibrary(libs[1:2], "tv", false))
}

func TestCleanRelAndTruncate(t *testing.T) {
	for in, want := range map[string]string{"a/b.mkv": "a/b.mkv", `a\b.mkv`: "a/b.mkv", "a/./b.mkv": "a/b.mkv"} {
		got, ok := cleanRel(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got)
	}
	for _, bad := range []string{"", "/etc/passwd", "../x.mkv", "a/../../x", `C:\x.mkv`} {
		_, ok := cleanRel(bad)
		assert.False(t, ok, bad)
	}
	assert.Equal(t, "ab", truncate("ab", 5))
	assert.Equal(t, "奥", truncate("奥本", 4), "不切开汉字")
}

func TestPlanReusesPriorEntryAndMarksItems(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.addTorrent("7777777777777777777777777777777777777777", "Mystery.Thing.2020.1080p.mkv", map[string]int{"Mystery.Thing.2020.1080p.mkv": 2}, nil)
	req := Request{DownloaderID: 1, Hash: "7777777777777777777777777777777777777777"}

	p, err := e.svc.Preview(e.ctx, req)
	require.NoError(t, err)
	assert.Contains(t, p.Problem, "没有识别出来：TMDB 上没有可靠的匹配。手动整理时填 TMDB 编号")
	require.Len(t, p.Items, 1)
	assert.Equal(t, ItemFailed, p.Items[0].Status, "整理不了时不写「待整理」")

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: req.Hash, MediaType: tmdb.KindMovie, TMDBID: 872585})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)

	// 之后不指定条目再预览：沿用之前整理用的条目，库里已经有了
	p, err = e.svc.Preview(e.ctx, req)
	require.NoError(t, err)
	require.Empty(t, p.Problem)
	assert.Equal(t, sourceHistory, p.Source)
	assert.Equal(t, 872585, p.Match.ID)
	assert.Equal(t, ItemDone, p.Items[0].Status)

	// 整理不了（没有启用的电影库）时，已经在库里的文件标成「已在库里」
	lib, err := e.svc.Libraries(e.ctx)
	require.NoError(t, err)
	for _, l := range lib {
		_, saveErr := e.svc.SaveLibrary(e.ctx, l.ID, LibraryInput{Name: l.Name, Kind: l.Kind, Path: l.Path, Mode: l.Mode, Enabled: false})
		require.NoError(t, saveErr)
	}
	p, err = e.svc.Preview(e.ctx, req)
	require.NoError(t, err)
	assert.Contains(t, p.Problem, "没有启用的电影媒体库")
	assert.Equal(t, ItemDone, p.Items[0].Status)
}

func TestPlanWithoutTMDBKey(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	empty := ""
	_, err := e.rec.SaveSettings(e.ctx, recognize.SettingsInput{TMDBKey: &empty})
	require.NoError(t, err)
	e.oppenheimer()
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, "没有填写 TMDB API Key：先在「媒体识别」里填写", p.Problem)
}

// 字幕没整理成时写进记录；之后再整理这个种子时补上
func TestSubtitleRetriedOnNextRun(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	subTarget := filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023) - 2160p BluRay HDR H.265.zh-CN.ass")
	require.NoError(t, os.MkdirAll(filepath.Dir(subTarget), 0o755))
	require.NoError(t, os.WriteFile(subTarget, []byte("someone else"), 0o644))
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	row := e.history()[0]
	assert.Equal(t, models.MediaTransferDone, row.Status)
	assert.Contains(t, row.Message, "字幕", "字幕没整理成写进记录")

	require.NoError(t, os.Remove(subTarget))
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Done)
	assert.True(t, sameFile(t, filepath.Join(e.dlDir, oppName, oppName+".chs.ass"), subTarget), "再整理时补上字幕")
	row = e.history()[0]
	assert.Empty(t, row.Message)
	assert.Len(t, func() []extra {
		var subs []extra
		for _, x := range decodeExtras(row.Extras) {
			if x.Kind == "" {
				subs = append(subs, x)
			}
		}
		return subs
	}(), 1)
}

// 配置了媒体服务器的路径映射、整理到的目录不在里面：不把本地路径发过去，写明原因
func TestServerPathMappingMismatch(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	emby := newFakeEmby(t)
	key := "embykey"
	_, err := e.svc.SaveServer(e.ctx, 0, ServerInput{
		Name: "Emby", Kind: models.MediaServerEmby, URL: emby.URL, Token: &key, Enabled: true, LocalPrefix: "/somewhere/else", ServerPrefix: "/data",
	})
	require.NoError(t, err)
	e.oppenheimer()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Empty(t, emby.got(), "不发本地路径")
	assert.Contains(t, strings.Join(res.Messages, "\n"), "不在路径映射 /somewhere/else → /data 里")
	var srv models.MediaServer
	require.NoError(t, e.db.First(&srv).Error)
	assert.Contains(t, srv.LastError, "不在路径映射")
}

// 库里的文件被换掉了：不再说「已在库里」，记为跳过
func TestReplacedLibraryFileIsNotDone(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeCopy})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	row := e.history()[0]
	require.NoError(t, os.Remove(row.TargetPath))
	require.NoError(t, os.WriteFile(row.TargetPath, []byte("another version"), 0o644))
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, ItemExists, p.Items[0].Status)
	assert.Contains(t, p.Items[0].Message, "已经不是当初整理出的那个")
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	skipped := e.history()[0]
	assert.Equal(t, models.MediaTransferSkipped, skipped.Status)
	assert.Equal(t, "another version", readFile(t, row.TargetPath))
	// 之前整理出的字幕还记在记录里，删除记录时照样清理
	require.NotEmpty(t, row.Extras)
	assert.Equal(t, row.Extras, skipped.Extras)
	assert.Equal(t, row.TargetFileID, skipped.TargetFileID)

	// 换掉以后又整理不了（这里是没有启用的电影媒体库）：不算已在库里，记成失败，记着的文件也留着
	require.NoError(t, e.db.Model(&models.MediaLibrary{}).Where("1 = 1").Update("enabled", false).Error)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Done)
	assert.Equal(t, 1, res.Failed)
	failed := e.history()[0]
	assert.Equal(t, models.MediaTransferFailed, failed.Status)
	assert.Equal(t, row.Extras, failed.Extras)
	assert.Equal(t, row.TargetPath, failed.TargetPath)
	assert.Equal(t, row.LibraryID, failed.LibraryID)
	page, err := e.svc.History(e.ctx, HistoryQuery{})
	require.NoError(t, err)
	assert.True(t, page.Items[0].HasFiles, "跳过、失败的记录也可以连文件删")
	kept, err := e.svc.DeleteHistory(e.ctx, failed.ID, true)
	require.NoError(t, err)
	assert.Len(t, kept, 1, "换掉的视频留着并写明")
	assert.Equal(t, "another version", readFile(t, row.TargetPath))
	assert.False(t, exists(strings.TrimSuffix(row.TargetPath, ".mkv")+".zh-CN.ass"), "整理的字幕删掉")
}

// 复制与移动方式下，之前整理好的字幕按记录认出来，没整理成的下次补上；移动以后源文件不在了也不挡
func TestSubtitleRetryCopyAndMove(t *testing.T) {
	for _, mode := range []string{models.MediaModeCopy, models.MediaModeMove} {
		t.Run(mode, func(t *testing.T) {
			e := newEnv(t)
			e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: mode})
			e.settings(SettingsInput{MinVideoMB: 1})
			e.addTorrent(oppHash, oppName, map[string]int{
				oppName + "/" + oppName + ".mkv":     2,
				oppName + "/" + oppName + ".chs.ass": 0,
				oppName + "/" + oppName + ".eng.srt": 0,
			}, func(t *downloader.Torrent) { t.State = downloader.TorrentPaused })
			dir := filepath.Join(e.movies, "奥本海默 (2023)")
			blocked := filepath.Join(dir, "奥本海默 (2023) - 2160p BluRay HDR H.265.en.srt")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(blocked, []byte("someone else"), 0o644))
			res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
			require.NoError(t, err)
			require.Equal(t, 1, res.Created, res.Plan.Problem)
			assert.Contains(t, e.history()[0].Message, ".eng.srt")

			require.NoError(t, os.Remove(blocked))
			res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
			require.NoError(t, err)
			require.Empty(t, res.Plan.Problem, "移动以后视频的源文件不在了，不当作路径映射错")
			assert.True(t, exists(blocked), "补上了没整理成的字幕")
			row := e.history()[0]
			assert.Empty(t, row.Message, "两个字幕都在了")

			res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
			require.NoError(t, err)
			assert.Empty(t, res.Messages, "之前整理好的字幕按记录认出来，不再报「目标已存在」")
			assert.Empty(t, e.history()[0].Message)
		})
	}
}

// 移动时删不掉源文件（下载目录只读）：库里那份按复制记成已整理，写明源文件还在；再整理时认得这份、方式不变，可以连文件删除
func TestMoveSourceKept(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() == 0 {
		t.Skip("只在 Linux 上、不是 root 时测（root 删得掉只读目录里的文件）")
	}
	lib, err := os.MkdirTemp("/dev/shm", "pt-tools-lib-")
	if err != nil {
		t.Skipf("/dev/shm 不可写: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(lib) })
	e := newEnv(t)
	a, _ := os.Stat(e.dlDir)
	b, _ := os.Stat(lib)
	if sameDevice(a, b) {
		t.Skip("临时目录与 /dev/shm 在同一个文件系统")
	}
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: lib, Mode: models.MediaModeMove})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.addTorrent(oppHash, oppName, map[string]int{
		oppName + "/" + oppName + ".mkv":     2,
		oppName + "/" + oppName + ".chs.ass": 0,
	}, func(t *downloader.Torrent) { t.State = downloader.TorrentPaused })
	srcDir := filepath.Join(e.dlDir, oppName)
	require.NoError(t, os.Chmod(srcDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(srcDir, 0o755) })

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created, res.Plan.Problem)
	row := e.history()[0]
	assert.Equal(t, models.MediaTransferDone, row.Status)
	assert.Equal(t, models.MediaModeCopy, row.Mode, "库里这份是复制过去的")
	assert.Contains(t, row.Message, "源文件留在下载目录里")
	assert.True(t, exists(row.TargetPath))
	assert.True(t, exists(filepath.Join(srcDir, oppName+".mkv")), "源文件还在")
	assert.Len(t, decodeExtras(row.Extras), 1, "字幕也放好了、记下了")

	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Done)
	assert.Equal(t, models.MediaModeCopy, e.history()[0].Mode, "再整理时方式不变")

	kept, err := e.svc.DeleteHistory(e.ctx, row.ID, true)
	require.NoError(t, err)
	assert.Empty(t, kept)
	assert.False(t, exists(row.TargetPath), "按复制记下的可以连文件删")
	assert.True(t, exists(filepath.Join(srcDir, oppName+".mkv")))
}

// 整理记录写不进数据库：这次复制进库的视频与字幕撤回（源文件还在），记为失败；之后能正常整理
func TestRecordFailureUndoesPlacedFiles(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeCopy})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.addTorrent(oppHash, oppName, map[string]int{
		oppName + "/" + oppName + ".mkv":     2,
		oppName + "/" + oppName + ".chs.ass": 0,
	}, nil)
	boom := errors.New("disk I/O error")
	cb := e.db.Callback().Create()
	require.NoError(t, cb.Before("gorm:create").Register("test:fail_history", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "media_transfer_histories" {
			_ = tx.AddError(boom)
		}
	}))
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Created)
	assert.Equal(t, 1, res.Failed)
	assert.Contains(t, strings.Join(res.Messages, " "), "整理记录写不进去")
	target := res.Plan.Items[0].Target
	require.NotEmpty(t, target)
	assert.False(t, exists(target), "库里不留没人认领的文件")
	assert.False(t, exists(strings.TrimSuffix(target, ".mkv")+".zh-CN.ass"))
	assert.True(t, exists(filepath.Join(e.dlDir, oppName, oppName+".mkv")), "源文件不动")
	assert.Empty(t, e.history())
	require.NoError(t, cb.Remove("test:fail_history"))

	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.True(t, exists(target))
}

// 媒体库后来改成了移动：已经按复制整理好的条目再整理时，补上的字幕也按复制放（源文件不删），记录的方式不变
func TestSubtitlesFollowRecordedMode(t *testing.T) {
	e := newEnv(t)
	lib := e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeCopy})
	e.settings(SettingsInput{MinVideoMB: 1})
	paused := func(t *downloader.Torrent) { t.State = downloader.TorrentPaused }
	e.addTorrent(oppHash, oppName, map[string]int{oppName + "/" + oppName + ".mkv": 2}, paused)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)

	require.NoError(t, e.db.Model(&models.MediaLibrary{}).Where("id = ?", lib.ID).Update("mode", models.MediaModeMove).Error)
	e.addTorrent(oppHash, oppName, map[string]int{oppName + "/" + oppName + ".mkv": 2, oppName + "/" + oppName + ".chs.ass": 0}, paused)
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Empty(t, res.Plan.Problem)
	assert.Equal(t, 1, res.Done)
	row := e.history()[0]
	assert.Equal(t, models.MediaModeCopy, row.Mode)
	assert.Len(t, decodeExtras(row.Extras), 1, "补上了字幕")
	assert.True(t, exists(filepath.Join(e.dlDir, oppName, oppName+".chs.ass")), "按复制放，下载目录里的字幕还在")
}
