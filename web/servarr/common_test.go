package servarr

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
)

func TestAuth(t *testing.T) {
	e := newEnv(t)
	status := radarrBase + apiPath + "/system/status"

	// 缺 Key、错 Key、只有会话：都回 401 {"message": ...}
	r := e.call(http.MethodGet, status, "", nil)
	requireStatus(t, http.StatusUnauthorized, r)
	assert.Contains(t, r.obj(t)["message"], "API Key")
	requireStatus(t, http.StatusUnauthorized, e.call(http.MethodGet, status, "ptt_1_wrong", nil))
	requireStatus(t, http.StatusUnauthorized, e.call(http.MethodGet, status, "not-a-token", nil))
	req := httptest.NewRequest(http.MethodGet, status, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc"})
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// 没有 servarr 权限的令牌：401；写请求记一条 denied:scope
	_, other, err := e.tokens.Create(e.ctx, apitoken.CreateInput{Name: "app", Scopes: []string{apitoken.ScopeAppRead, apitoken.ScopeAppWrite}}, "admin")
	require.NoError(t, err)
	r = e.call(http.MethodGet, status, other, nil)
	requireStatus(t, http.StatusUnauthorized, r)
	assert.Contains(t, r.obj(t)["message"], "servarr")
	requireStatus(t, http.StatusUnauthorized, e.call(http.MethodPost, radarrBase+apiPath+"/tag", other, map[string]any{"label": "x"}))
	denied := e.audits("POST /servarr/radarr/api/v3/tag")
	require.Len(t, denied, 1)
	assert.Equal(t, "denied:scope", denied[0].Result)
	assert.Empty(t, e.audits("GET "+status), "读请求不记审计")
	var tags int64
	require.NoError(t, e.db.Model(&models.ServarrTag{}).Count(&tags).Error)
	assert.Equal(t, int64(0), tags)

	// 查询参数 apikey 与请求头 X-Api-Key 都认
	requireStatus(t, http.StatusOK, e.call(http.MethodGet, status, e.key, nil))
	req = httptest.NewRequest(http.MethodGet, sonarrBase+apiPath+"/system/status", nil)
	req.Header.Set("X-Api-Key", e.key)
	rec = httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 撤销以后不认
	tok, err := e.tokens.Verify(e.ctx, e.key)
	require.NoError(t, err)
	require.NoError(t, e.tokens.Revoke(e.ctx, tok.ID))
	requireStatus(t, http.StatusUnauthorized, e.call(http.MethodGet, status, e.key, nil))

	// 日志里没有 Key
	for _, entry := range e.logs.All() {
		assert.NotContains(t, entry.Message, e.key)
		assert.NotContains(t, entry.Message, other)
		for _, f := range entry.Context {
			assert.NotContains(t, f.String, e.key)
		}
	}
}

func TestAuthRateLimit(t *testing.T) {
	e := newEnv(t)
	status := radarrBase + apiPath + "/system/status"
	for range failMax - 1 {
		requireStatus(t, http.StatusUnauthorized, e.call(http.MethodGet, status, "ptt_1_wrong", nil))
	}
	// 中间对了一次：计数清零
	requireStatus(t, http.StatusOK, e.call(http.MethodGet, status, e.key, nil))
	for range failMax {
		requireStatus(t, http.StatusUnauthorized, e.call(http.MethodGet, status, "ptt_1_wrong", nil))
	}
	// 错满 10 次：之后 5 分钟连对的 Key 也直接 429
	r := e.call(http.MethodGet, status, e.key, nil)
	requireStatus(t, http.StatusTooManyRequests, r)
	assert.Contains(t, r.obj(t)["message"], "太多")
	e.advance(failBan - time.Second)
	requireStatus(t, http.StatusTooManyRequests, e.call(http.MethodGet, status, e.key, nil))
	e.advance(2 * time.Second)
	requireStatus(t, http.StatusOK, e.call(http.MethodGet, status, e.key, nil))

	// 别的来源不受影响
	for range failMax {
		requireStatus(t, http.StatusUnauthorized, e.call(http.MethodGet, status, "ptt_1_wrong", nil))
	}
	req := httptest.NewRequest(http.MethodGet, status+"?apikey="+e.key, nil)
	req.RemoteAddr = "198.51.100.7:4000"
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	// 窗口过了才失败的不累计
	e.advance(failBan + failWindow + time.Second)
	for range failMax - 1 {
		requireStatus(t, http.StatusUnauthorized, e.call(http.MethodGet, status, "ptt_1_wrong", nil))
	}
	e.advance(failWindow + time.Second)
	requireStatus(t, http.StatusUnauthorized, e.call(http.MethodGet, status, "ptt_1_wrong", nil))
	requireStatus(t, http.StatusOK, e.call(http.MethodGet, status, e.key, nil))
}

func TestFailLockBounded(t *testing.T) {
	l := &failLock{m: map[string]*failEntry{}}
	now := time.Now()
	for i := range maxFailIPs + 5 {
		l.fail("ip-"+formatID(uint(i)), now.Add(time.Duration(i)*time.Millisecond))
	}
	assert.LessOrEqual(t, len(l.m), maxFailIPs)
	_, ok := l.m["ip-0"]
	assert.False(t, ok, "满了以后淘汰最早的")
	// 过期的先清掉
	later := now.Add(failWindow + time.Hour)
	l.fail("new", later)
	assert.Len(t, l.m, 1)
}

func TestRouting(t *testing.T) {
	e := newEnv(t)
	r := e.call(http.MethodGet, "/servarr/lidarr/api/v1/system/status", e.key, nil)
	requireStatus(t, http.StatusNotFound, r)
	assert.Contains(t, r.obj(t)["message"], "/servarr/radarr")
	assert.Equal(t, http.StatusNotFound, e.radarr(http.MethodGet, "/nothing", nil).status)
	assert.Equal(t, http.StatusNotFound, e.radarr(http.MethodGet, "/series", nil).status, "Radarr 没有剧集接口")
	assert.Equal(t, http.StatusNotFound, e.sonarr(http.MethodGet, "/movie", nil).status, "Sonarr 没有电影接口")
	assert.Equal(t, http.StatusNotFound, e.radarr(http.MethodGet, "/languageprofile", nil).status)
	assert.Equal(t, http.StatusMethodNotAllowed, e.radarr(http.MethodDelete, "/tag", nil).status)
	// 小写、大写的写法都认（Radarr 的路由不分大小写）
	requireStatus(t, http.StatusOK, e.radarr(http.MethodGet, "/qualityprofile", nil))
	requireStatus(t, http.StatusOK, e.radarr(http.MethodGet, "/rootFolder", nil))
	requireStatus(t, http.StatusOK, e.sonarr(http.MethodGet, "/languageProfile", nil))

	// 请求体：不是 JSON 400，超过 1 MiB 413
	assert.Equal(t, http.StatusBadRequest, e.radarr(http.MethodPost, "/tag", "{bad").status)
	big := `{"label":"` + strings.Repeat("a", maxBody) + `"}`
	assert.Equal(t, http.StatusRequestEntityTooLarge, e.radarr(http.MethodPost, "/tag", big).status)
}

func TestSystemStatus(t *testing.T) {
	e := newEnv(t)
	r := e.radarr(http.MethodGet, "/system/status", nil)
	requireStatus(t, http.StatusOK, r)
	st := r.obj(t)
	assert.Equal(t, "Radarr", st["appName"])
	assert.Equal(t, "pt-tools", st["instanceName"])
	assert.Equal(t, "/servarr/radarr", st["urlBase"])
	st = e.sonarr(http.MethodGet, "/system/status", nil).obj(t)
	assert.Equal(t, "Sonarr", st["appName"])
	assert.Equal(t, "/servarr/sonarr", st["urlBase"])
	assert.Equal(t, timeText(e.Now()), st["startTime"])
}

func TestTags(t *testing.T) {
	e := newEnv(t)
	r := e.radarr(http.MethodPost, "/tag", map[string]any{"label": " 1-admin "})
	requireStatus(t, http.StatusCreated, r)
	a := r.obj(t)
	assert.Equal(t, "1-admin", a["label"])
	// 同名的回已有的那个（两个前缀共用标签）
	r = e.sonarr(http.MethodPost, "/tag", map[string]any{"label": "1-admin"})
	requireStatus(t, http.StatusCreated, r)
	assert.Equal(t, a["id"], r.obj(t)["id"])
	b := e.radarr(http.MethodPost, "/tag", map[string]any{"label": "2-bob"}).obj(t)
	requireStatus(t, http.StatusBadRequest, e.radarr(http.MethodPost, "/tag", map[string]any{"label": "  "}))
	requireStatus(t, http.StatusBadRequest, e.radarr(http.MethodPost, "/tag", map[string]any{"label": strings.Repeat("长", 129)}))

	list := e.sonarr(http.MethodGet, "/tag", nil).list(t)
	assert.Equal(t, []any{a, b}, list)

	// 改名（Seerr 的 0007 迁移把旧的「1 - admin」改成「1-admin」）
	idA := num(a["id"])
	r = e.radarr(http.MethodPut, "/tag/"+formatID(uint(idA)), map[string]any{"id": idA, "label": "1-root"})
	requireStatus(t, http.StatusOK, r)
	assert.Equal(t, map[string]any{"id": a["id"], "label": "1-root"}, r.obj(t))
	requireStatus(t, http.StatusBadRequest, e.radarr(http.MethodPut, "/tag/"+formatID(uint(idA)), map[string]any{"label": "2-bob"}))
	requireStatus(t, http.StatusNotFound, e.radarr(http.MethodPut, "/tag/999", map[string]any{"label": "x"}))
	requireStatus(t, http.StatusNotFound, e.radarr(http.MethodPut, "/tag/abc", map[string]any{"label": "x"}))
	requireStatus(t, http.StatusBadRequest, e.radarr(http.MethodPut, "/tag/"+formatID(uint(idA)), map[string]any{"label": ""}))
	assert.Equal(t, "1-root", e.radarr(http.MethodGet, "/tag", nil).list(t)[0].(map[string]any)["label"])

	puts := e.audits("PUT /servarr/radarr/api/v3/tag/" + formatID(uint(idA)))
	require.Len(t, puts, 3)
	assert.Equal(t, "success", puts[0].Result)
	assert.Equal(t, "error:http_400", puts[1].Result)
}

func TestSetTagsIgnoresUnknown(t *testing.T) {
	e := newEnv(t)
	a := e.radarr(http.MethodPost, "/tag", map[string]any{"label": "a"}).obj(t)
	got, err := e.srv.setTags(e.ctx, tagKindMovie, 157336, []uint{uint(num(a["id"])), 999})
	require.NoError(t, err)
	assert.Equal(t, []uint{uint(num(a["id"]))}, got)
	assert.Equal(t, []uint{uint(num(a["id"]))}, e.srv.tagsOf(e.ctx, tagKindMovie, 157336))
	assert.Equal(t, []uint{}, e.srv.tagsOf(e.ctx, tagKindSeries, 157336), "电影与剧集的标签分开")
	got, err = e.srv.setTags(e.ctx, tagKindMovie, 157336, nil)
	require.NoError(t, err)
	assert.Equal(t, []uint{}, got)
	assert.Equal(t, []uint{}, e.srv.tagsOf(e.ctx, tagKindMovie, 157336))
}

func TestQueue(t *testing.T) {
	e := newEnv(t)
	for _, call := range []func(string, string, any) response{e.radarr, e.sonarr} {
		r := call(http.MethodGet, "/queue?includeEpisode=true", nil)
		requireStatus(t, http.StatusOK, r)
		q := r.obj(t)
		assert.Equal(t, []any{}, q["records"])
		assert.Equal(t, float64(0), q["totalRecords"])
		assert.Equal(t, float64(1), q["page"])
	}
}

func TestCommands(t *testing.T) {
	e := newEnv(t)
	movie, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 157336}, models.MediaSubFromManual)
	require.NoError(t, err)
	paused, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 27205, Paused: true}, models.MediaSubFromManual)
	require.NoError(t, err)
	tv, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}, models.MediaSubFromManual)
	require.NoError(t, err)
	later := e.Now().Add(6 * time.Hour)
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("1 = 1").Update("next_search_at", later).Error)
	e.advance(time.Minute)

	// MoviesSearch：电影订阅马上搜，暂停的跳过，不是电影的不算
	r := e.radarr(http.MethodPost, "/command", map[string]any{"name": "MoviesSearch", "movieIds": []uint{movie.ID, paused.ID, tv.ID, 0}})
	requireStatus(t, http.StatusCreated, r)
	cmd := r.obj(t)
	assert.Equal(t, "MoviesSearch", cmd["name"])
	assert.Equal(t, "queued", cmd["status"])
	assert.NotZero(t, cmd["id"])
	assert.True(t, e.sub(models.MediaKindMovie, 157336, 0).NextSearchAt.Equal(e.Now()))
	assert.True(t, e.sub(models.MediaKindMovie, 27205, 0).NextSearchAt.Equal(later))
	assert.True(t, e.sub(models.MediaKindTV, 100088, 2).NextSearchAt.Equal(later))
	audits := e.audits("POST /servarr/radarr/api/v3/command")
	require.Len(t, audits, 1)
	assert.Equal(t, []int{int(movie.ID)}, audits[0].Args["searched"])
	assert.Equal(t, []int{int(paused.ID)}, audits[0].Args["skipped"])
	requireStatus(t, http.StatusNotFound, e.radarr(http.MethodPost, "/command", map[string]any{"name": "MoviesSearch", "movieIds": []uint{tv.ID, 999}}))

	// MissingEpisodeSearch：这部剧在找的季马上搜
	r = e.sonarr(http.MethodPost, "/command", map[string]any{"name": "MissingEpisodeSearch", "seriesId": 100088})
	requireStatus(t, http.StatusCreated, r)
	assert.True(t, e.sub(models.MediaKindTV, 100088, 2).NextSearchAt.Equal(e.Now()))
	assert.Greater(t, num(r.obj(t)["id"]), num(cmd["id"]), "命令编号递增")
	requireStatus(t, http.StatusNotFound, e.sonarr(http.MethodPost, "/command", map[string]any{"name": "MissingEpisodeSearch", "seriesId": 1}))
	requireStatus(t, http.StatusBadRequest, e.sonarr(http.MethodPost, "/command", map[string]any{"name": "MissingEpisodeSearch"}))

	// RefreshMonitoredDownloads：两个都认、什么都不做、不记审计
	requireStatus(t, http.StatusCreated, e.radarr(http.MethodPost, "/command", map[string]any{"name": "RefreshMonitoredDownloads"}))
	requireStatus(t, http.StatusCreated, e.sonarr(http.MethodPost, "/command", map[string]any{"name": "RefreshMonitoredDownloads"}))

	// 别的名字、放错了前缀的：400
	r = e.radarr(http.MethodPost, "/command", map[string]any{"name": "RssSync"})
	requireStatus(t, http.StatusBadRequest, r)
	assert.Contains(t, r.obj(t)["message"], "MoviesSearch")
	requireStatus(t, http.StatusBadRequest, e.sonarr(http.MethodPost, "/command", map[string]any{"name": "MoviesSearch", "movieIds": []uint{movie.ID}}))
	requireStatus(t, http.StatusBadRequest, e.radarr(http.MethodPost, "/command", map[string]any{"name": "MissingEpisodeSearch", "seriesId": 100088}))

	assert.Len(t, e.audits("POST /servarr/radarr/api/v3/command"), 4, "两次 MoviesSearch、两次 400；Refresh 不算")
	assert.Len(t, e.audits("POST /servarr/sonarr/api/v3/command"), 4)
}

func TestQualityProfilesAndRootFolders(t *testing.T) {
	e := newEnv(t)
	p, err := e.subs.SaveProfile(e.ctx, 0, subscribe.ProfileInput{Name: "2160p"})
	require.NoError(t, err)
	got, err := e.srv.ptProfile(e.ctx, defaultProfileID)
	require.NoError(t, err)
	assert.Equal(t, uint(0), got)
	got, err = e.srv.ptProfile(e.ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, uint(0), got)
	got, err = e.srv.ptProfile(e.ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got)
	_, err = e.srv.ptProfile(e.ctx, p.ID+5)
	require.ErrorContains(t, err, "质量档案不存在")
	assert.Equal(t, uint(defaultProfileID), servarrProfile(0))
	assert.Equal(t, p.ID, servarrProfile(p.ID))

	lib, err := e.srv.libraryOf(e.ctx, models.MediaKindMovie, autoRootPath)
	require.NoError(t, err)
	assert.Equal(t, uint(0), lib)
	lib, err = e.srv.libraryOf(e.ctx, models.MediaKindMovie, "")
	require.NoError(t, err)
	assert.Equal(t, uint(0), lib)
	lib, err = e.srv.libraryOf(e.ctx, models.MediaKindMovie, e.movieLib.Path+"/")
	require.NoError(t, err)
	assert.Equal(t, e.movieLib.ID, lib, "末尾多一个斜杠也认")
	_, err = e.srv.libraryOf(e.ctx, models.MediaKindMovie, e.tvLib.Path)
	require.ErrorContains(t, err, "不是 pt-tools 里启用的媒体库", "类型不同的库不认")
	_, err = e.srv.libraryOf(e.ctx, models.MediaKindMovie, "/elsewhere")
	require.Error(t, err)

	libs, err := e.srv.libraries(e.ctx, models.MediaKindTV)
	require.NoError(t, err)
	assert.Equal(t, e.tvLib.Path, rootPathOf(libs, e.tvLib.ID))
	assert.Equal(t, autoRootPath, rootPathOf(libs, 0))
	assert.Equal(t, autoRootPath, rootPathOf(libs, 999), "库删了：写 auto")

	// 没有接上媒体库时只有 auto
	bare := New(Deps{DB: e.db, Tokens: e.tokens, Subs: e.subs, Now: e.Now})
	e.h = bare.Handler()
	folders := e.radarr(http.MethodGet, "/rootfolder", nil).list(t)
	require.Len(t, folders, 1)
	assert.Equal(t, autoRootPath, folders[0].(map[string]any)["path"])
	assert.Equal(t, float64(0), folders[0].(map[string]any)["freeSpace"])
}
