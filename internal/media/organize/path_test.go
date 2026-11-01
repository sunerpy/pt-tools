package organize

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// putIncoming 在临时目录的 incoming 下放一个文件（大小 MB），文件与它上面的各级目录的时间设成 age 以前
// （e.now 是固定的假时间，不设的话会被当成最近 2 分钟里还有改动）。
func (e *env) putIncoming(rel string, mb int, age time.Duration) string {
	e.t.Helper()
	return e.put(models.MediaWatchDir{Path: filepath.Join(e.root, "incoming")}, rel, mb, age)
}

// pathRows 是指定路径 p 整理出的记录（按 InfoHash 认）。
func (e *env) pathRows(p string) []models.MediaTransferHistory {
	e.t.Helper()
	var rows []models.MediaTransferHistory
	require.NoError(e.t, e.db.Where("info_hash = ?", pathHash(p)).Order("id").Find(&rows).Error)
	return rows
}

func fileSHA256(t *testing.T, p string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return sha256.Sum256(b)
}

// 源路径的检查：绝对路径、规整、不是根目录，不含 NUL，整个不超过 1024 字节、最后一段不超过 512 字节
func TestCleanSource(t *testing.T) {
	for _, bad := range []string{
		"", "  ", "relative/x", "/", "/a\x00b",
		"/" + strings.Repeat("a", 1100),
		"/x/" + strings.Repeat("b", 600),
	} {
		_, err := cleanSource(bad)
		require.ErrorIs(t, err, ErrInvalid, "%.40q", bad)
	}
	if runtime.GOOS != "windows" {
		for _, bad := range []string{`C:\Downloads\x`, `\\nas\share\x`} {
			_, err := cleanSource(bad)
			require.ErrorIs(t, err, ErrInvalid, "%q 在这个系统上不是绝对路径", bad)
		}
	}
	for in, want := range map[string]string{
		"/a/b/":         "/a/b",
		"/a/../b":       "/b",
		" /data/x.mkv ": "/data/x.mkv",
		"/a//b/./c":     "/a/b/c",
	} {
		got, err := cleanSource(in)
		require.NoError(t, err, in)
		assert.Equal(t, filepath.FromSlash(want), got, in)
	}
	_, err := cleanSource("/x/" + strings.Repeat("b", 512))
	require.NoError(t, err, "最后一段正好 512 字节")
}

// 指定路径的请求：只能有一种来源；路径要在、本身不能是软链接；不能是媒体库的目录、在库里或包含库；这些在排队以前就报错
func TestPathRequestValidation(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	src := filepath.Dir(e.putIncoming("Oppenheimer.2023.2160p.BluRay.x265-FRDS/Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 10*time.Minute))
	for _, req := range []Request{
		{SourcePath: src, DownloaderID: 1, Hash: oppHash},
		{SourcePath: src, Hash: oppHash},
		{SourcePath: src, WatchDirID: 1, Entry: "x"},
		{SourcePath: src, Entry: "x"},
		{SourcePath: src, TMDBID: 872585},
	} {
		_, err := e.svc.Preview(e.ctx, req)
		require.ErrorIs(t, err, ErrInvalid, "%+v", req)
		_, err = e.svc.Organize(e.ctx, req)
		require.ErrorIs(t, err, ErrInvalid, "%+v", req)
	}
	_, err := e.svc.Preview(e.ctx, Request{SourcePath: src, DownloaderID: 1})
	require.ErrorContains(t, err, "整理的来源只能选一种")

	_, err = e.svc.Preview(e.ctx, Request{SourcePath: filepath.Join(e.root, "incoming", "nope")})
	require.ErrorIs(t, err, ErrInvalid)
	require.ErrorContains(t, err, "找不到")

	link := filepath.Join(e.root, "incoming", "link")
	if os.Symlink(src, link) == nil {
		to, lerr := filepath.EvalSymlinks(link)
		require.NoError(t, lerr)
		_, err = e.svc.Preview(e.ctx, Request{SourcePath: link})
		require.ErrorIs(t, err, ErrInvalid)
		require.ErrorContains(t, err, "软链接")
		require.ErrorContains(t, err, to, "写明指向哪里")
	}

	inside := filepath.Join(e.movies, "sub")
	require.NoError(t, os.MkdirAll(inside, 0o755))
	for _, bad := range []string{e.movies, inside, filepath.Join(e.root, "media")} {
		_, err = e.svc.Preview(e.ctx, Request{SourcePath: bad})
		require.ErrorIs(t, err, ErrInvalid, bad)
		require.ErrorContains(t, err, "是媒体库「", bad)
		_, err = e.svc.Organize(e.ctx, Request{SourcePath: bad})
		require.ErrorIs(t, err, ErrInvalid, bad)
	}
	assert.Empty(t, e.svc.jobs, "都没排进队列")
	assert.Empty(t, e.history(), "都没写记录")
}

// 指定一个电影文件夹：自动识别、硬链接进电影库、带上字幕、刮削、通知媒体服务器、发入库通知；记录写明是指定路径，
// 重试按记录还原出这个路径；再整理一次是「已在库里」
func TestPathOrganizeMovieFolder(t *testing.T) {
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
	name := "Oppenheimer.2023.2160p.BluRay.x265-FRDS"
	video := e.putIncoming(name+"/"+name+".mkv", 5, 10*time.Minute)
	e.putIncoming(name+"/"+name+".chs.srt", 0, 10*time.Minute)
	dir := filepath.Dir(video)

	p, err := e.svc.Preview(e.ctx, Request{SourcePath: dir + string(filepath.Separator)})
	require.NoError(t, err)
	require.Empty(t, p.Problem)
	assert.Equal(t, dir, p.SourcePath, "规整后的路径")
	assert.Equal(t, pathSource, p.DownloaderName)
	assert.Zero(t, p.DownloaderID)
	assert.Equal(t, pathHash(dir), p.Hash)
	assert.Empty(t, p.TaskID)
	assert.Equal(t, name, p.Name)
	assert.Equal(t, filepath.Dir(dir), p.SavePath)
	assert.Equal(t, filepath.Dir(dir), p.LocalPath)
	assert.False(t, p.Mapped)
	assert.Empty(t, p.Notes, "下载器里没有这个种子：没有说明")
	require.Len(t, p.Items, 1)
	want := filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023) - 2160p BluRay H.265.mkv")
	assert.Equal(t, want, p.Items[0].Target)
	require.Len(t, p.Items[0].Subtitles, 1)
	assert.False(t, exists(want), "预览不改动文件")

	res, err := e.svc.Organize(e.ctx, Request{SourcePath: dir})
	require.NoError(t, err)
	require.Empty(t, res.Plan.Problem)
	assert.Equal(t, 1, res.Created)
	assert.True(t, sameFile(t, video, want), "硬链接进电影库")
	assert.True(t, exists(strings.TrimSuffix(want, ".mkv")+".zh-CN.srt"), "字幕跟着整理")
	assert.Contains(t, readFile(t, strings.TrimSuffix(want, ".mkv")+".nfo"), `<uniqueid type="tmdb" default="true">872585</uniqueid>`)
	assert.True(t, exists(filepath.Join(filepath.Dir(want), "poster.jpg")))
	paths := emby.got()
	require.Len(t, paths, 1)
	assert.Contains(t, paths[0], `"Path":"/data/media/movies/奥本海默 (2023)"`, "按媒体服务器的路径映射通知")
	notices := e.gotNotices()
	require.Len(t, notices, 1)
	assert.Equal(t, "入库：奥本海默 (2023)", notices[0].Title)
	assert.Contains(t, notices[0].Text, "手动整理："+name)
	assert.NotContains(t, notices[0].Text, "种子：")

	rows := e.pathRows(dir)
	require.Len(t, rows, 1)
	r := rows[0]
	assert.Equal(t, models.MediaTransferDone, r.Status)
	assert.Zero(t, r.DownloaderID)
	assert.Zero(t, r.WatchDirID)
	assert.Equal(t, pathSource, r.DownloaderName)
	assert.Empty(t, r.TaskID)
	assert.Equal(t, name, r.TorrentName)
	assert.Equal(t, filepath.Join(e.root, "incoming"), r.SaveRoot)
	assert.Equal(t, video, r.SourcePath)
	assert.Equal(t, models.MediaTriggerManual, r.Trigger)
	assert.NotEmpty(t, r.SourceFileID, "记下源文件的编号")
	assert.True(t, pathRow(r))
	assert.Equal(t, Request{SourcePath: dir}, requestOf(r))

	// 整理历史：指定路径的记录带上当时填的路径，按种子整理的没有
	e.lastOfUs()
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	page, err := e.svc.History(e.ctx, HistoryQuery{})
	require.NoError(t, err)
	require.Len(t, page.Items, 3)
	for _, it := range page.Items {
		if it.InfoHash == pathHash(dir) {
			assert.Equal(t, dir, it.ManualPath)
		} else {
			assert.Empty(t, it.ManualPath, it.SourcePath)
			assert.False(t, pathRow(it.MediaTransferHistory))
		}
	}

	res, err = e.svc.Organize(e.ctx, Request{SourcePath: dir})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Done, "已经在库里")
	assert.Zero(t, res.Created)
	assert.Len(t, e.pathRows(dir), 1)
	assert.Len(t, e.gotNotices(), 2, "再整理一次不重复通知（第二条是剧集的）")
}

// 指定单个视频文件：记录的种子名是文件名，上级目录是保存目录，重试还原出这个文件
func TestPathOrganizeSingleFile(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	name := "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv"
	src := e.putIncoming(name, 5, 10*time.Minute)

	res, err := e.svc.Organize(e.ctx, Request{SourcePath: src})
	require.NoError(t, err)
	require.Empty(t, res.Plan.Problem)
	assert.Equal(t, 1, res.Created)
	want := filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023) - 2160p BluRay H.265.mkv")
	assert.True(t, sameFile(t, src, want))
	rows := e.pathRows(src)
	require.Len(t, rows, 1)
	assert.Equal(t, name, rows[0].TorrentName)
	assert.Equal(t, filepath.Join(e.root, "incoming"), rows[0].SaveRoot)
	assert.Equal(t, src, requestOf(rows[0]).SourcePath)
}

// 指定一季剧集的文件夹：两集都进剧集库，季集与集标题对
func TestPathOrganizeSeasonFolder(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	name := "The.Last.of.Us.S01.2160p.WEB-DL.DV.H.265-HHWEB"
	e.putIncoming(name+"/The.Last.of.Us.S01E01.2160p.WEB-DL.DV.H.265-HHWEB.mkv", 2, 10*time.Minute)
	e.putIncoming(name+"/The.Last.of.Us.S01E02.2160p.WEB-DL.DV.H.265-HHWEB.mkv", 2, 10*time.Minute)
	dir := filepath.Join(e.root, "incoming", name)

	res, err := e.svc.Organize(e.ctx, Request{SourcePath: dir})
	require.NoError(t, err)
	require.Empty(t, res.Plan.Problem)
	assert.Equal(t, 2, res.Created)
	show := filepath.Join(e.tv, "最后生还者 (2023)", "Season 1")
	assert.True(t, exists(filepath.Join(show, "最后生还者 - S01E01 - 当你迷失在黑暗中.mkv")))
	assert.True(t, exists(filepath.Join(show, "最后生还者 - S01E02 - 感染.mkv")))
	assert.Len(t, e.pathRows(dir), 2, "每个视频一条记录")
}

// 名字认不出来时手填条目：类型与 TMDB 编号；记录记下这个条目
func TestPathManualEntry(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	src := filepath.Dir(e.putIncoming("my-download-01/video.mkv", 5, 10*time.Minute))

	p, err := e.svc.Preview(e.ctx, Request{SourcePath: src})
	require.NoError(t, err)
	assert.Contains(t, p.Problem, "没有识别出来")

	req := Request{SourcePath: src, MediaType: models.MediaKindMovie, TMDBID: 872585}
	p, err = e.svc.Preview(e.ctx, req)
	require.NoError(t, err)
	require.Empty(t, p.Problem)
	require.NotNil(t, p.Match)
	assert.Equal(t, 872585, p.Match.ID)
	assert.Equal(t, sourceManual, p.Source)
	assert.Equal(t, "电影", p.Library.Name)

	res, err := e.svc.Organize(e.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	rows := e.pathRows(src)
	require.Len(t, rows, 1)
	assert.Equal(t, 872585, rows[0].TMDBID)
	assert.Equal(t, models.MediaKindMovie, rows[0].MediaType)
}

// 识别失败（TMDB 暂时不能访问）的记录按退避自动重试：到期以后按记录还原出路径重新整理，还是同一条记录
func TestPathRetryDue(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	src := e.putIncoming("Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 10*time.Minute)
	e.tmdb.setDown(true)
	res, err := e.svc.Organize(e.ctx, Request{SourcePath: src})
	require.NoError(t, err)
	require.Contains(t, res.Plan.Problem, "识别失败")
	rows := e.pathRows(src)
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferFailed, rows[0].Status)
	require.NotNil(t, rows[0].NextRetryAt, "暂时的问题：过一会儿自动重试")

	e.tmdb.setDown(false)
	e.advance(11 * time.Minute)
	e.svc.retryDue(e.ctx, e.currentSettings())
	require.Equal(t, 1, e.drain())
	rows = e.pathRows(src)
	require.Len(t, rows, 1, "还是同一条记录")
	assert.Equal(t, models.MediaTransferDone, rows[0].Status)
	assert.Equal(t, pathHash(src), rows[0].InfoHash)
	assert.Equal(t, models.MediaTriggerManual, rows[0].Trigger)
	assert.Nil(t, rows[0].NextRetryAt)
	assert.Len(t, e.history(), 1)
}

// failedPathRow 插一条手动指定路径整理失败、手填过条目的记录（视频 video，指定的路径是它所在的目录）。
func (e *env) failedPathRow(video string, next *time.Time) models.MediaTransferHistory {
	e.t.Helper()
	dir := filepath.Dir(video)
	var lib models.MediaLibrary
	require.NoError(e.t, e.db.Where("kind = ?", models.MediaKindMovie).First(&lib).Error)
	row := models.MediaTransferHistory{
		DownloaderName: pathSource, InfoHash: pathHash(dir), TorrentName: filepath.Base(dir), SourcePath: video, SaveRoot: filepath.Dir(dir),
		MediaType: models.MediaKindMovie, TMDBID: 872585, LibraryID: lib.ID, Status: models.MediaTransferFailed, Message: "上次没整理成",
		Attempts: 1, NextRetryAt: next, Trigger: models.MediaTriggerManual, Size: 5 << 20,
	}
	require.NoError(e.t, e.db.Create(&row).Error)
	return row
}

// 手填过条目的失败记录：到期重试与「重试」都沿用记录里的条目与媒体库（名字本身认不出来）
func TestPathRetryUsesRecordedEntry(t *testing.T) {
	for _, way := range []string{"到期重试", "重试"} {
		t.Run(way, func(t *testing.T) {
			e := newEnv(t)
			e.defaultLibraries()
			e.settings(SettingsInput{MinVideoMB: 1})
			video := e.putIncoming("my-download-01/video.mkv", 5, 10*time.Minute)
			past := e.Now().Add(-time.Minute)
			row := e.failedPathRow(video, &past)
			if way == "到期重试" {
				e.svc.retryDue(e.ctx, e.currentSettings())
				require.Equal(t, 1, e.drain())
			} else {
				res, err := e.svc.Retry(e.ctx, row.ID)
				require.NoError(t, err)
				require.Empty(t, res.Plan.Problem)
				assert.Equal(t, 1, res.Created)
			}
			got := e.row(row.ID)
			assert.Equal(t, models.MediaTransferDone, got.Status)
			assert.Equal(t, "奥本海默", got.Title)
			assert.True(t, transfer.Within(e.movies, got.TargetPath), got.TargetPath)
			assert.True(t, sameFile(t, video, got.TargetPath))
		})
	}
}

// 源路径不在了：到期重试不再自动重试（记录留着），「重试」写明找不到
func TestPathRetryStopsWhenSourceGone(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	video := e.putIncoming("my-download-01/video.mkv", 5, 10*time.Minute)
	past := e.Now().Add(-time.Minute)
	row := e.failedPathRow(video, &past)
	require.NoError(t, os.RemoveAll(filepath.Dir(video)))

	e.svc.retryDue(e.ctx, e.currentSettings())
	require.Equal(t, 1, e.drain())
	got := e.row(row.ID)
	assert.Equal(t, models.MediaTransferFailed, got.Status, "记录留着")
	assert.Nil(t, got.NextRetryAt, "不再自动重试")
	e.advance(time.Hour)
	e.svc.retryDue(e.ctx, e.currentSettings())
	assert.Zero(t, e.drain())

	_, err := e.svc.Retry(e.ctx, row.ID)
	require.ErrorIs(t, err, ErrInvalid)
	require.ErrorContains(t, err, "找不到")
}

// 整理过的位置上换成了另一个文件：记录管着库里的文件，不整理，写明原因（不说「已在库里」），库里的文件不动
func TestPathOtherFileAtSamePath(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	src := e.putIncoming("Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 10*time.Minute)
	res, err := e.svc.Organize(e.ctx, Request{SourcePath: src})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	target := e.pathRows(src)[0].TargetPath

	require.NoError(t, os.Remove(src))
	e.putIncoming(filepath.Base(src), 6, 10*time.Minute)
	p, err := e.svc.Preview(e.ctx, Request{SourcePath: src})
	require.NoError(t, err)
	require.Len(t, p.Items, 1)
	assert.Equal(t, ItemExists, p.Items[0].Status)
	assert.True(t, strings.HasPrefix(p.Items[0].Message, pathOtherFileNote), p.Items[0].Message)
	assert.Contains(t, p.Items[0].Message, "先删除这条记录")

	res, err = e.svc.Organize(e.ctx, Request{SourcePath: src})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped)
	assert.Zero(t, res.Created)
	rows := e.pathRows(src)
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferDone, rows[0].Status, "那条记录还是原来的")
	assert.True(t, strings.HasPrefix(rows[0].Message, pathOtherFileNote), rows[0].Message)
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, int64(5<<20), info.Size(), "库里还是原来那个文件")
}

// 目标位置被别人的文件占着：跳过，不覆盖（内容不变）
func TestPathTargetOccupied(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	src := e.putIncoming("Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 10*time.Minute)
	target := filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023) - 2160p BluRay H.265.mkv")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte("别人的文件"), 0o644))
	before := fileSHA256(t, target)

	res, err := e.svc.Organize(e.ctx, Request{SourcePath: src})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped)
	assert.Zero(t, res.Created)
	rows := e.pathRows(src)
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferSkipped, rows[0].Status)
	assert.Contains(t, rows[0].Message, "目标位置已经有同名文件")
	assert.Equal(t, before, fileSHA256(t, target), "别人的文件不动")
}

// 指定的路径就是下载器里一个种子的内容：改成按种子整理（记录记在种子下，定期扫描认得）；没下载完的种子不整理
func TestPathTorrentContentBecomesTorrentOrganize(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	content := filepath.Join(e.dlDir, oppName)

	p, err := e.svc.Preview(e.ctx, Request{SourcePath: content})
	require.NoError(t, err)
	require.Empty(t, p.Problem)
	assert.Equal(t, uint(1), p.DownloaderID)
	assert.Equal(t, oppHash, p.Hash)
	assert.Equal(t, "qb", p.DownloaderName)
	assert.Empty(t, p.SourcePath)
	require.NotEmpty(t, p.Notes)
	assert.Contains(t, p.Notes[0], "按种子整理")
	assert.Contains(t, p.Notes[0], "下载器「qb」里的种子")

	res, err := e.svc.Organize(e.ctx, Request{SourcePath: content})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, uint(1), rows[0].DownloaderID)
	assert.Equal(t, oppHash, rows[0].InfoHash)
	assert.Equal(t, "qb", rows[0].DownloaderName)
	assert.False(t, pathRow(rows[0]))
	assert.Empty(t, e.pathRows(content))

	e.settings(SettingsInput{MinVideoMB: 1, ScanEnabled: true})
	e.svc.scan(e.ctx, e.currentSettings())
	assert.Zero(t, e.drain(), "定期扫描认得这个种子整理过了")

	half := "The.Last.of.Us.S01.2160p.WEB-DL.DV.H.265-HHWEB"
	e.addTorrent(tlouHash, half, map[string]int{half + "/The.Last.of.Us.S01E01.2160p.WEB-DL.DV.H.265-HHWEB.mkv": 5}, func(tr *downloader.Torrent) {
		tr.Progress = 0.5
	})
	res, err = e.svc.Organize(e.ctx, Request{SourcePath: filepath.Join(e.dlDir, half)})
	require.NoError(t, err)
	assert.Equal(t, "种子还没下载完", res.Plan.Problem)
	assert.Equal(t, int64(0), e.historyCount(tlouHash), "不写记录")
	assert.Len(t, e.history(), 1)
}

// 指定的路径和下载器里的种子部分重叠（包含种子，或在种子里）：不整理，说明原因，不写记录
func TestPathOverlapsTorrentRefused(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	for _, src := range []string{e.dlDir, filepath.Join(e.dlDir, oppName, oppName+".mkv")} {
		p, err := e.svc.Preview(e.ctx, Request{SourcePath: src})
		require.NoError(t, err, src)
		assert.Contains(t, p.Problem, "重叠", src)
		assert.Contains(t, p.Problem, "下载器「qb」里的种子", src)
		res, err := e.svc.Organize(e.ctx, Request{SourcePath: src})
		require.NoError(t, err, src)
		assert.Contains(t, res.Plan.Problem, "重叠", src)
	}
	assert.Empty(t, e.history(), "不写记录")
}

// 媒体库用移动整理：同名的种子（pt-tools 看不到它的文件）不移动；确认不了是不是种子时过一阵再试；
// 用硬链接时照常整理，写明没能确认
func TestPathMoveChecksDownloaders(t *testing.T) {
	e := newEnv(t)
	lib := e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeMove})
	e.settings(SettingsInput{MinVideoMB: 1})
	name := "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv"
	src := e.putIncoming(name, 5, 10*time.Minute)
	e.addTorrent("h1", name, map[string]int{name: 5}, func(tr *downloader.Torrent) {
		tr.SavePath, tr.ContentPath = `D:\Downloads`, `D:\Downloads\`+name
	})

	res, err := e.svc.Organize(e.ctx, Request{SourcePath: src})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "同名")
	assert.Contains(t, res.Plan.Problem, "qb：")
	assert.True(t, exists(src), "源文件不动")
	rows := e.pathRows(src)
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferFailed, rows[0].Status)
	assert.Nil(t, rows[0].NextRetryAt, "改了设置再手动重试")

	e.dl.mu.Lock()
	e.dl.listErr = errors.New("连接被拒绝")
	e.dl.mu.Unlock()
	res, err = e.svc.Organize(e.ctx, Request{SourcePath: src})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "移动整理以前要确认")
	assert.Contains(t, res.Plan.Problem, "连接被拒绝")
	assert.True(t, exists(src))
	rows = e.pathRows(src)
	require.Len(t, rows, 1)
	assert.NotNil(t, rows[0].NextRetryAt, "确认不了的过一阵再试")

	_, err = e.svc.SaveLibrary(e.ctx, lib.ID, LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeHardlink, Enabled: true})
	require.NoError(t, err)
	res, err = e.svc.Organize(e.ctx, Request{SourcePath: src})
	require.NoError(t, err)
	require.Empty(t, res.Plan.Problem)
	assert.Equal(t, 1, res.Created)
	require.NotEmpty(t, res.Plan.Notes)
	assert.Contains(t, strings.Join(res.Plan.Notes, "\n"), "没能确认")
	assert.True(t, exists(src), "硬链接：源文件留着")
}

// 最近 2 分钟里还有改动的、目录太深的不整理，也不写记录
func TestPathSettleAndTooBig(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	fresh := e.putIncoming("Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, time.Minute)
	res, err := e.svc.Organize(e.ctx, Request{SourcePath: fresh})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "最近 2 分钟")

	e.putIncoming("Deep/a/b/c/d/e/f/The.Last.of.Us.S01E01.2160p.WEB-DL.mkv", 2, 10*time.Minute)
	res, err = e.svc.Organize(e.ctx, Request{SourcePath: filepath.Join(e.root, "incoming", "Deep")})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "超过 6 层")
	assert.Empty(t, e.history(), "都不写记录")
}
