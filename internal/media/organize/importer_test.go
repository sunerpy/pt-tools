package organize

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// tree 在 root 下建这些文件：值是视频的大小（MB），以 / 结尾的是空目录；texts 是文本文件（NFO 一类）。
// 建好以后整棵树的时间设成一小时以前（导入不扫最近还在改动的）。
func (e *env) tree(root string, files map[string]int, texts map[string]string) {
	e.t.Helper()
	for rel, mb := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if strings.HasSuffix(rel, "/") {
			require.NoError(e.t, os.MkdirAll(p, 0o755))
			continue
		}
		require.NoError(e.t, os.MkdirAll(filepath.Dir(p), 0o755))
		f, err := os.Create(p)
		require.NoError(e.t, err)
		require.NoError(e.t, f.Truncate(int64(mb)<<20))
		require.NoError(e.t, f.Close())
	}
	for rel, text := range texts {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(e.t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(e.t, os.WriteFile(p, []byte(text), 0o644))
	}
	e.age(root)
}

// videos 是 tree 用的文件表：每个都是 3 MB 的视频，以 / 结尾的是空目录。
func videos(paths ...string) map[string]int {
	out := make(map[string]int, len(paths))
	for _, p := range paths {
		out[p] = 3
	}
	return out
}

// age 把 root 下所有文件与目录的时间设成一小时以前。
func (e *env) age(root string) {
	e.t.Helper()
	at := e.Now().Add(-time.Hour)
	require.NoError(e.t, filepath.Walk(root, func(p string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, at, at)
	}))
}

// scanImport 扫描媒体库 libID 里的 path，等它做完，回带每一项的结果。
func (e *env) scanImport(libID uint, path string) ImportScan {
	e.t.Helper()
	sc, err := e.svc.StartImportScan(e.ctx, libID, path)
	require.NoError(e.t, err)
	return e.waitScan(sc.ID)
}

func (e *env) waitScan(id string) ImportScan {
	e.t.Helper()
	e.svc.imp.mu.Lock()
	sc := e.svc.imp.scan
	e.svc.imp.mu.Unlock()
	require.NotNil(e.t, sc)
	require.Equal(e.t, id, sc.ID)
	select {
	case <-sc.done:
	case <-time.After(20 * time.Second):
		e.t.Fatal("扫描 20 秒还没做完")
	}
	got, err := e.svc.ImportScanStatus(id, true)
	require.NoError(e.t, err)
	return got
}

func importItems(sc ImportScan) map[string]ImportItem {
	out := map[string]ImportItem{}
	for _, it := range sc.Items {
		out[it.Name] = it
	}
	return out
}

func importFiles(it ImportItem) map[string]ImportFile {
	out := map[string]ImportFile{}
	for _, f := range it.Files {
		out[f.Rel] = f
	}
	return out
}

func (e *env) libraryNamed(name string) models.MediaLibrary {
	e.t.Helper()
	var lib models.MediaLibrary
	require.NoError(e.t, e.db.Where("name = ?", name).First(&lib).Error)
	return lib
}

const oppNFO = `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<movie>
  <title>随便写的标题</title>
  <actor><name>X</name><tmdbid>999</tmdbid></actor>
  <uniqueid type="imdb">tt0000001</uniqueid>
  <uniqueid type="tmdb" default="true">872585</uniqueid>
</movie>`

// 电影库：分类目录、自己分的组往下找；NFO、目录名里的编号与按名字识别依次认条目；原盘、没有视频、样片、隐藏目录的写明或不列；
// 已经有整理记录的视频标出来，都有记录的用记录里的条目，不问 TMDB
func TestImportScanMovies(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.tree(e.movies, videos(
		"Oppenheimer (2023)/Oppenheimer.2023.1080p.mkv",
		"随便/a.mkv",
		"另一个 [tmdbid=872585]/b.mkv",
		"IMDb/c.mkv",
		"华语电影/Oppenheimer.2023.mkv",
		"欧美/动作/Oppenheimer (2023)/x.mkv",
		"Nothing Here (1999)/n.mkv",
		"原盘/BDMV/STREAM/00000.m2ts",
		"空的/",
		"Sample Only/sample.mkv",
		"Oppenheimer.2023.2160p.mkv",
		"已整理 (2023)/o.mkv",
		"占位 [tmdbid=872585]/p.mkv",
		".hidden/h.mkv",
		"@eaDir/e.mkv",
		"合集/Oppenheimer.2023.mkv",
		"合集/Nothing.Here.1999.mkv",
		"两个版本 [tmdbid=872585]/Oppenheimer.2023.1080p.mkv",
		"两个版本 [tmdbid=872585]/Oppenheimer.2023.2160p.mkv",
		"两个版本 [tmdbid=872585]/Featurettes/making.mkv",
		"CD版 [tmdbid=872585]/CD1/a.mkv",
		"CD版 [tmdbid=872585]/CD2/b.mkv",
		"Oppenheimer (2023) [tmdbid=12345]/o.mkv",
	), map[string]string{
		"随便/movie.nfo":  oppNFO,
		"IMDb/c.nfo":    "<movie><imdbid>tt15398776</imdbid></movie>",
		"空的/readme.txt": "没有视频",
	})
	recorded := filepath.Join(e.movies, "已整理 (2023)", "o.mkv")
	require.NoError(t, e.db.Create(&models.MediaTransferHistory{
		SourcePath: "/downloads/o.mkv", TargetPath: recorded, Status: models.MediaTransferDone, MediaType: tmdb.KindMovie, TMDBID: 872585, Title: "奥本海默", Year: 2023,
	}).Error)
	// 跳过的记录里的目标是 pt-tools 想放、被别的文件占着的位置，不是 pt-tools 的文件
	occupied := filepath.Join(e.movies, "占位 [tmdbid=872585]", "p.mkv")
	require.NoError(t, e.db.Create(&models.MediaTransferHistory{SourcePath: "/downloads/p.mkv", TargetPath: occupied, Status: models.MediaTransferSkipped}).Error)

	sc := e.scanImport(e.libraryNamed("电影").ID, "")
	require.Equal(t, ImportDone, sc.Status, sc.Error)
	assert.Equal(t, e.movies, sc.Path, "空的路径是库目录")
	items := importItems(sc)
	assert.Equal(t, len(sc.Items), sc.Total)
	assert.Equal(t, sc.Total, sc.Scanned)
	assert.False(t, sc.Truncated)
	for _, name := range []string{".hidden", "@eaDir", "华语电影", "欧美", "欧美/动作", "合集"} {
		assert.NotContains(t, items, name, "隐藏的、系统目录、分类目录与自己分的组不是一项："+name)
	}
	match := func(name, source string) {
		t.Helper()
		it, ok := items[name]
		require.True(t, ok, "缺「%s」", name)
		require.NotNil(t, it.Match, "%s：%s", name, it.Problem)
		assert.Equal(t, 872585, it.Match.TMDBID, name)
		assert.Equal(t, "奥本海默", it.Match.Title, name)
		assert.Equal(t, source, it.Source, name)
		assert.Empty(t, it.Problem, name)
	}
	match("Oppenheimer (2023)", recognize.SourceSearch)
	match("随便", importSourceNFO)
	match("另一个 [tmdbid=872585]", importSourceDirname)
	match("IMDb", importSourceNFOIMDb)
	match("华语电影/Oppenheimer.2023.mkv", recognize.SourceSearch)
	match("欧美/动作/Oppenheimer (2023)", recognize.SourceSearch)
	match("Oppenheimer.2023.2160p.mkv", recognize.SourceSearch)
	match("合集/Oppenheimer.2023.mkv", recognize.SourceSearch)
	match("两个版本 [tmdbid=872585]", importSourceDirname)
	match("CD版 [tmdbid=872585]", importSourceDirname)
	match("占位 [tmdbid=872585]", importSourceDirname)
	match("Oppenheimer (2023) [tmdbid=12345]", recognize.SourceSearch)
	assert.Equal(t, []string{"名字里写的 TMDB 编号 12345 在 TMDB 上不是电影，按名字识别"}, items["Oppenheimer (2023) [tmdbid=12345]"].Notes)

	file := items["华语电影/Oppenheimer.2023.mkv"]
	assert.True(t, file.File)
	assert.Equal(t, []ImportFile{{Rel: "Oppenheimer.2023.mkv", Size: 3 << 20}}, file.Files)
	assert.Equal(t, 1, file.Ready)
	versions := items["两个版本 [tmdbid=872585]"]
	assert.Equal(t, 2, versions.FileCount, "一部的两个版本是一项；花絮不算")
	assert.Equal(t, int64(6<<20), versions.Size)
	assert.Equal(t, 2, items["CD版 [tmdbid=872585]"].FileCount, "分段放的是一项")
	assert.Equal(t, map[string]ImportFile{"CD1/a.mkv": {Rel: "CD1/a.mkv", Size: 3 << 20}, "CD2/b.mkv": {Rel: "CD2/b.mkv", Size: 3 << 20}}, importFiles(items["CD版 [tmdbid=872585]"]))
	assert.Equal(t, 1, items["占位 [tmdbid=872585]"].Ready, "跳过的记录的目标不算有记录")

	assert.Contains(t, items["Nothing Here (1999)"].Problem, "没有识别出来")
	assert.Nil(t, items["Nothing Here (1999)"].Match)
	assert.Contains(t, items["合集/Nothing.Here.1999.mkv"].Problem, "没有识别出来")
	assert.Equal(t, "原盘暂不导入", items["原盘"].Problem)
	assert.Equal(t, "没有可导入的视频文件", items["空的"].Problem)
	assert.Equal(t, "没有可导入的视频文件", items["Sample Only"].Problem, "样片不导入")

	done := items["已整理 (2023)"]
	assert.Equal(t, 1, done.Recorded)
	assert.Equal(t, 0, done.Ready)
	assert.True(t, done.Files[0].Recorded)
	assert.Equal(t, sourceHistory, done.Source, "视频都有记录：用记录里的条目")
	require.NotNil(t, done.Match)
	assert.Equal(t, 872585, done.Match.TMDBID)
	assert.Empty(t, done.Problem)

	// 只扫一个分类目录、一个电影目录
	sc = e.scanImport(e.libraryNamed("电影").ID, filepath.Join(e.movies, "华语电影"))
	require.Len(t, sc.Items, 1)
	assert.Equal(t, "华语电影/Oppenheimer.2023.mkv", sc.Items[0].Name)
	sc = e.scanImport(e.libraryNamed("电影").ID, filepath.Join(e.movies, "Oppenheimer (2023)"))
	require.Len(t, sc.Items, 1)
	assert.Equal(t, "Oppenheimer (2023)", sc.Items[0].Name, "选的是一部电影的目录：它就是一项")
	sc = e.scanImport(e.libraryNamed("电影").ID, filepath.Join(e.movies, "欧美"))
	require.Len(t, sc.Items, 1)
	assert.Equal(t, "欧美/动作/Oppenheimer (2023)", sc.Items[0].Name)
}

// 剧集库：季集从文件名认，文件名里没有季号时看季目录（Specials 是第 0 季），S00 写明的是第 0 季；没认出集数的、ISO 写明；
// 剧集目录名里没有季号也按剧集识别；tvshow.nfo 里的编号；直接放在库目录里的视频不导入
func TestImportScanTV(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.tree(e.tv, videos(
		"The Last of Us (2023)/Season 1/The.Last.of.Us.S01E01.mkv",
		"The Last of Us (2023)/Season 1/S01E02.mkv",
		"The Last of Us (2023)/Season 2/E01.mkv",
		"The Last of Us (2023)/第3季/E05.mkv",
		"The Last of Us (2023)/Specials/第1集.mkv",
		"The Last of Us (2023)/S00E03.mkv",
		"The Last of Us (2023)/no-episode.mkv",
		"The Last of Us (2023)/disc.S01E09.iso",
		"绝命毒师/Season 1/S01E01.mkv",
		"欧美剧/Breaking Bad [tmdbid=1396]/S01E02.mkv",
		"Sousou no Frieren/[SubsPlease] Sousou no Frieren - 01 (1080p).mkv",
		"stray.S01E01.mkv",
	), map[string]string{
		"绝命毒师/tvshow.nfo":          `<tvshow><uniqueid type="tmdb">1396</uniqueid></tvshow>`,
		"绝命毒师/Season 1/S01E01.nfo": `<episodedetails><uniqueid type="tmdb">62085</uniqueid></episodedetails>`,
	})
	sc := e.scanImport(e.libraryNamed("剧集").ID, "")
	require.Equal(t, ImportDone, sc.Status, sc.Error)
	items := importItems(sc)
	tlou := items["The Last of Us (2023)"]
	require.NotNil(t, tlou.Match, tlou.Problem)
	assert.Equal(t, 100088, tlou.Match.TMDBID, "剧集目录名里没有季号也按剧集识别")
	assert.Equal(t, recognize.SourceSearch, tlou.Source)
	files := importFiles(tlou)
	ep := func(rel string, season, episode int, problem string) {
		t.Helper()
		f, ok := files[rel]
		require.True(t, ok, "缺 %s", rel)
		assert.Equal(t, season, f.Season, rel)
		assert.Equal(t, episode, f.Episode, rel)
		if problem == "" {
			assert.Empty(t, f.Problem, rel)
		} else {
			assert.Contains(t, f.Problem, problem, rel)
		}
	}
	ep("Season 1/The.Last.of.Us.S01E01.mkv", 1, 1, "")
	ep("Season 1/S01E02.mkv", 1, 2, "")
	ep("Season 2/E01.mkv", 2, 1, "")
	ep("第3季/E05.mkv", 3, 5, "")
	ep("Specials/第1集.mkv", 0, 1, "")
	ep("S00E03.mkv", 0, 3, "")
	ep("no-episode.mkv", 1, 0, "没认出集数")
	ep("disc.S01E09.iso", 0, 0, "ISO")
	assert.Equal(t, 8, tlou.FileCount)
	assert.Equal(t, 6, tlou.Ready)

	bb := items["绝命毒师"]
	require.NotNil(t, bb.Match, bb.Problem)
	assert.Equal(t, 1396, bb.Match.TMDBID)
	assert.Equal(t, importSourceNFO, bb.Source, "tvshow.nfo 里的编号；一集的 NFO 里是那一集的编号，不看")
	cat := items["欧美剧/Breaking Bad [tmdbid=1396]"]
	require.NotNil(t, cat.Match, cat.Problem)
	assert.Equal(t, importSourceDirname, cat.Source)
	fr := items["Sousou no Frieren"]
	require.NotNil(t, fr.Match, fr.Problem)
	assert.Equal(t, 209867, fr.Match.TMDBID)
	assert.Equal(t, 1, fr.Files[0].Episode)
	assert.Contains(t, items["stray.S01E01.mkv"].Problem, "剧集的视频要放在剧集目录里")
	assert.NotContains(t, items, "欧美剧")
}

// 识别出的类型和媒体库不一样、TMDB 出错时写明；NFO 里只有网址
func TestImportScanProblems(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.tree(e.movies, map[string]int{"The Last of Us S01/a.mkv": 3, "网址/a.mkv": 3}, map[string]string{
		"网址/movie.nfo": "https://www.themoviedb.org/movie/872585-oppenheimer\n",
	})
	sc := e.scanImport(e.libraryNamed("电影").ID, "")
	items := importItems(sc)
	assert.Equal(t, "认成了剧集「最后生还者」，这是电影库：填电影的 TMDB 编号", items["The Last of Us S01"].Problem)
	require.NotNil(t, items["网址"].Match)
	assert.Equal(t, importSourceNFO, items["网址"].Source, "只写了网址的 NFO")

	e.tmdb.setDown(true)
	cachePurge(t, e)
	sc = e.scanImport(e.libraryNamed("电影").ID, "")
	items = importItems(sc)
	assert.Contains(t, items["网址"].Problem, "取 TMDB 详情失败")
	assert.Contains(t, items["The Last of Us S01"].Problem, "识别失败")
}

// 最近 2 分钟里改过的视频不导入（可能还在复制）；刚写进来的 NFO 与图片不算
func TestImportScanSettle(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.tree(e.movies, videos("Old (2023)/Oppenheimer.2023.mkv", "New (2023)/Oppenheimer.2023.mkv"), nil)
	now := e.Now()
	require.NoError(t, os.Chtimes(filepath.Join(e.movies, "New (2023)", "Oppenheimer.2023.mkv"), now, now))
	for _, side := range []string{"movie.nfo", "poster.jpg"} {
		p := filepath.Join(e.movies, "Old (2023)", side)
		require.NoError(t, os.WriteFile(p, []byte(oppNFO), 0o644))
		require.NoError(t, os.Chtimes(p, now, now))
	}
	require.NoError(t, os.Chtimes(filepath.Join(e.movies, "Old (2023)"), now, now))
	items := importItems(e.scanImport(e.libraryNamed("电影").ID, ""))
	assert.Equal(t, "最近 2 分钟里还有改动（可能还在复制），等写完再导入", items["New (2023)"].Problem)
	assert.Empty(t, items["Old (2023)"].Problem)
	assert.Equal(t, 1, items["Old (2023)"].Ready)
}

// 剧集库里的文件夹名多半没写季号：按剧集认，同名的电影更热门也不认成电影
func TestImportScanTVFolderIsShow(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.tree(e.tv, videos("Fallout (2024)/Season 1/Fallout.S01E01.mkv"), nil)
	it := importItems(e.scanImport(e.libraryNamed("剧集").ID, ""))["Fallout (2024)"]
	require.NotNil(t, it.Match, it.Problem)
	assert.Equal(t, 106379, it.Match.TMDBID)
}

// 一项里一部分视频已经有整理记录（pt-tools 放进库的）：导入时跳过它们，只记别的
func TestImportSkipsRecordedVideos(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	dir := filepath.Join(e.movies, "Oppenheimer (2023)")
	e.tree(e.movies, videos("Oppenheimer (2023)/Oppenheimer.2023.1080p.mkv", "Oppenheimer (2023)/Oppenheimer.2023.2160p.mkv"), nil)
	placed := filepath.Join(dir, "Oppenheimer.2023.1080p.mkv")
	id, err := fileIDOf(placed)
	require.NoError(t, err)
	require.NoError(t, e.db.Create(&models.MediaTransferHistory{
		DownloaderID: 1, InfoHash: oppHash, SourcePath: "/downloads/opp.mkv", TargetPath: placed, TargetFileID: id, Mode: models.MediaModeHardlink,
		MediaType: models.MediaKindMovie, TMDBID: 872585, Status: models.MediaTransferDone, LibraryID: e.libraryNamed("电影").ID,
	}).Error)
	sc := e.scanImport(e.libraryNamed("电影").ID, "")
	it := importItems(sc)["Oppenheimer (2023)"]
	require.Equal(t, 1, it.Ready, it.Problem)
	require.Equal(t, 1, it.Recorded)
	run, err := e.svc.QueueImport(e.ctx, sc.ID, []string{dir}, false)
	require.NoError(t, err)
	assert.Equal(t, 1, run.Created)
	assert.Equal(t, 1, run.Skipped)
	var n int64
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("target_path = ?", placed).Count(&n).Error)
	assert.EqualValues(t, 1, n, "pt-tools 放进库的视频不另记一条")
}

func TestImportScanRequests(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	lib := e.libraryNamed("电影")
	e.tree(e.movies, map[string]int{"a.mkv": 1}, nil)
	for _, p := range []string{"relative/path", e.tv, filepath.Join(e.movies, "a.mkv"), filepath.Join(e.movies, "missing")} {
		_, err := e.svc.StartImportScan(e.ctx, lib.ID, p)
		assert.ErrorIs(t, err, ErrInvalid, p)
	}
	_, err := e.svc.StartImportScan(e.ctx, 99, "")
	assert.ErrorIs(t, err, ErrNotFound)
	if serr := os.Symlink(e.tv, filepath.Join(e.movies, "link")); serr == nil {
		_, err = e.svc.StartImportScan(e.ctx, lib.ID, filepath.Join(e.movies, "link"))
		assert.ErrorIs(t, err, ErrInvalid, "经软链接指到库外的不行")
	}
	_, err = e.svc.ImportScanStatus("0123456789abcdef", true)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorIs(t, e.svc.CancelImportScan("0123456789abcdef"), ErrNotFound)

	e.svc.cfg.Recognizer = noTMDB{e.svc.cfg.Recognizer}
	_, err = e.svc.StartImportScan(e.ctx, lib.ID, "")
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "TMDB API Key")
	e.svc.cfg.Recognizer = brokenTMDB{noTMDB{e.rec}}
	_, err = e.svc.StartImportScan(e.ctx, lib.ID, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TMDB 不能用")
}

type brokenTMDB struct{ Recognizer }

func (brokenTMDB) TMDB(context.Context) (*tmdb.Client, error) {
	return nil, errors.New("代理连不上")
}

// 扫描的生命周期：再开一次时取消上一次；取消；扫描完了的丢掉；30 分钟不看就清掉
func TestImportScanLifecycle(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	lib := e.libraryNamed("电影")
	e.tree(e.movies, map[string]int{"Oppenheimer (2023)/a.mkv": 60}, nil)
	release := make(chan struct{})
	importScanStarted = func(ctx context.Context) {
		select {
		case <-release:
		case <-ctx.Done():
		}
	}
	t.Cleanup(func() { importScanStarted = func(context.Context) {} })

	first, err := e.svc.StartImportScan(e.ctx, lib.ID, "")
	require.NoError(t, err)
	assert.Equal(t, ImportRunning, first.Status)
	firstScan := e.svc.imp.scan
	got, err := e.svc.ImportScanStatus(first.ID, true)
	require.NoError(t, err)
	assert.Nil(t, got.Items, "还在扫描：没有每一项")
	_, err = e.svc.QueueImport(e.ctx, first.ID, []string{filepath.Join(e.movies, "Oppenheimer (2023)")}, false)
	assert.ErrorIs(t, err, ErrInvalid, "扫描还没完成")
	_, err = e.svc.SetImportItem(e.ctx, first.ID, filepath.Join(e.movies, "Oppenheimer (2023)"), 872585)
	assert.ErrorIs(t, err, ErrInvalid, "扫描还没完成")

	second, err := e.svc.StartImportScan(e.ctx, lib.ID, "")
	require.NoError(t, err)
	<-firstScan.done
	assert.Equal(t, ImportCanceled, firstScan.Status, "再开一次时取消上一次")
	_, err = e.svc.ImportScanStatus(first.ID, false)
	assert.ErrorIs(t, err, ErrNotFound, "上一次的结果不在了")

	require.NoError(t, e.svc.CancelImportScan(second.ID))
	got = e.waitScan(second.ID)
	assert.Equal(t, ImportCanceled, got.Status)
	assert.NotNil(t, got.FinishedAt)
	require.NoError(t, e.svc.CancelImportScan(second.ID), "扫描完了的丢掉结果")
	_, err = e.svc.ImportScanStatus(second.ID, false)
	assert.ErrorIs(t, err, ErrNotFound)

	close(release)
	third := e.scanImport(lib.ID, "")
	assert.Equal(t, ImportDone, third.Status)
	brief, err := e.svc.ImportScanStatus(third.ID, false)
	require.NoError(t, err)
	assert.Nil(t, brief.Items, "brief 时不带每一项")
	e.advance(20 * time.Minute)
	_, err = e.svc.ImportScanStatus(third.ID, false)
	require.NoError(t, err, "20 分钟以后还在（从最后一次看它算起）")
	e.advance(31 * time.Minute)
	_, err = e.svc.ImportScanStatus(third.ID, false)
	assert.ErrorIs(t, err, ErrNotFound, "30 分钟不看就清掉")

	// 停止服务时取消在跑的扫描
	importScanStarted = func(ctx context.Context) { <-ctx.Done() }
	fourth, err := e.svc.StartImportScan(e.ctx, lib.ID, "")
	require.NoError(t, err)
	e.svc.Stop()
	got, err = e.svc.ImportScanStatus(fourth.ID, false)
	require.NoError(t, err)
	assert.Equal(t, ImportCanceled, got.Status)
}

// 改一项的条目：按编号取详情（类型跟库），导入时用它；本身导入不了的、编号不对的不改
func TestImportSetItem(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	lib := e.libraryNamed("电影")
	e.tree(e.movies, map[string]int{"Nothing Here (1999)/n.mkv": 3, "原盘/BDMV/STREAM/0.m2ts": 3}, nil)
	sc := e.scanImport(lib.ID, "")
	nothing := filepath.Join(e.movies, "Nothing Here (1999)")
	_, err := e.svc.QueueImport(e.ctx, sc.ID, []string{nothing}, false)
	require.ErrorIs(t, err, ErrInvalid, "没认出条目的不能导入")
	assert.Contains(t, err.Error(), "没有识别出来")

	for _, c := range []struct {
		path string
		id   int
		want string
	}{
		{nothing, 0, "要填 TMDB 编号"},
		{nothing, 12345, "TMDB 上没有编号是 12345 的电影"},
		{filepath.Join(e.movies, "原盘"), 872585, "改条目也没用"},
		{filepath.Join(e.movies, "不在"), 872585, "不是这次扫描出的项"},
	} {
		_, serr := e.svc.SetImportItem(e.ctx, sc.ID, c.path, c.id)
		require.ErrorIs(t, serr, ErrInvalid, c.path)
		assert.Contains(t, serr.Error(), c.want)
	}
	it, err := e.svc.SetImportItem(e.ctx, sc.ID, nothing, 872585)
	require.NoError(t, err)
	assert.Equal(t, sourceManual, it.Source)
	assert.True(t, it.Edited)
	assert.Empty(t, it.Problem)
	assert.Equal(t, "Oppenheimer", it.Match.OriginalTitle)
	got, err := e.svc.ImportScanStatus(sc.ID, true)
	require.NoError(t, err)
	assert.Equal(t, 872585, importItems(got)["Nothing Here (1999)"].Match.TMDBID, "改好的记在扫描里")

	run, err := e.svc.QueueImport(e.ctx, sc.ID, []string{nothing, nothing}, false)
	require.NoError(t, err)
	assert.Equal(t, ImportDone, run.Status)
	assert.Equal(t, 1, run.Items, "同一项只导入一次")
	assert.Equal(t, 1, run.Created)
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, sourceManual, rows[0].MatchSource)

	_, err = e.svc.SetImportItem(e.ctx, "0123456789abcdef", nothing, 872585)
	assert.ErrorIs(t, err, ErrNotFound)
	e.svc.cfg.Recognizer = noTMDB{e.rec}
	_, err = e.svc.SetImportItem(e.ctx, sc.ID, nothing, 872585)
	assert.ErrorIs(t, err, ErrInvalid)
	e.svc.cfg.Recognizer = brokenTMDB{e.rec}
	_, err = e.svc.SetImportItem(e.ctx, sc.ID, nothing, 872585)
	assert.ErrorContains(t, err, "TMDB 不能用")
	e.svc.cfg.Recognizer = e.rec
	e.tmdb.setDown(true)
	cachePurge(t, e)
	_, err = e.svc.SetImportItem(e.ctx, sc.ID, nothing, 872585)
	assert.ErrorContains(t, err, "取 TMDB 详情失败")
}

// 导入：每个能导入的视频记一条整理记录（方式 existing、触发方式 import，源文件与库里的文件是同一个）；再扫描时它们有记录了
func TestImportRecords(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	lib, tvLib := e.libraryNamed("电影"), e.libraryNamed("剧集")
	e.tree(e.movies, map[string]int{"Oppenheimer (2023)/Oppenheimer.2023.1080p.mkv": 3, "Nothing Here (1999)/n.mkv": 3}, nil)
	e.tree(e.tv, map[string]int{"The Last of Us (2023)/Season 1/S01E01.mkv": 3, "The Last of Us (2023)/Season 1/S01E02.mkv": 3, "The Last of Us (2023)/x.mkv": 3}, nil)
	sc := e.scanImport(lib.ID, "")
	opp := filepath.Join(e.movies, "Oppenheimer (2023)")
	_, err := e.svc.QueueImport(e.ctx, sc.ID, nil, false)
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.QueueImport(e.ctx, sc.ID, []string{filepath.Join(e.movies, "不在")}, false)
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.QueueImport(e.ctx, "0123456789abcdef", []string{opp}, false)
	assert.ErrorIs(t, err, ErrNotFound)

	run, err := e.svc.QueueImport(e.ctx, sc.ID, []string{opp}, false)
	require.NoError(t, err)
	assert.Equal(t, ImportDone, run.Status)
	assert.Equal(t, 1, run.Created)
	assert.NotNil(t, run.FinishedAt)
	rows := e.history()
	require.Len(t, rows, 1)
	r := rows[0]
	video := filepath.Join(opp, "Oppenheimer.2023.1080p.mkv")
	id, err := transfer.FileID(video)
	require.NoError(t, err)
	assert.Equal(t, video, r.SourcePath)
	assert.Equal(t, video, r.TargetPath)
	assert.Equal(t, models.MediaModeExisting, r.Mode)
	assert.Equal(t, models.MediaTriggerImport, r.Trigger)
	assert.Equal(t, models.MediaTransferDone, r.Status)
	assert.Equal(t, importSource, r.DownloaderName)
	assert.Zero(t, r.DownloaderID)
	assert.True(t, strings.HasPrefix(r.InfoHash, importHashPrefix))
	assert.Equal(t, importHash(opp), r.InfoHash)
	assert.Equal(t, "Oppenheimer.2023.1080p.mkv", r.TorrentName)
	assert.Equal(t, id, r.TargetFileID)
	assert.Equal(t, id, r.SourceFileID)
	assert.Equal(t, e.movies, r.RootPath, "起点是这一项的上一级：电影有自己的目录")
	assert.Equal(t, lib.ID, r.LibraryID)
	assert.Equal(t, 872585, r.TMDBID)
	assert.Equal(t, "奥本海默", r.Title)
	assert.Equal(t, 2023, r.Year)
	assert.Equal(t, recognize.SourceSearch, r.MatchSource)
	assert.True(t, ours(&r), "库里的文件还是它")
	assert.Empty(t, r.ScrapeStatus, "没要刮削")

	// 扫描里它还是能导入的样子：再导入一次时再读一次，视频已经有记录了
	run, err = e.svc.QueueImport(e.ctx, sc.ID, []string{opp}, false)
	require.NoError(t, err)
	assert.Equal(t, 0, run.Created)
	assert.Equal(t, 1, run.Skipped)
	assert.Len(t, e.history(), 1)
	sc = e.scanImport(lib.ID, "")
	it := importItems(sc)["Oppenheimer (2023)"]
	assert.Equal(t, 1, it.Recorded)
	assert.Equal(t, sourceHistory, it.Source)
	_, err = e.svc.QueueImport(e.ctx, sc.ID, []string{opp}, false)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "没有能导入的视频")

	// 剧集：每一集一条，季集照扫描时认出的；没认出集数的写进结果
	sc = e.scanImport(tvLib.ID, "")
	show := filepath.Join(e.tv, "The Last of Us (2023)")
	run, err = e.svc.QueueImport(e.ctx, sc.ID, []string{show}, false)
	require.NoError(t, err)
	assert.Equal(t, 2, run.Created)
	assert.Equal(t, 1, run.Failed)
	assert.Equal(t, []string{"The Last of Us (2023)/x.mkv：文件名里没认出集数：可以加识别词，或改文件名后重新扫描"}, run.Messages)
	var eps []models.MediaTransferHistory
	require.NoError(t, e.db.Where("library_id = ?", tvLib.ID).Order("episode").Find(&eps).Error)
	require.Len(t, eps, 2)
	assert.Equal(t, []int{1, 1}, []int{eps[0].Season, eps[0].Episode})
	assert.Equal(t, []int{1, 2}, []int{eps[1].Season, eps[1].Episode})
	assert.Equal(t, 100088, eps[0].TMDBID)
	assert.Equal(t, e.tv, eps[0].RootPath, "剧集目录是这一项")
	assert.Equal(t, show, showDir(eps[0].RootPath, eps[0].TargetPath))
}

// 导入时再读一次：扫描以后视频变了、有了记录、TMDB 出错时写进结果
func TestImportRereadsItems(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	lib := e.libraryNamed("电影")
	e.tree(e.movies, map[string]int{"A [tmdbid=872585]/a.mkv": 3, "B [tmdbid=872585]/b.mkv": 3, "C [tmdbid=872585]/c.mkv": 3, "D [tmdbid=872585]/d.mkv": 3}, nil)
	sc := e.scanImport(lib.ID, "")
	paths := []string{}
	for _, n := range []string{"A", "B", "C", "D"} {
		paths = append(paths, filepath.Join(e.movies, n+" [tmdbid=872585]"))
	}
	require.NoError(t, os.RemoveAll(paths[0]))
	require.NoError(t, e.db.Create(&models.MediaTransferHistory{SourcePath: filepath.Join(paths[1], "b.mkv"), Status: models.MediaTransferFailed}).Error)
	fileIDOf = func(p string) (string, error) {
		if strings.HasSuffix(p, "c.mkv") {
			return "", errors.New("读不了")
		}
		return transfer.FileID(p)
	}
	t.Cleanup(func() { fileIDOf = transfer.FileID })
	run, err := e.svc.QueueImport(e.ctx, sc.ID, paths, false)
	require.NoError(t, err)
	assert.Equal(t, 4, run.Done)
	assert.Equal(t, 1, run.Created, "只有 D")
	assert.Equal(t, 1, run.Skipped, "B 有了记录")
	assert.Equal(t, 2, run.Failed)
	require.Len(t, run.Messages, 2)
	assert.Contains(t, run.Messages[0], "A [tmdbid=872585]：读不了")
	assert.Contains(t, run.Messages[1], "c.mkv：读不到文件编号")

	e.tmdb.setDown(true)
	cachePurge(t, e)
	require.NoError(t, e.db.Where("1 = 1").Delete(&models.MediaTransferHistory{}).Error)
	run, err = e.svc.QueueImport(e.ctx, sc.ID, paths[3:], false)
	require.NoError(t, err, "TMDB 不能用时这一项不导入，写进结果")
	assert.Equal(t, 1, run.Failed)
	assert.Contains(t, run.Messages[0], "取 TMDB 详情失败")
}

// 导入的已有文件：删除记录时不跟着删文件（要另外确认），确认以后按文件编号删；洗版不删；同名替换不换；重试没有意义
func TestImportedFilesAreProtected(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	lib := e.libraryNamed("电影")
	e.tree(e.movies, map[string]int{"A [tmdbid=872585]/a.mkv": 3, "B [tmdbid=872585]/b.mkv": 3, "C [tmdbid=872585]/c.mkv": 3}, nil)
	sc := e.scanImport(lib.ID, "")
	var paths []string
	for _, it := range sc.Items {
		paths = append(paths, it.Path)
	}
	_, err := e.svc.QueueImport(e.ctx, sc.ID, paths, false)
	require.NoError(t, err)
	rows := e.history()
	require.Len(t, rows, 3)
	a, b, c := rows[0], rows[1], rows[2]

	_, err = e.svc.Retry(e.ctx, a.ID)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), importRetryNote)
	res, err := e.svc.Batch(e.ctx, []uint{a.ID}, BatchRetry)
	require.NoError(t, err)
	assert.Equal(t, importRetryNote, res.Items[0].Message)

	_, err = e.svc.DeleteHistory(e.ctx, a.ID, true)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "导入时就在库里的文件")
	assert.FileExists(t, a.SourcePath)
	res, err = e.svc.Batch(e.ctx, []uint{a.ID}, BatchDeleteFiles)
	require.NoError(t, err)
	assert.Contains(t, res.Items[0].Message, "导入时就在库里的文件")
	assert.FileExists(t, a.SourcePath)
	assert.Len(t, e.history(), 3, "没确认时记录也不删")

	// 文件改过（编号变了）：确认了也不删
	require.NoError(t, os.WriteFile(b.SourcePath, []byte("改过"), 0o644))
	kept, err := e.svc.DeleteHistoryWith(e.ctx, b.ID, DeleteOptions{Files: true, AllowExisting: true})
	require.NoError(t, err)
	assert.Len(t, kept, 1)
	assert.FileExists(t, b.SourcePath)
	kept, err = e.svc.DeleteHistoryWith(e.ctx, a.ID, DeleteOptions{Files: true, AllowExisting: true})
	require.NoError(t, err)
	assert.Empty(t, kept)
	assert.NoFileExists(t, a.SourcePath, "确认以后删掉")
	res, err = e.svc.BatchWith(e.ctx, []uint{c.ID}, BatchDeleteFiles, BatchOptions{AllowExisting: true})
	require.NoError(t, err)
	assert.Empty(t, res.Items[0].Message)
	assert.NoFileExists(t, c.SourcePath)
	assert.Empty(t, e.history())
}

// 洗版的 Retire：导入的已有文件不删，记录照旧是已整理，写明（只写一次）
func TestRetireKeepsImportedFiles(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.tree(e.movies, map[string]int{"A [tmdbid=872585]/a.mkv": 3}, nil)
	sc := e.scanImport(e.libraryNamed("电影").ID, "")
	_, err := e.svc.QueueImport(e.ctx, sc.ID, []string{sc.Items[0].Path}, false)
	require.NoError(t, err)
	row := e.history()[0]
	_, err = e.svc.Retire(e.ctx, row.ID, "洗版：换成了 X")
	require.ErrorIs(t, err, ErrExistingKept)
	after := e.history()[0]
	assert.Equal(t, models.MediaTransferDone, after.Status)
	assert.Equal(t, "洗版：换成了 X；"+existingRetireNote, after.Message)
	assert.FileExists(t, row.SourcePath)
	assert.Equal(t, row.UpdatedAt.Unix(), after.UpdatedAt.Unix(), "不把记录挪到整理历史的最前面")
	_, err = e.svc.Retire(e.ctx, row.ID, "洗版：换成了 X")
	require.ErrorIs(t, err, ErrExistingKept)
}

// 种子整理到导入的已有文件的位置：开着「新文件更大时替换」也不换；种子的文件就是导入的文件时记录照旧
func TestOrganizeDoesNotReplaceImportedFiles(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, OverwriteMode: OverwriteLarger})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.tree(e.movies, map[string]int{"奥本海默 (2023)/奥本海默 (2023) - 2160p.mkv": 2}, nil)
	sc := e.scanImport(e.libraryNamed("电影").ID, "")
	run, err := e.svc.QueueImport(e.ctx, sc.ID, []string{sc.Items[0].Path}, false)
	require.NoError(t, err)
	require.Equal(t, 1, run.Created)
	imported := e.history()[0]

	e.addTorrent("hash1", "Oppenheimer.2023.2160p.mkv", map[string]int{"Oppenheimer.2023.2160p.mkv": 8}, nil)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "hash1"})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped)
	require.Len(t, res.Plan.Items, 1)
	assert.Equal(t, ItemExists, res.Plan.Items[0].Status)
	assert.Contains(t, res.Plan.Items[0].Message, "导入时就在库里的文件")
	info, err := os.Stat(imported.TargetPath)
	require.NoError(t, err)
	assert.Equal(t, int64(2<<20), info.Size(), "原来的文件没动")

	// 种子下载在库目录里，它的文件就是导入的那个：记录照旧（字幕也不按「已有文件」的方式放）
	inLib := filepath.Join(e.dlDir, "lib")
	e.library(LibraryInput{Name: "下载目录里的库", Kind: models.MediaKindMovie, Path: inLib})
	e.tree(inLib, map[string]int{"Oppenheimer (2023)/Oppenheimer.2023.mkv": 3}, map[string]string{"Oppenheimer (2023)/Oppenheimer.2023.chs.srt": "1"})
	sc = e.scanImport(e.libraryNamed("下载目录里的库").ID, "")
	_, err = e.svc.QueueImport(e.ctx, sc.ID, []string{sc.Items[0].Path}, false)
	require.NoError(t, err)
	e.addTorrent("hash2", "Oppenheimer (2023)", nil, func(t *downloader.Torrent) {
		t.SavePath, t.ContentPath = "/downloads/lib", "/downloads/lib/Oppenheimer (2023)"
	})
	e.dl.mu.Lock()
	e.dl.files["hash2"] = []downloader.TorrentFile{
		{Name: "Oppenheimer (2023)/Oppenheimer.2023.mkv", Size: 3 << 20, Progress: 1},
		{Name: "Oppenheimer (2023)/Oppenheimer.2023.chs.srt", Size: 1, Progress: 1},
	}
	e.dl.mu.Unlock()
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "hash2"})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Done)
	var row models.MediaTransferHistory
	require.NoError(t, e.db.Where("source_path = ?", filepath.Join(inLib, "Oppenheimer (2023)", "Oppenheimer.2023.mkv")).First(&row).Error)
	assert.Equal(t, models.MediaTriggerImport, row.Trigger)
	assert.Zero(t, row.DownloaderID)
	assert.Empty(t, row.Extras)
}

// 清理入库链接只看下载器的种子：导入的记录不动
func TestReconcileSkipsImported(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1, DeleteLinksOnRemove: true})
	e.tree(e.movies, map[string]int{"A [tmdbid=872585]/a.mkv": 3}, nil)
	sc := e.scanImport(e.libraryNamed("电影").ID, "")
	_, err := e.svc.QueueImport(e.ctx, sc.ID, []string{sc.Items[0].Path}, false)
	require.NoError(t, err)
	e.svc.Reconcile(e.ctx)
	row := e.history()[0]
	assert.Equal(t, models.MediaTransferDone, row.Status)
	assert.FileExists(t, row.TargetPath)
}

// 导入时补刮：只补没有的 NFO 与图片（别人放的文件不换，开着覆盖也一样）；媒体库关着刮削时不刮
func TestImportScrapesMissingFiles(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeOverwrite: true})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv})
	e.settings(SettingsInput{MinVideoMB: 1})
	dir := filepath.Join(e.movies, "Oppenheimer (2023)")
	e.tree(e.movies, map[string]int{"Oppenheimer (2023)/Oppenheimer.2023.mkv": 3}, map[string]string{"Oppenheimer (2023)/poster.jpg": "我的海报"})
	sc := e.scanImport(e.libraryNamed("电影").ID, "")
	run, err := e.svc.QueueImport(e.ctx, sc.ID, []string{dir}, true)
	require.NoError(t, err)
	assert.Equal(t, 1, run.Scraped)
	assert.Equal(t, "我的海报", readFile(t, filepath.Join(dir, "poster.jpg")), "别人放的海报不换")
	assert.FileExists(t, filepath.Join(dir, "Oppenheimer.2023.nfo"), "缺的 NFO 补上")
	assert.FileExists(t, filepath.Join(dir, "fanart.jpg"))
	row := e.history()[0]
	assert.NotEmpty(t, row.ScrapeStatus)
	assert.NotEmpty(t, row.Extras, "补上的文件记在记录里（删除时只删这些）")

	// 目录里已经有别的工具写的 movie.nfo：补刮不另写 <文件名>.nfo（Jellyfin 先读它，会盖住原来的），也不换 movie.nfo；
	// 开着覆盖重新刮削时换的是 movie.nfo
	arr := filepath.Join(e.movies, "Arrival (2016)")
	e.tree(e.movies, map[string]int{"Arrival (2016)/Arrival.2016.mkv": 3}, map[string]string{"Arrival (2016)/movie.nfo": oppNFO})
	sc = e.scanImport(e.libraryNamed("电影").ID, "")
	run, err = e.svc.QueueImport(e.ctx, sc.ID, []string{arr}, true)
	require.NoError(t, err)
	assert.Equal(t, 1, run.Scraped)
	assert.NoFileExists(t, filepath.Join(arr, "Arrival.2016.nfo"))
	assert.Equal(t, oppNFO, readFile(t, filepath.Join(arr, "movie.nfo")), "补刮不换别的工具写的 NFO")
	assert.FileExists(t, filepath.Join(arr, "poster.jpg"))
	var imported models.MediaTransferHistory
	require.NoError(t, e.db.Where("target_path = ?", filepath.Join(arr, "Arrival.2016.mkv")).First(&imported).Error)
	_, err = e.svc.rescrape(e.ctx, imported.ID)
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(arr, "Arrival.2016.nfo"))
	assert.Contains(t, readFile(t, filepath.Join(arr, "movie.nfo")), "<title>奥本海默</title>", "开着覆盖：换的是 movie.nfo")
	require.NoError(t, e.db.First(&imported, imported.ID).Error)
	assert.Contains(t, imported.Extras, "movie.nfo", "pt-tools 写过的 movie.nfo 记在记录里")
	// 连文件删除：目录里还有别的视频时 movie.nfo 留着（几个版本共用）；没有视频了才删
	other := filepath.Join(arr, "Arrival.2016.2160p.mkv")
	require.NoError(t, os.WriteFile(other, make([]byte, 2<<20), 0o644))
	_, err = e.svc.DeleteHistoryWith(e.ctx, imported.ID, DeleteOptions{Files: true, AllowExisting: true})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(arr, "Arrival.2016.mkv"))
	assert.FileExists(t, filepath.Join(arr, "movie.nfo"), "目录里还有视频：movie.nfo 留着")

	e.tree(e.tv, map[string]int{"The Last of Us (2023)/S01E01.mkv": 3}, nil)
	sc = e.scanImport(e.libraryNamed("剧集").ID, "")
	run, err = e.svc.QueueImport(e.ctx, sc.ID, []string{sc.Items[0].Path}, true)
	require.NoError(t, err)
	assert.Equal(t, 1, run.Created)
	assert.Zero(t, run.Scraped, "媒体库关着刮削")
	assert.Empty(t, run.Messages, "关着刮削时不去刮")
	assert.NoFileExists(t, filepath.Join(e.tv, "The Last of Us (2023)", "tvshow.nfo"))

	// 补刮失败写进结果（导入用扫描时取到的详情，补刮要现在的数据）
	e.library(LibraryInput{Name: "动画", Kind: models.MediaKindTV, Path: e.anime, Scrape: true})
	e.tree(e.anime, map[string]int{"Sousou no Frieren/S01E01.mkv": 3}, nil)
	sc = e.scanImport(e.libraryNamed("动画").ID, "")
	require.NotNil(t, sc.Items[0].Match, sc.Items[0].Problem)
	e.tmdb.setDown(true)
	run, err = e.svc.QueueImport(e.ctx, sc.ID, []string{sc.Items[0].Path}, true)
	require.NoError(t, err)
	assert.Equal(t, 1, run.Created)
	assert.Zero(t, run.Scraped)
	require.Len(t, run.Messages, 1)
	assert.Contains(t, run.Messages[0], "Sousou no Frieren：补刮失败")
}

// 导入在队列里分段做：一段做完重新排到队尾，队列满了接着做；后台在跑时排进队列，进度在扫描里
func TestImportRunsInQueue(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	files := map[string]int{}
	for i := range importChunk + 5 {
		files[filepath.Join("M"+strings.Repeat("x", i)+" [tmdbid=872585]", "a.mkv")] = 1
	}
	e.tree(e.movies, files, nil)
	sc := e.scanImport(e.libraryNamed("电影").ID, "")
	require.Len(t, sc.Items, importChunk+5)
	var paths []string
	for _, it := range sc.Items {
		paths = append(paths, it.Path)
	}

	tasks := func() *importRun {
		r := &importRun{libID: e.libraryNamed("电影").ID, minSize: 1 << 20, state: &ImportRun{Status: ImportQueued, Items: len(paths)}}
		for _, it := range sc.Items {
			r.tasks = append(r.tasks, importTask{path: it.Path, name: it.Name, tmdbID: 872585, source: importSourceDirname})
		}
		return r
	}

	// 一次最多做 importChunk 项：没做完时回假，进度停在这一段，下一次接着做
	r := tasks()
	assert.False(t, e.svc.runImport(e.ctx, r))
	assert.Equal(t, importChunk, r.state.Done)
	assert.Equal(t, ImportRunning, r.state.Status)
	assert.True(t, e.svc.runImport(e.ctx, r))
	assert.Equal(t, ImportDone, r.state.Status)
	assert.Equal(t, importChunk+5, r.state.Created)
	require.NoError(t, e.db.Where("1 = 1").Delete(&models.MediaTransferHistory{}).Error)

	// 队列满了：一段做完排不回去，接着做完
	r = tasks()
	run := r.state
	for range jobQueueSize {
		e.svc.jobs <- job{rescrape: 1}
	}
	e.svc.handleImport(e.ctx, job{imp: r})
	assert.Equal(t, ImportDone, run.Status)
	assert.Equal(t, importChunk+5, run.Created)
	for range jobQueueSize {
		<-e.svc.jobs
	}
	require.NoError(t, e.db.Where("1 = 1").Delete(&models.MediaTransferHistory{}).Error)

	// 后台在跑：排进队列，第一段做完排回队尾，做完以后进度在扫描里
	e.svc.Start()
	t.Cleanup(e.svc.Stop)
	sc = e.scanImport(e.libraryNamed("电影").ID, "")
	queued, err := e.svc.QueueImport(e.ctx, sc.ID, paths, false)
	require.NoError(t, err)
	assert.Equal(t, importChunk+5, queued.Items)
	require.Eventually(t, func() bool {
		got, serr := e.svc.ImportScanStatus(sc.ID, false)
		return serr == nil && got.Import.Status == ImportDone
	}, 20*time.Second, 20*time.Millisecond)
	got, err := e.svc.ImportScanStatus(sc.ID, false)
	require.NoError(t, err)
	assert.Equal(t, importChunk+5, got.Import.Created)
	assert.Equal(t, importChunk+5, got.Import.Done)
	assert.Len(t, e.history(), importChunk+5)
}

// 导入到一半媒体库删了、pt-tools 停了：写明没导入完
func TestImportStops(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	r := &importRun{libID: 99, tasks: []importTask{{path: "/x"}}, state: &ImportRun{}}
	assert.True(t, e.svc.runImport(e.ctx, r))
	assert.Equal(t, ImportFailed, r.state.Status)
	assert.Equal(t, []string{"媒体库删了，没导入完"}, r.state.Messages)

	ctx, cancel := context.WithCancel(e.ctx)
	cancel()
	r = &importRun{libID: e.libraryNamed("电影").ID, tasks: []importTask{{path: "/x"}}, state: &ImportRun{}}
	assert.True(t, e.svc.runImport(ctx, r))
	assert.Equal(t, ImportFailed, r.state.Status)
	assert.Contains(t, r.state.Messages[0], "pt-tools 停止了")
}

// 每周补刮：开着的库，上次补刮在 7 天以前的，刮削结果是部分成功或失败的条目排进队列（只补没有的文件）
func TestScrapeFill(t *testing.T) {
	e := newEnv(t)
	fill := e.library(LibraryInput{Name: "补刮", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeFill: true})
	e.library(LibraryInput{Name: "不补", Kind: models.MediaKindTV, Path: e.tv, Scrape: true})
	assert.True(t, fill.ScrapeFill)
	mk := func(lib uint, tmdbID int, status, src string) models.MediaTransferHistory {
		r := models.MediaTransferHistory{
			LibraryID: lib, SourcePath: src, TargetPath: src + ".lib", Status: models.MediaTransferDone, MediaType: tmdb.KindMovie, TMDBID: tmdbID, ScrapeStatus: status,
		}
		require.NoError(t, e.db.Create(&r).Error)
		return r
	}
	a1 := mk(fill.ID, 1, models.MediaScrapePartial, "/a1")
	mk(fill.ID, 1, models.MediaScrapeFailed, "/a2")
	b := mk(fill.ID, 2, models.MediaScrapeFailed, "/b")
	mk(fill.ID, 3, models.MediaScrapeOK, "/c")
	mk(e.libraryNamed("不补").ID, 4, models.MediaScrapeFailed, "/d")
	e.svc.scrapeFill(e.ctx)
	jobs := e.drainJobs()
	require.Len(t, jobs, 2)
	assert.ElementsMatch(t, []uint{a1.ID, b.ID}, []uint{jobs[0].rescrape, jobs[1].rescrape}, "每个条目排一次（最早的记录）")
	assert.True(t, jobs[0].fill && jobs[1].fill)
	lib := e.libraryNamed("补刮")
	require.NotNil(t, lib.ScrapeFilledAt)

	e.svc.scrapeFill(e.ctx)
	assert.Empty(t, e.drainJobs(), "7 天里不再补")
	e.advance(8 * 24 * time.Hour)
	e.svc.scrapeFill(e.ctx)
	assert.Len(t, e.drainJobs(), 2)

	// 队列满了：不记补刮的时间，下次再补
	e.advance(8 * 24 * time.Hour)
	for range jobQueueSize {
		e.svc.jobs <- job{rescrape: 9999}
	}
	e.svc.scrapeFill(e.ctx)
	assert.Equal(t, lib.ScrapeFilledAt.Add(8*24*time.Hour).Unix(), e.libraryNamed("补刮").ScrapeFilledAt.Unix())
	e.drainJobs()
}

// drainJobs 取出队列里的任务（测试自己处理），从去重表里拿掉。
func (e *env) drainJobs() []job {
	var out []job
	for {
		select {
		case j := <-e.svc.jobs:
			e.svc.qmu.Lock()
			delete(e.svc.queued, j.key())
			e.svc.qmu.Unlock()
			out = append(out, j)
		default:
			return out
		}
	}
}

// 补刮只补没有的文件：媒体库开着覆盖也不换别人放的；pt-tools 写的照样换成新的
func TestRescrapeFillKeepsOthersFiles(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeOverwrite: true})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	row := e.history()[0]
	dir := filepath.Dir(row.TargetPath)
	poster := filepath.Join(dir, "poster.jpg")
	require.NoError(t, os.WriteFile(poster, []byte("我的海报"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(dir, "fanart.jpg")))
	e.svc.mu.Lock()
	_, err = e.svc.rescrapeWith(e.ctx, row.ID, true)
	e.svc.mu.Unlock()
	require.NoError(t, err)
	assert.Equal(t, "我的海报", readFile(t, poster), "补刮：别人改过的不换")
	assert.FileExists(t, filepath.Join(dir, "fanart.jpg"), "缺的补上")
	e.svc.mu.Lock()
	_, err = e.svc.rescrapeWith(e.ctx, row.ID, false)
	e.svc.mu.Unlock()
	require.NoError(t, err)
	assert.NotEqual(t, "我的海报", readFile(t, poster), "重新刮削：开着覆盖时换")

	// 队列里的补刮任务
	require.NoError(t, os.WriteFile(poster, []byte("又改了"), 0o644))
	_, err = e.svc.handleRescrape(e.ctx, job{rescrape: row.ID, fill: true})
	require.NoError(t, err)
	assert.Equal(t, "又改了", readFile(t, poster))
}

// 演员名用中文：刮削时前 15 位换成 TMDB 上的中文译名；取不到时用原名并写明
func TestScrapeActorChineseNames(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeActorZh: true})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	row := e.history()[0]
	nfo := strings.TrimSuffix(row.TargetPath, filepath.Ext(row.TargetPath)) + ".nfo"
	assert.Contains(t, readFile(t, nfo), "<name>基里安·墨菲</name>")
	assert.Contains(t, readFile(t, nfo), "<role>J. Robert Oppenheimer</role>")

	e.tmdb.mu.Lock()
	e.tmdb.personDown = true
	e.tmdb.mu.Unlock()
	cachePurge(t, e)
	_, err = e.svc.Rescrape(e.ctx, row.ID)
	require.NoError(t, err)
	assert.Contains(t, readFile(t, nfo), "<name>Cillian Murphy</name>", "取不到中文名时用原名")
	assert.Contains(t, e.history()[0].ScrapeNote, "有的演员没取到中文名")

	// 关着时用原名（取得到译名也不用）
	e.tmdb.mu.Lock()
	e.tmdb.personDown = false
	e.tmdb.mu.Unlock()
	cachePurge(t, e)
	lib := e.libraryNamed("电影")
	v, err := e.svc.SaveLibrary(e.ctx, lib.ID, LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, Enabled: true})
	require.NoError(t, err)
	assert.False(t, v.ScrapeActorZh)
	_, err = e.svc.Rescrape(e.ctx, row.ID)
	require.NoError(t, err)
	assert.Contains(t, readFile(t, nfo), "<name>Cillian Murphy</name>")
	assert.NotContains(t, readFile(t, nfo), "基里安·墨菲")
}

// cachePurge 清掉 TMDB 的缓存（演员译名缓存 30 天）。
func cachePurge(t *testing.T, e *env) {
	t.Helper()
	require.NoError(t, e.db.Where("1 = 1").Delete(&models.MediaCache{}).Error)
}

// 媒体库的「演员名用中文」「每周补一次缺的刮削」要开着刮削才有
func TestLibraryScrapeOptions(t *testing.T) {
	e := newEnv(t)
	v := e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, ScrapeActorZh: true, ScrapeFill: true})
	assert.False(t, v.ScrapeActorZh, "关着刮削")
	assert.False(t, v.ScrapeFill)
	v, err := e.svc.SaveLibrary(e.ctx, v.ID, LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeActorZh: true, ScrapeFill: true, Enabled: true})
	require.NoError(t, err)
	assert.True(t, v.ScrapeActorZh)
	assert.True(t, v.ScrapeFill)
	lib := e.libraryNamed("电影")
	assert.True(t, lib.ScrapeActorZh && lib.ScrapeFill, "保存进库里")
}

func TestReadNFO(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(text), 0o644))
		return p
	}
	assert.Equal(t, nfoIDs{TMDB: 872585, IMDb: "tt0000001"}, readNFO(write("a.nfo", oppNFO), tmdb.KindMovie), "演员里的编号不算")
	assert.Equal(t, nfoIDs{}, readNFO(write("b.nfo", oppNFO), tmdb.KindTV), "根元素要是 tvshow")
	assert.Equal(t, nfoIDs{TMDB: 5, IMDb: "tt1234567"},
		readNFO(write("c.nfo", "\xef\xbb\xbf<?xml version=\"1.0\" encoding=\"GBK\"?><tvshow><tmdbid>5</tmdbid><id>tt1234567</id></tvshow>"), tmdb.KindTV), "带 BOM、声明成 GBK 的")
	assert.Equal(t, nfoIDs{IMDb: "tt7654321"}, readNFO(write("d.nfo", "<movie><imdb_id>tt7654321</imdb_id><tmdbid>abc</tmdbid><id>12</id></movie>"), tmdb.KindMovie))
	assert.Equal(t, nfoIDs{TMDB: 100088, IMDb: "tt3581920"},
		readNFO(write("e.nfo", "https://www.themoviedb.org/tv/100088\nhttps://www.imdb.com/title/tt3581920/\n"), tmdb.KindTV), "只写了网址")
	assert.Equal(t, nfoIDs{}, readNFO(write("f.nfo", "https://www.themoviedb.org/tv/100088"), tmdb.KindMovie), "网址的类型不对")
	assert.Equal(t, nfoIDs{}, readNFO(write("g.nfo", "<episodedetails><tmdbid>1</tmdbid></episodedetails> https://www.themoviedb.org/movie/9"), tmdb.KindMovie),
		"别的根元素：不看网址")
	assert.Equal(t, nfoIDs{}, readNFO(filepath.Join(dir, "missing.nfo"), tmdb.KindMovie))
	assert.Equal(t, nfoIDs{TMDB: 7}, readNFO(write("h.nfo", "<movie><uniqueid type=\"TMDB\"> 7 </uniqueid><broken"), tmdb.KindMovie), "后面坏了也用读到的")
}

func TestDirTags(t *testing.T) {
	for name, want := range map[string]nfoIDs{
		"Inception (2010) [tmdbid=27205]":         {TMDB: 27205},
		"Inception (2010) {tmdb-27205}":           {TMDB: 27205},
		"Inception.2010.tmdbid-27205":             {TMDB: 27205},
		"Inception (2010) [imdbid=tt1375666]":     {IMDb: "tt1375666"},
		"Inception {imdb-TT1375666} [tmdb=27205]": {TMDB: 27205, IMDb: "tt1375666"},
		"Inception (2010)":                        {},
		"xtmdb-27205":                             {},
	} {
		assert.Equal(t, want, dirTags(name), name)
	}
	assert.Equal(t, "Inception (2010)", stripTags("Inception (2010) [tmdbid=27205]"))
	assert.Equal(t, "Inception", stripTags("Inception {imdb-tt1375666}  [tmdb=27205]"))
}

func TestSeasonDir(t *testing.T) {
	for name, want := range map[string]int{
		"Season 1": 1, "Season 01 (2008)": 1, "S02": 2, "s3": 3, "Series 4": 4, "第五季": 5, "第 12 季": 12,
		"Specials": 0, "Special": 0, "特别篇": 0, "Sherlock": -1, "Extras": -1, "Season": -1,
	} {
		assert.Equal(t, want, seasonDir(name), name)
	}
}

// 一次最多列 importMaxItems 项；放着几项的目录最多往下找 importMaxNest 层
func TestDiscoverImportLimits(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	lib := e.libraryNamed("电影")
	files := map[string]int{}
	for i := range importMaxItems + 1 {
		files[strings.Repeat("a", i%50+1)+"/"+strings.Repeat("b", i/50+1)+".mkv"] = 0
	}
	e.tree(e.movies, files, nil)
	// 文件是空的：按 0 MB 当作视频（最小体积 0 时用默认值，这里直接看找出的项）
	entries, truncated, err := discoverImport(e.ctx, lib, e.movies, nil, 0)
	require.NoError(t, err)
	assert.Len(t, entries, 50, "空文件不是视频：每个目录是一项")
	assert.False(t, truncated)

	big := t.TempDir()
	for i := range importMaxItems + 1 {
		p := filepath.Join(big, strings.Repeat("m", i%40+1)+strings.Repeat("x", i/40)+".mkv")
		f, cerr := os.Create(p)
		require.NoError(t, cerr)
		require.NoError(t, f.Truncate(2<<20))
		require.NoError(t, f.Close())
	}
	lib.Path = big
	entries, truncated, err = discoverImport(e.ctx, lib, big, nil, 1<<20)
	require.NoError(t, err)
	assert.Len(t, entries, importMaxItems)
	assert.True(t, truncated, "多的分子目录扫描")

	deep := t.TempDir()
	nested := filepath.Join(deep, "a", "b", "c", "d", "e", "f")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "x.mkv"), make([]byte, 2<<20), 0o644))
	lib.Path = deep
	entries, _, err = discoverImport(e.ctx, lib, deep, nil, 1<<20)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, filepath.Join(deep, "a", "b", "c", "d", "e"), entries[0].path, "太深的当作一项")

	ctx, cancel := context.WithCancel(e.ctx)
	cancel()
	_, _, err = discoverImport(ctx, lib, deep, nil, 1<<20)
	assert.ErrorIs(t, err, context.Canceled)
}

// 一项里太深、读不了库目录、二级分类的规则读不出来
func TestImportScanEdgeCases(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	lib := e.libraryNamed("电影")
	e.tree(e.movies, map[string]int{"Deep/a/b/c/d/e/f/x.mkv": 3, "华语电影/Oppenheimer.2023.mkv": 3}, map[string]string{"Deep/movie.nfo": oppNFO})
	require.NoError(t, e.db.Model(&models.MediaOrganizeSetting{}).Where("id = 1").Update("classify_rules", "{bad").Error)
	sc := e.scanImport(lib.ID, "")
	require.Equal(t, ImportDone, sc.Status, sc.Error)
	items := importItems(sc)
	assert.Contains(t, items["Deep"].Problem, "目录超过 6 层或文件超过 5000 个")
	assert.Contains(t, items, "华语电影/Oppenheimer.2023.mkv", "规则读不出来时用预设的分类名")

	if os.Getuid() == 0 || filepath.Separator == '\\' {
		t.Skip("root 与 Windows 上去不掉读权限")
	}
	require.NoError(t, os.Chmod(e.movies, 0o300))
	t.Cleanup(func() { _ = os.Chmod(e.movies, 0o755) })
	sc = e.scanImport(lib.ID, "")
	assert.Equal(t, ImportFailed, sc.Status)
	assert.Contains(t, sc.Error, "读取目录失败")
}
