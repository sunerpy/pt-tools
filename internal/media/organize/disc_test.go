package organize

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

const (
	discHash = "cccccccccccccccccccccccccccccccccccccccc"
	discName = "Oppenheimer.2023.1080p.BluRay.AVC.DTS-HD.MA.5.1-GRP"
)

// discFiles 是一张蓝光原盘（大小 MB）：结构目录里的五个文件，外加根目录里的 nfo 与样片（不放进库）。
func discFiles(root string) map[string]int {
	return map[string]int{
		root + "/BDMV/index.bdmv":          0,
		root + "/BDMV/PLAYLIST/00000.mpls": 0,
		root + "/BDMV/STREAM/00001.m2ts":   5,
		root + "/BDMV/STREAM/00002.m2ts":   1,
		root + "/CERTIFICATE/id.bdmv":      0,
		root + "/" + root + ".nfo":         0,
		root + "/sample.mkv":               2,
	}
}

// discIns 是 discFiles 里放进库的文件（相对原盘根目录）。
var discIns = []string{"BDMV/index.bdmv", "BDMV/PLAYLIST/00000.mpls", "BDMV/STREAM/00001.m2ts", "BDMV/STREAM/00002.m2ts", "CERTIFICATE/id.bdmv"}

func (e *env) discTorrent(mod func(*downloader.Torrent)) downloader.Torrent {
	return e.addTorrent(discHash, discName, discFiles(discName), mod)
}

func discExtrasOf(t *testing.T, r models.MediaTransferHistory) (disc []extra, meta []string) {
	t.Helper()
	for _, x := range decodeExtras(r.Extras) {
		switch x.Kind {
		case extraDisc:
			disc = append(disc, x)
		case extraMeta:
			meta = append(meta, filepath.Base(x.Target))
		}
	}
	return disc, meta
}

func TestOrganizeDiscMovie(t *testing.T) {
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
	e.discTorrent(nil)
	root := filepath.Join(e.dlDir, discName)
	movieDir := filepath.Join(e.movies, "奥本海默 (2023)")

	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	require.Empty(t, p.Problem)
	require.Len(t, p.Items, 1, "一张原盘一项")
	it := p.Items[0]
	assert.Equal(t, discName, it.Rel)
	assert.Equal(t, root, it.Source)
	assert.Equal(t, movieDir, it.Target, "原盘整个放进模板算出的电影目录")
	assert.Equal(t, ItemPending, it.Status)
	require.NotNil(t, it.Disc)
	assert.Equal(t, DiscInfo{Kind: "BDMV", Files: 5}, *it.Disc)
	assert.Equal(t, int64(6)<<20, it.Size, "原盘文件的大小合计")
	require.Len(t, p.Skipped, 2, "根目录里的 nfo 与样片不放进库")
	assert.False(t, exists(movieDir), "预览不改动文件")

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Empty(t, res.Messages)
	for _, in := range discIns {
		assert.True(t, sameFile(t, filepath.Join(root, filepath.FromSlash(in)), filepath.Join(movieDir, filepath.FromSlash(in))), "硬链接 %s", in)
	}
	assert.False(t, exists(filepath.Join(movieDir, "sample.mkv")))
	assert.False(t, exists(filepath.Join(movieDir, discName+".nfo")))
	assert.Contains(t, readFile(t, filepath.Join(movieDir, "movie.nfo")), `<uniqueid type="tmdb" default="true">872585</uniqueid>`, "原盘写 movie.nfo（Jellyfin 的 MovieNfoSaver）")
	assert.Equal(t, "JPEG/t/p/w780/opp.jpg", readFile(t, filepath.Join(movieDir, "poster.jpg")))
	assert.True(t, exists(filepath.Join(movieDir, "fanart.jpg")))
	got := emby.got()
	require.Len(t, got, 1)
	assert.Contains(t, got[0], `/data/media/movies/奥本海默 (2023)"`, "媒体服务器扫电影目录")

	rows := e.history()
	require.Len(t, rows, 1)
	r := rows[0]
	assert.Equal(t, models.MediaTransferDone, r.Status)
	assert.Equal(t, root, r.SourcePath)
	assert.Equal(t, movieDir, r.TargetPath)
	assert.Equal(t, int64(6)<<20, r.Size)
	assert.Equal(t, models.MediaModeHardlink, r.Mode)
	mainID, err := transfer.FileID(filepath.Join(movieDir, "BDMV", "STREAM", "00001.m2ts"))
	require.NoError(t, err)
	assert.Equal(t, mainID, r.TargetFileID, "记录的文件编号是主文件的")
	disc, meta := discExtrasOf(t, r)
	require.Len(t, disc, 5)
	mains := 0
	for _, x := range disc {
		assert.NotEmpty(t, x.FileID)
		assert.True(t, transfer.Within(movieDir, x.Target))
		assert.True(t, transfer.Within(root, x.Source))
		if x.Main {
			mains++
			assert.Equal(t, filepath.Join(movieDir, "BDMV", "STREAM", "00001.m2ts"), x.Target)
		}
	}
	assert.Equal(t, 1, mains)
	assert.ElementsMatch(t, []string{"movie.nfo", "poster.jpg", "fanart.jpg"}, meta)
	assert.True(t, ours(&r))
	require.Len(t, e.gotNotices(), 1)
	assert.Equal(t, int64(5), e.gotNotices()[0].Event.Fields["files"], "入库的文件数是原盘里放进库的文件数")

	// 再整理：库里已经是这张原盘
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Created)
	assert.Equal(t, 1, res.Done)
	p, err = e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	assert.Equal(t, ItemDone, p.Items[0].Status)

	// 库里的主文件换掉了：不再认，不覆盖
	main := filepath.Join(movieDir, "BDMV", "STREAM", "00001.m2ts")
	require.NoError(t, os.Remove(main))
	require.NoError(t, os.WriteFile(main, []byte("someone else"), 0o644))
	r = e.history()[0]
	assert.False(t, ours(&r))
	p, err = e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	assert.Equal(t, ItemExists, p.Items[0].Status)
	assert.Contains(t, p.Items[0].Message, "原盘")
}

func TestOrganizeDiscModes(t *testing.T) {
	for _, mode := range []string{models.MediaModeCopy, models.MediaModeSymlink, models.MediaModeMove} {
		t.Run(mode, func(t *testing.T) {
			e := newEnv(t)
			e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: mode})
			e.settings(SettingsInput{MinVideoMB: 1})
			e.discTorrent(func(t *downloader.Torrent) { t.State = downloader.TorrentPaused })
			root := filepath.Join(e.dlDir, discName)
			movieDir := filepath.Join(e.movies, "奥本海默 (2023)")
			srcInfo := map[string]os.FileInfo{}
			for _, in := range discIns {
				fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(in)))
				require.NoError(t, err)
				srcInfo[in] = fi
			}

			res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
			require.NoError(t, err)
			require.Equal(t, 1, res.Created, res.Plan.Problem)
			for _, in := range discIns {
				src, dst := filepath.Join(root, filepath.FromSlash(in)), filepath.Join(movieDir, filepath.FromSlash(in))
				switch mode {
				case models.MediaModeCopy:
					assert.False(t, sameFile(t, src, dst), "复制出的是另一个文件")
					assert.Equal(t, readFile(t, src), readFile(t, dst))
				case models.MediaModeSymlink:
					to, err := os.Readlink(dst)
					require.NoError(t, err)
					assert.Equal(t, src, to)
				case models.MediaModeMove:
					assert.False(t, exists(src), "移走了")
					dstInfo, err := os.Stat(dst)
					require.NoError(t, err)
					assert.True(t, os.SameFile(srcInfo[in], dstInfo), "同一个文件挪了位置")
				}
			}
			r := e.history()[0]
			assert.Equal(t, mode, r.Mode)
			assert.True(t, ours(&r))
			if mode == models.MediaModeMove {
				assert.False(t, exists(filepath.Join(root, "BDMV")), "移空了的原盘目录删掉")
				assert.False(t, exists(filepath.Join(root, "CERTIFICATE")))
				assert.True(t, exists(filepath.Join(root, "sample.mkv")), "原盘以外的文件留在原处")
			}
		})
	}
}

func TestOrganizeDiscProblems(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})

	// 剧集原盘：跳过，不重试
	tvName := "The.Last.of.Us.S01.1080p.BluRay.AVC-GRP"
	e.addTorrent("d1", tvName, discFiles(tvName), func(t *downloader.Torrent) { t.Category = "tv" })
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "d1"})
	require.NoError(t, err)
	assert.Equal(t, "剧集原盘暂不整理", res.Plan.Problem)
	assert.Equal(t, 1, res.Skipped)
	var row models.MediaTransferHistory
	require.NoError(t, e.db.Where("info_hash = ?", "d1").First(&row).Error)
	assert.Equal(t, models.MediaTransferSkipped, row.Status)
	assert.Equal(t, filepath.Join(e.dlDir, tvName), row.SourcePath, "记在原盘根目录上")
	assert.Nil(t, row.NextRetryAt)

	// 种子里有两张原盘、没有视频流、BDMV 直接在保存目录里：识别以前就跳过，记在内容路径上
	for _, tc := range []struct {
		hash, name, want string
		files            map[string]int
	}{
		{"d2", "Box.2020", "2 张原盘", map[string]int{"Box.2020/D1/BDMV/STREAM/1.m2ts": 1, "Box.2020/D2/BDMV/STREAM/1.m2ts": 1}},
		{"d3", "Empty.2020", "没有视频流", map[string]int{"Empty.2020/BDMV/index.bdmv": 0}},
		{"d4", "BDMV", "没有自己的文件夹", map[string]int{"BDMV/STREAM/00001.m2ts": 1}},
	} {
		e.addTorrent(tc.hash, tc.name, tc.files, nil)
		res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tc.hash})
		require.NoError(t, err)
		assert.Contains(t, res.Plan.Problem, tc.want, tc.name)
		var r models.MediaTransferHistory
		require.NoError(t, e.db.Where("info_hash = ?", tc.hash).First(&r).Error)
		assert.Equal(t, models.MediaTransferSkipped, r.Status, tc.name)
		assert.Equal(t, filepath.Join(e.dlDir, tc.name), r.SourcePath, tc.name)
	}

	// 电影目录里已经有文件（另一个版本）：不和它放在一起
	movieDir := filepath.Join(e.movies, "奥本海默 (2023)")
	require.NoError(t, os.MkdirAll(movieDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(movieDir, "奥本海默 (2023) - 2160p.mkv"), []byte("other"), 0o644))
	e.discTorrent(nil)
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped)
	assert.Contains(t, res.Plan.Items[0].Message, "已经有文件")
	assert.False(t, exists(filepath.Join(movieDir, "BDMV")))
	var skipped models.MediaTransferHistory
	require.NoError(t, e.db.Where("info_hash = ?", discHash).First(&skipped).Error)
	assert.Equal(t, models.MediaTransferSkipped, skipped.Status)

	// 空的电影目录照常放
	require.NoError(t, os.Remove(filepath.Join(movieDir, "奥本海默 (2023) - 2160p.mkv")))
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
}

// 电影的命名模板没有目录：原盘没地方放，记失败、不重试
func TestOrganizeDiscNeedsMovieDir(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Template: "{{.Title}} ({{.Year}})"})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.discTorrent(nil)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
	assert.Contains(t, res.Plan.Items[0].Message, "原盘要放在单独的目录里")
	r := e.history()[0]
	assert.Equal(t, models.MediaTransferFailed, r.Status)
	assert.Nil(t, r.NextRetryAt)
	entries, err := os.ReadDir(e.movies)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// 记录删掉了（库里的文件留着）以后再整理：库里每个文件都是源文件的硬链接，认回来、补一条记录
func TestOrganizeDiscAdopts(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.discTorrent(nil)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	_, err = e.svc.DeleteHistory(e.ctx, e.history()[0].ID, false)
	require.NoError(t, err)
	require.Empty(t, e.history())

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Done)
	assert.Equal(t, 0, res.Created)
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferDone, rows[0].Status)
	disc, _ := discExtrasOf(t, rows[0])
	assert.Len(t, disc, 5)
	assert.NotEmpty(t, rows[0].TargetFileID)
	assert.True(t, ours(&rows[0]))
}

// 放到一半失败：撤回这次放进去的文件；移动时挪回原处
func TestOrganizeDiscRollback(t *testing.T) {
	for _, mode := range []string{models.MediaModeHardlink, models.MediaModeMove} {
		t.Run(mode, func(t *testing.T) {
			e := newEnv(t)
			e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: mode})
			e.settings(SettingsInput{MinVideoMB: 1})
			e.discTorrent(func(t *downloader.Torrent) { t.State = downloader.TorrentPaused })
			root := filepath.Join(e.dlDir, discName)
			movieDir := filepath.Join(e.movies, "奥本海默 (2023)")
			old := transferDetail
			t.Cleanup(func() { transferDetail = old })
			calls := 0
			transferDetail = func(src, dst, m string) (transfer.Outcome, transfer.Detail, error) {
				calls++
				if calls == 3 {
					return 0, transfer.Detail{}, errors.New("磁盘满了")
				}
				return old(src, dst, m)
			}
			res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
			require.NoError(t, err)
			assert.Equal(t, 1, res.Failed)
			assert.Contains(t, res.Plan.Items[0].Message, "磁盘满了")
			assert.False(t, exists(movieDir), "撤回放进去的文件，删掉建的目录")
			for _, in := range discIns {
				assert.True(t, exists(filepath.Join(root, filepath.FromSlash(in))), "源文件都在原处 %s", in)
			}
			r := e.history()[0]
			assert.Equal(t, models.MediaTransferFailed, r.Status)
			assert.NotNil(t, r.NextRetryAt, "过一会儿再试")
			assert.Empty(t, decodeExtras(r.Extras))

			transferDetail = old
			res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
			require.NoError(t, err)
			assert.Equal(t, 1, res.Created)
		})
	}
}

// 移动挪不回去的文件留在库里，写进记录（方式是移动，pt-tools 不删）
func TestOrganizeDiscMoveRollbackLeftovers(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeMove})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.discTorrent(func(t *downloader.Torrent) { t.State = downloader.TorrentPaused })
	old := transferDetail
	t.Cleanup(func() { transferDetail = old })
	calls := 0
	transferDetail = func(src, dst, m string) (transfer.Outcome, transfer.Detail, error) {
		calls++
		switch {
		case calls == 3:
			return 0, transfer.Detail{}, errors.New("磁盘满了")
		case calls > 3:
			// 挪回去也失败
			return 0, transfer.Detail{}, errors.New("只读")
		}
		return old(src, dst, m)
	}
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
	msg := res.Plan.Items[0].Message
	assert.Contains(t, msg, "2 个文件没能挪回")
	r := e.history()[0]
	assert.Equal(t, models.MediaTransferFailed, r.Status)
	assert.Equal(t, models.MediaModeMove, r.Mode)
	disc, _ := discExtrasOf(t, r)
	assert.Len(t, disc, 2, "库里留着的文件记在记录里")
	assert.True(t, hasLibraryFiles(&r))
	_, err = e.svc.DeleteHistory(e.ctx, r.ID, true)
	require.ErrorIs(t, err, ErrInvalid, "移动的文件是唯一的一份，不在这里删")
}

// 之前硬链接整理过、库里的目录被删了，改成移动以后放到一半失败、挪不回去：记录改成移动（库里留着的是唯一的一份），
// 不沿用之前硬链接的方式与文件，删除记录时不能连这些文件删
func TestOrganizeDiscMoveLeftoversReplacePriorRecord(t *testing.T) {
	e := newEnv(t)
	lib := e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.discTorrent(func(t *downloader.Torrent) { t.State = downloader.TorrentPaused })
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	require.Equal(t, models.MediaModeHardlink, e.history()[0].Mode)
	movieDir := filepath.Join(e.movies, "奥本海默 (2023)")
	require.NoError(t, os.RemoveAll(movieDir))
	old := transferDetail
	t.Cleanup(func() { transferDetail = old })
	calls := 0
	transferDetail = func(src, dst, m string) (transfer.Outcome, transfer.Detail, error) {
		calls++
		switch {
		case calls == 3:
			return 0, transfer.Detail{}, errors.New("磁盘满了")
		case calls > 3:
			return 0, transfer.Detail{}, errors.New("只读")
		}
		return old(src, dst, m)
	}
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash, LibraryID: lib.ID, Mode: models.MediaModeMove})
	require.NoError(t, err)
	require.Equal(t, 1, res.Failed)
	r := e.history()[0]
	assert.Equal(t, models.MediaModeMove, r.Mode, "库里留着的是移动过去的文件")
	disc, _ := discExtrasOf(t, r)
	assert.Len(t, disc, 2)
	_, err = e.svc.DeleteHistory(e.ctx, r.ID, true)
	require.ErrorIs(t, err, ErrInvalid)
	for _, x := range disc {
		assert.True(t, exists(x.Target), "移动过去的文件留着 %s", x.Target)
	}
}

func TestDiscDeleteAndRetire(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.discTorrent(nil)
	root := filepath.Join(e.dlDir, discName)
	movieDir := filepath.Join(e.movies, "奥本海默 (2023)")
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	r := e.history()[0]

	kept, err := e.svc.DeleteHistory(e.ctx, r.ID, true)
	require.NoError(t, err)
	assert.Empty(t, kept)
	assert.False(t, exists(movieDir), "原盘文件、NFO 与图片、空目录都删掉")
	assert.True(t, exists(e.movies), "库目录留着")
	for _, in := range discIns {
		assert.True(t, exists(filepath.Join(root, filepath.FromSlash(in))), "下载目录不动")
	}

	// 洗版：换掉库里的这张原盘
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	r = e.history()[0]
	// 有一个文件被换掉了：留着，写明
	changed := filepath.Join(movieDir, "BDMV", "STREAM", "00002.m2ts")
	require.NoError(t, os.Remove(changed))
	require.NoError(t, os.WriteFile(changed, []byte("mine"), 0o644))
	kept, err = e.svc.Retire(e.ctx, r.ID, "洗版：换成了 2160p")
	require.NoError(t, err)
	require.Len(t, kept, 1)
	assert.Contains(t, kept[0], "00002.m2ts")
	assert.True(t, exists(changed))
	assert.False(t, exists(filepath.Join(movieDir, "BDMV", "STREAM", "00001.m2ts")))
	assert.False(t, exists(filepath.Join(movieDir, "movie.nfo")), "movie.nfo 是这条记录自己的，跟着删")
	assert.True(t, exists(filepath.Join(movieDir, "poster.jpg")), "目录里还有视频（换掉的那个流），目录里的图片留着")
	var after models.MediaTransferHistory
	require.NoError(t, e.db.First(&after, r.ID).Error)
	assert.Equal(t, models.MediaTransferRemoved, after.Status)
}

// 删除库里的文件时，目录里还有 ISO、DVD 的 VOB 也算有视频（目录里的海报等留着）
func TestHasVideoCountsDiscAndISO(t *testing.T) {
	for name, want := range map[string]bool{"Movie.iso": true, "VTS_01_1.VOB": true, "00001.m2ts": true, "movie.mkv": true, "poster.jpg": false} {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", name), nil, 0o644))
		assert.Equal(t, want, hasVideo(dir), name)
	}
}

func TestDiscRescrape(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.discTorrent(nil)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash})
	require.NoError(t, err)
	nfo := filepath.Join(e.movies, "奥本海默 (2023)", "movie.nfo")
	require.NoError(t, os.Remove(nfo))
	out, err := e.svc.Rescrape(e.ctx, e.history()[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 1, out.Records)
	assert.Contains(t, readFile(t, nfo), "872585", "重新刮削照样写 movie.nfo")
	assert.False(t, exists(filepath.Join(e.movies, "奥本海默 (2023).nfo")), "不按目录名写在库目录里")
}

func TestOrganizeMovieISO(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	name := "Oppenheimer.2023.1080p.BluRay-GRP"
	e.addTorrent("i1", name, map[string]int{name + "/" + name + ".ISO": 5}, nil)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "i1"})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created, res.Plan.Problem)
	want := filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023) - 1080p BluRay.iso")
	assert.True(t, exists(want), "ISO 按视频文件的规则命名，扩展名小写")
	assert.True(t, exists(strings.TrimSuffix(want, ".iso")+".nfo"))
}

func TestOrganizeTVISO(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	name := "The.Last.of.Us.S01.1080p.BluRay-GRP"
	e.addTorrent("i2", name, map[string]int{
		name + "/The.Last.of.Us.S01E01.1080p.BluRay-GRP.iso": 5,
		name + "/The.Last.of.Us.S01E02.1080p.BluRay-GRP.mkv": 5,
	}, func(t *downloader.Torrent) { t.Category = "tv" })
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "i2"})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 1, res.Skipped, "剧集里的 ISO 记一条跳过")
	var iso models.MediaTransferHistory
	require.NoError(t, e.db.Where("source_path = ?", filepath.Join(e.dlDir, name, "The.Last.of.Us.S01E01.1080p.BluRay-GRP.iso")).First(&iso).Error)
	assert.Equal(t, models.MediaTransferSkipped, iso.Status)
	assert.Contains(t, iso.Message, "ISO")
	reasons := map[string]string{}
	for _, s := range res.Plan.Skipped {
		reasons[filepath.Base(s.File.Rel)] = s.Reason
	}
	assert.Contains(t, reasons["The.Last.of.Us.S01E01.1080p.BluRay-GRP.iso"], "ISO", "预览里列在没选上的文件里")

	// 只有 ISO 的剧集：整个跳过，每个 ISO 一条记录
	only := "The.Last.of.Us.S01E02.1080p.BluRay.ISO-GRP"
	e.addTorrent("i3", only, map[string]int{only + "/" + only + ".iso": 5}, func(t *downloader.Torrent) { t.Category = "tv" })
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "i3"})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "ISO")
	var n int64
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("source_path = ? AND status = ?", filepath.Join(e.dlDir, only, only+".iso"), models.MediaTransferSkipped).Count(&n).Error)
	assert.Equal(t, int64(1), n)
}

// 目录监控里的原盘与剧集 ISO：整理（或跳过）一次，之后扫描不再排
func TestWatchDiscAndISO(t *testing.T) {
	e, d := watchEnv(t)
	for _, in := range discIns {
		e.put(d, discName+"/"+in, 1, 10*time.Minute)
	}
	e.put(d, "The.Last.of.Us.S01E01.1080p.BluRay-GRP.iso", 2, 10*time.Minute)
	e.svc.watchScan(e.ctx, e.mustSettings())
	require.Equal(t, 2, e.drain())
	var disc models.MediaTransferHistory
	require.NoError(t, e.db.Where("source_path = ?", filepath.Join(d.Path, discName)).First(&disc).Error)
	assert.Equal(t, models.MediaTransferDone, disc.Status)
	assert.True(t, exists(filepath.Join(e.movies, "奥本海默 (2023)", "BDMV", "STREAM", "00001.m2ts")))
	var iso models.MediaTransferHistory
	require.NoError(t, e.db.Where("source_path = ?", filepath.Join(d.Path, "The.Last.of.Us.S01E01.1080p.BluRay-GRP.iso")).First(&iso).Error)
	assert.Equal(t, models.MediaTransferSkipped, iso.Status)

	e.svc.watchScan(e.ctx, e.mustSettings())
	assert.Zero(t, e.drain(), "整理过、跳过的不再排")
}

// 原盘在这一项里面一层（这一项不是原盘根目录）：记录在原盘根目录上，之后扫描按它认处理过了
func TestWatchNestedDisc(t *testing.T) {
	e, d := watchEnv(t)
	entry := "Oppenheimer.2023.BluRay.Box"
	for _, in := range discIns {
		e.put(d, entry+"/DISC1/"+in, 1, 10*time.Minute)
	}
	e.svc.watchScan(e.ctx, e.mustSettings())
	require.Equal(t, 1, e.drain())
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, filepath.Join(d.Path, entry, "DISC1"), rows[0].SourcePath)
	assert.Equal(t, models.MediaTransferDone, rows[0].Status)
	e.svc.watchScan(e.ctx, e.mustSettings())
	assert.Zero(t, e.drain(), "按原盘根目录上的记录认处理过了")
}

// 复制用了写时复制：整理记录写明
func TestOrganizeCloneNote(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeCopy})
	e.settings(SettingsInput{MinVideoMB: 1})
	old := transferDetail
	t.Cleanup(func() { transferDetail = old })
	transferDetail = func(src, dst, m string) (transfer.Outcome, transfer.Detail, error) {
		out, d, err := old(src, dst, m)
		d.Cloned = err == nil && m == models.MediaModeCopy && (strings.HasSuffix(dst, ".mkv") || strings.HasSuffix(dst, "00001.m2ts"))
		return out, d, err
	}
	e.oppenheimer()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	assert.Contains(t, e.history()[0].Message, "写时复制")

	// 原盘放进同一部电影的目录里会被挡住：换一个库
	discLib := filepath.Join(e.root, "media", "discs")
	require.NoError(t, os.MkdirAll(discLib, 0o755))
	lib := e.library(LibraryInput{Name: "原盘", Kind: models.MediaKindMovie, Path: discLib, Mode: models.MediaModeCopy})
	e.discTorrent(nil)
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: discHash, LibraryID: lib.ID})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created, res.Plan.Items[0].Message)
	var r models.MediaTransferHistory
	require.NoError(t, e.db.Where("info_hash = ?", discHash).First(&r).Error)
	assert.Contains(t, r.Message, "写时复制", "原盘的主文件用了写时复制")
}
