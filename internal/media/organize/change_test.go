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

	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// 照用户反馈写的：剧集按名字搜索认成了一部（最后生还者），整理进库、刮削了；之后加了纠正认成另一部（绝命毒师）。
// 自动整理不换；用户自己点的整理先在新位置放好、记录改成纠正的条目，再删掉旧位置里 pt-tools 放的文件（共用的 NFO 也删、空目录删）。

func (e *env) tlouToBreakingBad() {
	e.t.Helper()
	_, err := e.rec.SetOverride(e.ctx, recognize.OverrideInput{Title: tlouName, MediaType: models.MediaKindTV, TMDBID: bbID})
	require.NoError(e.t, err)
}

func TestChangeItemAfterCorrection(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeItems: []string{"nfo"}})
	e.lastOfUs()
	req := Request{DownloaderID: 1, Hash: tlouHash}
	res, err := e.svc.Organize(e.ctx, req)
	require.NoError(t, err)
	require.Equal(t, 2, res.Created)
	oldDir := filepath.Join(e.tv, "最后生还者 (2023)")
	require.FileExists(t, filepath.Join(oldDir, "tvshow.nfo"))
	before := e.history()
	for _, r := range before {
		require.Equal(t, recognize.SourceSearch, r.MatchSource)
		require.Equal(t, tlouID, r.TMDBID)
	}

	e.tlouToBreakingBad()
	p, err := e.svc.Preview(e.ctx, req)
	require.NoError(t, err)
	require.Empty(t, p.Problem)
	assert.Equal(t, bbID, p.Match.ID)
	assert.Equal(t, recognize.SourceOverride, p.Source)
	assert.Contains(t, p.Notes, "手动纠正把它认成 TMDB tv 1396，不再沿用之前整理用的 tv 100088")
	require.Len(t, p.Items, 2)
	for _, it := range p.Items {
		assert.Equal(t, ItemChange, it.Status, it.Rel)
		assert.Contains(t, it.Target, filepath.Join(e.tv, "绝命毒师 (2008)"))
		assert.Equal(t, tlouID, it.PriorTMDBID)
		assert.Equal(t, models.MediaKindTV, it.PriorMediaType)
		assert.Contains(t, it.PriorTarget, oldDir)
	}

	// 自动整理（自动重试、订阅补整理）不换
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash, Auto: true})
	require.NoError(t, err)
	assert.Zero(t, res.Changed)
	assert.Equal(t, 2, res.Done)
	// 定期扫描、目录监控也不换
	e.svc.mu.Lock()
	res, err = e.svc.organize(e.ctx, req, models.MediaTriggerScan)
	e.svc.mu.Unlock()
	require.NoError(t, err)
	assert.Zero(t, res.Changed)
	for i, r := range e.history() {
		assert.Equal(t, before[i].TargetPath, r.TargetPath, "自动整理不动文件与记录")
		assert.Equal(t, tlouID, r.TMDBID)
	}

	// 用户点整理：换过去
	e.advance(time.Hour)
	res, err = e.svc.Organize(e.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Changed, res.Messages)
	assert.Zero(t, res.Failed, res.Messages)
	assert.Zero(t, res.Created, "换条目不算新入库")
	newDir := filepath.Join(e.tv, "绝命毒师 (2008)")
	for _, r := range e.history() {
		assert.Equal(t, bbID, r.TMDBID)
		assert.Equal(t, recognize.SourceOverride, r.MatchSource)
		assert.True(t, strings.HasPrefix(r.TargetPath, newDir), r.TargetPath)
		assert.Equal(t, models.MediaTransferDone, r.Status)
		assert.WithinDuration(t, e.Now(), r.CreatedAt, time.Second, "整理时间改成这次的，自动补刮按这次算")
		assert.Contains(t, r.Message, "手动纠正把它认成 TMDB tv 1396")
		src, serr := os.Stat(r.SourcePath)
		require.NoError(t, serr)
		dst, derr := os.Stat(r.TargetPath)
		require.NoError(t, derr)
		assert.True(t, os.SameFile(src, dst), "新位置是源文件的硬链接")
		assert.NotContains(t, r.Extras, oldDir, "旧位置的文件不留在记录里")
	}
	_, err = os.Stat(oldDir)
	assert.ErrorIs(t, err, os.ErrNotExist, "旧位置里 pt-tools 放的文件（视频、单集 NFO、tvshow.nfo、season.nfo）都删了，空目录也删了")
	show := readFile(t, filepath.Join(newDir, "tvshow.nfo"))
	assert.Contains(t, show, "<title>绝命毒师</title>", "按纠正的条目刮削")

	// 再预览：已经是这个条目，库里有了
	p, err = e.svc.Preview(e.ctx, req)
	require.NoError(t, err)
	for _, it := range p.Items {
		assert.Equal(t, ItemDone, it.Status)
		assert.Empty(t, it.PriorTarget)
	}
}

// 明确指定的条目（手动指定、订阅的、推送时说明的）不被纠正换掉；沿用时来源照旧，重试写回记录以后再加冲突的纠正也不换。
// 手动点的整理、条目是自动识别的（Trigger=manual, MatchSource=search）会被纠正换掉。
func TestChangeItemPinnedEntries(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(e *env)
		req    Request
		source string
	}{
		{"手动指定", func(*env) {}, Request{DownloaderID: 1, Hash: tlouHash, MediaType: models.MediaKindTV, TMDBID: tlouID}, sourceManual},
		{"订阅的", func(e *env) { e.follows(MediaHint{MediaType: models.MediaKindTV, TMDBID: tlouID}) }, Request{DownloaderID: 1, Hash: tlouHash}, sourceSubscription},
		{"推送时说明的", func(e *env) { e.pushedMedia(tlouHash, models.MediaKindTV, tlouID, 1) }, Request{DownloaderID: 1, Hash: tlouHash}, sourcePush},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.settings(SettingsInput{MinVideoMB: 1})
			e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv})
			e.lastOfUs()
			c.setup(e)
			_, err := e.svc.Organize(e.ctx, c.req)
			require.NoError(t, err)
			rows := e.history()
			require.Len(t, rows, 2)
			require.Equal(t, c.source, rows[0].MatchSource)
			e.tlouToBreakingBad()
			for range 2 {
				// 重试一次（写回记录）以后再重试：还是明确指定的来源、条目不换
				_, err = e.svc.Retry(e.ctx, rows[0].ID)
				require.NoError(t, err)
				for _, r := range e.history() {
					assert.Equal(t, tlouID, r.TMDBID)
					assert.Equal(t, c.source, r.MatchSource)
				}
			}
			p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
			require.NoError(t, err)
			assert.Equal(t, tlouID, p.Match.ID)
			assert.Equal(t, c.source, p.Source)
		})
	}

	// 手动点的整理、条目是自动识别的：纠正换掉
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv})
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	rows := e.history()
	require.Equal(t, models.MediaTriggerManual, rows[0].Trigger)
	require.Equal(t, recognize.SourceSearch, rows[0].MatchSource)
	e.tlouToBreakingBad()
	res, err := e.svc.Retry(e.ctx, rows[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Changed, res.Messages)
	assert.Equal(t, bbID, e.history()[0].TMDBID)
}

func TestPinnedAndManualEntry(t *testing.T) {
	row := func(src, trigger string) models.MediaTransferHistory {
		return models.MediaTransferHistory{MatchSource: src, Trigger: trigger}
	}
	for _, c := range []struct {
		r              models.MediaTransferHistory
		pinned, manual bool
		prior          string
	}{
		{row(sourceManual, models.MediaTriggerScan), true, true, sourceManual},
		{row(sourceSubscription, models.MediaTriggerAuto), true, false, sourceSubscription},
		{row(sourcePush, models.MediaTriggerAuto), true, false, sourcePush},
		{row("", models.MediaTriggerManual), true, true, sourceManual},
		{row("", models.MediaTriggerScan), false, false, sourceHistory},
		{row(recognize.SourceSearch, models.MediaTriggerManual), false, false, sourceHistory},
		{row(recognize.SourceOverride, models.MediaTriggerManual), false, false, sourceHistory},
		{row(sourceHistory, models.MediaTriggerManual), false, false, sourceHistory},
	} {
		assert.Equal(t, c.pinned, pinnedEntry(c.r), "%+v", c.r)
		assert.Equal(t, c.manual, manualEntry(c.r), "%+v", c.r)
		assert.Equal(t, c.prior, priorSource(c.r), "%+v", c.r)
	}
	assert.Equal(t, Request{DownloaderID: 1, Hash: "h", retried: true}, retryRequest(models.MediaTransferHistory{
		DownloaderID: 1, InfoHash: "h", Trigger: models.MediaTriggerManual, MatchSource: recognize.SourceSearch, MediaType: "tv", TMDBID: 5, LibraryID: 2,
	}), "手动点的、条目是自动识别的：不沿用")
	assert.Equal(t, Request{DownloaderID: 1, Hash: "h", MediaType: "tv", TMDBID: 5, LibraryID: 2, retried: true}, retryRequest(models.MediaTransferHistory{
		DownloaderID: 1, InfoHash: "h", Trigger: models.MediaTriggerScan, MatchSource: sourceManual, MediaType: "tv", TMDBID: 5, LibraryID: 2,
	}))
}

// 条目变了、这个文件不换的几种情况：移动整理的、导入的已有文件、源文件不在、新位置被别的文件占着。预览写明原因，
// 整理后库里的文件与记录都不变（补了字幕也不写记录）。
func TestChangeItemKept(t *testing.T) {
	cases := []struct {
		name  string
		setup func(e *env, rows []models.MediaTransferHistory)
		want  string
	}{
		{"移动整理的", func(e *env, rows []models.MediaTransferHistory) {
			require.NoError(e.t, e.db.Model(&models.MediaTransferHistory{}).Where("1 = 1").Update("mode", models.MediaModeMove).Error)
		}, "移动整理的文件（库里唯一的一份）不换条目"},
		{"导入的已有文件", func(e *env, rows []models.MediaTransferHistory) {
			require.NoError(e.t, e.db.Model(&models.MediaTransferHistory{}).Where("1 = 1").Update("mode", models.MediaModeExisting).Error)
		}, "导入时就在库里的文件不换条目"},
		{"源文件不在", func(e *env, rows []models.MediaTransferHistory) {
			for _, r := range rows {
				require.NoError(e.t, os.Remove(r.SourcePath))
			}
		}, "下载目录里的源文件不在了，换不了条目"},
		{"新位置被别的文件占着", func(e *env, rows []models.MediaTransferHistory) {
			p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
			require.NoError(e.t, err)
			for _, it := range p.Items {
				require.NoError(e.t, os.MkdirAll(filepath.Dir(it.Target), 0o755))
				require.NoError(e.t, os.WriteFile(it.Target, []byte("someone else"), 0o644))
			}
		}, "换条目：新位置已经有文件"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.settings(SettingsInput{MinVideoMB: 1})
			e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv})
			e.lastOfUs()
			_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
			require.NoError(t, err)
			e.tlouToBreakingBad()
			c.setup(e, e.history())
			before := e.history()
			p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
			require.NoError(t, err)
			require.Empty(t, p.Problem)
			for _, it := range p.Items {
				assert.Equal(t, ItemDone, it.Status, it.Rel)
				assert.Contains(t, it.Message, c.want)
				assert.Equal(t, tlouID, it.PriorTMDBID)
			}
			res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
			require.NoError(t, err)
			assert.Zero(t, res.Changed)
			assert.Equal(t, 2, res.Done)
			assert.Contains(t, strings.Join(res.Messages, "\n"), c.want)
			after := e.history()
			for i := range before {
				assert.Equal(t, before[i].TMDBID, after[i].TMDBID)
				assert.Equal(t, before[i].TargetPath, after[i].TargetPath)
				assert.Equal(t, before[i].Extras, after[i].Extras)
				assert.Equal(t, before[i].Status, after[i].Status)
				assert.Equal(t, before[i].MatchSource, after[i].MatchSource)
				_, lerr := os.Lstat(after[i].TargetPath)
				assert.NoError(t, lerr, "库里的文件还在")
			}
		})
	}
}

// 新位置放不进去：旧文件、旧记录都不变，旧位置一个文件都没删。
func TestChangeItemPlaceFails(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeItems: []string{"nfo"}})
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	e.tlouToBreakingBad()
	before := e.history()
	old := transferDetail
	t.Cleanup(func() { transferDetail = old })
	transferDetail = func(string, string, string) (transfer.Outcome, transfer.Detail, error) {
		return transfer.Created, transfer.Detail{}, errors.New("磁盘满了")
	}
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Zero(t, res.Changed)
	assert.Equal(t, 2, res.Failed)
	assert.Contains(t, strings.Join(res.Messages, "\n"), "换条目没成：磁盘满了，库里还是原来的")
	for i, r := range e.history() {
		assert.Equal(t, before[i], r, "记录不变")
		_, serr := os.Stat(r.TargetPath)
		assert.NoError(t, serr)
	}
	assert.FileExists(t, filepath.Join(e.tv, "最后生还者 (2023)", "tvshow.nfo"))
}

// 新旧条目算出的位置相同（模板里没有剧名）：只改记录，按新条目重新刮削，文件不动。
func TestChangeItemInPlace(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{
		Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Template: "Shows/Season {{.Season}}/{{.SeasonEpisode}}", Scrape: true, ScrapeItems: []string{"nfo"},
	})
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	before := e.history()
	e.tlouToBreakingBad()
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	for _, it := range p.Items {
		assert.Equal(t, ItemChange, it.Status)
		assert.Contains(t, it.Message, "库里的文件位置不变")
	}
	e.advance(time.Hour)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Changed, res.Messages)
	for i, r := range e.history() {
		assert.Equal(t, bbID, r.TMDBID)
		assert.Equal(t, before[i].TargetPath, r.TargetPath)
		assert.Equal(t, before[i].TargetFileID, r.TargetFileID)
		assert.NotEmpty(t, r.ScrapeStatus, "重新刮削了")
		_, serr := os.Stat(r.TargetPath)
		assert.NoError(t, serr)
	}
	assert.Contains(t, readFile(t, filepath.Join(e.tv, "Shows", "tvshow.nfo")), "<title>绝命毒师</title>")
}

// usLibrary 加第二个剧集库「美剧」（不是默认选的那个）。
func (e *env) usLibrary() (uint, string) {
	e.t.Helper()
	us := filepath.Join(e.root, "media", "us")
	require.NoError(e.t, os.MkdirAll(us, 0o755))
	return e.library(LibraryInput{Name: "美剧", Kind: models.MediaKindTV, Path: us}).ID, us
}

// 手动整理时选了不是默认的库、条目是自动识别的：之后请求里不带库（重试、重新整理）时，已整理的文件不动，
// 没整理好的文件接着放进这个种子原来的库，同一个种子不分到两个库。只有库不同不算换条目。
func TestChangeItemLibraryOnlyStays(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv})
	usID, us := e.usLibrary()
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash, LibraryID: usID})
	require.NoError(t, err)
	rows := e.history()
	require.Len(t, rows, 2)
	for _, r := range rows {
		require.Equal(t, usID, r.LibraryID)
		require.Equal(t, recognize.SourceSearch, r.MatchSource)
	}
	// 第二条改成失败、库里的文件删掉，然后点它的「重试」
	require.NoError(t, os.Remove(rows[1].TargetPath))
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", rows[1].ID).Update("status", models.MediaTransferFailed).Error)
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	require.NotNil(t, p.Library)
	assert.Equal(t, usID, p.Library.ID, "接着用这个种子原来的库")
	for _, it := range p.Items {
		assert.NotEqual(t, ItemChange, it.Status, it.Rel)
	}
	res, err := e.svc.Retry(e.ctx, rows[1].ID)
	require.NoError(t, err)
	assert.Zero(t, res.Changed, res.Messages)
	assert.Equal(t, 1, res.Created, res.Messages)
	assert.Equal(t, 1, res.Done, res.Messages)
	for _, r := range e.history() {
		assert.Equal(t, usID, r.LibraryID)
		assert.True(t, strings.HasPrefix(r.TargetPath, us), r.TargetPath)
		_, serr := os.Stat(r.TargetPath)
		assert.NoError(t, serr, "库里的文件在")
	}

	// 整理弹窗里明确选了同一个条目、另一个库：条目没变，已整理的文件也不搬
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash, MediaType: models.MediaKindTV, TMDBID: tlouID, LibraryID: e.libraryID(models.MediaKindTV)})
	require.NoError(t, err)
	assert.Zero(t, res.Changed, res.Messages)
	assert.Equal(t, 2, res.Done, res.Messages)
	for _, r := range e.history() {
		assert.Equal(t, usID, r.LibraryID)
	}
}

// 加了纠正换条目：留在这个种子原来的库里（之前选的不是默认的库）。
func TestChangeItemKeepsLibrary(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv})
	usID, us := e.usLibrary()
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash, LibraryID: usID})
	require.NoError(t, err)
	e.tlouToBreakingBad()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Changed, res.Messages)
	for _, r := range e.history() {
		assert.Equal(t, bbID, r.TMDBID)
		assert.Equal(t, usID, r.LibraryID)
		assert.True(t, strings.HasPrefix(r.TargetPath, filepath.Join(us, "绝命毒师 (2008)")), r.TargetPath)
	}
	_, err = os.Stat(filepath.Join(us, "最后生还者 (2023)"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// planOrder 是这个种子的记录按整理计划里的顺序（换条目时按这个顺序处理）。
func (e *env) planOrder() []models.MediaTransferHistory {
	e.t.Helper()
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(e.t, err)
	byID := map[uint]models.MediaTransferHistory{}
	for _, r := range e.history() {
		byID[r.ID] = r
	}
	var out []models.MediaTransferHistory
	for _, it := range p.Items {
		out = append(out, byID[it.HistoryID])
	}
	return out
}

// 同一个种子里一条记录的条目是手动指定的：加了纠正以后，自动识别来的换过去，手动指定的那条不换。
func TestChangeItemSkipsPinnedRow(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv})
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	rows := e.planOrder()
	require.Len(t, rows, 2)
	pinned := rows[1]
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", pinned.ID).Update("match_source", sourceManual).Error)
	e.tlouToBreakingBad()
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	for _, it := range p.Items {
		if it.HistoryID == pinned.ID {
			assert.Equal(t, ItemDone, it.Status, "手动指定的不换")
		} else {
			assert.Equal(t, ItemChange, it.Status, "自动识别来的换")
		}
	}
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Changed, res.Messages)
	var got models.MediaTransferHistory
	require.NoError(t, e.db.First(&got, pinned.ID).Error)
	assert.Equal(t, tlouID, got.TMDBID)
	assert.Equal(t, pinned.TargetPath, got.TargetPath)
	_, err = os.Stat(got.TargetPath)
	assert.NoError(t, err)
}

// 重试一条手动指定了别的条目的失败记录：带着的条目是那条记录的，不算这次明确指定的，同一个种子里已整理的文件不跟着换。
func TestRetryCarriedEntryKeepsOthers(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv})
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	rows := e.history()
	require.NoError(t, os.Remove(rows[1].TargetPath))
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", rows[1].ID).Updates(map[string]any{
		"status": models.MediaTransferFailed, "match_source": sourceManual, "tmdb_id": bbID,
	}).Error)
	res, err := e.svc.Retry(e.ctx, rows[1].ID)
	require.NoError(t, err)
	assert.Zero(t, res.Changed, res.Messages)
	var first models.MediaTransferHistory
	require.NoError(t, e.db.First(&first, rows[0].ID).Error)
	assert.Equal(t, tlouID, first.TMDBID)
	assert.Equal(t, rows[0].TargetPath, first.TargetPath)
	_, err = os.Stat(first.TargetPath)
	assert.NoError(t, err)
}

// 目录里共用的刮削文件（tvshow.nfo、season.nfo）只记在先处理的那条记录里（另一集是后来整理的，没有重写它们）：
// 换条目以后旧目录也删干净。
func TestChangeItemRemovesSharedFiles(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeItems: []string{"nfo"}})
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	rows := e.planOrder()
	require.Len(t, rows, 2)
	var shared []extra
	own := make([][]extra, len(rows))
	for i, r := range rows {
		for _, x := range decodeExtras(r.Extras) {
			if dirShared(x, false) {
				shared = mergeShared(shared, []extra{x})
			} else {
				own[i] = append(own[i], x)
			}
		}
	}
	require.NotEmpty(t, shared)
	for i, es := range [][]extra{mergeShared(own[0], shared), own[1]} {
		require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", rows[i].ID).Update("extras", encodeExtras(es)).Error)
	}
	e.tlouToBreakingBad()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Changed, res.Messages)
	_, err = os.Stat(filepath.Join(e.tv, "最后生还者 (2023)"))
	assert.ErrorIs(t, err, os.ErrNotExist, "共用的 tvshow.nfo、season.nfo 也删了，空目录删了")
}

// 另一条已整理的记录（同一份数据的辅种保存在别的目录、各自整理过）也指向旧位置的文件：那个文件留着、转给那一条；没人用的照常删。
func TestChangeItemKeepsFileOtherRecordUses(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv})
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	rows := e.history()
	require.Len(t, rows, 2)
	other := rows[0]
	other.ID = 0
	other.SourcePath = filepath.Join(e.root, "reseed", filepath.Base(rows[0].SourcePath))
	other.InfoHash, other.TaskID, other.Extras = "cccccccccccccccccccccccccccccccccccccccc", "", ""
	require.NoError(t, e.db.Create(&other).Error)
	e.tlouToBreakingBad()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Changed, res.Messages)
	assert.Contains(t, strings.Join(res.Messages, "\n"), "还被另一条整理记录")
	_, err = os.Stat(rows[0].TargetPath)
	assert.NoError(t, err, "另一条记录用着的旧文件留着")
	_, err = os.Stat(rows[1].TargetPath)
	assert.ErrorIs(t, err, os.ErrNotExist, "没人用的旧文件删了")
	var got models.MediaTransferHistory
	require.NoError(t, e.db.First(&got, other.ID).Error)
	assert.Equal(t, rows[0].Extras, got.Extras, "旧记录的字幕转给了那一条")
}

// 位置不变的换条目、媒体库关着刮削：只改记录，不报「重新刮削没成」。
func TestChangeItemInPlaceScrapeOff(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Template: "Shows/Season {{.Season}}/{{.SeasonEpisode}}"})
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	e.tlouToBreakingBad()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Changed, res.Messages)
	assert.NotContains(t, strings.Join(res.Messages, "\n"), "重新刮削没成")
	for _, r := range e.history() {
		assert.Equal(t, bbID, r.TMDBID)
	}
}

func TestSameEntry(t *testing.T) {
	dir := t.TempDir()
	show := filepath.Join(dir, "tv", "Show")
	require.NoError(t, os.MkdirAll(show, 0o755))
	f := filepath.Join(show, "S01E01.mkv")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
	assert.True(t, sameEntry(f, f))
	assert.True(t, sameEntry(f, filepath.Join(dir, "tv", "Show", ".", "S01E01.mkv")))
	other := filepath.Join(dir, "tv", "Other")
	require.NoError(t, os.MkdirAll(other, 0o755))
	link := filepath.Join(other, "S01E01.mkv")
	require.NoError(t, os.Link(f, link))
	assert.False(t, sameEntry(f, link), "两个目录里的硬链接是两个位置")
	assert.False(t, sameEntry(f, filepath.Join(show, "S01E02.mkv")))
	assert.False(t, sameEntry(f, filepath.Join(dir, "missing", "S01E01.mkv")))
	if err := os.Symlink(filepath.Join(dir, "tv"), filepath.Join(dir, "alias")); err != nil {
		t.Skipf("建不了软链接: %v", err)
	}
	assert.True(t, sameEntry(f, filepath.Join(dir, "alias", "Show", "S01E01.mkv")), "媒体库路径是软链接")
}
