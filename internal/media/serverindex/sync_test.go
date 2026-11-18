package serverindex

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/media/server"
	"github.com/sunerpy/pt-tools/models"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "t.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(MEMORY)&_pragma=synchronous(OFF)"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&models.MediaServer{}, &models.MediaServerItem{}, &models.MediaIssue{}, &models.MediaTransferHistory{}))
	return db
}

// fakeClient 是假的媒体服务器：pages 是每一页（一页一个切片）；changedAt 是哪一页报总数变了；failAt 是哪一页出错。
type fakeClient struct {
	mu        sync.Mutex
	info      server.Info
	testErr   error
	pages     [][]server.Item
	changedAt int
	failAt    int
	calls     int
	block     chan struct{}
}

func (f *fakeClient) Test(context.Context) (server.Info, error)    { return f.info, f.testErr }
func (f *fakeClient) RefreshPaths(context.Context, []string) error { return nil }
func (f *fakeClient) RefreshAll(context.Context) error             { return nil }
func (f *fakeClient) Exists(context.Context, server.Query) (bool, error) {
	return false, nil
}

func (f *fakeClient) Users(context.Context) ([]server.User, error) { return nil, server.ErrUnsupported }

func (f *fakeClient) Resume(context.Context, string, int) ([]server.ResumeItem, error) {
	return nil, server.ErrUnsupported
}

func (f *fakeClient) ListItems(ctx context.Context, cursor string) (server.ItemPage, error) {
	f.mu.Lock()
	f.calls++
	block := f.block
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return server.ItemPage{}, ctx.Err()
		}
	}
	i := 0
	if cursor != "" {
		i, _ = strconv.Atoi(cursor)
	}
	if f.failAt > 0 && i+1 == f.failAt {
		return server.ItemPage{}, fmt.Errorf("%w: HTTP 500", server.ErrUnavailable)
	}
	if i >= len(f.pages) {
		return server.ItemPage{}, nil
	}
	p := server.ItemPage{Items: f.pages[i], Changed: f.changedAt > 0 && i+1 == f.changedAt}
	if i+1 < len(f.pages) {
		p.Next = strconv.Itoa(i + 1)
	}
	return p, nil
}

var t0 = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func movie(id string, tmdb int, title, path string) server.Item {
	return server.Item{ID: id, Kind: models.MediaServerItemMovie, TMDBID: tmdb, Title: title, Path: path, AddedAt: t0.Add(-time.Hour)}
}

func show(id string, tmdb int, title string) server.Item {
	return server.Item{ID: id, Kind: models.MediaServerItemSeries, TMDBID: tmdb, IMDbID: "tt100", TVDBID: "392256", Title: title}
}

func ep(id, series string, season, episode int, path string) server.Item {
	return server.Item{ID: id, Kind: models.MediaServerItemEpisode, SeriesID: series, Season: season, Episode: episode, TMDBID: 99000 + episode, Title: "第 " + strconv.Itoa(episode) + " 集", Path: path}
}

func newServer(t *testing.T, db *gorm.DB, mut func(*models.MediaServer)) models.MediaServer {
	t.Helper()
	srv := models.MediaServer{Name: "Jellyfin", Kind: models.MediaServerJellyfin, URL: "http://jf:8096", Enabled: true, LocalPrefix: "/media", ServerPrefix: "/data"}
	if mut != nil {
		mut(&srv)
	}
	require.NoError(t, db.Create(&srv).Error)
	return srv
}

func reload(t *testing.T, db *gorm.DB, id uint) models.MediaServer {
	t.Helper()
	var srv models.MediaServer
	require.NoError(t, db.First(&srv, id).Error)
	return srv
}

func items(t *testing.T, db *gorm.DB, srvID uint) map[string]models.MediaServerItem {
	t.Helper()
	var rows []models.MediaServerItem
	require.NoError(t, db.Where("server_id = ?", srvID).Find(&rows).Error)
	out := map[string]models.MediaServerItem{}
	for _, r := range rows {
		out[r.ItemID] = r
	}
	return out
}

func issues(t *testing.T, db *gorm.DB) []models.MediaIssue {
	t.Helper()
	var rows []models.MediaIssue
	require.NoError(t, db.Order("id").Find(&rows).Error)
	return rows
}

func done(t *testing.T, db *gorm.DB, h models.MediaTransferHistory) {
	t.Helper()
	h.Status = models.MediaTransferDone
	if h.SourcePath == "" {
		h.SourcePath = "/downloads/" + filepath.Base(h.TargetPath)
	}
	require.NoError(t, db.Create(&h).Error)
}

func TestSyncWritesIndex(t *testing.T) {
	db := newDB(t)
	srv := newServer(t, db, nil)
	c := &fakeClient{info: server.Info{Name: "jf", Version: "12.2", ID: "srv-1"}, pages: [][]server.Item{
		{movie("m1", 872585, "奥本海默", "/data/movies/奥本海默 (2023)/奥本海默 (2023).mkv"), show("s1", 100088, "最后生还者")},
		{ep("e1", "s1", 1, 1, "/data/tv/最后生还者/S01E01.mkv"), ep("e2", "s1", 1, 2, "/elsewhere/S01E02.mkv"), ep("e3", "s404", 1, 3, "")},
	}}
	res, err := Sync(context.Background(), db, srv, c, t0)
	require.NoError(t, err)
	assert.Equal(t, Result{Info: c.info, Movies: 1, Series: 1, Episodes: 3, Added: 5}, res)

	got := items(t, db, srv.ID)
	require.Len(t, got, 5)
	m := got["m1"]
	assert.Equal(t, "/media/movies/奥本海默 (2023)/奥本海默 (2023).mkv", m.LocalPath, "按服务器的路径前缀换成 pt-tools 的路径")
	require.NotNil(t, m.AddedAt)
	assert.True(t, m.AddedAt.Equal(t0.Add(-time.Hour)))
	assert.True(t, m.SyncedAt.Equal(t0))
	e1 := got["e1"]
	assert.Equal(t, [4]any{100088, "tt100", "392256", "s1"}, [4]any{e1.TMDBID, e1.IMDbID, e1.TVDBID, e1.SeriesItemID}, "集的编号换成剧的")
	assert.Empty(t, got["e2"].LocalPath, "不在服务器的路径前缀里的换不了")
	assert.Equal(t, 0, got["e3"].TMDBID, "列不到所在的剧时没有编号")

	s := reload(t, db, srv.ID)
	require.NotNil(t, s.IndexedAt)
	assert.True(t, s.IndexedAt.Equal(t0))
	assert.Equal(t, [3]int{1, 1, 3}, [3]int{s.IndexMovies, s.IndexSeries, s.IndexEpisodes})
	assert.Equal(t, "srv-1", s.ServerUID)
	assert.Empty(t, s.IndexError)

	// 再同步一次，什么都没变：不写行
	res, err = Sync(context.Background(), db, s, c, t0.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 0, res.Added+res.Updated+res.Removed+res.Restored)
	assert.True(t, items(t, db, srv.ID)["m1"].SyncedAt.Equal(t0), "没变的行不重写")

	// 改了名字：更新
	c.pages[0][0].Title = "奥本海默（导演剪辑版）"
	res, err = Sync(context.Background(), db, s, c, t0.Add(2*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, res.Updated)
	assert.Equal(t, "奥本海默（导演剪辑版）", items(t, db, srv.ID)["m1"].Title)

	// 服务器里换了文件：更新路径与换成的 pt-tools 路径（删了以后按路径对整理记录）
	c.pages[0][0].Path = "/data/movies/奥本海默 (2023)/Oppenheimer.2023.2160p.mkv"
	res, err = Sync(context.Background(), db, s, c, t0.Add(3*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, res.Updated)
	assert.Equal(t, "/media/movies/奥本海默 (2023)/Oppenheimer.2023.2160p.mkv", items(t, db, srv.ID)["m1"].LocalPath)
}

func TestSyncNoPrefixKeepsPath(t *testing.T) {
	db := newDB(t)
	srv := newServer(t, db, func(s *models.MediaServer) { s.LocalPrefix, s.ServerPrefix = "", "" })
	c := &fakeClient{pages: [][]server.Item{{movie("m1", 1, "A", "/media/movies/A.mkv")}}}
	_, err := Sync(context.Background(), db, srv, c, t0)
	require.NoError(t, err)
	assert.Equal(t, "/media/movies/A.mkv", items(t, db, srv.ID)["m1"].LocalPath, "没设前缀时两边一样")
}

func TestSyncFailureWritesNothing(t *testing.T) {
	db := newDB(t)
	srv := newServer(t, db, nil)
	ok := &fakeClient{pages: [][]server.Item{{movie("m1", 1, "A", "")}, {movie("m2", 2, "B", "")}}}
	_, err := Sync(context.Background(), db, srv, ok, t0)
	require.NoError(t, err)
	srv = reload(t, db, srv.ID)

	bad := &fakeClient{pages: [][]server.Item{{movie("m1", 1, "A", "")}, {movie("m2", 2, "B", "")}}, failAt: 2}
	_, err = Sync(context.Background(), db, srv, bad, t0.Add(time.Hour))
	require.ErrorIs(t, err, server.ErrUnavailable)
	got := items(t, db, srv.ID)
	assert.Len(t, got, 2)
	assert.Nil(t, got["m2"].RemovedAt, "拉取中途失败时这一轮不写，不把没拉到的当成删了")
	s := reload(t, db, srv.ID)
	assert.Contains(t, s.IndexError, "HTTP 500")
	assert.True(t, s.IndexedAt.Equal(t0), "上一次成功的时间不变")

	// 下一次成功时清掉错误
	_, err = Sync(context.Background(), db, s, ok, t0.Add(2*time.Hour))
	require.NoError(t, err)
	assert.Empty(t, reload(t, db, srv.ID).IndexError)

	// Test 失败
	_, err = Sync(context.Background(), db, s, &fakeClient{testErr: server.ErrUnauthorized}, t0.Add(3*time.Hour))
	require.ErrorIs(t, err, server.ErrUnauthorized)
	assert.Contains(t, reload(t, db, srv.ID).IndexError, "API Key")
}

func TestSyncCanceledNotRecorded(t *testing.T) {
	db := newDB(t)
	srv := newServer(t, db, nil)
	ctx, cancel := context.WithCancel(context.Background())
	c := &fakeClient{pages: [][]server.Item{{movie("m1", 1, "A", "")}}, block: make(chan struct{})}
	errc := make(chan error, 1)
	go func() {
		_, err := Sync(ctx, db, srv, c, t0)
		errc <- err
	}()
	require.Eventually(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.calls > 0
	}, 5*time.Second, 5*time.Millisecond)
	cancel()
	require.ErrorIs(t, <-errc, context.Canceled)
	assert.Empty(t, reload(t, db, srv.ID).IndexError, "退出时被取消的不记成失败")
	assert.Empty(t, items(t, db, srv.ID))
}

func TestSyncTooMany(t *testing.T) {
	db := newDB(t)
	srv := newServer(t, db, nil)
	page := make([]server.Item, MaxItems+1)
	for i := range page {
		page[i] = movie("m"+strconv.Itoa(i), 0, "x", "")
	}
	_, err := Sync(context.Background(), db, srv, &fakeClient{pages: [][]server.Item{page}}, t0)
	require.ErrorIs(t, err, ErrTooMany)
	assert.Empty(t, items(t, db, srv.ID))
	assert.Contains(t, reload(t, db, srv.ID).IndexError, "20 万")
}

func TestSyncRemovalMarksAndIssues(t *testing.T) {
	db := newDB(t)
	srv := newServer(t, db, nil)
	// pt-tools 整理过的：电影（按路径对上）、第 1 集（按路径）、第 2 集（换不了路径，按编号对上）
	done(t, db, models.MediaTransferHistory{MediaType: models.MediaKindMovie, TMDBID: 872585, TargetPath: "/media/movies/奥本海默 (2023)/奥本海默 (2023).mkv"})
	done(t, db, models.MediaTransferHistory{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 1, Episode: 1, TargetPath: "/media/tv/最后生还者/S01E01.mkv"})
	done(t, db, models.MediaTransferHistory{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 1, Episode: 2, EpisodeEnd: 3, TargetPath: "/other/S01E02-E03.mkv"})
	all := []server.Item{
		movie("m1", 872585, "奥本海默", "/data/movies/奥本海默 (2023)/奥本海默 (2023).mkv"),
		movie("m2", 27205, "盗梦空间", "/data/movies/盗梦空间.mkv"),  // 不是 pt-tools 整理的
		movie("m3", 872585, "奥本海默", "/data/movies/别的版本.mkv"), // 同一部，但路径对不上：不是 pt-tools 的那个文件
		show("s1", 100088, "最后生还者"),
		ep("e1", "s1", 1, 1, "/data/tv/最后生还者/S01E01.mkv"),
		ep("e3", "s1", 1, 3, "/nas/S01E03.mkv"), // 换不了路径：按编号，第 2–3 集那条记录对上
	}
	c := &fakeClient{info: server.Info{ID: "u1"}, pages: [][]server.Item{all}}
	_, err := Sync(context.Background(), db, srv, c, t0)
	require.NoError(t, err)
	srv = reload(t, db, srv.ID)

	// 服务器里删了：电影两部、整部剧
	c.pages = [][]server.Item{{all[1]}}
	t1 := t0.Add(time.Hour)
	res, err := Sync(context.Background(), db, srv, c, t1)
	require.NoError(t, err)
	assert.Equal(t, 5, res.Removed)
	assert.Equal(t, 3, res.Issues, "只对 pt-tools 整理过的写：奥本海默、第 1 集、第 3 集")
	got := items(t, db, srv.ID)
	require.NotNil(t, got["m1"].RemovedAt)
	assert.True(t, got["m1"].RemovedAt.Equal(t1))
	assert.Nil(t, got["m2"].RemovedAt)
	is := issues(t, db)
	require.Len(t, is, 3)
	byItem := map[string]models.MediaIssue{}
	for _, i := range is {
		byItem[i.ServerItemID] = i
		assert.Equal(t, models.MediaIssueServerRemoved, i.Kind)
		assert.Equal(t, models.MediaIssueOpen, i.Status)
		assert.Equal(t, srv.ID, i.ServerID)
		assert.Zero(t, i.HistoryID, "server_removed 不按整理记录认")
	}
	assert.Equal(t, [4]any{models.MediaKindMovie, 872585, "奥本海默", 0}, [4]any{byItem["m1"].MediaType, byItem["m1"].TMDBID, byItem["m1"].Title, byItem["m1"].Season})
	assert.Equal(t, [5]any{models.MediaKindTV, 100088, "最后生还者", 1, 1}, [5]any{byItem["e1"].MediaType, byItem["e1"].TMDBID, byItem["e1"].Title, byItem["e1"].Season, byItem["e1"].Episode},
		"集的问题写剧名（整部剧都删了时从索引里取）")
	assert.Contains(t, byItem["e1"].Detail, "Jellyfin")
	assert.Contains(t, byItem["e1"].Detail, "/data/tv/最后生还者/S01E01.mkv")
	_, has := byItem["e3"]
	assert.True(t, has)

	// 重复同步不会多出一行，也不改状态
	res, err = Sync(context.Background(), db, srv, c, t1.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 0, res.Removed+res.Issues)
	assert.Len(t, issues(t, db), 3)

	// 用户忽略第 1 集
	d, err := Dismiss(context.Background(), db, byItem["e1"].ID, t1.Add(2*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, models.MediaIssueDismissed, d.Status)

	// 又出现了：open 与 dismissed 都改成 resolved
	t2 := t1.Add(3 * time.Hour)
	c.pages = [][]server.Item{all}
	res, err = Sync(context.Background(), db, srv, c, t2)
	require.NoError(t, err)
	assert.Equal(t, 5, res.Restored)
	for _, i := range issues(t, db) {
		assert.Equal(t, models.MediaIssueResolved, i.Status, i.ServerItemID)
		assert.Equal(t, "媒体服务器里又有了", i.Detail)
		require.NotNil(t, i.ResolvedAt)
		assert.True(t, i.ResolvedAt.Equal(t2))
	}
	assert.Nil(t, items(t, db, srv.ID)["m1"].RemovedAt)
	// 解决了的不能再忽略（忽略只对还开着的）
	d, err = Dismiss(context.Background(), db, byItem["e1"].ID, t2.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, models.MediaIssueResolved, d.Status)

	// 之后又被删：同一行改回 open
	c.pages = [][]server.Item{{all[1], all[2], all[3], all[4], all[5]}}
	_, err = Sync(context.Background(), db, srv, c, t2.Add(time.Hour))
	require.NoError(t, err)
	is = issues(t, db)
	require.Len(t, is, 3)
	for _, i := range is {
		want := models.MediaIssueResolved
		if i.ServerItemID == "m1" {
			want = models.MediaIssueOpen
		}
		assert.Equal(t, want, i.Status, i.ServerItemID)
	}
	m1 := is[0]
	for _, i := range is {
		if i.ServerItemID == "m1" {
			m1 = i
		}
	}
	assert.Nil(t, m1.ResolvedAt)
}

// 换不了路径的集按剧的 TMDB 编号与季集对整理记录：没整理过的集、别的季不算。
func TestSyncRemovedEpisodeMatchesBySeasonEpisode(t *testing.T) {
	db := newDB(t)
	srv := newServer(t, db, nil)
	done(t, db, models.MediaTransferHistory{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 1, Episode: 2, EpisodeEnd: 3, TargetPath: "/other/S01E02-E03.mkv"})
	all := []server.Item{
		show("s1", 100088, "最后生还者"),
		ep("e3", "s1", 1, 3, "/nas/S01E03.mkv"),   // 第 2–3 集那条记录对上
		ep("e4", "s1", 1, 4, "/nas/S01E04.mkv"),   // 第 4 集没整理过
		ep("s2e3", "s1", 2, 3, "/nas/S02E03.mkv"), // 别的季
	}
	c := &fakeClient{pages: [][]server.Item{all}}
	_, err := Sync(context.Background(), db, srv, c, t0)
	require.NoError(t, err)
	c.pages = [][]server.Item{{all[0]}}
	res, err := Sync(context.Background(), db, reload(t, db, srv.ID), c, t0.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 3, res.Removed)
	is := issues(t, db)
	require.Len(t, is, 1, "只有第 3 集对得上整理记录")
	assert.Equal(t, "e3", is[0].ServerItemID)
}

func TestSyncDismissedStaysWhileGone(t *testing.T) {
	db := newDB(t)
	srv := newServer(t, db, nil)
	done(t, db, models.MediaTransferHistory{MediaType: models.MediaKindMovie, TMDBID: 1, TargetPath: "/media/a.mkv"})
	c := &fakeClient{pages: [][]server.Item{{movie("m1", 1, "A", "/data/a.mkv")}}}
	_, err := Sync(context.Background(), db, srv, c, t0)
	require.NoError(t, err)
	c.pages = [][]server.Item{{}}
	_, err = Sync(context.Background(), db, srv, c, t0.Add(time.Hour))
	require.NoError(t, err)
	is := issues(t, db)
	require.Len(t, is, 1)
	_, err = Dismiss(context.Background(), db, is[0].ID, t0.Add(2*time.Hour))
	require.NoError(t, err)
	// 还是没有：dismissed 保持（Raise 不改忽略的）
	require.NoError(t, Raise(db, removedKey(srv.ID, "m1"), IssueInfo{MediaType: models.MediaKindMovie, TMDBID: 1, Title: "A"}, t0.Add(3*time.Hour)))
	assert.Equal(t, models.MediaIssueDismissed, issues(t, db)[0].Status)
	// 再忽略一次（已经不是开着的）不报错、不变
	d, err := Dismiss(context.Background(), db, is[0].ID, t0.Add(4*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, models.MediaIssueDismissed, d.Status)
	_, err = Dismiss(context.Background(), db, 9999, t0)
	require.ErrorIs(t, err, ErrIssueNotFound)
}

func TestSyncPartialSkipsRemovals(t *testing.T) {
	db := newDB(t)
	srv := newServer(t, db, nil)
	c := &fakeClient{pages: [][]server.Item{{movie("m1", 1, "A", ""), movie("m2", 2, "B", "")}}}
	_, err := Sync(context.Background(), db, srv, c, t0)
	require.NoError(t, err)
	c.pages = [][]server.Item{{movie("m1", 1, "A2", "")}, {movie("m3", 3, "C", "")}}
	c.changedAt = 2
	res, err := Sync(context.Background(), db, srv, c, t0.Add(time.Hour))
	require.NoError(t, err)
	assert.True(t, res.Partial)
	assert.Equal(t, [3]int{1, 1, 0}, [3]int{res.Added, res.Updated, res.Removed}, "照样写新增与修改，不打删除标记")
	assert.Nil(t, items(t, db, srv.ID)["m2"].RemovedAt)
}

func TestSyncPurge(t *testing.T) {
	db := newDB(t)
	srv := newServer(t, db, nil)
	done(t, db, models.MediaTransferHistory{MediaType: models.MediaKindMovie, TMDBID: 1, TargetPath: "/media/a.mkv"})
	c := &fakeClient{pages: [][]server.Item{{movie("m1", 1, "A", "/data/a.mkv"), movie("m2", 2, "B", "/data/b.mkv"), movie("m3", 3, "C", "/data/c.mkv")}}}
	_, err := Sync(context.Background(), db, srv, c, t0)
	require.NoError(t, err)
	c.pages = [][]server.Item{{movie("m3", 3, "C", "/data/c.mkv")}}
	_, err = Sync(context.Background(), db, srv, c, t0.Add(time.Hour))
	require.NoError(t, err)
	// 89 天后还在
	res, err := Sync(context.Background(), db, srv, c, t0.Add(89*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 0, res.Purged)
	// 91 天后：m2 删掉；m1 有未处理的问题，留着
	res, err = Sync(context.Background(), db, srv, c, t0.Add(91*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, res.Purged)
	got := items(t, db, srv.ID)
	assert.NotContains(t, got, "m2")
	assert.Contains(t, got, "m1")
	// 问题忽略以后再过一轮就删
	_, err = Dismiss(context.Background(), db, issues(t, db)[0].ID, t0.Add(92*24*time.Hour))
	require.NoError(t, err)
	res, err = Sync(context.Background(), db, srv, c, t0.Add(93*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, res.Purged)
	assert.NotContains(t, items(t, db, srv.ID), "m1")
}

func TestSyncResetWhenServerChanged(t *testing.T) {
	db := newDB(t)
	srv := newServer(t, db, nil)
	done(t, db, models.MediaTransferHistory{MediaType: models.MediaKindMovie, TMDBID: 1, TargetPath: "/media/a.mkv"})
	c := &fakeClient{info: server.Info{ID: "old"}, pages: [][]server.Item{{movie("m1", 1, "A", "/data/a.mkv")}}}
	_, err := Sync(context.Background(), db, srv, c, t0)
	require.NoError(t, err)
	srv = reload(t, db, srv.ID)
	// 换了一台服务器：条目编号全变了
	other := &fakeClient{info: server.Info{ID: "new"}, pages: [][]server.Item{{movie("x9", 1, "A", "/data/a.mkv")}}}
	res, err := Sync(context.Background(), db, srv, other, t0.Add(time.Hour))
	require.NoError(t, err)
	assert.True(t, res.Reset)
	assert.Equal(t, 0, res.Removed+res.Issues, "不把旧服务器的条目当成删了")
	got := items(t, db, srv.ID)
	assert.Len(t, got, 1)
	assert.Contains(t, got, "x9")
	assert.Equal(t, "new", reload(t, db, srv.ID).ServerUID)
}

func TestForget(t *testing.T) {
	db := newDB(t)
	a := newServer(t, db, nil)
	b := newServer(t, db, func(s *models.MediaServer) { s.Name = "Emby" })
	for _, srv := range []models.MediaServer{a, b} {
		require.NoError(t, db.Create(&models.MediaServerItem{ServerID: srv.ID, ItemID: "m1", Kind: models.MediaServerItemMovie, SyncedAt: t0}).Error)
		require.NoError(t, Raise(db, removedKey(srv.ID, "m1"), IssueInfo{MediaType: models.MediaKindMovie, TMDBID: 1}, t0))
	}
	require.NoError(t, Raise(db, IssueKey{Kind: models.MediaIssueLibraryMissing, HistoryID: 3}, IssueInfo{MediaType: models.MediaKindMovie, TMDBID: 1}, t0))
	require.NoError(t, Forget(db, a.ID))
	assert.Empty(t, items(t, db, a.ID))
	assert.Len(t, items(t, db, b.ID), 1)
	is := issues(t, db)
	require.Len(t, is, 2, "只删这台的 server_removed")
	assert.Equal(t, b.ID, is[0].ServerID)
	assert.Equal(t, models.MediaIssueLibraryMissing, is[1].Kind)
}

func TestLocalPath(t *testing.T) {
	srv := models.MediaServer{LocalPrefix: `D:\Media`, ServerPrefix: "/data"}
	assert.Equal(t, `D:\Media\movies\a.mkv`, LocalPath(srv, "/data/movies/a.mkv"), "跟 pt-tools 那一边的分隔符")
	assert.Empty(t, LocalPath(srv, "/data2/a.mkv"))
	assert.Empty(t, LocalPath(srv, ""))
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "ab", truncate("ab", 4))
	assert.Equal(t, "中", truncate("中文", 4), "不截断多字节字符")
	assert.Equal(t, "", truncate("中", 2))
}

func TestSyncTransactionFailure(t *testing.T) {
	db := newDB(t)
	srv := newServer(t, db, nil)
	// 表坏了：写库失败时回错误、记下原因
	require.NoError(t, db.Migrator().DropTable(&models.MediaServerItem{}))
	_, err := Sync(context.Background(), db, srv, &fakeClient{pages: [][]server.Item{{movie("m1", 1, "A", "")}}}, t0)
	require.Error(t, err)
	assert.Contains(t, reload(t, db, srv.ID).IndexError, "读取索引失败")
}
