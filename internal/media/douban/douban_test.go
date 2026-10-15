package douban

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 豆瓣接口的样子（2026-10 实测的响应删减而来）。
const (
	suggestOppenheimer = `[{"episode":"","img":"https://img2.doubanio.com/view/photo/s_ratio_poster/public/p2876555451.jpg","title":"奥本海默","url":"https://movie.douban.com/subject/35593344/","type":"movie","year":"2023","sub_title":"Oppenheimer","id":"35593344"},
{"episode":"","title":"奥本海默传","type":"book","year":"2023","sub_title":"","id":"999"}]`
	suggestTLOU = `[{"episode":"9","title":"最后生还者 第一季","type":"movie","year":"2023","sub_title":"The Last of Us","id":"25848328"},
{"episode":"7","title":"最后生还者 第二季","type":"movie","year":"2025","sub_title":"The Last of Us","id":"36221305"},
{"episode":"unknow","title":"最后生还者 第三季","type":"movie","year":"","sub_title":"The Last of Us","id":"37000000"}]`
	detailOppenheimer = `{"id":"35593344","title":"奥本海默","original_title":"Oppenheimer","year":"2023","intro":"  原子弹之父的故事。\n","rating":{"count":850981,"max":10,"star_count":4.5,"value":8.8},"subtype":"movie","is_tv":false}`
	detailTLOU2       = `{"id":"36221305","title":"最后生还者 第二季","year":"2025","intro":"第二季。","rating":{"count":100,"max":10,"value":7.5},"subtype":"tv","is_tv":true}`
	detailUnrated     = `{"id":"37000000","title":"最后生还者 第三季","year":"","intro":"","rating":null}`
)

type fakeDouban struct {
	*httptest.Server
	mu       sync.Mutex
	queries  []string
	referers []string
	agents   []string
	status   int
	body     string
}

func newFakeDouban(t *testing.T) *fakeDouban {
	t.Helper()
	f := &fakeDouban{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.queries = append(f.queries, r.URL.Path+"?"+r.URL.RawQuery)
		f.referers = append(f.referers, r.Header.Get("Referer"))
		f.agents = append(f.agents, r.Header.Get("User-Agent"))
		status, body := f.status, f.body
		f.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
			return
		}
		q := r.URL.Query().Get("q")
		switch {
		case r.URL.Path == "/j/subject_suggest" && (q == "奥本海默" || q == "Oppenheimer"):
			_, _ = w.Write([]byte(suggestOppenheimer))
		case r.URL.Path == "/j/subject_suggest" && q == "最后生还者":
			_, _ = w.Write([]byte(suggestTLOU))
		case r.URL.Path == "/j/subject_suggest":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/rexxar/api/v2/movie/35593344":
			_, _ = w.Write([]byte(detailOppenheimer))
		case r.URL.Path == "/rexxar/api/v2/tv/36221305":
			_, _ = w.Write([]byte(detailTLOU2))
		case r.URL.Path == "/rexxar/api/v2/tv/37000000":
			_, _ = w.Write([]byte(detailUnrated))
		case r.URL.Path == "/redirect":
			http.Redirect(w, r, "http://example.com/x", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeDouban) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.queries...)
}

func (f *fakeDouban) fail(status int, body string) {
	f.mu.Lock()
	f.status, f.body = status, body
	f.mu.Unlock()
}

type memCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (c *memCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.m[key]
	return b, ok
}

func (c *memCache) Set(key string, value []byte, _ time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string][]byte{}
	}
	c.m[key] = value
}

func newClient(f *fakeDouban, cache Cache) *Client {
	return New(Config{SearchURL: f.URL + "/j/subject_suggest", DetailURL: f.URL + "/rexxar/api/v2/", Cache: cache, Interval: time.Millisecond})
}

// 电影：按中文名搜到、取详情（带 Referer 与手机浏览器的 User-Agent），第二次走缓存
func TestFindMovie(t *testing.T) {
	f := newFakeDouban(t)
	c := newClient(f, &memCache{})
	got, err := c.Find(context.Background(), Query{Kind: KindMovie, Title: "奥本海默", OriginalTitle: "Oppenheimer", Year: 2023})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, Subject{ID: "35593344", Title: "奥本海默", OriginalTitle: "Oppenheimer", Year: 2023, Intro: "原子弹之父的故事。", Rating: 8.8, Votes: 850981}, *got)
	assert.Equal(t, []string{"/j/subject_suggest?q=%E5%A5%A5%E6%9C%AC%E6%B5%B7%E9%BB%98", "/rexxar/api/v2/movie/35593344?"}, f.requests())
	assert.Equal(t, "https://m.douban.com/movie/subject/35593344/", f.referers[1])
	assert.Contains(t, f.agents[0], "Mobile")

	again, err := c.Find(context.Background(), Query{Kind: KindMovie, Title: "奥本海默", OriginalTitle: "Oppenheimer", Year: 2023})
	require.NoError(t, err)
	assert.Equal(t, got, again)
	assert.Len(t, f.requests(), 2, "走缓存")
}

// 中文名搜不到时用原名（对上豆瓣给的原名）；年份差一年以内；种类不对、年份不对、名字不对的不要
func TestFindFallbacksAndMisses(t *testing.T) {
	f := newFakeDouban(t)
	c := newClient(f, nil)
	got, err := c.Find(context.Background(), Query{Kind: KindMovie, Title: "奥本海默（台）", OriginalTitle: "Oppenheimer", Year: 2024})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "35593344", got.ID)

	for _, q := range []Query{
		{Kind: KindMovie, Title: "奥本海默", Year: 2020},
		{Kind: KindTV, Title: "奥本海默", Year: 2023},
		{Kind: KindMovie, Title: "不存在的电影", Year: 2023},
		{Kind: KindMovie, Title: "", OriginalTitle: ""},
	} {
		got, err := c.Find(context.Background(), q)
		require.NoError(t, err, q)
		assert.Nil(t, got, q)
	}
}

// 剧集按季找：标题里的「第 N 季」，年份用这一季的；没人评分的评分为 0
func TestFindSeason(t *testing.T) {
	f := newFakeDouban(t)
	c := newClient(f, nil)
	got, err := c.Find(context.Background(), Query{Kind: KindTV, Title: "最后生还者", OriginalTitle: "The Last of Us", Year: 2025, Season: 2})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, Subject{ID: "36221305", Title: "最后生还者 第二季", Year: 2025, Intro: "第二季。", Rating: 7.5, Votes: 100}, *got)
	assert.Contains(t, f.requests(), "/rexxar/api/v2/tv/36221305?")

	got, err = c.Find(context.Background(), Query{Kind: KindTV, Title: "最后生还者", Season: 3})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Zero(t, got.Rating)

	got, err = c.Find(context.Background(), Query{Kind: KindTV, Title: "最后生还者", Year: 2023, Season: 2})
	require.NoError(t, err)
	assert.Nil(t, got, "第二季的年份对不上")
}

// 豆瓣拒绝、返回网页（例如验证页）、跳到别的网站时报错；编号不对时不发请求
func TestErrors(t *testing.T) {
	f := newFakeDouban(t)
	c := newClient(f, &memCache{})
	f.fail(http.StatusForbidden, "")
	_, err := c.Find(context.Background(), Query{Kind: KindMovie, Title: "奥本海默"})
	require.ErrorContains(t, err, "拒绝")

	f.fail(http.StatusOK, "<html>请输入验证码</html>")
	_, err = c.Find(context.Background(), Query{Kind: KindMovie, Title: "奥本海默"})
	require.ErrorContains(t, err, "不是预期的内容")

	f.fail(http.StatusInternalServerError, "")
	_, err = c.Subject(context.Background(), KindMovie, "35593344")
	require.ErrorContains(t, err, "HTTP 500")

	f.fail(0, "")
	n := len(f.requests())
	_, err = c.Subject(context.Background(), KindMovie, "../x")
	require.Error(t, err)
	assert.Len(t, f.requests(), n)

	c = New(Config{SearchURL: f.URL + "/redirect", Interval: time.Millisecond})
	_, err = c.Find(context.Background(), Query{Kind: KindMovie, Title: "奥本海默"})
	require.ErrorContains(t, err, "跳转")
}

// 请求之间至少隔 Interval；等的时候 ctx 取消就返回
func TestInterval(t *testing.T) {
	f := newFakeDouban(t)
	c := New(Config{SearchURL: f.URL + "/j/subject_suggest", Interval: 60 * time.Millisecond})
	start := time.Now()
	var done atomic.Int32
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			if _, err := c.suggest(context.Background(), "x"); err == nil {
				done.Add(1)
			}
		})
	}
	wg.Wait()
	assert.Equal(t, int32(3), done.Load())
	assert.GreaterOrEqual(t, time.Since(start), 120*time.Millisecond, "三个请求占三个时间位")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c = New(Config{SearchURL: f.URL + "/j/subject_suggest", Interval: time.Hour})
	_, _ = c.suggest(context.Background(), "x")
	_, err := c.suggest(ctx, "y")
	require.ErrorIs(t, err, context.Canceled)
}

func TestMatchHelpers(t *testing.T) {
	for in, want := range map[string]int{"一": 1, "二": 2, "两": 2, "十": 10, "十一": 11, "二十": 20, "二十三": 23, "一百零一": 101, "12": 12, "第": 0} {
		assert.Equal(t, want, cnNumber(in), in)
	}
	assert.Equal(t, "lastofus", norm("Last of Us!"))
	assert.Equal(t, "奥本海默", norm("奥本海默 · "))
	assert.True(t, isDouban("m.douban.com"))
	assert.False(t, isDouban("douban.com.evil.example"))
	q := Query{Kind: KindTV, Title: "最后生还者", OriginalTitle: "The Last of Us"}
	assert.Equal(t, "1", match([]suggestion{{ID: "1", Title: "最后生还者", SubTitle: "The Last of Us Season 1", Type: "movie", Episode: "9"}}, q),
		"没写季是第 1 季；原名里的 Season 1 去掉再比")
	assert.True(t, strings.HasPrefix(DefaultDetailURL, "https://m.douban.com/"))
}

// 同名的里年份相同的优先；没有年份相同的时，差一年以内的只有一个才用；没给年份时只有一个对得上才用
func TestMatchYears(t *testing.T) {
	two := []suggestion{
		{ID: "2023", Title: "沙丘", SubTitle: "Dune", Year: "2023", Type: "movie"},
		{ID: "2024", Title: "沙丘", SubTitle: "Dune", Year: "2024", Type: "movie"},
		{ID: "1984", Title: "沙丘", SubTitle: "Dune", Year: "1984", Type: "movie"},
	}
	assert.Equal(t, "2024", match(two, Query{Kind: KindMovie, Title: "沙丘", Year: 2024}), "年份相同的优先")
	assert.Equal(t, "1984", match(two, Query{Kind: KindMovie, Title: "沙丘", Year: 1985}), "差一年以内的只有一个")
	around := []suggestion{
		{ID: "2022", Title: "沙丘", Year: "2022", Type: "movie"},
		{ID: "2024", Title: "沙丘", Year: "2024", Type: "movie"},
	}
	assert.Empty(t, match(around, Query{Kind: KindMovie, Title: "沙丘", Year: 2023}), "差一年以内的不止一个：拿不准")
	assert.Empty(t, match(two, Query{Kind: KindMovie, Title: "沙丘"}), "没给年份、同名的不止一个：拿不准")
	assert.Equal(t, "1984", match(two[2:], Query{Kind: KindMovie, Title: "沙丘"}), "没给年份、只有一个")
}

// 站点给的编号取到的条目：名字对得上、年份差一年以内（有一边不知道时只看名字）才算
func TestSubjectMatches(t *testing.T) {
	s := &Subject{ID: "1", Title: "最后生还者 第一季", OriginalTitle: "The Last of Us Season 1", Year: 2023}
	assert.True(t, s.Matches(Query{Kind: KindTV, Title: "最后生还者", Year: 2023}))
	assert.True(t, s.Matches(Query{Kind: KindTV, OriginalTitle: "The Last of Us", Year: 2024}))
	assert.False(t, s.Matches(Query{Kind: KindTV, Title: "最后生还者", Year: 2020}), "年份对不上")
	assert.False(t, s.Matches(Query{Kind: KindTV, Title: "别的剧", Year: 2023}), "名字对不上")
	assert.True(t, (&Subject{Title: "奥本海默"}).Matches(Query{Title: "奥本海默", Year: 2023}), "条目没有年份时只看名字")
}
