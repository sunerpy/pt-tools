package qbitcompat

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/models"
)

// markCompat 把种子记成经兼容入口加的（写接口默认只动这些）。
func (e *env) markCompat(hash string) {
	e.t.Helper()
	h := hash
	require.NoError(e.t, e.db.Create(&models.TorrentInfo{SiteName: "qa", TorrentID: "hash:" + hash, TorrentHash: &h, DownloadSource: Source}).Error)
}

// 默认只动经兼容入口加的种子：别的跳过、照样回 200，审计记 denied:not_compat；hashes=all 也一样；打开完全控制后不限
func TestWritesScopedToCompatTorrents(t *testing.T) {
	e := newEnv(t)
	plain := e.token(apitoken.ScopeQbitCompat)
	ck := e.login(plain)
	e.markCompat(hashMovie)

	w := e.do(http.MethodPost, "/api/v2/torrents/pause", url.Values{"hashes": {hashDebian + "|" + hashMovie + "|ffff"}}, ck)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Empty(t, w.Body.String())
	w = e.do(http.MethodPost, "/api/v2/torrents/resume", url.Values{"hashes": {"all"}}, ck)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"pause " + hashMovie, "resume " + hashMovie}, e.dl.got())

	audit := e.audit.all()
	require.Len(t, audit, 2)
	assert.Equal(t, ChannelType, audit[0].ChannelType)
	assert.Equal(t, "POST /api/v2/torrents/pause", audit[0].Command)
	assert.Equal(t, "denied:not_compat", audit[0].Result)
	assert.EqualValues(t, 1, audit[0].Args["count"])
	assert.EqualValues(t, 1, audit[0].Args["denied"])
	assert.Equal(t, "moviepilot", audit[0].Args["username"])
	assert.NotEmpty(t, audit[0].ChannelUserID)

	_, err := SaveSettings(t.Context(), e.db, Settings{FullControl: true})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/stop", url.Values{"hashes": {"all"}}, ck).Code)
	assert.Equal(t, "pause "+hashMovie+",7,"+hashDebian, e.dl.got()[2], "完全控制：全部种子（按 hash 排序，Transmission 的种子用它自己的编号）")
	assert.Equal(t, "success", e.audit.all()[2].Result)

	assert.Equal(t, http.StatusMethodNotAllowed, e.do(http.MethodGet, "/api/v2/torrents/pause", nil, ck).Code)
}

// 删除（deleteFiles）、改分类（qB 后端先建分类）、加标签（原有的加上新的）、去标签（qB 用 removeTags）
func TestWriteOperations(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	e.markCompat(hashMovie)
	require.NoError(t, e.db.Create(&models.QbitCompatSetting{ID: 1, Categories: `{"sonarr":"/tv"}`}).Error)

	post := func(path string, form url.Values) {
		t.Helper()
		form.Set("hashes", hashMovie)
		w := e.do(http.MethodPost, path, form, ck)
		require.Equal(t, http.StatusOK, w.Code, path+" "+w.Body.String())
	}
	post("/api/v2/torrents/delete", url.Values{"deleteFiles": {"true"}})
	post("/api/v2/torrents/setCategory", url.Values{"category": {"sonarr"}})
	post("/api/v2/torrents/addTags", url.Values{"tags": {"4k, new"}})
	post("/api/v2/torrents/removeTags", url.Values{"tags": {"hdsky"}})
	assert.Equal(t, []string{
		"remove " + hashMovie + " data=true",
		"createCategory sonarr=/tv",
		"category " + hashMovie + "=sonarr",
		"tags " + hashMovie + "=hdsky,4k,new",
		"removeTags " + hashMovie + "=hdsky",
	}, e.dl.got())
	assert.Equal(t, true, e.audit.all()[0].Args["delete_files"])
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/api/v2/torrents/addTags", url.Values{"hashes": {hashMovie}, "tags": {" , "}}, ck).Code)
}

// 后端是 Transmission：去标签改写成剩下的标签；tags 为空时去掉全部；改分类不建分类
func TestWriteOperationsTransmission(t *testing.T) {
	e := newEnv(t)
	e.asTR = true
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	e.markCompat(hashMovie)
	for _, form := range []url.Values{
		{"tags": {"hdsky"}},
		{"tags": {""}},
	} {
		form.Set("hashes", hashMovie)
		require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/removeTags", form, ck).Code)
	}
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/setCategory", url.Values{"hashes": {hashMovie}, "category": {"tv"}}, ck).Code)
	assert.Equal(t, []string{"tags " + hashMovie + "=4k", "tags " + hashMovie + "=", "category " + hashMovie + "=tv"}, e.dl.got())
}

// 建分类与标签：记在设置里，分类与标签列表里能看到；分类名为空 400；后端是 qB 时也在 qB 里建
func TestCreateCategoryAndTags(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/api/v2/torrents/createCategory", url.Values{"category": {""}}, ck).Code)
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/createCategory", url.Values{"category": {"radarr"}, "savePath": {"/movies/radarr"}}, ck).Code)
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/createTags", url.Values{"tags": {"autobrr,cross-seed"}}, ck).Code)
	assert.Equal(t, []string{"createCategory radarr=/movies/radarr"}, e.dl.got())

	cats := decode[map[string]categoryView](t, e.do(http.MethodGet, "/api/v2/torrents/categories", nil, ck))
	assert.Equal(t, categoryView{Name: "radarr", SavePath: "/movies/radarr"}, cats["radarr"])
	tags := decode[[]string](t, e.do(http.MethodGet, "/api/v2/torrents/tags", nil, ck))
	assert.Contains(t, tags, "autobrr")
	assert.Contains(t, tags, "cross-seed")
	cmds := []string{}
	for _, a := range e.audit.all() {
		cmds = append(cmds, a.Command+" "+a.Result)
	}
	assert.Equal(t, []string{"POST /api/v2/torrents/createCategory success", "POST /api/v2/torrents/createTags success"}, cmds)
}

// qbittorrent-api 把布尔值写成 True/False（Python 的 str(True)）：qB 不分大小写，兼容入口也一样
func TestBoolParamsCaseInsensitive(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	e.markCompat(hashMovie)
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/delete", url.Values{"hashes": {hashMovie}, "deleteFiles": {"True"}}, ck).Code)
	assert.Equal(t, []string{"remove " + hashMovie + " data=true"}, e.dl.got())
	names := func(reverse string) []string {
		var out []string
		for _, item := range decode[[]map[string]any](t, e.do(http.MethodGet, "/api/v2/torrents/info", url.Values{"sort": {"size"}, "reverse": {reverse}}, ck)) {
			out = append(out, item["name"].(string))
		}
		return out
	}
	assert.Equal(t, names("true"), names("True"))
	assert.Equal(t, "Dune.Part.Two.2024.2160p", names("TRUE")[0])
}
