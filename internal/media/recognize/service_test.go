package recognize

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

type fakeCipher struct{}

func (fakeCipher) Encrypt(p string) (string, error) { return "enc:" + reverse(p), nil }
func (fakeCipher) Decrypt(c string) (string, error) {
	return reverse(strings.TrimPrefix(c, "enc:")), nil
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

const goodKey = "0123456789abcdef0123456789abcdef"

// fakeTMDB 认 goodKey；肖申克的救赎与权力的游戏能搜到。
func fakeTMDB(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("api_key") != goodKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"status_code":7}`))
			return
		}
		q := r.URL.Query().Get("query")
		switch {
		case r.URL.Path == "/3/configuration":
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Path == "/3/search/movie" && (strings.Contains(q, "Shawshank") || strings.Contains(q, "肖申克")):
			_, _ = w.Write([]byte(`{"results":[
				{"id":278,"title":"肖申克的救赎","original_title":"The Shawshank Redemption","release_date":"1994-09-23","popularity":100},
				{"id":5000,"title":"Shawshank: The Redeeming Feature","original_title":"Shawshank: The Redeeming Feature","release_date":"2001-01-01","popularity":2}]}`))
		case r.URL.Path == "/3/search/movie" && strings.Contains(q, "Inside Out"):
			_, _ = w.Write([]byte(`{"results":[
				{"id":1022789,"title":"头脑特工队2","original_title":"Inside Out 2","release_date":"2024-06-11","popularity":500},
				{"id":150540,"title":"头脑特工队","original_title":"Inside Out","release_date":"2015-06-09","popularity":200}]}`))
		case r.URL.Path == "/3/search/movie" && (strings.Contains(q, "Oppenheimer") || strings.Contains(q, "奥本海默")):
			_, _ = w.Write([]byte(`{"results":[]}`))
		case r.URL.Path == "/3/search/tv" && strings.Contains(q, "Game of Thrones"):
			_, _ = w.Write([]byte(`{"results":[{"id":1399,"name":"权力的游戏","original_name":"Game of Thrones","first_air_date":"2011-04-17","popularity":300}]}`))
		case strings.HasPrefix(r.URL.Path, "/3/search/"):
			_, _ = w.Write([]byte(`{"results":[]}`))
		case r.URL.Path == "/3/find/tt0111161":
			_, _ = w.Write([]byte(`{"movie_results":[{"id":278,"title":"肖申克的救赎","original_title":"The Shawshank Redemption","release_date":"1994-09-23"}],"tv_results":[]}`))
		case r.URL.Path == "/3/movie/278":
			_, _ = w.Write([]byte(`{"id":278,"title":"肖申克的救赎","original_title":"The Shawshank Redemption","release_date":"1994-09-23","overview":"希望让人自由。","imdb_id":"tt0111161"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status_code":34}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{}, &models.MediaCache{}))
	srv := fakeTMDB(t)
	return New(Config{DB: db, Cipher: fakeCipher{}, BaseURL: srv.URL + "/3", RatePerSecond: 1000}), db
}

func ptr(s string) *string { return &s }

func withKey(t *testing.T, s *Service) {
	t.Helper()
	_, err := s.SaveSettings(context.Background(), SettingsInput{TMDBKey: ptr(goodKey)})
	require.NoError(t, err)
}

func TestSettings(t *testing.T) {
	s, db := newService(t)
	ctx := context.Background()
	got, err := s.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, Settings{Language: "zh-CN"}, got)

	got, err = s.SaveSettings(ctx, SettingsInput{TMDBKey: ptr(" " + goodKey + " "), Language: "en-US", ProxyURL: ptr("http://user:pw@127.0.0.1:7890")})
	require.NoError(t, err)
	assert.Equal(t, Settings{HasTMDBKey: true, Language: "en-US", ProxyURL: "http://user:***@127.0.0.1:7890"}, got)
	var row models.MediaSetting
	require.NoError(t, db.First(&row, 1).Error)
	assert.NotContains(t, row.TMDBKeyEncrypted, goodKey, "API Key 加密保存")
	assert.NotContains(t, row.ProxyEncrypted, "pw@", "代理地址加密保存")

	// 原样提交密码换成 *** 的地址：不改；不带 Key 时不改 Key
	got, err = s.SaveSettings(ctx, SettingsInput{Language: "en-US", ProxyURL: ptr("http://user:***@127.0.0.1:7890")})
	require.NoError(t, err)
	assert.True(t, got.HasTMDBKey)
	c, err := s.client(ctx)
	require.NoError(t, err)
	require.NotNil(t, c)
	proxy, _ := fakeCipher{}.Decrypt(func() string { _ = db.First(&row, 1); return row.ProxyEncrypted }())
	assert.Equal(t, "http://user:pw@127.0.0.1:7890", proxy)

	for _, bad := range []SettingsInput{
		{Language: "fr-FR"},
		{TMDBKey: ptr("short")},
		{TMDBKey: ptr(goodKey + " x")},
		{ProxyURL: ptr("ftp://127.0.0.1:21")},
	} {
		_, saveErr := s.SaveSettings(ctx, bad)
		assert.ErrorIs(t, saveErr, ErrInvalid)
	}

	got, err = s.SaveSettings(ctx, SettingsInput{TMDBKey: ptr(""), ProxyURL: ptr("")})
	require.NoError(t, err)
	assert.Equal(t, Settings{Language: "zh-CN"}, got, "清除 Key 与代理；语言留空时用默认值")
	assert.ErrorIs(t, s.TestTMDB(ctx), tmdb.ErrNoKey)
}

func TestTestTMDB(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	_, err := s.SaveSettings(ctx, SettingsInput{TMDBKey: ptr("BADKEY0123456789abcdef")})
	require.NoError(t, err)
	assert.ErrorIs(t, s.TestTMDB(ctx), tmdb.ErrUnauthorized)
	withKey(t, s)
	assert.NoError(t, s.TestTMDB(ctx))
}

func TestRecognize(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	_, err := s.Recognize(ctx, Input{})
	require.ErrorIs(t, err, ErrInvalid)

	// 没有 API Key：只做标题解析
	res, err := s.Recognize(ctx, Input{Title: "The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi"})
	require.NoError(t, err)
	assert.Equal(t, SourceNone, res.Source)
	assert.Equal(t, "The Shawshank Redemption", res.Meta.NameEN)
	assert.Equal(t, "The Shawshank Redemption 1994", res.Summary)
	assert.Contains(t, res.Message, "没有填写 TMDB API Key")

	withKey(t, s)
	res, err = s.Recognize(ctx, Input{Title: "The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi"})
	require.NoError(t, err)
	require.NotNil(t, res.Match)
	assert.Equal(t, SourceSearch, res.Source)
	assert.Equal(t, 278, res.Match.ID)
	assert.Equal(t, "tt0111161", res.Match.IMDbID, "认定后用详情补全（搜索结果里没有 IMDb 编号）")
	assert.Equal(t, "希望让人自由。", res.Match.Overview)
	assert.InDelta(t, 1.4, res.Score, 0.001, "名字相同 1 + 年份相同 0.25 + 类型 0.05 + 排第一 0.1")
	require.NotEmpty(t, res.Candidates)
	assert.Equal(t, 278, res.Candidates[0].ID)

	// 年份对不上：不认定，给出候选
	res, err = s.Recognize(ctx, Input{Title: "The.Shawshank.Redemption.1971.1080p.BluRay"})
	require.NoError(t, err)
	assert.Nil(t, res.Match)
	assert.NotEmpty(t, res.Candidates)
	assert.Contains(t, res.Message, "没有找到可靠的匹配")

	// 中文名也能搜（副标题里的）
	res, err = s.Recognize(ctx, Input{Title: "TSR.1994.1080p", Subtitle: "肖申克的救赎 | 主演: 蒂姆·罗宾斯"})
	require.NoError(t, err)
	require.NotNil(t, res.Match)
	assert.Equal(t, 278, res.Match.ID)

	// 有 IMDb 编号时直接查找
	res, err = s.Recognize(ctx, Input{Title: "Whatever.1080p", IMDbID: "https://www.imdb.com/title/tt0111161/"})
	require.NoError(t, err)
	assert.Equal(t, SourceIMDb, res.Source)
	assert.Equal(t, 278, res.Match.ID)

	// 剧集按剧集搜：后面几季的年份晚于首播年份不扣分；取不到详情时用搜索结果
	res, err = s.Recognize(ctx, Input{Title: "Game.of.Thrones.S08E06.2019.1080p.WEB-DL"})
	require.NoError(t, err)
	require.NotNil(t, res.Match)
	assert.Equal(t, 1399, res.Match.ID)
	assert.Equal(t, "Game of Thrones", res.Match.OriginalTitle)
	assert.Equal(t, tmdb.KindTV, res.Match.MediaType)

	// TMDB 出错：解析结果照样返回，写明原因（搜过的名字读缓存，与 Key 无关，所以换一个没搜过的）
	_, err = s.SaveSettings(ctx, SettingsInput{TMDBKey: ptr("BADKEY0123456789abcdef")})
	require.NoError(t, err)
	res, err = s.Recognize(ctx, Input{Title: "Unseen.Title.2001.1080p"})
	require.NoError(t, err)
	assert.Equal(t, "Unseen Title", res.Meta.NameEN)
	assert.Equal(t, tmdb.ErrUnauthorized.Error(), res.Error)
	assert.Empty(t, res.Message)

	_, err = s.Recognize(ctx, Input{Title: strings.Repeat("长", maxInputLen+1)})
	assert.ErrorIs(t, err, ErrInvalid)
}

func TestOverrides(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	withKey(t, s)
	_, err := s.SetOverride(ctx, OverrideInput{Title: "Some.Unknown.Movie.2020.1080p", TMDBID: 278, MediaType: "music"})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = s.SetOverride(ctx, OverrideInput{Title: "Some.Unknown.Movie.2020.1080p", TMDBID: 0, MediaType: tmdb.KindMovie})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = s.SetOverride(ctx, OverrideInput{Title: "1080p", TMDBID: 278, MediaType: tmdb.KindMovie})
	require.ErrorIs(t, err, ErrInvalid, "解析不出名字")
	_, err = s.SetOverride(ctx, OverrideInput{Title: "Some.Unknown.Movie.2020.1080p", TMDBID: 999, MediaType: tmdb.KindMovie})
	require.ErrorIs(t, err, ErrInvalid, "TMDB 上没有这个条目")

	ov, err := s.SetOverride(ctx, OverrideInput{Title: "Some.Unknown.Movie.2020.1080p", Subtitle: "某部电影", TMDBID: 278, MediaType: tmdb.KindMovie})
	require.NoError(t, err)
	assert.Equal(t, "肖申克的救赎", ov.Title)
	assert.Equal(t, "某部电影 / Some Unknown Movie 2020", ov.Label)

	// 同一个解析结果（换了分辨率、制作组）都按纠正识别
	res, err := s.Recognize(ctx, Input{Title: "Some.Unknown.Movie.2020.2160p.WEB-DL-GRP", Subtitle: "某部电影"})
	require.NoError(t, err)
	assert.Equal(t, SourceOverride, res.Source)
	assert.Equal(t, 278, res.Match.ID)
	assert.Equal(t, ov.ID, res.OverrideID)

	// 再纠正一次覆盖原来的
	_, err = s.SetOverride(ctx, OverrideInput{Title: "Some.Unknown.Movie.2020.720p", Subtitle: "某部电影", TMDBID: 278, MediaType: tmdb.KindMovie})
	require.NoError(t, err)
	list, err := s.Overrides(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, s.DeleteOverride(ctx, list[0].ID))
	assert.ErrorIs(t, s.DeleteOverride(ctx, list[0].ID), ErrNotFound)
	res, err = s.Recognize(ctx, Input{Title: "Some.Unknown.Movie.2020.2160p", Subtitle: "某部电影"})
	require.NoError(t, err)
	assert.NotEqual(t, SourceOverride, res.Source)
}

func TestWords(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	withKey(t, s)
	_, err := s.SaveWord(ctx, models.MediaWordRule{Kind: "drop", Pattern: "x"})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = s.SaveWord(ctx, models.MediaWordRule{ID: 99, Kind: models.MediaWordBlock, Pattern: "x"})
	require.ErrorIs(t, err, ErrNotFound)

	w, err := s.SaveWord(ctx, models.MediaWordRule{Kind: models.MediaWordReplace, Pattern: "TSR", Replacement: "The.Shawshank.Redemption", Enabled: true, Note: " 简称 ", Offset: 5})
	require.NoError(t, err)
	assert.Equal(t, "简称", w.Note)
	assert.Zero(t, w.Offset, "替换规则不留偏移")
	res, err := s.Recognize(ctx, Input{Title: "TSR.1994.1080p.BluRay"})
	require.NoError(t, err)
	assert.Equal(t, []uint{w.ID}, res.RuleHits)
	require.NotNil(t, res.Match)
	assert.Equal(t, 278, res.Match.ID)

	// 停用后不再套用
	w.Enabled = false
	w, err = s.SaveWord(ctx, *w)
	require.NoError(t, err)
	assert.False(t, w.Enabled)
	res, err = s.Recognize(ctx, Input{Title: "TSR.1994.1080p.BluRay"})
	require.NoError(t, err)
	assert.Empty(t, res.RuleHits)

	off, err := s.SaveWord(ctx, models.MediaWordRule{Kind: models.MediaWordOffset, Pattern: "Some.Show", Offset: -12, Enabled: true, Replacement: "x"})
	require.NoError(t, err)
	assert.Empty(t, off.Replacement)
	res, err = s.Recognize(ctx, Input{Title: "Some.Show.S02E13.1080p"})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Meta.Episode)

	list, err := s.Words(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.NoError(t, s.DeleteWord(ctx, w.ID))
	assert.ErrorIs(t, s.DeleteWord(ctx, w.ID), ErrNotFound)
}

// 只靠年份判成电影的标题，电影里没有可靠匹配时补搜剧集。
func TestRecognizeFallsBackToTV(t *testing.T) {
	s, _ := newService(t)
	withKey(t, s)
	res, err := s.Recognize(context.Background(), Input{Title: "Game.of.Thrones.2019.1080p.BluRay"})
	require.NoError(t, err)
	assert.Equal(t, meta.TypeMovie, res.Meta.Type, "解析只看到年份")
	require.NotNil(t, res.Match)
	assert.Equal(t, 1399, res.Match.ID)
	assert.Equal(t, tmdb.KindTV, res.Match.MediaType)
}

// 续集编号对不上（Inside Out 对 Inside Out 2）不算名字相同；同名的旧片年份差太多也不认定。
func TestRecognizeSequelNumbers(t *testing.T) {
	s, _ := newService(t)
	withKey(t, s)
	res, err := s.Recognize(context.Background(), Input{Title: "Inside.Out.2024.1080p.WEB-DL"})
	require.NoError(t, err)
	assert.Nil(t, res.Match)
	require.Len(t, res.Candidates, 2)
	res, err = s.Recognize(context.Background(), Input{Title: "Inside.Out.2.2024.1080p.WEB-DL"})
	require.NoError(t, err)
	require.NotNil(t, res.Match)
	assert.Equal(t, 1022789, res.Match.ID)
}

// 纠正保存英文名为主键、中文名为别名：同一个标题有没有中文副标题都命中；只有中文名的标题也命中。
func TestOverrideAliases(t *testing.T) {
	s, db := newService(t)
	ctx := context.Background()
	withKey(t, s)
	// 先只按中文名纠正过一次
	cnOnly, err := s.SetOverride(ctx, OverrideInput{Title: "奥本海默 2023 1080p", TMDBID: 278, MediaType: tmdb.KindMovie})
	require.NoError(t, err)
	assert.Equal(t, "movie|奥本海默|2023", cnOnly.Key)

	ov, err := s.SetOverride(ctx, OverrideInput{Title: "Oppenheimer.2023.1080p.BluRay", Subtitle: "奥本海默 | 中字", TMDBID: 278, MediaType: tmdb.KindMovie})
	require.NoError(t, err)
	assert.Equal(t, "movie|oppenheimer|2023", ov.Key)
	assert.Equal(t, "movie|奥本海默|2023", ov.AltKey)
	var n int64
	require.NoError(t, db.Model(&models.MediaOverride{}).Count(&n).Error)
	assert.EqualValues(t, 1, n, "带英文名的纠正取代了只按中文名的那条")

	for _, in := range []Input{
		{Title: "Oppenheimer.2023.2160p.WEB-DL"},
		{Title: "Oppenheimer.2023.2160p.WEB-DL", Subtitle: "奥本海默 | 国语"},
		{Title: "奥本海默 2023 2160p WEB-DL"},
	} {
		res, err := s.Recognize(ctx, in)
		require.NoError(t, err)
		assert.Equal(t, SourceOverride, res.Source, in.Title+" "+in.Subtitle)
		assert.Equal(t, ov.ID, res.OverrideID)
	}
}

// 所有 TMDB 客户端共用服务的限速额度：每次识别新建客户端也不会重置。
func TestSharedLimiter(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{}, &models.MediaCache{}))
	srv := fakeTMDB(t)
	s := New(Config{DB: db, Cipher: fakeCipher{}, BaseURL: srv.URL + "/3", RatePerSecond: 0.5})
	withKey(t, s)
	require.NoError(t, s.TestTMDB(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	assert.Error(t, s.TestTMDB(ctx), "第二次要等限速，100ms 内等不到")
	c1, err := s.client(context.Background())
	require.NoError(t, err)
	c2, err := s.client(context.Background())
	require.NoError(t, err)
	assert.NotSame(t, c1, c2)
}

func TestPick(t *testing.T) {
	m := meta.Meta{NameEN: "The Office US", Year: 2005, Type: meta.TypeTV}
	us := Candidate{Result: tmdb.Result{Title: "The Office", Year: 2005, MediaType: tmdb.KindTV}, Score: 1.15}
	uk := Candidate{Result: tmdb.Result{Title: "The Office", Year: 2001, MediaType: tmdb.KindTV}, Score: 1.1}
	assert.Nil(t, pick(m, []Candidate{us, uk}), "名字只是部分相同、和第二名差不到 0.1：只列候选")
	uk.Score = 0.5
	assert.NotNil(t, pick(m, []Candidate{us, uk}))
	exact := meta.Meta{NameEN: "Dune", Year: 2021, Type: meta.TypeMovie}
	a := Candidate{Result: tmdb.Result{Title: "Dune", Year: 2021}, Score: 1.4}
	b := Candidate{Result: tmdb.Result{Title: "Dune", Year: 2021}, Score: 1.35}
	assert.NotNil(t, pick(exact, []Candidate{a, b}), "名字完全相同时不看分差")
	assert.Nil(t, pick(exact, nil))
}

func TestScore(t *testing.T) {
	m := meta.Meta{NameEN: "Shogun", Year: 2024, Type: meta.TypeTV, Season: 1}
	assert.Equal(t, 1.0, titleScore(m, tmdb.Result{Title: "幕府将军", OriginalTitle: "Shōgun"}), "去掉重音再比")
	assert.Equal(t, 0.75, titleScore(meta.Meta{NameEN: "The Office US"}, tmdb.Result{Title: "The Office"}))
	assert.Equal(t, 0.0, titleScore(meta.Meta{NameEN: "Hell"}, tmdb.Result{Title: "Hellboy"}), "短的不到长的六成不算包含")
	assert.Equal(t, 1.0, titleScore(meta.Meta{NameEN: "Mission Impossible Dead Reckoning Part One"}, tmdb.Result{OriginalTitle: "Mission: Impossible - Dead Reckoning Part One"}))
	assert.Equal(t, 0.0, titleScore(meta.Meta{}, tmdb.Result{Title: "x"}))

	movie := meta.Meta{NameEN: "Dune", Year: 2021, Type: meta.TypeMovie}
	assert.InDelta(t, 1.4, score(movie, tmdb.Result{Title: "Dune", Year: 2021, MediaType: tmdb.KindMovie}, 0), 0.001)
	assert.InDelta(t, 0.75, score(movie, tmdb.Result{Title: "Dune", Year: 1984, MediaType: tmdb.KindMovie}, 2), 0.001, "同名不同年的翻拍片扣分")
	assert.InDelta(t, 1.2, score(movie, tmdb.Result{Title: "Dune", Year: 2022, MediaType: tmdb.KindMovie}, 1), 0.001, "差一年 +0.1、排第二 +0.05")
	tv := meta.Meta{NameEN: "House of the Dragon", Year: 2024, Type: meta.TypeTV, Season: 2}
	assert.InDelta(t, 1.15, score(tv, tmdb.Result{Title: "House of the Dragon", Year: 2022, MediaType: tmdb.KindTV}, 0), 0.001, "后面几季不扣分")

	assert.NotNil(t, pick(movie, []Candidate{{Result: tmdb.Result{Title: "Dune"}, Score: 1.0}}))
	assert.Nil(t, pick(movie, []Candidate{{Result: tmdb.Result{Title: "Dune"}, Score: 0.9}}))
	assert.Nil(t, pick(movie, []Candidate{{Result: tmdb.Result{Title: "Other"}, Score: 2}}), "名字对不上分再高也不认")

	assert.Equal(t, 0.0, titleScore(meta.Meta{NameEN: "Inside Out"}, tmdb.Result{Title: "Inside Out 2"}), "续集编号对不上")
	assert.Equal(t, 0.0, titleScore(meta.Meta{NameEN: "The Wandering Earth"}, tmdb.Result{Title: "The Wandering Earth II"}))
	assert.Equal(t, 0.0, titleScore(meta.Meta{NameCN: "流浪地球"}, tmdb.Result{Title: "流浪地球2"}))
	assert.Equal(t, 0.75, titleScore(meta.Meta{NameEN: "Blade Runner 2049 Final"}, tmdb.Result{Title: "Blade Runner 2049"}))
}

func TestOverrideKey(t *testing.T) {
	p, a := overrideKeys(meta.Meta{Year: 2020})
	assert.Empty(t, p)
	assert.Empty(t, a)
	p, a = overrideKeys(meta.Meta{NameEN: "Dune", NameCN: "沙丘", Year: 2021, Type: meta.TypeMovie})
	assert.Equal(t, "movie|dune|2021", p)
	assert.Equal(t, "movie|沙丘|2021", a)
	p, a = overrideKeys(meta.Meta{NameCN: "繁花", Year: 2023, Type: meta.TypeTV})
	assert.Equal(t, "tv|繁花", p, "剧集不带年份")
	assert.Empty(t, a)
	long := overrideKey(meta.Meta{NameEN: strings.Repeat("a", 300)}, strings.Repeat("a", 300))
	assert.True(t, strings.HasPrefix(long, "h|"))
	assert.LessOrEqual(t, len(long), 255)
}

func TestMaskProxy(t *testing.T) {
	assert.Equal(t, "socks5://u:***@1.2.3.4:1080", maskProxy("socks5://u:p@1.2.3.4:1080"))
	assert.Equal(t, "http://127.0.0.1:7890", maskProxy("http://127.0.0.1:7890"))
	assert.Equal(t, "http://u@h:1", maskProxy("http://u@h:1"))
}
