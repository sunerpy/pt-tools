package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// listAll 把 ListItems 一页页列到底，返回全部项、页数与是不是有一页报了变了。
func listAll(t *testing.T, c Client) ([]Item, int, bool) {
	t.Helper()
	var all []Item
	cursor, pages, changed := "", 0, false
	for {
		page, err := c.ListItems(context.Background(), cursor)
		require.NoError(t, err)
		pages++
		all = append(all, page.Items...)
		changed = changed || page.Changed
		if page.Next == "" {
			return all, pages, changed
		}
		require.Less(t, pages, 50, "分页没有结束")
		cursor = page.Next
	}
}

// embyListing 是 /Items 的回放：items 是按 StartIndex 切的全部项；total 为空时总数是 len(items)，否则按第几次请求给总数。
type embyListing struct {
	items []string
	total func(call int) int
	calls int
}

func (l *embyListing) handle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	start, _ := strconv.Atoi(q.Get("StartIndex"))
	limit, _ := strconv.Atoi(q.Get("Limit"))
	l.calls++
	total := len(l.items)
	if l.total != nil {
		total = l.total(l.calls)
	}
	end := min(start+limit, len(l.items))
	var page []string
	if start < len(l.items) {
		page = l.items[start:end]
	}
	_, _ = fmt.Fprintf(w, `{"Items":[%s],"TotalRecordCount":%d,"StartIndex":%d}`, strings.Join(page, ","), total, start)
}

func TestEmbyListItems(t *testing.T) {
	items := []string{
		`{"Id":"m1","Name":"奥本海默","Type":"Movie","ProductionYear":2023,"Path":"/data/movies/奥本海默 (2023)/奥本海默 (2023).mkv","DateCreated":"2026-10-01T08:00:00.0000000Z","ProviderIds":{"Tmdb":"872585","Imdb":"tt15398776"}}`,
		`{"Id":"s1","Name":"最后生还者","Type":"Series","ProductionYear":2023,"Path":"/data/tv/最后生还者 (2023)","ProviderIds":{"Tmdb":"100088","Tvdb":"392256"}}`,
		`{"Id":"e1","Name":"当你迷失在黑暗中","Type":"Episode","SeriesId":"s1","SeriesName":"最后生还者","ParentIndexNumber":1,"IndexNumber":1,"Path":"/data/tv/最后生还者 (2023)/Season 01/S01E01.mkv","DateCreated":"2026-10-02T09:30:00.1234567+08:00","ProviderIds":{"Tmdb":"2181581"}}`,
		`{"Id":"e2","Name":"两集合一","Type":"Episode","SeriesId":"s1","ParentIndexNumber":1,"IndexNumber":2,"IndexNumberEnd":3,"Path":"/data/tv/最后生还者 (2023)/Season 01/S01E02-E03.mkv"}`,
		`{"Id":"v1","Name":"缺的一集","Type":"Episode","SeriesId":"s1","ParentIndexNumber":1,"IndexNumber":4,"LocationType":"Virtual"}`,
		`{"Id":"b1","Name":"合集","Type":"BoxSet"}`,
	}
	l := &embyListing{items: items}
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		l.handle(w, r)
	})
	c, err := New(Config{Kind: models.MediaServerEmby, URL: f.URL, Token: "k"})
	require.NoError(t, err)
	got, pages, changed := listAll(t, c)
	assert.Equal(t, 1, pages)
	assert.False(t, changed)
	require.Len(t, got, 4, "缺集占位（Virtual）与合集不列")
	assert.Equal(t, Item{
		ID: "m1", Kind: models.MediaServerItemMovie, TMDBID: 872585, IMDbID: "tt15398776", Title: "奥本海默", Year: 2023,
		Path: "/data/movies/奥本海默 (2023)/奥本海默 (2023).mkv", AddedAt: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC),
	}, got[0])
	assert.Equal(t, Item{ID: "s1", Kind: models.MediaServerItemSeries, TMDBID: 100088, TVDBID: "392256", Title: "最后生还者", Year: 2023, Path: "/data/tv/最后生还者 (2023)"}, got[1])
	assert.Equal(t, models.MediaServerItemEpisode, got[2].Kind)
	assert.Equal(t, "s1", got[2].SeriesID)
	assert.Equal(t, [3]int{1, 1, 0}, [3]int{got[2].Season, got[2].Episode, got[2].EpisodeEnd})
	assert.Equal(t, 2181581, got[2].TMDBID, "集的编号是这一集的（索引里换成剧的）")
	assert.Equal(t, time.Date(2026, 10, 2, 1, 30, 0, 123456700, time.UTC), got[2].AddedAt, "七位小数与时区都认，换成 UTC")
	assert.Equal(t, [3]int{1, 2, 3}, [3]int{got[3].Season, got[3].Episode, got[3].EpisodeEnd})

	q, err := url.ParseQuery(f.requests()[0].Query)
	require.NoError(t, err)
	assert.Equal(t, "/Items", f.requests()[0].Path)
	for k, v := range map[string]string{
		"Recursive": "true", "IncludeItemTypes": "Movie,Series,Episode", "Fields": "ProviderIds,Path,DateCreated",
		"IsMissing": "false", "ExcludeLocationTypes": "Virtual", "SortBy": "DateCreated,SortName", "SortOrder": "Ascending",
		"StartIndex": "0", "Limit": "500", "EnableImages": "false", "EnableUserData": "false",
	} {
		assert.Equal(t, v, q.Get(k), k)
	}
}

func TestEmbyListItemsPaging(t *testing.T) {
	mk := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf(`{"Id":"m%d","Name":"电影 %d","Type":"Movie"}`, i, i)
		}
		return out
	}
	t.Run("三页", func(t *testing.T) {
		l := &embyListing{items: mk(1200)}
		f := newFake(t, l.handle)
		c, _ := New(Config{Kind: models.MediaServerJellyfin, URL: f.URL, Token: "k"})
		got, pages, changed := listAll(t, c)
		assert.Equal(t, 3, pages)
		assert.Len(t, got, 1200)
		assert.False(t, changed)
		var starts []string
		for _, r := range f.requests() {
			q, _ := url.ParseQuery(r.Query)
			starts = append(starts, q.Get("StartIndex"))
		}
		assert.Equal(t, []string{"0", "500", "1000"}, starts)
	})
	t.Run("列的过程中总数变了", func(t *testing.T) {
		l := &embyListing{items: mk(1200), total: func(call int) int { return 1200 + call }}
		f := newFake(t, l.handle)
		c, _ := New(Config{Kind: models.MediaServerJellyfin, URL: f.URL, Token: "k"})
		_, _, changed := listAll(t, c)
		assert.True(t, changed, "第二页报的总数和第一页不一样")
	})
	t.Run("列的过程中少了项", func(t *testing.T) {
		l := &embyListing{items: mk(1200), total: func(call int) int {
			if call == 1 {
				return 1201
			}
			return 1200
		}}
		f := newFake(t, l.handle)
		c, _ := New(Config{Kind: models.MediaServerJellyfin, URL: f.URL, Token: "k"})
		got, pages, changed := listAll(t, c)
		assert.Equal(t, 3, pages)
		assert.Len(t, got, 1200)
		assert.True(t, changed, "后面几页报的总数比第一页少，最后一页不是空的")
	})
	t.Run("还没到总数就是空页", func(t *testing.T) {
		l := &embyListing{items: mk(500), total: func(int) int { return 600 }}
		f := newFake(t, l.handle)
		c, _ := New(Config{Kind: models.MediaServerJellyfin, URL: f.URL, Token: "k"})
		got, pages, changed := listAll(t, c)
		assert.Equal(t, 2, pages)
		assert.Len(t, got, 500)
		assert.True(t, changed, "少了项，当作变了")
	})
	t.Run("一项也没有", func(t *testing.T) {
		l := &embyListing{}
		f := newFake(t, l.handle)
		c, _ := New(Config{Kind: models.MediaServerEmby, URL: f.URL, Token: "k"})
		got, pages, changed := listAll(t, c)
		assert.Equal(t, 1, pages)
		assert.Empty(t, got)
		assert.False(t, changed)
	})
	t.Run("位置写错了", func(t *testing.T) {
		f := newFake(t, (&embyListing{}).handle)
		c, _ := New(Config{Kind: models.MediaServerEmby, URL: f.URL, Token: "k"})
		for _, bad := range []string{"x", "1", "1:x", "-1:3"} {
			_, err := c.ListItems(context.Background(), bad)
			require.ErrorIs(t, err, ErrBadConfig, bad)
		}
	})
}

func TestEmbyUsersAndResume(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Users":
			_, _ = w.Write([]byte(`[{"Id":"u1","Name":"admin"},{"Id":"","Name":"坏的"},{"Id":"u2","Name":"家人"}]`))
		case "/Users/u1/Items/Resume":
			_, _ = w.Write([]byte(`{"Items":[
				{"Id":"e9","Name":"失踪者","Type":"Episode","SeriesId":"s1","SeriesName":"长夜将尽","ParentIndexNumber":2,"IndexNumber":7,"RunTimeTicks":27000000000,"UserData":{"PlayedPercentage":40.5,"PlaybackPositionTicks":9000000000},"ProviderIds":{"Tmdb":"5555"}},
				{"Id":"m1","Name":"沙丘2","Type":"Movie","ProductionYear":2024,"RunTimeTicks":0,"UserData":{"PlayedPercentage":12},"ProviderIds":{"Tmdb":"693134","Imdb":"tt15239678"}},
				{"Id":"a1","Name":"一首歌","Type":"Audio"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	c, _ := New(Config{Kind: models.MediaServerJellyfin, URL: f.URL, Token: "k"})
	users, err := c.Users(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []User{{ID: "u1", Name: "admin"}, {ID: "u2", Name: "家人"}}, users)

	items, err := c.Resume(context.Background(), "u1", 12)
	require.NoError(t, err)
	require.Len(t, items, 2, "不是电影与集的不要")
	assert.Equal(t, ResumeItem{
		ID: "e9", Kind: models.MediaServerItemEpisode, SeriesID: "s1", SeriesName: "长夜将尽", Title: "失踪者", Season: 2, Episode: 7,
		TMDBID: 5555, Percent: 40.5, RemainingSeconds: 1800,
	}, items[0])
	assert.Equal(t, ResumeItem{ID: "m1", Kind: models.MediaServerItemMovie, Title: "沙丘2", Year: 2024, TMDBID: 693134, IMDbID: "tt15239678", Percent: 12}, items[1],
		"不知道片长时剩余是 0")
	reqs := f.requests()
	q, _ := url.ParseQuery(reqs[len(reqs)-1].Query)
	assert.Equal(t, "12", q.Get("Limit"))
	assert.Equal(t, "ProviderIds", q.Get("Fields"))
	assert.Equal(t, "true", q.Get("EnableUserData"))

	_, err = c.Resume(context.Background(), " ", 12)
	require.ErrorIs(t, err, ErrBadConfig)
	_, err = c.Resume(context.Background(), "u404", 12)
	require.ErrorIs(t, err, ErrUnavailable)
}

func fakePlexLibrary(t *testing.T, sections string, pages map[string][]string, noTotal bool) *fakeServer {
	return newFake(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`{"MediaContainer":{"friendlyName":"NAS Plex","version":"1.41.0","machineIdentifier":"abc123"}}`))
			return
		case "/library/sections":
			_, _ = w.Write([]byte(sections))
			return
		}
		q := r.URL.Query()
		key := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/library/sections/"), "/all") + "/" + q.Get("type")
		all, ok := pages[key]
		if !ok || q.Get("includeGuids") != "1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		start, _ := strconv.Atoi(q.Get("X-Plex-Container-Start"))
		size, _ := strconv.Atoi(q.Get("X-Plex-Container-Size"))
		end := min(start+size, len(all))
		var page []string
		if start < len(all) {
			page = all[start:end]
		}
		total := fmt.Sprintf(`,"totalSize":%d`, len(all))
		if noTotal {
			total = ""
		}
		_, _ = fmt.Fprintf(w, `{"MediaContainer":{"size":%d%s,"offset":%d,"Metadata":[%s]}}`, len(page), total, start, strings.Join(page, ","))
	})
}

const plexSections = `{"MediaContainer":{"Directory":[
	{"key":"1","type":"movie","title":"电影"},
	{"key":"7","type":"artist","title":"音乐"},
	{"key":"2","type":"show","title":"剧集"}]}}`

func TestPlexListItems(t *testing.T) {
	pages := map[string][]string{
		"1/1": {
			`{"ratingKey":"101","type":"movie","title":"奥本海默","year":2023,"addedAt":1790000000,"Guid":[{"id":"imdb://tt15398776"},{"id":"tmdb://872585"}],"Media":[{"Part":[{"file":"/data/movies/奥本海默 (2023)/奥本海默 (2023).mkv"}]}]}`,
			`{"ratingKey":"","type":"movie","title":"没有编号"}`,
		},
		"2/2": {`{"ratingKey":"201","type":"show","title":"最后生还者","year":2023,"Guid":[{"id":"tmdb://100088"},{"id":"tvdb://392256"}]}`},
		"2/4": {`{"ratingKey":"301","type":"episode","title":"当你迷失在黑暗中","grandparentRatingKey":"201","parentIndex":1,"index":1,"addedAt":1790000100,"Guid":[{"id":"tmdb://2181581"}],"Media":[{"Part":[{"file":"/data/tv/最后生还者 (2023)/Season 01/S01E01.mkv"}]}]}`},
	}
	f := fakePlexLibrary(t, plexSections, pages, false)
	c, err := New(Config{Kind: models.MediaServerPlex, URL: f.URL, Token: "t"})
	require.NoError(t, err)
	info, err := c.Test(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "abc123", info.ID)

	got, pages2, changed := listAll(t, c)
	assert.Equal(t, 3, pages2, "电影分区一次，剧集分区剧与集各一次；音乐分区不列")
	assert.False(t, changed)
	require.Len(t, got, 3)
	assert.Equal(t, Item{
		ID: "101", Kind: models.MediaServerItemMovie, TMDBID: 872585, IMDbID: "tt15398776", Title: "奥本海默", Year: 2023,
		Path: "/data/movies/奥本海默 (2023)/奥本海默 (2023).mkv", AddedAt: time.Unix(1790000000, 0).UTC(),
	}, got[0])
	assert.Equal(t, Item{ID: "201", Kind: models.MediaServerItemSeries, TMDBID: 100088, TVDBID: "392256", Title: "最后生还者", Year: 2023}, got[1])
	assert.Equal(t, Item{
		ID: "301", Kind: models.MediaServerItemEpisode, SeriesID: "201", Season: 1, Episode: 1, TMDBID: 2181581, Title: "当你迷失在黑暗中",
		Path: "/data/tv/最后生还者 (2023)/Season 01/S01E01.mkv", AddedAt: time.Unix(1790000100, 0).UTC(),
	}, got[2])
}

func TestPlexListItemsPaging(t *testing.T) {
	mk := func(n int, typ string) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf(`{"ratingKey":"%s%d","type":"%s","title":"第 %d 个"}`, typ[:1], i, typ, i)
		}
		return out
	}
	t.Run("一个分区几页", func(t *testing.T) {
		f := fakePlexLibrary(t, plexSections, map[string][]string{"1/1": mk(1001, "movie"), "2/2": nil, "2/4": mk(3, "episode")}, false)
		c, _ := New(Config{Kind: models.MediaServerPlex, URL: f.URL, Token: "t"})
		got, pages, changed := listAll(t, c)
		assert.Len(t, got, 1004)
		assert.Equal(t, 5, pages, "电影三页、剧一页（空的）、集一页")
		assert.False(t, changed)
	})
	t.Run("没报总数时满一页再看一页", func(t *testing.T) {
		f := fakePlexLibrary(t, plexSections, map[string][]string{"1/1": mk(500, "movie"), "2/2": nil, "2/4": nil}, true)
		c, _ := New(Config{Kind: models.MediaServerPlex, URL: f.URL, Token: "t"})
		got, pages, changed := listAll(t, c)
		assert.Len(t, got, 500)
		assert.Equal(t, 4, pages)
		assert.False(t, changed, "看不出变没变时不算变了")
	})
	t.Run("列的过程中分区没了", func(t *testing.T) {
		f := fakePlexLibrary(t, plexSections, map[string][]string{"1/1": mk(10, "movie"), "2/2": nil, "2/4": nil}, false)
		c, _ := New(Config{Kind: models.MediaServerPlex, URL: f.URL, Token: "t"})
		page, err := c.ListItems(context.Background(), "9:1:")
		require.NoError(t, err)
		assert.True(t, page.Changed)
		assert.Empty(t, page.Next)
		for _, bad := range []string{"1", "1:x:0:3", "1:1:x"} {
			_, err := c.ListItems(context.Background(), bad)
			require.ErrorIs(t, err, ErrBadConfig, bad)
		}
	})
	t.Run("没有电影与剧集分区", func(t *testing.T) {
		f := fakePlexLibrary(t, `{"MediaContainer":{"Directory":[{"key":"7","type":"artist"}]}}`, nil, false)
		c, _ := New(Config{Kind: models.MediaServerPlex, URL: f.URL, Token: "t"})
		page, err := c.ListItems(context.Background(), "")
		require.NoError(t, err)
		assert.Empty(t, page.Items)
		assert.Empty(t, page.Next)
	})
	t.Run("没有继续观看", func(t *testing.T) {
		c, _ := New(Config{Kind: models.MediaServerPlex, URL: "http://127.0.0.1:1", Token: "t"})
		_, err := c.Users(context.Background())
		require.ErrorIs(t, err, ErrUnsupported)
		_, err = c.Resume(context.Background(), "u", 1)
		require.ErrorIs(t, err, ErrUnsupported)
	})
}

func TestItemURL(t *testing.T) {
	assert.Equal(t, "http://emby:8096/web/index.html#!/item?id=123&serverId=srv", ItemURL(models.MediaServerEmby, "http://emby:8096/", "srv", "123"))
	assert.Equal(t, "https://jf.example.com/web/#/details?id=abc&serverId=s1", ItemURL(models.MediaServerJellyfin, "https://jf.example.com", "s1", "abc"))
	assert.Equal(t, "https://jf.example.com/web/#/details?id=abc", ItemURL(models.MediaServerJellyfin, "https://jf.example.com", "", "abc"), "没有服务器编号时不带")
	assert.Equal(t, "http://plex:32400/web/index.html#!/server/m1/details?key=%2Flibrary%2Fmetadata%2F55", ItemURL(models.MediaServerPlex, "http://plex:32400", "m1", "55"))
	assert.Empty(t, ItemURL(models.MediaServerPlex, "http://plex:32400", "", "55"), "Plex 没有服务器编号拼不出")
	assert.Empty(t, ItemURL(models.MediaServerEmby, "", "s", "1"))
	assert.Empty(t, ItemURL(models.MediaServerEmby, "http://x", "s", ""))
	assert.Empty(t, ItemURL("kodi", "http://x", "s", "1"))
}

func TestHomeURL(t *testing.T) {
	assert.Equal(t, "http://emby:8096/web/index.html", HomeURL(models.MediaServerEmby, " http://emby:8096/ "))
	assert.Equal(t, "https://jf.example.com/web/", HomeURL(models.MediaServerJellyfin, "https://jf.example.com"))
	assert.Equal(t, "http://plex:32400/web/index.html", HomeURL(models.MediaServerPlex, "http://plex:32400"))
	assert.Empty(t, HomeURL(models.MediaServerEmby, ""))
	assert.Empty(t, HomeURL("kodi", "http://x"))
}
