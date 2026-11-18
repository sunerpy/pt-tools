package organize

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

const tlouTMDB = 100088

// libFile 在剧集库里建一个文件，返回路径与文件编号（整理记录按它认是不是当初放的）。
func (e *env) libFile(rel string) (string, string) {
	e.t.Helper()
	p := filepath.Join(e.tv, filepath.FromSlash(rel))
	require.NoError(e.t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(e.t, os.WriteFile(p, []byte(rel), 0o644))
	id, err := transfer.FileID(p)
	require.NoError(e.t, err)
	return p, id
}

func fileIDs(files []LibraryFile) []uint {
	out := []uint{}
	for _, f := range files {
		out = append(out, f.ID)
	}
	return out
}

// 删除预览的库文件：这个条目（剧集是这一季、选了集时有交集的）在库里有文件的记录，写明现在的样子与能不能在这里删。
func TestLibraryFiles(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	organized := e.history()
	require.Len(t, organized, 2)
	// 两集的记录按整理的先后写，不一定是 E01 在前
	slices.SortFunc(organized, func(a, b models.MediaTransferHistory) int { return a.Episode - b.Episode })
	e01, e02 := organized[0], organized[1]
	require.Equal(t, [4]int{tlouTMDB, 1, 1, 0}, [4]int{e01.TMDBID, e01.Season, e01.Episode, e01.EpisodeEnd})

	libID := e01.LibraryID
	mk := func(ep, end int, mode, status string, file bool) models.MediaTransferHistory {
		r := models.MediaTransferHistory{
			MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Season: 1, Episode: ep, EpisodeEnd: end, Mode: mode, Status: status,
			LibraryID: libID, Size: 100,
		}
		if file {
			r.TargetPath, r.TargetFileID = e.libFile(fmt.Sprintf("最后生还者 (2023)/Season 01/E%02d.mkv", ep))
		}
		return r
	}
	rows := e.putRows(
		mk(3, 0, models.MediaModeMove, models.MediaTransferDone, true),
		mk(4, 0, models.MediaModeExisting, models.MediaTransferDone, true),
		mk(5, 0, models.MediaModeHardlink, models.MediaTransferRemoved, true),
		mk(6, 0, models.MediaModeHardlink, models.MediaTransferFailed, false),
		mk(7, 8, models.MediaModeCopy, models.MediaTransferDone, true),
		models.MediaTransferHistory{MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Season: 2, Episode: 1, Status: models.MediaTransferDone, TargetPath: "/x"},
		models.MediaTransferHistory{MediaType: models.MediaKindMovie, TMDBID: tlouTMDB, Status: models.MediaTransferDone, TargetPath: "/y"},
	)
	moveRow, existingRow, packRow := rows[0], rows[1], rows[4]

	// E01 换成了别的文件、E02 不见了
	require.NoError(t, os.Remove(e01.TargetPath))
	require.NoError(t, os.WriteFile(e01.TargetPath, []byte("别人放的"), 0o644))
	require.NoError(t, os.Remove(e02.TargetPath))

	files, err := e.svc.LibraryFiles(e.ctx, FileQuery{MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Season: 1})
	require.NoError(t, err)
	assert.Equal(t, []uint{e01.ID, e02.ID, moveRow.ID, existingRow.ID, packRow.ID}, fileIDs(files), "被洗版删掉的、没整理出文件的、别的季与电影不列")
	byID := map[uint]LibraryFile{}
	for _, f := range files {
		byID[f.ID] = f
	}
	assert.Equal(t, FileChanged, byID[e01.ID].State)
	assert.Contains(t, byID[e01.ID].Note, "留着")
	assert.Equal(t, FileMissing, byID[e02.ID].State)
	assert.True(t, byID[e02.ID].Deletable, "文件不在了照样能删记录")
	assert.Equal(t, "剧集", byID[e02.ID].Library)
	assert.Equal(t, uint(1), byID[e02.ID].DownloaderID)
	assert.Equal(t, tlouHash, byID[e02.ID].InfoHash)
	assert.False(t, byID[moveRow.ID].Deletable)
	assert.Contains(t, byID[moveRow.ID].Note, "移动整理")
	assert.True(t, byID[existingRow.ID].Existing)
	assert.True(t, byID[existingRow.ID].Deletable)
	assert.Equal(t, FileOurs, byID[packRow.ID].State)
	assert.Equal(t, [2]int{7, 8}, [2]int{byID[packRow.ID].Episode, byID[packRow.ID].EpisodeEnd})

	files, err = e.svc.LibraryFiles(e.ctx, FileQuery{MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Season: 1, Episodes: []int{8, 2}})
	require.NoError(t, err)
	assert.Equal(t, []uint{e02.ID, packRow.ID}, fileIDs(files), "选了集时只要和这几集有交集的")

	files, err = e.svc.LibraryFiles(e.ctx, FileQuery{MediaType: models.MediaKindMovie, TMDBID: tlouTMDB})
	require.NoError(t, err)
	assert.Len(t, files, 1)
	files, err = e.svc.LibraryFiles(e.ctx, FileQuery{MediaType: models.MediaKindMovie})
	require.NoError(t, err)
	assert.Empty(t, files, "没有编号时什么也不列")
}

func TestFileQueryMatches(t *testing.T) {
	ep := func(s, n, end int) models.MediaTransferHistory {
		return models.MediaTransferHistory{MediaType: models.MediaKindTV, TMDBID: 9, Season: s, Episode: n, EpisodeEnd: end}
	}
	tv := FileQuery{MediaType: models.MediaKindTV, TMDBID: 9, Season: 1}
	assert.True(t, tv.Matches(ep(1, 3, 0)))
	assert.True(t, tv.Matches(ep(1, 0, 0)), "整季选时不知道集号的也算")
	assert.False(t, tv.Matches(ep(2, 3, 0)))
	assert.False(t, tv.Matches(models.MediaTransferHistory{MediaType: models.MediaKindMovie, TMDBID: 9}))
	some := FileQuery{MediaType: models.MediaKindTV, TMDBID: 9, Season: 1, Episodes: []int{4}}
	assert.True(t, some.Matches(ep(1, 3, 5)), "一个文件几集，有交集")
	assert.False(t, some.Matches(ep(1, 3, 0)))
	assert.False(t, some.Matches(ep(1, 0, 0)), "选了集时不知道集号的不算")
	assert.False(t, some.Matches(ep(1, 5, 6)))
	movie := FileQuery{MediaType: models.MediaKindMovie, TMDBID: 9, Season: 3}
	assert.True(t, movie.Matches(models.MediaTransferHistory{MediaType: models.MediaKindMovie, TMDBID: 9}), "电影不看季")
	assert.False(t, FileQuery{MediaType: models.MediaKindMovie}.Matches(models.MediaTransferHistory{MediaType: models.MediaKindMovie}))
}

// 库里的文件在不在：原盘看主文件，软链接要指向的文件也在。
func TestLibraryFilePresent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.mkv")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	assert.True(t, libraryFilePresent(&models.MediaTransferHistory{TargetPath: file, Mode: models.MediaModeHardlink}))
	assert.False(t, libraryFilePresent(&models.MediaTransferHistory{TargetPath: filepath.Join(dir, "none.mkv")}))

	link := filepath.Join(dir, "link.mkv")
	if err := os.Symlink(filepath.Join(dir, "src.mkv"), link); err == nil {
		assert.False(t, libraryFilePresent(&models.MediaTransferHistory{TargetPath: link, Mode: models.MediaModeSymlink}), "指向的文件不在")
		assert.True(t, libraryFilePresent(&models.MediaTransferHistory{TargetPath: link, Mode: models.MediaModeHardlink}), "不是软链接方式时只看它自己")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src.mkv"), []byte("x"), 0o644))
		assert.True(t, libraryFilePresent(&models.MediaTransferHistory{TargetPath: link, Mode: models.MediaModeSymlink}))
	}

	movieDir := filepath.Join(dir, "电影 (2020)")
	main := filepath.Join(movieDir, "BDMV", "STREAM", "00001.m2ts")
	extras, _ := json.Marshal([]extra{{Kind: extraDisc, Target: main, Main: true}})
	disc := &models.MediaTransferHistory{TargetPath: movieDir, Mode: models.MediaModeHardlink, Extras: string(extras)}
	require.NoError(t, os.MkdirAll(movieDir, 0o755))
	assert.False(t, libraryFilePresent(disc), "电影目录在，主文件不在")
	require.NoError(t, os.MkdirAll(filepath.Dir(main), 0o755))
	require.NoError(t, os.WriteFile(main, []byte("x"), 0o644))
	assert.True(t, libraryFilePresent(disc))
}

// 连数据删了种子、留着库里的硬链接：标了 keep_links 的记录，打开「删种时一并删除入库链接」也不删；同一个源文件重新整理时清掉。
func TestKeepLinksSurvivesReconcile(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1, DeleteLinksOnRemove: true})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	movie := e.history()[0]
	other := e.putRows(models.MediaTransferHistory{Status: models.MediaTransferDone, Mode: models.MediaModeCopy, TargetPath: "/c"},
		models.MediaTransferHistory{Status: models.MediaTransferFailed, Mode: models.MediaModeHardlink, TargetPath: "/f"})

	require.NoError(t, e.svc.KeepLinks(e.ctx, []uint{movie.ID, other[0].ID, other[1].ID}))
	require.NoError(t, e.svc.KeepLinks(e.ctx, nil))
	var flags []bool
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Order("id").Pluck("keep_links", &flags).Error)
	assert.Equal(t, []bool{true, false, false}, flags, "只标已整理的硬链接记录")

	e.dl.remove(oppHash)
	require.NoError(t, os.RemoveAll(filepath.Join(e.dlDir, oppName)))
	assert.Zero(t, e.svc.Reconcile(e.ctx), "用户选了留着库里的文件")
	assert.True(t, exists(movie.TargetPath))
	// 判断以后、清理以前才标上的也不删
	ok, err := e.svc.reconcileRow(e.ctx, movie)
	require.NoError(t, err)
	assert.False(t, ok)

	// 重新下载了同一个种子：库里留着的是唯一的一份，记录照旧，标记也留着
	e.oppenheimer()
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	var row models.MediaTransferHistory
	require.NoError(t, e.db.First(&row, movie.ID).Error)
	assert.True(t, row.KeepLinks)

	// 库里的文件删了以后重新整理：放进去的是新的链接，清掉标记
	require.NoError(t, os.Remove(movie.TargetPath))
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.NoError(t, e.db.First(&row, movie.ID).Error)
	assert.False(t, row.KeepLinks)
	assert.True(t, sameFile(t, row.TargetPath, row.SourcePath))
}

// 删了整理记录（或洗版换掉）以后：这条记录的 library_missing 改成解决了；这一部的 server_removed 对不上已整理记录的也改成解决了，
// 还对得上别的已整理记录的不动。
func TestRecordGoneClearsIssues(t *testing.T) {
	e := newEnv(t)
	now := e.Now()
	rows := e.putRows(
		models.MediaTransferHistory{MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Season: 1, Episode: 1, Status: models.MediaTransferDone, TargetPath: "/lib/E01.mkv"},
		models.MediaTransferHistory{MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Season: 1, Episode: 2, Status: models.MediaTransferDone, TargetPath: "/lib/E02.mkv"},
		models.MediaTransferHistory{MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Season: 1, Episode: 2, Status: models.MediaTransferDone, TargetPath: "/lib/E02.v2.mkv"},
	)
	srv := models.MediaServer{Name: "Jellyfin", Kind: models.MediaServerJellyfin, URL: "http://jf", Enabled: true}
	require.NoError(t, e.db.Create(&srv).Error)
	removed := now.Add(-time.Hour)
	require.NoError(t, e.db.Create(&[]models.MediaServerItem{
		{ServerID: srv.ID, ItemID: "a", Kind: models.MediaServerItemEpisode, TMDBID: tlouTMDB, Season: 1, Episode: 1, LocalPath: "/lib/E01.mkv", RemovedAt: &removed},
		{ServerID: srv.ID, ItemID: "b", Kind: models.MediaServerItemEpisode, TMDBID: tlouTMDB, Season: 1, Episode: 2, RemovedAt: &removed},
	}).Error)
	db := e.db
	info := func(ep int) serverindex.IssueInfo {
		return serverindex.IssueInfo{MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Season: 1, Episode: ep}
	}
	require.NoError(t, serverindex.Raise(db, serverindex.IssueKey{Kind: models.MediaIssueServerRemoved, ServerID: srv.ID, ServerItemID: "a"}, info(1), now))
	require.NoError(t, serverindex.Raise(db, serverindex.IssueKey{Kind: models.MediaIssueServerRemoved, ServerID: srv.ID, ServerItemID: "b"}, info(2), now))
	// 索引里已经清掉的一项（按问题记的季集对）
	require.NoError(t, serverindex.Raise(db, serverindex.IssueKey{Kind: models.MediaIssueServerRemoved, ServerID: srv.ID, ServerItemID: "gone"}, info(1), now))
	require.NoError(t, serverindex.Raise(db, serverindex.MissingKey(rows[0].ID), info(1), now))
	require.NoError(t, serverindex.Raise(db, serverindex.MissingKey(rows[1].ID), info(2), now))

	status := func() map[string]string {
		var list []models.MediaIssue
		require.NoError(t, db.Order("id").Find(&list).Error)
		out := map[string]string{}
		for _, is := range list {
			key := is.ServerItemID
			if is.Kind == models.MediaIssueLibraryMissing {
				key = fmt.Sprintf("h%d", is.HistoryID)
			}
			out[key] = is.Status
		}
		return out
	}

	_, err := e.svc.DeleteHistory(e.ctx, rows[0].ID, false)
	require.NoError(t, err)
	got := status()
	assert.Equal(t, models.MediaIssueResolved, got["a"], "E01 没有已整理记录了（按路径）")
	assert.Equal(t, models.MediaIssueResolved, got["gone"], "按问题记的季集也对不上了")
	assert.Equal(t, models.MediaIssueOpen, got["b"], "E02 还有两条记录")
	assert.Equal(t, models.MediaIssueResolved, got[fmt.Sprintf("h%d", rows[0].ID)])
	assert.Equal(t, models.MediaIssueOpen, got[fmt.Sprintf("h%d", rows[1].ID)])

	_, err = e.svc.DeleteHistory(e.ctx, rows[1].ID, false)
	require.NoError(t, err)
	assert.Equal(t, models.MediaIssueOpen, status()["b"], "E02 还有另一个版本的记录")
	assert.Equal(t, models.MediaIssueResolved, status()[fmt.Sprintf("h%d", rows[1].ID)])

	// 洗版换掉最后一条：E02 也对不上了
	_, err = e.svc.Retire(e.ctx, rows[2].ID, "洗版")
	require.NoError(t, err)
	assert.Equal(t, models.MediaIssueResolved, status()["b"])
	var is models.MediaIssue
	require.NoError(t, db.Where("server_item_id = ?", "b").First(&is).Error)
	assert.Equal(t, "洗版换掉了这个版本", is.Detail)
}
