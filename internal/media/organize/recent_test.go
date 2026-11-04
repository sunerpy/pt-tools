package organize

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// 最近入库：已整理的记录按条目与季合并（写最新那一集），最新在前；质量按种子名解析；海报从 TMDB 取；
// 服务器里新加、不是 pt-tools 整理的并进来；今天入库数只算今天整理的。
func TestRecent(t *testing.T) {
	e := newEnv(t)
	now := e.Now()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	e.putRows(
		models.MediaTransferHistory{
			MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Title: "最后生还者", Season: 1, Episode: 1, Status: models.MediaTransferDone,
			TorrentName: "The.Last.of.Us.S01E01.2160p.WEB-DL.H265-HHWEB", CreatedAt: ago(3 * time.Hour),
		},
		models.MediaTransferHistory{
			MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Title: "最后生还者", Season: 1, Episode: 2, EpisodeEnd: 3, Status: models.MediaTransferDone,
			TorrentName: "The.Last.of.Us.S01E02-E03.2160p.WEB-DL.H265-HHWEB", CreatedAt: ago(time.Hour),
		},
		models.MediaTransferHistory{
			MediaType: models.MediaKindMovie, TMDBID: 872585, Status: models.MediaTransferDone,
			SourcePath: "/dl/Oppenheimer.2023.BluRay.1080p.REMUX.AVC.DTS-HD.MA.5.1-QA.mkv", CreatedAt: ago(2 * time.Hour),
		},
		models.MediaTransferHistory{MediaType: models.MediaKindMovie, TMDBID: 872585, Status: models.MediaTransferFailed, CreatedAt: ago(time.Minute)},
		models.MediaTransferHistory{MediaType: models.MediaKindMovie, Status: models.MediaTransferDone, CreatedAt: ago(2 * time.Minute)},
		models.MediaTransferHistory{MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Season: 2, Episode: 1, Status: models.MediaTransferDone, CreatedAt: ago(48 * time.Hour)},
	)
	at := ago(30 * time.Minute)
	srv := models.MediaServer{Name: "Jellyfin", Kind: models.MediaServerJellyfin, URL: "http://jf", Enabled: true, IndexedAt: &at}
	require.NoError(t, e.db.Create(&srv).Error)
	require.NoError(t, e.db.Create(&models.MediaServerItem{
		ServerID: srv.ID, ItemID: "m1", Kind: models.MediaServerItemMovie, TMDBID: 603, Title: "黑客帝国", AddedAt: &at, SyncedAt: at,
	}).Error)
	// pt-tools 整理进去的那部在服务器里也有：只列整理记录那一项；服务器里同一季新加的别的集（不是 pt-tools 整理的）也合进那一项
	newer := ago(10 * time.Minute)
	require.NoError(t, e.db.Create(&[]models.MediaServerItem{
		{ServerID: srv.ID, ItemID: "m2", Kind: models.MediaServerItemMovie, TMDBID: 872585, Title: "奥本海默", AddedAt: &newer, SyncedAt: newer},
		{ServerID: srv.ID, ItemID: "s1", Kind: models.MediaServerItemSeries, TMDBID: tlouTMDB, Title: "最后生还者", SyncedAt: newer},
		{ServerID: srv.ID, ItemID: "e5", Kind: models.MediaServerItemEpisode, TMDBID: tlouTMDB, Season: 1, Episode: 5, SeriesItemID: "s1", AddedAt: &newer, SyncedAt: newer},
	}).Error)

	got, err := e.svc.Recent(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(4), got.TodayCount, "今天整理的 4 条（两天前的不算；失败的不算）")
	lines := []string{}
	for _, it := range got.Items {
		lines = append(lines, fmt.Sprintf("%s %s %d S%dE%d %s %s %s", it.Source, it.MediaType, it.TMDBID, it.Season, it.Episode, it.Title, it.Quality, it.PosterPath))
	}
	assert.Equal(t, []string{
		"server movie 603 S0E0 黑客帝国  ",
		"pt-tools tv 100088 S1E3 最后生还者 2160p /tlou.jpg",
		"pt-tools movie 872585 S0E0 奥本海默 Remux /opp.jpg",
		"pt-tools tv 100088 S2E1 最后生还者  /tlou.jpg",
	}, lines, "标题没记下时用 TMDB 的；TMDB 上没有的留着服务器里的名字；一条记录有几集时写最后一集")

	// 最多 12 项
	for i := range 15 {
		e.putRows(models.MediaTransferHistory{MediaType: models.MediaKindMovie, TMDBID: 9000 + i, Title: fmt.Sprint("电影", i), Status: models.MediaTransferDone, CreatedAt: ago(time.Minute)})
	}
	got, err = e.svc.Recent(e.ctx)
	require.NoError(t, err)
	assert.Len(t, got.Items, recentLimit)

	// 索引读不到时照样列整理记录
	require.NoError(t, e.db.Migrator().DropTable(&models.MediaServerItem{}))
	got, err = e.svc.Recent(e.ctx)
	require.NoError(t, err)
	assert.Len(t, got.Items, recentLimit)
	require.NoError(t, e.db.Migrator().DropTable(&models.MediaTransferHistory{}))
	_, err = e.svc.Recent(e.ctx)
	require.ErrorContains(t, err, "读取整理记录失败")
}
