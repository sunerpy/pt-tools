package subscribe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

type fakeCipher struct{}

func (fakeCipher) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (fakeCipher) Decrypt(c string) (string, error) {
	if !strings.HasPrefix(c, "enc:") {
		return "", errors.New("bad cipher text")
	}
	return strings.TrimPrefix(c, "enc:"), nil
}

const tmdbKey = "0123456789abcdef0123456789abcdef"

// varietySeason4 是现在就出发（综艺 231620）第 4 季：一期拆成上下两集，第 3、4 集同一天播出，第 5 集是占位的集名，第 6 集还没有播出日期。
const varietySeason4 = `{"season_number":4,"air_date":"2026-10-01","episodes":[
	{"episode_number":1,"name":"第1期上：看见蔚蓝生机","air_date":"2026-10-01"},
	{"episode_number":2,"name":"第1期下：神秘的热带雨林","air_date":"2026-10-02"},
	{"episode_number":3,"name":"第2期上：哞哞来了！","air_date":"2026-10-08"},
	{"episode_number":4,"name":"第2期下：热舞的哞哞","air_date":"2026-10-08"},
	{"episode_number":5,"name":"第 5 集","air_date":"2026-10-15"},
	{"episode_number":6,"name":"第 6 集"}]}`

// tmdbOverride 按路径换掉假 TMDB 的回应（测试里改 TMDB 上的数据）；为空时用下面固定的。
type tmdbOverride struct {
	mu sync.Mutex
	m  map[string]string
}

func (o *tmdbOverride) set(path, body string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.m == nil {
		o.m = map[string]string{}
	}
	o.m[path] = body
}

func (o *tmdbOverride) get(path string) (string, bool) {
	if o == nil {
		return "", false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	b, ok := o.m[path]
	return b, ok
}

// fakeTMDB：沙丘2（电影 693134）、最后生还者（剧集 100088，第 2 季 3 集，第 3 集 2026-12-01 播出）、
// 现在就出发（综艺 231620，第 4 季见 varietySeason4）。
func newFakeTMDB(t *testing.T) *httptest.Server {
	return newFakeTMDBWith(t, nil)
}

func newFakeTMDBWith(t *testing.T, over *tmdbOverride) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("api_key") != tmdbKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		q := r.URL.Query().Get("query")
		en := r.URL.Query().Get("language") == "en-US"
		if b, ok := over.get(r.URL.Path); ok {
			if b == "404" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(b))
			return
		}
		switch {
		case r.URL.Path == "/3/search/movie" && (strings.Contains(q, "沙丘") || strings.Contains(q, "Dune")):
			_, _ = w.Write([]byte(`{"results":[{"id":693134,"title":"沙丘2","original_title":"Dune: Part Two","release_date":"2024-02-27","popularity":300}]}`))
		case r.URL.Path == "/3/movie/693134" && en:
			_, _ = w.Write([]byte(`{"id":693134,"title":"Dune: Part Two","alternative_titles":{"titles":[{"title":"Dune Part Two"}]}}`))
		case r.URL.Path == "/3/movie/693134":
			_, _ = w.Write([]byte(`{"id":693134,"title":"沙丘2","original_title":"Dune: Part Two","release_date":"2024-02-27","imdb_id":"tt15239678","poster_path":"/dune2.jpg"}`))
		case r.URL.Path == "/3/search/tv" && (strings.Contains(q, "Last of Us") || strings.Contains(q, "最后生还者")):
			_, _ = w.Write([]byte(`{"results":[{"id":100088,"name":"最后生还者","original_name":"The Last of Us","first_air_date":"2023-01-15","popularity":500}]}`))
		case r.URL.Path == "/3/tv/100088" && en:
			_, _ = w.Write([]byte(`{"id":100088,"name":"The Last of Us","alternative_titles":{"results":[{"title":"TLOU"}]}}`))
		case r.URL.Path == "/3/tv/100088":
			_, _ = w.Write([]byte(`{"id":100088,"name":"最后生还者","original_name":"The Last of Us","first_air_date":"2023-01-15","number_of_seasons":2,"poster_path":"/tlou.jpg","external_ids":{"imdb_id":"tt3581920"}}`))
		case r.URL.Path == "/3/tv/100088/season/2":
			_, _ = w.Write([]byte(`{"season_number":2,"air_date":"2025-04-13","episodes":[
				{"episode_number":1,"name":"未来","air_date":"2025-04-13"},{"episode_number":2,"name":"穿越","air_date":"2025-04-20"},
				{"episode_number":3,"name":"路","air_date":"2026-12-01"}]}`))
		case r.URL.Path == "/3/search/tv" && (strings.Contains(q, "Natural High") || strings.Contains(q, "现在就出发")):
			_, _ = w.Write([]byte(`{"results":[{"id":231620,"name":"现在就出发","original_name":"现在就出发","first_air_date":"2023-08-27","popularity":50,"genre_ids":[10764]}]}`))
		case r.URL.Path == "/3/tv/231620" && en:
			_, _ = w.Write([]byte(`{"id":231620,"name":"Natural High","alternative_titles":{"results":[]}}`))
		case r.URL.Path == "/3/tv/231620":
			_, _ = w.Write([]byte(`{"id":231620,"name":"现在就出发","original_name":"现在就出发","first_air_date":"2023-08-27","number_of_seasons":4,"poster_path":"/nh.jpg","genres":[{"id":10764,"name":"真人秀"}]}`))
		case r.URL.Path == "/3/tv/231620/season/4":
			_, _ = w.Write([]byte(varietySeason4))
		case r.URL.Path == "/3/trending/movie/week":
			_, _ = w.Write([]byte(`{"page":1,"total_pages":1,"results":[{"id":693134,"title":"沙丘2","release_date":"2024-02-27"},{"id":1,"title":"别的电影","release_date":"2024-01-01"}]}`))
		case strings.HasPrefix(r.URL.Path, "/3/search/"):
			_, _ = w.Write([]byte(`{"results":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	// 测试库不刷盘（同 core.NewTempDBDir）：Windows runner 上每次写都要刷盘，一条 SQL 能慢到几秒
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "t.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(MEMORY)&_pragma=synchronous(OFF)"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })
	require.NoError(t, db.AutoMigrate(
		&models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{}, &models.MediaCache{},
		&models.MediaTransferHistory{}, &models.TorrentInfo{}, &models.DownloaderSetting{},
		&models.MediaQualityProfile{}, &models.MediaSubscription{}, &models.MediaSubscriptionTorrent{},
		&models.MediaSubscribeSetting{}, &models.MediaDoubanSource{}, &models.MediaDoubanItem{},
		&models.MediaSubscriptionEpisode{}, &models.MediaEpisodeRule{}, &models.MediaSubscriptionRun{}, &models.MediaLibrary{},
	))
	return db
}

// fakeSearch 按关键字回种子，记下搜过的关键字；cached 是搜索缓存里的种子（一键追剧按站点与编号取）。
type fakeSearch struct {
	mu      sync.Mutex
	items   []v2.TorrentItem
	queries []v2.MultiSiteSearchQuery
	err     error
	cached  map[string]v2.TorrentItem
}

func (f *fakeSearch) CachedTorrent(site, id string) (v2.TorrentItem, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	it, ok := f.cached[strings.ToLower(site)+"|"+id]
	return it, ok
}

func (f *fakeSearch) cache(it v2.TorrentItem) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cached == nil {
		f.cached = map[string]v2.TorrentItem{}
	}
	f.cached[strings.ToLower(it.SourceSite)+"|"+it.ID] = it
}

func (f *fakeSearch) Search(_ context.Context, q v2.MultiSiteSearchQuery) (*v2.MultiSiteSearchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, q)
	if f.err != nil {
		return nil, f.err
	}
	return &v2.MultiSiteSearchResult{Items: append([]v2.TorrentItem(nil), f.items...)}, nil
}

func (f *fakeSearch) set(items ...v2.TorrentItem) {
	f.mu.Lock()
	f.items = items
	f.mu.Unlock()
}

// fakeSite 回种子文件。
type fakeSite struct {
	v2.Site
	files map[string][]byte
	// onDownload 在下载种子文件时调用（模拟下载期间订阅被暂停或删掉）
	onDownload func(id string)
}

func (f *fakeSite) Download(_ context.Context, id string) ([]byte, error) {
	if f.onDownload != nil {
		f.onDownload(id)
	}
	if b, ok := f.files[id]; ok {
		return b, nil
	}
	return nil, errors.New("no such torrent")
}

// packBytes 是一个多文件的种子（整季包）：files 是包里的文件名。
func packBytes(name string, files ...string) []byte {
	var list strings.Builder
	for _, f := range files {
		fmt.Fprintf(&list, "d6:lengthi%de4:pathl%d:%see", int64(1)<<30, len(f), f)
	}
	info := fmt.Sprintf("d5:filesl%se4:name%d:%s12:piece lengthi16384e6:pieces20:%se", list.String(), len(name), name, strings.Repeat("y", 20))
	ann := "http://tracker/announce"
	return []byte(fmt.Sprintf("d8:announce%d:%s4:info%se", len(ann), ann, info))
}

func torrentBytes(name string) []byte {
	info := fmt.Sprintf("d6:lengthi%de4:name%d:%s12:piece lengthi16384e6:pieces20:%se", int64(10)<<30, len(name), name, strings.Repeat("x", 20))
	ann := "http://tracker/announce"
	return []byte(fmt.Sprintf("d8:announce%d:%s4:info%se", len(ann), ann, info))
}

type fakeOrganizer struct {
	mu       sync.Mutex
	retired  []uint
	retried  []uint
	retrying map[uint]bool
	// retryOK 为真时重新整理把记录改成已整理（模拟整理成了）
	retryOK bool
	db      *gorm.DB
}

func (f *fakeOrganizer) setRetrying(id uint, on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.retrying == nil {
		f.retrying = map[uint]bool{}
	}
	f.retrying[id] = on
}

func (f *fakeOrganizer) Retrying(id uint) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.retrying[id]
}

func (f *fakeOrganizer) Retire(_ context.Context, id uint, reason string) ([]string, error) {
	f.mu.Lock()
	f.retired = append(f.retired, id)
	f.mu.Unlock()
	return nil, f.db.Model(&models.MediaTransferHistory{}).Where("id = ?", id).Updates(map[string]any{"status": models.MediaTransferRemoved, "message": reason}).Error
}

func (f *fakeOrganizer) Retry(_ context.Context, id uint) (*organize.Result, error) {
	f.mu.Lock()
	f.retried = append(f.retried, id)
	ok := f.retryOK
	f.mu.Unlock()
	if ok {
		return &organize.Result{}, f.db.Model(&models.MediaTransferHistory{}).Where("id = ?", id).Update("status", models.MediaTransferDone).Error
	}
	return &organize.Result{}, nil
}

type fakeDL struct {
	downloader.Downloader
	mu      sync.Mutex
	removed []string
	// torrents 是下载器里的种子（追剧认领用）；listErr 不为空时列不出来
	torrents []downloader.Torrent
	listErr  error
	lists    int
}

func (f *fakeDL) GetAllTorrents() ([]downloader.Torrent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]downloader.Torrent(nil), f.torrents...), nil
}

func (f *fakeDL) setTorrents(list ...downloader.Torrent) {
	f.mu.Lock()
	f.torrents = list
	f.mu.Unlock()
}

func (f *fakeDL) RemoveTorrent(id string, removeData bool) error {
	f.mu.Lock()
	f.removed = append(f.removed, fmt.Sprintf("%s:%v", id, removeData))
	f.mu.Unlock()
	return nil
}

type fakeDLs struct{ dl *fakeDL }

func (f fakeDLs) Get(_ context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error) {
	return f.dl, models.DownloaderSetting{ID: id, Name: "qb"}, nil
}

type env struct {
	t       *testing.T
	ctx     context.Context
	db      *gorm.DB
	tmdb    *tmdbOverride
	svc     *Service
	search  *fakeSearch
	site    *fakeSite
	org     *fakeOrganizer
	dl      *fakeDL
	mu      sync.Mutex
	now     time.Time
	pushes  []ptinternal.PushTorrentRequest
	pushErr error
	// pushRes 不为空时假推送返回它（例如闸门拒绝、下载器里已经有）
	pushRes *ptinternal.PushTorrentResult
	// onPush 在每次推送时调用（模拟推送期间改了设置）
	onPush  func(req ptinternal.PushTorrentRequest)
	notices []Notice
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

func (e *env) gotPushes() []ptinternal.PushTorrentRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]ptinternal.PushTorrentRequest(nil), e.pushes...)
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := newDB(t)
	over := &tmdbOverride{}
	tm := newFakeTMDBWith(t, over)
	rec := recognize.New(recognize.Config{DB: db, Cipher: fakeCipher{}, BaseURL: tm.URL + "/3", ImageBaseURL: tm.URL + "/t/p/", RatePerSecond: 1000})
	key := tmdbKey
	_, err := rec.SaveSettings(context.Background(), recognize.SettingsInput{TMDBKey: &key})
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://qb", Enabled: true, IsDefault: true}).Error)
	e := &env{
		t: t, ctx: context.Background(), db: db, tmdb: over, search: &fakeSearch{}, site: &fakeSite{files: map[string][]byte{}},
		dl: &fakeDL{}, now: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC),
	}
	e.org = &fakeOrganizer{db: db}
	e.svc = New(Config{
		DB: db, Recognizer: rec, Search: e.search,
		Sites: func(name string) (v2.Site, bool) { return e.site, name == "hdsky" || name == "mteam" },
		Push: func(_ context.Context, req ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error) {
			e.mu.Lock()
			hook := e.onPush
			e.mu.Unlock()
			if hook != nil {
				hook(req)
			}
			e.mu.Lock()
			defer e.mu.Unlock()
			if e.pushErr != nil {
				return nil, e.pushErr
			}
			e.pushes = append(e.pushes, req)
			if e.pushRes != nil {
				return e.pushRes, nil
			}
			// 和真的推送一样记下种子（删旧种子前要查它的 H&R）
			info := models.TorrentInfo{SiteName: req.SiteID, TorrentID: req.TorrentID, Title: req.Title, DownloadSource: req.Source}
			if req.Meta != nil {
				info.HasHR = req.Meta.HasHR
			}
			if err := db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&info).Error; err != nil {
				return nil, err
			}
			return &ptinternal.PushTorrentResult{Success: true}, nil
		},
		Downloaders: fakeDLs{dl: e.dl}, Organizer: e.org, Now: e.Now,
		Jitter: func(time.Duration) time.Duration { return 0 },
		Notify: func(_ context.Context, n Notice) error {
			e.mu.Lock()
			e.notices = append(e.notices, n)
			e.mu.Unlock()
			return nil
		},
	})
	return e
}

// item 是站点上的一个种子；文件同时放进假站点。
func (e *env) item(site, id, title, sub string, sizeGB float64, seeders int) v2.TorrentItem {
	e.site.files[id] = torrentBytes(title)
	return v2.TorrentItem{ID: id, Title: title, Subtitle: sub, SourceSite: site, SizeBytes: int64(sizeGB * float64(gib)), Seeders: seeders}
}

func (e *env) hashOf(id string) string {
	h, err := qbit.ComputeTorrentHash(e.site.files[id])
	require.NoError(e.t, err)
	return strings.ToLower(h)
}

func (e *env) enable(mod func(*Settings)) Settings {
	e.t.Helper()
	in := Settings{Enabled: true}
	if mod != nil {
		mod(&in)
	}
	set, err := e.svc.SaveSettings(e.ctx, in)
	require.NoError(e.t, err)
	return set
}

func (e *env) sub(in SubscriptionInput) *models.MediaSubscription {
	e.t.Helper()
	s, err := e.svc.CreateSubscription(e.ctx, in, models.MediaSubFromManual)
	require.NoError(e.t, err)
	return s
}

func (e *env) subRow(id uint) models.MediaSubscription {
	e.t.Helper()
	r, err := e.svc.subRow(e.ctx, id)
	require.NoError(e.t, err)
	return r
}

func (e *env) linked(id uint) []models.MediaSubscriptionTorrent {
	e.t.Helper()
	rows, err := e.svc.torrents(e.ctx, id)
	require.NoError(e.t, err)
	return rows
}

// organized 记一条整理好的记录（模拟整理入库）。
func (e *env) organized(hash, kind string, tmdbID, season, episode int, target string) models.MediaTransferHistory {
	e.t.Helper()
	row := models.MediaTransferHistory{
		InfoHash: hash, SourcePath: fmt.Sprintf("/dl/%s/%d", hash, episode), TargetPath: target, MediaType: kind, TMDBID: tmdbID,
		Season: season, Episode: episode, Status: models.MediaTransferDone, Mode: models.MediaModeHardlink,
	}
	require.NoError(e.t, e.db.Create(&row).Error)
	return row
}
