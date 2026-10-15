package organize

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

func TestDefaultClassifyRules(t *testing.T) {
	movie := func(lang string, genres ...int) tmdb.Result {
		return tmdb.Result{MediaType: tmdb.KindMovie, OriginalLanguage: lang, GenreIDs: genres}
	}
	tv := func(countries []string, genres ...int) tmdb.Result {
		return tmdb.Result{MediaType: tmdb.KindTV, Countries: countries, GenreIDs: genres}
	}
	c := DefaultClassifyRules
	for _, x := range []struct {
		r    tmdb.Result
		want string
	}{
		{movie("zh", 16), "动画电影"},
		{movie("zh", 18), "华语电影"},
		{movie("cn"), "华语电影"},
		{movie("en", 18), "外语电影"},
		{movie(""), "外语电影"},
		{tv([]string{"CN"}, 16), "国漫"},
		{tv([]string{"JP"}, 16, 10759), "日番"},
		{tv([]string{"US"}, 16), "欧美剧"},
		{tv([]string{"GB"}, 99), "纪录片"},
		{tv(nil, 10762), "儿童"},
		{tv([]string{"KR"}, 10764), "综艺"},
		{tv([]string{"TW"}, 18), "国产剧"},
		{tv([]string{"US"}, 18), "欧美剧"},
		{tv([]string{"KR"}, 18), "日韩剧"},
		{tv([]string{"BR"}, 18), "未分类"},
	} {
		assert.Equal(t, x.want, c.Classify(x.r), "%+v", x.r)
	}
	assert.Empty(t, ClassifyRules{}.Classify(movie("zh")), "没有规则时不分类")
	assert.Empty(t, ClassifyRules{TV: []ClassifyRule{{Name: "日剧", Countries: []string{"JP"}}}}.Classify(tv([]string{"US"})), "都对不上时不分类")
}

func TestClassifyRulesClean(t *testing.T) {
	got, err := ClassifyRules{
		Movie: []ClassifyRule{{Name: " 华语 ", Languages: []string{"ZH", "zh", " cn "}, Genres: []int{18, 18}}},
		TV:    []ClassifyRule{{Name: "美剧", Countries: []string{"us", "US"}}},
	}.clean()
	require.NoError(t, err)
	assert.Equal(t, ClassifyRule{Name: "华语", Languages: []string{"zh", "cn"}, Genres: []int{18}}, got.Movie[0])
	assert.Equal(t, []string{"US"}, got.TV[0].Countries)

	many := make([]ClassifyRule, maxClassifyRules+1)
	for i := range many {
		many[i] = ClassifyRule{Name: strings.Repeat("a", i%30+1) + string(rune('a'+i%26))}
	}
	for name, bad := range map[string]ClassifyRules{
		"空名称":   {Movie: []ClassifyRule{{Name: " "}}},
		"斜杠":    {Movie: []ClassifyRule{{Name: "华语/电影"}}},
		"冒号":    {TV: []ClassifyRule{{Name: "a:b"}}},
		"太长":    {Movie: []ClassifyRule{{Name: strings.Repeat("长", maxClassifyName+1)}}},
		"重复":    {TV: []ClassifyRule{{Name: "美剧"}, {Name: "美剧"}}},
		"语言":    {Movie: []ClassifyRule{{Name: "x", Languages: []string{"chinese"}}}},
		"国家或地区": {TV: []ClassifyRule{{Name: "x", Countries: []string{"USA"}}}},
		"类型":    {Movie: []ClassifyRule{{Name: "x", Genres: []int{0}}}},
		"太多":    {Movie: many},
	} {
		_, err := bad.clean()
		require.ErrorIs(t, err, ErrInvalid, name)
	}
}

func TestClassifySaveAndLoad(t *testing.T) {
	e := newEnv(t)
	v, err := e.svc.Classify(e.ctx)
	require.NoError(t, err)
	assert.Empty(t, v.Rules.Movie)
	assert.Empty(t, v.Rules.TV)
	assert.Equal(t, DefaultClassifyRules, v.Presets)

	v, err = e.svc.SaveClassify(e.ctx, ClassifyRules{TV: []ClassifyRule{{Name: "美剧", Countries: []string{"us"}}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"US"}, v.Rules.TV[0].Countries)
	assert.Empty(t, v.Rules.Movie)
	again, err := e.svc.Classify(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, v.Rules, again.Rules)

	// 整理设置的保存不动分类规则
	e.settings(SettingsInput{MinVideoMB: 1})
	again, err = e.svc.Classify(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, v.Rules, again.Rules)

	_, err = e.svc.SaveClassify(e.ctx, ClassifyRules{Movie: []ClassifyRule{{Name: "a/b"}}})
	require.ErrorIs(t, err, ErrInvalid)
}

// 媒体库开了「按二级分类放进子目录」：文件放进分类目录，剧集的 NFO 与 Logo 写在分类目录下的剧集目录里
func TestOrganizeIntoClassifyDir(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	_, err := e.svc.SaveClassify(e.ctx, DefaultClassifyRules)
	require.NoError(t, err)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ClassifyDir: true})
	tvLib := e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeItems: []string{"nfo", "logo"}, ClassifyDir: true})
	assert.True(t, tvLib.ClassifyDir)
	e.oppenheimer()
	e.lastOfUs()

	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, "外语电影", p.Category)
	want := filepath.Join(e.movies, "外语电影", "奥本海默 (2023)", "奥本海默 (2023) - 2160p BluRay HDR H.265.mkv")
	require.Len(t, p.Items, 1)
	assert.Equal(t, want, p.Items[0].Target)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	assert.True(t, exists(want))
	assert.True(t, exists(filepath.Join(filepath.Dir(want), "poster.jpg")))

	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	require.Equal(t, 2, res.Created)
	assert.Equal(t, "欧美剧", res.Plan.Category)
	show := filepath.Join(e.tv, "欧美剧", "最后生还者 (2023)")
	assert.True(t, exists(filepath.Join(show, "Season 1", "最后生还者 - S01E01 - 当你迷失在黑暗中.mkv")))
	assert.True(t, exists(filepath.Join(show, "tvshow.nfo")), "剧集目录在分类目录下面")
	assert.True(t, exists(filepath.Join(show, "clearlogo.png")))
	assert.False(t, exists(filepath.Join(e.tv, "欧美剧", "tvshow.nfo")), "不把分类目录当成剧集目录")

	// 删掉全部记录：剧集目录与空了的分类目录一起删
	for _, r := range e.history() {
		if r.MediaType == tmdb.KindTV {
			require.NoError(t, e.del(r.ID, true))
		}
	}
	assert.False(t, exists(filepath.Join(e.tv, "欧美剧")))
	assert.True(t, exists(e.tv))
}

// 媒体库没开「按二级分类放进子目录」：有规则也不分类
func TestOrganizeClassifyOff(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	_, err := e.svc.SaveClassify(e.ctx, DefaultClassifyRules)
	require.NoError(t, err)
	e.defaultLibraries()
	e.oppenheimer()
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Empty(t, p.Category)
	assert.Equal(t, filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023) - 2160p BluRay HDR H.265.mkv"), p.Items[0].Target)
}

// 平铺的电影模板放进分类目录：分类目录不是电影自己的目录，海报带上文件名
func TestOrganizeClassifyFlatMovie(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	_, err := e.svc.SaveClassify(e.ctx, DefaultClassifyRules)
	require.NoError(t, err)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Template: "{{.Title}}", Scrape: true, ClassifyDir: true})
	e.oppenheimer()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created, res.Plan.Problem)
	cat := filepath.Join(e.movies, "外语电影")
	assert.True(t, exists(filepath.Join(cat, "奥本海默.mkv")))
	assert.True(t, exists(filepath.Join(cat, "奥本海默-poster.jpg")))
	assert.False(t, exists(filepath.Join(cat, "poster.jpg")), "不写成分类目录共用的海报")
}

// 存着的规则不是合法的 JSON：读规则报错；开了开关的媒体库不整理（不当成没有规则放进库目录）
func TestClassifyRulesCorrupt(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	require.NoError(t, e.db.Model(&models.MediaOrganizeSetting{}).Where("id = 1").Update("classify_rules", "{bad").Error)
	_, err := e.svc.Classify(e.ctx)
	require.Error(t, err)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, ClassifyDir: true})
	e.oppenheimer()
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Contains(t, p.Problem, "二级分类的规则读不出来")
	assert.Empty(t, p.Items[0].Target)

	_, err = e.svc.SaveClassify(e.ctx, DefaultClassifyRules)
	require.NoError(t, err, "重新保存就好了")
	p, err = e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Empty(t, p.Problem)
	assert.Equal(t, "外语电影", p.Category)
}

// 识别时 TMDB 详情没取到（用的是搜索结果，没有出品国家或地区）：按国家分的规则不能拿不全的条目定位置，这次不整理、过一会儿再试
func TestOrganizeClassifyNeedsDetails(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	_, err := e.svc.SaveClassify(e.ctx, ClassifyRules{Movie: []ClassifyRule{{Name: "美国电影", Countries: []string{"US"}}, {Name: "其他"}}})
	require.NoError(t, err)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, ClassifyDir: true})
	e.oppenheimer()
	e.tmdb.mu.Lock()
	e.tmdb.detailsDown = true
	e.tmdb.mu.Unlock()
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Contains(t, p.Problem, "取 TMDB 详情失败")
	assert.True(t, p.retryable)

	e.tmdb.mu.Lock()
	e.tmdb.detailsDown = false
	e.tmdb.mu.Unlock()
	p, err = e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Empty(t, p.Problem)
	assert.Equal(t, "其他", p.Category, "奥本海默的详情里没有写出品国家或地区")
}
