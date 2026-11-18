package servarr

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// seerr 照 Seerr dca1af6a 的 server/api/servarr/{base,radarr,sonarr}.ts 写的客户端：同样的方法、路径、参数与请求体，
// 同样的判断（addMovie、addSeries 的分支）。契约测试经它调兼容入口，看 Seerr 真这样调时 pt-tools 怎么回。
type seerr struct {
	t   *testing.T
	e   *env
	app string // radarr、sonarr
	// calls 记下每个请求（方法与路径，不含查询参数）
	calls []string
}

func (e *env) seerr(app string) *seerr { return &seerr{t: e.t, e: e, app: app} }

// do 发一个请求；非 2xx 时返回错误（Seerr 的 axios 把非 2xx 当作失败）。
func (c *seerr) do(method, path string, params url.Values, body any) (response, error) {
	c.t.Helper()
	full := "/servarr/" + c.app + apiPath + path
	q := url.Values{}
	for k, v := range params {
		q[k] = v
	}
	q.Set("apikey", c.e.key)
	c.calls = append(c.calls, method+" "+path)
	r := c.e.call(method, full+"?"+q.Encode(), "", body)
	if r.status < 200 || r.status > 299 {
		return r, fmt.Errorf("Request failed with status code %d: %s", r.status, r.body)
	}
	return r, nil
}

func (c *seerr) get(path string, params url.Values) (any, error) {
	r, err := c.do(http.MethodGet, path, params, nil)
	if err != nil {
		return nil, err
	}
	return r.json(c.t), nil
}

// base.ts
func (c *seerr) getSystemStatus() (map[string]any, error) {
	v, err := c.get("/system/status", nil)
	if err != nil {
		return nil, err
	}
	return v.(map[string]any), nil
}

func (c *seerr) getProfiles() ([]any, error)    { return c.list("/qualityProfile", nil) }
func (c *seerr) getRootFolders() ([]any, error) { return c.list("/rootfolder", nil) }
func (c *seerr) getTags() ([]any, error)        { return c.list("/tag", nil) }

func (c *seerr) getQueue() ([]any, error) {
	v, err := c.get("/queue", url.Values{"includeEpisode": {"true"}})
	if err != nil {
		return nil, err
	}
	return v.(map[string]any)["records"].([]any), nil
}

func (c *seerr) createTag(label string) (map[string]any, error) {
	r, err := c.do(http.MethodPost, "/tag", nil, map[string]any{"label": label})
	if err != nil {
		return nil, err
	}
	return r.obj(c.t), nil
}

func (c *seerr) renameTag(id int, label string) (map[string]any, error) {
	r, err := c.do(http.MethodPut, "/tag/"+strconv.Itoa(id), nil, map[string]any{"id": id, "label": label})
	if err != nil {
		return nil, err
	}
	return r.obj(c.t), nil
}

func (c *seerr) runCommand(name string, options map[string]any) error {
	body := map[string]any{"name": name}
	for k, v := range options {
		body[k] = v
	}
	_, err := c.do(http.MethodPost, "/command", nil, body)
	return err
}

func (c *seerr) refreshMonitoredDownloads() error {
	return c.runCommand("RefreshMonitoredDownloads", nil)
}

func (c *seerr) list(path string, params url.Values) ([]any, error) {
	v, err := c.get(path, params)
	if err != nil {
		return nil, err
	}
	return v.([]any), nil
}

// radarr.ts
func (c *seerr) getMovies() ([]any, error) { return c.list("/movie", nil) }

func (c *seerr) getLibraryMoviesByTmdbID(id int) ([]any, error) {
	return c.list("/movie", url.Values{"tmdbId": {strconv.Itoa(id)}})
}

func (c *seerr) getMovie(id int) (map[string]any, error) {
	v, err := c.get("/movie/"+strconv.Itoa(id), nil)
	if err != nil {
		return nil, err
	}
	return v.(map[string]any), nil
}

func (c *seerr) getMovieByTmdbID(id int) (map[string]any, error) {
	l, err := c.list("/movie/lookup", url.Values{"term": {"tmdb:" + strconv.Itoa(id)}})
	if err != nil {
		return nil, err
	}
	if len(l) == 0 {
		return nil, fmt.Errorf("Movie not found")
	}
	return l[0].(map[string]any), nil
}

// radarrMovieOptions 是 RadarrMovieOptions。
type radarrMovieOptions struct {
	title               string
	qualityProfileID    int
	minimumAvailability string
	tags                []int
	profileID           int
	year                int
	rootFolderPath      string
	tmdbID              int
	monitored           bool
	searchNow           bool
}

func num(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	}
	return 0
}

func ints(v any) []int {
	var out []int
	for _, x := range v.([]any) {
		out = append(out, num(x))
	}
	return out
}

func union(a, b []int) []int {
	out := slices.Clone(a)
	for _, x := range b {
		if !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

// addMovie 是 RadarrAPI.addMovie。
func (c *seerr) addMovie(o radarrMovieOptions) (map[string]any, error) {
	movie, err := c.getMovieByTmdbID(o.tmdbID)
	if err != nil {
		return nil, err
	}
	if movie["hasFile"] == true {
		return movie, nil
	}
	if num(movie["id"]) != 0 && movie["monitored"] != true {
		body := map[string]any{}
		for k, v := range movie {
			body[k] = v
		}
		body["title"], body["qualityProfileId"], body["profileId"] = o.title, o.qualityProfileID, o.profileID
		body["titleSlug"], body["minimumAvailability"], body["tmdbId"] = strconv.Itoa(o.tmdbID), o.minimumAvailability, o.tmdbID
		body["year"], body["tags"], body["rootFolderPath"] = o.year, union(ints(movie["tags"]), o.tags), o.rootFolderPath
		body["monitored"], body["addOptions"] = o.monitored, map[string]any{"searchForMovie": o.searchNow}
		r, perr := c.do(http.MethodPut, "/movie", nil, body)
		if perr != nil {
			return nil, perr
		}
		data := r.obj(c.t)
		if data["monitored"] != true {
			return nil, fmt.Errorf("Failed to update existing movie in Radarr")
		}
		if o.searchNow && data["isAvailable"] == true {
			c.searchMovie(num(data["id"]))
		}
		return data, nil
	}
	if num(movie["id"]) != 0 {
		if o.searchNow && movie["hasFile"] != true && movie["isAvailable"] == true {
			c.searchMovie(num(movie["id"]))
		}
		return movie, nil
	}
	r, err := c.do(http.MethodPost, "/movie", nil, map[string]any{
		"title": o.title, "qualityProfileId": o.qualityProfileID, "profileId": o.profileID, "titleSlug": strconv.Itoa(o.tmdbID),
		"minimumAvailability": o.minimumAvailability, "tmdbId": o.tmdbID, "year": o.year, "rootFolderPath": o.rootFolderPath,
		"monitored": o.monitored, "tags": o.tags, "addOptions": map[string]any{"searchForMovie": o.searchNow},
	})
	if err != nil {
		return nil, err
	}
	data := r.obj(c.t)
	if num(data["id"]) == 0 {
		return nil, fmt.Errorf("Failed to add movie to Radarr")
	}
	return data, nil
}

// searchMovie 是 RadarrAPI.searchMovie（Seerr 只记日志，这里出错时让测试失败）。
func (c *seerr) searchMovie(id int) {
	c.t.Helper()
	require.NoError(c.t, c.runCommand("MoviesSearch", map[string]any{"movieIds": []int{id}}))
}

// removeMovie 是 RadarrAPI.removeMovie。
func (c *seerr) removeMovie(tmdbID int) error {
	m, err := c.getMovieByTmdbID(tmdbID)
	if err != nil {
		return err
	}
	if num(m["id"]) == 0 {
		return nil
	}
	r, err := c.do(http.MethodDelete, "/movie/"+strconv.Itoa(num(m["id"])), url.Values{"deleteFiles": {"true"}, "addImportExclusion": {"false"}}, nil)
	if err != nil && r.status == http.StatusNotFound {
		return nil
	}
	return err
}

// sonarr.ts
func (c *seerr) getSeries() ([]any, error) { return c.list("/series", nil) }

func (c *seerr) getLibrarySeriesByTvdbID(id int) ([]any, error) {
	return c.list("/series", url.Values{"tvdbId": {strconv.Itoa(id)}})
}

func (c *seerr) getSeriesByID(id int) (map[string]any, error) {
	v, err := c.get("/series/"+strconv.Itoa(id), nil)
	if err != nil {
		return nil, err
	}
	return v.(map[string]any), nil
}

func (c *seerr) getSeriesByTitle(title string) ([]any, error) {
	l, err := c.list("/series/lookup", url.Values{"term": {title}})
	if err != nil {
		return nil, err
	}
	if len(l) == 0 {
		return nil, fmt.Errorf("No series found")
	}
	return l, nil
}

func (c *seerr) getSeriesByTvdbID(id int) (map[string]any, error) {
	l, err := c.list("/series/lookup", url.Values{"term": {"tvdb:" + strconv.Itoa(id)}})
	if err != nil {
		return nil, err
	}
	if len(l) == 0 {
		return nil, fmt.Errorf("Series not found")
	}
	return l[0].(map[string]any), nil
}

// addSeriesOptions 是 AddSeriesOptions。
type addSeriesOptions struct {
	tvdbID            int
	title             string
	profileID         int
	languageProfileID *int
	seasons           []int
	seasonFolder      bool
	rootFolderPath    string
	tags              []int
	seriesType        string
	monitored         *bool
	monitorNewItems   string
	searchNow         bool
}

// buildSeasonList 是 SonarrAPI.buildSeasonList。
func buildSeasonList(seasons []int, existing []any) []any {
	if existing != nil {
		for _, s := range existing {
			m := s.(map[string]any)
			if slices.Contains(seasons, num(m["seasonNumber"])) {
				m["monitored"] = true
			}
		}
		return existing
	}
	out := []any{}
	for _, n := range seasons {
		out = append(out, map[string]any{"seasonNumber": n, "monitored": true})
	}
	return out
}

// addSeries 是 SonarrAPI.addSeries。
func (c *seerr) addSeries(o addSeriesOptions) (map[string]any, error) {
	series, err := c.getSeriesByTvdbID(o.tvdbID)
	if err != nil {
		return nil, err
	}
	if num(series["id"]) != 0 {
		if o.monitored != nil {
			series["monitored"] = *o.monitored
		}
		if o.tags != nil {
			series["tags"] = union(ints(series["tags"]), o.tags)
		}
		series["seasons"] = buildSeasonList(o.seasons, series["seasons"].([]any))
		r, perr := c.do(http.MethodPut, "/series", nil, series)
		if perr != nil {
			return nil, perr
		}
		data := r.obj(c.t)
		if num(data["id"]) == 0 {
			return nil, fmt.Errorf("Failed to update series in Sonarr")
		}
		// 重新监控请求的季里没监控的集（失败只记日志）
		if eps, eerr := c.getEpisodes(num(data["id"])); eerr == nil {
			var ids []int
			for _, ep := range eps {
				m := ep.(map[string]any)
				if slices.Contains(o.seasons, num(m["seasonNumber"])) && m["monitored"] != true {
					ids = append(ids, num(m["id"]))
				}
			}
			if len(ids) > 0 {
				_, _ = c.do(http.MethodPut, "/episode/monitor", nil, map[string]any{"episodeIds": ids, "monitored": true})
			}
		}
		if o.searchNow {
			c.searchSeries(num(data["id"]))
		}
		return data, nil
	}
	existing := []any{}
	for _, s := range series["seasons"].([]any) {
		existing = append(existing, map[string]any{"seasonNumber": num(s.(map[string]any)["seasonNumber"]), "monitored": false})
	}
	body := map[string]any{
		"tvdbId": o.tvdbID, "title": o.title, "qualityProfileId": o.profileID,
		"seasons": buildSeasonList(o.seasons, existing), "tags": o.tags, "seasonFolder": o.seasonFolder,
		"monitorNewItems": o.monitorNewItems, "rootFolderPath": o.rootFolderPath, "seriesType": o.seriesType,
		"addOptions": map[string]any{"ignoreEpisodesWithFiles": true, "searchForMissingEpisodes": o.searchNow},
	}
	// JSON 里没有 undefined：Seerr 不传的字段不写
	if o.languageProfileID != nil {
		body["languageProfileId"] = *o.languageProfileID
	}
	if o.monitored != nil {
		body["monitored"] = *o.monitored
	}
	r, err := c.do(http.MethodPost, "/series", nil, body)
	if err != nil {
		return nil, err
	}
	data := r.obj(c.t)
	if num(data["id"]) == 0 {
		return nil, fmt.Errorf("Failed to add series to Sonarr")
	}
	return data, nil
}

func (c *seerr) getLanguageProfiles() ([]any, error) { return c.list("/languageprofile", nil) }

// searchSeries 是 SonarrAPI.searchSeries（Seerr 只记日志，这里出错时让测试失败）。
func (c *seerr) searchSeries(id int) {
	c.t.Helper()
	require.NoError(c.t, c.runCommand("MissingEpisodeSearch", map[string]any{"seriesId": id}))
}

func (c *seerr) getEpisodes(seriesID int) ([]any, error) {
	return c.list("/episode", url.Values{"seriesId": {strconv.Itoa(seriesID)}})
}

// removeSeries 是 SonarrAPI.removeSeries。
func (c *seerr) removeSeries(tvdbID int) error {
	s, err := c.getSeriesByTvdbID(tvdbID)
	if err != nil {
		return err
	}
	if num(s["id"]) == 0 {
		return nil
	}
	r, err := c.do(http.MethodDelete, "/series/"+strconv.Itoa(num(s["id"])), url.Values{"deleteFiles": {"true"}, "addImportExclusion": {"false"}}, nil)
	if err != nil && r.status == http.StatusNotFound {
		return nil
	}
	return err
}
