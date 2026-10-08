package qbitcompat

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// markCompat 把种子记成经兼容入口加进这台下载器的（写接口默认只动这些）：记下下载器里它的添加时间。
func (e *env) markCompat(hash string) {
	e.t.Helper()
	row := models.QbitCompatTorrent{DownloaderID: e.dlSet.ID, InfoHash: hash, CreatedAt: e.now}
	for _, t := range e.dl.torrents {
		if strings.EqualFold(t.InfoHash, hash) {
			row.AddedAt = t.DateAdded
		}
	}
	require.NoError(e.t, e.db.Create(&row).Error)
}

// markCompatAt 同 markCompat（给 server_test 里的用例用，名字分开免得看错）。
func (e *env) markCompatAt(hash string) { e.markCompat(hash) }

// 所有权只认这台下载器上、确实经兼容入口加进去的那一个：只有来源记录（没加成功、原来就在）不算；别的下载器上的不算；
// 记下了下载器给的添加时间以后，只认添加时间一样的（删掉再从别处加回来的时间不一样）；下载器不给添加时间时不认（宁可不动）
func TestOwnershipIsStrict(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	h := hashMovie
	require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "qa", TorrentID: "x", TorrentHash: &h, DownloadSource: Source}).Error)
	other := models.DownloaderSetting{Name: "other", Type: "qbittorrent", URL: "http://127.0.0.1:9", Enabled: true}
	require.NoError(t, e.db.Create(&other).Error)
	require.NoError(t, e.db.Create(&models.QbitCompatTorrent{DownloaderID: other.ID, InfoHash: hashDebian, CreatedAt: e.now}).Error)
	// 记下的添加时间是 1700000000，下载器里这个种子的添加时间是 1700000060：是后来又加回来的
	require.NoError(t, e.db.Create(&models.QbitCompatTorrent{DownloaderID: e.dlSet.ID, InfoHash: hashTR, AddedAt: 1700000000, CreatedAt: e.now}).Error)
	e.dl.torrents[2].DateAdded = 1700000060

	w := e.do(http.MethodPost, "/api/v2/torrents/pause", url.Values{"hashes": {"all"}}, ck)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, e.dl.got(), "一个都不算兼容入口的")
	assert.EqualValues(t, 3, e.audit.all()[0].Args["denied"])

	// 添加时间对得上的才算；下载器不给添加时间的不算
	e.dl.torrents[2].DateAdded = 1700000000
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/pause", url.Values{"hashes": {hashTR}}, ck).Code)
	assert.Equal(t, []string{"pause 7"}, e.dl.got())
	e.dl.torrents[2].DateAdded = 0
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/pause", url.Values{"hashes": {hashTR}}, ck).Code)
	assert.Len(t, e.dl.got(), 1, "不给添加时间：不认")
}

// 所有权只认加完以后在下载器里看到的那一个：qB 是异步加的，过一会儿才列出来时等得到就记；一直看不到、或者下载器不给添加时间时不记，
// 之后同一个种子出现在下载器里（比如删掉以后从别处加回来，添加时间就在加的那会儿）也不认；所有权表里没有添加时间的旧记录也不认
func TestOwnershipNeedsObservedAdd(t *testing.T) {
	e := newEnv(t)
	e.withSites()
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	ownedAt := func(h string) (int64, bool) {
		var rows []models.QbitCompatTorrent
		require.NoError(t, e.db.Where("info_hash = ?", h).Find(&rows).Error)
		if len(rows) == 0 {
			return 0, false
		}
		return rows[0].AddedAt, true
	}
	pushOnly := func(show func(h, name string)) {
		e.srv.deps.Push = func(_ context.Context, req internal.PushTorrentRequest) (*internal.PushTorrentResult, error) {
			if show != nil {
				p, err := v2.ParseTorrent(req.TorrentData)
				require.NoError(t, err)
				show(strings.ToLower(p.InfoHash), p.Name)
			}
			return &internal.PushTorrentResult{Success: true}, nil
		}
	}

	// 过 50 毫秒才列出来：等得到，记下它的添加时间
	e.srv.observe = 5 * time.Second
	pushOnly(func(h, name string) {
		time.AfterFunc(50*time.Millisecond, func() { e.dl.list(downloader.Torrent{ID: h, InfoHash: h, Name: name, DateAdded: 1700000500}) })
	})
	late := torrentFile("late", "https://qa.example/announce", "")
	require.Equal(t, "Ok.", e.postAdd(ck, [][]byte{late}, nil).Body.String())
	at, ok := ownedAt(hashOf(t, late))
	assert.True(t, ok, "等到了")
	assert.EqualValues(t, 1700000500, at)

	// 一直看不到：不记；之后它出现在下载器里，添加时间就在加的那会儿，也动不了
	e.srv.observe = 30 * time.Millisecond
	pushOnly(nil)
	unseen := torrentFile("unseen", "https://qa.example/announce", "")
	require.Equal(t, "Ok.", e.postAdd(ck, [][]byte{unseen}, nil).Body.String(), "加进去了，客户端照样看到 Ok.")
	h := hashOf(t, unseen)
	_, ok = ownedAt(h)
	assert.False(t, ok, "看不到：不记")
	e.dl.list(downloader.Torrent{ID: h, InfoHash: h, Name: "unseen", DateAdded: e.now.Unix()})
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/pause", url.Values{"hashes": {h}}, ck).Code)
	assert.Empty(t, e.dl.got(), "不认")

	// 看得到，但下载器不给添加时间：不记
	pushOnly(func(h, name string) { e.dl.list(downloader.Torrent{ID: h, InfoHash: h, Name: name}) })
	untimed := torrentFile("untimed", "https://qa.example/announce", "")
	require.Equal(t, "Ok.", e.postAdd(ck, [][]byte{untimed}, nil).Body.String())
	_, ok = ownedAt(hashOf(t, untimed))
	assert.False(t, ok, "不给添加时间：不记")

	// 没有添加时间的旧记录：不认
	require.NoError(t, e.db.Create(&models.QbitCompatTorrent{DownloaderID: e.dlSet.ID, InfoHash: hashMovie, CreatedAt: time.Unix(1700000100, 0)}).Error)
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/pause", url.Values{"hashes": {hashMovie}}, ck).Code)
	assert.Empty(t, e.dl.got(), "记录里没有添加时间：不认")
}

// 加成功了才记所有权（已经在下载器里的、推送失败的不记）；经兼容入口删掉以后所有权一起去掉
func TestOwnershipLifecycle(t *testing.T) {
	e := newEnv(t)
	e.withSites()
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	owned := func() int64 {
		var n int64
		require.NoError(t, e.db.Model(&models.QbitCompatTorrent{}).Count(&n).Error)
		return n
	}
	added := torrentFile("added", "https://qa.example/announce", "")
	require.Equal(t, "Ok.", e.postAdd(ck, [][]byte{added}, nil).Body.String())
	assert.EqualValues(t, 1, owned())

	e.srv.deps.Push = func(context.Context, internal.PushTorrentRequest) (*internal.PushTorrentResult, error) {
		return &internal.PushTorrentResult{Success: true, Skipped: true}, nil
	}
	require.Equal(t, "Ok.", e.postAdd(ck, [][]byte{torrentFile("existing", "https://qa.example/announce", "")}, nil).Body.String())
	e.srv.deps.Push = func(context.Context, internal.PushTorrentRequest) (*internal.PushTorrentResult, error) {
		return &internal.PushTorrentResult{Success: false, Message: "磁盘空间不足"}, nil
	}
	require.Equal(t, "Fails.", e.postAdd(ck, [][]byte{torrentFile("blocked", "https://qa.example/announce", "")}, nil).Body.String())
	assert.EqualValues(t, 1, owned(), "已经在的、没加成功的都不算")

	// 加进去的那个在下载器里（推送时就列出来了），删掉它
	h := hashOf(t, added)
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/delete", url.Values{"hashes": {h}, "deleteFiles": {"false"}}, ck).Code)
	assert.Equal(t, []string{"remove " + h + " data=false"}, e.dl.got())
	assert.EqualValues(t, 0, owned(), "删掉以后所有权去掉")
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
	post("/api/v2/torrents/setCategory", url.Values{"category": {"sonarr"}})
	post("/api/v2/torrents/addTags", url.Values{"tags": {"4k, new"}})
	post("/api/v2/torrents/removeTags", url.Values{"tags": {"hdsky"}})
	post("/api/v2/torrents/delete", url.Values{"deleteFiles": {"true"}})
	assert.Equal(t, []string{
		"createCategory sonarr=/tv",
		"category " + hashMovie + "=sonarr",
		"tags " + hashMovie + "=hdsky,4k,new",
		"removeTags " + hashMovie + "=hdsky",
		"remove " + hashMovie + " data=true",
	}, e.dl.got())
	assert.Equal(t, true, e.audit.all()[3].Args["delete_files"])
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/api/v2/torrents/addTags", url.Values{"hashes": {hashMovie}, "tags": {" , "}}, ck).Code)
}

// 后端是 Transmission：分类与标签都在 labels 里，第一个是分类、其余是标签。读出来的 tags 不含分类；
// 改分类、加减标签都改写整份 labels，分类留在第一个，标签互不影响
func TestWriteOperationsTransmission(t *testing.T) {
	e := newEnv(t)
	e.asTR = true
	e.dlSet.Type = "transmission"
	require.NoError(t, e.db.Save(&e.dlSet).Error)
	e.dl.torrents[1].Raw = nil
	e.dl.torrents[1].Category, e.dl.torrents[1].Tags = "movies", "movies,hdsky,4k"
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	e.markCompat(hashMovie)

	var movie map[string]any
	for _, item := range decode[[]map[string]any](t, e.do(http.MethodGet, "/api/v2/torrents/info", nil, ck)) {
		if item["hash"] == hashMovie {
			movie = item
		}
	}
	require.NotNil(t, movie)
	assert.Equal(t, "movies", movie["category"])
	assert.Equal(t, "hdsky,4k", movie["tags"], "标签里不含分类")

	post := func(path string, form url.Values) {
		t.Helper()
		form.Set("hashes", hashMovie)
		require.Equal(t, http.StatusOK, e.do(http.MethodPost, path, form, ck).Code, path)
	}
	post("/api/v2/torrents/removeTags", url.Values{"tags": {"hdsky"}})
	post("/api/v2/torrents/removeTags", url.Values{"tags": {""}})
	post("/api/v2/torrents/setCategory", url.Values{"category": {"tv"}})
	post("/api/v2/torrents/addTags", url.Values{"tags": {"x"}})
	assert.Equal(t, []string{
		"tags " + hashMovie + "=movies,4k",
		"tags " + hashMovie + "=movies",
		"tags " + hashMovie + "=tv,hdsky,4k",
		"tags " + hashMovie + "=movies,hdsky,4k,x",
	}, e.dl.got(), "分类一直在第一个，标签不被改分类冲掉")
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
	assert.Equal(t, []string{
		"POST /api/v2/torrents/createCategory error:http_400",
		"POST /api/v2/torrents/createCategory success",
		"POST /api/v2/torrents/createTags success",
	}, cmds, "名字为空的那次也记")
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

// 审计记下写操作的耗时（审计页的「延迟」列）
func TestWriteAuditLatency(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	e.markCompat(hashMovie)
	e.dl.delay = 30 * time.Millisecond
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/pause", url.Values{"hashes": {hashMovie}}, ck).Code)
	a := e.audit.all()
	require.Len(t, a, 1)
	assert.GreaterOrEqual(t, a[0].LatencyMs, int64(30))
}

// 写请求在半路失败（参数不对、没给种子、没有可用的下载器）也记审计；正常的只记一条
func TestWriteFailuresAudited(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	require.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/api/v2/torrents/createCategory", url.Values{"category": {""}}, ck).Code)
	require.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/api/v2/torrents/addTags", url.Values{"hashes": {"all"}}, ck).Code)
	require.Equal(t, "Fails.", e.do(http.MethodPost, "/api/v2/torrents/add", url.Values{}, ck).Body.String())
	require.NoError(t, e.db.Model(&models.DownloaderSetting{}).Where("id = ?", e.dlSet.ID).Update("enabled", false).Error)
	require.Equal(t, http.StatusServiceUnavailable, e.do(http.MethodPost, "/api/v2/torrents/pause", url.Values{"hashes": {"all"}}, ck).Code)

	var got []string
	for _, a := range e.audit.all() {
		got = append(got, a.Command+" "+a.Result)
	}
	assert.Equal(t, []string{
		"POST /api/v2/torrents/createCategory error:http_400",
		"POST /api/v2/torrents/addTags error:http_400",
		"POST /api/v2/torrents/add error:nothing_added",
		"POST /api/v2/torrents/pause error:http_503",
	}, got)
}
