package qbitcompat

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestMain(m *testing.M) {
	global.GlobalLogger = zap.NewNop()
	os.Exit(m.Run())
}

// fakeDL 是后端下载器。没覆写的方法来自 nil 接口，调到就 panic —— 测试能看出兼容入口调了不该调的方法。
type fakeDL struct {
	downloader.Downloader
	mu       sync.Mutex
	torrents []downloader.Torrent
	files    map[string][]downloader.TorrentFile
	trackers map[string][]downloader.TorrentTracker
	labels   []string
	paths    []string
	status   downloader.ClientStatus
	free     int64
	calls    []string
	// delay 让暂停慢一点（测审计里的耗时）
	delay time.Duration
	// listCalls 是 GetAllTorrents 被调用的次数，byCalls 是 GetTorrentsBy 的
	listCalls, byCalls int
}

func (f *fakeDL) GetAllTorrents() ([]downloader.Torrent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return append([]downloader.Torrent(nil), f.torrents...), nil
}

func (f *fakeDL) GetTorrent(id string) (downloader.Torrent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.torrents {
		if strings.EqualFold(t.InfoHash, id) || t.ID == id {
			return t, nil
		}
	}
	return downloader.Torrent{}, downloader.ErrTorrentNotFound
}

// GetTorrentsBy 只按 hash 过滤（兼容入口只这样用）。
func (f *fakeDL) GetTorrentsBy(filter downloader.TorrentFilter) ([]downloader.Torrent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byCalls++
	var out []downloader.Torrent
	for _, t := range f.torrents {
		if slices.ContainsFunc(filter.Hashes, func(h string) bool { return strings.EqualFold(h, t.InfoHash) }) {
			out = append(out, t)
		}
	}
	return out, nil
}

// list 让一个种子出现在下载器里（已经有了就不动）。
func (f *fakeDL) list(t downloader.Torrent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.torrents {
		if strings.EqualFold(x.InfoHash, t.InfoHash) {
			return
		}
	}
	f.torrents = append(f.torrents, t)
}

func (f *fakeDL) GetTorrentFiles(id string) ([]downloader.TorrentFile, error) {
	return f.files[id], nil
}

func (f *fakeDL) GetTorrentTrackers(id string) ([]downloader.TorrentTracker, error) {
	return f.trackers[id], nil
}
func (f *fakeDL) GetClientLabels() ([]string, error)                { return f.labels, nil }
func (f *fakeDL) GetClientPaths() ([]string, error)                 { return f.paths, nil }
func (f *fakeDL) GetClientStatus() (downloader.ClientStatus, error) { return f.status, nil }
func (f *fakeDL) GetSpeedLimit() (downloader.SpeedLimit, error)     { return downloader.SpeedLimit{}, nil }
func (f *fakeDL) GetClientFreeSpace(context.Context) (int64, error) { return f.free, nil }

func (f *fakeDL) log(format string, args ...any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
	return nil
}

func (f *fakeDL) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeDL) PauseTorrents(ids []string) error {
	time.Sleep(f.delay)
	return f.log("pause %s", strings.Join(ids, ","))
}

func (f *fakeDL) ResumeTorrents(ids []string) error {
	return f.log("resume %s", strings.Join(ids, ","))
}

func (f *fakeDL) RemoveTorrents(ids []string, data bool) error {
	return f.log("remove %s data=%t", strings.Join(ids, ","), data)
}

func (f *fakeDL) SetTorrentCategory(id, c string) error { return f.log("category %s=%s", id, c) }
func (f *fakeDL) SetTorrentTags(id, tags string) error  { return f.log("tags %s=%s", id, tags) }

// fakeQB 是后端为 qB 时的样子：多了 removeTags 与 createCategory 两个可选能力。
type fakeQB struct{ *fakeDL }

func (q fakeQB) RemoveTorrentTags(ids []string, tags string) error {
	return q.log("removeTags %s=%s", strings.Join(ids, ","), tags)
}

func (q fakeQB) CreateCategory(name, savePath string) error {
	return q.log("createCategory %s=%s", name, savePath)
}

type fakeAudit struct {
	mu      sync.Mutex
	entries []app.AuditEntry
}

func (a *fakeAudit) Record(_ context.Context, e app.AuditEntry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, e)
	return nil
}

func (a *fakeAudit) all() []app.AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]app.AuditEntry(nil), a.entries...)
}

type env struct {
	t  *testing.T
	db *gorm.DB
	// asTR 让后端当成 Transmission：没有 qB 的可选能力
	asTR bool
	// fieldPerFile 让 postAdd 拿文件名当上传字段名（qbittorrent-api 的做法）
	fieldPerFile bool
	srv          *Server
	h            http.Handler
	dl           *fakeDL
	tokens       *apitoken.Store
	audit        *fakeAudit
	now          time.Time
	dlSet        models.DownloaderSetting

	mu     sync.Mutex
	pushes []internal.PushTorrentRequest
}

const (
	hashDebian = "8c212779b4abde7c6bc608063a0d008b7e40ce32"
	hashMovie  = "1111111111111111111111111111111111111111"
	hashTR     = "2222222222222222222222222222222222222222"
)

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "t.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.APIToken{}, &models.QbitCompatSetting{}, &models.QbitCompatTorrent{}, &models.DownloaderSetting{}, &models.TorrentInfo{}))
	e := &env{t: t, db: db, audit: &fakeAudit{}, now: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)}
	e.dlSet = models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true, AutoStart: true}
	require.NoError(t, db.Create(&e.dlSet).Error)
	e.dl = &fakeDL{
		torrents: []downloader.Torrent{
			{
				ID: hashDebian, InfoHash: hashDebian, Name: "debian-8.1.0-amd64-CD-1.iso", Progress: 0.161, DateAdded: 1700000000,
				SavePath: "/downloads", Category: "", Tags: "", State: downloader.TorrentDownloading, TotalSize: 657457152,
				DownloadSpeed: 9681262, ETA: 87, Tracker: "https://tracker.example/announce.php?passkey=SECRETPASSKEY1234",
				NumSeeds: 54, NumPeers: 2, AmountLeft: 551000000,
				Raw: map[string]any{"state": "downloading", "num_complete": float64(-1), "num_incomplete": float64(-1), "priority": float64(1), "dl_limit": float64(-1)},
			},
			{
				ID: hashMovie, InfoHash: strings.ToUpper(hashMovie), Name: "Dune.Part.Two.2024.2160p", Progress: 1, IsCompleted: true,
				DateAdded: 1700000100, CompletionOn: 1700003600, SavePath: "/movies", Category: "movies", Tags: "hdsky, 4k",
				State: downloader.TorrentSeeding, TotalSize: 60 << 30, UploadSpeed: 1024, Ratio: 1.5,
				Tracker: "https://pt.example/announce/0123456789abcdef0123456789abcdef", SeedingTime: 3600,
				Raw: map[string]any{"state": "stoppedUP"},
			},
			{
				// Transmission 当后端：没有 qB 原样的字段，状态按接口换算
				ID: "7", InfoHash: hashTR, Name: "tr-torrent", Progress: 0.5, State: downloader.TorrentPaused, TotalSize: 1000,
				AmountLeft: 500, ETA: -1,
			},
		},
		files: map[string][]downloader.TorrentFile{hashDebian: {{Index: 0, Name: "debian.iso", Size: 657457152, Progress: 0.161, Priority: 1}}},
		trackers: map[string][]downloader.TorrentTracker{hashDebian: {{
			URL: "https://tracker.example/announce.php?passkey=SECRETPASSKEY1234", Status: 2, Seeds: 54, Peers: 2,
			Message: "passkey=SECRETPASSKEY1234 已失效",
		}}},
		labels: []string{"movies", "tv"}, paths: []string{"/downloads"}, free: 500 << 30,
		status: downloader.ClientStatus{UpSpeed: 1024, DlSpeed: 9681262, SessionDlData: 100, SessionUpData: 200, DlData: 1000, UpData: 2000},
	}
	e.tokens = apitoken.New(db)
	e.srv = New(Deps{
		DB: db, Tokens: e.tokens, Audit: e.audit, Now: func() time.Time { return e.now },
		Instance: func(context.Context, string) (downloader.Downloader, error) {
			if e.asTR {
				return e.dl, nil
			}
			return fakeQB{e.dl}, nil
		},
		Push: func(_ context.Context, req internal.PushTorrentRequest) (*internal.PushTorrentResult, error) {
			e.mu.Lock()
			e.pushes = append(e.pushes, req)
			e.mu.Unlock()
			// 推送成功的种子出现在下载器里：添加时间是现在，带着推送给的分类与标签
			if p, err := v2.ParseTorrent(req.TorrentData); err == nil {
				h := strings.ToLower(p.InfoHash)
				e.dl.list(downloader.Torrent{ID: h, InfoHash: h, Name: p.Name, DateAdded: e.now.Unix(), Category: req.Category, Tags: req.Tags})
			}
			return &internal.PushTorrentResult{Success: true}, nil
		},
	})
	// 加完以后只看一次下载器（等下载器列出种子的用例自己设）
	e.srv.observe = 0
	e.h = e.srv.Handler()
	return e
}

func (e *env) token(scopes ...string) string {
	e.t.Helper()
	_, plain, err := e.tokens.Create(context.Background(), apitoken.CreateInput{Name: "qb " + strings.Join(scopes, ","), Scopes: scopes}, "admin")
	require.NoError(e.t, err)
	return plain
}

func (e *env) do(method, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	var body *strings.Reader
	if form != nil && method == http.MethodPost {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
		if form != nil {
			path += "?" + form.Encode()
		}
	}
	req := httptest.NewRequest(method, path, body)
	req.RemoteAddr = "192.0.2.10:5000"
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if ck != nil {
		req.AddCookie(ck)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	return w
}

func (e *env) login(plain string) *http.Cookie {
	e.t.Helper()
	w := e.do(http.MethodPost, "/api/v2/auth/login", url.Values{"username": {"moviepilot"}, "password": {plain}}, nil)
	require.Equal(e.t, http.StatusOK, w.Code)
	require.Equal(e.t, "Ok.", w.Body.String())
	for _, c := range w.Result().Cookies() {
		if c.Name == "SID" {
			return c
		}
	}
	e.t.Fatal("没有 SID")
	return nil
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v), w.Body.String())
	return v
}

// qbFields 是 qB 文档里各接口回应表列出的字段（testdata/qb_fields.json 从文档抽出来）。
func qbFields(t *testing.T) map[string][]string {
	t.Helper()
	b, err := os.ReadFile("testdata/qb_fields.json")
	require.NoError(t, err)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &raw))
	out := map[string][]string{}
	for k, v := range raw {
		var list []string
		if json.Unmarshal(v, &list) == nil {
			out[k] = list
		}
	}
	return out
}

func assertKeys(t *testing.T, got map[string]any, want []string, what string) {
	t.Helper()
	for _, k := range want {
		assert.Contains(t, got, k, "%s 缺字段 %s", what, k)
	}
}

// 登录：令牌当密码；失败回 200 Fails.；成功回 Ok. 与 SID（HttpOnly、SameSite=Strict）；没有 qbit:compat 的令牌登录不了
func TestLogin(t *testing.T) {
	e := newEnv(t)
	plain := e.token(apitoken.ScopeQbitCompat)
	w := e.do(http.MethodPost, "/api/v2/auth/login", url.Values{"username": {"x"}, "password": {"ptt_1_wrong"}}, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Fails.", w.Body.String())
	assert.Empty(t, w.Result().Cookies())

	appOnly := e.token(apitoken.ScopeAppRead)
	assert.Equal(t, "Fails.", e.do(http.MethodPost, "/api/v2/auth/login", url.Values{"password": {appOnly}}, nil).Body.String(), "要 qbit:compat")

	ck := e.login(plain)
	assert.True(t, ck.HttpOnly)
	assert.Equal(t, http.SameSiteStrictMode, ck.SameSite)
	assert.Len(t, ck.Value, 43)
	w = e.do(http.MethodGet, "/api/v2/app/version", nil, ck)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, AppVersion, w.Body.String())

	audit := e.audit.all()
	require.Len(t, audit, 2, "两次失败的登录记审计，成功的不记")
	assert.Equal(t, ChannelType, audit[0].ChannelType)
	assert.Equal(t, "ip:192.0.2.10", audit[0].ChannelUserID)
	assert.Equal(t, "denied:bad_credentials", audit[0].Result)
	assert.Equal(t, "denied:scope", audit[1].Result)
	assert.NotContains(t, audit[1].Args, "password")
}

// 同一 IP 15 分钟内失败 5 次锁 15 分钟（403），锁定期间正确的令牌也登录不了；过了锁定时间可以再试
func TestLoginLockout(t *testing.T) {
	e := newEnv(t)
	plain := e.token(apitoken.ScopeQbitCompat)
	for range loginMaxFails {
		assert.Equal(t, "Fails.", e.do(http.MethodPost, "/api/v2/auth/login", url.Values{"password": {"bad"}}, nil).Body.String())
	}
	w := e.do(http.MethodPost, "/api/v2/auth/login", url.Values{"password": {plain}}, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	e.now = e.now.Add(loginBan + time.Second)
	e.login(plain)
}

// 没登录 403；方法不对 405（带 Allow）；GET 接口也接受 POST；没有的接口 404；退出以后会话作废
func TestRoutingAndSession(t *testing.T) {
	e := newEnv(t)
	plain := e.token(apitoken.ScopeQbitCompat)
	w := e.do(http.MethodGet, "/api/v2/torrents/info", nil, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "Forbidden", w.Body.String())
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodGet, "/api/v2/torrents/info", nil, &http.Cookie{Name: "SID", Value: "nope"}).Code)

	w = e.do(http.MethodGet, "/api/v2/auth/login", nil, nil)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Equal(t, http.MethodPost, w.Header().Get("Allow"))
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/api/v2/nope/thing", nil, nil).Code)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/other", nil, nil).Code)

	ck := e.login(plain)
	assert.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/app/webapiVersion", url.Values{}, ck).Code, "GET 接口也接受 POST")
	assert.Equal(t, WebAPIVersion, e.do(http.MethodGet, "/api/v2/app/webapiVersion", nil, ck).Body.String())
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/auth/logout", url.Values{}, ck).Code)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodGet, "/api/v2/app/version", nil, ck).Code, "退出以后")
}

// 会话按令牌复查：撤销、过期以后马上 403；闲置超过 1 小时也要重新登录
func TestSessionFollowsToken(t *testing.T) {
	e := newEnv(t)
	plain := e.token(apitoken.ScopeQbitCompat)
	ck := e.login(plain)
	toks, err := e.tokens.List(context.Background())
	require.NoError(t, err)
	require.NoError(t, e.tokens.Revoke(context.Background(), toks[0].ID))
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodGet, "/api/v2/app/version", nil, ck).Code, "撤销立即生效")

	ck = e.login(e.token(apitoken.ScopeQbitCompat))
	e.now = e.now.Add(sessionTTL + time.Minute)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodGet, "/api/v2/app/version", nil, ck).Code, "闲置过期")
}

// torrents/info：字段齐全（按 qB 文档）；hash 小写；tracker 与 magnet 里没有 passkey；状态按 qB 的值
func TestTorrentsInfoContract(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	w := e.do(http.MethodGet, "/api/v2/torrents/info", nil, ck)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "SECRETPASSKEY1234")
	assert.NotContains(t, w.Body.String(), "0123456789abcdef0123456789abcdef")
	list := decode[[]map[string]any](t, w)
	require.Len(t, list, 3)
	want := qbFields(t)["torrent"]
	for _, item := range list {
		assertKeys(t, item, want, "torrents/info")
	}
	byHash := map[string]map[string]any{}
	for _, item := range list {
		byHash[item["hash"].(string)] = item
	}
	require.Contains(t, byHash, hashMovie, "hash 一律小写")
	assert.Equal(t, "downloading", byHash[hashDebian]["state"])
	assert.Equal(t, "pausedUP", byHash[hashMovie]["state"], "qB 5 的 stoppedUP 换成 4.x 的 pausedUP")
	assert.Equal(t, "pausedDL", byHash[hashTR]["state"], "Transmission 的暂停按进度换算")
	assert.EqualValues(t, etaInfinite, byHash[hashTR]["eta"])
	assert.Equal(t, "magnet:?xt=urn:btih:"+hashDebian+"&dn=debian-8.1.0-amd64-CD-1.iso", byHash[hashDebian]["magnet_uri"])
	assert.Contains(t, byHash[hashDebian]["tracker"], "tracker.example")
}

// torrents/info 的筛选与排序
func TestTorrentsInfoFilters(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	names := func(q url.Values) []string {
		t.Helper()
		w := e.do(http.MethodGet, "/api/v2/torrents/info", q, ck)
		require.Equal(t, http.StatusOK, w.Code)
		var out []string
		for _, item := range decode[[]map[string]any](t, w) {
			out = append(out, item["name"].(string))
		}
		return out
	}
	assert.Equal(t, []string{"debian-8.1.0-amd64-CD-1.iso", "tr-torrent"}, names(url.Values{"filter": {"downloading"}}))
	assert.Equal(t, []string{"Dune.Part.Two.2024.2160p"}, names(url.Values{"filter": {"completed"}}))
	assert.Equal(t, []string{"Dune.Part.Two.2024.2160p", "tr-torrent"}, names(url.Values{"filter": {"paused"}}))
	assert.Equal(t, []string{"Dune.Part.Two.2024.2160p"}, names(url.Values{"category": {"movies"}}))
	assert.Equal(t, []string{"debian-8.1.0-amd64-CD-1.iso", "tr-torrent"}, names(url.Values{"category": {""}}), "空分类是没有分类的")
	assert.Equal(t, []string{"Dune.Part.Two.2024.2160p"}, names(url.Values{"tag": {"4k"}}))
	assert.Equal(t, []string{"Dune.Part.Two.2024.2160p"}, names(url.Values{"hashes": {strings.ToUpper(hashMovie) + "|nope"}}))
	assert.Equal(t, []string{"tr-torrent", "debian-8.1.0-amd64-CD-1.iso", "Dune.Part.Two.2024.2160p"}, names(url.Values{"sort": {"size"}}))
	assert.Equal(t, []string{"Dune.Part.Two.2024.2160p", "debian-8.1.0-amd64-CD-1.iso"}, names(url.Values{"sort": {"size"}, "reverse": {"true"}, "limit": {"2"}}))
	assert.Equal(t, []string{"Dune.Part.Two.2024.2160p"}, names(url.Values{"sort": {"size"}, "offset": {"-1"}}))
}

// properties、files、trackers：字段按 qB 文档；没有的 hash 回 404；tracker 的地址与回复里没有 passkey
func TestTorrentDetailContract(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	f := qbFields(t)
	w := e.do(http.MethodGet, "/api/v2/torrents/properties", url.Values{"hash": {strings.ToUpper(hashDebian)}}, ck)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertKeys(t, decode[map[string]any](t, w), f["properties"], "properties")
	w = e.do(http.MethodGet, "/api/v2/torrents/properties", url.Values{"hash": {"ffff"}}, ck)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "Torrent hash was not found", w.Body.String())

	w = e.do(http.MethodGet, "/api/v2/torrents/files", url.Values{"hash": {hashDebian}}, ck)
	require.Equal(t, http.StatusOK, w.Code)
	files := decode[[]map[string]any](t, w)
	require.Len(t, files, 1)
	assertKeys(t, files[0], f["files"], "files")

	w = e.do(http.MethodGet, "/api/v2/torrents/trackers", url.Values{"hash": {hashDebian}}, ck)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "SECRETPASSKEY1234")
	trackers := decode[[]map[string]any](t, w)
	require.Len(t, trackers, 1)
	assertKeys(t, trackers[0], f["trackers"], "trackers")
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/api/v2/torrents/trackers", nil, ck).Code, "没给 hash")
}

// 分类、标签、maindata、transfer/info、preferences
func TestCategoriesTagsAndState(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	require.NoError(t, e.db.Create(&models.QbitCompatSetting{ID: 1, Categories: `{"sonarr":"/tv"}`, Tags: `["moviepilot"]`}).Error)

	w := e.do(http.MethodGet, "/api/v2/torrents/categories", nil, ck)
	require.Equal(t, http.StatusOK, w.Code)
	cats := decode[map[string]categoryView](t, w)
	assert.Equal(t, categoryView{Name: "sonarr", SavePath: "/tv"}, cats["sonarr"])
	assert.Contains(t, cats, "movies")
	assert.Contains(t, cats, "tv", "qB 后端自己的分类")

	w = e.do(http.MethodGet, "/api/v2/torrents/tags", nil, ck)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"4k", "hdsky", "moviepilot"}, decode[[]string](t, w))

	f := qbFields(t)
	w = e.do(http.MethodGet, "/api/v2/sync/maindata", url.Values{"rid": {"0"}}, ck)
	require.Equal(t, http.StatusOK, w.Code)
	md := decode[map[string]any](t, w)
	assertKeys(t, md, []string{"rid", "full_update", "torrents", "categories", "tags", "server_state"}, "maindata")
	assert.Equal(t, true, md["full_update"])
	assert.Len(t, md["torrents"], 3)
	assertKeys(t, md["server_state"].(map[string]any), f["transfer_info"], "server_state")
	assert.NotContains(t, w.Body.String(), "SECRETPASSKEY1234")
	rid1 := md["rid"].(float64)
	md = decode[map[string]any](t, e.do(http.MethodGet, "/api/v2/sync/maindata", url.Values{"rid": {"1"}}, ck))
	assert.Greater(t, md["rid"].(float64), rid1)

	w = e.do(http.MethodGet, "/api/v2/transfer/info", nil, ck)
	require.Equal(t, http.StatusOK, w.Code)
	ti := decode[map[string]any](t, w)
	assertKeys(t, ti, f["transfer_info"], "transfer/info")
	assert.EqualValues(t, 9681262, ti["dl_info_speed"])

	w = e.do(http.MethodGet, "/api/v2/app/preferences", nil, ck)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/downloads", decode[map[string]any](t, w)["save_path"])
	assert.Equal(t, "/downloads", e.do(http.MethodGet, "/api/v2/app/defaultSavePath", nil, ck).Body.String())
}

// 没有可用的下载器：读接口回 503，写明原因
func TestNoBackend(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	require.NoError(t, e.db.Model(&models.DownloaderSetting{}).Where("id = ?", e.dlSet.ID).Update("enabled", false).Error)
	w := e.do(http.MethodGet, "/api/v2/torrents/info", nil, ck)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "没有可用的下载器")

	other := models.DownloaderSetting{Name: "tr", Type: "transmission", URL: "http://127.0.0.1:2", Enabled: true}
	require.NoError(t, e.db.Create(&other).Error)
	_, err := SaveSettings(context.Background(), e.db, Settings{DownloaderID: other.ID})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, e.do(http.MethodGet, "/api/v2/torrents/info", nil, ck).Code, "设置里指定的那台")
	_, err = SaveSettings(context.Background(), e.db, Settings{DownloaderID: 999})
	require.ErrorIs(t, err, ErrInvalid)
}

// 登录锁定表有硬上限：满了以后淘汰最早的，不会无限长
func TestLoginLockBounded(t *testing.T) {
	l := newLoginLock()
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	for i := range maxLockIPs + 500 {
		l.fail(fmt.Sprintf("ip-%d", i), now.Add(time.Duration(i)*time.Millisecond))
	}
	assert.LessOrEqual(t, len(l.m), maxLockIPs)
	assert.Contains(t, l.m, fmt.Sprintf("ip-%d", maxLockIPs+499), "最新的留着")
}

// 读接口共用一份短时的快照：客户端高频轮询 maindata、info 时不会每次都去读下载器的全部种子；
// 写操作用最新的列表，写完以后快照作废；单个种子的详情按 hash 查，不读全部
func TestSnapshotForReads(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	calls := func() int {
		e.dl.mu.Lock()
		defer e.dl.mu.Unlock()
		return e.dl.listCalls
	}
	for range 3 {
		require.Equal(t, http.StatusOK, e.do(http.MethodGet, "/api/v2/sync/maindata", nil, ck).Code)
		require.Equal(t, http.StatusOK, e.do(http.MethodGet, "/api/v2/torrents/info", nil, ck).Code)
	}
	assert.Equal(t, 1, calls(), "快照期内只读一次")
	md := decode[map[string]any](t, e.do(http.MethodGet, "/api/v2/sync/maindata", nil, ck))
	assert.EqualValues(t, 5000, md["server_state"].(map[string]any)["refresh_interval"], "建议客户端 5 秒刷新一次")

	require.Equal(t, http.StatusOK, e.do(http.MethodGet, "/api/v2/torrents/properties", url.Values{"hash": {hashDebian}}, ck).Code)
	require.Equal(t, http.StatusOK, e.do(http.MethodGet, "/api/v2/torrents/trackers", url.Values{"hash": {hashDebian}}, ck).Code)
	assert.Equal(t, 1, calls(), "按 hash 查，不读全部")

	e.markCompatAt(hashMovie)
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/torrents/pause", url.Values{"hashes": {hashMovie}}, ck).Code)
	assert.Equal(t, 2, calls(), "写操作用最新的列表")
	require.Equal(t, http.StatusOK, e.do(http.MethodGet, "/api/v2/torrents/info", nil, ck).Code)
	assert.Equal(t, 3, calls(), "写完以后快照作废")
}

// sync/maindata 按 rid 给增量（qB 的做法）：rid 对得上时只给变了的字段、删掉的种子与分类标签的增减；rid 不对或者是 0 时给全量
func TestMaindataIncremental(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	get := func(rid any) map[string]any {
		t.Helper()
		e.srv.invalidate(e.dlSet.ID) // 快照有 2 秒：测的是增量，不等快照过期
		return decode[map[string]any](t, e.do(http.MethodGet, "/api/v2/sync/maindata", url.Values{"rid": {fmt.Sprint(rid)}}, ck))
	}
	full := get(0)
	require.Equal(t, true, full["full_update"])
	require.Len(t, full["torrents"], 3)
	rid := full["rid"]

	same := get(rid)
	assert.Equal(t, false, same["full_update"])
	assert.NotContains(t, same, "torrents", "什么都没变")
	assert.NotContains(t, same, "torrents_removed")

	e.dl.mu.Lock()
	e.dl.torrents[0].Progress = 0.5
	e.dl.torrents = e.dl.torrents[:2] // 去掉 Transmission 的那个
	e.dl.torrents[1].Tags = "hdsky, 4k, new"
	e.dl.mu.Unlock()
	diff := get(same["rid"])
	assert.Equal(t, false, diff["full_update"])
	torrents := diff["torrents"].(map[string]any)
	require.Len(t, torrents, 2)
	assert.Equal(t, map[string]any{"progress": 0.5}, torrents[hashDebian], "只给变了的字段")
	assert.Equal(t, []any{hashTR}, diff["torrents_removed"])
	assert.Equal(t, []any{"new"}, diff["tags"], "新出现的标签")

	stale := get(12345)
	assert.Equal(t, true, stale["full_update"], "rid 对不上：全量")
	assert.Len(t, stale["torrents"], 2)
}

// maindata 的状态跟着会话走：登出、令牌撤销、会话过期时一起丢掉；会话被淘汰（超过 maxSession）时也一样
func TestMaindataStateFollowsSession(t *testing.T) {
	e := newEnv(t)
	states := func() (map[string]int, int) {
		e.srv.syncs.mu.Lock()
		defer e.srv.syncs.mu.Unlock()
		out := map[string]int{}
		for sid, st := range e.srv.syncs.m {
			out[sid] = len(st.torrents)
		}
		return out, e.srv.syncs.total
	}
	plain := e.token(apitoken.ScopeQbitCompat)
	a, b, c := e.login(plain), e.login(plain), e.login(e.token(apitoken.ScopeQbitCompat))
	for _, ck := range []*http.Cookie{a, b, c} {
		require.Equal(t, http.StatusOK, e.do(http.MethodGet, "/api/v2/sync/maindata", nil, ck).Code)
	}
	m, total := states()
	assert.Equal(t, map[string]int{a.Value: 3, b.Value: 3, c.Value: 3}, m)
	assert.Equal(t, 9, total)

	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v2/auth/logout", url.Values{}, a).Code)
	m, total = states()
	assert.NotContains(t, m, a.Value, "登出")
	assert.Equal(t, 6, total)

	// c 用的是后建的那个令牌：撤销它，下一次请求时会话与状态一起丢掉
	first, err := e.tokens.Verify(context.Background(), plain)
	require.NoError(t, err)
	toks, err := e.tokens.List(context.Background())
	require.NoError(t, err)
	for _, tk := range toks {
		if tk.ID != first.ID {
			require.NoError(t, e.tokens.Revoke(context.Background(), tk.ID))
		}
	}
	require.Equal(t, http.StatusForbidden, e.do(http.MethodGet, "/api/v2/sync/maindata", nil, c).Code)
	m, total = states()
	assert.NotContains(t, m, c.Value, "令牌撤销")
	assert.Equal(t, 3, total)

	e.now = e.now.Add(sessionTTL + time.Minute)
	require.Equal(t, http.StatusForbidden, e.do(http.MethodGet, "/api/v2/sync/maindata", nil, b).Code)
	m, total = states()
	assert.Empty(t, m, "会话过期")
	assert.Equal(t, 0, total)

	var dropped []string
	ss := newSessions(func(sid string) { dropped = append(dropped, sid) })
	start := time.Unix(1700000000, 0)
	for i := range maxSession {
		ss.put(fmt.Sprint(i), &session{seen: start.Add(time.Duration(i) * time.Second)})
	}
	ss.put("new", &session{seen: start.Add(time.Hour / 2)})
	assert.Equal(t, []string{"0"}, dropped, "淘汰最久没用的会话时一起通知")
}

// 所有会话的状态一共最多记 maxSyncTorrents 个种子：超出时丢掉最久没用的；一个状态就超过上限时不记（那个会话每次拿全量）
func TestSyncStatesBounded(t *testing.T) {
	x := syncStates{m: map[string]*syncState{}}
	state := func(n int) *syncState {
		st := &syncState{torrents: make(map[string]qbTorrent, n)}
		for i := range n {
			st.torrents[fmt.Sprint(i)] = qbTorrent{}
		}
		return st
	}
	keys := func() []string { return slices.Sorted(maps.Keys(x.m)) }
	assert.Nil(t, x.swap("a", state(15000)))
	assert.Nil(t, x.swap("b", state(5000)))
	assert.Equal(t, []string{"a", "b"}, keys())
	assert.Equal(t, maxSyncTorrents, x.total)

	x.swap("c", state(1))
	assert.Equal(t, []string{"b", "c"}, keys(), "超出：丢掉最久没用的 a")
	assert.Equal(t, 5001, x.total)

	assert.NotNil(t, x.swap("b", state(6000)), "同一个会话换成新的状态")
	assert.Equal(t, 6001, x.total)

	assert.NotNil(t, x.swap("c", state(maxSyncTorrents+1)), "返回上一次的")
	assert.Equal(t, []string{"b"}, keys(), "太大的状态不记，旧的也去掉")
	assert.Equal(t, 6000, x.total)

	x.drop("b")
	x.drop("missing")
	assert.Empty(t, keys())
	assert.Equal(t, 0, x.total)
}

// OwnerTag 不给客户端看：种子列表、maindata、标签列表里都没有，按它筛不出种子；Transmission 只有它一个 label 时不当分类
func TestOwnerTagHidden(t *testing.T) {
	e := newEnv(t)
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	e.dl.torrents[1].Tags = "hdsky, " + OwnerTag + ", 4k"
	require.NoError(t, e.db.Create(&models.QbitCompatSetting{ID: 1, Tags: `["` + OwnerTag + `","made"]`}).Error)

	info := decode[[]map[string]any](t, e.do(http.MethodGet, "/api/v2/torrents/info", url.Values{"hashes": {hashMovie}}, ck))
	require.Len(t, info, 1)
	assert.Equal(t, "hdsky, 4k", info[0]["tags"])
	assert.Empty(t, decode[[]map[string]any](t, e.do(http.MethodGet, "/api/v2/torrents/info", url.Values{"tag": {OwnerTag}}, ck)))
	assert.Equal(t, []any{"4k", "hdsky", "made"}, decode[[]any](t, e.do(http.MethodGet, "/api/v2/torrents/tags", nil, ck)))
	md := decode[map[string]any](t, e.do(http.MethodGet, "/api/v2/sync/maindata", nil, ck))
	assert.NotContains(t, md["tags"], OwnerTag)
	assert.Equal(t, "hdsky, 4k", md["torrents"].(map[string]any)[hashMovie].(map[string]any)["tags"])

	tr := newEnv(t)
	tr.asTR = true
	tr.dlSet.Type = "transmission"
	require.NoError(t, tr.db.Save(&tr.dlSet).Error)
	tr.dl.torrents[2].Category, tr.dl.torrents[2].Tags = OwnerTag, OwnerTag
	ck = tr.login(tr.token(apitoken.ScopeQbitCompat))
	info = decode[[]map[string]any](t, tr.do(http.MethodGet, "/api/v2/torrents/info", url.Values{"hashes": {hashTR}}, ck))
	require.Len(t, info, 1)
	assert.Empty(t, info[0]["category"], "只有 OwnerTag 一个 label：不当分类")
	assert.Empty(t, info[0]["tags"])
	assert.NotContains(t, decode[map[string]any](t, tr.do(http.MethodGet, "/api/v2/torrents/categories", nil, ck)), OwnerTag)
}
