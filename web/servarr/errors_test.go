package servarr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

func TestNoTMDB(t *testing.T) {
	e := newEnv(t)
	_, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}, models.MediaSubFromManual)
	require.NoError(t, err)
	d := e.srv.deps
	d.TMDB = nil
	e.h = New(d).Handler()
	for _, path := range []string{"/movie/lookup?term=tmdb:157336"} {
		r := e.radarr(http.MethodGet, path, nil)
		requireStatus(t, http.StatusServiceUnavailable, r)
		assert.Contains(t, r.obj(t)["message"], "还没有设置 TMDB API Key")
	}
	for _, path := range []string{"/series/100088", "/series?tvdbId=392256", "/series/lookup?term=tvdb:392256", "/series/lookup?term=Last"} {
		r := e.sonarr(http.MethodGet, path, nil)
		requireStatus(t, http.StatusServiceUnavailable, r)
		assert.Contains(t, r.obj(t)["message"], "还没有设置 TMDB API Key", path)
	}
	assert.Equal(t, []any{}, e.sonarr(http.MethodGet, "/series", nil).list(t))
	requireStatus(t, http.StatusServiceUnavailable, e.sonarr(http.MethodPost, "/series", addSeriesBody(392256, autoRootPath, map[int]bool{1: true}, true, true)))
}

func TestStorageErrors(t *testing.T) {
	e := newEnv(t)
	sub, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 157336}, models.MediaSubFromManual)
	require.NoError(t, err)
	_, err = e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}, models.MediaSubFromManual)
	require.NoError(t, err)

	// 媒体库读不到
	d := e.srv.deps
	d.Libraries = func(context.Context) ([]models.MediaLibrary, error) { return nil, errors.New("boom") }
	e.h = New(d).Handler()
	for _, path := range []string{"/rootfolder", "/movie", "/movie/" + formatID(sub.ID), "/movie/lookup?term=tmdb:157336"} {
		assert.Equal(t, http.StatusInternalServerError, e.radarr(http.MethodGet, path, nil).status, path)
	}
	assert.Equal(t, http.StatusInternalServerError, e.radarr(http.MethodPost, "/movie", addMovieBody(27205, e.movieLib.Path, true, true)).status)
	for _, path := range []string{"/series", "/series/100088", "/series?tvdbId=392256", "/series/lookup?term=tvdb:392256"} {
		assert.GreaterOrEqual(t, e.sonarr(http.MethodGet, path, nil).status, 500, path)
	}
	e.h = e.srv.Handler()

	// 标签表坏了
	require.NoError(t, e.db.Exec("DROP TABLE servarr_tag_links").Error)
	assert.Equal(t, []any{}, e.radarr(http.MethodGet, "/movie", nil).list(t)[0].(map[string]any)["tags"], "读不到标签时写空数组（记日志）")
	requireStatus(t, http.StatusInternalServerError, e.radarr(http.MethodPut, "/movie", map[string]any{"id": sub.ID, "tags": []int{}}))
	requireStatus(t, http.StatusOK, e.radarr(http.MethodDelete, "/movie/"+formatID(sub.ID), nil))
	require.NoError(t, e.db.Exec("DROP TABLE servarr_tags").Error)
	for _, r := range []response{
		e.radarr(http.MethodGet, "/tag", nil),
		e.radarr(http.MethodPost, "/tag", map[string]any{"label": "x"}),
		e.radarr(http.MethodPut, "/tag/1", map[string]any{"label": "x"}),
	} {
		requireStatus(t, http.StatusInternalServerError, r)
		assert.Contains(t, r.obj(t)["message"], "标签")
	}

	// 质量档案、订阅读不到
	require.NoError(t, e.db.Exec("DROP TABLE media_quality_profiles").Error)
	requireStatus(t, http.StatusInternalServerError, e.radarr(http.MethodGet, "/qualityProfile", nil))
	require.NoError(t, e.db.Exec("DROP TABLE media_subscriptions").Error)
	for _, r := range []response{
		e.radarr(http.MethodGet, "/movie", nil),
		e.radarr(http.MethodGet, "/movie/1", nil),
		e.radarr(http.MethodGet, "/movie/lookup?term=tmdb:157336", nil),
		e.radarr(http.MethodPost, "/movie", addMovieBody(157336, autoRootPath, true, true)),
		e.radarr(http.MethodDelete, "/movie/1", nil),
		e.sonarr(http.MethodGet, "/series", nil),
		e.sonarr(http.MethodGet, "/series/100088", nil),
		e.sonarr(http.MethodGet, "/series?tvdbId=392256", nil),
		e.sonarr(http.MethodGet, "/episode?seriesId=100088", nil),
		e.sonarr(http.MethodPost, "/series", addSeriesBody(392256, autoRootPath, map[int]bool{1: true}, true, true)),
		e.sonarr(http.MethodPut, "/series", map[string]any{"id": 100088}),
		e.sonarr(http.MethodDelete, "/series/100088", nil),
		e.sonarr(http.MethodPost, "/command", map[string]any{"name": "MissingEpisodeSearch", "seriesId": 100088}),
	} {
		assert.GreaterOrEqual(t, r.status, 500, "%s", r.body)
	}
	assert.Equal(t, http.StatusNotFound, e.radarr(http.MethodPost, "/command", map[string]any{"name": "MoviesSearch", "movieIds": []int{1}}).status,
		"读不到订阅的编号当作没有")
}

func TestUnavailableTokens(t *testing.T) {
	e := newEnv(t)
	d := e.srv.deps
	d.Tokens = nil
	e.h = New(d).Handler()
	requireStatus(t, http.StatusUnauthorized, e.radarr(http.MethodGet, "/system/status", nil))
	// 令牌库读不到：503，不算错 Key
	require.NoError(t, e.db.Exec("DROP TABLE api_tokens").Error)
	e.h = e.srv.Handler()
	r := e.radarr(http.MethodGet, "/system/status", nil)
	requireStatus(t, http.StatusServiceUnavailable, r)
	assert.Contains(t, r.obj(t)["message"], "暂时不能校验")
	assert.False(t, e.srv.fails.locked("192.0.2.10", e.Now()))
}

func TestWriteSubError(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{errBadRequest("x"), http.StatusBadRequest},
		{subscribe.ErrInvalid, http.StatusBadRequest},
		{subscribe.ErrNotFound, http.StatusNotFound},
		{fmt.Errorf("取详情: %w", tmdb.ErrNotFound), http.StatusNotFound},
		// 建订阅时要取 TMDB 的详情：没设 Key 时是 503（Seerr 过后重试），不是 500
		{fmt.Errorf("识别: %w", tmdb.ErrNoKey), http.StatusServiceUnavailable},
		{errors.New("boom"), http.StatusInternalServerError},
	}
	newEnv(t) // 500 时写日志
	for _, c := range cases {
		rec := newRecorder()
		writeSubError(rec, c.err, "订阅")
		assert.Equal(t, c.want, rec.Code, c.err.Error())
	}
}
