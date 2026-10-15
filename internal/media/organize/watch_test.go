package organize

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// watchEnv 是带一个监控目录的环境：电影与剧集媒体库，监控目录在临时目录的 incoming 下。
func watchEnv(t *testing.T) (*env, models.MediaWatchDir) {
	t.Helper()
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	in := filepath.Join(e.root, "incoming")
	require.NoError(t, os.MkdirAll(in, 0o755))
	d, err := e.svc.SaveWatchDir(e.ctx, 0, WatchDirInput{Path: in, Enabled: true})
	require.NoError(t, err)
	return e, d
}

// put 在监控目录里放一个文件（大小 MB），时间设成 age 以前（还在写的文件最近有改动）。
func (e *env) put(dir models.MediaWatchDir, rel string, mb int, age time.Duration) string {
	e.t.Helper()
	p := filepath.Join(dir.Path, filepath.FromSlash(rel))
	require.NoError(e.t, os.MkdirAll(filepath.Dir(p), 0o755))
	f, err := os.Create(p)
	require.NoError(e.t, err)
	require.NoError(e.t, f.Truncate(int64(mb)<<20))
	require.NoError(e.t, f.Close())
	at := e.Now().Add(-age)
	for q := p; q != dir.Path && len(q) > len(dir.Path); q = filepath.Dir(q) {
		require.NoError(e.t, os.Chtimes(q, at, at))
	}
	return p
}

func (e *env) queuedJobs() []job {
	var out []job
	for {
		select {
		case j := <-e.svc.jobs:
			// 和 handle 一样先从去重表里拿掉，下一轮扫描才能再排
			e.svc.qmu.Lock()
			delete(e.svc.queued, jobKey(j.req.DownloaderID, j.dlName, j.req.queueHash()))
			e.svc.qmu.Unlock()
			out = append(out, j)
		default:
			return out
		}
	}
}

// 监控目录里放进一部电影：按名字识别、放进电影库，记录里写明是目录监控、没有下载器
func TestWatchDirOrganize(t *testing.T) {
	e, d := watchEnv(t)
	src := e.put(d, "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 10*time.Minute)

	res, err := e.svc.Organize(e.ctx, Request{WatchDirID: d.ID, Entry: "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv"})
	require.NoError(t, err)
	require.Empty(t, res.Plan.Problem)
	require.Equal(t, 1, res.Created)
	want := filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023) - 2160p BluRay H.265.mkv")
	assert.True(t, sameFile(t, src, want), "硬链接进电影库")
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, d.ID, rows[0].WatchDirID)
	assert.Zero(t, rows[0].DownloaderID)
	assert.Equal(t, watchSource, rows[0].DownloaderName)
	assert.Equal(t, watchHash(d.ID, "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv"), rows[0].InfoHash)
	assert.Equal(t, "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", rows[0].TorrentName)
	notices := e.gotNotices()
	require.Len(t, notices, 0, "没设入库通知")

	// 重新整理（按记录）
	res, err = e.svc.Retry(e.ctx, rows[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Done, "已经在库里")
}

// 剧集文件夹：最近还在复制时先不整理；写完以后整理，之后又多了一集时再整理那一集
func TestWatchDirFolderAndScan(t *testing.T) {
	e, d := watchEnv(t)
	entry := "The.Last.of.Us.S01.2160p.WEB-DL"
	e.put(d, entry+"/The.Last.of.Us.S01E01.2160p.WEB-DL.mkv", 2, time.Minute)

	p, err := e.svc.Preview(e.ctx, Request{WatchDirID: d.ID, Entry: entry})
	require.NoError(t, err)
	assert.Contains(t, p.Problem, "还有改动")
	e.svc.watchScan(e.ctx, e.mustSettings())
	assert.Empty(t, e.queuedJobs(), "还在写的不排")

	e.advance(5 * time.Minute)
	e.svc.watchScan(e.ctx, e.mustSettings())
	jobs := e.queuedJobs()
	require.Len(t, jobs, 1)
	assert.Equal(t, models.MediaTriggerWatch, jobs[0].trigger)
	res, err := e.svc.Organize(e.ctx, jobs[0].req)
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	assert.True(t, exists(filepath.Join(e.tv, "最后生还者 (2023)", "Season 1", "最后生还者 - S01E01 - 当你迷失在黑暗中.mkv")))

	e.advance(time.Minute)
	e.svc.watchScan(e.ctx, e.mustSettings())
	assert.Empty(t, e.queuedJobs(), "整理过、没再改动的不排")

	e.put(d, entry+"/The.Last.of.Us.S01E02.2160p.WEB-DL.mkv", 2, 0)
	e.advance(5 * time.Minute)
	e.svc.watchScan(e.ctx, e.mustSettings())
	jobs = e.queuedJobs()
	require.Len(t, jobs, 1, "多了一集")
	res, err = e.svc.Organize(e.ctx, jobs[0].req)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 1, res.Done)

	// 关掉目录：不再扫
	_, err = e.svc.SaveWatchDir(e.ctx, d.ID, WatchDirInput{Path: d.Path, Enabled: false})
	require.NoError(t, err)
	e.put(d, "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 10*time.Minute)
	e.svc.watchScan(e.ctx, e.mustSettings())
	assert.Empty(t, e.queuedJobs())
}

// 跳过隐藏的、NAS 的系统目录与还没下载完的文件
func TestSkipWatchEntry(t *testing.T) {
	for _, n := range []string{".DS_Store", "@eaDir", "#recycle", "$RECYCLE.BIN", "lost+found", "a.mkv.part", "b.!qB", "x.mkv.pt-tools-new-1.tmp"} {
		assert.True(t, skipWatchEntry(n), n)
	}
	for _, n := range []string{"Movie.2023.mkv", "Show S01"} {
		assert.False(t, skipWatchEntry(n), n)
	}
}

// 监控目录不能是媒体库、在媒体库里或包含媒体库；不能重复
func TestSaveWatchDirValidation(t *testing.T) {
	e, d := watchEnv(t)
	for _, bad := range []string{e.movies, filepath.Join(e.movies, "in"), filepath.Join(e.root, "media"), "relative", d.Path} {
		_, err := e.svc.SaveWatchDir(e.ctx, 0, WatchDirInput{Path: bad, Enabled: true})
		require.ErrorIs(t, err, ErrInvalid, bad)
	}
	list, err := e.svc.WatchDirs(e.ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.NoError(t, e.svc.DeleteWatchDir(e.ctx, d.ID))
	require.ErrorIs(t, e.svc.DeleteWatchDir(e.ctx, d.ID), ErrNotFound)
	_, err = e.svc.Organize(e.ctx, Request{WatchDirID: d.ID, Entry: "x.mkv"})
	require.ErrorIs(t, err, ErrNotFound)
}

// 种子连数据删掉后清理入库链接：目录监控整理出的不跟着删
func TestReconcileSkipsWatchRows(t *testing.T) {
	e, d := watchEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1, DeleteLinksOnRemove: true})
	src := e.put(d, "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 10*time.Minute)
	_, err := e.svc.Organize(e.ctx, Request{WatchDirID: d.ID, Entry: filepath.Base(src)})
	require.NoError(t, err)
	require.NoError(t, os.Remove(src))
	assert.Zero(t, e.svc.Reconcile(e.ctx))
	assert.True(t, exists(e.history()[0].TargetPath))
}

func TestWatchRequestValidation(t *testing.T) {
	e, d := watchEnv(t)
	for _, bad := range []string{"", "..", "a/b", `a\b`} {
		_, err := e.svc.Preview(e.ctx, Request{WatchDirID: d.ID, Entry: bad})
		require.Error(t, err, bad)
	}
}

func (e *env) mustSettings() Settings {
	e.t.Helper()
	set, err := e.svc.Settings(e.ctx)
	require.NoError(e.t, err)
	return set
}

// 媒体库用移动整理时，监控目录里是下载器的种子的项不整理（移走了种子没法做种）；连不上下载器时确认不了，过一阵再试
func TestWatchDirMoveSkipsTorrents(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeMove})
	e.settings(SettingsInput{MinVideoMB: 1})
	// 监控了下载目录：里面有一个下载器的种子，还有一个别的工具放进来的
	d, err := e.svc.SaveWatchDir(e.ctx, 0, WatchDirInput{Path: e.dlDir, Enabled: true})
	require.NoError(t, err)
	seeding := "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv"
	e.addTorrent("h1", seeding, map[string]int{seeding: 5}, nil)
	old := e.Now().Add(-10 * time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(e.dlDir, seeding), old, old))

	res, err := e.svc.Organize(e.ctx, Request{WatchDirID: d.ID, Entry: seeding})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "是下载器里的种子（qb：")
	assert.True(t, exists(filepath.Join(e.dlDir, seeding)), "种子的文件不动")
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferFailed, rows[0].Status)
	assert.Nil(t, rows[0].NextRetryAt, "改了设置再手动重试")

	e.dl.mu.Lock()
	e.dl.listErr = errors.New("连接被拒绝")
	e.dl.mu.Unlock()
	other := e.put(d, "Oppenheimer.2023.1080p.WEB-DL.H264.mkv", 5, 10*time.Minute)
	res, err = e.svc.Organize(e.ctx, Request{WatchDirID: d.ID, Entry: filepath.Base(other)})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "连接被拒绝")
	assert.True(t, exists(other))
	rows = e.history()
	require.Len(t, rows, 2)
	assert.NotNil(t, rows[1].NextRetryAt, "确认不了的过一阵再试")

	e.dl.mu.Lock()
	e.dl.listErr = nil
	e.dl.mu.Unlock()
	res, err = e.svc.Organize(e.ctx, Request{WatchDirID: d.ID, Entry: filepath.Base(other)})
	require.NoError(t, err)
	require.Empty(t, res.Plan.Problem)
	assert.Equal(t, 1, res.Created)
	assert.False(t, exists(other), "不是种子的照常移动")
}

// 媒体库的目录不能是监控目录、在监控目录里或包含监控目录（库里的文件会被当成放进来的视频再整理一遍）
func TestSaveLibraryRejectsWatchDir(t *testing.T) {
	e, d := watchEnv(t)
	for _, bad := range []string{d.Path, filepath.Join(d.Path, "movies"), e.root} {
		_, err := e.svc.SaveLibrary(e.ctx, 0, LibraryInput{Name: "新库", Kind: models.MediaKindMovie, Path: bad, Enabled: true})
		require.ErrorIs(t, err, ErrInvalid, bad)
	}
	libs, err := e.svc.Libraries(e.ctx)
	require.NoError(t, err)
	_, err = e.svc.SaveLibrary(e.ctx, libs[0].ID, LibraryInput{Name: libs[0].Name, Kind: libs[0].Kind, Path: filepath.Join(d.Path, "x"), Enabled: true})
	require.ErrorIs(t, err, ErrInvalid, "改成监控目录里的路径也不行")
}

// 目录监控的一项整理出错（不是写进记录的失败）：按这一项退避，退避到期以后扫描再排它
func TestWatchFailBackoff(t *testing.T) {
	e, d := watchEnv(t)
	name := "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv"
	e.put(d, name, 5, 10*time.Minute)
	e.svc.watchScan(e.ctx, e.mustSettings())
	jobs := e.queuedJobs()
	require.Len(t, jobs, 1)

	e.svc.fail(e.ctx, jobs[0])
	assert.True(t, e.svc.backedOff(0, watchHash(d.ID, name)))
	e.svc.watchScan(e.ctx, e.mustSettings())
	assert.Empty(t, e.queuedJobs(), "退避期内不排")

	e.advance(3 * sweepInterval)
	e.svc.watchScan(e.ctx, e.mustSettings())
	require.Len(t, e.queuedJobs(), 1, "退避到期以后再排（没再改动也排）")
}

// 文件夹里：隐藏的、NAS 的系统目录、没下载完的、软链接不算；最近一次改动取文件与子目录里最新的
func TestWatchFiles(t *testing.T) {
	e, d := watchEnv(t)
	entry := "Show.S01"
	for _, rel := range []string{".hidden/x.mkv", "@eaDir/x.mkv", "S01E02.mkv.part", "S01E01.mkv"} {
		e.put(d, entry+"/"+rel, 1, time.Hour)
	}
	newer := e.put(d, entry+"/extra/S01E03.mkv", 1, time.Hour)
	if os.Symlink(newer, filepath.Join(d.Path, entry, "link.mkv")) == nil {
		require.NoError(t, os.Symlink(filepath.Join(d.Path, entry), filepath.Join(d.Path, "link-entry")))
		_, err := e.svc.Preview(e.ctx, Request{WatchDirID: d.ID, Entry: "link-entry"})
		require.ErrorContains(t, err, "不是文件也不是文件夹", "这一项本身是软链接")
	}
	root, sub, file := e.Now().Add(-time.Hour), e.Now().Add(-30*time.Minute), e.Now().Add(-time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(d.Path, entry), root, root))
	require.NoError(t, os.Chtimes(filepath.Dir(newer), sub, sub))
	require.NoError(t, os.Chtimes(newer, file, file))

	files, newest, err := watchFiles(d.Path, entry)
	require.NoError(t, err)
	var rels []string
	for _, f := range files {
		rels = append(rels, f.Rel)
	}
	assert.ElementsMatch(t, []string{entry + "/S01E01.mkv", entry + "/extra/S01E03.mkv"}, rels)
	assert.True(t, newest.Equal(file), newest.String())

	_, err = e.svc.Preview(e.ctx, Request{WatchDirID: d.ID, Entry: "missing.mkv"})
	require.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.Organize(e.ctx, Request{WatchDirID: d.ID})
	require.ErrorIs(t, err, ErrInvalid, "没写是哪一项")
	_, err = e.svc.Preview(e.ctx, Request{})
	require.ErrorIs(t, err, ErrInvalid, "既不是种子也不是监控目录")
}

// 最多 20 个监控目录；改成已有的目录、改不存在的都不行
func TestSaveWatchDirLimits(t *testing.T) {
	e, d := watchEnv(t)
	for i := 1; i < maxWatchDirs; i++ {
		_, err := e.svc.SaveWatchDir(e.ctx, 0, WatchDirInput{Path: filepath.Join(e.root, "in", strconv.Itoa(i))})
		require.NoError(t, err)
	}
	_, err := e.svc.SaveWatchDir(e.ctx, 0, WatchDirInput{Path: filepath.Join(e.root, "in", "x")})
	require.ErrorIs(t, err, ErrInvalid)
	list, err := e.svc.WatchDirs(e.ctx)
	require.NoError(t, err)
	_, err = e.svc.SaveWatchDir(e.ctx, list[1].ID, WatchDirInput{Path: d.Path})
	require.ErrorIs(t, err, ErrInvalid, "和已有的重复")
	_, err = e.svc.SaveWatchDir(e.ctx, 9999, WatchDirInput{Path: filepath.Join(e.root, "in", "y")})
	require.ErrorIs(t, err, ErrNotFound)
}

// 扫描排上的任务按目录监控整理并发入库通知；排上以后目录关掉了就不整理
func TestWatchJob(t *testing.T) {
	e, d := watchEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1, NotifyChannels: []uint{7}})
	e.put(d, "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 10*time.Minute)
	e.svc.watchScan(e.ctx, e.mustSettings())
	require.Equal(t, 1, e.drain())
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTriggerWatch, rows[0].Trigger)
	notices := e.gotNotices()
	require.Len(t, notices, 1)
	assert.Contains(t, notices[0].Text, "来自目录监控：Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv")

	e.put(d, "The.Last.of.Us.S01E01.2160p.WEB-DL.mkv", 2, 10*time.Minute)
	e.svc.watchScan(e.ctx, e.mustSettings())
	_, err := e.svc.SaveWatchDir(e.ctx, d.ID, WatchDirInput{Path: d.Path, Enabled: false})
	require.NoError(t, err)
	require.Equal(t, 1, e.drain())
	assert.Len(t, e.history(), 1, "目录关掉了：不整理")
}

// 目录太深的项整项不整理（不只整理其中一部分），写一条跳过的记录，之后扫描不再排
func TestWatchTooBig(t *testing.T) {
	e, d := watchEnv(t)
	entry := "Deep"
	e.put(d, entry+"/a/b/c/d/e/f/The.Last.of.Us.S01E01.2160p.WEB-DL.mkv", 2, 10*time.Minute)
	_, _, err := watchFiles(d.Path, entry)
	require.ErrorIs(t, err, errWatchTooBig)

	e.svc.watchScan(e.ctx, e.mustSettings())
	require.Equal(t, 1, e.drain())
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferSkipped, rows[0].Status)
	assert.Equal(t, filepath.Join(d.Path, entry), rows[0].SourcePath)
	assert.Contains(t, rows[0].Message, "超过 6 层")

	e.svc.forgetWatch(watchHash(d.ID, entry))
	e.svc.watchScan(e.ctx, e.mustSettings())
	assert.Zero(t, e.drain(), "写过跳过的记录：不再排")
}

// 路径比较：Windows 上不分大小写；经软链接的另一个名字也认得出是同一个位置
func TestWatchPathCompare(t *testing.T) {
	if runtime.GOOS == "windows" {
		assert.True(t, samePath(`C:\Media\In`, `c:\media\in\`))
	} else {
		assert.False(t, samePath("/media/In", "/media/in"))
	}
	assert.Equal(t, "a.mkv", baseName(`D:\Downloads\a.mkv`))
	assert.Equal(t, "a", baseName("/downloads/a/"))

	dir := t.TempDir()
	real := filepath.Join(dir, "downloads")
	require.NoError(t, os.MkdirAll(filepath.Join(real, "x"), 0o755))
	alias := filepath.Join(dir, "alias")
	if os.Symlink(real, alias) != nil {
		t.Skip("这个系统不能建软链接")
	}
	assert.True(t, overlaps(alias, real))
	assert.True(t, overlaps(filepath.Join(alias, "x", "not-yet.mkv"), filepath.Join(real, "x")), "还不存在的路径解开存在的那一段")
	assert.False(t, overlaps(filepath.Join(dir, "other"), real))
}

// 移动整理前认种子：经软链接的另一个名字监控了下载目录；种子的路径在 pt-tools 里找不到时按名字认
func TestWatchTorrentAliases(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeMove})
	e.settings(SettingsInput{MinVideoMB: 1})
	name := "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv"
	e.addTorrent("h1", name, map[string]int{name: 5}, nil)
	alias := filepath.Join(e.root, "alias")
	if os.Symlink(e.dlDir, alias) != nil {
		t.Skip("这个系统不能建软链接")
	}
	owner, err := e.svc.watchTorrent(e.ctx, filepath.Join(alias, name), name)
	require.NoError(t, err)
	assert.Equal(t, "qb："+name, owner)

	// 没配路径映射、路径在 pt-tools 里不存在的种子：同名（不分大小写）就当成是它
	e.addTorrent("h2", "Some.Show.S01", map[string]int{"Some.Show.S01/a.mkv": 2}, func(tr *downloader.Torrent) {
		tr.SavePath, tr.ContentPath = `D:\Downloads`, `D:\Downloads\Some.Show.S01`
	})
	owner, err = e.svc.watchTorrent(e.ctx, filepath.Join(e.root, "incoming", "some.show.s01"), "some.show.s01")
	require.NoError(t, err)
	assert.Equal(t, "qb：Some.Show.S01", owner)
	owner, err = e.svc.watchTorrent(e.ctx, filepath.Join(e.root, "incoming", "Other"), "Other")
	require.NoError(t, err)
	assert.Empty(t, owner)
}

// 监控目录的路径不能改（排着的整理和记录只记着目录编号）；开关可以改
func TestSaveWatchDirPathFixed(t *testing.T) {
	e, d := watchEnv(t)
	_, err := e.svc.SaveWatchDir(e.ctx, d.ID, WatchDirInput{Path: filepath.Join(e.root, "elsewhere"), Enabled: true})
	require.ErrorIs(t, err, ErrInvalid)
	got, err := e.svc.SaveWatchDir(e.ctx, d.ID, WatchDirInput{Path: d.Path + string(filepath.Separator), Enabled: false})
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.Equal(t, d.Path, got.Path)
}

// 排上以后目录关掉：不整理，重新打开以后扫描再排；目录删掉了，失败记录的到期重试不再排
func TestWatchJobDirOffAndGone(t *testing.T) {
	e, d := watchEnv(t)
	name := "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv"
	e.put(d, name, 5, 10*time.Minute)
	e.svc.watchScan(e.ctx, e.mustSettings())
	_, err := e.svc.SaveWatchDir(e.ctx, d.ID, WatchDirInput{Path: d.Path, Enabled: false})
	require.NoError(t, err)
	require.Equal(t, 1, e.drain())
	assert.Empty(t, e.history())

	_, err = e.svc.SaveWatchDir(e.ctx, d.ID, WatchDirInput{Path: d.Path, Enabled: true})
	require.NoError(t, err)
	e.svc.watchScan(e.ctx, e.mustSettings())
	require.Equal(t, 1, e.drain(), "重新打开以后再排")
	require.Len(t, e.history(), 1)

	// 失败的记录到期重试时目录已经删了：不再重试
	row := e.history()[0]
	next := e.Now().Add(-time.Minute)
	require.NoError(t, e.db.Model(&row).Updates(map[string]any{"status": models.MediaTransferFailed, "next_retry_at": &next}).Error)
	require.NoError(t, e.svc.DeleteWatchDir(e.ctx, d.ID))
	e.svc.retryDue(e.ctx, e.mustSettings())
	require.Equal(t, 1, e.drain())
	assert.Nil(t, e.history()[0].NextRetryAt)
}

// 移动整理以后同一个位置上又放进一个（大小不同的）文件：记录管着库里的文件，不改成新文件的、写明原因，
// 之后扫描不再排；删掉那条记录以后照常整理。记录不管库里的文件时直接按新文件整理
func TestWatchOtherFileAtSamePath(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeMove})
	e.settings(SettingsInput{MinVideoMB: 1})
	in := filepath.Join(e.root, "incoming")
	require.NoError(t, os.MkdirAll(in, 0o755))
	d, err := e.svc.SaveWatchDir(e.ctx, 0, WatchDirInput{Path: in, Enabled: true})
	require.NoError(t, err)
	name := "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv"
	src := e.put(d, name, 5, 10*time.Minute)
	e.svc.watchScan(e.ctx, e.mustSettings())
	require.Equal(t, 1, e.drain())
	require.False(t, exists(src), "移动进库")
	old := e.history()[0]
	require.Equal(t, models.MediaTransferDone, old.Status)

	e.put(d, name, 6, 10*time.Minute)
	e.svc.forgetWatch(watchHash(d.ID, name))
	e.svc.watchScan(e.ctx, e.mustSettings())
	require.Equal(t, 1, e.drain(), "大小不一样：是另一个文件")
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferDone, rows[0].Status, "那条记录还是原来的")
	assert.Equal(t, old.TargetPath, rows[0].TargetPath)
	assert.True(t, strings.HasPrefix(rows[0].Message, otherFileNote), rows[0].Message)
	assert.True(t, exists(src), "新文件留着")
	p, err := e.svc.Preview(e.ctx, Request{WatchDirID: d.ID, Entry: name})
	require.NoError(t, err)
	assert.Equal(t, ItemExists, p.Items[0].Status)
	assert.Contains(t, p.Items[0].Message, "先删除这条记录")

	e.svc.forgetWatch(watchHash(d.ID, name))
	e.svc.watchScan(e.ctx, e.mustSettings())
	assert.Zero(t, e.drain(), "写过说明：不再排")

	// 移动整理的记录只能删记录（库里那份是唯一的一份）；用户把库里的旧文件也删了，新文件照常整理进去
	_, err = e.svc.DeleteHistory(e.ctx, old.ID, false)
	require.NoError(t, err)
	require.NoError(t, os.Remove(old.TargetPath))
	e.svc.forgetWatch(watchHash(d.ID, name))
	e.svc.watchScan(e.ctx, e.mustSettings())
	require.Equal(t, 1, e.drain())
	rows = e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferDone, rows[0].Status)
	assert.Equal(t, int64(6)<<20, rows[0].Size, "这回整理的是新文件")
	assert.False(t, exists(src))

	// 记录不管库里的文件（例如识别失败）：换成的新文件直接整理
	bad := e.put(d, "Oppenheimer.2023.1080p.WEB-DL.H264.mkv", 3, 10*time.Minute)
	require.NoError(t, e.db.Create(&models.MediaTransferHistory{
		WatchDirID: d.ID, DownloaderName: watchSource, SourcePath: bad, Size: 1, Status: models.MediaTransferFailed, Message: "识别失败",
	}).Error)
	res, err := e.svc.Organize(e.ctx, Request{WatchDirID: d.ID, Entry: filepath.Base(bad)})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.False(t, exists(bad))
}
