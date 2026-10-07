package organize

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
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

// fakeTMDB 认 tmdbKey：奥本海默（电影）、最后生还者（剧集，第 1 季两集）、葬送的芙莉莲（动画剧集）能搜到；
// /t/p/ 下回图片。down 为真时接口回 503。
type fakeTMDB struct {
	*httptest.Server
	mu   sync.Mutex
	down bool
	hits map[string]int
}

func (f *fakeTMDB) setDown(v bool) {
	f.mu.Lock()
	f.down = v
	f.mu.Unlock()
}

func newFakeTMDB(t *testing.T) *fakeTMDB {
	f := &fakeTMDB{hits: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits[r.URL.Path]++
		down := f.down
		f.mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/t/p/") {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("JPEG" + r.URL.Path))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if down {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.URL.Query().Get("api_key") != tmdbKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		q := r.URL.Query().Get("query")
		switch {
		case r.URL.Path == "/3/search/movie" && (strings.Contains(q, "Oppenheimer") || strings.Contains(q, "奥本海默")):
			_, _ = w.Write([]byte(`{"results":[{"id":872585,"title":"奥本海默","original_title":"Oppenheimer","release_date":"2023-07-19","popularity":300,"genre_ids":[18,36]}]}`))
		case r.URL.Path == "/3/movie/872585":
			_, _ = w.Write([]byte(`{"id":872585,"title":"奥本海默","original_title":"Oppenheimer","release_date":"2023-07-19","overview":"原子弹之父的故事。",
				"imdb_id":"tt15398776","poster_path":"/opp.jpg","backdrop_path":"/opp-bd.jpg","vote_average":8.1,"runtime":181,"genres":[{"id":18,"name":"剧情"},{"id":36,"name":"历史"}]}`))
		case r.URL.Path == "/3/search/tv" && (strings.Contains(q, "Last of Us") || strings.Contains(q, "最后生还者")):
			_, _ = w.Write([]byte(`{"results":[{"id":100088,"name":"最后生还者","original_name":"The Last of Us","first_air_date":"2023-01-15","popularity":500,"genre_ids":[18]}]}`))
		case r.URL.Path == "/3/tv/100088":
			_, _ = w.Write([]byte(`{"id":100088,"name":"最后生还者","original_name":"The Last of Us","first_air_date":"2023-01-15","overview":"末日之后。",
				"poster_path":"/tlou.jpg","backdrop_path":"/tlou-bd.jpg","number_of_seasons":2,"genres":[{"id":18,"name":"剧情"}],"external_ids":{"imdb_id":"tt3581920"}}`))
		case r.URL.Path == "/3/tv/100088/season/1":
			_, _ = w.Write([]byte(`{"season_number":1,"name":"第 1 季","poster_path":"/tlou-s1.jpg","air_date":"2023-01-15",
				"episodes":[{"id":11,"episode_number":1,"name":"当你迷失在黑暗中","still_path":"/e1.jpg","air_date":"2023-01-15"},
				{"id":12,"episode_number":2,"name":"感染","still_path":"/e2.jpg","air_date":"2023-01-22"}]}`))
		case r.URL.Path == "/3/search/tv" && strings.Contains(q, "Frieren"):
			_, _ = w.Write([]byte(`{"results":[{"id":209867,"name":"葬送的芙莉莲","original_name":"葬送のフリーレン","first_air_date":"2023-09-29","popularity":200,"genre_ids":[16,10759]}]}`))
		case r.URL.Path == "/3/tv/209867" && r.URL.Query().Get("language") == "en-US":
			_, _ = w.Write([]byte(`{"id":209867,"name":"Frieren: Beyond Journey's End","alternative_titles":{"results":[{"title":"Sousou no Frieren"}]}}`))
		case r.URL.Path == "/3/tv/209867":
			_, _ = w.Write([]byte(`{"id":209867,"name":"葬送的芙莉莲","original_name":"葬送のフリーレン","first_air_date":"2023-09-29","genres":[{"id":16,"name":"动画"}]}`))
		case strings.HasPrefix(r.URL.Path, "/3/search/"):
			_, _ = w.Write([]byte(`{"results":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// fakeDL 是假的下载器：GetTorrent、GetTorrentFiles 与 GetAllTorrents 按表里的种子回。
type fakeDL struct {
	downloader.Downloader
	mu       sync.Mutex
	torrents map[string]downloader.Torrent
	files    map[string][]downloader.TorrentFile
	listErr  error
}

func (f *fakeDL) GetTorrent(id string) (downloader.Torrent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.torrents {
		if t.ID == id || strings.EqualFold(t.InfoHash, id) {
			return t, nil
		}
	}
	return downloader.Torrent{}, downloader.ErrTorrentNotFound
}

func (f *fakeDL) GetTorrentFiles(id string) ([]downloader.TorrentFile, error) {
	t, err := f.GetTorrent(id)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.files[t.InfoHash], nil
}

func (f *fakeDL) GetAllTorrents() ([]downloader.Torrent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]downloader.Torrent, 0, len(f.torrents))
	for _, t := range f.torrents {
		out = append(out, t)
	}
	return out, nil
}

func (f *fakeDL) remove(hash string) {
	f.mu.Lock()
	delete(f.torrents, hash)
	f.mu.Unlock()
}

// fakeDLs 是 Downloaders：一个叫 qb 的下载器（ID 1）。
type fakeDLs struct {
	dl      *fakeDL
	setting models.DownloaderSetting
}

func (f *fakeDLs) Get(_ context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error) {
	if id != f.setting.ID {
		return nil, models.DownloaderSetting{}, errors.New("下载器不存在")
	}
	return f.dl, f.setting, nil
}

func (f *fakeDLs) ByName(_ context.Context, name string) (downloader.Downloader, models.DownloaderSetting, error) {
	if name != f.setting.Name {
		return nil, models.DownloaderSetting{}, errors.New("下载器不存在")
	}
	return f.dl, f.setting, nil
}

func (f *fakeDLs) List(context.Context) ([]models.DownloaderSetting, error) {
	return []models.DownloaderSetting{f.setting}, nil
}

type env struct {
	t        *testing.T
	ctx      context.Context
	db       *gorm.DB
	svc      *Service
	rec      *recognize.Service
	tmdb     *fakeTMDB
	dl       *fakeDL
	dls      *fakeDLs
	root     string // 临时目录
	dlDir    string // pt-tools 里看到的下载目录
	movies   string
	tv       string
	anime    string
	mu       sync.Mutex
	notices  []Notice
	now      time.Time
	notifyEr error
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

func (e *env) gotNotices() []Notice {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Notice(nil), e.notices...)
}

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "t.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{}, &models.MediaCache{},
		&models.MediaLibrary{}, &models.MediaPathMap{}, &models.MediaServer{}, &models.MediaOrganizeSetting{},
		&models.MediaTransferHistory{}, &models.TorrentInfo{}, &models.DownloaderSetting{},
	))
	return db
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{t: t, ctx: context.Background(), db: newDB(t), tmdb: newFakeTMDB(t), now: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)}
	e.root = t.TempDir()
	e.dlDir = filepath.Join(e.root, "downloads")
	e.movies, e.tv, e.anime = filepath.Join(e.root, "media", "movies"), filepath.Join(e.root, "media", "tv"), filepath.Join(e.root, "media", "anime")
	for _, d := range []string{e.dlDir, e.movies, e.tv, e.anime} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	e.rec = recognize.New(recognize.Config{DB: e.db, Cipher: fakeCipher{}, BaseURL: e.tmdb.URL + "/3", ImageBaseURL: e.tmdb.URL + "/t/p/", RatePerSecond: 1000})
	key := tmdbKey
	_, err := e.rec.SaveSettings(e.ctx, recognize.SettingsInput{TMDBKey: &key})
	require.NoError(t, err)
	e.dl = &fakeDL{torrents: map[string]downloader.Torrent{}, files: map[string][]downloader.TorrentFile{}}
	setting := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://qb", Enabled: true}
	require.NoError(t, e.db.Create(&setting).Error)
	e.dls = &fakeDLs{dl: e.dl, setting: setting}
	e.svc = New(Config{
		DB: e.db, Cipher: fakeCipher{}, Recognizer: e.rec, Downloaders: e.dls, Now: e.Now,
		Notify: func(_ context.Context, n Notice) error {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.notices = append(e.notices, n)
			return e.notifyEr
		},
	})
	// 下载器看到的保存目录是 /downloads，pt-tools 里是临时目录
	_, err = e.svc.SavePathMap(e.ctx, 0, PathMapInput{DownloaderID: setting.ID, DownloaderPrefix: "/downloads", LocalPrefix: e.dlDir})
	require.NoError(t, err)
	return e
}

func (e *env) library(in LibraryInput) LibraryView {
	e.t.Helper()
	in.Enabled = true
	v, err := e.svc.SaveLibrary(e.ctx, 0, in)
	require.NoError(e.t, err)
	return v
}

func (e *env) defaultLibraries() {
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true})
}

// addTorrent 在下载目录里建好文件，再把种子放进假下载器（保存目录 /downloads）。sizes 是每个文件的大小（MB）。
func (e *env) addTorrent(hash, name string, files map[string]int, mod func(*downloader.Torrent)) downloader.Torrent {
	e.t.Helper()
	var tfs []downloader.TorrentFile
	var total int64
	for rel, mbs := range files {
		p := filepath.Join(e.dlDir, filepath.FromSlash(rel))
		require.NoError(e.t, os.MkdirAll(filepath.Dir(p), 0o755))
		f, err := os.Create(p)
		require.NoError(e.t, err)
		require.NoError(e.t, f.Truncate(int64(mbs)<<20))
		require.NoError(e.t, f.Close())
		tfs = append(tfs, downloader.TorrentFile{Name: rel, Size: int64(mbs) << 20, Progress: 1})
		total += int64(mbs) << 20
	}
	t := downloader.Torrent{
		ID: hash, InfoHash: hash, Name: name, Progress: 1, IsCompleted: true, SavePath: "/downloads",
		ContentPath: "/downloads/" + name, State: downloader.TorrentSeeding, TotalSize: total, Category: "movies",
	}
	if mod != nil {
		mod(&t)
	}
	e.dl.mu.Lock()
	e.dl.torrents[hash] = t
	e.dl.files[hash] = tfs
	e.dl.mu.Unlock()
	return t
}

func (e *env) history() []models.MediaTransferHistory {
	e.t.Helper()
	var rows []models.MediaTransferHistory
	require.NoError(e.t, e.db.Order("id").Find(&rows).Error)
	return rows
}

func (e *env) settings(in SettingsInput) Settings {
	e.t.Helper()
	v, err := e.svc.SaveSettings(e.ctx, in)
	require.NoError(e.t, err)
	return v
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return string(b)
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func sameFile(t *testing.T, a, b string) bool {
	t.Helper()
	ai, err := os.Stat(a)
	require.NoError(t, err)
	bi, err := os.Stat(b)
	require.NoError(t, err)
	return os.SameFile(ai, bi)
}

// fakeEmby 记下收到的路径通知。
type fakeEmby struct {
	*httptest.Server
	mu    sync.Mutex
	paths []string
}

func newFakeEmby(t *testing.T) *fakeEmby {
	f := &fakeEmby{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "embykey" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/System/Info":
			_, _ = w.Write([]byte(`{"ServerName":"Emby","Version":"4.8"}`))
		case "/Library/Media/Updated":
			b, _ := io.ReadAll(r.Body)
			f.mu.Lock()
			f.paths = append(f.paths, string(b))
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeEmby) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...)
}
