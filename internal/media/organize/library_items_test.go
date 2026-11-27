package organize

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func libraryItemLines(p LibraryItemPage) []string {
	out := []string{}
	for _, it := range p.Items {
		out = append(out, fmt.Sprintf("%s %d S%d %s %d eps=%v files=%d size=%d %s", it.MediaType, it.TMDBID, it.Season, it.Title, it.Year,
			it.Episodes, it.Files, it.Size, it.PosterPath))
	}
	return out
}

// 媒体库的内容：只算这个库里整理好的、认出条目的记录；电影一部一项（导入的已有文件也算），剧集一季一项，
// 集号展开去重；按最近入库的时刻倒序；海报从 TMDB 取，取不到时留空不报错。
func TestLibraryItems(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	movies, tv := e.libraryID(models.MediaKindMovie), e.libraryID(models.MediaKindTV)
	other, err := e.svc.SaveLibrary(e.ctx, 0, LibraryInput{Name: "旧电影", Kind: models.MediaKindMovie, Path: e.anime})
	require.NoError(t, err)
	cst := time.FixedZone("CST", 8*3600)
	at := func(h int, loc *time.Location) time.Time { return time.Date(2026, 11, 20, h, 0, 0, 0, loc) }
	e.putRows(
		// 奥本海默：导入的已有文件两个（同一部合成一项），另一条已删除的不算
		models.MediaTransferHistory{
			LibraryID: movies, MediaType: models.MediaKindMovie, TMDBID: 872585, Title: "奥本海默", Year: 2023, Mode: models.MediaModeExisting,
			Status: models.MediaTransferDone, Size: 100, CreatedAt: at(9, cst),
		},
		models.MediaTransferHistory{
			LibraryID: movies, MediaType: models.MediaKindMovie, TMDBID: 872585, Title: "奥本海默", Year: 2023, Mode: models.MediaModeExisting,
			Status: models.MediaTransferDone, Size: 50, CreatedAt: at(8, cst),
		},
		models.MediaTransferHistory{
			LibraryID: movies, MediaType: models.MediaKindMovie, TMDBID: 872585, Title: "奥本海默", Status: models.MediaTransferRemoved, Size: 999,
			CreatedAt: at(23, time.UTC),
		},
		// 文本上「2026-11-20 03:00:00+00:00」小于「2026-11-20 09:00:00+08:00」，时刻上更晚（09:00+08:00 是 01:00Z）
		models.MediaTransferHistory{
			LibraryID: movies, MediaType: models.MediaKindMovie, TMDBID: 603, Title: "黑客帝国", Year: 1999, Mode: models.MediaModeHardlink,
			Status: models.MediaTransferDone, Size: 7, CreatedAt: at(3, time.UTC),
		},
		// 失败的、没认出条目的、别的库的都不算
		models.MediaTransferHistory{LibraryID: movies, MediaType: models.MediaKindMovie, TMDBID: 155, Title: "黑暗骑士", Status: models.MediaTransferFailed},
		models.MediaTransferHistory{LibraryID: movies, MediaType: models.MediaKindMovie, Title: "不认识", Status: models.MediaTransferDone},
		models.MediaTransferHistory{
			LibraryID: other.ID, MediaType: models.MediaKindMovie, TMDBID: 13, Title: "阿甘正传", Status: models.MediaTransferDone, CreatedAt: at(5, time.UTC),
		},
		// 剧集：第 1 季 E01、E02–E03、E05、重复的 E02（换过版本的两条都在），第 2 季 E01
		models.MediaTransferHistory{
			LibraryID: tv, MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Title: "最后生还者", Year: 2023, Season: 1, Episode: 5,
			Status: models.MediaTransferDone, Size: 5, CreatedAt: at(1, time.UTC),
		},
		models.MediaTransferHistory{
			LibraryID: tv, MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Title: "最后生还者", Year: 2023, Season: 1, Episode: 2, EpisodeEnd: 3,
			Status: models.MediaTransferDone, Size: 23, CreatedAt: at(0, time.UTC),
		},
		models.MediaTransferHistory{
			LibraryID: tv, MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Title: "The Last of Us", Year: 2023, Season: 1, Episode: 1,
			Status: models.MediaTransferDone, Size: 1, CreatedAt: at(0, time.UTC),
		},
		models.MediaTransferHistory{
			LibraryID: tv, MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Title: "最后生还者", Year: 2023, Season: 1, Episode: 2,
			Status: models.MediaTransferDone, Size: 2, CreatedAt: at(0, time.UTC),
		},
		models.MediaTransferHistory{
			LibraryID: tv, MediaType: models.MediaKindTV, TMDBID: tlouTMDB, Title: "最后生还者", Year: 2023, Season: 2, Episode: 1,
			Status: models.MediaTransferDone, Size: 9, CreatedAt: at(2, time.UTC),
		},
	)

	// TMDB 读不到时海报留空，照样列出
	e.tmdb.setDown(true)
	page, err := e.svc.LibraryItems(e.ctx, LibraryItemsQuery{LibraryID: movies})
	require.NoError(t, err)
	assert.Equal(t, int64(2), page.Total)
	assert.Equal(t, []string{
		"movie 603 S0 黑客帝国 1999 eps=[] files=1 size=7 ",
		"movie 872585 S0 奥本海默 2023 eps=[] files=2 size=150 ",
	}, libraryItemLines(page), "按时刻倒序；同一部的两个文件合成一项；已删除的不算")
	assert.True(t, page.Items[1].LatestAt.Equal(at(9, cst)), page.Items[1].LatestAt)
	e.tmdb.setDown(false)

	page, err = e.svc.LibraryItems(e.ctx, LibraryItemsQuery{LibraryID: movies})
	require.NoError(t, err)
	assert.Equal(t, "/opp.jpg", page.Items[1].PosterPath, "海报从 TMDB 取")
	assert.Empty(t, page.Items[0].PosterPath, "TMDB 上没有的留空")

	page, err = e.svc.LibraryItems(e.ctx, LibraryItemsQuery{LibraryID: tv})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"tv 100088 S2 最后生还者 2023 eps=[1] files=1 size=9 /tlou.jpg",
		"tv 100088 S1 最后生还者 2023 eps=[1 2 3 5] files=4 size=31 /tlou.jpg",
	}, libraryItemLines(page), "一季一项；E02–E03 展开，重复的集只算一次；文件数按记录")

	// 关键字只决定哪些组命中：组里有一条记录的标题含关键字就整组列出，文件数、大小、集号仍按整组算
	page, err = e.svc.LibraryItems(e.ctx, LibraryItemsQuery{LibraryID: tv, Keyword: "Last of"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	assert.Equal(t, []string{"tv 100088 S1 最后生还者 2023 eps=[1 2 3 5] files=4 size=31 /tlou.jpg"}, libraryItemLines(page))
	page, err = e.svc.LibraryItems(e.ctx, LibraryItemsQuery{LibraryID: movies, Keyword: "100%"})
	require.NoError(t, err)
	assert.Empty(t, page.Items, "% 按字面")
	assert.Equal(t, int64(0), page.Total)

	// 分页
	page, err = e.svc.LibraryItems(e.ctx, LibraryItemsQuery{LibraryID: tv, Limit: 1, Offset: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(2), page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, 1, page.Items[0].Season)

	// 停用的库照样能看；不存在的库、关键字太长
	page, err = e.svc.LibraryItems(e.ctx, LibraryItemsQuery{LibraryID: other.ID})
	require.NoError(t, err)
	assert.Len(t, page.Items, 1)
	_, err = e.svc.LibraryItems(e.ctx, LibraryItemsQuery{LibraryID: 9999})
	require.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.LibraryItems(e.ctx, LibraryItemsQuery{})
	require.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.LibraryItems(e.ctx, LibraryItemsQuery{LibraryID: movies, Keyword: string(make([]byte, 201))})
	require.ErrorIs(t, err, ErrInvalid)
}

// 记录里没有标题的条目：显示时用 TMDB 的标题补上，但按补上的标题搜不到（只按整理时记下的片名搜索）
func TestLibraryItemsTitleFromTMDB(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	movies := e.libraryID(models.MediaKindMovie)
	e.putRows(models.MediaTransferHistory{
		LibraryID: movies, MediaType: models.MediaKindMovie, TMDBID: 872585, Status: models.MediaTransferDone, Size: 3, CreatedAt: e.Now(),
	})
	page, err := e.svc.LibraryItems(e.ctx, LibraryItemsQuery{LibraryID: movies})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "奥本海默", page.Items[0].Title)
	assert.Equal(t, "/opp.jpg", page.Items[0].PosterPath)
	page, err = e.svc.LibraryItems(e.ctx, LibraryItemsQuery{LibraryID: movies, Keyword: "奥本海默"})
	require.NoError(t, err)
	assert.Empty(t, page.Items)
}

// 集号：展开 episode..episode_end，去重升序；范围异常大的只算 episode
func TestLibraryEpisodes(t *testing.T) {
	assert.Equal(t, []int{}, libraryEpisodes(""))
	assert.Equal(t, []int{1, 2, 3, 5}, libraryEpisodes("5-0,2-3,1-0,2-0"))
	assert.Equal(t, []int{7}, libraryEpisodes("7-90000"))
	assert.Equal(t, []int{4}, libraryEpisodes("4-2"))
	assert.Equal(t, []int{}, libraryEpisodes("0-0,x-1,-3-0"))
}
