package serverindex

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// seed 建三台服务器：jf（开着、同步过、算作已入库）、emby（开着、同步过）、off（关着的）、noidx（索引关着），各放几项。
func seed(t *testing.T, db *gorm.DB) (jf, emby models.MediaServer) {
	t.Helper()
	at := t0
	jf = newServer(t, db, func(s *models.MediaServer) {
		s.PlayURL, s.ServerUID, s.IndexedAt, s.CountLibrary = "https://jf.example.com", "uid1", &at, true
	})
	emby = newServer(t, db, func(s *models.MediaServer) {
		s.Name, s.Kind, s.URL, s.IndexedAt = "Emby", models.MediaServerEmby, "http://emby:8096", &at
	})
	off := newServer(t, db, func(s *models.MediaServer) { s.Name, s.IndexedAt = "关着的", &at })
	require.NoError(t, db.Model(&off).Update("enabled", false).Error)
	noidx := newServer(t, db, func(s *models.MediaServer) { s.Name, s.IndexedAt, s.IndexOff = "索引关着", &at, true })
	never := newServer(t, db, func(s *models.MediaServer) { s.Name = "没同步过" })
	gone := t0.Add(-time.Hour)
	rows := []models.MediaServerItem{
		{ServerID: jf.ID, ItemID: "s1", Kind: models.MediaServerItemSeries, TMDBID: 100088, Title: "最后生还者"},
		{ServerID: jf.ID, ItemID: "e1", Kind: models.MediaServerItemEpisode, SeriesItemID: "s1", TMDBID: 100088, Season: 1, Episode: 1},
		{ServerID: jf.ID, ItemID: "e2", Kind: models.MediaServerItemEpisode, SeriesItemID: "s1", TMDBID: 100088, Season: 1, Episode: 2, EpisodeEnd: 3},
		{ServerID: jf.ID, ItemID: "e9", Kind: models.MediaServerItemEpisode, SeriesItemID: "s1", TMDBID: 100088, Season: 1, Episode: 9, RemovedAt: &gone},
		{ServerID: jf.ID, ItemID: "x1", Kind: models.MediaServerItemEpisode, SeriesItemID: "s1", TMDBID: 100088, Season: 0, Episode: 1},
		{ServerID: jf.ID, ItemID: "e2b", Kind: models.MediaServerItemEpisode, SeriesItemID: "s1", TMDBID: 100088, Season: 1, Episode: 2},
		{ServerID: emby.ID, ItemID: "es1", Kind: models.MediaServerItemSeries, TMDBID: 100088},
		{ServerID: emby.ID, ItemID: "ee1", Kind: models.MediaServerItemEpisode, SeriesItemID: "es1", TMDBID: 100088, Season: 1, Episode: 1},
		{ServerID: emby.ID, ItemID: "ee4", Kind: models.MediaServerItemEpisode, SeriesItemID: "es1", TMDBID: 100088, Season: 1, Episode: 4},
		{ServerID: emby.ID, ItemID: "em1", Kind: models.MediaServerItemMovie, TMDBID: 872585},
		{ServerID: off.ID, ItemID: "o1", Kind: models.MediaServerItemMovie, TMDBID: 872585},
		{ServerID: noidx.ID, ItemID: "n1", Kind: models.MediaServerItemMovie, TMDBID: 872585},
		{ServerID: never.ID, ItemID: "v1", Kind: models.MediaServerItemMovie, TMDBID: 872585},
	}
	for i := range rows {
		rows[i].SyncedAt = t0
	}
	require.NoError(t, db.Create(&rows).Error)
	return jf, emby
}

func TestServers(t *testing.T) {
	db := newDB(t)
	jf, emby := seed(t, db)
	all, err := Servers(context.Background(), db, false)
	require.NoError(t, err)
	require.Len(t, all, 2, "关着的、索引关着的、没同步过的都不算")
	assert.Equal(t, []uint{jf.ID, emby.ID}, []uint{all[0].ID, all[1].ID})
	counting, err := Servers(context.Background(), db, true)
	require.NoError(t, err)
	require.Len(t, counting, 1)
	assert.Equal(t, jf.ID, counting[0].ID)
}

func TestLookup(t *testing.T) {
	db := newDB(t)
	jf, emby := seed(t, db)
	ctx := context.Background()
	got, err := Lookup(ctx, db, models.MediaKindTV, 100088)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, jf.ID, got[0].Server.ID)
	assert.Equal(t, "s1", got[0].ItemID)
	assert.Equal(t, []Episode{
		{ItemID: "x1", Season: 0, Episode: 1}, {ItemID: "e1", Season: 1, Episode: 1}, {ItemID: "e2", Season: 1, Episode: 2, EpisodeEnd: 3}, {ItemID: "e2b", Season: 1, Episode: 2},
	}, got[0].Episodes, "删了的（e9）不算")
	assert.Equal(t, 4, EpisodeCount(got[0].Episodes), "第 1 季三集（同一集两个文件算一集）加特别篇一集")
	assert.Equal(t, [][2]int{{1, 3}, {0, 1}}, SeasonsOf(got[0].Episodes), "特别篇排最后")
	assert.Equal(t, "es1", got[1].ItemID)
	assert.Len(t, got[1].Episodes, 2)

	movies, err := Lookup(ctx, db, models.MediaKindMovie, 872585)
	require.NoError(t, err)
	require.Len(t, movies, 2)
	assert.Empty(t, movies[0].ItemID, "jf 里没有这部电影")
	assert.Equal(t, "em1", movies[1].ItemID)
	assert.Equal(t, emby.ID, movies[1].Server.ID)

	none, err := Lookup(ctx, db, models.MediaKindMovie, 0)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestSeasonEpisodes(t *testing.T) {
	db := newDB(t)
	jf, emby := seed(t, db)
	ctx := context.Background()
	got, err := SeasonEpisodes(ctx, db, 100088, 1, false)
	require.NoError(t, err)
	assert.Equal(t, map[int]SeasonHit{
		1: {Server: got[1].Server, ItemID: "e1"}, 2: {Server: got[2].Server, ItemID: "e2"}, 3: {Server: got[3].Server, ItemID: "e2"},
		4: {Server: got[4].Server, ItemID: "ee4"},
	}, got, "一个文件两集都算；同一集取编号小的服务器")
	assert.Equal(t, jf.ID, got[1].Server.ID)
	assert.Equal(t, emby.ID, got[4].Server.ID)
	assert.Equal(t, "1:e2", Ref(got[2]))

	counted, err := SeasonEpisodes(ctx, db, 100088, 1, true)
	require.NoError(t, err)
	assert.Len(t, counted, 3, "只看开了算作已入库的服务器")
	assert.NotContains(t, counted, 4)

	empty, err := SeasonEpisodes(ctx, db, 100088, 5, false)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestCountedEpisodes(t *testing.T) {
	db := newDB(t)
	seed(t, db)
	got, err := CountedEpisodes(context.Background(), db, []int{100088, 1})
	require.NoError(t, err)
	assert.Equal(t, map[EpisodeKey]bool{{100088, 1, 1}: true, {100088, 1, 2}: true, {100088, 1, 3}: true, {100088, 0, 1}: true}, got)
	empty, err := CountedEpisodes(context.Background(), db, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestSeriesTMDBAndOpenURL(t *testing.T) {
	db := newDB(t)
	jf, emby := seed(t, db)
	id, err := SeriesTMDB(context.Background(), db, jf.ID, "s1")
	require.NoError(t, err)
	assert.Equal(t, 100088, id)
	id, err = SeriesTMDB(context.Background(), db, jf.ID, "e1")
	require.NoError(t, err)
	assert.Zero(t, id, "不是剧")
	assert.Equal(t, "https://jf.example.com/web/#/details?id=s1&serverId=uid1", OpenURL(jf, "s1"), "用打开链接用的地址")
	assert.Equal(t, "http://emby:8096/web/index.html#!/item?id=em1", OpenURL(emby, "em1"), "没填时用服务器地址")
}

func TestIssueQueries(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	require.NoError(t, Raise(db, removedKey(1, "e3"), IssueInfo{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 1, Episode: 3}, t0))
	require.NoError(t, Raise(db, removedKey(1, "e1"), IssueInfo{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 1, Episode: 1}, t0.Add(time.Minute)))
	require.NoError(t, Raise(db, removedKey(1, "m1"), IssueInfo{MediaType: models.MediaKindMovie, TMDBID: 100088}, t0.Add(2*time.Minute)))
	require.NoError(t, Raise(db, IssueKey{Kind: models.MediaIssueLibraryMissing, HistoryID: 7}, IssueInfo{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 1, Episode: 2}, t0))
	require.NoError(t, Clear(db, removedKey(1, "e3"), "又有了", t0.Add(3*time.Minute)))

	got, err := ItemIssues(ctx, db, models.MediaKindTV, 100088)
	require.NoError(t, err)
	require.Len(t, got, 1, "只要还开着的 server_removed；电影与 library_missing 不算")
	assert.Equal(t, "e1", got[0].ServerItemID)
	none, err := ItemIssues(ctx, db, models.MediaKindTV, 0)
	require.NoError(t, err)
	assert.Empty(t, none)

	all, err := Issues(ctx, db, IssueFilter{})
	require.NoError(t, err)
	require.Len(t, all, 4)
	assert.Equal(t, "e3", all[0].ServerItemID, "最近改过的在前")
	open, err := Issues(ctx, db, IssueFilter{Status: models.MediaIssueOpen, Kind: models.MediaIssueServerRemoved, MediaType: models.MediaKindTV, TMDBID: 100088, Limit: 10})
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, "e1", open[0].ServerItemID)
}

func TestQueriesReportDBErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("索引表没了", func(t *testing.T) {
		db := newDB(t)
		seed(t, db)
		require.NoError(t, db.Migrator().DropTable(&models.MediaServerItem{}))
		_, err := Lookup(ctx, db, models.MediaKindTV, 100088)
		require.ErrorContains(t, err, "读取索引失败")
		_, err = SeasonEpisodes(ctx, db, 100088, 1, false)
		require.ErrorContains(t, err, "读取索引失败")
		_, err = CountedEpisodes(ctx, db, []int{100088})
		require.ErrorContains(t, err, "读取索引失败")
		_, err = SeriesTMDB(ctx, db, 1, "s1")
		require.ErrorContains(t, err, "读取索引失败")
		require.ErrorContains(t, Forget(db, 1), "清掉索引失败")
	})
	t.Run("服务器表与问题表没了", func(t *testing.T) {
		db := newDB(t)
		require.NoError(t, db.Migrator().DropTable(&models.MediaServer{}, &models.MediaIssue{}))
		_, err := Servers(ctx, db, false)
		require.ErrorContains(t, err, "读取媒体服务器失败")
		_, err = Lookup(ctx, db, models.MediaKindTV, 1)
		require.Error(t, err)
		_, err = SeasonEpisodes(ctx, db, 1, 1, false)
		require.Error(t, err)
		_, err = CountedEpisodes(ctx, db, []int{1})
		require.Error(t, err)
		_, err = ItemIssues(ctx, db, models.MediaKindTV, 1)
		require.ErrorContains(t, err, "读取问题失败")
		_, err = Issues(ctx, db, IssueFilter{})
		require.ErrorContains(t, err, "读取问题失败")
		require.ErrorContains(t, Raise(db, removedKey(1, "x"), IssueInfo{}, t0), "记下问题失败")
		require.ErrorContains(t, Clear(db, removedKey(1, "x"), "", t0), "记下问题解决失败")
		_, err = Dismiss(ctx, db, 1, t0)
		require.ErrorContains(t, err, "忽略问题失败")
	})
}
