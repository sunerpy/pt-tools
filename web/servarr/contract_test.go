package servarr

import (
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
)

// contract 是 testdata/seerr-types.json：Seerr dca1af6a 的 TypeScript 类型里 Seerr 读到的字段与它们的 JSON 类型。
type contract map[string]map[string]string

func loadContract(t *testing.T) contract {
	t.Helper()
	b, err := os.ReadFile("testdata/seerr-types.json")
	require.NoError(t, err)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &raw))
	out := contract{}
	for k, v := range raw {
		if k == "_source" {
			continue
		}
		var fields map[string]string
		require.NoError(t, json.Unmarshal(v, &fields), k)
		out[k] = fields
	}
	return out
}

func jsonType(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case nil:
		return "null"
	}
	return "?"
}

// check 断言 v 有类型 name 的每个字段、JSON 类型对得上；except 里的字段可以没有（查找的结果不带 id）。
func (c contract) check(t *testing.T, name string, v any, except ...string) {
	t.Helper()
	fields, ok := c[name]
	require.True(t, ok, "契约里没有 %s", name)
	obj, ok := v.(map[string]any)
	require.True(t, ok, "%s 不是对象：%v", name, v)
	for f, typ := range fields {
		got, has := obj[f]
		if !has && slices.Contains(except, f) {
			continue
		}
		if assert.True(t, has, "%s 缺字段 %s", name, f) {
			assert.Equal(t, typ, jsonType(got), "%s.%s 的类型", name, f)
		}
	}
}

func (c contract) checkSeries(t *testing.T, v any, except ...string) {
	t.Helper()
	c.check(t, "SonarrSeries", v, except...)
	s := v.(map[string]any)
	c.check(t, "SonarrSeriesStatistics", s["statistics"])
	for _, ss := range s["seasons"].([]any) {
		c.check(t, "SonarrSeason", ss)
		c.check(t, "SonarrSeasonStatistics", ss.(map[string]any)["statistics"])
	}
}

func TestContractShapes(t *testing.T) {
	e := newEnv(t)
	c := loadContract(t)
	_, err := e.subs.SaveProfile(e.ctx, 0, subscribe.ProfileInput{Name: "1080p"})
	require.NoError(t, err)

	for _, call := range []func(string, string, any) response{e.radarr, e.sonarr} {
		c.check(t, "SystemStatus", call(http.MethodGet, "/system/status", nil).json(t))
		for _, p := range call(http.MethodGet, "/qualityProfile", nil).list(t) {
			c.check(t, "QualityProfile", p)
		}
		for _, f := range call(http.MethodGet, "/rootfolder", nil).list(t) {
			c.check(t, "RootFolder", f)
		}
		c.check(t, "QueueResponse", call(http.MethodGet, "/queue?includeEpisode=true", nil).json(t))
	}
	tg := e.radarr(http.MethodPost, "/tag", map[string]any{"label": "1-admin"})
	requireStatus(t, http.StatusCreated, tg)
	c.check(t, "Tag", tg.json(t))
	for _, x := range e.sonarr(http.MethodGet, "/tag", nil).list(t) {
		c.check(t, "Tag", x)
	}
	for _, lp := range e.sonarr(http.MethodGet, "/languageprofile", nil).list(t) {
		c.check(t, "LanguageProfile", lp)
	}

	// Radarr：查找（没有订阅，不带 id）、新建、列表、单个、更新
	look := e.radarr(http.MethodGet, "/movie/lookup?term=tmdb:157336", nil).list(t)
	require.Len(t, look, 1)
	c.check(t, "RadarrMovie", look[0], "id")
	add := e.radarr(http.MethodPost, "/movie", addMovieBody(157336, autoRootPath, true, true))
	requireStatus(t, http.StatusCreated, add)
	c.check(t, "RadarrMovie", add.json(t))
	for _, m := range e.radarr(http.MethodGet, "/movie", nil).list(t) {
		c.check(t, "RadarrMovie", m)
	}
	id := num(add.obj(t)["id"])
	c.check(t, "RadarrMovie", e.radarr(http.MethodGet, "/movie/"+formatID(uint(id)), nil).json(t))
	put := e.radarr(http.MethodPut, "/movie", map[string]any{"id": id, "monitored": true})
	requireStatus(t, http.StatusOK, put)
	c.check(t, "RadarrMovie", put.json(t))

	// Sonarr：查找、新建、列表、单个、更新、分集
	slook := e.sonarr(http.MethodGet, "/series/lookup?term=tvdb:392256", nil).list(t)
	require.Len(t, slook, 1)
	c.checkSeries(t, slook[0], "id")
	sadd := e.sonarr(http.MethodPost, "/series", addSeriesBody(392256, autoRootPath, map[int]bool{2: true}, true, true))
	requireStatus(t, http.StatusCreated, sadd)
	c.checkSeries(t, sadd.json(t))
	for _, s := range e.sonarr(http.MethodGet, "/series", nil).list(t) {
		c.checkSeries(t, s)
	}
	c.checkSeries(t, e.sonarr(http.MethodGet, "/series/100088", nil).json(t))
	sput := e.sonarr(http.MethodPut, "/series", map[string]any{"id": 100088, "seasons": []map[string]any{{"seasonNumber": 2, "monitored": true}}})
	requireStatus(t, http.StatusOK, sput)
	c.checkSeries(t, sput.json(t))
	eps := e.sonarr(http.MethodGet, "/episode?seriesId=100088", nil).list(t)
	require.NotEmpty(t, eps)
	for _, ep := range eps {
		c.check(t, "EpisodeResult", ep)
	}
	for _, s := range e.sonarr(http.MethodGet, "/series/lookup?term=Last", nil).list(t) {
		c.checkSeries(t, s, "id")
	}
	assert.NotNil(t, e.sub(models.MediaKindTV, 100088, 2))
}
