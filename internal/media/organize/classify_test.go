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
	short := func(r tmdb.Result, minutes int) tmdb.Result {
		r.Runtime = minutes
		return r
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
		{short(tv([]string{"CN"}, 18), 8), "短剧"},
		{short(tv([]string{"US"}, 18), 15), "短剧"},
		{short(tv([]string{"CN"}, 18), 16), "国产剧"},
		{short(tv([]string{"CN"}, 16), 12), "国漫"}, // 一集十来分钟的动画先进动画的分类
		{short(tv(nil, 10762), 5), "儿童"},
	} {
		assert.Equal(t, x.want, c.Classify(x.r), "%+v", x.r)
	}
	assert.Equal(t, "短剧", c.Classify(tv([]string{"CN"}, 18), "某某 短剧 全集", "站点标题"), "种子名里有关键字")
	assert.Equal(t, "短剧", c.Classify(tv([]string{"CN"}, 18), "Some.Show.S01", "某某（短剧）"), "站点标题里有关键字")
	assert.Equal(t, "未分类", c.Classify(tv([]string{"BR"}, 18), "Some.Show.S01"), "片长未知、标题里没有关键字")
	assert.Empty(t, ClassifyRules{}.Classify(movie("zh")), "没有规则时不分类")
	assert.Empty(t, ClassifyRules{TV: []ClassifyRule{{Name: "日剧", Countries: []string{"JP"}}}}.Classify(tv([]string{"US"})), "都对不上时不分类")
}

// 说明对上的是第几条、对上了哪些条件；片长与关键字是一个条件里的「或」，和别的条件是「且」
func TestClassifyExplain(t *testing.T) {
	rules := ClassifyRules{TV: []ClassifyRule{
		{Name: "日番", Genres: []int{16, 10759}, Countries: []string{"CN", "JP"}},
		{Name: "国产短剧", Countries: []string{"CN"}, MaxRuntime: 10, Keywords: []string{"Short", "短剧"}},
		{Name: "华语", Languages: []string{"zh", "cn"}},
		{Name: "其他"},
	}}
	r := tmdb.Result{MediaType: tmdb.KindTV, GenreIDs: []int{16, 18}, Countries: []string{"JP", "US"}}
	assert.Equal(t, ClassifyNote{Rule: 1, Name: "日番", Rules: 4, Genres: []int{16}, Countries: []string{"JP"}}, rules.Explain(r))

	r = tmdb.Result{MediaType: tmdb.KindTV, Countries: []string{"CN"}, Runtime: 3, OriginalLanguage: "zh"}
	assert.Equal(t, ClassifyNote{Rule: 2, Name: "国产短剧", Rules: 4, Countries: []string{"CN"}, Runtime: 3, MaxRuntime: 10}, rules.Explain(r))
	assert.Equal(t, ClassifyNote{Rule: 2, Name: "国产短剧", Rules: 4, Countries: []string{"CN"}, Runtime: 3, MaxRuntime: 10, Keyword: "Short"},
		rules.Explain(r, "a.short.drama"), "两个都对上时都写，关键字不分大小写")
	r.Runtime = 0
	assert.Equal(t, ClassifyNote{Rule: 2, Name: "国产短剧", Rules: 4, Countries: []string{"CN"}, Keyword: "短剧"}, rules.Explain(r, "", "某某短剧"))
	assert.Equal(t, ClassifyNote{Rule: 3, Name: "华语", Rules: 4, Languages: []string{"zh"}}, rules.Explain(r, "某某"), "片长未知、没有关键字：这一条对不上")
	r.Runtime = 45
	assert.Equal(t, 3, rules.Explain(r).Rule, "片长超过上限")
	us := tmdb.Result{MediaType: tmdb.KindTV, Countries: []string{"US"}, Runtime: 3}
	assert.Equal(t, ClassifyNote{Rule: 4, Name: "其他", Rules: 4}, rules.Explain(us, "短剧"), "和国家或地区是「且」")

	assert.Equal(t, ClassifyNote{Rules: 0}, rules.Explain(tmdb.Result{MediaType: tmdb.KindMovie}), "电影没有规则")
	two := ClassifyRules{Movie: []ClassifyRule{{Name: "动画", Genres: []int{16}}, {Name: "短片", MaxRuntime: 40}}}
	assert.Equal(t, ClassifyNote{Rules: 2}, two.Explain(tmdb.Result{MediaType: tmdb.KindMovie, GenreIDs: []int{18}, Runtime: 120}), "都没对上")
}

func TestClassifyRulesClean(t *testing.T) {
	got, err := ClassifyRules{
		Movie: []ClassifyRule{{Name: " 华语 ", Languages: []string{"ZH", "zh", " cn "}, Genres: []int{18, 18}}},
		TV:    []ClassifyRule{{Name: "美剧", Countries: []string{"us", "US"}}, {Name: "短剧", MaxRuntime: 15, Keywords: []string{" 短剧 ", "短剧", "Short"}}},
	}.clean()
	require.NoError(t, err)
	assert.Equal(t, ClassifyRule{Name: "华语", Languages: []string{"zh", "cn"}, Genres: []int{18}}, got.Movie[0])
	assert.Equal(t, []string{"US"}, got.TV[0].Countries)
	assert.Equal(t, ClassifyRule{Name: "短剧", MaxRuntime: 15, Keywords: []string{"短剧", "Short"}}, got.TV[1])

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
		"片长为负":  {TV: []ClassifyRule{{Name: "x", MaxRuntime: -1}}},
		"片长太长":  {TV: []ClassifyRule{{Name: "x", MaxRuntime: maxClassifyRuntime + 1}}},
		"空关键字":  {TV: []ClassifyRule{{Name: "x", Keywords: []string{" "}}}},
		"关键字太长": {TV: []ClassifyRule{{Name: "x", Keywords: []string{strings.Repeat("长", maxClassifyKeyword+1)}}}},
		"关键字太多": {TV: []ClassifyRule{{Name: "x", Keywords: make([]string, maxClassifyValues+1)}}},
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

// 整理计划写明二级分类怎么定的；关键字也看站点上的标题
func TestOrganizeClassifyNote(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	_, err := e.svc.SaveClassify(e.ctx, DefaultClassifyRules)
	require.NoError(t, err)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, ClassifyDir: true})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, ClassifyDir: true})
	e.oppenheimer()
	e.lastOfUs()
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.NotNil(t, p.Classify)
	assert.Equal(t, ClassifyNote{Rule: 3, Name: "外语电影", Rules: 3}, *p.Classify)

	p, err = e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Equal(t, "欧美剧", p.Category)
	assert.Equal(t, 8, p.Classify.Rule)
	assert.Equal(t, []string{"US"}, p.Classify.Countries)

	hash := tlouHash
	require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "1", TorrentHash: &hash, Title: "最后生还者 短剧版"}).Error)
	p, err = e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Equal(t, "短剧", p.Category)
	assert.Equal(t, "短剧", p.Classify.Keyword)
	var targets []string
	for _, it := range p.Items {
		targets = append(targets, it.Target)
	}
	assert.Contains(t, targets, filepath.Join(e.tv, "短剧", "最后生还者 (2023)", "Season 1", "最后生还者 - S01E01 - 当你迷失在黑暗中.mkv"))

	// 这一类没有规则：不写说明
	_, err = e.svc.SaveClassify(e.ctx, ClassifyRules{TV: DefaultClassifyRules.TV})
	require.NoError(t, err)
	p, err = e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Nil(t, p.Classify)
	assert.Empty(t, p.Category)
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
	assert.Nil(t, p.Classify)
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
	assert.Equal(t, models.MediaFailRecognize, p.FailKind)

	e.tmdb.mu.Lock()
	e.tmdb.detailsDown = false
	e.tmdb.mu.Unlock()
	p, err = e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Empty(t, p.Problem)
	assert.Equal(t, "其他", p.Category, "奥本海默的详情里没有写出品国家或地区")
}
