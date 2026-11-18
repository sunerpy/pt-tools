package organize

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/models"
)

func (e *env) missingIssue(historyID uint) models.MediaIssue {
	e.t.Helper()
	var is models.MediaIssue
	require.NoError(e.t, e.db.Where("kind = ? AND history_id = ?", models.MediaIssueLibraryMissing, historyID).Limit(1).Find(&is).Error)
	return is
}

func (e *env) libCheckState() libCheckState {
	e.t.Helper()
	st, err := e.svc.loadLibCheck(e.ctx)
	require.NoError(e.t, err)
	return st
}

// 巡检：文件不见了的记成 library_missing，又有了的改成解决了，忽略的在文件回来以前一直忽略；媒体库目录不在（没挂载）、
// 媒体库删了的不查；整理记录删了的问题改成解决了。
func TestCheckLibraryFiles(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	tvID := e.libraryID(models.MediaKindTV)
	offline := filepath.Join(e.root, "offline")
	require.NoError(t, os.MkdirAll(offline, 0o755))
	off := e.library(LibraryInput{Name: "外接盘", Kind: models.MediaKindTV, Path: offline})
	require.NoError(t, os.Remove(offline))

	ep := func(n int, lib uint, status string) models.MediaTransferHistory {
		r := models.MediaTransferHistory{
			MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Season: 1, Episode: n, Status: status, LibraryID: lib,
			Mode: models.MediaModeHardlink, Title: "最后生还者",
		}
		r.TargetPath, r.TargetFileID = e.libFile(filepath.Join("最后生还者 (2023)", "Season 01", fmt.Sprintf("E%02d.mkv", n)))
		return r
	}
	rows := e.putRows(ep(1, tvID, models.MediaTransferDone), ep(2, tvID, models.MediaTransferDone), ep(3, off.ID, models.MediaTransferDone),
		ep(4, tvID, models.MediaTransferFailed), ep(5, 999, models.MediaTransferDone))
	r1, r2 := rows[0], rows[1]

	res, err := e.svc.CheckLibraryFiles(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, LibraryCheckResult{Checked: 2, Skipped: 2}, res, "外接盘没挂载、媒体库删了的不查，失败的记录不查")
	assert.False(t, e.libCheckState().At.IsZero())

	require.NoError(t, os.Remove(r2.TargetPath))
	res, err = e.svc.CheckLibraryFiles(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Missing)
	is := e.missingIssue(r2.ID)
	assert.Equal(t, models.MediaIssueOpen, is.Status)
	assert.Equal(t, [4]any{models.MediaKindTV, tlouTMDB, 1, 2}, [4]any{is.MediaType, is.TMDBID, is.Season, is.Episode})
	assert.Equal(t, "最后生还者", is.Title)
	assert.Contains(t, is.Detail, r2.TargetPath)
	assert.Zero(t, e.missingIssue(r1.ID).ID, "文件在的不写")

	// 用户忽略了：文件回来以前一直忽略
	_, err = serverindex.Dismiss(e.ctx, e.db, is.ID, e.Now())
	require.NoError(t, err)
	_, err = e.svc.CheckLibraryFiles(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, models.MediaIssueDismissed, e.missingIssue(r2.ID).Status)

	// 文件回来了：解决了；又不见了：重新打开
	require.NoError(t, os.WriteFile(r2.TargetPath, []byte("回来了"), 0o644))
	res, err = e.svc.CheckLibraryFiles(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Back)
	is = e.missingIssue(r2.ID)
	assert.Equal(t, models.MediaIssueResolved, is.Status)
	assert.Equal(t, "库里的文件又有了", is.Detail)
	require.NoError(t, os.Remove(r2.TargetPath))
	_, err = e.svc.CheckLibraryFiles(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, models.MediaIssueOpen, e.missingIssue(r2.ID).Status)

	// 记录被删了（不经删除整理记录）：问题改成解决了
	require.NoError(t, e.db.Delete(&models.MediaTransferHistory{}, r2.ID).Error)
	_, err = e.svc.CheckLibraryFiles(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, models.MediaIssueResolved, e.missingIssue(r2.ID).Status)
}

// 一轮查不完时按编号接着查，查到头以后从头再来；进度存在 app_meta 里，重启以后不从头查。
func TestCheckLibraryFilesRotates(t *testing.T) {
	old := libCheckBatch
	libCheckBatch = 2
	t.Cleanup(func() { libCheckBatch = old })
	e := newEnv(t)
	e.defaultLibraries()
	tvID := e.libraryID(models.MediaKindTV)
	var rows []models.MediaTransferHistory
	for n := 1; n <= 5; n++ {
		r := models.MediaTransferHistory{MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Season: 1, Episode: n, Status: models.MediaTransferDone, LibraryID: tvID}
		r.TargetPath, r.TargetFileID = e.libFile(filepath.Join("剧", fmt.Sprintf("E%02d.mkv", n)))
		rows = append(rows, e.putRows(r)...)
	}
	for _, r := range rows {
		require.NoError(t, os.Remove(r.TargetPath))
	}
	checked := func() []uint {
		var ids []uint
		require.NoError(t, e.db.Model(&models.MediaIssue{}).Order("history_id").Pluck("history_id", &ids).Error)
		return ids
	}
	_, err := e.svc.CheckLibraryFiles(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, []uint{rows[0].ID, rows[1].ID}, checked())
	assert.Equal(t, rows[1].ID, e.libCheckState().AfterID)

	// 换一个服务（重启）：从 app_meta 记下的地方接着
	e.svc = New(Config{DB: e.db, Cipher: fakeCipher{}, Recognizer: e.rec, Downloaders: e.dls, Now: e.Now})
	_, err = e.svc.CheckLibraryFiles(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, []uint{rows[0].ID, rows[1].ID, rows[2].ID, rows[3].ID}, checked())
	res, err := e.svc.CheckLibraryFiles(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Checked)
	assert.Zero(t, e.libCheckState().AfterID, "查到头了，下一轮从头来")
	assert.Len(t, checked(), 5)

	// 进度写坏了：从头来
	require.NoError(t, models.WriteAppMeta(e.db, libCheckMeta, "{坏的"))
	assert.Equal(t, libCheckState{}, e.libCheckState())
}

// 排期：从没巡检过时启动 10 分钟以后第一次，之后每 24 小时；失败时一小时以后再试；重启以后按上一轮的时间排。
func TestLibCheckTick(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	started := e.Now()
	lc := libCheck{started: started}
	e.svc.libCheckTick(e.ctx, &lc)
	assert.Equal(t, started.Add(libCheckFirst), lc.next)
	assert.True(t, e.libCheckState().At.IsZero(), "还没到时间")

	e.advance(libCheckFirst)
	e.svc.libCheckTick(e.ctx, &lc)
	ran := e.Now()
	assert.Equal(t, ran, e.libCheckState().At)
	assert.Equal(t, ran.Add(libCheckEvery), lc.next)

	e.advance(time.Hour)
	e.svc.libCheckTick(e.ctx, &lc)
	assert.Equal(t, ran, e.libCheckState().At, "24 小时以内不再巡检")

	// 重启：按上一轮的时间排，不是启动 10 分钟以后
	lc2 := libCheck{started: e.Now()}
	e.svc.libCheckTick(e.ctx, &lc2)
	assert.Equal(t, ran.Add(libCheckEvery), lc2.next)

	// 巡检失败：一小时以后再试
	e.advance(libCheckEvery)
	require.NoError(t, e.db.Migrator().DropTable(&models.MediaIssue{}))
	e.svc.libCheckTick(e.ctx, &lc2)
	assert.Equal(t, e.Now().Add(time.Hour), lc2.next)

	// 读不到进度：这一拍不排，下一拍再读
	require.NoError(t, e.db.Exec("DROP TABLE app_meta").Error)
	require.NoError(t, e.db.Exec("CREATE TABLE app_meta (key text)").Error)
	lc3 := libCheck{started: e.Now()}
	e.svc.libCheckTick(e.ctx, &lc3)
	assert.True(t, lc3.next.IsZero())
}
