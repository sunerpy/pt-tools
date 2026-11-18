package qbitcompat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// errDown 是下载器回的错误，里面带着 passkey：交给客户端以前要遮住。
var errDown = errors.New("下载器出错: https://tracker.example/announce.php?passkey=SECRETPASSKEY1234")

// 下载器读不出来：读接口、单个种子的详情与写接口都回 503，错误里的 passkey 遮住
func TestDownloaderReadErrors(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	e.markCompat(hashMovie)
	e.dl.failRead = errDown
	for _, c := range []struct {
		method, path string
		form         url.Values
	}{
		{http.MethodGet, "/api/v2/torrents/info", nil},
		{http.MethodGet, "/api/v2/torrents/categories", nil},
		{http.MethodGet, "/api/v2/torrents/tags", nil},
		{http.MethodGet, "/api/v2/sync/maindata", nil},
		{http.MethodGet, "/api/v2/torrents/properties", url.Values{"hash": {hashMovie}}},
		{http.MethodPost, "/api/v2/torrents/pause", url.Values{"hashes": {hashMovie}}},
	} {
		w := e.do(c.method, c.path, c.form, ck)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code, c.path)
		assert.NotContains(t, w.Body.String(), "SECRETPASSKEY1234", c.path)
	}
	// 读不到保存目录、传输状态时报空的与 0，不算失败
	assert.Empty(t, e.do(http.MethodGet, "/api/v2/app/defaultSavePath", nil, ck).Body.String())
	tr := decode[map[string]any](t, e.do(http.MethodGet, "/api/v2/transfer/info", nil, ck))
	assert.EqualValues(t, 0, tr["dl_info_speed"])
}

// 下载器写失败：回 500、错误里的 passkey 遮住，审计记 error:downloader（后端是 qB 与 Transmission 都一样）
func TestDownloaderWriteErrors(t *testing.T) {
	for _, asTR := range []bool{false, true} {
		e := newEnv(t)
		if asTR {
			e.asTR = true
			e.dlSet.Type = "transmission"
			require.NoError(t, e.db.Save(&e.dlSet).Error)
		}
		ck := e.login(e.token(apitoken.ScopeQbitCompat))
		e.markCompat(hashMovie)
		e.dl.failWrite = errDown
		for _, c := range []struct {
			path string
			form url.Values
		}{
			{"/api/v2/torrents/pause", url.Values{}},
			{"/api/v2/torrents/resume", url.Values{}},
			{"/api/v2/torrents/setCategory", url.Values{"category": {"tv"}}},
			{"/api/v2/torrents/addTags", url.Values{"tags": {"x"}}},
			{"/api/v2/torrents/removeTags", url.Values{"tags": {"hdsky"}}},
			{"/api/v2/torrents/delete", url.Values{"deleteFiles": {"false"}}},
		} {
			c.form.Set("hashes", hashMovie)
			w := e.do(http.MethodPost, c.path, c.form, ck)
			assert.Equal(t, http.StatusInternalServerError, w.Code, "%s transmission=%t", c.path, asTR)
			assert.NotContains(t, w.Body.String(), "SECRETPASSKEY1234")
			a := e.audit.all()
			assert.Equal(t, "error:downloader", a[len(a)-1].Result, c.path)
		}
		w := e.do(http.MethodPost, "/api/v2/torrents/createCategory", url.Values{"category": {"tv"}, "savePath": {"/tv"}}, ck)
		if asTR {
			assert.Equal(t, http.StatusOK, w.Code, "Transmission 没有分类：只记在设置里")
		} else {
			assert.Equal(t, http.StatusInternalServerError, w.Code)
			a := e.audit.all()
			assert.Equal(t, "error:downloader", a[len(a)-1].Result)
		}
	}
}

// 参数不对：分类名太长 400；建分类时名字为空 400、带反斜杠 409（qB 的行为）
func TestWriteValidation(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	e.markCompat(hashMovie)
	long := strings.Repeat("a", 256)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/api/v2/torrents/setCategory", url.Values{"hashes": {hashMovie}, "category": {long}}, ck).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/api/v2/torrents/createCategory", url.Values{"category": {" "}}, ck).Code)
	assert.Equal(t, http.StatusConflict, e.do(http.MethodPost, "/api/v2/torrents/createCategory", url.Values{"category": {`a\b`}}, ck).Code)
	assert.Equal(t, http.StatusConflict, e.do(http.MethodPost, "/api/v2/torrents/createCategory", url.Values{"category": {long}}, ck).Code)
	// 去掉全部标签、而客户端看得到的标签一个都没有：什么都不去
	e.dl.torrents[1].Tags = OwnerTag
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/removeTags", url.Values{"hashes": {hashMovie}}, ck).Code)
	assert.Empty(t, e.dl.got())
}

// 连不上绑定的下载器：读接口（preferences、defaultSavePath、transfer/info）、添加与建分类都回 503，不记成成功
func TestBackendUnavailable(t *testing.T) {
	e := newEnv(t)
	e.withSites()
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	e.srv.deps.Instance = func(context.Context, string) (downloader.Downloader, error) { return nil, errDown }
	for _, path := range []string{"/api/v2/app/preferences", "/api/v2/app/defaultSavePath", "/api/v2/transfer/info"} {
		w := e.do(http.MethodGet, path, nil, ck)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code, path)
		assert.NotContains(t, w.Body.String(), "SECRETPASSKEY1234", path)
	}
	assert.Equal(t, http.StatusServiceUnavailable, e.postAdd(ck, [][]byte{torrentFile("a", "https://qa.example/announce", "")}, nil).Code)
	assert.Equal(t, http.StatusServiceUnavailable, e.do(http.MethodPost, "/api/v2/torrents/createCategory", url.Values{"category": {"tv"}}, ck).Code)
	assert.Empty(t, e.gotPushes())
}

// 数据库出错：令牌校验不了时登录与请求回 503；设置、所有权读不出来时回 503，不当成默认设置；记不下标签回 500；审计写不进去不影响回应
func TestDatabaseErrors(t *testing.T) {
	e := newEnv(t)
	plain := e.token(apitoken.ScopeQbitCompat)
	ck := e.login(plain)

	// 所有权表读不出来：写接口回 503
	require.NoError(t, e.db.Migrator().DropTable(&models.QbitCompatTorrent{}))
	assert.Equal(t, http.StatusServiceUnavailable, e.do(http.MethodPost, "/api/v2/torrents/pause", url.Values{"hashes": {hashMovie}}, ck).Code)

	// 审计写不进去：只记日志，回应照常
	e.audit.fail = errors.New("审计表坏了")
	assert.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/createTags", url.Values{"tags": {"a"}}, ck).Code)

	// 设置表读不出来：读接口回 503，建标签回 500
	require.NoError(t, e.db.Migrator().DropTable(&models.QbitCompatSetting{}))
	assert.Equal(t, http.StatusServiceUnavailable, e.do(http.MethodGet, "/api/v2/torrents/info", nil, ck).Code)
	assert.Equal(t, http.StatusInternalServerError, e.do(http.MethodPost, "/api/v2/torrents/createTags", url.Values{"tags": {"b"}}, ck).Code)
	_, err := LoadSettings(context.Background(), e.db)
	require.Error(t, err)
	_, err = SaveSettings(context.Background(), e.db, Settings{})
	require.Error(t, err)

	// 数据库关了：校验不了令牌（登录、已登录的请求都回 503）；指定的下载器查不了
	sqlDB, err := e.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	assert.Equal(t, http.StatusServiceUnavailable, e.do(http.MethodGet, "/api/v2/app/version", nil, ck).Code)
	w := e.do(http.MethodPost, "/api/v2/auth/login", url.Values{"username": {"x"}, "password": {plain}}, nil)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	_, err = SaveSettings(context.Background(), e.db, Settings{DownloaderID: 5})
	require.Error(t, err)
}

// 添加的边界：一次超过 50 个回 Fails.（error:too_many）；种子文件超过 10 MiB 当坏种子；经站点下载失败、推送报错记失败原因，
// 失败原因最多记 10 条；按 announce-list 里的 tracker 认站点；表单解析不了回 400
func TestAddEdgeCases(t *testing.T) {
	e := newEnv(t)
	e.withSites()
	ck := e.login(e.token(apitoken.ScopeQbitCompat))

	urls := make([]string, maxAddItems+1)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://qa.example/download.php?id=%d", i+1)
	}
	w := e.do(http.MethodPost, "/api/v2/torrents/add", url.Values{"urls": {strings.Join(urls, "\n")}}, ck)
	assert.Equal(t, "Fails.", w.Body.String())
	assert.Equal(t, "error:too_many", e.audit.all()[0].Result)

	big := bytes.Repeat([]byte("x"), maxTorrentFile+1)
	assert.Equal(t, http.StatusUnsupportedMediaType, e.postAdd(ck, [][]byte{big}, nil).Code, "超过 10 MiB 的种子文件")

	bad := make([][]byte, 11)
	for i := range bad {
		bad[i] = []byte("not a torrent")
	}
	require.Equal(t, http.StatusUnsupportedMediaType, e.postAdd(ck, bad, nil).Code)
	a := e.audit.all()
	assert.Len(t, a[len(a)-1].Args["reasons"], 10, "失败原因最多记 10 条")

	e.srv.deps.Push = func(context.Context, internal.PushTorrentRequest) (*internal.PushTorrentResult, error) {
		return nil, errDown
	}
	assert.Equal(t, "Fails.", e.postAdd(ck, [][]byte{torrentFile("p", "https://qa.example/announce", "")}, nil).Body.String())
	a = e.audit.all()
	assert.Equal(t, "error:add_failed", a[len(a)-1].Result)
	assert.NotContains(t, fmt.Sprint(a[len(a)-1].Args["reasons"]), "SECRETPASSKEY1234")

	// announce 认不出、announce-list 里有站点的 tracker：认得出
	var pushed []internal.PushTorrentRequest
	e.srv.deps.Push = func(_ context.Context, req internal.PushTorrentRequest) (*internal.PushTorrentResult, error) {
		pushed = append(pushed, req)
		return &internal.PushTorrentResult{Success: true, Skipped: true}, nil
	}
	other, qa := "https://other.example/announce", "https://qa.example/announce"
	info := fmt.Sprintf("d6:lengthi1024e4:name4:list12:piece lengthi16384e6:pieces20:%se", strings.Repeat("x", 20))
	withList := []byte(fmt.Sprintf("d8:announce%d:%s13:announce-listll%d:%sel%d:%see4:info%se",
		len(other), other, len(other), other, len(qa), qa, info))
	require.Equal(t, "Ok.", e.postAdd(ck, [][]byte{withList}, nil).Body.String())
	require.Len(t, pushed, 1)
	assert.Equal(t, "qasite", pushed[0].SiteID, "announce-list 里的 tracker")

	// 表单解析不了
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/add", strings.NewReader("urls=%zz"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// 经站点下载失败：记失败原因，不推送
func TestAddSiteDownloadFails(t *testing.T) {
	e := newEnv(t)
	site := e.withSites()
	site.fail = errDown
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	w := e.do(http.MethodPost, "/api/v2/torrents/add", url.Values{"urls": {"https://qa.example/download.php?id=9"}}, ck)
	assert.Equal(t, "Fails.", w.Body.String())
	assert.Empty(t, e.gotPushes())
	a := e.audit.all()
	assert.Contains(t, fmt.Sprint(a[len(a)-1].Args["reasons"]), "经站点 qasite 下载种子 9 失败")
}

// 登录的边界：表单解析不了 400；用户名按字符截断（不截半个汉字）；没有端口的来源地址照样记
func TestLoginEdgeCases(t *testing.T) {
	e := newEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/login", strings.NewReader("username=%zz"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	assert.Equal(t, "中", truncate("中文", 4), "4 个字节里只放得下一个汉字")
	assert.Equal(t, "ab", truncate("ab", 4))

	req = httptest.NewRequest(http.MethodPost, "/api/v2/auth/login", strings.NewReader(url.Values{"username": {"x"}, "password": {"wrong"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "192.0.2.77"
	rec = httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	assert.Equal(t, "Fails.", rec.Body.String())
	assert.Equal(t, "ip:192.0.2.77", e.audit.all()[0].ChannelUserID)
}

// 会话：新登录时清掉闲置过期的（并通知 onDrop）；登录锁定表满了时先清掉早已过期的记录
func TestSessionAndLockCleanup(t *testing.T) {
	var dropped []string
	ss := newSessions(func(sid string) { dropped = append(dropped, sid) })
	start := time.Unix(1700000000, 0)
	ss.put("old", &session{seen: start})
	ss.put("new", &session{seen: start.Add(sessionTTL + time.Minute)})
	assert.Equal(t, []string{"old"}, dropped)
	_, ok := ss.get("old", start.Add(sessionTTL+time.Minute))
	assert.False(t, ok)

	l := newLoginLock()
	for i := range maxLockIPs {
		l.fail(fmt.Sprintf("ip-%d", i), start)
	}
	l.fail("fresh", start.Add(loginWindow+loginBan+time.Minute))
	assert.Len(t, l.m, 1, "早已过期的记录都清掉了")
	assert.Contains(t, l.m, "fresh")
}

// New 不给时钟时用当前时间；监听地址记在兼容入口上
func TestServerDefaults(t *testing.T) {
	s := New(Deps{})
	assert.WithinDuration(t, time.Now(), s.deps.Now(), time.Minute)
	assert.Empty(t, s.Addr())
	s.SetAddr("127.0.0.1:18132")
	assert.Equal(t, "127.0.0.1:18132", s.Addr())
}
