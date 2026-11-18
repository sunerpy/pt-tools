package organize

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/bangumi"
	"github.com/sunerpy/pt-tools/internal/media/metadata"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

const oppVideo = "奥本海默 (2023) - 2160p BluRay HDR H.265"

func metaExtras(t *testing.T, paths ...string) string {
	t.Helper()
	var out []extra
	for _, p := range paths {
		out = append(out, extra{Target: p, Kind: extraMeta})
	}
	b, err := json.Marshal(out)
	require.NoError(t, err)
	return string(b)
}

func (e *env) row(id uint) models.MediaTransferHistory {
	e.t.Helper()
	var r models.MediaTransferHistory
	require.NoError(e.t, e.db.First(&r, id).Error)
	return r
}

// 整理时记下刮削的结果与算目录的起点；重新刮削按媒体库现在的设置重写：pt-tools 写的、没被换过的文件照样换（没开覆盖也是），
// 用户换过的不动；不把记录挪到整理历史的最前面
func TestRescrape(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	lib := e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeItems: []string{"nfo", "poster"}})
	e.oppenheimer()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	dir := filepath.Join(e.movies, "奥本海默 (2023)")
	rec := e.history()[0]
	assert.Equal(t, models.MediaScrapeOK, rec.ScrapeStatus)
	assert.Empty(t, rec.ScrapeNote)
	require.NotNil(t, rec.ScrapedAt)
	assert.Equal(t, e.movies, rec.RootPath)
	assert.Equal(t, "JPEG/t/p/w780/opp.jpg", readFile(t, filepath.Join(dir, "poster.jpg")))

	// 改成英文海报、加上背景：没开覆盖，pt-tools 写的海报照样换成英文的，背景新写
	in := LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Enabled: true, Scrape: true, ScrapeItems: []string{"nfo", "poster", "fanart"}, ScrapeImageLang: "en"}
	_, err = e.svc.SaveLibrary(e.ctx, lib.ID, in)
	require.NoError(t, err)
	e.advance(time.Hour)
	got, err := e.svc.Rescrape(e.ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Records)
	assert.Equal(t, 2, got.Written, "海报换了、背景新写，NFO 内容一样不重写")
	assert.Empty(t, got.Messages)
	assert.Equal(t, "简介 TMDB，评分 TMDB", got.Sources)
	assert.Equal(t, "JPEG/t/p/w780/opp-en.jpg", readFile(t, filepath.Join(dir, "poster.jpg")))
	assert.Equal(t, "JPEG/t/p/w1280/opp-bd-none.jpg", readFile(t, filepath.Join(dir, "fanart.jpg")), "选了语言时背景用没有文字的")
	after := e.row(rec.ID)
	assert.Equal(t, rec.UpdatedAt.UnixNano(), after.UpdatedAt.UnixNano(), "不挪到整理历史的最前面")
	assert.True(t, after.ScrapedAt.After(*rec.ScrapedAt))
	var meta []string
	for _, x := range decodeExtras(after.Extras) {
		if x.Kind == extraMeta {
			meta = append(meta, filepath.Base(x.Target))
		}
	}
	assert.ElementsMatch(t, []string{oppVideo + ".nfo", "poster.jpg", "fanart.jpg"}, meta, "换过的海报记下新的文件编号")

	// 用户换了海报：不动
	require.NoError(t, os.Remove(filepath.Join(dir, "poster.jpg")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "poster.jpg"), []byte("mine"), 0o644))
	in.ScrapeImageLang = ""
	_, err = e.svc.SaveLibrary(e.ctx, lib.ID, in)
	require.NoError(t, err)
	got, err = e.svc.Rescrape(e.ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, "mine", readFile(t, filepath.Join(dir, "poster.jpg")))
	assert.Equal(t, []string{"改过的或不是 pt-tools 写的文件没换（媒体库开着覆盖时才换）：poster.jpg"}, got.Messages)
	// 开着覆盖：换掉；媒体服务器里锁定了的 NFO 不动
	in.ScrapeOverwrite = true
	_, err = e.svc.SaveLibrary(e.ctx, lib.ID, in)
	require.NoError(t, err)
	nfo := filepath.Join(dir, oppVideo+".nfo")
	locked := "<movie><title>我的</title><lockdata>true</lockdata></movie>"
	require.NoError(t, os.WriteFile(nfo, []byte(locked), 0o644))
	got, err = e.svc.Rescrape(e.ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, "JPEG/t/p/w780/opp.jpg", readFile(t, filepath.Join(dir, "poster.jpg")))
	assert.Equal(t, locked, readFile(t, nfo))
	assert.Equal(t, []string{"媒体服务器里锁定了的 NFO 没动：" + oppVideo + ".nfo"}, got.Messages)
	assert.Equal(t, models.MediaScrapeOK, e.row(rec.ID).ScrapeStatus, "没换的文件不算出错")
}

// 剧集：从一集的记录重新刮削整部剧在这个媒体库里整理好的每一集
func TestRescrapeWholeShow(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeItems: []string{"nfo"}})
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	ep2 := filepath.Join(show, "Season 1", "最后生还者 - S01E02 - 感染.nfo")
	require.NoError(t, os.WriteFile(ep2, []byte("<episodedetails><title>旧的</title></episodedetails>"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(show, "tvshow.nfo")))
	rows := e.history()
	require.Len(t, rows, 2)
	got, err := e.svc.Rescrape(e.ctx, rows[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 2, got.Records)
	assert.True(t, exists(filepath.Join(show, "tvshow.nfo")), "没有的补上")
	assert.Contains(t, readFile(t, ep2), "<title>旧的</title>", "换过的（文件编号对不上）不动")
	for _, r := range e.history() {
		assert.Equal(t, models.MediaScrapeOK, r.ScrapeStatus)
	}
}

func TestRescrapeRejects(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	lib := e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies})
	_, err := e.svc.Rescrape(e.ctx, 99)
	require.ErrorIs(t, err, ErrNotFound)
	e.oppenheimer()
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	rec := e.history()[0]
	assert.Empty(t, rec.ScrapeStatus, "关着刮削：没刮削过")
	_, err = e.svc.Rescrape(e.ctx, rec.ID)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "关着刮削")

	_, err = e.svc.SaveLibrary(e.ctx, lib.ID, LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Enabled: true, Scrape: true})
	require.NoError(t, err)
	require.NoError(t, os.Remove(rec.TargetPath))
	_, err = e.svc.Rescrape(e.ctx, rec.ID)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "库里的视频已经不在了")
	after := e.row(rec.ID)
	assert.Equal(t, models.MediaScrapeFailed, after.ScrapeStatus, "没能刮削也记下")
	assert.Equal(t, "库里的视频已经不在了，没有刮削", after.ScrapeNote)
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", rec.ID).Update("status", models.MediaTransferFailed).Error)
	_, err = e.svc.Rescrape(e.ctx, rec.ID)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "只有整理好的记录")
}

// 库里的视频换成了别的文件：不给它写旧条目的 NFO 与图片，记下原因
func TestRescrapeReplacedVideo(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeItems: []string{"nfo"}})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	rec := e.history()[0]
	nfo := filepath.Join(e.movies, "奥本海默 (2023)", oppVideo+".nfo")
	require.NoError(t, os.Remove(nfo))
	require.NoError(t, os.Remove(rec.TargetPath))
	require.NoError(t, os.WriteFile(rec.TargetPath, []byte("另一部电影"), 0o644))

	_, err = e.svc.Rescrape(e.ctx, rec.ID)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "换成了别的文件")
	assert.False(t, exists(nfo), "没有给换掉的视频写 NFO")
	after := e.row(rec.ID)
	assert.Equal(t, models.MediaScrapeFailed, after.ScrapeStatus)
	assert.Contains(t, after.ScrapeNote, "换成了别的文件")
}

// noTMDB 是取不到 TMDB 客户端的识别服务（没有填 Key）。
type noTMDB struct{ Recognizer }

func (noTMDB) TMDB(context.Context) (*tmdb.Client, error) { return nil, tmdb.ErrNoKey }

// TMDB 不能用：重新刮削不了，记录记成刮削失败并写明原因
func TestRescrapeWithoutTMDB(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeItems: []string{"nfo"}})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	rec := e.history()[0]
	e.svc.cfg.Recognizer = noTMDB{e.svc.cfg.Recognizer}
	_, err = e.svc.Rescrape(e.ctx, rec.ID)
	require.ErrorIs(t, err, ErrInvalid)
	after := e.row(rec.ID)
	assert.Equal(t, models.MediaScrapeFailed, after.ScrapeStatus)
	assert.Contains(t, after.ScrapeNote, "没有填写 TMDB API Key")
}

// 一集的视频换掉了、TMDB 又不能用：两条记录都记下这次没能刮削的原因
func TestRescrapeSkippedAndTMDBDown(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeItems: []string{"nfo"}})
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	rows := e.history()
	require.Len(t, rows, 2)
	require.NoError(t, os.Remove(rows[0].TargetPath))
	require.NoError(t, os.WriteFile(rows[0].TargetPath, []byte("别的视频"), 0o644))
	e.svc.cfg.Recognizer = noTMDB{e.svc.cfg.Recognizer}

	_, err = e.svc.Rescrape(e.ctx, rows[1].ID)
	require.ErrorIs(t, err, ErrInvalid)
	replaced, other := e.row(rows[0].ID), e.row(rows[1].ID)
	assert.Equal(t, models.MediaScrapeFailed, replaced.ScrapeStatus)
	assert.Contains(t, replaced.ScrapeNote, "换成了别的文件")
	assert.Equal(t, models.MediaScrapeFailed, other.ScrapeStatus)
	assert.Contains(t, other.ScrapeNote, "没有填写 TMDB API Key")
}

// 图片来源选了 Fanart.tv、它的图下载不了：用 TMDB 的那张，记录记成部分刮削并写明
func TestScrapeArtFallbackIsPartial(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	fan := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"movieposter":[{"url":"http://` + r.Host + `/assets/p.jpg","lang":"zh","likes":"5"}]}`))
	}))
	t.Cleanup(fan.Close)
	rec := recognize.New(recognize.Config{
		DB: e.db, Cipher: fakeCipher{}, BaseURL: e.tmdb.URL + "/3", ImageBaseURL: e.tmdb.URL + "/t/p/", RatePerSecond: 1000,
		FanartBaseURL: fan.URL + "/v3.2", FanartImageHosts: []string{fan.Listener.Addr().String()},
	})
	key := "fanartkey0123456789abcdef0123456"
	_, err := rec.SaveSettings(e.ctx, recognize.SettingsInput{FanartKey: &key})
	require.NoError(t, err)
	e.svc.cfg.Recognizer = rec
	src := metadata.Sources{Artwork: metadata.SourceFanart}
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeItems: []string{"poster"}, ScrapeSources: &src})
	e.oppenheimer()
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, "JPEG/t/p/w780/opp.jpg", readFile(t, filepath.Join(e.movies, "奥本海默 (2023)", "poster.jpg")), "用 TMDB 的那张")
	row := e.history()[0]
	assert.Equal(t, models.MediaScrapePartial, row.ScrapeStatus)
	assert.Contains(t, row.ScrapeNote, "poster.jpg：Fanart.tv 的下载不了（")
	assert.Contains(t, row.ScrapeNote, "用了 TMDB 的")
}

// 后台循环在跑时排进队列，和整理一样一次只做一件
func TestRescrapeQueued(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeItems: []string{"nfo"}})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	nfo := filepath.Join(e.movies, "奥本海默 (2023)", oppVideo+".nfo")
	require.NoError(t, os.Remove(nfo))
	e.svc.Start()
	t.Cleanup(e.svc.Stop)
	got, err := e.svc.Rescrape(e.ctx, e.history()[0].ID)
	require.NoError(t, err)
	assert.False(t, got.Queued)
	assert.Equal(t, 1, got.Written)
	assert.True(t, exists(nfo))
}

// 取不到图片列表：记录记成部分成功，写明原因
func TestScrapeStatusPartial(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeItems: []string{"nfo", "logo"}})
	e.tmdb.mu.Lock()
	e.tmdb.imagesDown = true
	e.tmdb.mu.Unlock()
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	rec := e.history()[0]
	assert.Equal(t, models.MediaScrapePartial, rec.ScrapeStatus)
	assert.Contains(t, rec.ScrapeNote, "取 TMDB 图片列表失败")
}

type fakeBangumi struct{ finds []bangumi.Query }

func (f *fakeBangumi) Find(_ context.Context, q bangumi.Query) (*bangumi.Subject, error) {
	f.finds = append(f.finds, q)
	if q.OriginalTitle == "葬送のフリーレン" && q.Date == "2023-09-29" {
		return &bangumi.Subject{ID: 400602, Summary: "Bangumi：打倒魔王之后的故事。", Score: 8.5, Votes: 36689}, nil
	}
	return nil, nil
}

func (f *fakeBangumi) Episodes(context.Context, int) ([]bangumi.Episode, error) {
	return []bangumi.Episode{{ID: 1, Ep: 1, NameCN: "冒险的结束"}}, nil
}

// 媒体库的数据来源：动画的简介、评分与每一集的标题用 Bangumi 的；填了 Fanart.tv 的 Key 时写只有它有的 clearart
func TestScrapeOtherSources(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	b := &fakeBangumi{}
	e.svc.cfg.Bangumi = b
	src := metadata.Sources{Plot: metadata.SourceBangumi, Rating: metadata.SourceBangumi, Episode: metadata.SourceBangumi}
	v := e.library(LibraryInput{Name: "动漫", Kind: models.MediaKindTV, Anime: true, Path: e.anime, Scrape: true, ScrapeItems: []string{"nfo"}, ScrapeSources: &src})
	assert.Equal(t, src, v.ScrapeSources)
	const hash = "cccccccccccccccccccccccccccccccccccccccc"
	e.addTorrent(hash, "[Group] Sousou no Frieren - 01 [1080p]", map[string]int{"[Group] Sousou no Frieren - 01 [1080p].mkv": 2}, nil)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: hash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created, res.Plan.Problem)
	show := filepath.Join(e.anime, "葬送的芙莉莲 (2023)")
	tvshow := readFile(t, filepath.Join(show, "tvshow.nfo"))
	assert.Contains(t, tvshow, "<plot>Bangumi：打倒魔王之后的故事。</plot>")
	assert.Contains(t, tvshow, `<uniqueid type="bangumi">400602</uniqueid>`)
	assert.Contains(t, tvshow, `<rating name="bangumi" max="10" default="true">`)
	assert.Contains(t, readFile(t, filepath.Join(show, "Season 1", "葬送的芙莉莲 - S01E01.nfo")), "<title>冒险的结束</title>")
	assert.Contains(t, strings.Join(res.Messages, "\n"), "TMDB 上没有第 1 季的详情", "TMDB 上没有的季写明")

	// Fanart.tv：识别服务按「媒体识别」里填的 Key 建客户端
	fan := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("PNG" + r.URL.Path))
			return
		}
		if r.Header.Get("api-key") != "fanartkey0123456789abcdef0123456" || r.URL.Path != "/v3.2/movies/872585" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hdmovieclearart":[{"url":"http://` + r.Host + `/assets/clearart.png","lang":"en","likes":"3"}]}`))
	}))
	t.Cleanup(fan.Close)
	rec := recognize.New(recognize.Config{
		DB: e.db, Cipher: fakeCipher{}, BaseURL: e.tmdb.URL + "/3", ImageBaseURL: e.tmdb.URL + "/t/p/", RatePerSecond: 1000,
		FanartBaseURL: fan.URL + "/v3.2", FanartImageHosts: []string{fan.Listener.Addr().String()},
	})
	key := "fanartkey0123456789abcdef0123456"
	_, err = rec.SaveSettings(e.ctx, recognize.SettingsInput{FanartKey: &key})
	require.NoError(t, err)
	e.svc.cfg.Recognizer = rec
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeItems: []string{"clearart", "banner"}})
	e.oppenheimer()
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	assert.Empty(t, res.Messages, "Fanart.tv 上没有横幅：不写，也不算出错")
	assert.Equal(t, "PNG/assets/clearart.png", readFile(t, filepath.Join(e.movies, "奥本海默 (2023)", "clearart.png")))
	assert.False(t, exists(filepath.Join(e.movies, "奥本海默 (2023)", "banner.jpg")))
	var row models.MediaTransferHistory
	for _, r := range e.history() {
		if r.MediaType == "movie" {
			row = r
		}
	}
	assert.Equal(t, models.MediaScrapePartial, row.ScrapeStatus, "选了的横幅没写成")
	assert.Equal(t, "来源里没有横幅", row.ScrapeNote)
}

// 只有 Fanart.tv 有的图片、没填它的 Key：记成部分刮削，说明里写怎么补上，不重复写「来源里没有」
func TestScrapeFanartOnlyWithoutKey(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeItems: []string{"nfo", "clearart"}})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	rec := e.history()[0]
	assert.Equal(t, models.MediaScrapePartial, rec.ScrapeStatus)
	assert.Equal(t, "Clearart 只有 Fanart.tv 有：在「媒体识别」里填了 Fanart.tv 的 API Key 才写", rec.ScrapeNote)
}

// 升级前的记录没记算目录的起点：没有按二级分类放进子目录时是库目录；否则看记下的刮削文件、剧集目录里的 tvshow.nfo，
// 再按模板的级数认，认不出时不猜
func TestScrapeRoot(t *testing.T) {
	root := t.TempDir()
	lib := models.MediaLibrary{Path: root, Kind: models.MediaKindTV, ClassifyDir: true}
	tvTpl := DefaultTemplate(models.MediaKindTV)
	ep := filepath.Join(root, "日漫", "芙莉莲 (2023)", "Season 1", "S01E01.mkv")
	row := models.MediaTransferHistory{MediaType: "tv", TargetPath: ep}

	assert.Equal(t, filepath.Join(root, "日漫"), scrapeRoot(row, lib, tvTpl), "按模板的级数：分类目录")
	assert.Equal(t, 3, templateDepth(tvTpl))
	flat := models.MediaTransferHistory{MediaType: "tv", TargetPath: filepath.Join(root, "芙莉莲 (2023)", "Season 1", "S01E01.mkv")}
	assert.Equal(t, root, scrapeRoot(flat, lib, tvTpl), "没分类的：库目录")
	assert.Empty(t, scrapeRoot(row, lib, "{{.Title}}/{{.SeasonEpisode}}"), "级数都对不上：不猜")

	withRoot := row
	withRoot.RootPath = filepath.Join(root, "日漫")
	assert.Equal(t, withRoot.RootPath, scrapeRoot(withRoot, lib, "{{.Title}}"), "整理时记下的优先")
	plain := lib
	plain.ClassifyDir = false
	assert.Equal(t, root, scrapeRoot(row, plain, "{{.Title}}"))

	// 记下的 tvshow.nfo
	nfo := row
	nfo.Extras = metaExtras(t, filepath.Join(root, "日漫", "芙莉莲 (2023)", "tvshow.nfo"))
	assert.Equal(t, filepath.Join(root, "日漫"), scrapeRoot(nfo, lib, "{{.Title}}"))
	// 剧集目录里有 tvshow.nfo
	require.NoError(t, os.MkdirAll(filepath.Join(root, "日漫", "芙莉莲 (2023)"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "日漫", "芙莉莲 (2023)", "tvshow.nfo"), nil, 0o644))
	assert.Equal(t, filepath.Join(root, "日漫"), scrapeRoot(row, lib, "{{.Title}}"))

	// 电影：目录里记下的海报说明有自己的目录，带文件名的说明平铺
	mlib := models.MediaLibrary{Path: root, Kind: models.MediaKindMovie, ClassifyDir: true}
	movie := models.MediaTransferHistory{MediaType: "movie", TargetPath: filepath.Join(root, "欧美", "M (2023)", "M.mkv")}
	movie.Extras = metaExtras(t, filepath.Join(root, "欧美", "M (2023)", "poster.jpg"))
	assert.Equal(t, filepath.Join(root, "欧美"), scrapeRoot(movie, mlib, "{{.Title}}"))
	flatMovie := models.MediaTransferHistory{MediaType: "movie", TargetPath: filepath.Join(root, "欧美", "M.mkv")}
	flatMovie.Extras = metaExtras(t, filepath.Join(root, "欧美", "M-poster.jpg"))
	assert.Equal(t, filepath.Join(root, "欧美"), scrapeRoot(flatMovie, mlib, "{{.Title}}/{{.Title}}"))
}
