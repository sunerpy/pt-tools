package servarr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/fsutil"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

const tmdbKey = "0123456789abcdef0123456789abcdef"

type fakeCipher struct{}

func (fakeCipher) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (fakeCipher) Decrypt(c string) (string, error) {
	if !strings.HasPrefix(c, "enc:") {
		return "", errors.New("bad cipher text")
	}
	return strings.TrimPrefix(c, "enc:"), nil
}

// fakeAudit 记下审计。
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

// TMDB 上的假条目：电影星际穿越、盗梦空间；剧集最后生还者（TVDB 392256，第 0–2 季）、现在就出发（TMDB 上没有 TVDB 编号）。
var fakeTMDB = map[string]string{
	"/3/movie/157336": `{"id":157336,"title":"星际穿越","original_title":"Interstellar","release_date":"2014-11-05",
		"imdb_id":"tt0816692","poster_path":"/interstellar.jpg","overview":"一队探险家穿过虫洞。"}`,
	"/3/movie/27205": `{"id":27205,"title":"盗梦空间","original_title":"Inception","release_date":"2010-07-15","imdb_id":"tt1375666",
		"poster_path":"/inception.jpg"}`,
	"/3/tv/100088": `{"id":100088,"name":"最后生还者","original_name":"The Last of Us","first_air_date":"2023-01-15",
		"number_of_seasons":2,"poster_path":"/tlou.jpg","overview":"末日之后。","external_ids":{"imdb_id":"tt3581920","tvdb_id":392256},
		"seasons":[{"season_number":0,"name":"特别篇","episode_count":2,"air_date":"2023-01-10"},
			{"season_number":1,"name":"第 1 季","episode_count":9,"air_date":"2023-01-15"},
			{"season_number":2,"name":"第 2 季","episode_count":3,"air_date":"2025-04-13"}]}`,
	"/3/tv/100088/season/1": `{"season_number":1,"air_date":"2023-01-15","episodes":[
		{"episode_number":1,"name":"当你迷失在黑暗中","air_date":"2023-01-15"},{"episode_number":2,"name":"感染","air_date":"2023-01-22"},
		{"episode_number":3,"name":"漫长岁月","air_date":"2023-01-29"},{"episode_number":4,"name":"请握紧我的手","air_date":"2023-02-05"},
		{"episode_number":5,"name":"忍耐与生存","air_date":"2023-02-10"},{"episode_number":6,"name":"亲人","air_date":"2023-02-19"},
		{"episode_number":7,"name":"抛下","air_date":"2023-02-26"},{"episode_number":8,"name":"当我们需要时","air_date":"2023-03-05"},
		{"episode_number":9,"name":"寻找光明","air_date":"2023-03-12"}]}`,
	"/3/tv/100088/season/2": `{"season_number":2,"air_date":"2025-04-13","episodes":[
		{"episode_number":1,"name":"未来","air_date":"2025-04-13"},{"episode_number":2,"name":"穿越","air_date":"2025-04-20"},
		{"episode_number":3,"name":"路","air_date":"2026-12-01"}]}`,
	"/3/tv/231620": `{"id":231620,"name":"现在就出发","original_name":"现在就出发","first_air_date":"2023-08-27","number_of_seasons":1,
		"external_ids":{"imdb_id":"","tvdb_id":null},"seasons":[{"season_number":1,"episode_count":2,"air_date":"2023-08-27"}]}`,
	"/3/tv/231620/season/1": `{"season_number":1,"air_date":"2023-08-27","episodes":[
		{"episode_number":1,"name":"第一期","air_date":"2023-08-27"},{"episode_number":2,"name":"第二期","air_date":"2023-09-03"}]}`,
	"/3/find/392256": `{"movie_results":[],"tv_results":[{"id":100088,"name":"最后生还者","original_name":"The Last of Us",
		"first_air_date":"2023-01-15"}]}`,
}

// tmdbServer 是假的 TMDB：按路径回 fakeTMDB；fail 里的路径回 500（测 TMDB 出错），slow 里的路径等一会儿再回（请求取消时马上结束）。
type tmdbServer struct {
	*httptest.Server
	mu   sync.Mutex
	fail map[string]bool
	slow map[string]time.Duration
	hits map[string]int
}

func newTMDB(t *testing.T) *tmdbServer {
	ts := &tmdbServer{fail: map[string]bool{}, slow: map[string]time.Duration{}, hits: map[string]int{}}
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("api_key") != tmdbKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		ts.mu.Lock()
		ts.hits[r.URL.Path]++
		fail, slow := ts.fail[r.URL.Path], ts.slow[r.URL.Path]
		ts.mu.Unlock()
		if slow > 0 {
			select {
			case <-time.After(slow):
			case <-r.Context().Done():
				return
			}
		}
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.URL.Path == "/3/search/tv" {
			q := r.URL.Query().Get("query")
			if strings.Contains(q, "Last") || strings.Contains(q, "生还") {
				_, _ = w.Write([]byte(`{"results":[{"id":100088,"name":"最后生还者","original_name":"The Last of Us","first_air_date":"2023-01-15",
					"poster_path":"/tlou.jpg"},{"id":231620,"name":"现在就出发","first_air_date":"2023-08-27"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"results":[]}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/3/find/") {
			if b, ok := fakeTMDB[r.URL.Path]; ok {
				_, _ = w.Write([]byte(b))
				return
			}
			_, _ = w.Write([]byte(`{"movie_results":[],"tv_results":[]}`))
			return
		}
		if b, ok := fakeTMDB[r.URL.Path]; ok {
			_, _ = w.Write([]byte(b))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status_code":34,"status_message":"The resource you requested could not be found."}`))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func (ts *tmdbServer) setFail(path string, on bool) {
	ts.mu.Lock()
	ts.fail[path] = on
	ts.mu.Unlock()
}

func (ts *tmdbServer) setSlow(path string, d time.Duration) {
	ts.mu.Lock()
	ts.slow[path] = d
	ts.mu.Unlock()
}

func (ts *tmdbServer) hitCount(path string) int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.hits[path]
}

// env 是测试环境：真的订阅服务（假 TMDB）、令牌库、两个媒体库与兼容入口。
type env struct {
	t      *testing.T
	ctx    context.Context
	db     *gorm.DB
	subs   *subscribe.Service
	tokens *apitoken.Store
	audit  *fakeAudit
	tmdb   *tmdbServer
	srv    *Server
	h      http.Handler
	key    string
	logs   *observer.ObservedLogs

	mu  sync.Mutex
	now time.Time

	movieLib, tvLib, animeLib models.MediaLibrary
}

func (e *env) Now() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.now
}

func (e *env) advance(d time.Duration) {
	e.mu.Lock()
	e.now = e.now.Add(d)
	e.mu.Unlock()
}

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "t.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(MEMORY)&_pragma=synchronous(OFF)"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, derr := db.DB(); derr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(
		&models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{}, &models.MediaCache{},
		&models.MediaTransferHistory{}, &models.TorrentInfo{}, &models.DownloaderSetting{},
		&models.MediaQualityProfile{}, &models.MediaSubscription{}, &models.MediaSubscriptionTorrent{},
		&models.MediaSubscribeSetting{}, &models.MediaDoubanSource{}, &models.MediaDoubanItem{},
		&models.MediaSubscriptionEpisode{}, &models.MediaEpisodeRule{}, &models.MediaSubscriptionRun{}, &models.MediaLibrary{},
		&models.MediaTorrentBlock{}, &models.MediaServer{}, &models.MediaServerItem{}, &models.MediaIssue{},
		&models.TorrentInfoArchive{}, &models.SiteSetting{}, &models.SettingsGlobal{},
		&models.APIToken{}, &models.ServarrTag{}, &models.ServarrTagLink{},
	))
	return db
}

func newEnv(t *testing.T) *env {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	saved := global.GlobalLogger
	global.GlobalLogger = zap.New(core)
	t.Cleanup(func() { global.GlobalLogger = saved })

	db := newDB(t)
	ts := newTMDB(t)
	rec := recognize.New(recognize.Config{DB: db, Cipher: fakeCipher{}, BaseURL: ts.URL + "/3", ImageBaseURL: ts.URL + "/t/p/", RatePerSecond: 1000})
	key := tmdbKey
	_, err := rec.SaveSettings(context.Background(), recognize.SettingsInput{TMDBKey: &key})
	require.NoError(t, err)

	e := &env{t: t, ctx: context.Background(), db: db, audit: &fakeAudit{}, tmdb: ts, logs: logs, now: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)}
	e.subs = subscribe.New(subscribe.Config{DB: db, Recognizer: rec, Now: e.Now, Jitter: func(time.Duration) time.Duration { return 0 }})
	e.tokens = apitoken.New(db)
	_, e.key, err = e.tokens.Create(e.ctx, apitoken.CreateInput{Name: "seerr", Scopes: []string{apitoken.ScopeServarr}}, "admin")
	require.NoError(t, err)

	dir := t.TempDir()
	e.movieLib = models.MediaLibrary{Name: "电影", Kind: models.MediaKindMovie, Path: filepath.Join(dir, "movies"), Enabled: true}
	e.tvLib = models.MediaLibrary{Name: "剧集", Kind: models.MediaKindTV, Path: filepath.Join(dir, "tv"), Enabled: true}
	e.animeLib = models.MediaLibrary{Name: "动画", Kind: models.MediaKindTV, Anime: true, Path: filepath.Join(dir, "anime"), Enabled: true}
	off := models.MediaLibrary{Name: "停用的", Kind: models.MediaKindMovie, Path: filepath.Join(dir, "off"), Enabled: false}
	for _, l := range []*models.MediaLibrary{&e.movieLib, &e.tvLib, &e.animeLib, &off} {
		require.NoError(t, db.Create(l).Error)
	}
	e.srv = New(Deps{
		DB: db, Tokens: e.tokens, Audit: e.audit, Subs: e.subs,
		Libraries: func(ctx context.Context) ([]models.MediaLibrary, error) {
			var rows []models.MediaLibrary
			return rows, db.WithContext(ctx).Order("id").Find(&rows).Error
		},
		TMDB: rec.TMDB,
		DiskUsage: func(path string) (fsutil.Usage, error) {
			if strings.HasSuffix(path, "anime") {
				return fsutil.Usage{}, errors.New("读不到")
			}
			return fsutil.Usage{Free: 100 << 30, Total: 400 << 30}, nil
		},
		Now: e.Now,
	})
	e.h = e.srv.Handler()
	return e
}

// response 是一次请求的结果。
type response struct {
	status int
	body   []byte
}

func (r response) json(t *testing.T) any {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal(r.body, &v), "回应不是 JSON：%s", r.body)
	return v
}

func (r response) obj(t *testing.T) map[string]any {
	t.Helper()
	v, ok := r.json(t).(map[string]any)
	require.True(t, ok, "回应不是对象：%s", r.body)
	return v
}

func (r response) list(t *testing.T) []any {
	t.Helper()
	v, ok := r.json(t).([]any)
	require.True(t, ok, "回应不是数组：%s", r.body)
	return v
}

// call 发一个请求：path 从 /servarr 起；key 为空时不带 Key。
func (e *env) call(method, path, key string, body any) response {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		if s, ok := body.(string); ok {
			rd = strings.NewReader(s)
		} else {
			b, err := json.Marshal(body)
			require.NoError(e.t, err)
			rd = bytes.NewReader(b)
		}
	}
	if key != "" {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		path += sep + "apikey=" + key
	}
	req := httptest.NewRequest(method, path, rd)
	req.RemoteAddr = "192.0.2.10:5055"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return response{status: rec.Code, body: rec.Body.Bytes()}
}

// radarr、sonarr 带着 Key 请求对应的前缀（path 从 /api/v3 以后起）。
func (e *env) radarr(method, path string, body any) response {
	e.t.Helper()
	return e.call(method, radarrBase+apiPath+path, e.key, body)
}

func (e *env) sonarr(method, path string, body any) response {
	e.t.Helper()
	return e.call(method, sonarrBase+apiPath+path, e.key, body)
}

// organized 记一条整理好的记录（模拟整理入库）。
func (e *env) organized(kind string, tmdbID, season, episode int, target string, size int64) {
	e.t.Helper()
	row := models.MediaTransferHistory{
		InfoHash: "h-" + target, SourcePath: "/dl/" + target, TargetPath: target, MediaType: kind, TMDBID: tmdbID,
		Season: season, Episode: episode, Size: size, Status: models.MediaTransferDone, Mode: models.MediaModeHardlink,
	}
	require.NoError(e.t, e.db.Create(&row).Error)
}

// sub 读订阅（按条目与季）；没有时为空。
func (e *env) sub(kind string, tmdbID, season int) *models.MediaSubscription {
	e.t.Helper()
	var row models.MediaSubscription
	require.NoError(e.t, e.db.Where("media_type = ? AND tmdb_id = ? AND season = ?", kind, tmdbID, season).Limit(1).Find(&row).Error)
	if row.ID == 0 {
		return nil
	}
	return &row
}

// audits 是这个命令的审计（命令是方法与路径）。
func (e *env) audits(command string) []app.AuditEntry {
	var out []app.AuditEntry
	for _, a := range e.audit.all() {
		if a.Command == command {
			out = append(out, a)
		}
	}
	return out
}

func newRecorder() *httptest.ResponseRecorder { return httptest.NewRecorder() }

// tmdbClient 是测试里直接用的 TMDB 客户端（预热缓存用）。
func (e *env) tmdbClient() *tmdb.Client {
	c, err := e.srv.deps.TMDB(e.ctx)
	require.NoError(e.t, err)
	return c
}
