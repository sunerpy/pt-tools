package organize

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/metadata"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
)

// 每一集选了 TheTVDB、TMDB 上刚播出的两集还没有标题、简介与截图：NFO 用 TheTVDB 的中文标题与简介，截图用 TheTVDB 的，
// 刮削好了。TheTVDB 的 Key 填错时写明不能用，每一集与截图用 TMDB 的。
func TestScrapeEpisodesFromTVDB(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.tmdb.mu.Lock()
	e.tmdb.bareEpisodes, e.tmdb.tvdbID = true, 392256
	e.tmdb.mu.Unlock()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v4/login":
			_, _ = w.Write([]byte(`{"data":{"token":"tok"}}`))
		case strings.HasPrefix(r.URL.Path, "/banners/"):
			assert.Empty(t, r.Header.Get("Authorization"), "图片请求不带令牌")
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("TVDB" + r.URL.Path))
		case r.URL.Path == "/v4/series/392256/episodes/official/zho" && r.Header.Get("Authorization") == "Bearer tok":
			ep := func(n int, name, plot string) string {
				return fmt.Sprintf(`{"id":%d,"seasonNumber":1,"number":%d,"name":%q,"overview":%q,"aired":"2023-01-%d","image":"%s/banners/e%d.jpg","nameTranslations":["zho"],"overviewTranslations":["zho"]}`,
					900+n, n, name, plot, 8+7*n, srv.URL, n)
			}
			_, _ = w.Write([]byte(`{"data":{"series":{"id":392256,"originalLanguage":"eng","episodes":[` +
				ep(1, "当你迷失在黑暗中", "乔尔与艾莉相遇。") + `,` + ep(2, "感染", "他们穿过城市。") + `]}},"links":{"next":null}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	rec := recognize.New(recognize.Config{
		DB: e.db, Cipher: fakeCipher{}, BaseURL: e.tmdb.URL + "/3", ImageBaseURL: e.tmdb.URL + "/t/p/", RatePerSecond: 1000,
		TVDBBaseURL: srv.URL + "/v4", TVDBImageHosts: []string{strings.TrimPrefix(srv.URL, "http://")},
	})
	tmdbK, tvdbK := tmdbKey, "aaaa-bbbb-cccc"
	_, err := rec.SaveSettings(context.Background(), recognize.SettingsInput{TMDBKey: &tmdbK, TVDBKey: &tvdbK})
	require.NoError(t, err)
	e.svc.cfg.Recognizer = rec
	e.library(LibraryInput{
		Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeItems: []string{"nfo", "thumb"},
		ScrapeSources: &metadata.Sources{Episode: metadata.SourceTVDB},
	})
	e.lastOfUs()
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	season := filepath.Join(e.tv, "最后生还者 (2023)", "Season 1")
	nfos, err := filepath.Glob(filepath.Join(season, "*S01E01*.nfo"))
	require.NoError(t, err)
	require.Len(t, nfos, 1)
	nfo := readFile(t, nfos[0])
	assert.Contains(t, nfo, "<title>当你迷失在黑暗中</title>")
	assert.Contains(t, nfo, "<plot>乔尔与艾莉相遇。</plot>")
	thumbs, err := filepath.Glob(filepath.Join(season, "*S01E02*-thumb.jpg"))
	require.NoError(t, err)
	require.Len(t, thumbs, 1)
	assert.Equal(t, "TVDB/banners/e2.jpg", readFile(t, thumbs[0]))
	for _, r := range e.history() {
		assert.Equal(t, models.MediaScrapeOK, r.ScrapeStatus, r.ScrapeNote)
	}

	// Key 不对（登录 401）：写明 TheTVDB 不能用，用 TMDB 的（这两集没有标题与截图，记成部分刮削）
	bad := "zzzz-yyyy-xxxx"
	_, err = rec.SaveSettings(context.Background(), recognize.SettingsInput{TVDBKey: &bad})
	require.NoError(t, err)
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	rows := e.history()
	require.NotEmpty(t, rows)
	res, err := e.svc.Rescrape(e.ctx, rows[0].ID)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(res.Messages, "\n"), "TheTVDB 取不到")
	for _, r := range e.history() {
		assert.Equal(t, models.MediaScrapePartial, r.ScrapeStatus)
	}
}
